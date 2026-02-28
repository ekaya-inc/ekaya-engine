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

// mockOntologyFinalizationDAGRepo is a minimal mock for the DAG repository.
type mockOntologyFinalizationDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockOntologyFinalizationDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockOntologyFinalizationDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockOntologyFinalizationDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockOntologyFinalizationDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockOntologyFinalizationDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockOntologyFinalizationDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockOntologyFinalizationDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockOntologyFinalizationDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockOntologyFinalizationDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockOntologyFinalizationDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockOntologyFinalizationDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// mockOntologyFinalizationMethods implements OntologyFinalizationMethods for testing.
type mockOntologyFinalizationMethods struct {
	finalizeFunc func(ctx context.Context, projectID uuid.UUID) error
}

func (m *mockOntologyFinalizationMethods) Finalize(ctx context.Context, projectID uuid.UUID) error {
	if m.finalizeFunc != nil {
		return m.finalizeFunc(ctx, projectID)
	}
	return nil
}

func TestOntologyFinalizationNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockSvc := &mockOntologyFinalizationMethods{
		finalizeFunc: func(ctx context.Context, pID uuid.UUID) error {
			assert.Equal(t, projectID, pID)
			return nil
		},
	}

	mockRepo := &mockOntologyFinalizationDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewOntologyFinalizationNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	require.Len(t, progressReports, 2)
	assert.Equal(t, "Generating domain summary...", progressReports[0])
	assert.Equal(t, "Ontology finalization complete", progressReports[1])
}

func TestOntologyFinalizationNode_Execute_ServiceError(t *testing.T) {
	ctx := context.Background()
	nodeID := uuid.New()

	mockSvc := &mockOntologyFinalizationMethods{
		finalizeFunc: func(ctx context.Context, pID uuid.UUID) error {
			return errors.New("finalization failed")
		},
	}

	mockRepo := &mockOntologyFinalizationDAGRepo{}

	node := NewOntologyFinalizationNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: uuid.New(),
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "finalize ontology")
	assert.Contains(t, err.Error(), "finalization failed")
}

func TestOntologyFinalizationNode_Execute_ProgressReportingError(t *testing.T) {
	ctx := context.Background()
	nodeID := uuid.New()

	mockSvc := &mockOntologyFinalizationMethods{
		finalizeFunc: func(ctx context.Context, pID uuid.UUID) error {
			return nil
		},
	}

	mockRepo := &mockOntologyFinalizationDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			return errors.New("progress update failed")
		},
	}

	node := NewOntologyFinalizationNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: uuid.New(),
	}

	// Should succeed despite progress reporting errors
	err := node.Execute(ctx, dag)
	require.NoError(t, err)
}

func TestOntologyFinalizationNode_Name(t *testing.T) {
	node := NewOntologyFinalizationNode(&mockOntologyFinalizationDAGRepo{}, &mockOntologyFinalizationMethods{}, zap.NewNop())
	assert.Equal(t, models.DAGNodeOntologyFinalization, node.Name())
}
