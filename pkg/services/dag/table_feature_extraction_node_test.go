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

// mockTableFeatureDAGRepo is a minimal mock for the DAG repository.
type mockTableFeatureDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockTableFeatureDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

// Implement other required interface methods with stubs
func (m *mockTableFeatureDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockTableFeatureDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockTableFeatureDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockTableFeatureDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockTableFeatureDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockTableFeatureDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockTableFeatureDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockTableFeatureDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockTableFeatureDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockTableFeatureDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockTableFeatureDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// mockTableFeatureExtractionMethods implements TableFeatureExtractionMethods for testing.
type mockTableFeatureExtractionMethods struct {
	extractResult          int
	extractErr             error
	progressCallbackCalled bool
	capturedProgressCalls  []progressCall
}

type progressCall struct {
	current int
	total   int
	message string
}

func (m *mockTableFeatureExtractionMethods) ExtractTableFeatures(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (int, error) {
	// Simulate progress callbacks if the progressCallback is provided
	if progressCallback != nil {
		m.progressCallbackCalled = true
		// Simulate processing 3 tables
		progressCallback(1, 3, "Analyzing table users (1/3)")
		progressCallback(2, 3, "Analyzing table orders (2/3)")
		progressCallback(3, 3, "Analyzing table products (3/3)")
		m.capturedProgressCalls = append(m.capturedProgressCalls,
			progressCall{1, 3, "Analyzing table users (1/3)"},
			progressCall{2, 3, "Analyzing table orders (2/3)"},
			progressCall{3, 3, "Analyzing table products (3/3)"},
		)
	}
	return m.extractResult, m.extractErr
}

func TestTableFeatureExtractionNode_Execute_NoOp(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockTableFeatureDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	// Pass nil for methods to test no-op mode
	node := NewTableFeatureExtractionNode(mockRepo, nil, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)

	// Verify progress message indicates no-op behavior
	assert.Contains(t, progressMessages, "Table feature extraction skipped (not configured)")
}

func TestTableFeatureExtractionNode_Execute_WithExtraction(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockTableFeatureDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	// Mock that returns 10 tables processed
	mockMethods := &mockTableFeatureExtractionMethods{
		extractResult: 10,
		extractErr:    nil,
	}

	node := NewTableFeatureExtractionNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)

	// Verify initial and final progress messages
	assert.Contains(t, progressMessages, "Analyzing tables...")
	assert.Contains(t, progressMessages, "Table feature extraction complete")

	// Verify the progress callback was called
	assert.True(t, mockMethods.progressCallbackCalled)
}

func TestTableFeatureExtractionNode_Execute_ProgressCallback(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockTableFeatureDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	mockMethods := &mockTableFeatureExtractionMethods{
		extractResult: 3,
		extractErr:    nil,
	}

	node := NewTableFeatureExtractionNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)

	// Verify granular progress messages were reported (per-table updates)
	assert.Contains(t, progressMessages, "Analyzing table users (1/3)")
	assert.Contains(t, progressMessages, "Analyzing table orders (2/3)")
	assert.Contains(t, progressMessages, "Analyzing table products (3/3)")
}

func TestTableFeatureExtractionNode_Execute_ExtractionError(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockTableFeatureDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	// Mock that returns an error
	mockMethods := &mockTableFeatureExtractionMethods{
		extractResult: 0,
		extractErr:    errors.New("LLM unavailable"),
	}

	node := NewTableFeatureExtractionNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	// Should fail - table feature extraction errors should propagate
	err := node.Execute(context.Background(), dag)
	assert.Error(t, err)
	assert.Equal(t, "LLM unavailable", err.Error())
}

func TestTableFeatureExtractionNode_Execute_NoTablesProcessed(t *testing.T) {
	progressMessages := make([]string, 0)
	mockRepo := &mockTableFeatureDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressMessages = append(progressMessages, progress.Message)
			return nil
		},
	}

	// Mock that returns 0 tables (empty schema or all tables already processed)
	mockMethods := &mockTableFeatureExtractionMethods{
		extractResult: 0,
		extractErr:    nil,
	}

	node := NewTableFeatureExtractionNode(mockRepo, mockMethods, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ID:           uuid.New(),
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	assert.NoError(t, err)

	// Should still complete successfully
	assert.Contains(t, progressMessages, "Analyzing tables...")
	assert.Contains(t, progressMessages, "Table feature extraction complete")
}

func TestTableFeatureExtractionNode_Name(t *testing.T) {
	node := NewTableFeatureExtractionNode(&mockTableFeatureDAGRepo{}, nil, zap.NewNop())
	assert.Equal(t, models.DAGNodeTableFeatureExtraction, node.Name())
}
