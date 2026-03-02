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

// mockBaseNodeDAGRepo is a minimal mock for testing BaseNode directly.
type mockBaseNodeDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockBaseNodeDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockBaseNodeDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockBaseNodeDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockBaseNodeDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockBaseNodeDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockBaseNodeDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockBaseNodeDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockBaseNodeDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockBaseNodeDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockBaseNodeDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockBaseNodeDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockBaseNodeDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

func TestBaseNode_ReportProgress_NilNodeID(t *testing.T) {
	mockRepo := &mockBaseNodeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			t.Fatal("should not call UpdateNodeProgress when nodeID is nil")
			return nil
		},
	}

	node := NewBaseNode("test_node", mockRepo, zap.NewNop())
	// Don't set node ID — it stays as uuid.Nil

	err := node.ReportProgress(context.Background(), 1, 10, "test message")
	require.NoError(t, err)
}

func TestBaseNode_ReportProgress_WithValidNodeID(t *testing.T) {
	nodeID := uuid.New()
	var capturedNodeID uuid.UUID
	var capturedProgress *models.DAGNodeProgress

	mockRepo := &mockBaseNodeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			capturedNodeID = nID
			capturedProgress = progress
			return nil
		},
	}

	node := NewBaseNode("test_node", mockRepo, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	err := node.ReportProgress(context.Background(), 5, 10, "processing step 5")
	require.NoError(t, err)

	assert.Equal(t, nodeID, capturedNodeID)
	require.NotNil(t, capturedProgress)
	assert.Equal(t, 5, capturedProgress.Current)
	assert.Equal(t, 10, capturedProgress.Total)
	assert.Equal(t, "processing step 5", capturedProgress.Message)
}

func TestBaseNode_ReportProgress_RepoError(t *testing.T) {
	nodeID := uuid.New()

	mockRepo := &mockBaseNodeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			return errors.New("database unavailable")
		},
	}

	node := NewBaseNode("test_node", mockRepo, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	err := node.ReportProgress(context.Background(), 1, 1, "test")
	require.Error(t, err)
	assert.Equal(t, "database unavailable", err.Error())
}

func TestBaseNode_Name(t *testing.T) {
	node := NewBaseNode(models.DAGNodeColumnEnrichment, &mockBaseNodeDAGRepo{}, zap.NewNop())
	assert.Equal(t, models.DAGNodeColumnEnrichment, node.Name())
}

func TestNewExecutionContext(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	nodeID := uuid.New()

	dag := &models.OntologyDAG{
		ID:           dagID,
		ProjectID:    projectID,
		DatasourceID: datasourceID,
	}

	dagNode := &models.DAGNode{
		ID: nodeID,
	}

	execCtx := NewExecutionContext(dag, dagNode)

	assert.Equal(t, dag, execCtx.DAG)
	assert.Equal(t, nodeID, execCtx.NodeID)
	assert.Equal(t, projectID, execCtx.ProjectID)
	assert.Equal(t, datasourceID, execCtx.DatasourceID)
}
