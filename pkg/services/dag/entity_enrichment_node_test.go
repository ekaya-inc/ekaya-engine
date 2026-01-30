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

// mockEntityEnrichmentMethods implements EntityEnrichmentMethods for testing.
type mockEntityEnrichmentMethods struct {
	enrichFunc func(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID, progressCallback ProgressCallback) error
}

func (m *mockEntityEnrichmentMethods) EnrichEntitiesWithLLM(ctx context.Context, projectID, ontologyID, datasourceID uuid.UUID, progressCallback ProgressCallback) error {
	if m.enrichFunc != nil {
		return m.enrichFunc(ctx, projectID, ontologyID, datasourceID, progressCallback)
	}
	return nil
}

// mockEntityEnrichmentDAGRepo implements OntologyDAGRepository for testing.
type mockEntityEnrichmentDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockEntityEnrichmentDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) GetByID(ctx context.Context, dagID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) GetByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) UpdateCurrentNode(ctx context.Context, dagID uuid.UUID, nodeName models.DAGNodeName) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) CompleteDAG(ctx context.Context, dagID uuid.UUID) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) FailDAG(ctx context.Context, dagID uuid.UUID, errorMsg string) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) GetNodeByName(ctx context.Context, dagID uuid.UUID, nodeName models.DAGNodeName) (*models.DAGNode, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) CreateNode(ctx context.Context, node *models.DAGNode) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMsg *string) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) CompleteNode(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) FailNode(ctx context.Context, nodeID uuid.UUID, errorMsg string) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}

func (m *mockEntityEnrichmentDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}

func (m *mockEntityEnrichmentDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}

func (m *mockEntityEnrichmentDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

func TestEntityEnrichmentNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	nodeID := uuid.New()

	// Track progress reports
	var progressReports []string

	mockEntity := &mockEntityEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID, oID, dID uuid.UUID, progressCallback ProgressCallback) error {
			assert.Equal(t, projectID, pID)
			assert.Equal(t, ontologyID, oID)
			assert.Equal(t, datasourceID, dID)
			// Simulate progress reporting
			if progressCallback != nil {
				progressCallback(5, 10, "Enriching entities...")
			}
			return nil
		},
	}

	mockRepo := &mockEntityEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewEntityEnrichmentNode(mockRepo, mockEntity, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		OntologyID:   &ontologyID,
		DatasourceID: datasourceID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify progress was reported (initial, intermediate from enrichment, completion)
	assert.GreaterOrEqual(t, len(progressReports), 2, "Should report at least initial and completion progress")
	assert.Equal(t, "Generating entity names and descriptions...", progressReports[0])
	assert.Equal(t, "Entity enrichment complete", progressReports[len(progressReports)-1])
}

func TestEntityEnrichmentNode_Execute_NoOntologyID(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	datasourceID := uuid.New()

	mockEntity := &mockEntityEnrichmentMethods{}
	mockRepo := &mockEntityEnrichmentDAGRepo{}

	node := NewEntityEnrichmentNode(mockRepo, mockEntity, zap.NewNop())

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		OntologyID:   nil, // Missing ontology ID
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ontology ID is required")
}

func TestEntityEnrichmentNode_Execute_EnrichmentError_FailsFast(t *testing.T) {
	// Entity enrichment errors should now fail the execution (fail-fast behavior)
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	nodeID := uuid.New()

	expectedErr := errors.New("enrichment failed")

	// Track progress reports
	var progressReports []string

	mockEntity := &mockEntityEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID, oID, dID uuid.UUID, progressCallback ProgressCallback) error {
			return expectedErr
		},
	}

	mockRepo := &mockEntityEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewEntityEnrichmentNode(mockRepo, mockEntity, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		OntologyID:   &ontologyID,
		DatasourceID: datasourceID,
	}

	// Should fail due to enrichment error (fail-fast behavior)
	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity enrichment failed")
	assert.ErrorIs(t, err, expectedErr)

	// Only initial progress should be reported (completion not reached)
	assert.Len(t, progressReports, 1, "Should only report initial progress before failure")
	assert.Equal(t, "Generating entity names and descriptions...", progressReports[0])
}

func TestEntityEnrichmentNode_Execute_ProgressReportingError(t *testing.T) {
	// Progress reporting errors should not fail the execution
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	nodeID := uuid.New()

	mockEntity := &mockEntityEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID, oID, dID uuid.UUID, progressCallback ProgressCallback) error {
			return nil
		},
	}

	mockRepo := &mockEntityEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			return errors.New("progress update failed")
		},
	}

	node := NewEntityEnrichmentNode(mockRepo, mockEntity, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		OntologyID:   &ontologyID,
		DatasourceID: datasourceID,
	}

	// Should succeed despite progress reporting errors
	err := node.Execute(ctx, dag)
	require.NoError(t, err)
}

func TestEntityEnrichmentNode_Execute_ProgressCallbackInvoked(t *testing.T) {
	// Verify that progress callback passed to enrichment is properly wired to ReportProgress
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	nodeID := uuid.New()

	// Track progress values reported
	var progressValues []struct {
		current int
		total   int
		message string
	}

	mockEntity := &mockEntityEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID, oID, dID uuid.UUID, progressCallback ProgressCallback) error {
			// Simulate batch progress reporting
			if progressCallback != nil {
				progressCallback(0, 50, "Starting enrichment...")
				progressCallback(25, 50, "Enriching entities (25/50)...")
				progressCallback(50, 50, "Enriched 50 entities")
			}
			return nil
		},
	}

	mockRepo := &mockEntityEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			progressValues = append(progressValues, struct {
				current int
				total   int
				message string
			}{progress.Current, progress.Total, progress.Message})
			return nil
		},
	}

	node := NewEntityEnrichmentNode(mockRepo, mockEntity, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:    projectID,
		OntologyID:   &ontologyID,
		DatasourceID: datasourceID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Should have: initial (0/100), then service progress reports, then completion (100/100)
	require.GreaterOrEqual(t, len(progressValues), 4, "Should have initial, service progress, and completion reports")

	// First should be initial progress from node
	assert.Equal(t, "Generating entity names and descriptions...", progressValues[0].message)

	// Last should be completion from node
	lastProgress := progressValues[len(progressValues)-1]
	assert.Equal(t, 100, lastProgress.current)
	assert.Equal(t, 100, lastProgress.total)
	assert.Equal(t, "Entity enrichment complete", lastProgress.message)

	// Middle ones should include service progress
	foundServiceProgress := false
	for _, p := range progressValues[1 : len(progressValues)-1] {
		if p.total == 50 {
			foundServiceProgress = true
			break
		}
	}
	assert.True(t, foundServiceProgress, "Should include progress reports from the enrichment service")
}

func TestEntityEnrichmentNode_Name(t *testing.T) {
	mockRepo := &mockEntityEnrichmentDAGRepo{}
	mockEntity := &mockEntityEnrichmentMethods{}

	node := NewEntityEnrichmentNode(mockRepo, mockEntity, zap.NewNop())

	assert.Equal(t, models.DAGNodeEntityEnrichment, node.Name())
}
