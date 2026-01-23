package dag

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockKnowledgeDAGRepo is a minimal mock for the DAG repository.
type mockKnowledgeDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockKnowledgeDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

// Implement other required interface methods with stubs
func (m *mockKnowledgeDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockKnowledgeDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockKnowledgeDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockKnowledgeDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockKnowledgeDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockKnowledgeDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockKnowledgeDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockKnowledgeDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockKnowledgeDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockKnowledgeDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockKnowledgeDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

func TestKnowledgeSeedingNode_Execute_NoOp(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockKnowledgeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	node := NewKnowledgeSeedingNode(mockRepo, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)

	// Verify progress message indicates no-op behavior
	assert.Contains(t, progressMessages, "Knowledge seeding complete (inference-based)")
}

func TestKnowledgeSeedingNode_Name(t *testing.T) {
	node := NewKnowledgeSeedingNode(&mockKnowledgeDAGRepo{}, zap.NewNop())
	assert.Equal(t, models.DAGNodeKnowledgeSeeding, node.Name())
}
