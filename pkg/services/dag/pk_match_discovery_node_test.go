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

// mockPKMatchDiscoveryDAGRepo is a minimal mock for the DAG repository.
type mockPKMatchDiscoveryDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockPKMatchDiscoveryDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockPKMatchDiscoveryDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockPKMatchDiscoveryDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockPKMatchDiscoveryDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockPKMatchDiscoveryDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockPKMatchDiscoveryDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockPKMatchDiscoveryDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockPKMatchDiscoveryDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockPKMatchDiscoveryDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockPKMatchDiscoveryDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockPKMatchDiscoveryDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockPKMatchDiscoveryDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// mockPKMatchDiscoveryMethods implements PKMatchDiscoveryMethods for testing.
type mockPKMatchDiscoveryMethods struct {
	discoverFunc func(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (*PKMatchDiscoveryResult, error)
}

func (m *mockPKMatchDiscoveryMethods) DiscoverPKMatchRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (*PKMatchDiscoveryResult, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, projectID, datasourceID, progressCallback)
	}
	return &PKMatchDiscoveryResult{}, nil
}

func TestPKMatchDiscoveryNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	datasourceID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockSvc := &mockPKMatchDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*PKMatchDiscoveryResult, error) {
			assert.Equal(t, projectID, pID)
			assert.Equal(t, datasourceID, dsID)
			return &PKMatchDiscoveryResult{
				InferredRelationships: 8,
			}, nil
		},
	}

	mockRepo := &mockPKMatchDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewPKMatchDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	require.Len(t, progressReports, 2)
	assert.Equal(t, "Discovering relationships via pairwise SQL join analysis...", progressReports[0])
	assert.Equal(t, "Discovered 8 relationships via join analysis", progressReports[1])
}

func TestPKMatchDiscoveryNode_Execute_ServiceError(t *testing.T) {
	ctx := context.Background()
	nodeID := uuid.New()

	mockSvc := &mockPKMatchDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*PKMatchDiscoveryResult, error) {
			return nil, errors.New("query timeout")
		},
	}

	mockRepo := &mockPKMatchDiscoveryDAGRepo{}

	node := NewPKMatchDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discover pk_match relationships")
	assert.Contains(t, err.Error(), "query timeout")
}

func TestPKMatchDiscoveryNode_Execute_ProgressCallback(t *testing.T) {
	var progressReports []string

	mockSvc := &mockPKMatchDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*PKMatchDiscoveryResult, error) {
			progressCallback(5, 20, "Analyzing pair 5/20")
			return &PKMatchDiscoveryResult{InferredRelationships: 3}, nil
		},
	}

	mockRepo := &mockPKMatchDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewPKMatchDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(uuid.New())

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	require.NoError(t, err)

	assert.Contains(t, progressReports, "Analyzing pair 5/20")
}

func TestPKMatchDiscoveryNode_Name(t *testing.T) {
	node := NewPKMatchDiscoveryNode(&mockPKMatchDiscoveryDAGRepo{}, &mockPKMatchDiscoveryMethods{}, zap.NewNop())
	assert.Equal(t, models.DAGNodePKMatchDiscovery, node.Name())
}
