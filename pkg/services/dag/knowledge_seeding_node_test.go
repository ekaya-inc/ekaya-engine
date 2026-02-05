package dag

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
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

// mockKnowledgeSeedingMethods implements KnowledgeSeedingMethods for testing.
type mockKnowledgeSeedingMethods struct {
	extractResult int
	extractErr    error
}

func (m *mockKnowledgeSeedingMethods) ExtractKnowledgeFromOverview(ctx context.Context, projectID, datasourceID uuid.UUID) (int, error) {
	return m.extractResult, m.extractErr
}

func TestKnowledgeSeedingNode_Execute_NoOp(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockKnowledgeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	// Pass nil for knowledgeSeedingMethods to test no-op mode
	node := NewKnowledgeSeedingNode(mockRepo, nil, zap.NewNop())
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

func TestKnowledgeSeedingNode_Execute_WithExtraction(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockKnowledgeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	// Mock that returns 3 facts extracted
	mockMethods := &mockKnowledgeSeedingMethods{
		extractResult: 3,
		extractErr:    nil,
	}

	node := NewKnowledgeSeedingNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)

	// Verify progress message indicates facts were extracted
	assert.Contains(t, progressMessages, "Extracted 3 domain facts")
}

func TestKnowledgeSeedingNode_Execute_NoFactsExtracted(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockKnowledgeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	// Mock that returns 0 facts (no overview provided)
	mockMethods := &mockKnowledgeSeedingMethods{
		extractResult: 0,
		extractErr:    nil,
	}

	node := NewKnowledgeSeedingNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)

	// Verify progress message indicates no facts
	assert.Contains(t, progressMessages, "No knowledge facts extracted")
}

func TestKnowledgeSeedingNode_Execute_ExtractionError(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockKnowledgeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	// Mock that returns an error - should log but not fail the node
	mockMethods := &mockKnowledgeSeedingMethods{
		extractResult: 0,
		extractErr:    errors.New("LLM unavailable"),
	}

	node := NewKnowledgeSeedingNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	// Should NOT fail - knowledge seeding errors are logged but don't block the pipeline
	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)

	// Verify progress message indicates no facts (graceful degradation)
	assert.Contains(t, progressMessages, "No knowledge facts extracted")
}

func TestKnowledgeSeedingNode_Execute_EndpointError_Propagates(t *testing.T) {
	// Endpoint errors (like connection refused) should propagate to fail the DAG
	// because they indicate LLM configuration problems that will affect ALL nodes.
	mockRepo := &mockKnowledgeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			return nil
		},
	}

	// Mock that returns an endpoint error (connection refused)
	endpointErr := llm.NewError(llm.ErrorTypeEndpoint, "connection failed", true, errors.New("dial tcp: connection refused"))
	mockMethods := &mockKnowledgeSeedingMethods{
		extractResult: 0,
		extractErr:    endpointErr,
	}

	node := NewKnowledgeSeedingNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	// Should FAIL - endpoint errors must propagate to show user the configuration problem
	err := node.Execute(context.Background(), dag)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM configuration error")
}

func TestKnowledgeSeedingNode_Execute_AuthError_Propagates(t *testing.T) {
	// Auth errors should propagate to fail the DAG
	// because they indicate LLM configuration problems that will affect ALL nodes.
	mockRepo := &mockKnowledgeDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			return nil
		},
	}

	// Mock that returns an auth error (invalid API key)
	authErr := llm.NewError(llm.ErrorTypeAuth, "invalid API key", false, errors.New("401 Unauthorized"))
	mockMethods := &mockKnowledgeSeedingMethods{
		extractResult: 0,
		extractErr:    authErr,
	}

	node := NewKnowledgeSeedingNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	// Should FAIL - auth errors must propagate
	err := node.Execute(context.Background(), dag)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM configuration error")
}

func TestKnowledgeSeedingNode_Name(t *testing.T) {
	node := NewKnowledgeSeedingNode(&mockKnowledgeDAGRepo{}, nil, zap.NewNop())
	assert.Equal(t, models.DAGNodeKnowledgeSeeding, node.Name())
}
