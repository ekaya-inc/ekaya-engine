//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// workflowStateTestContext holds test dependencies for workflow state repository tests.
type workflowStateTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       WorkflowStateRepository
	projectID  uuid.UUID
	ontologyID uuid.UUID
	workflowID uuid.UUID
}

// setupWorkflowStateTest initializes the test context with shared testcontainer.
func setupWorkflowStateTest(t *testing.T) *workflowStateTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &workflowStateTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewWorkflowStateRepository(),
		projectID:  uuid.MustParse("00000000-0000-0000-0000-000000000081"),
		ontologyID: uuid.MustParse("00000000-0000-0000-0000-000000000082"),
		workflowID: uuid.MustParse("00000000-0000-0000-0000-000000000083"),
	}
	tc.ensureTestFixtures()
	return tc
}

// ensureTestFixtures creates the test project, ontology, and workflow if they don't exist.
func (tc *workflowStateTestContext) ensureTestFixtures() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for fixture setup: %v", err)
	}
	defer scope.Close()

	now := time.Now()

	// Create project
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Workflow State Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}

	// Create ontology
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, is_active, created_at, updated_at)
		VALUES ($1, $2, true, $3, $3)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID, now)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
	}

	// Create workflow
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontology_workflows (id, project_id, ontology_id, state, created_at, updated_at)
		VALUES ($1, $2, $3, 'running', $4, $4)
		ON CONFLICT (id) DO NOTHING
	`, tc.workflowID, tc.projectID, tc.ontologyID, now)
	if err != nil {
		tc.t.Fatalf("failed to ensure test workflow: %v", err)
	}
}

// cleanup removes test workflow states.
func (tc *workflowStateTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_workflow_state WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *workflowStateTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestState creates a workflow state for testing.
func (tc *workflowStateTestContext) createTestState(
	ctx context.Context,
	entityType models.WorkflowEntityType,
	entityKey string,
	status models.WorkflowEntityStatus,
) *models.WorkflowEntityState {
	tc.t.Helper()
	state := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: entityType,
		EntityKey:  entityKey,
		Status:     status,
	}
	err := tc.repo.Create(ctx, state)
	if err != nil {
		tc.t.Fatalf("failed to create test state: %v", err)
	}
	return state
}

// ============================================================================
// Create Tests
// ============================================================================

func TestWorkflowStateRepository_Create_Global(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: models.WorkflowEntityTypeGlobal,
		EntityKey:  "",
		Status:     models.WorkflowEntityStatusPending,
	}

	err := tc.repo.Create(ctx, state)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if state.ID == uuid.Nil {
		t.Error("expected ID to be set")
	}
	if state.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify by fetching
	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.EntityType != models.WorkflowEntityTypeGlobal {
		t.Errorf("expected entity_type 'global', got %q", retrieved.EntityType)
	}
	if retrieved.EntityKey != "" {
		t.Errorf("expected empty entity_key, got %q", retrieved.EntityKey)
	}
}

func TestWorkflowStateRepository_Create_Table(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: models.WorkflowEntityTypeTable,
		EntityKey:  "orders",
		Status:     models.WorkflowEntityStatusPending,
	}

	err := tc.repo.Create(ctx, state)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.EntityType != models.WorkflowEntityTypeTable {
		t.Errorf("expected entity_type 'table', got %q", retrieved.EntityType)
	}
	if retrieved.EntityKey != "orders" {
		t.Errorf("expected entity_key 'orders', got %q", retrieved.EntityKey)
	}
}

func TestWorkflowStateRepository_Create_Column(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: models.WorkflowEntityTypeColumn,
		EntityKey:  "orders.status",
		Status:     models.WorkflowEntityStatusScanned,
		StateData: &models.WorkflowStateData{
			Gathered: map[string]any{
				"distinct_count": 5,
				"sample_values":  []string{"pending", "shipped", "delivered"},
			},
		},
	}

	err := tc.repo.Create(ctx, state)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.EntityType != models.WorkflowEntityTypeColumn {
		t.Errorf("expected entity_type 'column', got %q", retrieved.EntityType)
	}
	if retrieved.EntityKey != "orders.status" {
		t.Errorf("expected entity_key 'orders.status', got %q", retrieved.EntityKey)
	}
	if retrieved.StateData == nil {
		t.Error("expected StateData to be set")
	}
	if retrieved.StateData.Gathered == nil {
		t.Error("expected StateData.Gathered to be set")
	}
	if retrieved.StateData.Gathered["distinct_count"] != float64(5) {
		t.Errorf("expected distinct_count 5, got %v", retrieved.StateData.Gathered["distinct_count"])
	}
}

func TestWorkflowStateRepository_Create_WithError(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	errorMsg := "connection timeout"
	state := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: models.WorkflowEntityTypeTable,
		EntityKey:  "users",
		Status:     models.WorkflowEntityStatusFailed,
		LastError:  &errorMsg,
		RetryCount: 3,
	}

	err := tc.repo.Create(ctx, state)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.LastError == nil || *retrieved.LastError != errorMsg {
		t.Errorf("expected last_error %q, got %v", errorMsg, retrieved.LastError)
	}
	if retrieved.RetryCount != 3 {
		t.Errorf("expected retry_count 3, got %d", retrieved.RetryCount)
	}
}

// ============================================================================
// CreateBatch Tests
// ============================================================================

func TestWorkflowStateRepository_CreateBatch_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	states := []*models.WorkflowEntityState{
		{
			ProjectID:  tc.projectID,
			OntologyID: tc.ontologyID,
			WorkflowID: tc.workflowID,
			EntityType: models.WorkflowEntityTypeGlobal,
			EntityKey:  "",
		},
		{
			ProjectID:  tc.projectID,
			OntologyID: tc.ontologyID,
			WorkflowID: tc.workflowID,
			EntityType: models.WorkflowEntityTypeTable,
			EntityKey:  "orders",
		},
		{
			ProjectID:  tc.projectID,
			OntologyID: tc.ontologyID,
			WorkflowID: tc.workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  "orders.id",
		},
		{
			ProjectID:  tc.projectID,
			OntologyID: tc.ontologyID,
			WorkflowID: tc.workflowID,
			EntityType: models.WorkflowEntityTypeColumn,
			EntityKey:  "orders.status",
		},
	}

	err := tc.repo.CreateBatch(ctx, states)
	if err != nil {
		t.Fatalf("CreateBatch failed: %v", err)
	}

	// All should have IDs set
	for i, s := range states {
		if s.ID == uuid.Nil {
			t.Errorf("state %d: expected ID to be set", i)
		}
	}

	// Verify count
	all, err := tc.repo.ListByWorkflow(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("ListByWorkflow failed: %v", err)
	}
	if len(all) != 4 {
		t.Errorf("expected 4 states, got %d", len(all))
	}
}

func TestWorkflowStateRepository_CreateBatch_Empty(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.CreateBatch(ctx, []*models.WorkflowEntityState{})
	if err != nil {
		t.Fatalf("CreateBatch with empty slice should not error: %v", err)
	}
}

// ============================================================================
// GetByID Tests
// ============================================================================

func TestWorkflowStateRepository_GetByID_NotFound(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state, err := tc.repo.GetByID(ctx, uuid.New())
	if err != nil {
		t.Fatalf("GetByID should not error for non-existent: %v", err)
	}
	if state != nil {
		t.Error("expected nil for non-existent state")
	}
}

// ============================================================================
// GetByEntity Tests
// ============================================================================

func TestWorkflowStateRepository_GetByEntity_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	created := tc.createTestState(ctx, models.WorkflowEntityTypeColumn, "orders.status", models.WorkflowEntityStatusScanned)

	retrieved, err := tc.repo.GetByEntity(ctx, tc.workflowID, models.WorkflowEntityTypeColumn, "orders.status")
	if err != nil {
		t.Fatalf("GetByEntity failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected state, got nil")
	}
	if retrieved.ID != created.ID {
		t.Errorf("expected ID %v, got %v", created.ID, retrieved.ID)
	}
}

func TestWorkflowStateRepository_GetByEntity_NotFound(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state, err := tc.repo.GetByEntity(ctx, tc.workflowID, models.WorkflowEntityTypeColumn, "nonexistent.column")
	if err != nil {
		t.Fatalf("GetByEntity should not error for non-existent: %v", err)
	}
	if state != nil {
		t.Error("expected nil for non-existent entity")
	}
}

// ============================================================================
// ListByWorkflow Tests
// ============================================================================

func TestWorkflowStateRepository_ListByWorkflow_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestState(ctx, models.WorkflowEntityTypeGlobal, "", models.WorkflowEntityStatusComplete)
	tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusScanned)
	tc.createTestState(ctx, models.WorkflowEntityTypeColumn, "orders.id", models.WorkflowEntityStatusPending)

	states, err := tc.repo.ListByWorkflow(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("ListByWorkflow failed: %v", err)
	}
	if len(states) != 3 {
		t.Errorf("expected 3 states, got %d", len(states))
	}

	// Should be ordered by entity_type, entity_key
	if states[0].EntityType != models.WorkflowEntityTypeColumn {
		t.Errorf("expected first to be column (alphabetically), got %q", states[0].EntityType)
	}
}

func TestWorkflowStateRepository_ListByWorkflow_Empty(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	states, err := tc.repo.ListByWorkflow(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("ListByWorkflow failed: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected 0 states, got %d", len(states))
	}
}

// ============================================================================
// ListByStatus Tests
// ============================================================================

func TestWorkflowStateRepository_ListByStatus_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestState(ctx, models.WorkflowEntityTypeGlobal, "", models.WorkflowEntityStatusComplete)
	tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusPending)
	tc.createTestState(ctx, models.WorkflowEntityTypeTable, "users", models.WorkflowEntityStatusPending)
	tc.createTestState(ctx, models.WorkflowEntityTypeColumn, "orders.id", models.WorkflowEntityStatusScanned)

	pending, err := tc.repo.ListByStatus(ctx, tc.workflowID, models.WorkflowEntityStatusPending)
	if err != nil {
		t.Fatalf("ListByStatus failed: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("expected 2 pending states, got %d", len(pending))
	}

	complete, err := tc.repo.ListByStatus(ctx, tc.workflowID, models.WorkflowEntityStatusComplete)
	if err != nil {
		t.Fatalf("ListByStatus failed: %v", err)
	}
	if len(complete) != 1 {
		t.Errorf("expected 1 complete state, got %d", len(complete))
	}
}

// ============================================================================
// UpdateStatus Tests
// ============================================================================

func TestWorkflowStateRepository_UpdateStatus_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusPending)

	err := tc.repo.UpdateStatus(ctx, state.ID, models.WorkflowEntityStatusScanning, nil)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Status != models.WorkflowEntityStatusScanning {
		t.Errorf("expected status 'scanning', got %q", retrieved.Status)
	}
}

func TestWorkflowStateRepository_UpdateStatus_WithError(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusAnalyzing)

	errorMsg := "LLM rate limit exceeded"
	err := tc.repo.UpdateStatus(ctx, state.ID, models.WorkflowEntityStatusFailed, &errorMsg)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.Status != models.WorkflowEntityStatusFailed {
		t.Errorf("expected status 'failed', got %q", retrieved.Status)
	}
	if retrieved.LastError == nil || *retrieved.LastError != errorMsg {
		t.Errorf("expected last_error %q, got %v", errorMsg, retrieved.LastError)
	}
}

func TestWorkflowStateRepository_UpdateStatus_NotFound(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.UpdateStatus(ctx, uuid.New(), models.WorkflowEntityStatusComplete, nil)
	if err == nil {
		t.Error("expected error for non-existent state")
	}
}

// ============================================================================
// UpdateStateData Tests
// ============================================================================

func TestWorkflowStateRepository_UpdateStateData_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeColumn, "orders.status", models.WorkflowEntityStatusScanned)

	newStateData := &models.WorkflowStateData{
		Gathered: map[string]any{
			"distinct_count": 5,
			"sample_values":  []string{"pending", "shipped", "delivered", "cancelled", "refunded"},
		},
		LLMAnalysis: map[string]any{
			"thinking":      "This appears to be an order lifecycle status column",
			"semantic_type": "order_lifecycle",
		},
	}

	err := tc.repo.UpdateStateData(ctx, state.ID, newStateData)
	if err != nil {
		t.Fatalf("UpdateStateData failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.StateData == nil {
		t.Fatal("expected StateData to be set")
	}
	if retrieved.StateData.LLMAnalysis == nil {
		t.Fatal("expected LLMAnalysis to be set")
	}
	if retrieved.StateData.LLMAnalysis["semantic_type"] != "order_lifecycle" {
		t.Errorf("expected semantic_type 'order_lifecycle', got %v", retrieved.StateData.LLMAnalysis["semantic_type"])
	}
}

func TestWorkflowStateRepository_UpdateStateData_NotFound(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.UpdateStateData(ctx, uuid.New(), &models.WorkflowStateData{})
	if err == nil {
		t.Error("expected error for non-existent state")
	}
}

// ============================================================================
// IncrementRetryCount Tests
// ============================================================================

func TestWorkflowStateRepository_IncrementRetryCount_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusFailed)

	err := tc.repo.IncrementRetryCount(ctx, state.ID)
	if err != nil {
		t.Fatalf("IncrementRetryCount failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.RetryCount != 1 {
		t.Errorf("expected retry_count 1, got %d", retrieved.RetryCount)
	}

	// Increment again
	err = tc.repo.IncrementRetryCount(ctx, state.ID)
	if err != nil {
		t.Fatalf("IncrementRetryCount failed: %v", err)
	}

	retrieved, err = tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.RetryCount != 2 {
		t.Errorf("expected retry_count 2, got %d", retrieved.RetryCount)
	}
}

// ============================================================================
// Delete Tests
// ============================================================================

func TestWorkflowStateRepository_Delete_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusComplete)

	err := tc.repo.Delete(ctx, state.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil after delete")
	}
}

func TestWorkflowStateRepository_Delete_NotFound(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	err := tc.repo.Delete(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for non-existent state")
	}
}

// ============================================================================
// DeleteByWorkflow Tests
// ============================================================================

func TestWorkflowStateRepository_DeleteByWorkflow_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	tc.createTestState(ctx, models.WorkflowEntityTypeGlobal, "", models.WorkflowEntityStatusComplete)
	tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusComplete)
	tc.createTestState(ctx, models.WorkflowEntityTypeColumn, "orders.id", models.WorkflowEntityStatusComplete)

	// Verify 3 exist
	states, err := tc.repo.ListByWorkflow(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("ListByWorkflow failed: %v", err)
	}
	if len(states) != 3 {
		t.Fatalf("expected 3 states before delete, got %d", len(states))
	}

	err = tc.repo.DeleteByWorkflow(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("DeleteByWorkflow failed: %v", err)
	}

	// Verify all deleted
	states, err = tc.repo.ListByWorkflow(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("ListByWorkflow failed: %v", err)
	}
	if len(states) != 0 {
		t.Errorf("expected 0 states after delete, got %d", len(states))
	}
}

// ============================================================================
// Unique Constraint Tests
// ============================================================================

func TestWorkflowStateRepository_UniqueConstraint(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create first state
	tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusPending)

	// Try to create duplicate
	duplicate := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: models.WorkflowEntityTypeTable,
		EntityKey:  "orders",
		Status:     models.WorkflowEntityStatusScanned,
	}

	err := tc.repo.Create(ctx, duplicate)
	if err == nil {
		t.Error("expected error for duplicate entity key")
	}
}

// ============================================================================
// No Tenant Scope Tests (RLS Enforcement)
// ============================================================================

func TestWorkflowStateRepository_NoTenantScope(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx := context.Background() // No tenant scope

	state := &models.WorkflowEntityState{
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		WorkflowID: tc.workflowID,
		EntityType: models.WorkflowEntityTypeGlobal,
		EntityKey:  "",
		Status:     models.WorkflowEntityStatusPending,
	}

	// Create should fail
	err := tc.repo.Create(ctx, state)
	if err == nil {
		t.Error("expected error for Create without tenant scope")
	}

	// CreateBatch should fail
	err = tc.repo.CreateBatch(ctx, []*models.WorkflowEntityState{state})
	if err == nil {
		t.Error("expected error for CreateBatch without tenant scope")
	}

	// GetByID should fail
	_, err = tc.repo.GetByID(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for GetByID without tenant scope")
	}

	// GetByEntity should fail
	_, err = tc.repo.GetByEntity(ctx, tc.workflowID, models.WorkflowEntityTypeGlobal, "")
	if err == nil {
		t.Error("expected error for GetByEntity without tenant scope")
	}

	// ListByWorkflow should fail
	_, err = tc.repo.ListByWorkflow(ctx, tc.workflowID)
	if err == nil {
		t.Error("expected error for ListByWorkflow without tenant scope")
	}

	// ListByStatus should fail
	_, err = tc.repo.ListByStatus(ctx, tc.workflowID, models.WorkflowEntityStatusPending)
	if err == nil {
		t.Error("expected error for ListByStatus without tenant scope")
	}

	// UpdateStatus should fail
	err = tc.repo.UpdateStatus(ctx, uuid.New(), models.WorkflowEntityStatusComplete, nil)
	if err == nil {
		t.Error("expected error for UpdateStatus without tenant scope")
	}

	// UpdateStateData should fail
	err = tc.repo.UpdateStateData(ctx, uuid.New(), &models.WorkflowStateData{})
	if err == nil {
		t.Error("expected error for UpdateStateData without tenant scope")
	}

	// IncrementRetryCount should fail
	err = tc.repo.IncrementRetryCount(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for IncrementRetryCount without tenant scope")
	}

	// Delete should fail
	err = tc.repo.Delete(ctx, uuid.New())
	if err == nil {
		t.Error("expected error for Delete without tenant scope")
	}

	// DeleteByWorkflow should fail
	err = tc.repo.DeleteByWorkflow(ctx, tc.workflowID)
	if err == nil {
		t.Error("expected error for DeleteByWorkflow without tenant scope")
	}
}

// ============================================================================
// Model Helper Method Tests
// ============================================================================

func TestWorkflowState_IsTerminal(t *testing.T) {
	tests := []struct {
		status   models.WorkflowEntityStatus
		expected bool
	}{
		{models.WorkflowEntityStatusPending, false},
		{models.WorkflowEntityStatusScanning, false},
		{models.WorkflowEntityStatusScanned, false},
		{models.WorkflowEntityStatusAnalyzing, false},
		{models.WorkflowEntityStatusNeedsInput, false},
		{models.WorkflowEntityStatusComplete, true},
		{models.WorkflowEntityStatusFailed, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			if got := tt.status.IsTerminal(); got != tt.expected {
				t.Errorf("IsTerminal() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestWorkflowEntityState_TableName(t *testing.T) {
	tests := []struct {
		entityType models.WorkflowEntityType
		entityKey  string
		expected   string
	}{
		{models.WorkflowEntityTypeGlobal, "", ""},
		{models.WorkflowEntityTypeTable, "orders", "orders"},
		{models.WorkflowEntityTypeColumn, "orders.status", "orders"},
		{models.WorkflowEntityTypeColumn, "users.email", "users"},
	}

	for _, tt := range tests {
		t.Run(string(tt.entityType)+":"+tt.entityKey, func(t *testing.T) {
			ws := &models.WorkflowEntityState{EntityType: tt.entityType, EntityKey: tt.entityKey}
			if got := ws.TableName(); got != tt.expected {
				t.Errorf("TableName() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestWorkflowEntityState_ColumnName(t *testing.T) {
	tests := []struct {
		entityType models.WorkflowEntityType
		entityKey  string
		expected   string
	}{
		{models.WorkflowEntityTypeGlobal, "", ""},
		{models.WorkflowEntityTypeTable, "orders", ""},
		{models.WorkflowEntityTypeColumn, "orders.status", "status"},
		{models.WorkflowEntityTypeColumn, "users.email", "email"},
	}

	for _, tt := range tests {
		t.Run(string(tt.entityType)+":"+tt.entityKey, func(t *testing.T) {
			ws := &models.WorkflowEntityState{EntityType: tt.entityType, EntityKey: tt.entityKey}
			if got := ws.ColumnName(); got != tt.expected {
				t.Errorf("ColumnName() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestEntityKeyHelpers(t *testing.T) {
	if got := models.GlobalEntityKey(); got != "" {
		t.Errorf("GlobalEntityKey() = %q, expected empty", got)
	}

	if got := models.TableEntityKey("orders"); got != "orders" {
		t.Errorf("TableEntityKey() = %q, expected 'orders'", got)
	}

	if got := models.ColumnEntityKey("orders", "status"); got != "orders.status" {
		t.Errorf("ColumnEntityKey() = %q, expected 'orders.status'", got)
	}

	table, col := models.ParseColumnEntityKey("orders.status")
	if table != "orders" || col != "status" {
		t.Errorf("ParseColumnEntityKey() = (%q, %q), expected ('orders', 'status')", table, col)
	}

	table, col = models.ParseColumnEntityKey("invalid")
	if table != "" || col != "" {
		t.Errorf("ParseColumnEntityKey() for invalid = (%q, %q), expected empty", table, col)
	}
}

// ============================================================================
// Question Operations Tests
// ============================================================================

func TestWorkflowStateRepository_AddQuestionsToEntity_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a table entity state
	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)

	// Add questions
	questions := []models.WorkflowQuestion{
		{
			ID:         uuid.New().String(),
			Text:       "What does 'pending' status mean?",
			Priority:   1,
			IsRequired: true,
			Category:   "business_rules",
			Status:     "pending",
		},
		{
			ID:         uuid.New().String(),
			Text:       "Are there any other statuses?",
			Priority:   3,
			IsRequired: false,
			Category:   "enumeration",
			Status:     "pending",
		},
	}

	err := tc.repo.AddQuestionsToEntity(ctx, state.ID, questions)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	// Verify questions were added
	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.StateData == nil {
		t.Fatal("expected StateData to be set")
	}
	if len(retrieved.StateData.Questions) != 2 {
		t.Errorf("expected 2 questions, got %d", len(retrieved.StateData.Questions))
	}
	if retrieved.StateData.Questions[0].Text != "What does 'pending' status mean?" {
		t.Errorf("unexpected question text: %s", retrieved.StateData.Questions[0].Text)
	}
}

func TestWorkflowStateRepository_AddQuestionsToEntity_Append(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)

	// Add first question
	q1 := []models.WorkflowQuestion{{
		ID:         uuid.New().String(),
		Text:       "First question",
		Priority:   1,
		IsRequired: true,
		Status:     "pending",
	}}
	err := tc.repo.AddQuestionsToEntity(ctx, state.ID, q1)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	// Add second question - should append
	q2 := []models.WorkflowQuestion{{
		ID:         uuid.New().String(),
		Text:       "Second question",
		Priority:   2,
		IsRequired: false,
		Status:     "pending",
	}}
	err = tc.repo.AddQuestionsToEntity(ctx, state.ID, q2)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	// Verify both questions exist
	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if len(retrieved.StateData.Questions) != 2 {
		t.Errorf("expected 2 questions after append, got %d", len(retrieved.StateData.Questions))
	}
}

func TestWorkflowStateRepository_AddQuestionsToEntity_Empty(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)

	// Adding empty slice should not error
	err := tc.repo.AddQuestionsToEntity(ctx, state.ID, []models.WorkflowQuestion{})
	if err != nil {
		t.Fatalf("AddQuestionsToEntity with empty slice should not error: %v", err)
	}
}

func TestWorkflowStateRepository_UpdateQuestionInEntity_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)

	questionID := uuid.New().String()
	questions := []models.WorkflowQuestion{{
		ID:         questionID,
		Text:       "What does 'pending' status mean?",
		Priority:   1,
		IsRequired: true,
		Status:     "pending",
	}}
	err := tc.repo.AddQuestionsToEntity(ctx, state.ID, questions)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	// Update question status and answer
	err = tc.repo.UpdateQuestionInEntity(ctx, state.ID, questionID, "answered", "Pending means order received but not processed")
	if err != nil {
		t.Fatalf("UpdateQuestionInEntity failed: %v", err)
	}

	// Verify update
	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	q := retrieved.StateData.Questions[0]
	if q.Status != "answered" {
		t.Errorf("expected status 'answered', got %q", q.Status)
	}
	if q.Answer != "Pending means order received but not processed" {
		t.Errorf("unexpected answer: %s", q.Answer)
	}
	if q.AnsweredAt == nil {
		t.Error("expected AnsweredAt to be set")
	}
}

func TestWorkflowStateRepository_UpdateQuestionInEntity_NotFound(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)

	// Add a question
	questions := []models.WorkflowQuestion{{
		ID:         uuid.New().String(),
		Text:       "A question",
		Priority:   1,
		IsRequired: true,
		Status:     "pending",
	}}
	err := tc.repo.AddQuestionsToEntity(ctx, state.ID, questions)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	// Try to update non-existent question
	err = tc.repo.UpdateQuestionInEntity(ctx, state.ID, uuid.New().String(), "answered", "Some answer")
	if err == nil {
		t.Error("expected error for non-existent question")
	}
}

func TestWorkflowStateRepository_RecordAnswerInEntity_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)

	answer := models.WorkflowAnswer{
		QuestionID:     uuid.New().String(),
		Answer:         "Pending means the order was received but not yet processed",
		AnsweredBy:     uuid.New().String(),
		AnsweredAt:     time.Now(),
		EntityUpdates:  []string{"Updated orders entity description"},
		KnowledgeFacts: []string{uuid.New().String()},
	}

	err := tc.repo.RecordAnswerInEntity(ctx, state.ID, answer)
	if err != nil {
		t.Fatalf("RecordAnswerInEntity failed: %v", err)
	}

	// Verify answer was recorded
	retrieved, err := tc.repo.GetByID(ctx, state.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved.StateData == nil {
		t.Fatal("expected StateData to be set")
	}
	if len(retrieved.StateData.Answers) != 1 {
		t.Errorf("expected 1 answer, got %d", len(retrieved.StateData.Answers))
	}
	if retrieved.StateData.Answers[0].Answer != answer.Answer {
		t.Errorf("unexpected answer text: %s", retrieved.StateData.Answers[0].Answer)
	}
}

func TestWorkflowStateRepository_GetNextPendingQuestion_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create two entities with questions
	state1 := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)
	state2 := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "users", models.WorkflowEntityStatusNeedsInput)

	// Add questions with different priorities
	q1ID := uuid.New().String()
	q1 := []models.WorkflowQuestion{{
		ID:         q1ID,
		Text:       "Low priority question",
		Priority:   5,
		IsRequired: false,
		Status:     "pending",
	}}
	err := tc.repo.AddQuestionsToEntity(ctx, state1.ID, q1)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	q2ID := uuid.New().String()
	q2 := []models.WorkflowQuestion{{
		ID:         q2ID,
		Text:       "High priority required question",
		Priority:   1,
		IsRequired: true,
		Status:     "pending",
	}}
	err = tc.repo.AddQuestionsToEntity(ctx, state2.ID, q2)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	// Get next pending - should return highest priority (priority 1) required question
	question, entityID, err := tc.repo.GetNextPendingQuestion(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("GetNextPendingQuestion failed: %v", err)
	}
	if question == nil {
		t.Fatal("expected a question, got nil")
	}
	if question.ID != q2ID {
		t.Errorf("expected question %s, got %s", q2ID, question.ID)
	}
	if entityID != state2.ID {
		t.Errorf("expected entity %s, got %s", state2.ID, entityID)
	}
}

func TestWorkflowStateRepository_GetNextPendingQuestion_NoQuestions(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create entity without questions
	tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusComplete)

	question, entityID, err := tc.repo.GetNextPendingQuestion(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("GetNextPendingQuestion failed: %v", err)
	}
	if question != nil {
		t.Error("expected nil question when no pending questions exist")
	}
	if entityID != uuid.Nil {
		t.Error("expected nil entityID when no pending questions exist")
	}
}

func TestWorkflowStateRepository_GetNextPendingQuestion_SkipsAnswered(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)

	answeredID := uuid.New().String()
	pendingID := uuid.New().String()
	questions := []models.WorkflowQuestion{
		{
			ID:         answeredID,
			Text:       "Already answered",
			Priority:   1,
			IsRequired: true,
			Status:     "answered",
		},
		{
			ID:         pendingID,
			Text:       "Still pending",
			Priority:   2,
			IsRequired: true,
			Status:     "pending",
		},
	}
	err := tc.repo.AddQuestionsToEntity(ctx, state.ID, questions)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	question, _, err := tc.repo.GetNextPendingQuestion(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("GetNextPendingQuestion failed: %v", err)
	}
	if question == nil {
		t.Fatal("expected a question")
	}
	if question.ID != pendingID {
		t.Errorf("expected pending question %s, got %s", pendingID, question.ID)
	}
}

func TestWorkflowStateRepository_GetPendingQuestionsCount_Success(t *testing.T) {
	tc := setupWorkflowStateTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	state := tc.createTestState(ctx, models.WorkflowEntityTypeTable, "orders", models.WorkflowEntityStatusNeedsInput)

	questions := []models.WorkflowQuestion{
		{
			ID:         uuid.New().String(),
			Text:       "Required pending 1",
			Priority:   1,
			IsRequired: true,
			Status:     "pending",
		},
		{
			ID:         uuid.New().String(),
			Text:       "Required pending 2",
			Priority:   2,
			IsRequired: true,
			Status:     "pending",
		},
		{
			ID:         uuid.New().String(),
			Text:       "Optional pending",
			Priority:   3,
			IsRequired: false,
			Status:     "pending",
		},
		{
			ID:         uuid.New().String(),
			Text:       "Already answered",
			Priority:   1,
			IsRequired: true,
			Status:     "answered",
		},
	}
	err := tc.repo.AddQuestionsToEntity(ctx, state.ID, questions)
	if err != nil {
		t.Fatalf("AddQuestionsToEntity failed: %v", err)
	}

	required, optional, err := tc.repo.GetPendingQuestionsCount(ctx, tc.workflowID)
	if err != nil {
		t.Fatalf("GetPendingQuestionsCount failed: %v", err)
	}
	if required != 2 {
		t.Errorf("expected 2 required pending, got %d", required)
	}
	if optional != 1 {
		t.Errorf("expected 1 optional pending, got %d", optional)
	}
}
