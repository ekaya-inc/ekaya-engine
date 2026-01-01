package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// WorkItems represents tasks that need to be enqueued by the orchestrator.
type WorkItems struct {
	ScanTasks    []string // entity keys that need scanning
	AnalyzeTasks []string // entity keys that need analyzing
}

// DefaultPollInterval is the default interval for polling entity states.
const DefaultPollInterval = 2 * time.Second

// Orchestrator coordinates ontology extraction workflows by reading entity states
// and determining what work needs to be done (scanning, analyzing, etc.).
type Orchestrator struct {
	// Core dependencies
	stateRepo    repositories.WorkflowStateRepository
	getTenantCtx TenantContextFunc
	logger       *zap.Logger

	// Queue for enqueuing tasks
	queue *workqueue.Queue

	// Configuration
	pollInterval time.Duration

	// Dependencies for task creation
	workflowRepo   repositories.OntologyWorkflowRepository
	schemaRepo     repositories.SchemaRepository
	ontologyRepo   repositories.OntologyRepository
	questionRepo   repositories.OntologyQuestionRepository
	dsSvc          DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	builder        OntologyBuilderService
	workflowSvc    OntologyWorkflowService
}

// NewOrchestrator creates a new workflow orchestrator.
// If pollInterval is 0, DefaultPollInterval is used.
func NewOrchestrator(
	stateRepo repositories.WorkflowStateRepository,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
	queue *workqueue.Queue,
	workflowRepo repositories.OntologyWorkflowRepository,
	schemaRepo repositories.SchemaRepository,
	ontologyRepo repositories.OntologyRepository,
	questionRepo repositories.OntologyQuestionRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	builder OntologyBuilderService,
	workflowSvc OntologyWorkflowService,
	pollInterval time.Duration,
) *Orchestrator {
	if pollInterval == 0 {
		pollInterval = DefaultPollInterval
	}
	return &Orchestrator{
		stateRepo:      stateRepo,
		getTenantCtx:   getTenantCtx,
		logger:         logger.Named("orchestrator"),
		queue:          queue,
		pollInterval:   pollInterval,
		workflowRepo:   workflowRepo,
		schemaRepo:     schemaRepo,
		ontologyRepo:   ontologyRepo,
		questionRepo:   questionRepo,
		dsSvc:          dsSvc,
		adapterFactory: adapterFactory,
		builder:        builder,
		workflowSvc:    workflowSvc,
	}
}

// Run executes the orchestration loop for a workflow.
// Polls entity states and enqueues tasks until all entities complete.
func (o *Orchestrator) Run(ctx context.Context, projectID, workflowID uuid.UUID) error {
	o.logger.Info("Starting orchestrator",
		zap.String("workflow_id", workflowID.String()))

	for {
		// Check context cancellation
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled: %w", err)
		}

		// Get tenant context for database access
		tenantCtx, cleanup, err := o.getTenantCtx(ctx, projectID)
		if err != nil {
			return fmt.Errorf("get tenant context: %w", err)
		}

		// Read current entity states
		states, err := o.stateRepo.ListByWorkflow(tenantCtx, workflowID)
		if err != nil {
			cleanup()
			return fmt.Errorf("list workflow states: %w", err)
		}

		// Determine what work is needed (table entities only)
		items := o.determineWorkFromStates(states)

		// Update progress based on current entity states
		o.updateProgress(tenantCtx, workflowID, states, items)

		// Release DB connection before potentially waiting
		cleanup()

		// Check if we should trigger global entity analysis
		// This happens when all table entities are complete
		globalTriggered, err := o.maybeEnqueueGlobalTask(ctx, projectID, workflowID, states)
		if err != nil {
			return fmt.Errorf("check global entity: %w", err)
		}

		// If no work for tables and no global task triggered
		if len(items.ScanTasks) == 0 && len(items.AnalyzeTasks) == 0 && !globalTriggered {
			if o.isAllComplete(states) {
				o.logger.Info("Workflow complete - all entities in terminal state",
					zap.String("workflow_id", workflowID.String()))
				return o.finalizeWorkflow(ctx, projectID, workflowID)
			}

			// No work to do but not all complete (e.g., waiting for input or in-progress)
			o.logger.Debug("No work to enqueue - waiting for in-progress tasks or user input",
				zap.String("workflow_id", workflowID.String()))
			time.Sleep(o.pollInterval)
			continue
		}

		// Enqueue tasks for pending/scanned entities
		if len(items.ScanTasks) > 0 || len(items.AnalyzeTasks) > 0 {
			o.logger.Info("Enqueuing tasks",
				zap.String("workflow_id", workflowID.String()),
				zap.Int("scan_tasks", len(items.ScanTasks)),
				zap.Int("analyze_tasks", len(items.AnalyzeTasks)))

			if err := o.enqueueTasks(ctx, projectID, workflowID, items); err != nil {
				return fmt.Errorf("enqueue tasks: %w", err)
			}
		}

		// Wait for enqueued tasks to complete before next iteration
		waitErr := o.queue.Wait(ctx)

		// Update progress AFTER tasks complete (critical for accurate progress reporting)
		tenantCtx2, cleanup2, err := o.getTenantCtx(ctx, projectID)
		if err != nil {
			return fmt.Errorf("get tenant context for progress update: %w", err)
		}
		updatedStates, err := o.stateRepo.ListByWorkflow(tenantCtx2, workflowID)
		if err != nil {
			cleanup2()
			return fmt.Errorf("list workflow states for progress update: %w", err)
		}
		updatedItems := o.determineWorkFromStates(updatedStates)
		o.updateProgress(tenantCtx2, workflowID, updatedStates, updatedItems)
		cleanup2()

		// Handle wait errors gracefully - don't fail the entire workflow for individual task failures
		if waitErr != nil {
			// Context cancellation is fatal
			if ctx.Err() != nil {
				return fmt.Errorf("context cancelled during wait: %w", waitErr)
			}

			// Log the task failure but continue processing
			// The failed task's entity will be in "failed" state, and the workflow can still complete
			// with partial results (other entities may succeed)
			o.logger.Warn("Task failed during execution - continuing with remaining tasks",
				zap.String("workflow_id", workflowID.String()),
				zap.Error(waitErr))

			// Continue the loop - the failed entity is now in terminal state (failed)
			// and won't be retried. Other entities can still complete.
		}
	}
}

// determineWorkFromStates examines entity states and determines what work is needed.
// Only considers TABLE entities - global and column entities are handled separately.
func (o *Orchestrator) determineWorkFromStates(states []*models.WorkflowEntityState) *WorkItems {
	items := &WorkItems{
		ScanTasks:    make([]string, 0),
		AnalyzeTasks: make([]string, 0),
	}

	for _, state := range states {
		// Only process table entities - global and column entities are handled separately
		if state.EntityType != models.WorkflowEntityTypeTable {
			continue
		}

		switch state.Status {
		case models.WorkflowEntityStatusPending:
			// Pending table entities need to start scanning
			items.ScanTasks = append(items.ScanTasks, state.EntityKey)

		case models.WorkflowEntityStatusScanned:
			// Scanned table entities need to start analyzing
			items.AnalyzeTasks = append(items.AnalyzeTasks, state.EntityKey)

		case models.WorkflowEntityStatusScanning,
			models.WorkflowEntityStatusAnalyzing,
			models.WorkflowEntityStatusNeedsInput,
			models.WorkflowEntityStatusComplete,
			models.WorkflowEntityStatusFailed:
			// No action needed for these states
		}
	}

	return items
}

// isAllComplete returns true if all entities are in terminal state.
func (o *Orchestrator) isAllComplete(states []*models.WorkflowEntityState) bool {
	for _, state := range states {
		if !state.Status.IsTerminal() {
			return false
		}
	}
	return true
}

// allTablesComplete returns true if all table entities are complete.
func (o *Orchestrator) allTablesComplete(states []*models.WorkflowEntityState) bool {
	for _, state := range states {
		if state.EntityType != models.WorkflowEntityTypeTable {
			continue
		}
		if state.Status != models.WorkflowEntityStatusComplete {
			return false
		}
	}
	return true
}

// maybeEnqueueGlobalTask checks if all table entities are complete and
// triggers the global entity analysis (BuildTieredOntologyTask) if so.
// Returns true if a global task was enqueued.
func (o *Orchestrator) maybeEnqueueGlobalTask(ctx context.Context, projectID, workflowID uuid.UUID, states []*models.WorkflowEntityState) (bool, error) {
	// Find the global entity
	var globalState *models.WorkflowEntityState
	for _, state := range states {
		if state.EntityType == models.WorkflowEntityTypeGlobal {
			globalState = state
			break
		}
	}

	// If no global entity or already processing/complete, nothing to do
	if globalState == nil {
		return false, nil
	}
	if globalState.Status != models.WorkflowEntityStatusPending {
		return false, nil
	}

	// Check if all table entities are complete
	if !o.allTablesComplete(states) {
		return false, nil
	}

	o.logger.Info("All table entities complete - triggering global analysis",
		zap.String("workflow_id", workflowID.String()))

	// Transition global entity: pending -> scanned -> analyzing
	tenantCtx, cleanup, err := o.getTenantCtx(ctx, projectID)
	if err != nil {
		return false, fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Update to scanned (skip scanning phase - no data to scan for global)
	if err := o.stateRepo.UpdateStatus(tenantCtx, globalState.ID, models.WorkflowEntityStatusScanned, nil); err != nil {
		return false, fmt.Errorf("update global entity to scanned: %w", err)
	}

	// Update to analyzing
	if err := o.stateRepo.UpdateStatus(tenantCtx, globalState.ID, models.WorkflowEntityStatusAnalyzing, nil); err != nil {
		return false, fmt.Errorf("update global entity to analyzing: %w", err)
	}

	// Enqueue BuildTieredOntologyTask
	task := NewBuildTieredOntologyTask(
		o.builder,
		o.getTenantCtx,
		projectID,
		workflowID,
	)
	o.queue.Enqueue(task)

	return true, nil
}

// tableInfo holds cached table info for task creation.
type tableInfo struct {
	table       *models.SchemaTable
	columnNames []string
}

// enqueueTasks creates and enqueues tasks for entities that need work.
func (o *Orchestrator) enqueueTasks(ctx context.Context, projectID, workflowID uuid.UUID, items *WorkItems) error {
	// Get tenant context to load table/column info
	tenantCtx, cleanup, err := o.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get workflow to get datasource ID
	workflow, err := o.workflowRepo.GetByID(tenantCtx, workflowID)
	if err != nil {
		return fmt.Errorf("get workflow: %w", err)
	}
	datasourceID := workflow.Config.DatasourceID

	// Load table info cache for all tables we need
	tableInfoCache, err := o.loadTableInfoCache(tenantCtx, projectID, datasourceID, items)
	if err != nil {
		return fmt.Errorf("load table info: %w", err)
	}

	// Enqueue scan tasks for pending table entities
	for _, entityKey := range items.ScanTasks {
		info, ok := tableInfoCache[entityKey]
		if !ok {
			return fmt.Errorf("table not found in cache: %s", entityKey)
		}
		task := NewScanTableDataTask(
			o.ontologyRepo,
			o.stateRepo,
			o.dsSvc,
			o.adapterFactory,
			o.getTenantCtx,
			projectID,
			workflowID,
			datasourceID,
			info.table.TableName,
			info.table.SchemaName,
			info.columnNames,
		)
		o.queue.Enqueue(task)
	}

	// Enqueue analyze tasks for scanned table entities
	for _, entityKey := range items.AnalyzeTasks {
		info, ok := tableInfoCache[entityKey]
		if !ok {
			return fmt.Errorf("table not found in cache: %s", entityKey)
		}
		task := NewUnderstandEntityTask(
			o.ontologyRepo,
			o.questionRepo,
			o.stateRepo,
			o.builder,
			o.workflowSvc,
			o.getTenantCtx,
			projectID,
			workflowID,
			workflow.OntologyID,
			info.table.TableName,
			info.table.SchemaName,
			info.columnNames,
		)
		o.queue.Enqueue(task)
	}

	return nil
}

// loadTableInfoCache loads table and column info for the tables we need to process.
func (o *Orchestrator) loadTableInfoCache(ctx context.Context, projectID, datasourceID uuid.UUID, items *WorkItems) (map[string]*tableInfo, error) {
	// Collect all table names we need
	neededTables := make(map[string]bool)
	for _, entityKey := range items.ScanTasks {
		neededTables[entityKey] = true
	}
	for _, entityKey := range items.AnalyzeTasks {
		neededTables[entityKey] = true
	}

	if len(neededTables) == 0 {
		return make(map[string]*tableInfo), nil
	}

	// Load all tables for this datasource
	tables, err := o.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	// Build table by name map
	tableByName := make(map[string]*models.SchemaTable)
	for _, t := range tables {
		tableByName[t.TableName] = t
	}

	// Load columns for each needed table
	cache := make(map[string]*tableInfo)
	for tableName := range neededTables {
		table, ok := tableByName[tableName]
		if !ok {
			return nil, fmt.Errorf("table not found: %s", tableName)
		}

		columns, err := o.schemaRepo.ListColumnsByTable(ctx, projectID, table.ID)
		if err != nil {
			return nil, fmt.Errorf("list columns for %s: %w", tableName, err)
		}

		columnNames := make([]string, len(columns))
		for i, col := range columns {
			columnNames[i] = col.ColumnName
		}

		cache[tableName] = &tableInfo{
			table:       table,
			columnNames: columnNames,
		}
	}

	return cache, nil
}

// finalizeWorkflow is called when all entities are complete.
// It writes a clean ontology by stripping workflow fields (Status, ScanData),
// then deletes ephemeral workflow state rows.
func (o *Orchestrator) finalizeWorkflow(ctx context.Context, projectID, workflowID uuid.UUID) error {
	o.logger.Info("Finalizing workflow - writing clean ontology",
		zap.String("workflow_id", workflowID.String()))

	// Get tenant context for database access
	tenantCtx, cleanup, err := o.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get entity count BEFORE deleting state (for accurate final progress)
	states, err := o.stateRepo.ListByWorkflow(tenantCtx, workflowID)
	if err != nil {
		o.logger.Warn("Failed to get entity count for final progress", zap.Error(err))
	}
	entityCount := len(states)

	// Update progress with final entity count before deletion
	if entityCount > 0 {
		_ = o.workflowRepo.UpdateProgress(tenantCtx, workflowID, &models.WorkflowProgress{
			CurrentPhase: models.WorkflowPhaseCompleting,
			Message:      "Finalizing ontology...",
			Current:      entityCount,
			Total:        entityCount,
		})
	}

	// Write clean ontology (strips workflow fields)
	if err := o.ontologyRepo.WriteCleanOntology(tenantCtx, projectID); err != nil {
		return fmt.Errorf("write clean ontology: %w", err)
	}

	o.logger.Info("Clean ontology written successfully",
		zap.String("workflow_id", workflowID.String()))

	// NOTE: We intentionally preserve workflow_state after completion.
	// The gathered data (sample_values, distinct_count, etc.) is valuable for:
	// - assess-deterministic tool to verify input preparation quality
	// - Debugging and auditing extraction runs
	// Cleanup happens when a NEW extraction starts, deleting state for the
	// previous ontology (see ontology_workflow.go StartExtraction).

	return nil
}

// updateProgress updates workflow progress based on current entity states.
func (o *Orchestrator) updateProgress(ctx context.Context, workflowID uuid.UUID, states []*models.WorkflowEntityState, items *WorkItems) {
	// In simplified flow, we only have the global entity
	// Count completed and total entities
	completed := 0
	total := len(states)
	for _, state := range states {
		if state.Status == models.WorkflowEntityStatusComplete {
			completed++
		}
	}

	// Determine the current phase based on remaining work
	// Simplified flow: no scanning phase, go straight to Tier1Building
	phase := models.WorkflowPhaseTier1Building
	if len(items.ScanTasks) == 0 && len(items.AnalyzeTasks) == 0 && completed == total && total > 0 {
		phase = models.WorkflowPhaseTier1Building
	}

	// Build message - simplified for global-only flow
	message := "Building ontology from entities and relationships..."
	if completed == total && total > 0 {
		message = "Ontology build complete"
	}

	// Update progress (ignore errors - progress is non-critical)
	_ = o.workflowSvc.UpdateProgress(ctx, workflowID, &models.WorkflowProgress{
		CurrentPhase: phase,
		Message:      message,
		Current:      completed,
		Total:        total,
	})
}
