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

// mockColumnFeatureDAGRepo is a minimal mock for the DAG repository.
type mockColumnFeatureDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockColumnFeatureDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockColumnFeatureDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnFeatureDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnFeatureDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnFeatureDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnFeatureDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnFeatureDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnFeatureDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockColumnFeatureDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockColumnFeatureDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockColumnFeatureDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockColumnFeatureDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// mockColumnFeatureExtractionMethods implements ColumnFeatureExtractionMethods for testing.
type mockColumnFeatureExtractionMethods struct {
	extractFunc func(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (int, error)
}

func (m *mockColumnFeatureExtractionMethods) ExtractColumnFeatures(ctx context.Context, projectID, datasourceID uuid.UUID, progressCallback ProgressCallback) (int, error) {
	if m.extractFunc != nil {
		return m.extractFunc(ctx, projectID, datasourceID, progressCallback)
	}
	return 0, nil
}

func TestColumnFeatureExtractionNode_Execute_NilMethods(t *testing.T) {
	var progressReports []string
	mockRepo := &mockColumnFeatureDAGRepo{
		updateProgressFunc: func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	// Pass nil for methods to test no-op mode
	node := NewColumnFeatureExtractionNode(mockRepo, nil, zap.NewNop())
	nodeID := uuid.New()
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	require.NoError(t, err)

	assert.Contains(t, progressReports, "Column feature extraction skipped (not configured)")
}

func TestColumnFeatureExtractionNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	datasourceID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockSvc := &mockColumnFeatureExtractionMethods{
		extractFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (int, error) {
			assert.Equal(t, projectID, pID)
			assert.Equal(t, datasourceID, dsID)
			return 25, nil
		},
	}

	mockRepo := &mockColumnFeatureDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewColumnFeatureExtractionNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	assert.Contains(t, progressReports, "Extracting column features...")
	assert.Contains(t, progressReports, "Column feature extraction complete")
}

func TestColumnFeatureExtractionNode_Execute_ServiceError(t *testing.T) {
	ctx := context.Background()
	nodeID := uuid.New()

	mockSvc := &mockColumnFeatureExtractionMethods{
		extractFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (int, error) {
			return 0, errors.New("query execution failed")
		},
	}

	mockRepo := &mockColumnFeatureDAGRepo{}

	node := NewColumnFeatureExtractionNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Equal(t, "query execution failed", err.Error())
}

func TestColumnFeatureExtractionNode_Execute_ProgressCallback(t *testing.T) {
	var progressReports []string

	mockSvc := &mockColumnFeatureExtractionMethods{
		extractFunc: func(ctx context.Context, pID, dsID uuid.UUID, progressCallback ProgressCallback) (int, error) {
			progressCallback(1, 5, "Processing column 1/5")
			progressCallback(5, 5, "Processing column 5/5")
			return 5, nil
		},
	}

	mockRepo := &mockColumnFeatureDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewColumnFeatureExtractionNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(uuid.New())

	dag := &models.OntologyDAG{
		ProjectID:    uuid.New(),
		DatasourceID: uuid.New(),
	}

	err := node.Execute(context.Background(), dag)
	require.NoError(t, err)

	assert.Contains(t, progressReports, "Processing column 1/5")
	assert.Contains(t, progressReports, "Processing column 5/5")
}

func TestColumnFeatureExtractionNode_Name(t *testing.T) {
	node := NewColumnFeatureExtractionNode(&mockColumnFeatureDAGRepo{}, nil, zap.NewNop())
	assert.Equal(t, models.DAGNodeColumnFeatureExtraction, node.Name())
}
