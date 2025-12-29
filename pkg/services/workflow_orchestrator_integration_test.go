//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// orchestratorTestContext holds all dependencies for orchestrator integration tests.
type orchestratorTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	stateRepo    repositories.WorkflowStateRepository
	workflowRepo repositories.OntologyWorkflowRepository
	ontologyRepo repositories.OntologyRepository
	getTenantCtx TenantContextFunc
	projectID    uuid.UUID
	logger       *zap.Logger
}

// setupOrchestratorTest creates a test context for orchestrator tests.
func setupOrchestratorTest(t *testing.T) *orchestratorTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	logger := zap.NewNop()

	// Create repositories
	stateRepo := repositories.NewWorkflowStateRepository()
	workflowRepo := repositories.NewOntologyWorkflowRepository()
	ontologyRepo := repositories.NewOntologyRepository()

	// Use unique project ID for test isolation
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000301")

	// Create getTenantCtx function
	getTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		scope, err := engineDB.DB.WithTenant(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		tenantCtx := database.SetTenantScope(ctx, scope)
		return tenantCtx, func() { scope.Close() }, nil
	}

	tc := &orchestratorTestContext{
		t:            t,
		engineDB:     engineDB,
		stateRepo:    stateRepo,
		workflowRepo: workflowRepo,
		ontologyRepo: ontologyRepo,
		getTenantCtx: getTenantCtx,
		projectID:    projectID,
		logger:       logger,
	}

	// Ensure project exists
	tc.ensureTestProject()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *orchestratorTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, scope)

	return ctx, func() {
		scope.Close()
	}
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *orchestratorTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Orchestrator Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// cleanup removes all test data.
func (tc *orchestratorTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete in order respecting foreign keys
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_workflow_state WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_workflows WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)
}

// createTestWorkflow creates a workflow record for testing.
func (tc *orchestratorTestContext) createTestWorkflow(ctx context.Context, ontologyID uuid.UUID) *models.OntologyWorkflow {
	tc.t.Helper()

	workflow := &models.OntologyWorkflow{
		ID:         uuid.New(),
		ProjectID:  tc.projectID,
		OntologyID: ontologyID,
		State:      models.WorkflowStateRunning,
		Config: &models.WorkflowConfig{
			DatasourceID: uuid.New(),
		},
	}

	if err := tc.workflowRepo.Create(ctx, workflow); err != nil {
		tc.t.Fatalf("Failed to create workflow: %v", err)
	}

	return workflow
}

// createTestOntology creates an active ontology for testing.
func (tc *orchestratorTestContext) createTestOntology(ctx context.Context) *models.TieredOntology {
	tc.t.Helper()

	ontology := &models.TieredOntology{
		ID:        uuid.New(),
		ProjectID: tc.projectID,
		Version:   1,
		IsActive:  true,
		EntitySummaries: map[string]*models.EntitySummary{
			"test_table": {
				TableName:   "test_table",
				Description: "Test table",
			},
		},
	}

	if err := tc.ontologyRepo.Create(ctx, ontology); err != nil {
		tc.t.Fatalf("Failed to create ontology: %v", err)
	}

	return ontology
}

// createWorkflowStates creates test workflow state rows.
func (tc *orchestratorTestContext) createWorkflowStates(ctx context.Context, workflowID, ontologyID uuid.UUID) []*models.WorkflowEntityState {
	tc.t.Helper()

	states := []*models.WorkflowEntityState{
		{
			ID:         uuid.New(),
			ProjectID:  tc.projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeGlobal,
			EntityKey:  "",
			Status:     models.WorkflowEntityStatusComplete,
		},
		{
			ID:         uuid.New(),
			ProjectID:  tc.projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeTable,
			EntityKey:  "test_table",
			Status:     models.WorkflowEntityStatusComplete,
		},
		{
			ID:         uuid.New(),
			ProjectID:  tc.projectID,
			OntologyID: ontologyID,
			WorkflowID: workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  "test_table.id",
			Status:     models.WorkflowEntityStatusComplete,
		},
	}

	for _, state := range states {
		if err := tc.stateRepo.Create(ctx, state); err != nil {
			tc.t.Fatalf("Failed to create workflow state: %v", err)
		}
	}

	return states
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestOrchestrator_FinalizeWorkflow_PreservesState_Integration(t *testing.T) {
	tc := setupOrchestratorTest(t)
	tc.cleanup()
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test data (order matters: ontology first, then workflow with ontology ID)
	ontology := tc.createTestOntology(ctx)
	workflow := tc.createTestWorkflow(ctx, ontology.ID)
	tc.createWorkflowStates(ctx, workflow.ID, ontology.ID)

	// Verify states exist before finalization
	statesBefore, err := tc.stateRepo.ListByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListByWorkflow failed: %v", err)
	}
	if len(statesBefore) != 3 {
		t.Fatalf("expected 3 workflow states before finalization, got %d", len(statesBefore))
	}

	// Create orchestrator with minimal dependencies
	orch := &Orchestrator{
		stateRepo:    tc.stateRepo,
		workflowRepo: tc.workflowRepo,
		ontologyRepo: tc.ontologyRepo,
		getTenantCtx: tc.getTenantCtx,
		logger:       tc.logger,
	}

	// Call finalizeWorkflow
	err = orch.finalizeWorkflow(context.Background(), tc.projectID, workflow.ID)
	if err != nil {
		t.Fatalf("finalizeWorkflow failed: %v", err)
	}

	// Verify states are PRESERVED after finalization (for assess-deterministic tool)
	// Cleanup happens when a NEW extraction starts, not on finalization.
	statesAfter, err := tc.stateRepo.ListByWorkflow(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ListByWorkflow after finalization failed: %v", err)
	}
	if len(statesAfter) != 3 {
		t.Errorf("expected 3 workflow states preserved after finalization, got %d", len(statesAfter))
	}

	// Verify ontology still exists
	activeOntology, err := tc.ontologyRepo.GetActive(ctx, tc.projectID)
	if err != nil {
		t.Fatalf("GetActive failed: %v", err)
	}
	if activeOntology == nil {
		t.Error("expected active ontology to still exist after finalization")
	}
}
