package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// Note: Full integration tests are in pkg/services/ontology_dag_integration_test.go
// These unit tests focus on DAG node creation and executor selection logic.

func TestDAGNodes_AllNodesHaveCorrectOrder(t *testing.T) {
	allNodes := models.AllDAGNodes()

	expectedOrder := []models.DAGNodeName{
		models.DAGNodeKnowledgeSeeding,
		models.DAGNodeEntityDiscovery,
		models.DAGNodeEntityEnrichment,
		models.DAGNodeFKDiscovery,
		models.DAGNodeColumnEnrichment,
		models.DAGNodePKMatchDiscovery,
		models.DAGNodeRelationshipEnrichment,
		models.DAGNodeOntologyFinalization,
		models.DAGNodeGlossaryDiscovery,
		models.DAGNodeGlossaryEnrichment,
	}

	assert.Equal(t, len(expectedOrder), len(allNodes))

	for i, expected := range expectedOrder {
		assert.Equal(t, expected, allNodes[i])
		assert.Equal(t, i+1, models.DAGNodeOrder[expected])
	}
}

func TestNodeExecutorInterfaces_AreWellDefined(t *testing.T) {
	// Verify that the interface methods are properly defined
	// by creating implementations that satisfy them

	// EntityDiscoveryMethods
	var edm dag.EntityDiscoveryMethods = &testEntityDiscovery{}
	assert.NotNil(t, edm)

	// EntityEnrichmentMethods
	var eem dag.EntityEnrichmentMethods = &testEntityEnrichment{}
	assert.NotNil(t, eem)

	// FKDiscoveryMethods
	var fkm dag.FKDiscoveryMethods = &testFKDiscovery{}
	assert.NotNil(t, fkm)

	// PKMatchDiscoveryMethods
	var pkm dag.PKMatchDiscoveryMethods = &testPKMatchDiscovery{}
	assert.NotNil(t, pkm)

	// RelationshipEnrichmentMethods
	var rem dag.RelationshipEnrichmentMethods = &testRelationshipEnrichment{}
	assert.NotNil(t, rem)

	// OntologyFinalizationMethods
	var ofm dag.OntologyFinalizationMethods = &testFinalization{}
	assert.NotNil(t, ofm)

	// ColumnEnrichmentMethods
	var cem dag.ColumnEnrichmentMethods = &testColumnEnrichment{}
	assert.NotNil(t, cem)

	// GlossaryDiscoveryMethods
	var gdm dag.GlossaryDiscoveryMethods = &testGlossaryDiscovery{}
	assert.NotNil(t, gdm)
}

func TestDAGStatus_ValidStatuses(t *testing.T) {
	validStatuses := []models.DAGStatus{
		models.DAGStatusPending,
		models.DAGStatusRunning,
		models.DAGStatusCompleted,
		models.DAGStatusFailed,
		models.DAGStatusCancelled,
	}

	for _, status := range validStatuses {
		assert.NotEmpty(t, string(status))
	}
}

func TestDAGNodeStatus_ValidStatuses(t *testing.T) {
	validStatuses := []models.DAGNodeStatus{
		models.DAGNodeStatusPending,
		models.DAGNodeStatusRunning,
		models.DAGNodeStatusCompleted,
		models.DAGNodeStatusFailed,
		models.DAGNodeStatusSkipped,
	}

	for _, status := range validStatuses {
		assert.NotEmpty(t, string(status))
	}
}

func TestNewExecutionContext(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	dagID := uuid.New()
	nodeID := uuid.New()

	dagRecord := &models.OntologyDAG{
		ID:           dagID,
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		OntologyID:   &ontologyID,
	}

	node := &models.DAGNode{
		ID:       nodeID,
		DAGID:    dagID,
		NodeName: string(models.DAGNodeEntityDiscovery),
	}

	ctx := dag.NewExecutionContext(dagRecord, node)

	assert.Equal(t, dagRecord, ctx.DAG)
	assert.Equal(t, nodeID, ctx.NodeID)
	assert.Equal(t, projectID, ctx.ProjectID)
	assert.Equal(t, datasourceID, ctx.DatasourceID)
	assert.Equal(t, ontologyID, ctx.OntologyID)
}

func TestNewExecutionContext_NilOntologyID(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	nodeID := uuid.New()

	dagRecord := &models.OntologyDAG{
		ID:           dagID,
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		OntologyID:   nil, // No ontology assigned yet
	}

	node := &models.DAGNode{
		ID:       nodeID,
		DAGID:    dagID,
		NodeName: string(models.DAGNodeEntityDiscovery),
	}

	ctx := dag.NewExecutionContext(dagRecord, node)

	assert.Equal(t, uuid.Nil, ctx.OntologyID)
}

// Test implementations to verify interfaces compile correctly

type testEntityDiscovery struct{}

func (t *testEntityDiscovery) IdentifyEntitiesFromDDL(_ context.Context, _, _, _ uuid.UUID) (int, []*models.SchemaTable, []*models.SchemaColumn, error) {
	return 0, nil, nil, nil
}

type testEntityEnrichment struct{}

func (t *testEntityEnrichment) EnrichEntitiesWithLLM(_ context.Context, _, _, _ uuid.UUID) error {
	return nil
}

type testFKDiscovery struct{}

func (t *testFKDiscovery) DiscoverFKRelationships(_ context.Context, _, _ uuid.UUID, _ dag.ProgressCallback) (*dag.FKDiscoveryResult, error) {
	return nil, nil
}

type testPKMatchDiscovery struct{}

func (t *testPKMatchDiscovery) DiscoverPKMatchRelationships(_ context.Context, _, _ uuid.UUID, _ dag.ProgressCallback) (*dag.PKMatchDiscoveryResult, error) {
	return nil, nil
}

type testRelationshipEnrichment struct{}

func (t *testRelationshipEnrichment) EnrichProject(_ context.Context, _ uuid.UUID, _ dag.ProgressCallback) (*dag.RelationshipEnrichmentResult, error) {
	return nil, nil
}

type testFinalization struct{}

func (t *testFinalization) Finalize(_ context.Context, _ uuid.UUID) error {
	return nil
}

type testColumnEnrichment struct{}

func (t *testColumnEnrichment) EnrichProject(_ context.Context, _ uuid.UUID, _ []string, _ dag.ProgressCallback) (*dag.ColumnEnrichmentResult, error) {
	return nil, nil
}

type testGlossaryDiscovery struct{}

func (t *testGlossaryDiscovery) DiscoverGlossaryTerms(_ context.Context, _, _ uuid.UUID) (int, error) {
	return 0, nil
}

// ============================================================================
// Delete Method Tests
// ============================================================================

// Note: Delete method integration tests would require real database fixtures.
// The handler tests in pkg/handlers/ontology_dag_handler_test.go provide
// end-to-end coverage with mocked service. The service-level logic is
// straightforward repository delegation, so integration tests are more valuable
// than unit tests with complex mock implementations.

// ============================================================================
// Cancel Method Tests
// ============================================================================

// mockDAGRepository is a mock implementation for testing Cancel
type mockDAGRepository struct {
	getNodesByDAGFunc    func(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error)
	updateNodeStatusFunc func(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error
	updateStatusFunc     func(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error
	getByIDWithNodesFunc func(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error)
}

func (m *mockDAGRepository) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	if m.getNodesByDAGFunc != nil {
		return m.getNodesByDAGFunc(ctx, dagID)
	}
	return nil, nil
}

func (m *mockDAGRepository) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	if m.updateNodeStatusFunc != nil {
		return m.updateNodeStatusFunc(ctx, nodeID, status, errorMessage)
	}
	return nil
}

func (m *mockDAGRepository) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, dagID, status, currentNode)
	}
	return nil
}

// Stub methods to satisfy the interface
func (m *mockDAGRepository) Create(ctx context.Context, dag *models.OntologyDAG) error { return nil }
func (m *mockDAGRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	if m.getByIDWithNodesFunc != nil {
		return m.getByIDWithNodesFunc(ctx, id)
	}
	return nil, nil
}
func (m *mockDAGRepository) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) Update(ctx context.Context, dag *models.OntologyDAG) error { return nil }
func (m *mockDAGRepository) Delete(ctx context.Context, id uuid.UUID) error            { return nil }
func (m *mockDAGRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockDAGRepository) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockDAGRepository) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockDAGRepository) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error { return nil }
func (m *mockDAGRepository) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockDAGRepository) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	return nil
}
func (m *mockDAGRepository) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockDAGRepository) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// TestCancel_MarksNonCompletedNodesAsSkipped verifies that canceling a DAG
// marks all running and pending nodes as skipped.
func TestCancel_MarksNonCompletedNodesAsSkipped(t *testing.T) {
	dagID := uuid.New()
	ctx := context.Background()

	// Create test nodes with various statuses
	nodes := []models.DAGNode{
		{ID: uuid.New(), DAGID: dagID, NodeName: "EntityDiscovery", Status: models.DAGNodeStatusCompleted},
		{ID: uuid.New(), DAGID: dagID, NodeName: "EntityEnrichment", Status: models.DAGNodeStatusRunning},
		{ID: uuid.New(), DAGID: dagID, NodeName: "FKDiscovery", Status: models.DAGNodeStatusPending},
		{ID: uuid.New(), DAGID: dagID, NodeName: "RelationshipEnrichment", Status: models.DAGNodeStatusPending},
	}

	// Track which nodes were marked as skipped
	skippedNodes := make(map[uuid.UUID]bool)

	mockRepo := &mockDAGRepository{
		getNodesByDAGFunc: func(_ context.Context, id uuid.UUID) ([]models.DAGNode, error) {
			assert.Equal(t, dagID, id)
			return nodes, nil
		},
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, _ *string) error {
			assert.Equal(t, models.DAGNodeStatusSkipped, status)
			skippedNodes[nodeID] = true
			return nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			assert.Equal(t, dagID, id)
			assert.Equal(t, models.DAGStatusCancelled, status)
			return nil
		},
	}

	// Create service with mock repository
	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
	}

	// Execute Cancel
	err := service.Cancel(ctx, dagID)
	assert.NoError(t, err)

	// Verify only non-completed nodes were marked as skipped
	// Node 0 (completed) should NOT be skipped
	assert.False(t, skippedNodes[nodes[0].ID], "Completed node should not be skipped")
	// Node 1 (running) should be skipped
	assert.True(t, skippedNodes[nodes[1].ID], "Running node should be skipped")
	// Node 2 (pending) should be skipped
	assert.True(t, skippedNodes[nodes[2].ID], "Pending node should be skipped")
	// Node 3 (pending) should be skipped
	assert.True(t, skippedNodes[nodes[3].ID], "Pending node should be skipped")
}

// ============================================================================
// markDAGFailed Tests
// ============================================================================

// TestMarkDAGFailed_StoresErrorOnCurrentNode verifies that early startup errors
// are properly stored on the current node for UI visibility.
func TestMarkDAGFailed_StoresErrorOnCurrentNode(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	currentNodeName := string(models.DAGNodeEntityDiscovery)
	errorMessage := "failed to get tenant context"

	// Create test nodes
	nodes := []models.DAGNode{
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeEntityDiscovery), Status: models.DAGNodeStatusRunning},
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeEntityEnrichment), Status: models.DAGNodeStatusPending},
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeFKDiscovery), Status: models.DAGNodeStatusPending},
	}

	dag := &models.OntologyDAG{
		ID:          dagID,
		ProjectID:   projectID,
		Status:      models.DAGStatusRunning,
		CurrentNode: &currentNodeName,
		Nodes:       nodes,
	}

	// Track updates
	var updatedNodeID uuid.UUID
	var updatedErrorMsg string
	var updatedDAGStatus models.DAGStatus

	mockRepo := &mockDAGRepository{
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errMsg *string) error {
			updatedNodeID = nodeID
			if errMsg != nil {
				updatedErrorMsg = *errMsg
			}
			assert.Equal(t, models.DAGNodeStatusFailed, status)
			return nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			updatedDAGStatus = status
			assert.Equal(t, dagID, id)
			return nil
		},
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			assert.Equal(t, dagID, id)
			return dag, nil
		},
	}

	// Create service with mock repository and mock getTenantCtx
	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			// Return a valid context for this test
			return ctx, func() {}, nil
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify the current node was marked as failed with the error message
	assert.Equal(t, nodes[0].ID, updatedNodeID, "Current node should be marked as failed")
	assert.Equal(t, errorMessage, updatedErrorMsg, "Error message should be stored on the node")
	assert.Equal(t, models.DAGStatusFailed, updatedDAGStatus, "DAG should be marked as failed")
}

// TestMarkDAGFailed_StoresErrorOnFirstPendingNode verifies that when no current node
// is set, the error is stored on the first pending/running node.
func TestMarkDAGFailed_StoresErrorOnFirstPendingNode(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	errorMessage := "failed during initialization"

	// Create test nodes - first node is pending (no current node set yet)
	nodes := []models.DAGNode{
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeEntityDiscovery), Status: models.DAGNodeStatusPending},
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeEntityEnrichment), Status: models.DAGNodeStatusPending},
	}

	dag := &models.OntologyDAG{
		ID:          dagID,
		ProjectID:   projectID,
		Status:      models.DAGStatusPending,
		CurrentNode: nil, // No current node set
		Nodes:       nodes,
	}

	// Track updates
	var updatedNodeID uuid.UUID
	var updatedErrorMsg string

	mockRepo := &mockDAGRepository{
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errMsg *string) error {
			updatedNodeID = nodeID
			if errMsg != nil {
				updatedErrorMsg = *errMsg
			}
			assert.Equal(t, models.DAGNodeStatusFailed, status)
			return nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			assert.Equal(t, dagID, id)
			assert.Equal(t, models.DAGStatusFailed, status)
			return nil
		},
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			return dag, nil
		},
	}

	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify the first pending node was marked as failed
	assert.Equal(t, nodes[0].ID, updatedNodeID, "First pending node should be marked as failed")
	assert.Equal(t, errorMessage, updatedErrorMsg, "Error message should be stored on the node")
}

// TestMarkDAGFailed_WhenGetByIDWithNodesFails_StillMarksDAGFailed verifies that even
// if we can't get the nodes, the DAG status is still updated to failed.
func TestMarkDAGFailed_WhenGetByIDWithNodesFails_StillMarksDAGFailed(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	errorMessage := "initialization failed"

	// Track updates
	var dagUpdateCalled bool

	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			return nil, assert.AnError // Simulate failure to get DAG
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			dagUpdateCalled = true
			assert.Equal(t, dagID, id)
			assert.Equal(t, models.DAGStatusFailed, status)
			return nil
		},
	}

	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify the DAG status was still updated even though GetByIDWithNodes failed
	assert.True(t, dagUpdateCalled, "DAG status should be updated even if GetByIDWithNodes fails")
}

// TestMarkDAGFailed_WithAllNodesCompleted_MarksFirstNode verifies that when all nodes
// are completed (no pending/running nodes), the first node is marked as failed.
func TestMarkDAGFailed_WithAllNodesCompleted_MarksFirstNode(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	errorMessage := "post-completion error"

	// Create test nodes - all completed
	nodes := []models.DAGNode{
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeEntityDiscovery), Status: models.DAGNodeStatusCompleted},
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeEntityEnrichment), Status: models.DAGNodeStatusCompleted},
	}

	dag := &models.OntologyDAG{
		ID:          dagID,
		ProjectID:   projectID,
		Status:      models.DAGStatusCompleted,
		CurrentNode: nil,
		Nodes:       nodes,
	}

	// Track updates
	var updatedNodeID uuid.UUID

	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			return dag, nil
		},
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errMsg *string) error {
			updatedNodeID = nodeID
			assert.Equal(t, models.DAGNodeStatusFailed, status)
			assert.Equal(t, errorMessage, *errMsg)
			return nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			assert.Equal(t, models.DAGStatusFailed, status)
			return nil
		},
	}

	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify the first node was marked as failed (fallback behavior)
	assert.Equal(t, nodes[0].ID, updatedNodeID, "First node should be marked as failed when all nodes are completed")
}

// TestMarkDAGFailed_WhenTenantCtxFails_LogsError verifies that when getTenantCtx fails,
// only error logging occurs and no updates are attempted.
func TestMarkDAGFailed_WhenTenantCtxFails_LogsError(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	errorMessage := "some error"

	// Track if any repository methods were called
	var repoMethodCalled bool

	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			repoMethodCalled = true
			return nil, nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			repoMethodCalled = true
			return nil
		},
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errMsg *string) error {
			repoMethodCalled = true
			return nil
		},
	}

	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return nil, nil, assert.AnError // Simulate failure to get tenant context
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify no repository methods were called
	assert.False(t, repoMethodCalled, "No repository methods should be called when getTenantCtx fails")
}

// TestExecuteDAG_PanicRecovery verifies that panics during DAG execution
// are recovered, cleanup happens properly, and the DAG is marked as failed.
func TestExecuteDAG_PanicRecovery(t *testing.T) {
	projectID := uuid.New()
	dagID := uuid.New()

	// Track whether markDAGFailed was called
	var markDAGFailedCalls []string
	var mu sync.Mutex

	// Create mock repository that tracks calls
	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			// Return a DAG with one node
			return &models.OntologyDAG{
				ID:        dagID,
				ProjectID: projectID,
				Nodes: []models.DAGNode{
					{
						ID:       uuid.New(),
						DAGID:    dagID,
						NodeName: string(models.DAGNodeEntityDiscovery),
						Status:   models.DAGNodeStatusPending,
					},
				},
			}, nil
		},
		updateStatusFunc: func(ctx context.Context, id uuid.UUID, status models.DAGStatus, currentNode *string) error {
			if status == models.DAGStatusFailed {
				mu.Lock()
				markDAGFailedCalls = append(markDAGFailedCalls, "updateStatus")
				mu.Unlock()
			}
			return nil
		},
		updateNodeStatusFunc: func(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
			if status == models.DAGNodeStatusFailed && errorMessage != nil {
				mu.Lock()
				markDAGFailedCalls = append(markDAGFailedCalls, fmt.Sprintf("updateNodeStatus: %s", *errorMessage))
				mu.Unlock()
			}
			return nil
		},
	}

	// Create a test service
	service := &ontologyDAGService{
		dagRepo:          mockRepo,
		logger:           zap.NewNop(),
		serverInstanceID: uuid.New(),
	}

	// Set getTenantCtx to cause a panic only on first call
	// (subsequent calls from markDAGFailed should succeed)
	callCount := 0
	var callMu sync.Mutex
	service.getTenantCtx = func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		callMu.Lock()
		callCount++
		count := callCount
		callMu.Unlock()

		if count == 1 {
			// First call - panic to test recovery
			panic("simulated panic during DAG execution")
		}
		// Subsequent calls - succeed to allow markDAGFailed to work
		return ctx, func() {}, nil
	}

	// Execute DAG in a goroutine (as it would be in production)
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.executeDAG(projectID, dagID)
	}()

	// Wait for goroutine to complete with timeout
	select {
	case <-done:
		// Good, execution completed
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for executeDAG to complete")
	}

	// Give a small amount of time for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	// Verify markDAGFailed was called (which updates status and node status)
	mu.Lock()
	defer mu.Unlock()
	assert.True(t, len(markDAGFailedCalls) >= 1, "markDAGFailed should have been called at least once")

	// Check that at least one call mentions panic
	foundPanicMessage := false
	for _, call := range markDAGFailedCalls {
		if contains(call, "panic") {
			foundPanicMessage = true
			break
		}
	}
	assert.True(t, foundPanicMessage, "Error message should mention panic")

	// Verify activeDAGs was cleaned up
	_, exists := service.activeDAGs.Load(dagID)
	assert.False(t, exists, "DAG should be removed from activeDAGs after panic")

	// Verify heartbeat was cleaned up
	_, exists = service.heartbeatCancel.Load(dagID)
	assert.False(t, exists, "Heartbeat should be cleaned up after panic")
}

// TestExecuteDAG_HeartbeatCleanupOrder verifies that the heartbeat
// is properly started and stopped even when execution fails early.
func TestExecuteDAG_HeartbeatCleanupOrder(t *testing.T) {
	projectID := uuid.New()
	dagID := uuid.New()

	// Create mock repository
	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			// Return error to cause early exit
			return nil, fmt.Errorf("simulated repository error")
		},
		updateStatusFunc: func(ctx context.Context, id uuid.UUID, status models.DAGStatus, currentNode *string) error {
			return nil
		},
		updateNodeStatusFunc: func(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
			return nil
		},
	}

	// Create a test service
	service := &ontologyDAGService{
		dagRepo:          mockRepo,
		logger:           zap.NewNop(),
		serverInstanceID: uuid.New(),
	}

	// Set getTenantCtx to succeed
	service.getTenantCtx = func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	// Execute DAG in a goroutine
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.executeDAG(projectID, dagID)
	}()

	// Wait for goroutine to complete with timeout
	select {
	case <-done:
		// Good, execution completed
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for executeDAG to complete")
	}

	// Give a small amount of time for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	// Verify activeDAGs was cleaned up
	_, exists := service.activeDAGs.Load(dagID)
	assert.False(t, exists, "DAG should be removed from activeDAGs after completion")

	// Verify heartbeat was cleaned up
	_, exists = service.heartbeatCancel.Load(dagID)
	assert.False(t, exists, "Heartbeat should be cleaned up after completion")
}

// ============================================================================
// GetNodeExecutor Tests
// ============================================================================

func TestGetNodeExecutor_GlossaryDiscovery(t *testing.T) {
	service := &ontologyDAGService{
		dagRepo:                  &mockDAGRepository{},
		logger:                   zap.NewNop(),
		glossaryDiscoveryMethods: &testGlossaryDiscovery{},
	}

	nodeID := uuid.New()
	executor, err := service.getNodeExecutor(models.DAGNodeGlossaryDiscovery, nodeID)

	assert.NoError(t, err)
	assert.NotNil(t, executor)
	assert.IsType(t, &dag.GlossaryDiscoveryNode{}, executor)
}

func TestGetNodeExecutor_GlossaryDiscovery_NotSet(t *testing.T) {
	service := &ontologyDAGService{
		dagRepo: &mockDAGRepository{},
		logger:  zap.NewNop(),
		// glossaryDiscoveryMethods intentionally not set
	}

	nodeID := uuid.New()
	executor, err := service.getNodeExecutor(models.DAGNodeGlossaryDiscovery, nodeID)

	assert.Error(t, err)
	assert.Nil(t, executor)
	assert.Contains(t, err.Error(), "glossary discovery methods not set")
}

func TestSetGlossaryMethods(t *testing.T) {
	service := &ontologyDAGService{
		dagRepo: &mockDAGRepository{},
		logger:  zap.NewNop(),
	}

	// Test SetGlossaryDiscoveryMethods
	discoveryMethods := &testGlossaryDiscovery{}
	service.SetGlossaryDiscoveryMethods(discoveryMethods)
	assert.Equal(t, discoveryMethods, service.glossaryDiscoveryMethods)
}
