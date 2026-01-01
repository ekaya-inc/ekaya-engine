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
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workflow"
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

	// GetOntologyEntityCount returns the total number of entities (1 global + tables + columns)
	// that would be processed by an ontology extraction workflow.
	GetOntologyEntityCount(ctx context.Context, projectID uuid.UUID) (int, error)
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
	infra          *workflow.WorkflowInfra
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
	namedLogger := logger.Named("ontology-workflow")
	infra := workflow.NewWorkflowInfra(workflowRepo, workflow.TenantContextFunc(getTenantCtx), namedLogger)

	return &ontologyWorkflowService{
		workflowRepo:   workflowRepo,
		ontologyRepo:   ontologyRepo,
		schemaRepo:     schemaRepo,
		stateRepo:      stateRepo,
		questionRepo:   questionRepo,
		convRepo:       convRepo,
		dsSvc:          dsSvc,
		adapterFactory: adapterFactory,
		builder:        builder,
		getTenantCtx:   getTenantCtx,
		logger:         namedLogger,
		infra:          infra,
	}
}

var _ OntologyWorkflowService = (*ontologyWorkflowService)(nil)

func (s *ontologyWorkflowService) StartExtraction(ctx context.Context, projectID uuid.UUID, config *models.WorkflowConfig) (*models.OntologyWorkflow, error) {
	// Set defaults
	if config == nil {
		config = models.DefaultWorkflowConfig()
	}

	// Step 0: Check prerequisites - BOTH entities AND relationships phases must complete
	// Ontology is the "combination layer" that consumes data from both phases.

	// Check entities phase
	entitiesWorkflow, err := s.workflowRepo.GetLatestByDatasourceAndPhase(ctx, config.DatasourceID, models.WorkflowPhaseEntities)
	if err != nil {
		s.logger.Error("Failed to check entities workflow status",
			zap.String("datasource_id", config.DatasourceID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("check entities workflow: %w", err)
	}
	if entitiesWorkflow == nil {
		return nil, fmt.Errorf("entities phase must complete before ontology extraction: no entity discovery workflow found for datasource")
	}
	if entitiesWorkflow.State != models.WorkflowStateCompleted {
		return nil, fmt.Errorf("entities phase must complete before ontology extraction: current state is %s", entitiesWorkflow.State)
	}

	// Check relationships phase
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

	s.logger.Info("Prerequisites verified: entities and relationships phases complete",
		zap.String("datasource_id", config.DatasourceID.String()),
		zap.String("entities_workflow_id", entitiesWorkflow.ID.String()),
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
	wf := &models.OntologyWorkflow{
		ID:         uuid.New(),
		ProjectID:  projectID,
		OntologyID: ontology.ID,
		State:      models.WorkflowStatePending,
		Phase:      models.WorkflowPhaseOntology, // This is the ontology extraction phase
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

	if err := s.workflowRepo.Create(ctx, wf); err != nil {
		s.logger.Error("Failed to create workflow",
			zap.String("project_id", projectID.String()),
			zap.String("ontology_id", ontology.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Step 7: Initialize workflow state with global entity only
	if err := s.initializeWorkflowEntities(ctx, projectID, wf.ID, ontology.ID); err != nil {
		s.logger.Error("Failed to initialize workflow entities",
			zap.String("workflow_id", wf.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Claim ownership and transition to running
	claimed, err := s.workflowRepo.ClaimOwnership(ctx, wf.ID, s.infra.ServerInstanceID())
	if err != nil {
		s.logger.Error("Failed to claim workflow ownership",
			zap.String("workflow_id", wf.ID.String()),
			zap.Error(err))
		return nil, fmt.Errorf("claim ownership: %w", err)
	}
	if !claimed {
		s.logger.Error("Workflow already owned by another server",
			zap.String("workflow_id", wf.ID.String()))
		return nil, fmt.Errorf("workflow already owned by another server")
	}

	// Transition to running
	wf.State = models.WorkflowStateRunning
	if err := s.workflowRepo.UpdateState(ctx, wf.ID, models.WorkflowStateRunning, ""); err != nil {
		s.logger.Error("Failed to start workflow",
			zap.String("workflow_id", wf.ID.String()),
			zap.Error(err))
		return nil, err
	}

	// Start heartbeat goroutine to maintain ownership
	s.infra.StartHeartbeat(wf.ID, projectID)

	s.logger.Info("Ontology extraction started",
		zap.String("project_id", projectID.String()),
		zap.String("workflow_id", wf.ID.String()),
		zap.String("ontology_id", ontology.ID.String()),
		zap.String("server_instance_id", s.infra.ServerInstanceID().String()),
		zap.Int("ontology_version", nextVersion))

	// Create work queue and enqueue the initialization task
	// InitializeOntologyTask will enqueue all other tasks (data tasks, LLM tasks, etc.)
	// Queue uses ParallelLLMStrategy: LLM tasks run in parallel (no limit),
	// data tasks serialize with data tasks.
	// This allows efficient batching of LLM requests to the model provider.
	// Default retry config: 24 retries with exponential backoff (2s initial, 30s max, ~10min total).
	queue := workqueue.New(s.logger, workqueue.WithStrategy(workqueue.NewParallelLLMStrategy()))
	s.infra.StoreQueue(wf.ID, queue)

	// Start single writer goroutine for task queue updates.
	// This prevents race conditions where multiple goroutines overwrite each other's updates.
	s.infra.StartTaskQueueWriter(wf.ID)

	// Set up callback to sync task queue to database for UI visibility
	workflowID := wf.ID // Capture for closure
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
		s.infra.SendTaskQueueUpdate(workflow.TaskQueueUpdate{
			ProjectID:  projectID,
			WorkflowID: workflowID,
			Tasks:      tasks,
		})
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
		wf.ID,
		config.DatasourceID,
		config.ProjectDescription,
	)
	queue.Enqueue(initTask)

	// Run workflow in background - HTTP request returns immediately
	go s.runWorkflow(projectID, wf.ID, queue)

	return wf, nil
}

// runWorkflow waits for init task, then runs the orchestrator.
// Runs in a background goroutine - acquires its own DB connection.
func (s *ontologyWorkflowService) runWorkflow(projectID, workflowID uuid.UUID, queue *workqueue.Queue) {
	// Clean up when done
	defer s.infra.DeleteQueue(workflowID)
	defer s.infra.StopTaskQueueWriter(workflowID) // Stop the writer and flush final state
	defer s.infra.StopHeartbeat(workflowID)       // Stop heartbeat goroutine
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
		s.ontologyRepo,
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
	if queue, ok := s.infra.LoadQueue(workflowID); ok {
		queue.Cancel()
		s.logger.Info("Queue cancelled for workflow",
			zap.String("workflow_id", workflowID.String()))
		s.infra.DeleteQueue(workflowID)
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
	wf, err := s.workflowRepo.GetLatestByProject(ctx, projectID)
	if err != nil {
		// Log but continue - we still want to delete ontologies even if workflow lookup fails
		s.logger.Error("Failed to get latest workflow for delete",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	}
	if wf != nil && !wf.State.IsTerminal() {
		if queue, ok := s.infra.LoadQueue(wf.ID); ok {
			queue.Cancel()
		}
		s.infra.DeleteQueue(wf.ID)
		s.infra.StopTaskQueueWriter(wf.ID)
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

// ============================================================================
// Workflow Entity State Initialization
// ============================================================================

// initializeWorkflowEntities creates workflow state rows for the ontology extraction.
// Creates only a global entity - table/column scanning is handled by the relationships phase.
//
// The ontology workflow now:
// 1. Requires entities AND relationships phases to be complete (checked in StartExtraction)
// 2. Uses schema data and domain entities as input (no re-scanning)
// 3. Immediately triggers BuildTieredOntologyTask since there are no table entities to process
func (s *ontologyWorkflowService) initializeWorkflowEntities(
	ctx context.Context,
	projectID, workflowID, ontologyID uuid.UUID,
) error {
	// Only create the global entity
	// The orchestrator will see no table entities and immediately trigger BuildTieredOntologyTask
	states := []*models.WorkflowEntityState{
		{
			ProjectID:  projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeGlobal,
			EntityKey:  models.GlobalEntityKey(),
			Status:     models.WorkflowEntityStatusPending,
			StateData:  &models.WorkflowStateData{},
		},
	}

	// Create the global entity
	if err := s.stateRepo.CreateBatch(ctx, states); err != nil {
		return fmt.Errorf("create workflow entities: %w", err)
	}

	s.logger.Info("Initialized workflow with global entity only",
		zap.String("workflow_id", workflowID.String()),
		zap.String("ontology_id", ontologyID.String()))

	return nil
}

// ============================================================================
// Graceful Shutdown
// ============================================================================

// Shutdown gracefully stops all active workflows owned by this server.
// Called during server shutdown to release ownership so new servers can take over.
func (s *ontologyWorkflowService) Shutdown(ctx context.Context) error {
	s.logger.Info("Shutting down ontology workflow service",
		zap.String("server_instance_id", s.infra.ServerInstanceID().String()))

	return s.infra.Shutdown(ctx, func(workflowID, projectID uuid.UUID, queue *workqueue.Queue) {
		s.logger.Info("Cancelling workflow for shutdown",
			zap.String("workflow_id", workflowID.String()))

		// Cancel the queue (signals tasks to stop)
		queue.Cancel()

		// Release ownership if we have a project ID
		if projectID != uuid.Nil {
			tenantCtx, cleanup, err := s.getTenantCtx(context.Background(), projectID)
			if err == nil {
				if releaseErr := s.workflowRepo.ReleaseOwnership(tenantCtx, workflowID); releaseErr != nil {
					s.logger.Error("Failed to release ownership during shutdown",
						zap.String("workflow_id", workflowID.String()),
						zap.Error(releaseErr))
				}
				cleanup()
			}
		}
	})
}

// GetOntologyEntityCount returns the total number of entities that would be
// processed by an ontology extraction workflow.
// Since the ontology now builds from domain entities (from the Entities phase)
// and relationships (from the Relationships phase), we only have a single
// global entity in the workflow state.
func (s *ontologyWorkflowService) GetOntologyEntityCount(_ context.Context, _ uuid.UUID) (int, error) {
	// The ontology workflow now only has a global entity.
	// The actual work is done by BuildTieredOntology which processes
	// domain entities and relationships from the prerequisite phases.
	return 1, nil
}
