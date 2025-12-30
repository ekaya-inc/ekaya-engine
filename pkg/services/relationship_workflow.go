package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// RelationshipWorkflowService provides operations for relationship discovery workflow management.
type RelationshipWorkflowService interface {
	// StartDetection initiates a new relationship detection workflow.
	StartDetection(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetStatus returns the current workflow status for a datasource.
	GetStatus(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetByID returns a specific workflow by its ID.
	GetByID(ctx context.Context, workflowID uuid.UUID) (*models.OntologyWorkflow, error)

	// Cancel cancels a running workflow.
	Cancel(ctx context.Context, workflowID uuid.UUID) error

	// SaveRelationships saves accepted candidates as relationships and marks workflow complete.
	SaveRelationships(ctx context.Context, workflowID uuid.UUID) error

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
	workflowRepo      repositories.OntologyWorkflowRepository
	candidateRepo     repositories.RelationshipCandidateRepository
	schemaRepo        repositories.SchemaRepository
	stateRepo         repositories.WorkflowStateRepository
	dsSvc             DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	discoveryService  RelationshipDiscoveryService
	getTenantCtx      TenantContextFunc
	logger            *zap.Logger
	serverInstanceID  uuid.UUID
	activeQueues      sync.Map // workflowID -> *workqueue.Queue
	taskQueueWriters  sync.Map // workflowID -> *taskQueueWriter
	heartbeatStop     sync.Map // workflowID -> *relationshipHeartbeatInfo
}

// NewRelationshipWorkflowService creates a new relationship workflow service.
func NewRelationshipWorkflowService(
	workflowRepo repositories.OntologyWorkflowRepository,
	candidateRepo repositories.RelationshipCandidateRepository,
	schemaRepo repositories.SchemaRepository,
	stateRepo repositories.WorkflowStateRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
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
		dsSvc:            dsSvc,
		adapterFactory:   adapterFactory,
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

	// Step 2: Create a temporary ontology for this workflow
	// We need an ontology ID for the workflow, but this won't be the "real" ontology
	// until the ontology phase runs later
	ontology := &models.TieredOntology{
		ID:              uuid.New(),
		ProjectID:       projectID,
		Version:         0, // Temporary version, will be replaced by actual ontology
		IsActive:        false,
		EntitySummaries: make(map[string]*models.EntitySummary),
		ColumnDetails:   make(map[string][]models.ColumnDetail),
		Metadata:        make(map[string]any),
	}

	// Create temporary ontology - this is just a placeholder for the workflow
	// The actual ontology will be created during the ontology phase
	ontologyRepo := repositories.NewOntologyRepository()
	if err := ontologyRepo.Create(ctx, ontology); err != nil {
		s.logger.Error("Failed to create temporary ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
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
	go s.runWorkflow(projectID, workflow.ID, datasourceID, queue)

	return workflow, nil
}

// runWorkflow orchestrates the relationship detection phases.
// Runs in a background goroutine - acquires its own DB connection.
func (s *relationshipWorkflowService) runWorkflow(projectID, workflowID, datasourceID uuid.UUID, queue *workqueue.Queue) {
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

	// Task flow:
	// 1. Scan columns (parallel) - populate workflow_state with sample values
	// 2. Match values (single task) - create candidates from value overlap
	// 3. Infer from names (single task) - create candidates from naming patterns
	// 4. Test join (per candidate, parallel) - SQL join to get cardinality
	// 5. Analyze relationships (single LLM task) - confirm/reject/add candidates

	// TODO: Implement task orchestration
	// For now, just mark as complete
	s.logger.Info("Relationship workflow orchestration not yet implemented",
		zap.String("workflow_id", workflowID.String()))

	// Mark workflow as complete
	s.finalizeWorkflow(projectID, workflowID)
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

func (s *relationshipWorkflowService) SaveRelationships(ctx context.Context, workflowID uuid.UUID) error {
	// Check for required pending candidates
	requiredPending, err := s.candidateRepo.CountRequiredPending(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("check required pending candidates: %w", err)
	}

	if requiredPending > 0 {
		return fmt.Errorf("cannot save: %d relationships require user review", requiredPending)
	}

	// Get all accepted candidates
	accepted, err := s.candidateRepo.GetByWorkflowAndStatus(ctx, workflowID, models.RelCandidateStatusAccepted)
	if err != nil {
		return fmt.Errorf("get accepted candidates: %w", err)
	}

	// Get workflow to extract projectID
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		return fmt.Errorf("get workflow: %w", err)
	}

	// Create relationships from accepted candidates
	savedCount := 0
	for _, candidate := range accepted {
		// Get source and target columns to get table IDs
		sourceCol, err := s.schemaRepo.GetColumnByID(ctx, workflow.ProjectID, candidate.SourceColumnID)
		if err != nil {
			s.logger.Error("Failed to get source column",
				zap.String("column_id", candidate.SourceColumnID.String()),
				zap.Error(err))
			continue
		}

		targetCol, err := s.schemaRepo.GetColumnByID(ctx, workflow.ProjectID, candidate.TargetColumnID)
		if err != nil {
			s.logger.Error("Failed to get target column",
				zap.String("column_id", candidate.TargetColumnID.String()),
				zap.Error(err))
			continue
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
			s.logger.Error("Failed to create relationship",
				zap.String("candidate_id", candidate.ID.String()),
				zap.Error(err))
			continue
		}

		savedCount++
	}

	s.logger.Info("Relationships saved",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("saved_count", savedCount))

	return nil
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
