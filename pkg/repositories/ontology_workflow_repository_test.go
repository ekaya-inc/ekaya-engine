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

// workflowTestContext holds test dependencies for workflow repository tests.
type workflowTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	repo       OntologyWorkflowRepository
	projectID  uuid.UUID
	ontologyID uuid.UUID
}

// setupWorkflowTest initializes the test context with shared testcontainer.
// Each test gets a unique project ID to avoid unique constraint conflicts.
func setupWorkflowTest(t *testing.T) *workflowTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &workflowTestContext{
		t:          t,
		engineDB:   engineDB,
		repo:       NewOntologyWorkflowRepository(),
		projectID:  uuid.New(), // Unique per test to avoid constraint conflicts
		ontologyID: uuid.New(),
	}
	tc.ensureTestProject()
	tc.ensureTestOntology()
	return tc
}

// ensureTestProject creates the test project if it doesn't exist.
func (tc *workflowTestContext) ensureTestProject() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Workflow Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}
}

// ensureTestOntology creates the test ontology if it doesn't exist.
func (tc *workflowTestContext) ensureTestOntology() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for ontology setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_ontologies (id, project_id, version, is_active)
		VALUES ($1, $2, 1, true)
		ON CONFLICT (id) DO NOTHING
	`, tc.ontologyID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test ontology: %v", err)
	}
}

// cleanup removes test workflows.
func (tc *workflowTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_workflows WHERE project_id = $1", tc.projectID)
}

// createTestContext returns a context with tenant scope.
func (tc *workflowTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

// createTestWorkflow creates a workflow for testing.
func (tc *workflowTestContext) createTestWorkflow(ctx context.Context) *models.OntologyWorkflow {
	tc.t.Helper()
	now := time.Now()
	workflow := &models.OntologyWorkflow{
		ID:         uuid.New(),
		ProjectID:  tc.projectID,
		OntologyID: tc.ontologyID,
		State:      models.WorkflowStatePending,
		Progress: &models.WorkflowProgress{
			CurrentPhase: models.WorkflowPhaseInitializing,
			Current:      0,
			Total:        10,
		},
		Config:    models.DefaultWorkflowConfig(),
		StartedAt: &now,
	}
	err := tc.repo.Create(ctx, workflow)
	if err != nil {
		tc.t.Fatalf("failed to create test workflow: %v", err)
	}
	return workflow
}

// ============================================================================
// Ownership Tests
// ============================================================================

func TestWorkflowRepository_ClaimOwnership_Unowned(t *testing.T) {
	tc := setupWorkflowTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a workflow without owner
	workflow := tc.createTestWorkflow(ctx)
	serverID := uuid.New()

	// Claim ownership
	claimed, err := tc.repo.ClaimOwnership(ctx, workflow.ID, serverID)
	if err != nil {
		t.Fatalf("ClaimOwnership failed: %v", err)
	}
	if !claimed {
		t.Fatal("expected to claim unowned workflow")
	}

	// Verify ownership was set
	updated, err := tc.repo.GetByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.OwnerID == nil || *updated.OwnerID != serverID {
		t.Errorf("expected owner_id to be %s, got %v", serverID, updated.OwnerID)
	}
	if updated.LastHeartbeat == nil {
		t.Error("expected last_heartbeat to be set")
	}
}

func TestWorkflowRepository_ClaimOwnership_AlreadyOwnedBySelf(t *testing.T) {
	tc := setupWorkflowTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a workflow and claim it
	workflow := tc.createTestWorkflow(ctx)
	serverID := uuid.New()

	claimed, err := tc.repo.ClaimOwnership(ctx, workflow.ID, serverID)
	if err != nil {
		t.Fatalf("First ClaimOwnership failed: %v", err)
	}
	if !claimed {
		t.Fatal("expected to claim unowned workflow")
	}

	// Claim again by same server (should succeed - idempotent)
	claimed2, err := tc.repo.ClaimOwnership(ctx, workflow.ID, serverID)
	if err != nil {
		t.Fatalf("Second ClaimOwnership failed: %v", err)
	}
	if !claimed2 {
		t.Fatal("expected to re-claim own workflow")
	}
}

func TestWorkflowRepository_ClaimOwnership_OwnedByOther(t *testing.T) {
	tc := setupWorkflowTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a workflow and claim it by server 1
	workflow := tc.createTestWorkflow(ctx)
	server1ID := uuid.New()
	server2ID := uuid.New()

	claimed, err := tc.repo.ClaimOwnership(ctx, workflow.ID, server1ID)
	if err != nil {
		t.Fatalf("First ClaimOwnership failed: %v", err)
	}
	if !claimed {
		t.Fatal("expected to claim unowned workflow")
	}

	// Try to claim by server 2 (should fail)
	claimed2, err := tc.repo.ClaimOwnership(ctx, workflow.ID, server2ID)
	if err != nil {
		t.Fatalf("Second ClaimOwnership should not return error: %v", err)
	}
	if claimed2 {
		t.Fatal("expected NOT to claim workflow owned by another server")
	}

	// Verify owner is still server 1
	updated, err := tc.repo.GetByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.OwnerID == nil || *updated.OwnerID != server1ID {
		t.Errorf("expected owner to still be %s, got %v", server1ID, updated.OwnerID)
	}
}

func TestWorkflowRepository_UpdateHeartbeat(t *testing.T) {
	tc := setupWorkflowTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a workflow and claim it
	workflow := tc.createTestWorkflow(ctx)
	serverID := uuid.New()

	claimed, err := tc.repo.ClaimOwnership(ctx, workflow.ID, serverID)
	if err != nil || !claimed {
		t.Fatalf("ClaimOwnership failed: %v, claimed: %v", err, claimed)
	}

	// Get initial heartbeat
	initial, _ := tc.repo.GetByID(ctx, workflow.ID)
	initialHeartbeat := initial.LastHeartbeat

	// Wait a bit then update heartbeat
	time.Sleep(10 * time.Millisecond)

	err = tc.repo.UpdateHeartbeat(ctx, workflow.ID, serverID)
	if err != nil {
		t.Fatalf("UpdateHeartbeat failed: %v", err)
	}

	// Verify heartbeat was updated
	updated, err := tc.repo.GetByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.LastHeartbeat == nil {
		t.Fatal("expected last_heartbeat to be set")
	}
	if !updated.LastHeartbeat.After(*initialHeartbeat) {
		t.Errorf("expected heartbeat to be updated, initial: %v, updated: %v",
			initialHeartbeat, updated.LastHeartbeat)
	}
}

func TestWorkflowRepository_UpdateHeartbeat_WrongOwner(t *testing.T) {
	tc := setupWorkflowTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a workflow and claim it by server 1
	workflow := tc.createTestWorkflow(ctx)
	server1ID := uuid.New()
	server2ID := uuid.New()

	claimed, err := tc.repo.ClaimOwnership(ctx, workflow.ID, server1ID)
	if err != nil || !claimed {
		t.Fatalf("ClaimOwnership failed: %v, claimed: %v", err, claimed)
	}

	// Try to update heartbeat as server 2 (should fail)
	err = tc.repo.UpdateHeartbeat(ctx, workflow.ID, server2ID)
	if err == nil {
		t.Fatal("expected UpdateHeartbeat to fail for wrong owner")
	}
}

func TestWorkflowRepository_ReleaseOwnership(t *testing.T) {
	tc := setupWorkflowTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a workflow and claim it
	workflow := tc.createTestWorkflow(ctx)
	serverID := uuid.New()

	claimed, err := tc.repo.ClaimOwnership(ctx, workflow.ID, serverID)
	if err != nil || !claimed {
		t.Fatalf("ClaimOwnership failed: %v, claimed: %v", err, claimed)
	}

	// Release ownership
	err = tc.repo.ReleaseOwnership(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ReleaseOwnership failed: %v", err)
	}

	// Verify ownership was cleared
	updated, err := tc.repo.GetByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.OwnerID != nil {
		t.Errorf("expected owner_id to be nil, got %v", updated.OwnerID)
	}
	if updated.LastHeartbeat != nil {
		t.Errorf("expected last_heartbeat to be nil, got %v", updated.LastHeartbeat)
	}
}

func TestWorkflowRepository_ReleaseOwnership_AllowsReclaim(t *testing.T) {
	tc := setupWorkflowTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a workflow and claim it by server 1
	workflow := tc.createTestWorkflow(ctx)
	server1ID := uuid.New()
	server2ID := uuid.New()

	claimed, err := tc.repo.ClaimOwnership(ctx, workflow.ID, server1ID)
	if err != nil || !claimed {
		t.Fatalf("First ClaimOwnership failed: %v, claimed: %v", err, claimed)
	}

	// Release ownership
	err = tc.repo.ReleaseOwnership(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("ReleaseOwnership failed: %v", err)
	}

	// Server 2 should now be able to claim
	claimed2, err := tc.repo.ClaimOwnership(ctx, workflow.ID, server2ID)
	if err != nil {
		t.Fatalf("Second ClaimOwnership failed: %v", err)
	}
	if !claimed2 {
		t.Fatal("expected server 2 to claim released workflow")
	}

	// Verify owner is now server 2
	updated, err := tc.repo.GetByID(ctx, workflow.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if updated.OwnerID == nil || *updated.OwnerID != server2ID {
		t.Errorf("expected owner to be %s, got %v", server2ID, updated.OwnerID)
	}
}
