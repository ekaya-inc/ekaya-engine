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

// EntityDiscoveryService provides operations for standalone entity discovery workflows.
// This is separate from relationship detection and can be run independently.
type EntityDiscoveryService interface {
	// StartDiscovery initiates a new entity discovery workflow.
	StartDiscovery(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetStatus returns the current entity discovery workflow status for a datasource.
	GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetStatusWithCounts returns workflow status with entity counts.
	GetStatusWithCounts(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, *EntityDiscoveryCounts, error)

	// Cancel cancels a running entity discovery workflow.
	Cancel(ctx context.Context, workflowID uuid.UUID) error

	// Shutdown gracefully stops all active workflows owned by this server.
	Shutdown(ctx context.Context) error
}

// EntityDiscoveryCounts holds counts of discovered entities.
type EntityDiscoveryCounts struct {
	EntityCount     int
	OccurrenceCount int
}

// entityHeartbeatInfo holds info needed for heartbeat goroutine
type entityHeartbeatInfo struct {
	projectID uuid.UUID
	stop      chan struct{}
}

type entityDiscoveryService struct {
	workflowRepo   repositories.OntologyWorkflowRepository
	entityRepo     repositories.SchemaEntityRepository
	schemaRepo     repositories.SchemaRepository
	ontologyRepo   repositories.OntologyRepository
	dsSvc          DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	llmFactory     llm.LLMClientFactory
	getTenantCtx   TenantContextFunc
	logger         *zap.Logger

	serverInstanceID uuid.UUID
	activeQueues     sync.Map // workflowID -> *workqueue.Queue
	taskQueueWriters sync.Map // workflowID -> *entityTaskQueueWriter
	heartbeatStop    sync.Map // workflowID -> *entityHeartbeatInfo
}

// entityTaskQueueWriter handles batched task queue updates.
type entityTaskQueueWriter struct {
	updates chan entityTaskQueueUpdate
	done    chan struct{}
}

// entityTaskQueueUpdate holds a pending task queue update.
type entityTaskQueueUpdate struct {
	projectID  uuid.UUID
	workflowID uuid.UUID
	tasks      []models.WorkflowTask
}

// NewEntityDiscoveryService creates a new entity discovery service.
func NewEntityDiscoveryService(
	workflowRepo repositories.OntologyWorkflowRepository,
	entityRepo repositories.SchemaEntityRepository,
	schemaRepo repositories.SchemaRepository,
	ontologyRepo repositories.OntologyRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	llmFactory llm.LLMClientFactory,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) EntityDiscoveryService {
	serverID := uuid.New()
	namedLogger := logger.Named("entity-discovery")
	namedLogger.Info("Entity discovery service initialized", zap.String("server_instance_id", serverID.String()))

	return &entityDiscoveryService{
		workflowRepo:     workflowRepo,
		entityRepo:       entityRepo,
		schemaRepo:       schemaRepo,
		ontologyRepo:     ontologyRepo,
		dsSvc:            dsSvc,
		adapterFactory:   adapterFactory,
		llmFactory:       llmFactory,
		getTenantCtx:     getTenantCtx,
		logger:           namedLogger,
		serverInstanceID: serverID,
	}
}

var _ EntityDiscoveryService = (*entityDiscoveryService)(nil)

func (s *entityDiscoveryService) StartDiscovery(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyWorkflow, error) {
	// Step 1: Check for existing workflow for this datasource in entities phase
	existing, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseEntities)
	if err != nil {
		s.logger.Error("Failed to check existing workflow",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		return nil, err
	}

	// If there's an active workflow, don't start a new one
	if existing != nil && !existing.State.IsTerminal() {
		return nil, fmt.Errorf("entity discovery already in progress for this datasource")
	}

	// Step 2: Get or create ontology for this project
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

		s.logger.Info("Created new ontology for entity discovery",
			zap.String("project_id", projectID.String()),
			zap.String("ontology_id", ontology.ID.String()),
			zap.Int("version", nextVersion))
	} else {
		s.logger.Info("Reusing existing ontology for entity discovery",
			zap.String("project_id", projectID.String()),
			zap.String("ontology_id", ontology.ID.String()),
			zap.Int("version", ontology.Version))

		// Delete existing entities for this ontology (fresh discovery)
		if err := s.entityRepo.DeleteByOntology(ctx, ontology.ID); err != nil {
			s.logger.Error("Failed to delete existing entities",
				zap.String("ontology_id", ontology.ID.String()),
				zap.Error(err))
			return nil, fmt.Errorf("delete existing entities: %w", err)
		}
	}

	// Step 3: Create workflow for entities phase
	now := time.Now()
	workflow := &models.OntologyWorkflow{
		ID:           uuid.New(),
		ProjectID:    projectID,
		OntologyID:   ontology.ID,
		State:        models.WorkflowStatePending,
		Phase:        models.WorkflowPhaseEntities,
		DatasourceID: &datasourceID,
		Progress: &models.WorkflowProgress{
			CurrentPhase: "initializing",
			Current:      0,
			Total:        100,
			Message:      "Starting entity discovery...",
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

	// Step 4: Claim ownership and transition to running
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

	s.logger.Info("Entity discovery started",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", workflow.ID.String()),
		zap.String("datasource_id", datasourceID.String()),
		zap.String("server_instance_id", s.serverInstanceID.String()))

	// Step 5: Create work queue and enqueue tasks
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
		case writer.updates <- entityTaskQueueUpdate{
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

// runWorkflow orchestrates the entity discovery phases.
// Runs in a background goroutine - acquires its own DB connection.
func (s *entityDiscoveryService) runWorkflow(projectID, workflowID, ontologyID, datasourceID uuid.UUID, queue *workqueue.Queue) {
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

	// Entity discovery phases:
	// 0. Collect column statistics (for all tables) - distinct counts, row counts, ratios
	// 0.5. Filter entity candidates (deterministic)
	// 0.75. Analyze graph connectivity (FK relationships)
	// 1. Entity Discovery (LLM) - identify entities with descriptions and occurrences

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

	if err := s.updateProgress(ctx, projectID, workflowID, "Filtering entity candidates...", 20); err != nil {
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

	if err := s.updateProgress(ctx, projectID, workflowID, "Analyzing graph connectivity...", 35); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	components, islands, err := s.analyzeGraphConnectivity(ctx, projectID, workflowID, datasourceID)
	if err != nil {
		s.logger.Error("Failed to analyze graph connectivity", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("graph connectivity: %v", err))
		return
	}

	// Log summary of data available for Entity Discovery LLM
	s.logger.Info("Data collected for entity discovery",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("candidate_columns", len(candidates)),
		zap.Int("excluded_columns", len(excluded)),
		zap.Int("connected_components", len(components)),
		zap.Int("island_tables", len(islands)))

	// Phase 1: Entity Discovery (LLM)
	s.logger.Info("Phase 1: Entity discovery with LLM",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Discovering entities with LLM...", 50); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	entityTask := NewEntityDiscoveryTask(
		s.entityRepo,
		s.schemaRepo,
		s.llmFactory,
		s.adapterFactory,
		s.dsSvc,
		s.getTenantCtx,
		projectID,
		workflowID,
		ontologyID,
		datasourceID,
		candidates,
		excluded,
		components,
		islands,
		statsMap,
		s.logger,
	)
	queue.Enqueue(entityTask)

	if err := queue.Wait(ctx); err != nil {
		s.logger.Error("Entity discovery failed", zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, fmt.Sprintf("entity discovery: %v", err))
		return
	}

	// Mark workflow as complete
	s.logger.Info("Entity discovery complete - finalizing workflow",
		zap.String("workflow_id", workflowID.String()))

	if err := s.updateProgress(ctx, projectID, workflowID, "Complete", 100); err != nil {
		s.logger.Error("Failed to update progress", zap.Error(err))
	}

	s.finalizeWorkflow(projectID, workflowID)
}

// collectColumnStatistics gathers statistics for all columns across all tables.
// Returns a map of "schema.table.column" -> ColumnStats for use by filtering.
func (s *entityDiscoveryService) collectColumnStatistics(ctx context.Context, projectID, workflowID, datasourceID uuid.UUID) (map[string]datasource.ColumnStats, error) {
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

		// Log statistics for this table
		s.logger.Info(fmt.Sprintf("Column statistics: %s.%s (%d columns)", table.SchemaName, table.TableName, len(stats)),
			zap.String("workflow_id", workflowID.String()))

		for _, stat := range stats {
			percentage := 0.0
			if stat.RowCount > 0 {
				percentage = (float64(stat.DistinctCount) / float64(stat.RowCount)) * 100.0
			}

			s.logger.Debug(fmt.Sprintf("  - %s: %d distinct / %d rows (%.1f%%)",
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
func (s *entityDiscoveryService) filterEntityCandidates(
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

// analyzeGraphConnectivity builds a graph from foreign key relationships.
func (s *entityDiscoveryService) analyzeGraphConnectivity(
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

	for _, t := range tables {
		graph.AddTable(t.SchemaName, t.TableName)
	}

	// Find connected components and islands
	components, islands := graph.FindConnectedComponents(s.logger)

	// Log results
	s.logger.Info("Graph connectivity analysis complete",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("connected_components", len(components)),
		zap.Int("island_tables", len(islands)))

	return components, islands, nil
}

// updateProgress updates the workflow progress.
func (s *entityDiscoveryService) updateProgress(ctx context.Context, projectID, workflowID uuid.UUID, message string, percentage int) error {
	tenantCtx, cleanup, err := s.getTenantCtx(ctx, projectID)
	if err != nil {
		return fmt.Errorf("get tenant context: %w", err)
	}
	defer cleanup()

	progress := &models.WorkflowProgress{
		CurrentPhase: "entity_discovery",
		Current:      percentage,
		Total:        100,
		Message:      message,
	}

	return s.workflowRepo.UpdateProgress(tenantCtx, workflowID, progress)
}

// finalizeWorkflow marks the workflow as completed.
func (s *entityDiscoveryService) finalizeWorkflow(projectID, workflowID uuid.UUID) {
	ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
	if err != nil {
		s.logger.Error("Failed to acquire DB connection for finalization",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return
	}
	defer cleanup()

	if err := s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateCompleted, ""); err != nil {
		s.logger.Error("Failed to mark workflow complete",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return
	}

	s.logger.Info("Workflow completed successfully",
		zap.String("workflow_id", workflowID.String()))
}

// markWorkflowFailed marks the workflow as failed with an error message.
func (s *entityDiscoveryService) markWorkflowFailed(projectID, workflowID uuid.UUID, errMsg string) {
	ctx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
	if err != nil {
		s.logger.Error("Failed to acquire DB connection for failure marking",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return
	}
	defer cleanup()

	if err := s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateFailed, errMsg); err != nil {
		s.logger.Error("Failed to mark workflow as failed",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
	}

	s.logger.Error("Workflow failed",
		zap.String("workflow_id", workflowID.String()),
		zap.String("error", errMsg))
}

// startTaskQueueWriter creates and starts a single writer goroutine for a workflow.
func (s *entityDiscoveryService) startTaskQueueWriter(workflowID uuid.UUID) *entityTaskQueueWriter {
	writer := &entityTaskQueueWriter{
		updates: make(chan entityTaskQueueUpdate, 100),
		done:    make(chan struct{}),
	}
	s.taskQueueWriters.Store(workflowID, writer)
	go s.runTaskQueueWriter(writer)
	return writer
}

// stopTaskQueueWriter closes the channel and waits for the writer to finish.
func (s *entityDiscoveryService) stopTaskQueueWriter(workflowID uuid.UUID) {
	if writerVal, ok := s.taskQueueWriters.LoadAndDelete(workflowID); ok {
		writer := writerVal.(*entityTaskQueueWriter)
		close(writer.updates)
		<-writer.done // Wait for writer to finish
	}
}

// runTaskQueueWriter is the single writer goroutine that processes updates.
func (s *entityDiscoveryService) runTaskQueueWriter(writer *entityTaskQueueWriter) {
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
func (s *entityDiscoveryService) persistTaskQueue(projectID, workflowID uuid.UUID, tasks []models.WorkflowTask) {
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

// startHeartbeat launches a background goroutine that periodically updates
// the workflow's last_heartbeat timestamp to maintain ownership.
func (s *entityDiscoveryService) startHeartbeat(workflowID, projectID uuid.UUID) {
	stop := make(chan struct{})
	info := &entityHeartbeatInfo{
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
func (s *entityDiscoveryService) stopHeartbeat(workflowID uuid.UUID) {
	if infoVal, ok := s.heartbeatStop.LoadAndDelete(workflowID); ok {
		info := infoVal.(*entityHeartbeatInfo)
		close(info.stop)
	}
}

// GetStatus returns the current entity discovery workflow status for a datasource.
func (s *entityDiscoveryService) GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, error) {
	return s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseEntities)
}

// GetStatusWithCounts returns workflow status with entity counts.
func (s *entityDiscoveryService) GetStatusWithCounts(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, *EntityDiscoveryCounts, error) {
	workflow, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, datasourceID, models.WorkflowPhaseEntities)
	if err != nil {
		return nil, nil, fmt.Errorf("get workflow: %w", err)
	}

	if workflow == nil {
		return nil, nil, nil
	}

	// Get entity counts
	counts := &EntityDiscoveryCounts{}
	entities, err := s.entityRepo.GetByOntology(ctx, workflow.OntologyID)
	if err != nil {
		s.logger.Error("Failed to get entities for counts",
			zap.String("ontology_id", workflow.OntologyID.String()),
			zap.Error(err))
		// Return workflow without counts rather than failing
		return workflow, counts, nil
	}

	counts.EntityCount = len(entities)
	// Count occurrences for each entity
	for _, e := range entities {
		occurrences, err := s.entityRepo.GetOccurrencesByEntity(ctx, e.ID)
		if err != nil {
			s.logger.Error("Failed to get occurrences for entity",
				zap.String("entity_id", e.ID.String()),
				zap.Error(err))
			continue
		}
		counts.OccurrenceCount += len(occurrences)
	}

	return workflow, counts, nil
}

// Cancel cancels a running entity discovery workflow.
func (s *entityDiscoveryService) Cancel(ctx context.Context, workflowID uuid.UUID) error {
	// Get the workflow to find its project ID
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("get workflow: %w", err)
	}

	if workflow == nil {
		return fmt.Errorf("workflow not found")
	}

	// Cancel the queue if we own it
	if queueVal, ok := s.activeQueues.Load(workflowID); ok {
		queue := queueVal.(*workqueue.Queue)
		queue.Cancel()
	}

	// Update state to failed (cancelled is treated as failed)
	return s.workflowRepo.UpdateState(ctx, workflowID, models.WorkflowStateFailed, "cancelled by user")
}

// Shutdown gracefully stops all active workflows owned by this server.
func (s *entityDiscoveryService) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down entity discovery service",
		zap.String("server_instance_id", s.serverInstanceID.String()))

	var wg sync.WaitGroup

	// Cancel all active queues and release ownership
	s.activeQueues.Range(func(key, value any) bool {
		workflowID := key.(uuid.UUID)
		queue := value.(*workqueue.Queue)

		// Get project ID from heartbeat info if available
		var projectID uuid.UUID
		if infoVal, ok := s.heartbeatStop.Load(workflowID); ok {
			info := infoVal.(*entityHeartbeatInfo)
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
		s.logger.Info("All entity discovery workflows cancelled successfully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("Shutdown timed out, some workflows may not have been cleaned up")
		return ctx.Err()
	}
}
