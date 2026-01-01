package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// DefaultPollInterval is the default interval for polling entity states.
const DefaultPollInterval = 2 * time.Second

// Orchestrator coordinates ontology extraction workflows by reading entity states
// and determining what work needs to be done.
// In the simplified flow, it only manages the global entity and triggers BuildTieredOntologyTask.
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
	workflowRepo repositories.OntologyWorkflowRepository
	ontologyRepo repositories.OntologyRepository
	builder      OntologyBuilderService
	workflowSvc  OntologyWorkflowService
}

// NewOrchestrator creates a new workflow orchestrator.
// If pollInterval is 0, DefaultPollInterval is used.
func NewOrchestrator(
	stateRepo repositories.WorkflowStateRepository,
	getTenantCtx TenantContextFunc,
	logger *zap.Logger,
	queue *workqueue.Queue,
	workflowRepo repositories.OntologyWorkflowRepository,
	ontologyRepo repositories.OntologyRepository,
	builder OntologyBuilderService,
	workflowSvc OntologyWorkflowService,
	pollInterval time.Duration,
) *Orchestrator {
	if pollInterval == 0 {
		pollInterval = DefaultPollInterval
	}
	return &Orchestrator{
		stateRepo:    stateRepo,
		getTenantCtx: getTenantCtx,
		logger:       logger.Named("orchestrator"),
		queue:        queue,
		pollInterval: pollInterval,
		workflowRepo: workflowRepo,
		ontologyRepo: ontologyRepo,
		builder:      builder,
		workflowSvc:  workflowSvc,
	}
}

// Run executes the orchestration loop for a workflow.
// In the simplified flow, this only manages the global entity and triggers BuildTieredOntologyTask.
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

		// Update progress based on current entity states
		o.updateProgress(tenantCtx, workflowID, states)

		// Release DB connection before potentially waiting
		cleanup()

		// Check if we should trigger global entity analysis
		globalTriggered, err := o.maybeEnqueueGlobalTask(ctx, projectID, workflowID, states)
		if err != nil {
			return fmt.Errorf("check global entity: %w", err)
		}

		// If no global task triggered, check if we're done
		if !globalTriggered {
			if o.isAllComplete(states) {
				o.logger.Info("Workflow complete - all entities in terminal state",
					zap.String("workflow_id", workflowID.String()))
				return o.finalizeWorkflow(ctx, projectID, workflowID)
			}

			// No work to do but not all complete (waiting for in-progress tasks)
			o.logger.Debug("No work to enqueue - waiting for in-progress tasks",
				zap.String("workflow_id", workflowID.String()))
			time.Sleep(o.pollInterval)
			continue
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
		o.updateProgress(tenantCtx2, workflowID, updatedStates)
		cleanup2()

		// Handle wait errors gracefully
		if waitErr != nil {
			// Context cancellation is fatal
			if ctx.Err() != nil {
				return fmt.Errorf("context cancelled during wait: %w", waitErr)
			}

			// Log the task failure but continue processing
			o.logger.Warn("Task failed during execution - continuing with remaining tasks",
				zap.String("workflow_id", workflowID.String()),
				zap.Error(waitErr))
		}
	}
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

// maybeEnqueueGlobalTask checks if the global entity is pending and triggers
// the BuildTieredOntologyTask. Returns true if a global task was enqueued.
// In the simplified flow, there are no TABLE entities - only the global entity.
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

	o.logger.Info("Triggering global ontology build",
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
func (o *Orchestrator) updateProgress(ctx context.Context, workflowID uuid.UUID, states []*models.WorkflowEntityState) {
	// In simplified flow, we only have the global entity
	completed := 0
	total := len(states)
	for _, state := range states {
		if state.Status == models.WorkflowEntityStatusComplete {
			completed++
		}
	}

	// Build message - simplified for global-only flow
	message := "Building ontology from entities and relationships..."
	if completed == total && total > 0 {
		message = "Ontology build complete"
	}

	// Update progress (ignore errors - progress is non-critical)
	_ = o.workflowSvc.UpdateProgress(ctx, workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseTier1Building,
		Message:      message,
		Current:      completed,
		Total:        total,
	})
}
