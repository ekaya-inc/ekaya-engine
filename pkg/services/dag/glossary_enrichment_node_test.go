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

// mockGlossaryEnrichmentMethods implements GlossaryEnrichmentMethods for testing.
type mockGlossaryEnrichmentMethods struct {
	enrichFunc func(ctx context.Context, projectID, ontologyID uuid.UUID) error
}

func (m *mockGlossaryEnrichmentMethods) EnrichGlossaryTerms(ctx context.Context, projectID, ontologyID uuid.UUID) error {
	if m.enrichFunc != nil {
		return m.enrichFunc(ctx, projectID, ontologyID)
	}
	return nil
}

// mockDAGRepository implements OntologyDAGRepository for testing.
type mockGlossaryEnrichmentDAGRepo struct {
	updateProgressFunc func(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error
}

func (m *mockGlossaryEnrichmentDAGRepo) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	if m.updateProgressFunc != nil {
		return m.updateProgressFunc(ctx, nodeID, progress)
	}
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) Create(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetByID(ctx context.Context, dagID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) UpdateCurrentNode(ctx context.Context, dagID uuid.UUID, nodeName models.DAGNodeName) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) CompleteDAG(ctx context.Context, dagID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) FailDAG(ctx context.Context, dagID uuid.UUID, errorMsg string) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetNodeByName(ctx context.Context, dagID uuid.UUID, nodeName models.DAGNodeName) (*models.DAGNode, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) CreateNode(ctx context.Context, node *models.DAGNode) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMsg *string) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) CompleteNode(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) FailNode(ctx context.Context, nodeID uuid.UUID, errorMsg string) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) Update(ctx context.Context, dag *models.OntologyDAG) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	return nil, nil
}

func (m *mockGlossaryEnrichmentDAGRepo) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}

func (m *mockGlossaryEnrichmentDAGRepo) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

func TestGlossaryEnrichmentNode_Execute_Success(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	nodeID := uuid.New()

	// Track progress reports
	var progressReports []string

	mockGlossary := &mockGlossaryEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID, oID uuid.UUID) error {
			assert.Equal(t, projectID, pID)
			assert.Equal(t, ontologyID, oID)
			return nil
		},
	}

	mockRepo := &mockGlossaryEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewGlossaryEnrichmentNode(mockRepo, mockGlossary, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:  projectID,
		OntologyID: &ontologyID,
	}

	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify progress was reported
	assert.Len(t, progressReports, 2, "Should report initial and completion progress")
	assert.Equal(t, "Generating SQL definitions for glossary terms...", progressReports[0])
	assert.Equal(t, "Glossary enrichment complete", progressReports[1])
}

func TestGlossaryEnrichmentNode_Execute_NoOntologyID(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	mockGlossary := &mockGlossaryEnrichmentMethods{}
	mockRepo := &mockGlossaryEnrichmentDAGRepo{}

	node := NewGlossaryEnrichmentNode(mockRepo, mockGlossary, zap.NewNop())

	dag := &models.OntologyDAG{
		ProjectID:  projectID,
		OntologyID: nil, // Missing ontology ID
	}

	err := node.Execute(ctx, dag)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ontology ID is required")
}

func TestGlossaryEnrichmentNode_Execute_EnrichmentError(t *testing.T) {
	// Enrichment errors should not fail the execution - should log warning and continue
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	nodeID := uuid.New()

	expectedErr := errors.New("enrichment failed")

	// Track progress reports
	var progressReports []string

	mockGlossary := &mockGlossaryEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID, oID uuid.UUID) error {
			return expectedErr
		},
	}

	mockRepo := &mockGlossaryEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			assert.Equal(t, nodeID, nID)
			progressReports = append(progressReports, progress.Message)
			return nil
		},
	}

	node := NewGlossaryEnrichmentNode(mockRepo, mockGlossary, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:  projectID,
		OntologyID: &ontologyID,
	}

	// Should succeed despite enrichment error (warning logged)
	err := node.Execute(ctx, dag)
	require.NoError(t, err)

	// Verify progress was reported normally
	assert.Len(t, progressReports, 2, "Should report initial and completion progress")
	assert.Equal(t, "Generating SQL definitions for glossary terms...", progressReports[0])
	assert.Equal(t, "Glossary enrichment complete", progressReports[1])
}

func TestGlossaryEnrichmentNode_Execute_ProgressReportingError(t *testing.T) {
	// Progress reporting errors should not fail the execution
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	nodeID := uuid.New()

	mockGlossary := &mockGlossaryEnrichmentMethods{
		enrichFunc: func(ctx context.Context, pID, oID uuid.UUID) error {
			return nil
		},
	}

	mockRepo := &mockGlossaryEnrichmentDAGRepo{
		updateProgressFunc: func(ctx context.Context, nID uuid.UUID, progress *models.DAGNodeProgress) error {
			return errors.New("progress update failed")
		},
	}

	node := NewGlossaryEnrichmentNode(mockRepo, mockGlossary, zap.NewNop())
	node.SetCurrentNodeID(nodeID)

	dag := &models.OntologyDAG{
		ProjectID:  projectID,
		OntologyID: &ontologyID,
	}

	// Should succeed despite progress reporting errors
	err := node.Execute(ctx, dag)
	require.NoError(t, err)
}

func TestGlossaryEnrichmentNode_Name(t *testing.T) {
	mockRepo := &mockGlossaryEnrichmentDAGRepo{}
	mockGlossary := &mockGlossaryEnrichmentMethods{}

	node := NewGlossaryEnrichmentNode(mockRepo, mockGlossary, zap.NewNop())

	assert.Equal(t, models.DAGNodeGlossaryEnrichment, node.Name())
}
