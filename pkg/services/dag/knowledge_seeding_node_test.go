package dag

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockKnowledgeSeedingMethods is a mock implementation for testing.
type mockKnowledgeSeedingMethods struct {
	seedFunc func(ctx context.Context, projectID uuid.UUID) (int, error)
}

func (m *mockKnowledgeSeedingMethods) SeedKnowledgeFromFile(ctx context.Context, projectID uuid.UUID) (int, error) {
	if m.seedFunc != nil {
		return m.seedFunc(ctx, projectID)
	}
	return 0, nil
}

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

func TestKnowledgeSeedingNode_Execute_Success_NoSeedPath(t *testing.T) {
	mockMethods := &mockKnowledgeSeedingMethods{
		seedFunc: func(ctx context.Context, projectID uuid.UUID) (int, error) {
			return 0, nil // No seed path configured
		},
	}

	node := NewKnowledgeSeedingNode(&mockKnowledgeDAGRepo{}, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)
}

func TestKnowledgeSeedingNode_Execute_Success_WithFacts(t *testing.T) {
	calledWithProjectID := uuid.Nil
	mockMethods := &mockKnowledgeSeedingMethods{
		seedFunc: func(ctx context.Context, projectID uuid.UUID) (int, error) {
			calledWithProjectID = projectID
			return 5, nil // Seeded 5 facts
		},
	}

	progressMessages := make([]string, 0)
	mockRepo := &mockKnowledgeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	node := NewKnowledgeSeedingNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	projectID := uuid.New()
	dag := &models.OntologyDAG{
		ID:        uuid.New(),
		ProjectID: projectID,
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)
	assert.Equal(t, projectID, calledWithProjectID)

	// Verify progress messages
	assert.Contains(t, progressMessages, "Loading project knowledge...")
	assert.Contains(t, progressMessages, "Seeded 5 knowledge facts")
}

func TestKnowledgeSeedingNode_Execute_Error(t *testing.T) {
	expectedErr := errors.New("failed to read seed file")
	mockMethods := &mockKnowledgeSeedingMethods{
		seedFunc: func(ctx context.Context, projectID uuid.UUID) (int, error) {
			return 0, expectedErr
		},
	}

	node := NewKnowledgeSeedingNode(&mockKnowledgeDAGRepo{}, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "seed knowledge from file")
}

func TestKnowledgeSeedingNode_Name(t *testing.T) {
	node := NewKnowledgeSeedingNode(&mockKnowledgeDAGRepo{}, &mockKnowledgeSeedingMethods{}, zap.NewNop())
	assert.Equal(t, models.DAGNodeKnowledgeSeeding, node.Name())
}
