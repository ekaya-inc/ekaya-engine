package services

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// Note: Full integration tests are in pkg/services/ontology_dag_integration_test.go
// These unit tests focus on DAG node creation and executor selection logic.

func TestDAGNodes_AllNodesHaveCorrectOrder(t *testing.T) {
	allNodes := models.AllDAGNodes()

	// NOTE: The current DAG order is designed for schema-first extraction.
	// FKDiscovery and TableFeatureExtraction run BEFORE column enrichment.
	// Glossary nodes (GlossaryDiscovery, GlossaryEnrichment) are not part of
	// the main DAG — they run separately after ontology questions are answered.
	expectedOrder := []models.DAGNodeName{
		models.DAGNodeKnowledgeSeeding,
		models.DAGNodeColumnFeatureExtraction,
		models.DAGNodeFKDiscovery,
		models.DAGNodeTableFeatureExtraction,
		models.DAGNodePKMatchDiscovery,
		models.DAGNodeColumnEnrichment,
		models.DAGNodeOntologyFinalization,
	}

	assert.Equal(t, len(expectedOrder), len(allNodes))

	for i, expected := range expectedOrder {
		assert.Equal(t, expected, allNodes[i])
		assert.Equal(t, models.DAGNodeOrder[expected], models.DAGNodeOrder[allNodes[i]])
	}
}

func TestNodeExecutorInterfaces_AreWellDefined(t *testing.T) {
	// Verify that the interface methods are properly defined
	// by creating implementations that satisfy them

	// FKDiscoveryMethods
	var fkm dag.FKDiscoveryMethods = &testFKDiscovery{}
	assert.NotNil(t, fkm)

	// PKMatchDiscoveryMethods
	var pkm dag.PKMatchDiscoveryMethods = &testPKMatchDiscovery{}
	assert.NotNil(t, pkm)

	// OntologyFinalizationMethods
	var ofm dag.OntologyFinalizationMethods = &testFinalization{}
	assert.NotNil(t, ofm)

	// ColumnEnrichmentMethods
	var cem dag.ColumnEnrichmentMethods = &testColumnEnrichment{}
	assert.NotNil(t, cem)

	// GlossaryDiscoveryMethods
	var gdm dag.GlossaryDiscoveryMethods = &testGlossaryDiscovery{}
	assert.NotNil(t, gdm)

	// ColumnFeatureExtractionMethods
	var cfm dag.ColumnFeatureExtractionMethods = &testColumnFeatureExtraction{}
	assert.NotNil(t, cfm)

	// TableFeatureExtractionMethods
	var tfm dag.TableFeatureExtractionMethods = &testTableFeatureExtraction{}
	assert.NotNil(t, tfm)
}

func TestDAGStatus_ValidStatuses(t *testing.T) {
	validStatuses := []models.DAGStatus{
		models.DAGStatusPending,
		models.DAGStatusRunning,
		models.DAGStatusCompleted,
		models.DAGStatusFailed,
		models.DAGStatusCancelled,
	}

	for _, status := range validStatuses {
		assert.NotEmpty(t, string(status))
	}
}

func TestDAGNodeStatus_ValidStatuses(t *testing.T) {
	validStatuses := []models.DAGNodeStatus{
		models.DAGNodeStatusPending,
		models.DAGNodeStatusRunning,
		models.DAGNodeStatusCompleted,
		models.DAGNodeStatusFailed,
		models.DAGNodeStatusSkipped,
	}

	for _, status := range validStatuses {
		assert.NotEmpty(t, string(status))
	}
}

func TestNewExecutionContext(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ontologyID := uuid.New()
	dagID := uuid.New()
	nodeID := uuid.New()

	dagRecord := &models.OntologyDAG{
		ID:           dagID,
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		OntologyID:   &ontologyID,
	}

	node := &models.DAGNode{
		ID:       nodeID,
		DAGID:    dagID,
		NodeName: string(models.DAGNodeKnowledgeSeeding),
	}

	ctx := dag.NewExecutionContext(dagRecord, node)

	assert.Equal(t, dagRecord, ctx.DAG)
	assert.Equal(t, nodeID, ctx.NodeID)
	assert.Equal(t, projectID, ctx.ProjectID)
	assert.Equal(t, datasourceID, ctx.DatasourceID)
	assert.Equal(t, ontologyID, ctx.OntologyID)
}

func TestNewExecutionContext_NilOntologyID(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	dagID := uuid.New()
	nodeID := uuid.New()

	dagRecord := &models.OntologyDAG{
		ID:           dagID,
		ProjectID:    projectID,
		DatasourceID: datasourceID,
		OntologyID:   nil, // No ontology assigned yet
	}

	node := &models.DAGNode{
		ID:       nodeID,
		DAGID:    dagID,
		NodeName: string(models.DAGNodeKnowledgeSeeding),
	}

	ctx := dag.NewExecutionContext(dagRecord, node)

	assert.Equal(t, uuid.Nil, ctx.OntologyID)
}

// Test implementations to verify interfaces compile correctly

type testFKDiscovery struct{}

func (t *testFKDiscovery) DiscoverFKRelationships(_ context.Context, _, _ uuid.UUID, _ dag.ProgressCallback) (*dag.FKDiscoveryResult, error) {
	return nil, nil
}

type testPKMatchDiscovery struct{}

func (t *testPKMatchDiscovery) DiscoverPKMatchRelationships(_ context.Context, _, _ uuid.UUID, _ dag.ProgressCallback) (*dag.PKMatchDiscoveryResult, error) {
	return nil, nil
}

type testFinalization struct{}

func (t *testFinalization) Finalize(_ context.Context, _ uuid.UUID) error {
	return nil
}

type testColumnEnrichment struct{}

func (t *testColumnEnrichment) EnrichProject(_ context.Context, _ uuid.UUID, _ []string, _ dag.ProgressCallback) (*dag.ColumnEnrichmentResult, error) {
	return nil, nil
}

type testGlossaryDiscovery struct{}

func (t *testGlossaryDiscovery) DiscoverGlossaryTerms(_ context.Context, _, _ uuid.UUID) (int, error) {
	return 0, nil
}

type testColumnFeatureExtraction struct{}

func (t *testColumnFeatureExtraction) ExtractColumnFeatures(_ context.Context, _, _ uuid.UUID, _ dag.ProgressCallback) (int, error) {
	return 0, nil
}

type testTableFeatureExtraction struct{}

func (t *testTableFeatureExtraction) ExtractTableFeatures(_ context.Context, _, _ uuid.UUID, _ dag.ProgressCallback) (int, error) {
	return 0, nil
}

// ============================================================================
// Delete Method Tests
// ============================================================================

// Note: Delete method integration tests would require real database fixtures.
// The handler tests in pkg/handlers/ontology_dag_handler_test.go provide
// end-to-end coverage with mocked service. The service-level logic is
// straightforward repository delegation, so integration tests are more valuable
// than unit tests with complex mock implementations.

// ============================================================================
// Cancel Method Tests
// ============================================================================

// mockDAGRepository is a mock implementation for testing Cancel
type mockDAGRepository struct {
	getNodesByDAGFunc         func(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error)
	updateNodeStatusFunc      func(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error
	updateStatusFunc          func(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error
	getByIDWithNodesFunc      func(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error)
	getActiveByDatasourceFunc func(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error)
}

func (m *mockDAGRepository) GetNodesByDAG(ctx context.Context, dagID uuid.UUID) ([]models.DAGNode, error) {
	if m.getNodesByDAGFunc != nil {
		return m.getNodesByDAGFunc(ctx, dagID)
	}
	return nil, nil
}

func (m *mockDAGRepository) UpdateNodeStatus(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
	if m.updateNodeStatusFunc != nil {
		return m.updateNodeStatusFunc(ctx, nodeID, status, errorMessage)
	}
	return nil
}

func (m *mockDAGRepository) UpdateStatus(ctx context.Context, dagID uuid.UUID, status models.DAGStatus, currentNode *string) error {
	if m.updateStatusFunc != nil {
		return m.updateStatusFunc(ctx, dagID, status, currentNode)
	}
	return nil
}

// Stub methods to satisfy the interface
func (m *mockDAGRepository) Create(ctx context.Context, dag *models.OntologyDAG) error { return nil }
func (m *mockDAGRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) GetByIDWithNodes(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
	if m.getByIDWithNodesFunc != nil {
		return m.getByIDWithNodesFunc(ctx, id)
	}
	return nil, nil
}
func (m *mockDAGRepository) GetLatestByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) GetLatestByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) GetActiveByDatasource(ctx context.Context, datasourceID uuid.UUID) (*models.OntologyDAG, error) {
	if m.getActiveByDatasourceFunc != nil {
		return m.getActiveByDatasourceFunc(ctx, datasourceID)
	}
	return nil, nil
}
func (m *mockDAGRepository) GetActiveByProject(ctx context.Context, projectID uuid.UUID) (*models.OntologyDAG, error) {
	return nil, nil
}
func (m *mockDAGRepository) Update(ctx context.Context, dag *models.OntologyDAG) error { return nil }
func (m *mockDAGRepository) Delete(ctx context.Context, id uuid.UUID) error            { return nil }
func (m *mockDAGRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockDAGRepository) ClaimOwnership(ctx context.Context, dagID, ownerID uuid.UUID) (bool, error) {
	return true, nil
}
func (m *mockDAGRepository) UpdateHeartbeat(ctx context.Context, dagID, ownerID uuid.UUID) error {
	return nil
}
func (m *mockDAGRepository) ReleaseOwnership(ctx context.Context, dagID uuid.UUID) error { return nil }
func (m *mockDAGRepository) CreateNodes(ctx context.Context, nodes []models.DAGNode) error {
	return nil
}
func (m *mockDAGRepository) UpdateNodeProgress(ctx context.Context, nodeID uuid.UUID, progress *models.DAGNodeProgress) error {
	return nil
}
func (m *mockDAGRepository) IncrementNodeRetryCount(ctx context.Context, nodeID uuid.UUID) error {
	return nil
}
func (m *mockDAGRepository) GetNextPendingNode(ctx context.Context, dagID uuid.UUID) (*models.DAGNode, error) {
	return nil, nil
}

// mockKnowledgeRepository is a mock implementation of KnowledgeRepository for testing Start.
type mockKnowledgeRepository struct {
	createFunc    func(ctx context.Context, fact *models.KnowledgeFact) error
	updateFunc    func(ctx context.Context, fact *models.KnowledgeFact) error
	getByTypeFunc func(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error)
}

func (m *mockKnowledgeRepository) Create(ctx context.Context, fact *models.KnowledgeFact) error {
	if m.createFunc != nil {
		return m.createFunc(ctx, fact)
	}
	return nil
}

func (m *mockKnowledgeRepository) Update(ctx context.Context, fact *models.KnowledgeFact) error {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, fact)
	}
	return nil
}

func (m *mockKnowledgeRepository) GetByProject(_ context.Context, _ uuid.UUID) ([]*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeRepository) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	if m.getByTypeFunc != nil {
		return m.getByTypeFunc(ctx, projectID, factType)
	}
	return nil, nil
}

func (m *mockKnowledgeRepository) Delete(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeRepository) DeleteByProject(_ context.Context, _ uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeRepository) DeleteBySource(_ context.Context, _ uuid.UUID, _ models.ProvenanceSource) error {
	return nil
}

// ============================================================================
// Start Method - Project Overview Storage Tests
// ============================================================================

// createAuthenticatedContext creates a context with JWT claims for the given user ID.
func createAuthenticatedContext(userID uuid.UUID) context.Context {
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID.String(),
		},
	}
	return context.WithValue(context.Background(), auth.ClaimsKey, claims)
}

// TestStart_StoresProjectOverview verifies that when a non-empty project overview
// is provided, it is stored as project knowledge with the correct attributes.
func TestStart_StoresProjectOverview(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	userID := uuid.New()
	projectOverview := "This is a CRM application for managing customer relationships"

	// Track Create calls
	var capturedFact *models.KnowledgeFact
	var createCalled bool

	mockKnowledgeRepo := &mockKnowledgeRepository{
		createFunc: func(ctx context.Context, fact *models.KnowledgeFact) error {
			createCalled = true
			capturedFact = fact

			// Verify provenance was set correctly (manual provenance)
			prov, ok := models.GetProvenance(ctx)
			assert.True(t, ok, "Provenance should be set in context")
			assert.Equal(t, models.SourceManual, prov.Source, "Source should be manual")
			assert.Equal(t, userID, prov.UserID, "UserID should match the authenticated user")

			return nil
		},
	}

	dagID := uuid.New()
	mockDAGRepo := &mockDAGRepository{
		// Return no existing active DAG
		getActiveByDatasourceFunc: func(_ context.Context, _ uuid.UUID) (*models.OntologyDAG, error) {
			return nil, nil
		},
		// Provide a valid DAG record for background goroutine cleanup
		getByIDWithNodesFunc: func(_ context.Context, _ uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:        dagID,
				ProjectID: projectID,
				Nodes:     []models.DAGNode{},
			}, nil
		},
	}


	// Create entity and relationship repository mocks (use existing mocks from same package)
	logger := zap.NewNop()
	service := &ontologyDAGService{
		dagRepo:       mockDAGRepo,
		knowledgeRepo: mockKnowledgeRepo,
		logger:        logger,
		// Provide getTenantCtx to prevent panic in background goroutine
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Create authenticated context
	ctx := createAuthenticatedContext(userID)

	// Call Start - it will proceed through overview storage and continue to DAG creation
	// We ignore the result as we're only testing overview storage
	_, _ = service.Start(ctx, projectID, datasourceID, projectOverview)

	// Verify Create was called with correct fact structure
	assert.True(t, createCalled, "Create should be called when overview is provided")
	assert.NotNil(t, capturedFact, "Fact should be captured")
	assert.Equal(t, projectID, capturedFact.ProjectID, "ProjectID should match")
	assert.Equal(t, "project_overview", capturedFact.FactType, "FactType should be 'project_overview'")
	assert.Equal(t, projectOverview, capturedFact.Value, "Value should match the overview text")
}

// TestStart_UpdatesExistingProjectOverview verifies that when a project_overview
// already exists, it is updated rather than creating a duplicate.
func TestStart_UpdatesExistingProjectOverview(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	userID := uuid.New()
	existingOverviewID := uuid.New()
	originalOverview := "Original project overview"
	newOverview := "Updated project overview with more details"

	// Track calls
	var createCalled bool
	var updateCalled bool
	var updatedFact *models.KnowledgeFact

	mockKnowledgeRepo := &mockKnowledgeRepository{
		getByTypeFunc: func(_ context.Context, _ uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
			if factType == "project_overview" {
				// Return existing project_overview
				return []*models.KnowledgeFact{
					{
						ID:        existingOverviewID,
						ProjectID: projectID,
						FactType:  "project_overview",
						Value:     originalOverview,
					},
				}, nil
			}
			return nil, nil
		},
		createFunc: func(_ context.Context, _ *models.KnowledgeFact) error {
			createCalled = true
			return nil
		},
		updateFunc: func(ctx context.Context, fact *models.KnowledgeFact) error {
			updateCalled = true
			updatedFact = fact

			// Verify provenance was set correctly (manual provenance)
			prov, ok := models.GetProvenance(ctx)
			assert.True(t, ok, "Provenance should be set in context")
			assert.Equal(t, models.SourceManual, prov.Source, "Source should be manual")
			assert.Equal(t, userID, prov.UserID, "UserID should match the authenticated user")

			return nil
		},
	}

	dagID := uuid.New()
	mockDAGRepo := &mockDAGRepository{
		getActiveByDatasourceFunc: func(_ context.Context, _ uuid.UUID) (*models.OntologyDAG, error) {
			return nil, nil
		},
		getByIDWithNodesFunc: func(_ context.Context, _ uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:        dagID,
				ProjectID: projectID,
				Nodes:     []models.DAGNode{},
			}, nil
		},
	}


	logger := zap.NewNop()
	service := &ontologyDAGService{
		dagRepo:       mockDAGRepo,
		knowledgeRepo: mockKnowledgeRepo,
		logger:        logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	ctx := createAuthenticatedContext(userID)

	// Call Start with new overview
	_, _ = service.Start(ctx, projectID, datasourceID, newOverview)

	// Verify Update was called instead of Create
	assert.False(t, createCalled, "Create should NOT be called when project_overview exists")
	assert.True(t, updateCalled, "Update should be called when project_overview exists")
	assert.NotNil(t, updatedFact, "Updated fact should be captured")
	assert.Equal(t, existingOverviewID, updatedFact.ID, "Should update the existing fact")
	assert.Equal(t, newOverview, updatedFact.Value, "Value should be updated to new overview")
}

// TestStart_SkipsOverviewStorageWhenEmpty verifies that when an empty project
// overview is provided, Upsert is NOT called.
func TestStart_SkipsOverviewStorageWhenEmpty(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	userID := uuid.New()
	emptyOverview := ""

	// Track Create calls
	createCalled := false

	mockKnowledgeRepo := &mockKnowledgeRepository{
		createFunc: func(_ context.Context, _ *models.KnowledgeFact) error {
			createCalled = true
			return nil
		},
	}

	dagID := uuid.New()
	mockDAGRepo := &mockDAGRepository{
		getActiveByDatasourceFunc: func(_ context.Context, _ uuid.UUID) (*models.OntologyDAG, error) {
			return nil, nil
		},
		// Provide a valid DAG record for background goroutine cleanup
		getByIDWithNodesFunc: func(_ context.Context, _ uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:        dagID,
				ProjectID: projectID,
				Nodes:     []models.DAGNode{},
			}, nil
		},
	}


	logger := zap.NewNop()
	service := &ontologyDAGService{
		dagRepo:       mockDAGRepo,
		knowledgeRepo: mockKnowledgeRepo,
		logger:        logger,
		// Provide getTenantCtx to prevent panic in background goroutine
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Create authenticated context
	ctx := createAuthenticatedContext(userID)

	// Call Start with empty overview
	_, _ = service.Start(ctx, projectID, datasourceID, emptyOverview)

	// Verify Create was NOT called
	assert.False(t, createCalled, "Create should NOT be called when overview is empty")
}

// TestStart_ContinuesOnOverviewStorageError verifies that when Upsert fails,
// the extraction still continues (non-fatal error handling).
func TestStart_ContinuesOnOverviewStorageError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	userID := uuid.New()
	projectOverview := "A valid project overview"

	// Track the flow of execution
	var createCalled bool
	var getActiveByDatasourceCalled bool

	mockKnowledgeRepo := &mockKnowledgeRepository{
		createFunc: func(_ context.Context, _ *models.KnowledgeFact) error {
			createCalled = true
			// Return an error to simulate storage failure
			return fmt.Errorf("database connection failed")
		},
	}

	dagID := uuid.New()
	mockDAGRepo := &mockDAGRepository{
		getActiveByDatasourceFunc: func(_ context.Context, _ uuid.UUID) (*models.OntologyDAG, error) {
			getActiveByDatasourceCalled = true
			return nil, nil // No existing DAG, continue with creation
		},
		// Provide a valid DAG record for background goroutine cleanup
		getByIDWithNodesFunc: func(_ context.Context, _ uuid.UUID) (*models.OntologyDAG, error) {
			return &models.OntologyDAG{
				ID:        dagID,
				ProjectID: projectID,
				Nodes:     []models.DAGNode{},
			}, nil
		},
	}


	logger := zap.NewNop()
	service := &ontologyDAGService{
		dagRepo:       mockDAGRepo,
		knowledgeRepo: mockKnowledgeRepo,
		logger:        logger,
		// Provide getTenantCtx to prevent panic in background goroutine
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Create authenticated context
	ctx := createAuthenticatedContext(userID)

	// Call Start with overview that will fail to store
	_, _ = service.Start(ctx, projectID, datasourceID, projectOverview)

	// Verify both steps were attempted:
	// 1. Create was called (and failed)
	assert.True(t, createCalled, "Create should be called")
	// 2. Extraction continued after Create failure (getActiveByDatasource was called)
	assert.True(t, getActiveByDatasourceCalled, "Extraction should continue after Create failure")
}

// TestCancel_MarksNonCompletedNodesAsSkipped verifies that canceling a DAG
// marks all running and pending nodes as skipped.
func TestCancel_MarksNonCompletedNodesAsSkipped(t *testing.T) {
	dagID := uuid.New()
	ctx := context.Background()

	// Create test nodes with various statuses
	nodes := []models.DAGNode{
		{ID: uuid.New(), DAGID: dagID, NodeName: "EntityDiscovery", Status: models.DAGNodeStatusCompleted},
		{ID: uuid.New(), DAGID: dagID, NodeName: "EntityEnrichment", Status: models.DAGNodeStatusRunning},
		{ID: uuid.New(), DAGID: dagID, NodeName: "FKDiscovery", Status: models.DAGNodeStatusPending},
		{ID: uuid.New(), DAGID: dagID, NodeName: "RelationshipEnrichment", Status: models.DAGNodeStatusPending},
	}

	// Track which nodes were marked as skipped
	skippedNodes := make(map[uuid.UUID]bool)

	mockRepo := &mockDAGRepository{
		getNodesByDAGFunc: func(_ context.Context, id uuid.UUID) ([]models.DAGNode, error) {
			assert.Equal(t, dagID, id)
			return nodes, nil
		},
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, _ *string) error {
			assert.Equal(t, models.DAGNodeStatusSkipped, status)
			skippedNodes[nodeID] = true
			return nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			assert.Equal(t, dagID, id)
			assert.Equal(t, models.DAGStatusCancelled, status)
			return nil
		},
	}

	// Create service with mock repository
	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
	}

	// Execute Cancel
	err := service.Cancel(ctx, dagID)
	assert.NoError(t, err)

	// Verify only non-completed nodes were marked as skipped
	// Node 0 (completed) should NOT be skipped
	assert.False(t, skippedNodes[nodes[0].ID], "Completed node should not be skipped")
	// Node 1 (running) should be skipped
	assert.True(t, skippedNodes[nodes[1].ID], "Running node should be skipped")
	// Node 2 (pending) should be skipped
	assert.True(t, skippedNodes[nodes[2].ID], "Pending node should be skipped")
	// Node 3 (pending) should be skipped
	assert.True(t, skippedNodes[nodes[3].ID], "Pending node should be skipped")
}

// ============================================================================
// markDAGFailed Tests
// ============================================================================

// TestMarkDAGFailed_StoresErrorOnCurrentNode verifies that early startup errors
// are properly stored on the current node for UI visibility.
func TestMarkDAGFailed_StoresErrorOnCurrentNode(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	currentNodeName := string(models.DAGNodeKnowledgeSeeding)
	errorMessage := "failed to get tenant context"

	// Create test nodes
	nodes := []models.DAGNode{
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeKnowledgeSeeding), Status: models.DAGNodeStatusRunning},
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeColumnFeatureExtraction), Status: models.DAGNodeStatusPending},
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeFKDiscovery), Status: models.DAGNodeStatusPending},
	}

	dag := &models.OntologyDAG{
		ID:          dagID,
		ProjectID:   projectID,
		Status:      models.DAGStatusRunning,
		CurrentNode: &currentNodeName,
		Nodes:       nodes,
	}

	// Track updates
	var updatedNodeID uuid.UUID
	var updatedErrorMsg string
	var updatedDAGStatus models.DAGStatus

	mockRepo := &mockDAGRepository{
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errMsg *string) error {
			updatedNodeID = nodeID
			if errMsg != nil {
				updatedErrorMsg = *errMsg
			}
			assert.Equal(t, models.DAGNodeStatusFailed, status)
			return nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			updatedDAGStatus = status
			assert.Equal(t, dagID, id)
			return nil
		},
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			assert.Equal(t, dagID, id)
			return dag, nil
		},
	}

	// Create service with mock repository and mock getTenantCtx
	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			// Return a valid context for this test
			return ctx, func() {}, nil
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify the current node was marked as failed with the error message
	assert.Equal(t, nodes[0].ID, updatedNodeID, "Current node should be marked as failed")
	assert.Equal(t, errorMessage, updatedErrorMsg, "Error message should be stored on the node")
	assert.Equal(t, models.DAGStatusFailed, updatedDAGStatus, "DAG should be marked as failed")
}

// TestMarkDAGFailed_StoresErrorOnFirstPendingNode verifies that when no current node
// is set, the error is stored on the first pending/running node.
func TestMarkDAGFailed_StoresErrorOnFirstPendingNode(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	errorMessage := "failed during initialization"

	// Create test nodes - first node is pending (no current node set yet)
	nodes := []models.DAGNode{
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeKnowledgeSeeding), Status: models.DAGNodeStatusPending},
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeColumnFeatureExtraction), Status: models.DAGNodeStatusPending},
	}

	dag := &models.OntologyDAG{
		ID:          dagID,
		ProjectID:   projectID,
		Status:      models.DAGStatusPending,
		CurrentNode: nil, // No current node set
		Nodes:       nodes,
	}

	// Track updates
	var updatedNodeID uuid.UUID
	var updatedErrorMsg string

	mockRepo := &mockDAGRepository{
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errMsg *string) error {
			updatedNodeID = nodeID
			if errMsg != nil {
				updatedErrorMsg = *errMsg
			}
			assert.Equal(t, models.DAGNodeStatusFailed, status)
			return nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			assert.Equal(t, dagID, id)
			assert.Equal(t, models.DAGStatusFailed, status)
			return nil
		},
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			return dag, nil
		},
	}

	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify the first pending node was marked as failed
	assert.Equal(t, nodes[0].ID, updatedNodeID, "First pending node should be marked as failed")
	assert.Equal(t, errorMessage, updatedErrorMsg, "Error message should be stored on the node")
}

// TestMarkDAGFailed_WhenGetByIDWithNodesFails_StillMarksDAGFailed verifies that even
// if we can't get the nodes, the DAG status is still updated to failed.
func TestMarkDAGFailed_WhenGetByIDWithNodesFails_StillMarksDAGFailed(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	errorMessage := "initialization failed"

	// Track updates
	var dagUpdateCalled bool

	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			return nil, assert.AnError // Simulate failure to get DAG
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			dagUpdateCalled = true
			assert.Equal(t, dagID, id)
			assert.Equal(t, models.DAGStatusFailed, status)
			return nil
		},
	}

	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify the DAG status was still updated even though GetByIDWithNodes failed
	assert.True(t, dagUpdateCalled, "DAG status should be updated even if GetByIDWithNodes fails")
}

// TestMarkDAGFailed_WithAllNodesCompleted_MarksFirstNode verifies that when all nodes
// are completed (no pending/running nodes), the first node is marked as failed.
func TestMarkDAGFailed_WithAllNodesCompleted_MarksFirstNode(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	errorMessage := "post-completion error"

	// Create test nodes - all completed
	nodes := []models.DAGNode{
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeKnowledgeSeeding), Status: models.DAGNodeStatusCompleted},
		{ID: uuid.New(), DAGID: dagID, NodeName: string(models.DAGNodeColumnFeatureExtraction), Status: models.DAGNodeStatusCompleted},
	}

	dag := &models.OntologyDAG{
		ID:          dagID,
		ProjectID:   projectID,
		Status:      models.DAGStatusCompleted,
		CurrentNode: nil,
		Nodes:       nodes,
	}

	// Track updates
	var updatedNodeID uuid.UUID

	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			return dag, nil
		},
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errMsg *string) error {
			updatedNodeID = nodeID
			assert.Equal(t, models.DAGNodeStatusFailed, status)
			assert.Equal(t, errorMessage, *errMsg)
			return nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			assert.Equal(t, models.DAGStatusFailed, status)
			return nil
		},
	}

	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return ctx, func() {}, nil
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify the first node was marked as failed (fallback behavior)
	assert.Equal(t, nodes[0].ID, updatedNodeID, "First node should be marked as failed when all nodes are completed")
}

// TestMarkDAGFailed_WhenTenantCtxFails_LogsError verifies that when getTenantCtx fails,
// only error logging occurs and no updates are attempted.
func TestMarkDAGFailed_WhenTenantCtxFails_LogsError(t *testing.T) {
	dagID := uuid.New()
	projectID := uuid.New()
	errorMessage := "some error"

	// Track if any repository methods were called
	var repoMethodCalled bool

	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(_ context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			repoMethodCalled = true
			return nil, nil
		},
		updateStatusFunc: func(_ context.Context, id uuid.UUID, status models.DAGStatus, _ *string) error {
			repoMethodCalled = true
			return nil
		},
		updateNodeStatusFunc: func(_ context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errMsg *string) error {
			repoMethodCalled = true
			return nil
		},
	}

	logger, _ := zap.NewDevelopment()
	service := &ontologyDAGService{
		dagRepo: mockRepo,
		logger:  logger,
		getTenantCtx: func(ctx context.Context, _ uuid.UUID) (context.Context, func(), error) {
			return nil, nil, assert.AnError // Simulate failure to get tenant context
		},
	}

	// Execute markDAGFailed
	service.markDAGFailed(projectID, dagID, errorMessage)

	// Verify no repository methods were called
	assert.False(t, repoMethodCalled, "No repository methods should be called when getTenantCtx fails")
}

// TestExecuteDAG_PanicRecovery verifies that panics during DAG execution
// are recovered, cleanup happens properly, and the DAG is marked as failed.
func TestExecuteDAG_PanicRecovery(t *testing.T) {
	projectID := uuid.New()
	dagID := uuid.New()

	// Track whether markDAGFailed was called
	var markDAGFailedCalls []string
	var mu sync.Mutex

	// Create mock repository that tracks calls
	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			// Return a DAG with one node
			return &models.OntologyDAG{
				ID:        dagID,
				ProjectID: projectID,
				Nodes: []models.DAGNode{
					{
						ID:       uuid.New(),
						DAGID:    dagID,
						NodeName: string(models.DAGNodeKnowledgeSeeding),
						Status:   models.DAGNodeStatusPending,
					},
				},
			}, nil
		},
		updateStatusFunc: func(ctx context.Context, id uuid.UUID, status models.DAGStatus, currentNode *string) error {
			if status == models.DAGStatusFailed {
				mu.Lock()
				markDAGFailedCalls = append(markDAGFailedCalls, "updateStatus")
				mu.Unlock()
			}
			return nil
		},
		updateNodeStatusFunc: func(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
			if status == models.DAGNodeStatusFailed && errorMessage != nil {
				mu.Lock()
				markDAGFailedCalls = append(markDAGFailedCalls, fmt.Sprintf("updateNodeStatus: %s", *errorMessage))
				mu.Unlock()
			}
			return nil
		},
	}

	// Create a test service
	service := &ontologyDAGService{
		dagRepo:          mockRepo,
		logger:           zap.NewNop(),
		serverInstanceID: uuid.New(),
	}

	// Set getTenantCtx to cause a panic only on first call
	// (subsequent calls from markDAGFailed should succeed)
	callCount := 0
	var callMu sync.Mutex
	service.getTenantCtx = func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		callMu.Lock()
		callCount++
		count := callCount
		callMu.Unlock()

		if count == 1 {
			// First call - panic to test recovery
			panic("simulated panic during DAG execution")
		}
		// Subsequent calls - succeed to allow markDAGFailed to work
		return ctx, func() {}, nil
	}

	// Execute DAG in a goroutine (as it would be in production)
	// Use a test user ID for provenance tracking
	testUserID := uuid.New()
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.executeDAG(projectID, dagID, testUserID)
	}()

	// Wait for goroutine to complete with timeout
	select {
	case <-done:
		// Good, execution completed
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for executeDAG to complete")
	}

	// Give a small amount of time for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	// Verify markDAGFailed was called (which updates status and node status)
	mu.Lock()
	defer mu.Unlock()
	assert.True(t, len(markDAGFailedCalls) >= 1, "markDAGFailed should have been called at least once")

	// Check that at least one call mentions panic
	foundPanicMessage := false
	for _, call := range markDAGFailedCalls {
		if contains(call, "panic") {
			foundPanicMessage = true
			break
		}
	}
	assert.True(t, foundPanicMessage, "Error message should mention panic")

	// Verify activeDAGs was cleaned up
	_, exists := service.activeDAGs.Load(dagID)
	assert.False(t, exists, "DAG should be removed from activeDAGs after panic")

	// Verify heartbeat was cleaned up
	_, exists = service.heartbeatCancel.Load(dagID)
	assert.False(t, exists, "Heartbeat should be cleaned up after panic")
}

// TestExecuteDAG_HeartbeatCleanupOrder verifies that the heartbeat
// is properly started and stopped even when execution fails early.
func TestExecuteDAG_HeartbeatCleanupOrder(t *testing.T) {
	projectID := uuid.New()
	dagID := uuid.New()

	// Create mock repository
	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			// Return error to cause early exit
			return nil, fmt.Errorf("simulated repository error")
		},
		updateStatusFunc: func(ctx context.Context, id uuid.UUID, status models.DAGStatus, currentNode *string) error {
			return nil
		},
		updateNodeStatusFunc: func(ctx context.Context, nodeID uuid.UUID, status models.DAGNodeStatus, errorMessage *string) error {
			return nil
		},
	}

	// Create a test service
	service := &ontologyDAGService{
		dagRepo:          mockRepo,
		logger:           zap.NewNop(),
		serverInstanceID: uuid.New(),
	}

	// Set getTenantCtx to succeed
	service.getTenantCtx = func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	// Execute DAG in a goroutine
	// Use a test user ID for provenance tracking
	testUserID := uuid.New()
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.executeDAG(projectID, dagID, testUserID)
	}()

	// Wait for goroutine to complete with timeout
	select {
	case <-done:
		// Good, execution completed
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for executeDAG to complete")
	}

	// Give a small amount of time for cleanup to complete
	time.Sleep(100 * time.Millisecond)

	// Verify activeDAGs was cleaned up
	_, exists := service.activeDAGs.Load(dagID)
	assert.False(t, exists, "DAG should be removed from activeDAGs after completion")

	// Verify heartbeat was cleaned up
	_, exists = service.heartbeatCancel.Load(dagID)
	assert.False(t, exists, "Heartbeat should be cleaned up after completion")
}

// ============================================================================
// GetNodeExecutor Tests
// ============================================================================

func TestGetNodeExecutor_GlossaryDiscovery(t *testing.T) {
	service := &ontologyDAGService{
		dagRepo:                  &mockDAGRepository{},
		logger:                   zap.NewNop(),
		glossaryDiscoveryMethods: &testGlossaryDiscovery{},
	}

	nodeID := uuid.New()
	executor, err := service.getNodeExecutor(models.DAGNodeGlossaryDiscovery, nodeID)

	assert.NoError(t, err)
	assert.NotNil(t, executor)
	assert.IsType(t, &dag.GlossaryDiscoveryNode{}, executor)
}

func TestGetNodeExecutor_GlossaryDiscovery_NotSet(t *testing.T) {
	service := &ontologyDAGService{
		dagRepo: &mockDAGRepository{},
		logger:  zap.NewNop(),
		// glossaryDiscoveryMethods intentionally not set
	}

	nodeID := uuid.New()
	executor, err := service.getNodeExecutor(models.DAGNodeGlossaryDiscovery, nodeID)

	assert.Error(t, err)
	assert.Nil(t, executor)
	assert.Contains(t, err.Error(), "glossary discovery methods not set")
}

func TestSetGlossaryMethods(t *testing.T) {
	service := &ontologyDAGService{
		dagRepo: &mockDAGRepository{},
		logger:  zap.NewNop(),
	}

	// Test SetGlossaryDiscoveryMethods
	discoveryMethods := &testGlossaryDiscovery{}
	service.SetGlossaryDiscoveryMethods(discoveryMethods)
	assert.Equal(t, discoveryMethods, service.glossaryDiscoveryMethods)
}

// ============================================================================
// Start Method Provenance Tests
// ============================================================================

// TestStart_RequiresAuthenticatedUser verifies that Start requires a valid user ID
// from JWT claims for provenance tracking.
func TestStart_RequiresAuthenticatedUser(t *testing.T) {
	service := &ontologyDAGService{
		dagRepo: &mockDAGRepository{},
		logger:  zap.NewNop(),
	}

	// Try to start without authentication (no claims in context)
	ctx := context.Background()
	projectID := uuid.New()
	datasourceID := uuid.New()

	_, err := service.Start(ctx, projectID, datasourceID, "")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "user authentication required")
}

// TestExecuteDAG_SetsInferenceProvenance verifies that executeDAG properly sets
// inference provenance on the tenant context with the triggering user's ID.
func TestExecuteDAG_SetsInferenceProvenance(t *testing.T) {
	projectID := uuid.New()
	dagID := uuid.New()
	ontologyID := uuid.New()
	userID := uuid.New()

	// Track what provenance was set on the context
	var capturedProvenance models.ProvenanceContext
	var provenanceFound bool

	// Create mock repository that captures the context
	mockRepo := &mockDAGRepository{
		getByIDWithNodesFunc: func(ctx context.Context, id uuid.UUID) (*models.OntologyDAG, error) {
			// Capture the provenance from the context
			capturedProvenance, provenanceFound = models.GetProvenance(ctx)
			return &models.OntologyDAG{
				ID:           dagID,
				ProjectID:    projectID,
				DatasourceID: uuid.New(),
				OntologyID:   &ontologyID,
				Nodes:        []models.DAGNode{},
			}, nil
		},
	}

	// Create service
	service := &ontologyDAGService{
		dagRepo:          mockRepo,
		logger:           zap.NewNop(),
		serverInstanceID: uuid.New(),
	}

	// Set getTenantCtx to succeed (returns context as-is but with provenance wrapped by executeDAG)
	service.getTenantCtx = func(ctx context.Context, pid uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	// Execute DAG with a specific user ID
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.executeDAG(projectID, dagID, userID)
	}()

	// Wait for goroutine to complete
	select {
	case <-done:
		// Good, execution completed
	case <-time.After(2 * time.Second):
		t.Fatal("Test timed out waiting for executeDAG to complete")
	}

	// Give a small amount of time for cleanup
	time.Sleep(100 * time.Millisecond)

	// Verify provenance was set correctly
	assert.True(t, provenanceFound, "Provenance should be set in context")
	assert.Equal(t, models.SourceInferred, capturedProvenance.Source, "Source should be inference")
	assert.Equal(t, userID, capturedProvenance.UserID, "UserID should match the triggering user")
}
