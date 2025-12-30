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

// OntologyWorkflowService provides operations for ontology workflow management.
type OntologyWorkflowService interface {
	// StartExtraction initiates a new ontology extraction workflow.
	StartExtraction(ctx context.Context, projectID uuid.UUID, config *models.WorkflowConfig) (*models.OntologyWorkflow, error)

	// GetStatus returns the current workflow status for a project.
	GetStatus(ctx context.Context, projectID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetByID returns a specific workflow by its ID.
	GetByID(ctx context.Context, workflowID uuid.UUID) (*models.OntologyWorkflow, error)

	// GetOntology returns the active tiered ontology for a project.
	GetOntology(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error)

	// Cancel cancels a running workflow.
	Cancel(ctx context.Context, workflowID uuid.UUID) error

	// DeleteOntology deletes all ontology data for a project (workflows, ontologies, questions).
	DeleteOntology(ctx context.Context, projectID uuid.UUID) error

	// UpdateProgress updates the workflow progress.
	UpdateProgress(ctx context.Context, workflowID uuid.UUID, progress *models.WorkflowProgress) error

	// MarkComplete marks a workflow as completed.
	MarkComplete(ctx context.Context, workflowID uuid.UUID) error

	// MarkFailed marks a workflow as failed with an error message.
	MarkFailed(ctx context.Context, workflowID uuid.UUID, errMsg string) error

	// Shutdown gracefully stops all active workflows owned by this server.
	// Called during server shutdown to release ownership so new servers can take over.
	Shutdown(ctx context.Context) error

	// GetSchemaEntityCount returns the total number of entities (1 global + tables + columns)
	// that would be processed by an ontology extraction workflow.
	GetSchemaEntityCount(ctx context.Context, projectID uuid.UUID) (int, error)
}

// taskQueueUpdate holds data for a task queue database update.
type taskQueueUpdate struct {
	projectID  uuid.UUID
	workflowID uuid.UUID
	tasks      []models.WorkflowTask
}

// taskQueueWriter manages serialized writes for a single workflow.
type taskQueueWriter struct {
	updates chan taskQueueUpdate
	done    chan struct{}
}

// heartbeatInfo holds info needed for heartbeat goroutine
type heartbeatInfo struct {
	projectID uuid.UUID
	stop      chan struct{}
}

type ontologyWorkflowService struct {
	workflowRepo   repositories.OntologyWorkflowRepository
	ontologyRepo   repositories.OntologyRepository
	schemaRepo     repositories.SchemaRepository
	stateRepo      repositories.WorkflowStateRepository
	questionRepo   repositories.OntologyQuestionRepository
	convRepo       repositories.ConversationRepository
	dsSvc          DatasourceService
	adapterFactory datasource.DatasourceAdapterFactory
	builder        OntologyBuilderService
	getTenantCtx   TenantContextFunc
	logger         *zap.Logger

	// activeQueues maps workflowID -> *workqueue.Queue for cancellation support
	activeQueues sync.Map
	// taskQueueWriters maps workflowID -> *taskQueueWriter for serialized DB writes
	taskQueueWriters sync.Map

	// Ownership fields for multi-server robustness
	serverInstanceID uuid.UUID // Unique ID for this server instance
	heartbeatStop    sync.Map  // workflowID -> *heartbeatInfo
}

// NewOntologyWorkflowService creates a new ontology workflow service.
func NewOntologyWorkflowService(
	workflowRepo repositories.OntologyWorkflowRepository,
	ontologyRepo repositories.OntologyRepository,
	schemaRepo repositories.SchemaRepository,
	stateRepo repositories.WorkflowStateRepository,
	questionRepo repositories.OntologyQuestionRepository,
	convRepo repositories.ConversationRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	builder OntologyBuilderService,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
) OntologyWorkflowService {
	serverID := uuid.New()
	namedLogger := logger.Named("ontology-workflow")
	namedLogger.Info("Workflow service initialized", zap.String("server_instance_id", serverID.String()))

	return &ontologyWorkflowService{
		workflowRepo:     workflowRepo,
		ontologyRepo:     ontologyRepo,
		schemaRepo:       schemaRepo,
		stateRepo:        stateRepo,
		questionRepo:     questionRepo,
		convRepo:         convRepo,
		dsSvc:            dsSvc,
		adapterFactory:   adapterFactory,
		builder:          builder,
		getTenantCtx:     getTenantCtx,
		logger:           namedLogger,
		serverInstanceID: serverID,
	}
}

var _ OntologyWorkflowService = (*ontologyWorkflowService)(nil)

func (s *ontologyWorkflowService) StartExtraction(ctx context.Context, projectID uuid.UUID, config *models.WorkflowConfig) (*models.OntologyWorkflow, error) {
	// Set defaults
	if config == nil {
		config = models.DefaultWorkflowConfig()
	}

	// Step 0: Check if relationships phase has completed for this datasource (Milestone 3.3)
	// The relationships phase must complete before ontology extraction can start.
	relWorkflow, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, config.DatasourceID, models.WorkflowPhaseRelationships)
	if err != nil {
		s.logger.Error("Failed to check relationships workflow status",
			zap.String("datasource_id", config.DatasourceID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("check relationships workflow: %w", err)
	}
	if relWorkflow == nil {
		return nil, fmt.Errorf("relationships phase must complete before ontology extraction: no relationship workflow found for datasource")
	}
	if relWorkflow.State != models.WorkflowStateCompleted {
		return nil, fmt.Errorf("relationships phase must complete before ontology extraction: current state is %s", relWorkflow.State)
	}

	s.logger.Info("Relationships phase verified complete",
		zap.String("datasource_id", config.DatasourceID.String()),
		zap.String("rel_workflow_id", relWorkflow.ID.String()))

	// Step 1: Get active ontology ID (needed for cleanup after deactivation).
	// We capture the ID here before deactivating so we can clean up its workflow state.
	previousOntology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get active ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}

	// Step 2: Deactivate existing ontology.
	// This must succeed before we delete workflow state to ensure we don't lose
	// data for an ontology that's still active (transaction safety).
	if err := s.ontologyRepo.DeactivateAll(ctx, projectID); err != nil {
		s.logger.Error("Failed to deactivate existing ontologies",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}

	// Step 3: Clean up workflow state from previous ontology.
	// We preserve workflow_state after completion for assess-deterministic tool,
	// but delete it here when starting a new extraction to prevent unbounded growth.
	// Scoped to ontology (not project) to support future multi-ontology scenarios.
	// Safe to delete now since the ontology has been deactivated.
	if previousOntology != nil {
		if err := s.stateRepo.DeleteByOntology(ctx, previousOntology.ID); err != nil {
			s.logger.Error("Failed to clean up previous workflow states",
				zap.String("ontology_id", previousOntology.ID.String()),
				zap.Error(err))
			return nil, err
		}
	}

	// Step 4: Create new ontology version
	nextVersion, err := s.ontologyRepo.GetNextVersion(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get next ontology version",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}

	ontology := &models.TieredOntology{
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
		return nil, err
	}

	// Step 5: Check for existing workflow for this ontology
	// The unique index on ontology_id prevents duplicates at DB level, but we check here for better error messages
	existing, err := s.workflowRepo.GetByOntology(ctx, ontology.ID)
	if err != nil {
		s.logger.Error("Failed to check existing workflow for ontology",
			zap.String("ontology_id", ontology.ID.String()),
			zap.Error(err))
		return nil, err
	}

	if existing != nil {
		return nil, fmt.Errorf("workflow already exists for this ontology")
	}

	// Step 6: Create new workflow linked to the ontology
	now := time.Now()
	workflow := &models.OntologyWorkflow{
		ID:         uuid.New(),
		ProjectID:  projectID,
		OntologyID: ontology.ID,
		State:      models.WorkflowStatePending,
		Progress: &models.WorkflowProgress{
			CurrentPhase: models.WorkflowPhaseInitializing,
			Current:      0,
			Total:        0, // Will be set by InitializeOntologyTask
			Message:      "Starting ontology extraction...",
		},
		TaskQueue: []models.WorkflowTask{},
		Config:    config,
		StartedAt: &now,
	}

	if err := s.workflowRepo.Create(ctx, workflow); err != nil {
		s.logger.Error("Failed to create workflow",
			zap.String("project_id", projectID.String()),
			zap.String("ontology_id", ontology.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Step 7: Initialize workflow state for all entities
	// Pass the relationships workflow ID to reuse scan data (Milestone 3.4)
	if err := s.initializeWorkflowEntities(ctx, projectID, workflow.ID, ontology.ID, config.DatasourceID, relWorkflow.ID); err != nil {
		s.logger.Error("Failed to initialize workflow entities",
			zap.String("workflow_id", workflow.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Claim ownership and transition to running
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

	s.logger.Info("Ontology extraction started",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", workflow.ID.String()),
		zap.String("ontology_id", ontology.ID.String()),
		zap.String("server_instance_id", s.serverInstanceID.String()),
		zap.Int("ontology_version", nextVersion))

	// Create work queue and enqueue the initialization task
	// InitializeOntologyTask will enqueue all other tasks (data tasks, LLM tasks, etc.)
	// Queue uses ParallelLLMStrategy: LLM tasks run in parallel (no limit),
	// data tasks serialize with data tasks.
	// This allows efficient batching of LLM requests to the model provider.
	// Default retry config: 24 retries with exponential backoff (2s initial, 30s max, ~10min total).
	queue := workqueue.New(s.logger, workqueue.WithStrategy(workqueue.NewParallelLLMStrategy()))
	s.activeQueues.Store(workflow.ID, queue)

	// Start single writer goroutine for task queue updates.
	// This prevents race conditions where multiple goroutines overwrite each other's updates.
	writer := s.startTaskQueueWriter(workflow.ID)

	// Set up callback to sync task queue to database for UI visibility
	queue.SetOnUpdate(func(snapshots []workqueue.TaskSnapshot) {
		// Convert snapshots to models.WorkflowTask with status mapping
		tasks := make([]models.WorkflowTask, len(snapshots))
		for i, snap := range snapshots {
			// Map workqueue status to model status
			// workqueue: pending, running, completed, failed, cancelled, paused
			// model/UI:  queued, processing, complete, failed, paused
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
			// Buffer full - this shouldn't happen with buffer size 100
			s.logger.Warn("Task queue update buffer full, dropping update",
				zap.String("workflow_id", workflow.ID.String()))
		}
	})

	initTask := NewInitializeOntologyTask(
		s.schemaRepo,
		s.ontologyRepo,
		s.stateRepo,
		s.dsSvc,
		s.adapterFactory,
		s.builder,
		s, // OntologyWorkflowService for progress updates
		s.getTenantCtx,
		projectID,
		workflow.ID,
		config.DatasourceID,
		config.ProjectDescription,
	)
	queue.Enqueue(initTask)

	// Run workflow in background - HTTP request returns immediately
	go s.runWorkflow(projectID, workflow.ID, queue)

	return workflow, nil
}

// runWorkflow waits for init task, then runs the orchestrator.
// Runs in a background goroutine - acquires its own DB connection.
func (s *ontologyWorkflowService) runWorkflow(projectID, workflowID uuid.UUID, queue *workqueue.Queue) {
	// Clean up when done
	defer s.activeQueues.Delete(workflowID)
	defer s.stopTaskQueueWriter(workflowID) // Stop the writer and flush final state
	defer s.stopHeartbeat(workflowID)       // Stop heartbeat goroutine
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

	// Wait for init task to complete (description processing if any)
	err := queue.Wait(context.Background())
	if err != nil {
		s.logger.Error("Init task failed",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, err.Error())
		return
	}

	// Create orchestrator to drive the workflow
	orchestrator := NewOrchestrator(
		s.stateRepo,
		s.getTenantCtx,
		s.logger,
		queue,
		s.workflowRepo,
		s.schemaRepo,
		s.ontologyRepo,
		s.questionRepo,
		s.dsSvc,
		s.adapterFactory,
		s.builder,
		s, // OntologyWorkflowService
		0, // Use DefaultPollInterval
	)

	// Run orchestrator loop
	if err := orchestrator.Run(context.Background(), projectID, workflowID); err != nil {
		s.logger.Error("Orchestrator failed",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		s.markWorkflowFailed(projectID, workflowID, err.Error())
		return
	}

	// Orchestrator completed - check final state
	s.finalizeWorkflow(projectID, workflowID)
}

// markWorkflowFailed updates workflow state to failed.
func (s *ontologyWorkflowService) markWorkflowFailed(projectID, workflowID uuid.UUID, errMsg string) {
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

// finalizeWorkflow marks workflow as complete.
// Questions are now decoupled from workflow lifecycle - workflow completes regardless of pending questions.
func (s *ontologyWorkflowService) finalizeWorkflow(projectID, workflowID uuid.UUID) {
	ctx, cleanup, dbErr := s.getTenantCtx(context.Background(), projectID)
	if dbErr != nil {
		s.logger.Error("Failed to acquire DB connection for workflow completion",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(dbErr))
		return
	}
	defer cleanup()

	// Get current workflow to preserve entity counts from orchestrator
	workflow, wfErr := s.workflowRepo.GetByID(ctx, workflowID)
	entityCount := 0
	if wfErr == nil && workflow != nil && workflow.Progress != nil {
		entityCount = workflow.Progress.Total
	}
	if entityCount == 0 {
		entityCount = 1 // Fallback to avoid 0/0
	}

	// Get pending question counts for logging (questions are in separate table now)
	counts, qErr := s.questionRepo.GetPendingCounts(ctx, projectID)
	pendingCount := 0
	if qErr != nil {
		s.logger.Error("Failed to check pending questions",
			zap.String("project_id", projectID.String()),
			zap.Error(qErr))
	} else if counts != nil {
		pendingCount = counts.Required + counts.Optional
	}

	// Always mark workflow as completed - questions are handled independently
	completionMsg := "Ontology extraction complete"
	if pendingCount > 0 {
		completionMsg = fmt.Sprintf("Ontology extraction complete (%d questions pending)", pendingCount)
	}

	if updateErr := s.workflowRepo.UpdateProgress(ctx, workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseCompleting,
		Message:      completionMsg,
		Current:      entityCount,
		Total:        entityCount,
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

	s.logger.Info("Workflow completed successfully",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("pending_questions", pendingCount),
		zap.Int("entity_count", entityCount))
}

func (s *ontologyWorkflowService) GetStatus(ctx context.Context, projectID uuid.UUID) (*models.OntologyWorkflow, error) {
	workflow, err := s.workflowRepo.GetLatestByProject(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get workflow status",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}
	return workflow, nil
}

func (s *ontologyWorkflowService) GetByID(ctx context.Context, workflowID uuid.UUID) (*models.OntologyWorkflow, error) {
	workflow, err := s.workflowRepo.GetByID(ctx, workflowID)
	if err != nil {
		s.logger.Error("Failed to get workflow by ID",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return nil, err
	}
	return workflow, nil
}

func (s *ontologyWorkflowService) GetOntology(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	ontology, err := s.ontologyRepo.GetActive(ctx, projectID)
	if err != nil {
		s.logger.Error("Failed to get active ontology",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return nil, err
	}
	return ontology, nil
}

func (s *ontologyWorkflowService) Cancel(ctx context.Context, workflowID uuid.UUID) error {
	// Cancel active queue if running (signals tasks to stop)
	if queueVal, ok := s.activeQueues.Load(workflowID); ok {
		if queue, ok := queueVal.(*workqueue.Queue); ok {
			queue.Cancel()
			s.logger.Info("Queue cancelled for workflow",
				zap.String("workflow_id", workflowID.String()))
		}
		s.activeQueues.Delete(workflowID)
	}

	// Delete workflow record - cascades to questions and LLM conversations
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

func (s *ontologyWorkflowService) DeleteOntology(ctx context.Context, projectID uuid.UUID) error {
	// Cancel any active workflow first
	workflow, err := s.workflowRepo.GetLatestByProject(ctx, projectID)
	if err != nil {
		// Log but continue - we still want to delete ontologies even if workflow lookup fails
		s.logger.Error("Failed to get latest workflow for delete",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	}
	if workflow != nil && !workflow.State.IsTerminal() {
		if queueVal, ok := s.activeQueues.Load(workflow.ID); ok {
			if queue, ok := queueVal.(*workqueue.Queue); ok {
				queue.Cancel()
			}
		}
		s.activeQueues.Delete(workflow.ID)
		s.stopTaskQueueWriter(workflow.ID)
	}

	// Delete all ontologies for project (cascades to workflows, questions, and chat messages via FK)
	if err := s.ontologyRepo.DeleteByProject(ctx, projectID); err != nil {
		s.logger.Error("Failed to delete ontologies",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return fmt.Errorf("delete ontologies: %w", err)
	}

	// Delete LLM conversations for this project (audit trail from ontology extraction)
	if err := s.convRepo.DeleteByProject(ctx, projectID); err != nil {
		s.logger.Error("Failed to delete LLM conversations",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		return fmt.Errorf("delete llm conversations: %w", err)
	}

	s.logger.Info("Deleted all ontology data for project",
		zap.String("project_id", projectID.String()))

	return nil
}

func (s *ontologyWorkflowService) UpdateProgress(ctx context.Context, workflowID uuid.UUID, progress *models.WorkflowProgress) error {
	if err := s.workflowRepo.UpdateProgress(ctx, workflowID, progress); err != nil {
		s.logger.Error("Failed to update workflow progress",
			zap.String("workflow_id", workflowID.String()),
			zap.Error(err))
		return err
	}
	return nil
}

func (s *ontologyWorkflowService) MarkComplete(ctx context.Context, workflowID uuid.UUID) error {
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

func (s *ontologyWorkflowService) MarkFailed(ctx context.Context, workflowID uuid.UUID, errMsg string) error {
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
// All task queue updates are serialized through this writer to prevent race conditions.
func (s *ontologyWorkflowService) startTaskQueueWriter(workflowID uuid.UUID) *taskQueueWriter {
	writer := &taskQueueWriter{
		updates: make(chan taskQueueUpdate, 100), // Buffer to avoid blocking the workqueue
		done:    make(chan struct{}),
	}
	s.taskQueueWriters.Store(workflowID, writer)
	go s.runTaskQueueWriter(writer)
	return writer
}

// stopTaskQueueWriter closes the channel and waits for the writer to finish.
func (s *ontologyWorkflowService) stopTaskQueueWriter(workflowID uuid.UUID) {
	if writerVal, ok := s.taskQueueWriters.LoadAndDelete(workflowID); ok {
		writer := writerVal.(*taskQueueWriter)
		close(writer.updates)
		<-writer.done // Wait for writer to finish
	}
}

// runTaskQueueWriter is the single writer goroutine that processes updates.
// It drains all pending updates and only persists the latest one (debounce).
func (s *ontologyWorkflowService) runTaskQueueWriter(writer *taskQueueWriter) {
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
func (s *ontologyWorkflowService) persistTaskQueue(projectID, workflowID uuid.UUID, tasks []models.WorkflowTask) {
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

// ============================================================================
// Workflow Entity State Initialization
// ============================================================================

// initializeWorkflowEntities creates workflow state rows for all entities.
// Creates one global entity, one entity per table, and one entity per column.
//
// If relWorkflowID is provided (non-nil), scan data from the relationships phase
// will be copied to column entities, and both column and table entities will be
// marked as "scanned" instead of "pending" (Milestone 3.4: skip scanning/reuse data).
func (s *ontologyWorkflowService) initializeWorkflowEntities(
	ctx context.Context,
	projectID, workflowID, ontologyID, datasourceID, relWorkflowID uuid.UUID,
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

	// Load scan data from relationships workflow (Milestone 3.4)
	// This allows us to skip the scanning phase and reuse data
	relScanData := make(map[string]*models.WorkflowStateData) // entityKey -> stateData
	if relWorkflowID != uuid.Nil {
		relStates, err := s.stateRepo.ListByWorkflow(ctx, relWorkflowID)
		if err != nil {
			s.logger.Warn("Failed to load relationships workflow state, will scan from scratch",
				zap.String("rel_workflow_id", relWorkflowID.String()),
				zap.Error(err))
		} else {
			for _, state := range relStates {
				// Only copy column scan data (relationships workflow only has column entities)
				if state.EntityType == models.WorkflowEntityTypeColumn && state.StateData != nil && state.StateData.Gathered != nil {
					relScanData[state.EntityKey] = state.StateData
				}
			}
			s.logger.Info("Loaded scan data from relationships workflow",
				zap.String("rel_workflow_id", relWorkflowID.String()),
				zap.Int("columns_with_scan_data", len(relScanData)))
		}
	}

	// Build entity states slice
	states := make([]*models.WorkflowEntityState, 0, 1+len(tables)+len(columns))

	// 1. Global entity (always pending - no scan data to reuse)
	states = append(states, &models.WorkflowEntityState{
		ProjectID:  projectID,
		OntologyID: ontologyID,
		WorkflowID: workflowID,
		EntityType: models.WorkflowEntityTypeGlobal,
		EntityKey:  models.GlobalEntityKey(),
		Status:     models.WorkflowEntityStatusPending,
		StateData:  &models.WorkflowStateData{},
	})

	// Track tables that have all columns with scan data
	tableHasAllColumnData := make(map[string]bool)
	tableCols := make(map[string]int)

	// Build table lookup and count columns per table
	tableByID := make(map[uuid.UUID]*models.SchemaTable)
	for _, table := range tables {
		tableByID[table.ID] = table
		tableHasAllColumnData[table.TableName] = true // Assume true, will be set false if any column lacks data
	}
	for _, col := range columns {
		table := tableByID[col.SchemaTableID]
		if table == nil {
			continue
		}
		tableCols[table.TableName]++
		entityKey := models.ColumnEntityKey(table.TableName, col.ColumnName)
		if _, hasScanData := relScanData[entityKey]; !hasScanData {
			tableHasAllColumnData[table.TableName] = false
		}
	}

	// 2. Table entities
	// If all columns for a table have scan data, mark table as "scanned" (skip scanning phase)
	tablesWithScanData := 0
	for _, table := range tables {
		status := models.WorkflowEntityStatusPending
		if len(relScanData) > 0 && tableHasAllColumnData[table.TableName] && tableCols[table.TableName] > 0 {
			status = models.WorkflowEntityStatusScanned
			tablesWithScanData++
		}
		states = append(states, &models.WorkflowEntityState{
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeTable,
			EntityKey:  models.TableEntityKey(table.TableName),
			Status:     status,
			StateData:  &models.WorkflowStateData{},
		})
	}

	// 3. Column entities
	// Copy scan data from relationships workflow if available
	columnsWithScanData := 0
	for _, col := range columns {
		table := tableByID[col.SchemaTableID]
		if table == nil {
			continue // Skip orphaned columns
		}
		entityKey := models.ColumnEntityKey(table.TableName, col.ColumnName)

		status := models.WorkflowEntityStatusPending
		stateData := &models.WorkflowStateData{}

		// Copy scan data from relationships workflow if available
		if scanData, hasScanData := relScanData[entityKey]; hasScanData {
			status = models.WorkflowEntityStatusScanned
			stateData = scanData
			columnsWithScanData++
		}

		states = append(states, &models.WorkflowEntityState{
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  entityKey,
			Status:     status,
			StateData:  stateData,
		})
	}

	// Batch create all entities
	if err := s.stateRepo.CreateBatch(ctx, states); err != nil {
		return fmt.Errorf("create workflow entities: %w", err)
	}

	s.logger.Info("Initialized workflow entities",
		zap.String("workflow_id", workflowID.String()),
		zap.Int("total_entities", len(states)),
		zap.Int("tables", len(tables)),
		zap.Int("columns", len(columns)),
		zap.Int("tables_with_scan_data", tablesWithScanData),
		zap.Int("columns_with_scan_data", columnsWithScanData))

	return nil
}

// ============================================================================
// Heartbeat Management
// ============================================================================

// startHeartbeat launches a background goroutine that periodically updates
// the workflow's last_heartbeat timestamp to maintain ownership.
func (s *ontologyWorkflowService) startHeartbeat(workflowID, projectID uuid.UUID) {
	stop := make(chan struct{})
	info := &heartbeatInfo{
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
func (s *ontologyWorkflowService) stopHeartbeat(workflowID uuid.UUID) {
	if infoVal, ok := s.heartbeatStop.LoadAndDelete(workflowID); ok {
		info := infoVal.(*heartbeatInfo)
		close(info.stop)
	}
}

// ============================================================================
// Graceful Shutdown
// ============================================================================

// Shutdown gracefully stops all active workflows owned by this server.
// Called during server shutdown to release ownership so new servers can take over.
func (s *ontologyWorkflowService) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down workflow service",
		zap.String("server_instance_id", s.serverInstanceID.String()))

	var wg sync.WaitGroup

	// Cancel all active queues and release ownership
	s.activeQueues.Range(func(key, value any) bool {
		workflowID := key.(uuid.UUID)
		queue := value.(*workqueue.Queue)

		// Get project ID from heartbeat info if available
		var projectID uuid.UUID
		if infoVal, ok := s.heartbeatStop.Load(workflowID); ok {
			info := infoVal.(*heartbeatInfo)
			projectID = info.projectID
		}

		wg.Add(1)
		go func(wfID uuid.UUID, q *workqueue.Queue, pID uuid.UUID) {
			defer wg.Done()

			s.logger.Info("Cancelling workflow for shutdown",
				zap.String("workflow_id", wfID.String()))

			// Cancel the queue (signals tasks to stop)
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
		s.logger.Info("All workflows cancelled successfully")
		return nil
	case <-ctx.Done():
		s.logger.Warn("Shutdown timed out, some workflows may not have been cleaned up")
		return ctx.Err()
	}
}

// GetSchemaEntityCount returns the total number of entities (1 global + tables + columns)
// that would be processed by an ontology extraction workflow.
func (s *ontologyWorkflowService) GetSchemaEntityCount(ctx context.Context, projectID uuid.UUID) (int, error) {
	// Get datasources for this project to find the one to count entities from
	datasources, err := s.dsSvc.List(ctx, projectID)
	if err != nil {
		return 0, fmt.Errorf("list datasources: %w", err)
	}

	if len(datasources) == 0 {
		return 0, fmt.Errorf("no datasource configured for project")
	}

	// Use the first datasource. Projects typically have one datasource, or the first
	// is the primary one used for ontology extraction. A more robust approach would
	// be to use the project's default_datasource_id, but that requires ProjectService.
	datasourceID := datasources[0].ID

	// Count tables
	tables, err := s.schemaRepo.ListTablesByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return 0, fmt.Errorf("list tables: %w", err)
	}

	// Count columns
	columns, err := s.schemaRepo.ListColumnsByDatasource(ctx, projectID, datasourceID)
	if err != nil {
		return 0, fmt.Errorf("list columns: %w", err)
	}

	// Total = 1 (global) + tables + columns
	return 1 + len(tables) + len(columns), nil
}
