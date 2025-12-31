package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// CandidateCounts holds counts of candidates by status.
type CandidateCounts struct {
	Confirmed       int
	NeedsReview     int
	Rejected        int
	EntityCount     int
	OccurrenceCount int
	IslandCount     int
	CanSave         bool
}

// CandidatesGrouped holds candidates grouped by status.
type CandidatesGrouped struct {
	Confirmed   []*models.RelationshipCandidate
	NeedsReview []*models.RelationshipCandidate
	Rejected    []*models.RelationshipCandidate
}

// EntityWithOccurrences represents a discovered entity with its occurrences.
type EntityWithOccurrences struct {
	Entity      *models.OntologyEntity
	Occurrences []*models.OntologyEntityOccurrence
}

// RelationshipWorkflowService provides operations for relationship discovery workflow management.
type RelationshipWorkflowService interface {
	// StartDetection initiates a new relationship detection workflow.
	StartDetection(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetStatus returns the current workflow status for a datasource.
	GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetStatusWithCounts returns workflow status with candidate counts.
	GetStatusWithCounts(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, *CandidateCounts, error)

	// GetByID returns a specific workflow by its ID.
	GetByID(ctx context.Context, workflowID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetCandidatesGrouped returns candidates grouped by status for a datasource.
	GetCandidatesGrouped(ctx context.Context, datasourceID uuid.UUID) (*CandidatesGrouped, error)

	// GetEntitiesWithOccurrences returns discovered entities with their occurrences for a datasource.
	GetEntitiesWithOccurrences(ctx context.Context, datasourceID uuid.UUID) ([]*EntityWithOccurrences, error)

	// UpdateCandidateDecision updates a candidate's decision, verifying it belongs to the datasource.
	UpdateCandidateDecision(ctx context.Context, datasourceID, candidateID uuid.UUID, decision string) (*models.RelationshipCandidate, error)

	// Cancel cancels a running workflow.
	Cancel(ctx context.Context, workflowID uuid.UUID) error

	// SaveRelationships saves accepted candidates as relationships and marks workflow complete.
	// Returns the count of saved relationships.
	SaveRelationships(ctx context.Context, workflowID uuid.UUID) (int, error)

	// UpdateProgress updates the workflow progress.
	UpdateProgress(ctx context.Context, workflowID uuid.UUID, progress *models.WorkflowProgress) error

	// MarkComplete marks a workflow as completed.
	MarkComplete(ctx context.Context, workflowID uuid.UUID) error

	// MarkFailed marks a workflow as failed with an error message.
	MarkFailed(ctx context.Context, workflowID uuid.UUID, errMsg string) error

	// Shutdown gracefully stops all active workflows owned by this server.
	Shutdown(ctx context.Context) error
}

// heartbeatInfo holds info needed for heartbeat goroutine
type relationshipHeartbeatInfo struct {
	projectID uuid.UUID
	stop      chan struct{}
}

type relationshipWorkflowService struct {
	workflowRepo     repositories.OntologyWorkflowRepository
	candidateRepo    repositories.RelationshipCandidateRepository
	schemaRepo       repositories.SchemaRepository
	stateRepo        repositories.WorkflowStateRepository
	ontologyRepo     repositories.OntologyRepository
	entityRepo       repositories.OntologyEntityRepository
	dsSvc            DatasourceService
	adapterFactory   datasource.DatasourceAdapterFactory
	llmFactory       llm.LLMClientFactory
	discoveryService RelationshipDiscoveryService
	getTenantCtx     TenantContextFunc
	logger           *zap.Logger
	serverInstanceID uuid.UUID
	activeQueues     sync.Map // workflowID -> *workqueue.Queue
	taskQueueWriters sync.Map // workflowID -> *taskQueueWriter
	heartbeatStop    sync.Map // workflowID -> *relationshipHeartbeatInfo
}

// NewRelationshipWorkflowService creates a new relationship workflow service.
func NewRelationshipWorkflowService(
	workflowRepo repositories.OntologyWorkflowRepository,
	candidateRepo repositories.RelationshipCandidateRepository,
	schemaRepo repositories.SchemaRepository,
	stateRepo repositories.WorkflowStateRepository,
	ontologyRepo repositories.OntologyRepository,
	entityRepo repositories.OntologyEntityRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	discoveryService RelationshipDiscoveryService,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) RelationshipWorkflowService {
	serverID := uuid.New()
	namedLogger := logger.Named("relationship-workflow")
	namedLogger.Info("Relationship workflow service initialized", zap.String("server_instance_id", serverID.String()))

	return &relationshipWorkflowService{
		workflowRepo:     workflowRepo,
		candidateRepo:    candidateRepo,
		schemaRepo:       schemaRepo,
		stateRepo:        stateRepo,
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		dsSvc:            dsSvc,
		adapterFactory:   adapterFactory,
		llmFactory:       llmFactory,
		discoveryService: discoveryService,
		getTenantCtx:     getTenantCtx,
		logger:           namedLogger,
		serverInstanceID: serverID,
	}
}

var _ RelationshipWorkflowService = (*relationshipWorkflowService)(nil)

func (s *relationshipWorkflowService) StartDetection(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyWorkflow, error) {
	// Step 1: Check for existing workflow for this datasource in relationships phase
	existing, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseRelationships)
	if err != nil {
		s.logger.Error("Failed to check existing workflow",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, err
	}

	// If there's an active workflow, don't start a new one
	if existing != nil && !existing.State.IsTerminal() {
		return nil, fmt.Errorf("relationship detection already in progress for this datasource")
	}

	// Step 1.5: Check that entities exist (entity discovery must run first)
	entities, err := s.entityRepo.GetByProject(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to check for existing entities",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("check existing entities: %w", err)
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("no entities found - run entity discovery first")
	}

	s.logger.Info("Found existing entities for relationship detection",
		zap.String("project_id", projectID.String()),
		zap.Int("entity_count", len(entities)))

	// Step 2: Get or create ontology for this project
	// Reuse existing active ontology if one exists, otherwise create a new one
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to check for existing ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("check existing ontology: %w", err)
	}

	if ontology == nil {
		// No active ontology exists - create one
		nextVersion, err := s.ontologyRepo.GetNextVersion(ctx, projectID)
		if err != nil {
			s.logger.Error("Failed to get next ontology version",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("get next version: %w", err)
		}

		ontology = &models.TieredOntology{
			ID:              uuid.New(),
			ProjectID:       projectID,
			Version:         nextVersion,
			IsActive:        true,
			EntitySummaries: make(map[string]*models.EntitySummary),
			ColumnDetails:   make(map[string][]models.ColumnDetail),
			Metadata:        make(map[string]any),
		}

		if err := s.ontologyRepo.Create(ctx, ontology); err != nil {
			s.logger.Error("Failed to create ontology",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("failed to create ontology: %w", err)
		}

		s.logger.Info("Created new ontology for relationship detection",
			zap.String("project_id", projectID.String()),
			zap.String("ontology_id", ontology.ID.String()),
			zap.Int("version", nextVersion))
	} else {
		s.logger.Info("Reusing existing ontology for relationship detection",
			zap.String("project_id", projectID.String()),
			zap.String("ontology_id", ontology.ID.String()),
			zap.Int("version", ontology.Version))
	}

	// Step 3: Create workflow for relationships phase
	now := time.Now()
	workflow := &models.OntologyWorkflow{
		ID:           uuid.New(),
		ProjectID:    projectID,
		OntologyID:   ontology.ID,
		State:        models.WorkflowStatePending,
		Phase:        models.WorkflowPhaseRelationships,
		DatasourceID: &datasourceID,
		Progress: &models.WorkflowProgress{
			CurrentPhase: models.WorkflowPhaseInitializing,
			Current:      0,
			Total:        0,
			Message:      "Starting relationship detection...",
		},
		TaskQueue: []models.WorkflowTask{},
		Config: &models.WorkflowConfig{
			DatasourceID: datasourceID,
		},
		StartedAt: &now,
	}

	if err := s.workflowRepo.Create(ctx, workflow); err != nil {
		s.logger.Error("Failed to create workflow",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, err
	}

	// Step 4: Initialize workflow state for all columns (needed for scanning)
	if err := s.initializeWorkflowEntities(ctx, projectID, workflow.ID, ontology.ID, datasourceID); err != nil {
		s.logger.Error("Failed to initialize workflow entities",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Step 5: Claim ownership and transition to running
	claimed, err := s.workflowRepo.ClaimOwnership(ctx, workflow.ID, s.serverInstanceID)
	if err != nil {
		s.logger.Error("Failed to claim workflow ownership",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("claim ownership: %w", err)
	}
	if !claimed {
		s.logger.Error("Workflow already owned by another server",
			zap.String("workflow_id", workflow.ID.String()))
		return nil, fmt.Errorf("workflow already owned by another server")
	}

	// Transition to running
	workflow.State = models.WorkflowStateRunning
	if err := s.workflowRepo.UpdateState(ctx, workflow.ID, models.WorkflowStateRunning, ""); err != nil {
		s.logger.Error("Failed to start workflow",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Start heartbeat goroutine to maintain ownership
	s.startHeartbeat(workflow.ID, projectID)

	s.logger.Info("Relationship detection started",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", workflow.ID.String()),
		zap.String("datasource_id", datasourceID.String()),
		zap.String("server_instance_id", s.serverInstanceID.String()))

	// Step 6: Create work queue and enqueue tasks
	queue := workqueue.New(s.logger, workqueue.WithStrategy(workqueue.NewParallelLLMStrategy()))
	s.activeQueues.Store(workflow.ID, queue)

	// Start single writer goroutine for task queue updates
	writer := s.startTaskQueueWriter(workflow.ID)

	// Set up callback to sync task queue to database for UI visibility
	queue.SetOnUpdate(func(snapshots []workqueue.TaskSnapshot) {
		// Convert snapshots to models.WorkflowTask with status mapping
		tasks := make([]models.WorkflowTask, len(snapshots))
		for i, snap := range snapshots {
			status := string(snap.Status)
			switch snap.Status {
			case workqueue.TaskStatusPending:
				status = models.TaskStatusQueued
			case workqueue.TaskStatusRunning:
				status = models.TaskStatusProcessing
			case workqueue.TaskStatusCompleted:
				status = models.TaskStatusComplete
			case workqueue.TaskStatusFailed, workqueue.TaskStatusCancelled:
				status = models.TaskStatusFailed
			case workqueue.TaskStatusPaused:
				status = models.TaskStatusPaused
			}

			tasks[i] = models.WorkflowTask{
				ID:          snap.ID,
				Name:        snap.Name,
				Status:      status,
				RequiresLLM: snap.RequiresLLM,
				Error:       snap.Error,
				RetryCount:  snap.RetryCount,
			}
		}
		// Send to single writer goroutine (non-blocking due to buffered channel)
		select {
		case writer.updates <- taskQueueUpdate{
			projectID:  projectID,
			workflowID: workflow.ID,
			tasks:      tasks,
		}:
		default:
			s.logger.Warn("Task queue update buffer full, dropping update",
				zap.String("workflow_id", workflow.ID.String()))
		}
	})

	// Run workflow in background - HTTP request returns immediately
	go s.runWorkflow(projectID, workflow.ID, ontology.ID, datasourceID, queue)

	return workflow, nil
}

// runWorkflow orchestrates the relationship detection phases.
// Runs in a background goroutine - acquires its own DB connection.
func (s *relationshipWorkflowService) runWorkflow(projectID, workflowID, ontologyID, datasourceID uuid.UUID, queue *workqueue.Queue) {
	// Clean up when done
	defer s.activeQueues.Delete(workflowID)
	defer s.stopTaskQueueWriter(workflowID)
	defer s.stopHeartbeat(workflowID)
	defer func() {
		// Release ownership so other servers can take over if needed
		ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
		if err != nil {
			s.logger.Error("Failed to acquire DB connection for ownership release",
				zap.String("workflow_id", workflowID.String()),
				zap.Error(err))
			return
		}
		defer cleanup()
		if releaseErr := s.workflowRepo.ReleaseOwnership(ctx, workflowID); releaseErr != nil {
			s.logger.Error("Failed to release workflow ownership",
				zap.String("workflow_id", workflowID.String()),
				zap.Error(releaseErr))
		}
	}()

	// Task flow (requires entities to exist from entity discovery workflow):
	// 0. Collect column statistics - distinct counts, row counts, ratios
	// 0.5. Filter entity candidates - identify columns likely to be entity IDs
	// 0.75. Analyze graph connectivity - find connected components via FK
	// 1. Scan columns (parallel) - populate workflow_state with sample values
	// 2. Match values (single task) - create candidates from value overlap
	// 3. Infer from names (single task) - create candidates from naming patterns
	// 4. Test join (per candidate, parallel) - SQL join to get cardinality
	// 5. Analyze relationships (single LLM task) - confirm/reject/add candidates

	ctx := context.Background()

	// Phase 0: Collect column statistics for all tables
	s.logger.Info("Phase 0: Collecting column statistics",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Collecting column statistics...", 5); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	statsMap, err := s.collectColumnStatistics(ctx, projectID, workflowID, datasourceID)
	if err != nil {
		s.logger.Error("Failed to collect column statistics", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("column statistics: %v", err))
		return
	}

	// Phase 0.5: Filter columns to identify entity candidates
	s.logger.Info("Phase 0.5: Filtering entity candidates",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Filtering entity candidates...", 8); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	candidates, excluded, err := s.filterEntityCandidates(ctx, projectID, workflowID, datasourceID, statsMap)
	if err != nil {
		s.logger.Error("Failed to filter entity candidates", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("entity filtering: %v", err))
		return
	}

	// Phase 0.75: Analyze graph connectivity
	s.logger.Info("Phase 0.75: Analyzing graph connectivity",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Analyzing graph connectivity...", 9); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	components, islands, err := s.analyzeGraphConnectivity(ctx, projectID, workflowID, datasourceID)
	if err != nil {
		s.logger.Error("Failed to analyze graph connectivity", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("graph connectivity: %v", err))
		return
	}

	// Log summary of data collected (used for relationship analysis)
	s.logger.Info("Data collected for relationship analysis",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("candidate_columns", len(candidates)),
		zap.Int("excluded_columns", len(excluded)),
		zap.Int("connected_components", len(components)),
		zap.Int("island_tables", len(islands)))

	// Note: Entity discovery is now a separate workflow run from the Entities page.
	// The prerequisite check in StartDetection ensures entities exist before we proceed.

	// Phase 1: Scan all columns in parallel
	s.logger.Info("Phase 1: Scanning columns",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Scanning columns...", 10); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	if err := s.enqueueColumnScans(ctx, projectID, workflowID, datasourceID, queue); err != nil {
		s.logger.Error("Failed to enqueue column scan tasks", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("column scan: %v", err))
		return
	}

	// Wait for all column scans to complete
	if err := queue.Wait(ctx); err != nil {
		s.logger.Error("Column scan phase failed", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("column scan failed: %v", err))
		return
	}

	// Phase 2: Match values (single task)
	s.logger.Info("Phase 2: Matching values",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Matching column values...", 30); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	valueMatchTask := NewValueMatchTask(
		s.stateRepo,
		s.candidateRepo,
		s.schemaRepo,
		s.getTenantCtx,
		projectID,
		workflowID,
		datasourceID,
		s.logger,
	)
	queue.Enqueue(valueMatchTask)

	if err := queue.Wait(ctx); err != nil {
		s.logger.Error("Value match phase failed", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("value match failed: %v", err))
		return
	}

	// Phase 3: Infer from names (single task)
	s.logger.Info("Phase 3: Inferring relationships from names",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Inferring relationships from naming patterns...", 50); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	nameInferenceTask := NewNameInferenceTask(
		s.candidateRepo,
		s.schemaRepo,
		s.getTenantCtx,
		projectID,
		workflowID,
		datasourceID,
		s.logger,
	)
	queue.Enqueue(nameInferenceTask)

	if err := queue.Wait(ctx); err != nil {
		s.logger.Error("Name inference phase failed", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("name inference failed: %v", err))
		return
	}

	// Phase 4: Test joins for all candidates (parallel)
	s.logger.Info("Phase 4: Testing joins for candidates",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Testing SQL joins...", 60); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	if err := s.enqueueTestJoins(ctx, projectID, workflowID, datasourceID, queue); err != nil {
		s.logger.Error("Failed to enqueue test join tasks", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("test join: %v", err))
		return
	}

	if err := queue.Wait(ctx); err != nil {
		s.logger.Error("Test join phase failed", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("test join failed: %v", err))
		return
	}

	// Phase 5: LLM analysis (single task)
	s.logger.Info("Phase 5: Analyzing relationships with LLM",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Analyzing relationships...", 80); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	analyzeTask := NewAnalyzeRelationshipsTask(
		s.candidateRepo,
		s.schemaRepo,
		s.llmFactory,
		s.getTenantCtx,
		projectID,
		workflowID,
		datasourceID,
		s.logger,
	)
	queue.Enqueue(analyzeTask)

	if err := queue.Wait(ctx); err != nil {
		s.logger.Error("Analyze relationships phase failed", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("analyze relationships failed: %v", err))
		return
	}

	// Mark workflow as complete
	s.logger.Info("All phases complete - finalizing workflow",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Complete", 100); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	s.finalizeWorkflow(projectID, workflowID)
}

// collectColumnStatistics gathers statistics for all columns across all tables.
// Logs detailed statistics per column and a summary across all tables.
// Returns a map of "schema.table.column" -> ColumnStats for use by filtering.
func (s *relationshipWorkflowService) collectColumnStatistics(ctx context.Context, projectID, workflowID, datasourceID uuid.UUID) (map[string]datasource.ColumnStats, error) {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get datasource to create adapter
	ds, err := s.dsSvc.Get(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer adapter
	adapter, err := s.adapterFactory.NewSchemaDiscoverer(tenantCtx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer adapter.Close()

	// Get all tables for this datasource
	tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}

	// Get all columns for this datasource
	columns, err := s.schemaRepo.ListColumnsByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}

	// Build table lookup and group columns by table
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	columnsByTableID := make(map[uuid.UUID][]*models.SchemaColumn)
	for _, t := range tables {
		tableByID[t.ID] = t
		columnsByTableID[t.ID] = make([]*models.SchemaColumn, 0)
	}

	for _, col := range columns {
		columnsByTableID[col.SchemaTableID] = append(columnsByTableID[col.SchemaTableID], col)
	}

	// Map to store stats by "schema.table.column" key
	statsMap := make(map[string]datasource.ColumnStats)
	totalColumns := 0

	// Collect stats for each table
	for _, table := range tables {
		tableCols := columnsByTableID[table.ID]
		if len(tableCols) == 0 {
			continue
		}

		// Get column names for this table
		columnNames := make([]string, len(tableCols))
		for i, col := range tableCols {
			columnNames[i] = col.ColumnName
		}

		// Call AnalyzeColumnStats for this table
		stats, err := adapter.AnalyzeColumnStats(tenantCtx, table.SchemaName, table.TableName, columnNames)
		if err != nil {
			s.logger.Error("Failed to analyze column stats",
				zap.String("table", fmt.Sprintf("%s.%s", table.SchemaName, table.TableName)),
				zap.Error(err))
			return nil, fmt.Errorf("analyze column stats for %s.%s: %w", table.SchemaName, table.TableName, err)
		}

		// Log detailed statistics for this table
		s.logger.Info(fmt.Sprintf("Column statistics: %s.%s (%d columns)", table.SchemaName, table.TableName, len(stats)),
			zap.String("workflow_id", workflowID.String()))

		for _, stat := range stats {
			percentage := 0.0
			if stat.RowCount > 0 {
				percentage = (float64(stat.DistinctCount) / float64(stat.RowCount)) * 100.0
			}

			s.logger.Info(fmt.Sprintf("  - %s: %d distinct / %d rows (%.1f%%)",
				stat.ColumnName,
				stat.DistinctCount,
				stat.RowCount,
				percentage))

			// Store stats in map for filtering
			statsKey := fmt.Sprintf("%s.%s.%s", table.SchemaName, table.TableName, stat.ColumnName)
			statsMap[statsKey] = stat
		}

		totalColumns += len(stats)
	}

	// Log summary
	s.logger.Info(fmt.Sprintf("Summary: Collected stats for %d columns across %d tables",
		totalColumns,
		len(tables)),
		zap.String("workflow_id", workflowID.String()))

	return statsMap, nil
}

// filterEntityCandidates applies heuristics to filter columns and identify entity candidates.
// Logs results showing candidates vs excluded columns with reasoning.
// Returns candidates and excluded columns for use by subsequent phases.
func (s *relationshipWorkflowService) filterEntityCandidates(
	ctx context.Context,
	projectID, workflowID, datasourceID uuid.UUID,
	statsMap map[string]datasource.ColumnStats,
) ([]ColumnFilterResult, []ColumnFilterResult, error) {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get all tables for this datasource
	tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("list tables: %w", err)
	}

	// Get all columns for this datasource
	columns, err := s.schemaRepo.ListColumnsByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("list columns: %w", err)
	}

	// Build table lookup by UUID string
	tableByID := make(map[string]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID.String()] = t
	}

	// Apply filtering heuristics
	candidates, excluded := FilterEntityCandidates(columns, tableByID, statsMap, s.logger)

	// Log results
	LogFilterResults(candidates, excluded, s.logger)

	return candidates, excluded, nil
}

// analyzeGraphConnectivity builds a graph from foreign key relationships
// and identifies connected components using DFS. Logs results for UI visibility.
// Returns connected components and island tables for use by subsequent phases.
func (s *relationshipWorkflowService) analyzeGraphConnectivity(
	ctx context.Context,
	projectID, workflowID, datasourceID uuid.UUID,
) ([]ConnectedComponent, []string, error) {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return nil, nil, fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get datasource to create adapter
	ds, err := s.dsSvc.Get(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("get datasource: %w", err)
	}

	// Create schema discoverer adapter
	adapter, err := s.adapterFactory.NewSchemaDiscoverer(tenantCtx, ds.DatasourceType, ds.Config, projectID, datasourceID, "")
	if err != nil {
		return nil, nil, fmt.Errorf("create schema discoverer: %w", err)
	}
	defer adapter.Close()

	// Check if datasource supports foreign keys
	if !adapter.SupportsForeignKeys() {
		s.logger.Info("Datasource does not support foreign keys, skipping graph analysis",
			zap.String("workflow_id", workflowID.String()),
			zap.String("datasource_type", string(ds.DatasourceType)))
		// Return empty results rather than nil to indicate success with no FKs
		return []ConnectedComponent{}, []string{}, nil
	}

	// Discover foreign keys
	fks, err := adapter.DiscoverForeignKeys(tenantCtx)
	if err != nil {
		return nil, nil, fmt.Errorf("discover foreign keys: %w", err)
	}

	// Build graph from foreign keys
	graph := NewTableGraph()
	for _, fk := range fks {
		graph.AddForeignKey(fk)
	}

	// Get all tables and add them to the graph (to identify islands)
	tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return nil, nil, fmt.Errorf("list tables: %w", err)
	}

	for _, table := range tables {
		graph.AddTable(table.SchemaName, table.TableName)
	}

	// Find connected components
	components, islands := graph.FindConnectedComponents(s.logger)

	// Log connectivity results
	LogConnectivity(len(fks), components, islands, s.logger)

	return components, islands, nil
}

// enqueueColumnScans creates and enqueues column scan tasks for all columns.
func (s *relationshipWorkflowService) enqueueColumnScans(ctx context.Context, projectID, workflowID, datasourceID uuid.UUID, queue *workqueue.Queue) error {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get all tables for this datasource
	tables, err := s.schemaRepo.ListTablesByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	// Get all columns for this datasource
	columns, err := s.schemaRepo.ListColumnsByDatasource(tenantCtx, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}

	// Build table lookup
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}

	// Enqueue scan task for each column
	for _, col := range columns {
		table := tableByID[col.SchemaTableID]
		if table == nil {
			continue
		}

		task := NewColumnScanTask(
			s.stateRepo,
			s.dsSvc,
			s.adapterFactory,
			s.getTenantCtx,
			projectID,
			workflowID,
			datasourceID,
			table.TableName,
			table.SchemaName,
			col.ColumnName,
		)
		queue.Enqueue(task)
	}

	s.logger.Info("Enqueued column scan tasks",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("count", len(columns)))

	return nil
}

// enqueueTestJoins creates and enqueues test join tasks for all candidates.
func (s *relationshipWorkflowService) enqueueTestJoins(ctx context.Context, projectID, workflowID, datasourceID uuid.UUID, queue *workqueue.Queue) error {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	// Get all candidates for this workflow
	candidates, err := s.candidateRepo.GetByWorkflow(tenantCtx, workflowID)
	if err != nil {
		return fmt.Errorf("get candidates: %w", err)
	}

	// Enqueue test join task for each candidate
	for _, candidate := range candidates {
		task := NewTestJoinTask(
			s.candidateRepo,
			s.schemaRepo,
			s.dsSvc,
			s.adapterFactory,
			s.getTenantCtx,
			projectID,
			workflowID,
			datasourceID,
			candidate.ID,
			s.logger,
		)
		queue.Enqueue(task)
	}

	s.logger.Info("Enqueued test join tasks",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("count", len(candidates)))

	return nil
}

// updateProgress updates the workflow progress.
func (s *relationshipWorkflowService) updateProgress(ctx context.Context, projectID, workflowID uuid.UUID, message string, percent int) error {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	progress := &models.WorkflowProgress{
		CurrentPhase: string(models.WorkflowPhaseRelationships),
		Message:      message,
		Current:      percent,
		Total:        100,
	}

	return s.workflowRepo.UpdateProgress(tenantCtx, workflowID, progress)
}

// finalizeWorkflow marks workflow as complete.
func (s *relationshipWorkflowService) finalizeWorkflow(projectID, workflowID uuid.UUID) {
	ctx, cleanup, dbErr := s.getTenantCtx(context.Background(), projectID)
	if dbErr != nil {
		s.logger.Error("Failed to acquire DB connection for workflow completion",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(dbErr))
		return
	}
	defer cleanup()

	// Get pending candidate counts
	requiredPending, err := s.candidateRepo.CountRequiredPending(ctx, workflowID)
	if err != nil {
		s.logger.Error("Failed to check required pending candidates",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		requiredPending = 0
	}

	// Update progress
	completionMsg := "Relationship detection complete"
	if requiredPending > 0 {
		completionMsg = fmt.Sprintf("Relationship detection complete (%d relationships need review)", requiredPending)
	}

	if updateErr := s.workflowRepo.UpdateProgress(ctx, workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseCompleting,
		Message:      completionMsg,
		Current:      100,
		Total:        100,
	}); updateErr != nil {
		s.logger.Error("Failed to update final progress",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(updateErr))
	}

	if updateErr := s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateCompleted, ""); updateErr != nil {
		s.logger.Error("Failed to mark workflow as completed",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(updateErr))
		return
	}

	s.logger.Info("Relationship workflow completed successfully",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("required_pending", requiredPending))
}

// markWorkflowFailed updates workflow state to failed.
func (s *relationshipWorkflowService) markWorkflowFailed(projectID, workflowID uuid.UUID, errMsg string) {
	ctx, cleanup, dbErr := s.getTenantCtx(context.Background(), projectID)
	if dbErr != nil {
		s.logger.Error("Failed to acquire DB connection for workflow failure update",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(dbErr))
		return
	}
	defer cleanup()

	if updateErr := s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateFailed, errMsg); updateErr != nil {
		s.logger.Error("Failed to mark workflow as failed",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(updateErr))
	}
}

func (s *relationshipWorkflowService) GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, error) {
	workflow, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseRelationships)
	if err != nil {
		s.logger.Error("Failed to get workflow status",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, err
	}
	return workflow, nil
}

func (s *relationshipWorkflowService) GetByID(ctx context.Context, workflowID uuid.UUID) (*models.OntologyWorkflow, error) {
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		s.logger.Error("Failed to get workflow by ID",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return nil, err
	}
	return workflow, nil
}

func (s *relationshipWorkflowService) Cancel(ctx context.Context, workflowID uuid.UUID) error {
	// Cancel active queue if running (signals tasks to stop)
	if queueVal, ok := s.activeQueues.Load(workflowID); ok {
		if queue, ok := queueVal.(*workqueue.Queue); ok {
			queue.Cancel()
			s.logger.Info("Queue cancelled for workflow",
				zap.String("workflow_id", workflowID.String()))
		}
		s.activeQueues.Delete(workflowID)
	}

	// Delete all candidates for this workflow
	if err := s.candidateRepo.DeleteByWorkflow(ctx, workflowID); err != nil {
		s.logger.Error("Failed to delete candidates",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return err
	}

	// Delete workflow record
	if err := s.workflowRepo.Delete(ctx, workflowID); err != nil {
		s.logger.Error("Failed to delete workflow",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return err
	}

	s.logger.Info("Workflow cancelled and deleted",
		zap.String("workflow_id", workflowID.String()))

	return nil
}

func (s *relationshipWorkflowService) GetStatusWithCounts(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, *CandidateCounts, error) {
	workflow, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseRelationships)
	if err != nil {
		s.logger.Error("Failed to get workflow status",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, nil, err
	}
	if workflow == nil {
		return nil, nil, nil
	}

	// Get candidates grouped by status
	candidates, err := s.candidateRepo.GetByWorkflowWithNames(ctx, workflow.ID)
	if err != nil {
		s.logger.Error("Failed to get candidates",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		return nil, nil, err
	}

	counts := &CandidateCounts{}
	for _, c := range candidates {
		switch c.Status {
		case models.RelCandidateStatusAccepted:
			counts.Confirmed++
		case models.RelCandidateStatusPending:
			counts.NeedsReview++
		case models.RelCandidateStatusRejected:
			counts.Rejected++
		}
	}

	// Get entity counts
	ontology, err := s.ontologyRepo.GetActive(ctx, workflow.ProjectID)
	if err == nil && ontology != nil {
		entities, err := s.entityRepo.GetByOntology(ctx, ontology.ID)
		if err == nil {
			counts.EntityCount = len(entities)
			for _, entity := range entities {
				occurrences, err := s.entityRepo.GetOccurrencesByEntity(ctx, entity.ID)
				if err == nil {
					counts.OccurrenceCount += len(occurrences)
				}
			}
		}
	}

	// Island count would come from graph analysis - for now, set to 0
	// This will be populated when graph connectivity data is stored
	counts.IslandCount = 0

	// Can save if workflow is complete and no pending candidates remain
	counts.CanSave = workflow.State == models.WorkflowStateCompleted && counts.NeedsReview == 0

	return workflow, counts, nil
}

func (s *relationshipWorkflowService) GetCandidatesGrouped(ctx context.Context, datasourceID uuid.UUID) (*CandidatesGrouped, error) {
	// Get latest workflow for this datasource
	workflow, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseRelationships)
	if err != nil {
		s.logger.Error("Failed to get workflow",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, err
	}
	if workflow == nil {
		return nil, fmt.Errorf("no relationship workflow found for datasource")
	}

	// Get all candidates with table/column names
	candidates, err := s.candidateRepo.GetByWorkflowWithNames(ctx, workflow.ID)
	if err != nil {
		s.logger.Error("Failed to get candidates",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Group by status
	grouped := &CandidatesGrouped{
		Confirmed:   make([]*models.RelationshipCandidate, 0),
		NeedsReview: make([]*models.RelationshipCandidate, 0),
		Rejected:    make([]*models.RelationshipCandidate, 0),
	}

	for _, c := range candidates {
		switch c.Status {
		case models.RelCandidateStatusAccepted:
			grouped.Confirmed = append(grouped.Confirmed, c)
		case models.RelCandidateStatusPending:
			grouped.NeedsReview = append(grouped.NeedsReview, c)
		case models.RelCandidateStatusRejected:
			grouped.Rejected = append(grouped.Rejected, c)
		}
	}

	return grouped, nil
}

func (s *relationshipWorkflowService) GetEntitiesWithOccurrences(ctx context.Context, datasourceID uuid.UUID) ([]*EntityWithOccurrences, error) {
	// Get latest workflow for this datasource
	workflow, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseRelationships)
	if err != nil {
		s.logger.Error("Failed to get workflow",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, err
	}
	if workflow == nil {
		return nil, fmt.Errorf("no relationship workflow found for datasource")
	}

	// Get the active ontology for this workflow
	ontology, err := s.ontologyRepo.GetActive(ctx, workflow.ProjectID)
	if err != nil {
		s.logger.Error("Failed to get active ontology",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		return nil, err
	}
	if ontology == nil {
		return nil, fmt.Errorf("no active ontology found for project")
	}

	// Get all entities for this ontology
	entities, err := s.entityRepo.GetByOntology(ctx, ontology.ID)
	if err != nil {
		s.logger.Error("Failed to get entities",
			zap.String("ontology_id", ontology.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Fetch occurrences for each entity
	result := make([]*EntityWithOccurrences, 0, len(entities))
	for _, entity := range entities {
		occurrences, err := s.entityRepo.GetOccurrencesByEntity(ctx, entity.ID)
		if err != nil {
			s.logger.Error("Failed to get occurrences",
				zap.String("entity_id", entity.ID.String()),
				zap.Error(err))
			return nil, err
		}

		result = append(result, &EntityWithOccurrences{
			Entity:      entity,
			Occurrences: occurrences,
		})
	}

	return result, nil
}

func (s *relationshipWorkflowService) UpdateCandidateDecision(ctx context.Context, datasourceID, candidateID uuid.UUID, decision string) (*models.RelationshipCandidate, error) {
	// Get the candidate first
	candidate, err := s.candidateRepo.GetByID(ctx, candidateID)
	if err != nil {
		s.logger.Error("Failed to get candidate",
			zap.String("candidate_id", candidateID.String()),
			zap.Error(err))
		return nil, err
	}
	if candidate == nil {
		return nil, fmt.Errorf("candidate not found")
	}

	// Security check: verify candidate belongs to this datasource
	if candidate.DatasourceID != datasourceID {
		s.logger.Warn("Candidate datasource mismatch",
			zap.String("candidate_id", candidateID.String()),
			zap.String("expected_datasource", datasourceID.String()),
			zap.String("actual_datasource", candidate.DatasourceID.String()))
		return nil, fmt.Errorf("candidate not found") // Don't reveal existence
	}

	// Validate decision
	var status models.RelationshipCandidateStatus
	switch decision {
	case "accepted":
		status = models.RelCandidateStatusAccepted
	case "rejected":
		status = models.RelCandidateStatusRejected
	default:
		return nil, fmt.Errorf("invalid decision: must be 'accepted' or 'rejected'")
	}

	// Update status
	candidate.Status = status
	if err := s.candidateRepo.Update(ctx, candidate); err != nil {
		s.logger.Error("Failed to update candidate",
			zap.String("candidate_id", candidateID.String()),
			zap.Error(err))
		return nil, err
	}

	return candidate, nil
}

func (s *relationshipWorkflowService) SaveRelationships(ctx context.Context, workflowID uuid.UUID) (int, error) {
	// Get workflow to extract projectID and datasourceID
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return 0, fmt.Errorf("get workflow: %w", err)
	}
	if workflow == nil {
		return 0, fmt.Errorf("workflow not found")
	}

	if workflow.DatasourceID == nil {
		return 0, fmt.Errorf("workflow has no datasource ID")
	}

	// Check for required pending candidates
	requiredPending, err := s.candidateRepo.CountRequiredPending(ctx, workflowID)
	if err != nil {
		return 0, fmt.Errorf("check required pending candidates: %w", err)
	}

	if requiredPending > 0 {
		return 0, fmt.Errorf("cannot save: %d relationships require user review", requiredPending)
	}

	// Get all accepted candidates
	accepted, err := s.candidateRepo.GetByWorkflowAndStatus(ctx, workflowID, models.RelCandidateStatusAccepted)
	if err != nil {
		return 0, fmt.Errorf("get accepted candidates: %w", err)
	}

	// Load all columns for the datasource once (avoid N+N queries)
	allColumns, err := s.schemaRepo.ListColumnsByDatasource(ctx, workflow.ProjectID, *workflow.DatasourceID)
	if err != nil {
		return 0, fmt.Errorf("list columns: %w", err)
	}

	// Build lookup map by column ID for O(1) access
	columnByID := make(map[uuid.UUID]*models.SchemaColumn, len(allColumns))
	for _, col := range allColumns {
		columnByID[col.ID] = col
	}

	// Create relationships from accepted candidates
	savedCount := 0
	for _, candidate := range accepted {
		// Look up columns from pre-loaded map
		sourceCol, ok := columnByID[candidate.SourceColumnID]
		if !ok {
			return 0, fmt.Errorf("source column %s not found in schema", candidate.SourceColumnID)
		}

		targetCol, ok := columnByID[candidate.TargetColumnID]
		if !ok {
			return 0, fmt.Errorf("target column %s not found in schema", candidate.TargetColumnID)
		}

		// Determine relationship type based on how it was detected
		relType := models.RelationshipTypeInferred
		if candidate.IsUserAccepted() {
			relType = models.RelationshipTypeReview // User validated
		}

		// Map detection method to inference method
		var inferenceMethod *string
		switch candidate.DetectionMethod {
		case models.DetectionMethodValueMatch:
			method := models.InferenceMethodValueOverlap
			inferenceMethod = &method
		case models.DetectionMethodNameInference:
			method := "name_pattern"
			inferenceMethod = &method
		case models.DetectionMethodLLM:
			method := "llm_analysis"
			inferenceMethod = &method
		}

		// Determine cardinality (default to N:1 if not set)
		cardinality := models.CardinalityNTo1
		if candidate.Cardinality != nil {
			cardinality = *candidate.Cardinality
		}

		// Create relationship
		rel := &models.SchemaRelationship{
			ProjectID:        sourceCol.ProjectID,
			SourceTableID:    sourceCol.SchemaTableID,
			SourceColumnID:   candidate.SourceColumnID,
			TargetTableID:    targetCol.SchemaTableID,
			TargetColumnID:   candidate.TargetColumnID,
			RelationshipType: relType,
			Cardinality:      cardinality,
			Confidence:       candidate.Confidence,
			InferenceMethod:  inferenceMethod,
			IsValidated:      true,
		}

		// Create discovery metrics
		metrics := &models.DiscoveryMetrics{}
		if candidate.ValueMatchRate != nil {
			metrics.MatchRate = *candidate.ValueMatchRate
		}
		if candidate.MatchedRows != nil {
			metrics.MatchedCount = *candidate.MatchedRows
		}

		if err := s.schemaRepo.UpsertRelationshipWithMetrics(ctx, rel, metrics); err != nil {
			return 0, fmt.Errorf("create relationship for candidate %s: %w", candidate.ID, err)
		}

		savedCount++
	}

	s.logger.Info("Relationships saved",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("saved_count", savedCount))

	return savedCount, nil
}

func (s *relationshipWorkflowService) UpdateProgress(ctx context.Context, workflowID uuid.UUID, progress *models.WorkflowProgress) error {
	if err := s.workflowRepo.UpdateProgress(ctx, workflowID, progress); err != nil {
		s.logger.Error("Failed to update workflow progress",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return err
	}
	return nil
}

func (s *relationshipWorkflowService) MarkComplete(ctx context.Context, workflowID uuid.UUID) error {
	if err := s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateCompleted, ""); err != nil {
		s.logger.Error("Failed to complete workflow",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return err
	}

	s.logger.Info("Workflow completed",
		zap.String("workflow_id", workflowID.String()))

	return nil
}

func (s *relationshipWorkflowService) MarkFailed(ctx context.Context, workflowID uuid.UUID, errMsg string) error {
	if err := s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateFailed, errMsg); err != nil {
		s.logger.Error("Failed to mark workflow as failed",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return err
	}

	s.logger.Error("Workflow failed",
		zap.String("workflow_id", workflowID.String()),
		zap.String("error", errMsg))

	return nil
}

// startTaskQueueWriter creates and starts a single writer goroutine for a workflow.
func (s *relationshipWorkflowService) startTaskQueueWriter(workflowID uuid.UUID) *taskQueueWriter {
	writer := &taskQueueWriter{
		updates: make(chan taskQueueUpdate, 100),
		done:    make(chan struct{}),
	}
	s.taskQueueWriters.Store(workflowID, writer)
	go s.runTaskQueueWriter(writer)
	return writer
}

// stopTaskQueueWriter closes the channel and waits for the writer to finish.
func (s *relationshipWorkflowService) stopTaskQueueWriter(workflowID uuid.UUID) {
	if writerVal, ok := s.taskQueueWriters.LoadAndDelete(workflowID); ok {
		writer := writerVal.(*taskQueueWriter)
		close(writer.updates)
		<-writer.done // Wait for writer to finish
	}
}

// runTaskQueueWriter is the single writer goroutine that processes updates.
func (s *relationshipWorkflowService) runTaskQueueWriter(writer *taskQueueWriter) {
	defer close(writer.done)

	for {
		// Wait for at least one update
		update, ok := <-writer.updates
		if !ok {
			return // Channel closed, exit
		}

		// Drain any additional pending updates, keeping only the latest
		for {
			select {
			case newer, ok := <-writer.updates:
				if !ok {
					// Channel closed while draining - persist what we have and exit
					s.persistTaskQueue(update.projectID, update.workflowID, update.tasks)
					return
				}
				update = newer // Keep the newer update
			default:
				// No more pending updates, persist the latest
				goto persist
			}
		}

	persist:
		s.persistTaskQueue(update.projectID, update.workflowID, update.tasks)
	}
}

// persistTaskQueue saves the task queue to the database.
func (s *relationshipWorkflowService) persistTaskQueue(projectID, workflowID uuid.UUID, tasks []models.WorkflowTask) {
	// Acquire a fresh DB connection since this runs in a goroutine
	ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
	if err != nil {
		s.logger.Error("Failed to acquire DB connection for task queue update",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return
	}
	defer cleanup()

	if err := s.workflowRepo.UpdateTaskQueue(ctx, workflowID, tasks); err != nil {
		s.logger.Error("Failed to persist task queue",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
	}
}

// initializeWorkflowEntities creates workflow state rows for columns.
// All entities start with status='pending'.
func (s *relationshipWorkflowService) initializeWorkflowEntities(
	ctx context.Context,
	projectID, workflowID, ontologyID, datasourceID uuid.UUID,
) error {
	// Get all tables for this datasource
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("list tables: %w", err)
	}

	// Get all columns for this datasource
	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return fmt.Errorf("list columns: %w", err)
	}

	// Build table lookup map
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, t := range tables {
		tableByID[t.ID] = t
	}

	// Build entity states slice - only column entities for relationship detection
	states := make([]*models.WorkflowEntityState, 0, len(columns))

	for _, col := range columns {
		table := tableByID[col.SchemaTableID]
		if table == nil {
			continue // Skip orphaned columns
		}
		states = append(states, &models.WorkflowEntityState{
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  models.ColumnEntityKey(table.TableName, col.ColumnName),
			Status:     models.WorkflowEntityStatusPending,
			StateData:  &models.WorkflowStateData{},
		})
	}

	// Batch create all entities
	if err := s.stateRepo.CreateBatch(ctx, states); err != nil {
		return fmt.Errorf("create workflow entities: %w", err)
	}

	s.logger.Info("Initialized workflow entities",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("total_entities", len(states)),
		zap.Int("columns", len(columns)))

	return nil
}

// startHeartbeat launches a background goroutine that periodically updates
// the workflow's last_heartbeat timestamp to maintain ownership.
func (s *relationshipWorkflowService) startHeartbeat(workflowID, projectID uuid.UUID) {
	stop := make(chan struct{})
	info := &relationshipHeartbeatInfo{
		projectID: projectID,
		stop:      stop,
	}
	s.heartbeatStop.Store(workflowID, info)

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				s.logger.Debug("Heartbeat stopped",
					zap.String("workflow_id", workflowID.String()))
				return
			case <-ticker.C:
				ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
				if err != nil {
					s.logger.Error("Failed to acquire DB connection for heartbeat",
						zap.String("workflow_id", workflowID.String()),
						zap.Error(err))
					continue
				}
				if err := s.workflowRepo.UpdateHeartbeat(ctx, workflowID, s.serverInstanceID); err != nil {
					s.logger.Error("Failed to update heartbeat",
						zap.String("workflow_id", workflowID.String()),
						zap.Error(err))
				}
				cleanup()
			}
		}
	}()

	s.logger.Debug("Heartbeat started",
		zap.String("workflow_id", workflowID.String()),
		zap.String("server_instance_id", s.serverInstanceID.String()))
}

// stopHeartbeat stops the heartbeat goroutine for a workflow.
func (s *relationshipWorkflowService) stopHeartbeat(workflowID uuid.UUID) {
	if infoVal, ok := s.heartbeatStop.LoadAndDelete(workflowID); ok {
		info := infoVal.(*relationshipHeartbeatInfo)
		close(info.stop)
	}
}

// Shutdown gracefully stops all active workflows owned by this server.
func (s *relationshipWorkflowService) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down relationship workflow service",
		zap.String("server_instance_id", s.serverInstanceID.String()))

	var wg sync.WaitGroup

	// Cancel all active queues and release ownership
	s.activeQueues.Range(func(key, value any) bool {
		workflowID := key.(uuid.UUID)
		queue := value.(*workqueue.Queue)

		// Get project ID from heartbeat info if available
		var projectID uuid.UUID
		if infoVal, ok := s.heartbeatStop.Load(workflowID); ok {
			info := infoVal.(*relationshipHeartbeatInfo)
			projectID = info.projectID
		}

		wg.Add(1)
		go func(wfID uuid.UUID, q *workqueue.Queue, pID uuid.UUID) {
			defer wg.Done()

			s.logger.Info("Cancelling workflow for shutdown",
				zap.String("workflow_id", wfID.String()))

			// Cancel the queue
			q.Cancel()

			// Stop heartbeat
			s.stopHeartbeat(wfID)

			// Release ownership if we have a project ID
			if pID != uuid.Nil {
				tenantCtx, cleanup, err := s.getTenantCtx(context.Background(), pID)
				if err == nil {
					if releaseErr := s.workflowRepo.ReleaseOwnership(tenantCtx, wfID); releaseErr != nil {
						s.logger.Error("Failed to release ownership during shutdown",
							zap.String("workflow_id", wfID.String()),
							zap.Error(releaseErr))
					}
					cleanup()
				}
			}
		}(workflowID, queue, projectID)

		return true
	})

	// Wait for all cancellations with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("All relationship workflows cancelled successfully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("Shutdown timed out, some workflows may not have been cleaned up")
		return ctx.Err()
	}
}
