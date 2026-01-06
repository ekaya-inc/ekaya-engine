package dag

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockGlossaryDiscoveryMethods implements GlossaryDiscoveryMethods for testing.
type mockGlossaryDiscoveryMethods struct {
	discoverFunc func(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error)
}

func (m *mockGlossaryDiscoveryMethods) DiscoverGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) (int, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, projectID, ontologyID)
	}
	return 0, nil
}

// mockDAGRepository implements OntologyDAGRepository for testing.
type mockGlossaryDiscoveryDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockGlossaryDiscoveryDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetByID(ctx context.Context, dagID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) UpdateCurrentNode(ctx context.Context, dagID uuid.UUID, nodeName models.DAGNodeName) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) CompleteDAG(ctx context.Context, dagID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) FailDAG(ctx context.Context, dagID uuid.UUID, errorMsg string) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetNodeByName(ctx context.Context, dagID uuid.UUID, nodeName models.DAGNodeName) (*models.DAGNode, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) CreateNode(ctx context.Context, node *models.DAGNode) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMsg *string) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) CompleteNode(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) FailNode(ctx context.Context, nodeID uuid.UUID, errorMsg string) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}

func (m *mockGlossaryDiscoveryDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryDiscoveryDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

func TestGlossaryDiscoveryNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	nodeID := uuid.New()

	// Track progress reports
	var progressReports []string

	mockGlossary := &mockGlossaryDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, oID uuid.UUID) (int, error) {
			assert.Equal(t, projectID, pID)
			assert.Equal(t, ontologyID, oID)
			return 5, nil // 5 terms discovered
		},
	}

	mockRepo := &mockGlossaryDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewGlossaryDiscoveryNode(mockRepo, mockGlossary, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:  projectID,
		OntologyID: &ontologyID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify progress was reported
	assert.Len(t, progressReports, 2, "Should report initial and completion progress")
	assert.Equal(t, "Discovering business terms...", progressReports[0])
	assert.Equal(t, "Discovered 5 business terms", progressReports[1])
}

func TestGlossaryDiscoveryNode_Execute_NoOntologyID(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	mockGlossary := &mockGlossaryDiscoveryMethods{}
	mockRepo := &mockGlossaryDiscoveryDAGRepo{}

	node := NewGlossaryDiscoveryNode(mockRepo, mockGlossary, zap.NewNop())

	dag := &models.OntologyDAG{
		ProjectID:  projectID,
		OntologyID: nil, // Missing ontology ID
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ontology ID is required")
}

func TestGlossaryDiscoveryNode_Execute_DiscoveryError(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	expectedErr := errors.New("discovery failed")

	mockGlossary := &mockGlossaryDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, oID uuid.UUID) (int, error) {
			return 0, expectedErr
		},
	}

	mockRepo := &mockGlossaryDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			return nil
		},
	}

	node := NewGlossaryDiscoveryNode(mockRepo, mockGlossary, zap.NewNop())
	node.SetCurrentNodeID(uuid.New())

	dag := &models.OntologyDAG{
		ProjectID:  projectID,
		OntologyID: &ontologyID,
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discover glossary terms")
	assert.Contains(t, err.Error(), expectedErr.Error())
}

func TestGlossaryDiscoveryNode_Execute_ProgressReportingError(t *testing.T) {
	// Progress reporting errors should not fail the execution
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	nodeID := uuid.New()

	mockGlossary := &mockGlossaryDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, oID uuid.UUID) (int, error) {
			return 3, nil
		},
	}

	mockRepo := &mockGlossaryDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			return errors.New("progress update failed")
		},
	}

	node := NewGlossaryDiscoveryNode(mockRepo, mockGlossary, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:  projectID,
		OntologyID: &ontologyID,
	}

	// Should succeed despite progress reporting errors
	err := node.Execute(ctx, dag)
	require.NoError(t, err)
}

func TestGlossaryDiscoveryNode_Name(t *testing.T) {
	mockRepo := &mockGlossaryDiscoveryDAGRepo{}
	mockGlossary := &mockGlossaryDiscoveryMethods{}

	node := NewGlossaryDiscoveryNode(mockRepo, mockGlossary, zap.NewNop())

	assert.Equal(t, models.DAGNodeGlossaryDiscovery, node.Name())
}
