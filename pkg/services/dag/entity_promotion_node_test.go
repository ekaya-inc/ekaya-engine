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

// mockEntityPromotionMethods implements EntityPromotionMethods for testing.
type mockEntityPromotionMethods struct {
	scoreFunc func(ctx context.Context, projectID uuid.UUID) (promoted int, demoted int, err error)
}

func (m *mockEntityPromotionMethods) ScoreAndPromoteEntities(ctx context.Context, projectID uuid.UUID) (promoted int, demoted int, err error) {
	if m.scoreFunc != nil {
		return m.scoreFunc(ctx, projectID)
	}
	return 0, 0, nil
}

// mockEntityPromotionDAGRepo implements OntologyDAGRepository for testing.
type mockEntityPromotionDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockEntityPromotionDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockEntityPromotionDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) GetByID(ctx context.Context, dagID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) GetByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) UpdateCurrentNode(ctx context.Context, dagID uuid.UUID, nodeName models.DAGNodeName) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) CompleteDAG(ctx context.Context, dagID uuid.UUID) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) FailDAG(ctx context.Context, dagID uuid.UUID, errorMsg string) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) GetNodeByName(ctx context.Context, dagID uuid.UUID, nodeName models.DAGNodeName) (*models.DAGNode, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) CreateNode(ctx context.Context, node *models.DAGNode) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMsg *string) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) CompleteNode(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) FailNode(ctx context.Context, nodeID uuid.UUID, errorMsg string) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}

func (m *mockEntityPromotionDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}

func (m *mockEntityPromotionDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}

func (m *mockEntityPromotionDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

func TestEntityPromotionNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	// Track progress reports
	var progressReports []string

	mockPromotion := &mockEntityPromotionMethods{
		scoreFunc: func(ctx context.Context, pID uuid.UUID) (int, int, error) {
			assert.Equal(t, projectID, pID)
			return 5, 3, nil // 5 promoted, 3 demoted
		},
	}

	mockRepo := &mockEntityPromotionDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewEntityPromotionNode(mockRepo, mockPromotion, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify progress was reported
	assert.Len(t, progressReports, 2, "Should report initial and completion progress")
	assert.Equal(t, "Evaluating entity promotion scores...", progressReports[0])
	assert.Equal(t, "Entity promotion complete: 5 promoted, 3 demoted", progressReports[1])
}

func TestEntityPromotionNode_Execute_NoServiceConfigured(t *testing.T) {
	// When no service is configured, node should operate in no-op mode
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockRepo := &mockEntityPromotionDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	// Pass nil for entityPromotionSvc
	node := NewEntityPromotionNode(mockRepo, nil, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify progress shows skipped message
	assert.Len(t, progressReports, 2)
	assert.Equal(t, "Evaluating entity promotion scores...", progressReports[0])
	assert.Equal(t, "Entity promotion skipped (no service configured)", progressReports[1])
}

func TestEntityPromotionNode_Execute_PromotionError(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	expectedErr := errors.New("promotion failed")

	mockPromotion := &mockEntityPromotionMethods{
		scoreFunc: func(ctx context.Context, pID uuid.UUID) (int, int, error) {
			return 0, 0, expectedErr
		},
	}

	mockRepo := &mockEntityPromotionDAGRepo{}

	node := NewEntityPromotionNode(mockRepo, mockPromotion, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "score and promote entities")
	assert.Contains(t, err.Error(), "promotion failed")
}

func TestEntityPromotionNode_Execute_ProgressReportingError(t *testing.T) {
	// Progress reporting errors should not fail the execution
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	mockPromotion := &mockEntityPromotionMethods{
		scoreFunc: func(ctx context.Context, pID uuid.UUID) (int, int, error) {
			return 3, 2, nil
		},
	}

	mockRepo := &mockEntityPromotionDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			return errors.New("progress update failed")
		},
	}

	node := NewEntityPromotionNode(mockRepo, mockPromotion, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	// Should succeed despite progress reporting errors
	err := node.Execute(ctx, dag)
	require.NoError(t, err)
}

func TestEntityPromotionNode_Name(t *testing.T) {
	mockRepo := &mockEntityPromotionDAGRepo{}
	mockPromotion := &mockEntityPromotionMethods{}

	node := NewEntityPromotionNode(mockRepo, mockPromotion, zap.NewNop())

	assert.Equal(t, models.DAGNodeEntityPromotion, node.Name())
}

func TestEntityPromotionNode_Execute_AllDemoted(t *testing.T) {
	// Test case where all entities are demoted (0 promoted)
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockPromotion := &mockEntityPromotionMethods{
		scoreFunc: func(ctx context.Context, pID uuid.UUID) (int, int, error) {
			return 0, 10, nil // All demoted
		},
	}

	mockRepo := &mockEntityPromotionDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewEntityPromotionNode(mockRepo, mockPromotion, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	assert.Equal(t, "Entity promotion complete: 0 promoted, 10 demoted", progressReports[1])
}

func TestEntityPromotionNode_Execute_AllPromoted(t *testing.T) {
	// Test case where all entities are promoted (0 demoted)
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockPromotion := &mockEntityPromotionMethods{
		scoreFunc: func(ctx context.Context, pID uuid.UUID) (int, int, error) {
			return 8, 0, nil // All promoted
		},
	}

	mockRepo := &mockEntityPromotionDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewEntityPromotionNode(mockRepo, mockPromotion, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	assert.Equal(t, "Entity promotion complete: 8 promoted, 0 demoted", progressReports[1])
}
