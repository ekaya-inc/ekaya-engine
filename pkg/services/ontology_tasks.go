package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workflow"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workqueue"
)

// TenantContextFunc is an alias for workflow.TenantContextFunc for backwards compatibility.
// New code should import from pkg/services/workflow directly.
type TenantContextFunc = workflow.TenantContextFunc

// NewTenantContextFunc creates a TenantContextFunc that uses the given database.
func NewTenantContextFunc(db *database.DB) workflow.TenantContextFunc {
	return func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		scope, err := db.WithTenant(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		tenantCtx := database.SetTenantScope(ctx, scope)
		return tenantCtx, func() { scope.Close() }, nil
	}
}

// BuildTieredOntologyTask builds Tier 0 (domain summary) and Tier 1 (entity summaries).
// This is an LLM task - only one can run at a time.
type BuildTieredOntologyTask struct {
	workqueue.BaseTask
	builder      OntologyBuilderService
	getTenantCtx TenantContextFunc
	projectID    uuid.UUID
	workflowID   uuid.UUID
}

// NewBuildTieredOntologyTask creates a new build task.
func NewBuildTieredOntologyTask(
	builder OntologyBuilderService,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
) *BuildTieredOntologyTask {
	return &BuildTieredOntologyTask{
		BaseTask:     workqueue.NewBaseTask("Build Tiered Ontology", true),
		builder:      builder,
		getTenantCtx: getTenantCtx,
		projectID:    projectID,
		workflowID:   workflowID,
	}
}

// Execute implements workqueue.Task.
func (t *BuildTieredOntologyTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	return t.builder.BuildTieredOntology(tenantCtx, t.projectID, t.workflowID)
}

// InitializeOntologyTask is a quick non-LLM task that sets up the extraction workflow.
// It loads tables from schema and enqueues child tasks for processing.
type InitializeOntologyTask struct {
	workqueue.BaseTask
	schemaRepo        repositories.SchemaRepository
	ontologyRepo      repositories.OntologyRepository
	workflowStateRepo repositories.WorkflowStateRepository
	dsSvc             DatasourceService
	adapterFactory    datasource.DatasourceAdapterFactory
	builder           OntologyBuilderService
	workflowService   OntologyWorkflowService
	getTenantCtx      TenantContextFunc
	projectID         uuid.UUID
	workflowID        uuid.UUID
	datasourceID      uuid.UUID
	description       string
}

// NewInitializeOntologyTask creates a new initialization task.
func NewInitializeOntologyTask(
	schemaRepo repositories.SchemaRepository,
	ontologyRepo repositories.OntologyRepository,
	workflowStateRepo repositories.WorkflowStateRepository,
	dsSvc DatasourceService,
	adapterFactory datasource.DatasourceAdapterFactory,
	builder OntologyBuilderService,
	workflowService OntologyWorkflowService,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	datasourceID uuid.UUID,
	description string,
) *InitializeOntologyTask {
	return &InitializeOntologyTask{
		BaseTask:          workqueue.NewBaseTask("Initialize Ontology", false), // Non-LLM task
		schemaRepo:        schemaRepo,
		ontologyRepo:      ontologyRepo,
		workflowStateRepo: workflowStateRepo,
		dsSvc:             dsSvc,
		adapterFactory:    adapterFactory,
		builder:           builder,
		workflowService:   workflowService,
		getTenantCtx:      getTenantCtx,
		projectID:         projectID,
		workflowID:        workflowID,
		datasourceID:      datasourceID,
		description:       description,
	}
}

// Execute implements workqueue.Task.
// This task initializes the ontology workflow and optionally enqueues the project description task.
// The orchestrator will immediately trigger BuildTieredOntologyTask since there are no table entities.
func (t *InitializeOntologyTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Update progress - ontology now builds from entities and relationships
	_ = t.workflowService.UpdateProgress(tenantCtx, t.workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseTier1Building,
		Message:      "Building ontology from entities and relationships...",
		Current:      0,
		Total:        1, // Single global entity
	})

	// Enqueue UnderstandProjectDescriptionTask if description provided (LLM task)
	// This runs before the orchestrator takes over, processing the user's description
	if t.description != "" {
		descTask := NewUnderstandProjectDescriptionTask(
			t.builder,
			t.workflowService,
			t.getTenantCtx,
			t.projectID,
			t.workflowID,
			t.description,
			1, // Single global entity
		)
		enqueuer.Enqueue(descTask)
	}

	// The orchestrator will see no table entities and immediately trigger BuildTieredOntologyTask

	return nil
}

// UnderstandProjectDescriptionTask processes the user's project description with LLM.
// This is an LLM task - only one can run at a time.
type UnderstandProjectDescriptionTask struct {
	workqueue.BaseTask
	builder         OntologyBuilderService
	workflowService OntologyWorkflowService
	getTenantCtx    TenantContextFunc
	projectID       uuid.UUID
	workflowID      uuid.UUID
	description     string
	tableCount      int
}

// NewUnderstandProjectDescriptionTask creates a new description processing task.
func NewUnderstandProjectDescriptionTask(
	builder OntologyBuilderService,
	workflowService OntologyWorkflowService,
	getTenantCtx TenantContextFunc,
	projectID uuid.UUID,
	workflowID uuid.UUID,
	description string,
	tableCount int,
) *UnderstandProjectDescriptionTask {
	return &UnderstandProjectDescriptionTask{
		BaseTask:        workqueue.NewBaseTask("Understand Project Description", true), // LLM task
		builder:         builder,
		workflowService: workflowService,
		getTenantCtx:    getTenantCtx,
		projectID:       projectID,
		workflowID:      workflowID,
		description:     description,
		tableCount:      tableCount,
	}
}

// Execute implements workqueue.Task.
func (t *UnderstandProjectDescriptionTask) Execute(ctx context.Context, enqueuer workqueue.TaskEnqueuer) error {
	tenantCtx, cleanup, err := t.getTenantCtx(ctx, t.projectID)
	if err != nil {
		return fmt.Errorf("acquire tenant connection: %w", err)
	}
	defer cleanup()

	// Update progress
	_ = t.workflowService.UpdateProgress(tenantCtx, t.workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseDescriptionProcessing,
		Message:      "Analyzing your project description...",
		Current:      0,
		Total:        1, // Single global entity
	})

	// Process description with LLM
	_, err = t.builder.ProcessProjectDescription(tenantCtx, t.projectID, t.workflowID, t.description)
	if err != nil {
		// Non-fatal - continue without description context
		// ProcessProjectDescription stores results in metadata even on partial failure
	}

	// Update progress - transition to building phase (no scanning in simplified flow)
	_ = t.workflowService.UpdateProgress(tenantCtx, t.workflowID, &models.WorkflowProgress{
		CurrentPhase: models.WorkflowPhaseTier1Building,
		Message:      "Building ontology from entities and relationships...",
		Current:      0,
		Total:        1, // Single global entity
	})

	return nil
}

// NOTE: UnderstandEntityTask, ScanTableDataTask, and question deduplication were removed.
// The ontology workflow now builds directly from domain entities and relationships,
// without per-table scanning or LLM analysis phases.
