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

// mockFKDiscoveryDAGRepo is a minimal mock for the DAG repository.
type mockFKDiscoveryDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockFKDiscoveryDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockFKDiscoveryDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockFKDiscoveryDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockFKDiscoveryDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockFKDiscoveryDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockFKDiscoveryDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockFKDiscoveryDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockFKDiscoveryDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockFKDiscoveryDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockFKDiscoveryDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockFKDiscoveryDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockFKDiscoveryDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// mockFKDiscoveryMethods implements FKDiscoveryMethods for testing.
type mockFKDiscoveryMethods struct {
	discoverFunc func(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (*FKDiscoveryResult, error)
}

func (m *mockFKDiscoveryMethods) DiscoverFKRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (*FKDiscoveryResult, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, projectID, datasourceID, progressCallback)
	}
	return &FKDiscoveryResult{}, nil
}

func TestFKDiscoveryNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	datasourceID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockSvc := &mockFKDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*FKDiscoveryResult, error) {
			assert.Equal(t, projectID, pID)
			assert.Equal(t, datasourceID, dsID)
			return &FKDiscoveryResult{
				FKRelationships: 12,
			}, nil
		},
	}

	mockRepo := &mockFKDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewFKDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	require.Len(t, progressReports, 2)
	assert.Equal(t, "Discovering FK relationships from database constraints...", progressReports[0])
	assert.Equal(t, "Discovered 12 FK relationships from database constraints", progressReports[1])
}

func TestFKDiscoveryNode_Execute_ServiceError(t *testing.T) {
	ctx := context.Background()
	nodeID := uuid.New()

	mockSvc := &mockFKDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*FKDiscoveryResult, error) {
			return nil, errors.New("connection refused")
		},
	}

	mockRepo := &mockFKDiscoveryDAGRepo{}

	node := NewFKDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "discover FK relationships")
	assert.Contains(t, err.Error(), "connection refused")
}

func TestFKDiscoveryNode_Execute_ProgressCallback(t *testing.T) {
	var progressReports []string

	mockSvc := &mockFKDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*FKDiscoveryResult, error) {
			progressCallback(3, 10, "Scanning table 3/10")
			return &FKDiscoveryResult{FKRelationships: 5}, nil
		},
	}

	mockRepo := &mockFKDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewFKDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(uuid.New())

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	require.NoError(t, err)

	assert.Contains(t, progressReports, "Scanning table 3/10")
}

func TestFKDiscoveryNode_Name(t *testing.T) {
	node := NewFKDiscoveryNode(&mockFKDiscoveryDAGRepo{}, &mockFKDiscoveryMethods{}, zap.NewNop())
	assert.Equal(t, models.DAGNodeFKDiscovery, node.Name())
}
