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

// mockRelationshipDiscoveryDAGRepo is a minimal mock for the DAG repository.
type mockRelationshipDiscoveryDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockRelationshipDiscoveryDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockRelationshipDiscoveryDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockRelationshipDiscoveryDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockRelationshipDiscoveryDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockRelationshipDiscoveryDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockRelationshipDiscoveryDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockRelationshipDiscoveryDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockRelationshipDiscoveryDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockRelationshipDiscoveryDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockRelationshipDiscoveryDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockRelationshipDiscoveryDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// mockLLMRelationshipDiscoveryMethods implements LLMRelationshipDiscoveryMethods for testing.
type mockLLMRelationshipDiscoveryMethods struct {
	discoverFunc func(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (*LLMRelationshipDiscoveryResult, error)
}

func (m *mockLLMRelationshipDiscoveryMethods) DiscoverRelationships(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (*LLMRelationshipDiscoveryResult, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, projectID, datasourceID, progressCallback)
	}
	return &LLMRelationshipDiscoveryResult{}, nil
}

func TestRelationshipDiscoveryNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	datasourceID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockSvc := &mockLLMRelationshipDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*LLMRelationshipDiscoveryResult, error) {
			assert.Equal(t, projectID, pID)
			assert.Equal(t, datasourceID, dsID)
			return &LLMRelationshipDiscoveryResult{
				CandidatesEvaluated:   15,
				RelationshipsCreated:  10,
				RelationshipsRejected: 5,
				PreservedDBFKs:        3,
				PreservedColumnFKs:    2,
				DurationMs:            3000,
			}, nil
		},
	}

	mockRepo := &mockRelationshipDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewRelationshipDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Only initial progress — the node doesn't report completion (service does)
	require.Len(t, progressReports, 1)
	assert.Equal(t, "Starting LLM-validated relationship discovery...", progressReports[0])
}

func TestRelationshipDiscoveryNode_Execute_ServiceError(t *testing.T) {
	ctx := context.Background()
	nodeID := uuid.New()

	mockSvc := &mockLLMRelationshipDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*LLMRelationshipDiscoveryResult, error) {
			return nil, errors.New("LLM rate limited")
		},
	}

	mockRepo := &mockRelationshipDiscoveryDAGRepo{}

	node := NewRelationshipDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM relationship discovery failed")
	assert.Contains(t, err.Error(), "LLM rate limited")
}

func TestRelationshipDiscoveryNode_Execute_ProgressCallback(t *testing.T) {
	var progressReports []string

	mockSvc := &mockLLMRelationshipDiscoveryMethods{
		discoverFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (*LLMRelationshipDiscoveryResult, error) {
			progressCallback(3, 10, "Evaluating candidate 3/10")
			progressCallback(10, 10, "Discovery complete")
			return &LLMRelationshipDiscoveryResult{
				CandidatesEvaluated:  10,
				RelationshipsCreated: 7,
			}, nil
		},
	}

	mockRepo := &mockRelationshipDiscoveryDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewRelationshipDiscoveryNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(uuid.New())

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	require.NoError(t, err)

	assert.Contains(t, progressReports, "Evaluating candidate 3/10")
	assert.Contains(t, progressReports, "Discovery complete")
}

func TestRelationshipDiscoveryNode_Name(t *testing.T) {
	// Note: RelationshipDiscoveryNode uses DAGNodePKMatchDiscovery as its name
	// (it replaces the old PKMatchDiscoveryNode)
	node := NewRelationshipDiscoveryNode(&mockRelationshipDiscoveryDAGRepo{}, &mockLLMRelationshipDiscoveryMethods{}, zap.NewNop())
	assert.Equal(t, models.DAGNodePKMatchDiscovery, node.Name())
}
