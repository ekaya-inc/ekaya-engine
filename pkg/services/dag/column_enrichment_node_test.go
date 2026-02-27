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

// mockColumnEnrichmentDAGRepo is a minimal mock for the DAG repository.
type mockColumnEnrichmentDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockColumnEnrichmentDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockColumnEnrichmentDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnEnrichmentDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnEnrichmentDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnEnrichmentDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnEnrichmentDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnEnrichmentDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockColumnEnrichmentDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) Delete(ctx context.Context, id uuid.UUID) error { return nil }
func (m *mockColumnEnrichmentDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockColumnEnrichmentDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}
func (m *mockColumnEnrichmentDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockColumnEnrichmentDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// mockColumnEnrichmentMethods implements ColumnEnrichmentMethods for testing.
type mockColumnEnrichmentMethods struct {
	enrichFunc func(ctx context.Context, projectID uuid.UUID, tableNames []string, progressCallback ProgressCallback) (*ColumnEnrichmentResult, error)
}

func (m *mockColumnEnrichmentMethods) EnrichProject(ctx context.Context, projectID uuid.UUID, tableNames []string, progressCallback ProgressCallback) (*ColumnEnrichmentResult, error) {
	if m.enrichFunc != nil {
		return m.enrichFunc(ctx, projectID, tableNames, progressCallback)
	}
	return &ColumnEnrichmentResult{}, nil
}

func TestColumnEnrichmentNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockSvc := &mockColumnEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID uuid.UUID, tableNames []string, progressCallback ProgressCallback) (*ColumnEnrichmentResult, error) {
			assert.Equal(t, projectID, pID)
			assert.Nil(t, tableNames, "should pass nil to enrich all tables")
			return &ColumnEnrichmentResult{
				TablesEnriched: []string{"users", "orders", "products"},
				TablesFailed:   map[string]string{},
				DurationMs:     1500,
			}, nil
		},
	}

	mockRepo := &mockColumnEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewColumnEnrichmentNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify progress was reported
	require.Len(t, progressReports, 2, "Should report initial and completion progress")
	assert.Equal(t, "Enriching column metadata...", progressReports[0])
	assert.Equal(t, "Enriched 3 tables", progressReports[1])
}

func TestColumnEnrichmentNode_Execute_WithFailedTables(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockSvc := &mockColumnEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID uuid.UUID, tableNames []string, progressCallback ProgressCallback) (*ColumnEnrichmentResult, error) {
			return &ColumnEnrichmentResult{
				TablesEnriched: []string{"users", "orders"},
				TablesFailed:   map[string]string{"products": "timeout"},
				DurationMs:     2000,
			}, nil
		},
	}

	mockRepo := &mockColumnEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewColumnEnrichmentNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify completion message includes failure count
	require.Len(t, progressReports, 2)
	assert.Equal(t, "Enriched 2 tables (1 failed)", progressReports[1])
}

func TestColumnEnrichmentNode_Execute_ServiceError(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	mockSvc := &mockColumnEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID uuid.UUID, tableNames []string, progressCallback ProgressCallback) (*ColumnEnrichmentResult, error) {
			return nil, errors.New("LLM unavailable")
		},
	}

	mockRepo := &mockColumnEnrichmentDAGRepo{}

	node := NewColumnEnrichmentNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enrich columns")
	assert.Contains(t, err.Error(), "LLM unavailable")
}

func TestColumnEnrichmentNode_Execute_ProgressCallback(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	nodeID := uuid.New()

	var progressReports []string

	mockSvc := &mockColumnEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID uuid.UUID, tableNames []string, progressCallback ProgressCallback) (*ColumnEnrichmentResult, error) {
			// Simulate progress callbacks from service
			progressCallback(1, 3, "Enriching table users (1/3)")
			progressCallback(2, 3, "Enriching table orders (2/3)")
			progressCallback(3, 3, "Enriching table products (3/3)")
			return &ColumnEnrichmentResult{
				TablesEnriched: []string{"users", "orders", "products"},
				TablesFailed:   map[string]string{},
			}, nil
		},
	}

	mockRepo := &mockColumnEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewColumnEnrichmentNode(mockRepo, mockSvc, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID: projectID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify granular progress messages were reported
	assert.Contains(t, progressReports, "Enriching table users (1/3)")
	assert.Contains(t, progressReports, "Enriching table orders (2/3)")
	assert.Contains(t, progressReports, "Enriching table products (3/3)")
}

func TestColumnEnrichmentNode_Name(t *testing.T) {
	node := NewColumnEnrichmentNode(&mockColumnEnrichmentDAGRepo{}, &mockColumnEnrichmentMethods{}, zap.NewNop())
	assert.Equal(t, models.DAGNodeColumnEnrichment, node.Name())
}
