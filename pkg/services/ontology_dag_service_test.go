package services

import (
	"context"
	"testing"

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
		models.DAGNodeEntityDiscovery,
		models.DAGNodeEntityEnrichment,
		models.DAGNodeRelationshipDiscovery,
		models.DAGNodeRelationshipEnrichment,
		models.DAGNodeOntologyFinalization,
		models.DAGNodeColumnEnrichment,
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

	// DeterministicRelationshipMethods
	var drm dag.DeterministicRelationshipMethods = &testRelationshipDiscovery{}
	assert.NotNil(t, drm)

	// RelationshipEnrichmentMethods
	var rem dag.RelationshipEnrichmentMethods = &testRelationshipEnrichment{}
	assert.NotNil(t, rem)

	// OntologyFinalizationMethods
	var ofm dag.OntologyFinalizationMethods = &testFinalization{}
	assert.NotNil(t, ofm)

	// ColumnEnrichmentMethods
	var cem dag.ColumnEnrichmentMethods = &testColumnEnrichment{}
	assert.NotNil(t, cem)
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

type testRelationshipDiscovery struct{}

func (t *testRelationshipDiscovery) DiscoverRelationships(_ context.Context, _, _ uuid.UUID) (*dag.RelationshipDiscoveryResult, error) {
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
		{ID: uuid.New(), DAGID: dagID, NodeName: "RelationshipDiscovery", Status: models.DAGNodeStatusPending},
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
