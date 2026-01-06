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

// dagTestContext holds test dependencies for DAG repository tests.
type dagTestContext struct {
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	repo         OntologyDAGRepository
	projectID    uuid.UUID
	datasourceID uuid.UUID
	ontologyID   uuid.UUID
}

// setupDAGTest initializes the test context with shared testcontainer.
func setupDAGTest(t *testing.T) *dagTestContext {
	engineDB := testhelpers.GetEngineDB(t)
	tc := &dagTestContext{
		t:            t,
		engineDB:     engineDB,
		repo:         NewOntologyDAGRepository(),
		projectID:    uuid.New(),
		datasourceID: uuid.New(),
		ontologyID:   uuid.New(),
	}
	tc.ensureTestProject()
	tc.ensureTestDatasource()
	tc.ensureTestOntology()
	return tc
}

func (tc *dagTestContext) ensureTestProject() {
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
	`, tc.projectID, "DAG Test Project")
	if err != nil {
		tc.t.Fatalf("failed to ensure test project: %v", err)
	}
}

func (tc *dagTestContext) ensureTestDatasource() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for datasource setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, 'Test Datasource', 'postgres', 'encrypted_config')
		ON CONFLICT (id) DO NOTHING
	`, tc.datasourceID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("failed to ensure test datasource: %v", err)
	}
}

func (tc *dagTestContext) ensureTestOntology() {
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

func (tc *dagTestContext) cleanup() {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create scope for cleanup: %v", err)
	}
	defer scope.Close()

	_, _ = scope.Conn.Exec(ctx, "DELETE FROM engine_ontology_dag WHERE project_id = $1", tc.projectID)
}

func (tc *dagTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()
	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("failed to create tenant scope: %v", err)
	}
	ctx = database.SetTenantScope(ctx, scope)
	return ctx, func() { scope.Close() }
}

func (tc *dagTestContext) createTestDAG(ctx context.Context) *models.OntologyDAG {
	tc.t.Helper()
	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		OntologyID:   &tc.ontologyID,
		Status:       models.DAGStatusPending,
	}
	err := tc.repo.Create(ctx, dag)
	if err != nil {
		tc.t.Fatalf("failed to create test DAG: %v", err)
	}
	return dag
}

// ============================================================================
// DAG CRUD Tests
// ============================================================================

func TestDAGRepository_Create(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := &models.OntologyDAG{
		ID:                uuid.New(),
		ProjectID:         tc.projectID,
		DatasourceID:      tc.datasourceID,
		OntologyID:        &tc.ontologyID,
		Status:            models.DAGStatusPending,
		SchemaFingerprint: strPtr("abc123"),
	}

	err := tc.repo.Create(ctx, dag)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Verify created
	retrieved, err := tc.repo.GetByID(ctx, dag.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("expected to find DAG")
	}
	if retrieved.Status != models.DAGStatusPending {
		t.Errorf("expected status pending, got %s", retrieved.Status)
	}
	if *retrieved.SchemaFingerprint != "abc123" {
		t.Errorf("expected fingerprint abc123, got %s", *retrieved.SchemaFingerprint)
	}
}

func TestDAGRepository_GetLatestByDatasource(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create first DAG
	dag1 := tc.createTestDAG(ctx)

	// Mark dag1 as completed to avoid unique constraint violation
	// (only one active DAG per datasource is allowed)
	err := tc.repo.UpdateStatus(ctx, dag1.ID, models.DAGStatusCompleted, nil)
	if err != nil {
		t.Fatalf("Failed to complete first DAG: %v", err)
	}

	time.Sleep(10 * time.Millisecond)

	// Create second DAG
	dag2 := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		OntologyID:   &tc.ontologyID,
		Status:       models.DAGStatusRunning,
	}
	err = tc.repo.Create(ctx, dag2)
	if err != nil {
		t.Fatalf("Create second DAG failed: %v", err)
	}

	// Get latest
	latest, err := tc.repo.GetLatestByDatasource(ctx, tc.datasourceID)
	if err != nil {
		t.Fatalf("GetLatestByDatasource failed: %v", err)
	}
	if latest == nil {
		t.Fatal("expected to find latest DAG")
	}
	if latest.ID != dag2.ID {
		t.Errorf("expected latest DAG to be %s, got %s", dag2.ID, latest.ID)
	}
}

func TestDAGRepository_GetActiveByDatasource(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create completed DAG
	dag1 := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		OntologyID:   &tc.ontologyID,
		Status:       models.DAGStatusCompleted,
	}
	err := tc.repo.Create(ctx, dag1)
	if err != nil {
		t.Fatalf("Create completed DAG failed: %v", err)
	}

	// No active DAG yet
	active, err := tc.repo.GetActiveByDatasource(ctx, tc.datasourceID)
	if err != nil {
		t.Fatalf("GetActiveByDatasource failed: %v", err)
	}
	if active != nil {
		t.Error("expected no active DAG")
	}

	// Create running DAG
	dag2 := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    tc.projectID,
		DatasourceID: tc.datasourceID,
		OntologyID:   &tc.ontologyID,
		Status:       models.DAGStatusRunning,
	}
	err = tc.repo.Create(ctx, dag2)
	if err != nil {
		t.Fatalf("Create running DAG failed: %v", err)
	}

	// Now should find active
	active, err = tc.repo.GetActiveByDatasource(ctx, tc.datasourceID)
	if err != nil {
		t.Fatalf("GetActiveByDatasource failed: %v", err)
	}
	if active == nil {
		t.Fatal("expected to find active DAG")
	}
	if active.ID != dag2.ID {
		t.Errorf("expected active DAG to be %s, got %s", dag2.ID, active.ID)
	}
}

func TestDAGRepository_UpdateStatus(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := tc.createTestDAG(ctx)

	// Update to running
	currentNode := "EntityDiscovery"
	err := tc.repo.UpdateStatus(ctx, dag.ID, models.DAGStatusRunning, &currentNode)
	if err != nil {
		t.Fatalf("UpdateStatus to running failed: %v", err)
	}

	// Verify
	updated, _ := tc.repo.GetByID(ctx, dag.ID)
	if updated.Status != models.DAGStatusRunning {
		t.Errorf("expected status running, got %s", updated.Status)
	}
	if updated.CurrentNode == nil || *updated.CurrentNode != "EntityDiscovery" {
		t.Errorf("expected current_node EntityDiscovery, got %v", updated.CurrentNode)
	}
	if updated.StartedAt == nil {
		t.Error("expected started_at to be set")
	}

	// Update to completed
	err = tc.repo.UpdateStatus(ctx, dag.ID, models.DAGStatusCompleted, nil)
	if err != nil {
		t.Fatalf("UpdateStatus to completed failed: %v", err)
	}

	// Verify
	completed, _ := tc.repo.GetByID(ctx, dag.ID)
	if completed.Status != models.DAGStatusCompleted {
		t.Errorf("expected status completed, got %s", completed.Status)
	}
	if completed.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
}

// ============================================================================
// Node Tests
// ============================================================================

func TestDAGRepository_CreateNodes(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := tc.createTestDAG(ctx)

	// Create all nodes (7 nodes in current DAG)
	nodes := make([]models.DAGNode, 0, 7)
	for _, nodeName := range models.AllDAGNodes() {
		nodes = append(nodes, models.DAGNode{
			DAGID:     dag.ID,
			NodeName:  string(nodeName),
			NodeOrder: models.DAGNodeOrder[nodeName],
			Status:    models.DAGNodeStatusPending,
		})
	}

	err := tc.repo.CreateNodes(ctx, nodes)
	if err != nil {
		t.Fatalf("CreateNodes failed: %v", err)
	}

	// Verify nodes
	retrieved, err := tc.repo.GetNodesByDAG(ctx, dag.ID)
	if err != nil {
		t.Fatalf("GetNodesByDAG failed: %v", err)
	}
	expectedNodes := len(models.AllDAGNodes())
	if len(retrieved) != expectedNodes {
		t.Fatalf("expected %d nodes, got %d", expectedNodes, len(retrieved))
	}

	// Verify order
	for i, node := range retrieved {
		if node.NodeOrder != i+1 {
			t.Errorf("expected node order %d, got %d for node %s", i+1, node.NodeOrder, node.NodeName)
		}
	}
}

func TestDAGRepository_UpdateNodeStatus(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := tc.createTestDAG(ctx)

	// Create a node
	node := models.DAGNode{
		DAGID:     dag.ID,
		NodeName:  "EntityDiscovery",
		NodeOrder: 1,
		Status:    models.DAGNodeStatusPending,
	}
	err := tc.repo.CreateNodes(ctx, []models.DAGNode{node})
	if err != nil {
		t.Fatalf("CreateNodes failed: %v", err)
	}

	nodes, _ := tc.repo.GetNodesByDAG(ctx, dag.ID)
	nodeID := nodes[0].ID

	// Update to running
	err = tc.repo.UpdateNodeStatus(ctx, nodeID, models.DAGNodeStatusRunning, nil)
	if err != nil {
		t.Fatalf("UpdateNodeStatus to running failed: %v", err)
	}

	nodes, _ = tc.repo.GetNodesByDAG(ctx, dag.ID)
	if nodes[0].Status != models.DAGNodeStatusRunning {
		t.Errorf("expected status running, got %s", nodes[0].Status)
	}
	if nodes[0].StartedAt == nil {
		t.Error("expected started_at to be set")
	}

	// Update to completed
	err = tc.repo.UpdateNodeStatus(ctx, nodeID, models.DAGNodeStatusCompleted, nil)
	if err != nil {
		t.Fatalf("UpdateNodeStatus to completed failed: %v", err)
	}

	nodes, _ = tc.repo.GetNodesByDAG(ctx, dag.ID)
	if nodes[0].Status != models.DAGNodeStatusCompleted {
		t.Errorf("expected status completed, got %s", nodes[0].Status)
	}
	if nodes[0].CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
	if nodes[0].DurationMs == nil {
		t.Error("expected duration_ms to be set")
	}
}

func TestDAGRepository_UpdateNodeProgress(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := tc.createTestDAG(ctx)

	// Create a node
	node := models.DAGNode{
		DAGID:     dag.ID,
		NodeName:  "EntityDiscovery",
		NodeOrder: 1,
		Status:    models.DAGNodeStatusRunning,
	}
	err := tc.repo.CreateNodes(ctx, []models.DAGNode{node})
	if err != nil {
		t.Fatalf("CreateNodes failed: %v", err)
	}

	nodes, _ := tc.repo.GetNodesByDAG(ctx, dag.ID)
	nodeID := nodes[0].ID

	// Update progress
	progress := &models.DAGNodeProgress{
		Current: 5,
		Total:   10,
		Message: "Processing table users",
	}
	err = tc.repo.UpdateNodeProgress(ctx, nodeID, progress)
	if err != nil {
		t.Fatalf("UpdateNodeProgress failed: %v", err)
	}

	// Verify
	nodes, _ = tc.repo.GetNodesByDAG(ctx, dag.ID)
	if nodes[0].Progress == nil {
		t.Fatal("expected progress to be set")
	}
	if nodes[0].Progress.Current != 5 {
		t.Errorf("expected current 5, got %d", nodes[0].Progress.Current)
	}
	if nodes[0].Progress.Total != 10 {
		t.Errorf("expected total 10, got %d", nodes[0].Progress.Total)
	}
	if nodes[0].Progress.Message != "Processing table users" {
		t.Errorf("expected message 'Processing table users', got %s", nodes[0].Progress.Message)
	}
}

func TestDAGRepository_GetNextPendingNode(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := tc.createTestDAG(ctx)

	// Create nodes
	nodes := []models.DAGNode{
		{DAGID: dag.ID, NodeName: "EntityDiscovery", NodeOrder: 1, Status: models.DAGNodeStatusCompleted},
		{DAGID: dag.ID, NodeName: "EntityEnrichment", NodeOrder: 2, Status: models.DAGNodeStatusPending},
		{DAGID: dag.ID, NodeName: "FKDiscovery", NodeOrder: 3, Status: models.DAGNodeStatusPending},
	}
	err := tc.repo.CreateNodes(ctx, nodes)
	if err != nil {
		t.Fatalf("CreateNodes failed: %v", err)
	}

	// Get next pending
	next, err := tc.repo.GetNextPendingNode(ctx, dag.ID)
	if err != nil {
		t.Fatalf("GetNextPendingNode failed: %v", err)
	}
	if next == nil {
		t.Fatal("expected to find next pending node")
	}
	if next.NodeName != "EntityEnrichment" {
		t.Errorf("expected EntityEnrichment, got %s", next.NodeName)
	}
}

func TestDAGRepository_GetByIDWithNodes(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := tc.createTestDAG(ctx)

	// Create nodes
	nodes := []models.DAGNode{
		{DAGID: dag.ID, NodeName: "EntityDiscovery", NodeOrder: 1, Status: models.DAGNodeStatusPending},
		{DAGID: dag.ID, NodeName: "EntityEnrichment", NodeOrder: 2, Status: models.DAGNodeStatusPending},
	}
	err := tc.repo.CreateNodes(ctx, nodes)
	if err != nil {
		t.Fatalf("CreateNodes failed: %v", err)
	}

	// Get with nodes
	dagWithNodes, err := tc.repo.GetByIDWithNodes(ctx, dag.ID)
	if err != nil {
		t.Fatalf("GetByIDWithNodes failed: %v", err)
	}
	if dagWithNodes == nil {
		t.Fatal("expected to find DAG")
	}
	if len(dagWithNodes.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(dagWithNodes.Nodes))
	}
}

// ============================================================================
// Ownership Tests
// ============================================================================

func TestDAGRepository_ClaimOwnership(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := tc.createTestDAG(ctx)
	serverID := uuid.New()

	// Claim ownership
	claimed, err := tc.repo.ClaimOwnership(ctx, dag.ID, serverID)
	if err != nil {
		t.Fatalf("ClaimOwnership failed: %v", err)
	}
	if !claimed {
		t.Fatal("expected to claim unowned DAG")
	}

	// Verify ownership was set
	updated, _ := tc.repo.GetByID(ctx, dag.ID)
	if updated.OwnerID == nil || *updated.OwnerID != serverID {
		t.Errorf("expected owner_id to be %s, got %v", serverID, updated.OwnerID)
	}

	// Try to claim by another server (should fail)
	server2ID := uuid.New()
	claimed2, err := tc.repo.ClaimOwnership(ctx, dag.ID, server2ID)
	if err != nil {
		t.Fatalf("ClaimOwnership by server2 should not error: %v", err)
	}
	if claimed2 {
		t.Error("expected NOT to claim DAG owned by another server")
	}
}

func TestDAGRepository_ReleaseOwnership(t *testing.T) {
	tc := setupDAGTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	dag := tc.createTestDAG(ctx)
	serverID := uuid.New()

	// Claim and release
	tc.repo.ClaimOwnership(ctx, dag.ID, serverID)
	err := tc.repo.ReleaseOwnership(ctx, dag.ID)
	if err != nil {
		t.Fatalf("ReleaseOwnership failed: %v", err)
	}

	// Verify released
	updated, _ := tc.repo.GetByID(ctx, dag.ID)
	if updated.OwnerID != nil {
		t.Errorf("expected owner_id to be nil, got %v", updated.OwnerID)
	}
}

// Helper
func strPtr(s string) *string {
	return &s
}
