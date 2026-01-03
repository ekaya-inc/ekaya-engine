package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/dag"
)

// Note: Full integration tests are in pkg/services/ontology_dag_integration_test.go
// These unit tests focus on DAG node creation and executor selection logic.

func TestDAGNodes_AllNodesHaveCorrectOrder(t *testing.T) {
	allNodes := models.AllDAGNodes()

	expectedOrder := []models.DAGNodeName{
		models.DAGNodeEntityDiscovery,
		models.DAGNodeEntityEnrichment,
		models.DAGNodeRelationshipDiscovery,
		models.DAGNodeRelationshipEnrichment,
		models.DAGNodeOntologyFinalization,
		models.DAGNodeColumnEnrichment,
	}

	assert.Equal(t, len(expectedOrder), len(allNodes))

	for i, expected := range expectedOrder {
		assert.Equal(t, expected, allNodes[i])
		assert.Equal(t, i+1, models.DAGNodeOrder[expected])
	}
}

func TestNodeExecutorInterfaces_AreWellDefined(t *testing.T) {
	// Verify that the interface methods are properly defined
	// by creating implementations that satisfy them

	// EntityDiscoveryMethods
	var edm dag.EntityDiscoveryMethods = &testEntityDiscovery{}
	assert.NotNil(t, edm)

	// EntityEnrichmentMethods
	var eem dag.EntityEnrichmentMethods = &testEntityEnrichment{}
	assert.NotNil(t, eem)

	// DeterministicRelationshipMethods
	var drm dag.DeterministicRelationshipMethods = &testRelationshipDiscovery{}
	assert.NotNil(t, drm)

	// RelationshipEnrichmentMethods
	var rem dag.RelationshipEnrichmentMethods = &testRelationshipEnrichment{}
	assert.NotNil(t, rem)

	// OntologyFinalizationMethods
	var ofm dag.OntologyFinalizationMethods = &testFinalization{}
	assert.NotNil(t, ofm)

	// ColumnEnrichmentMethods
	var cem dag.ColumnEnrichmentMethods = &testColumnEnrichment{}
	assert.NotNil(t, cem)
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
		NodeName: string(models.DAGNodeEntityDiscovery),
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
		NodeName: string(models.DAGNodeEntityDiscovery),
	}

	ctx := dag.NewExecutionContext(dagRecord, node)

	assert.Equal(t, uuid.Nil, ctx.OntologyID)
}

// Test implementations to verify interfaces compile correctly

type testEntityDiscovery struct{}

func (t *testEntityDiscovery) IdentifyEntitiesFromDDL(_ context.Context, _, _, _ uuid.UUID) (int, []*models.SchemaTable, []*models.SchemaColumn, error) {
	return 0, nil, nil, nil
}

type testEntityEnrichment struct{}

func (t *testEntityEnrichment) EnrichEntitiesWithLLM(_ context.Context, _, _, _ uuid.UUID) error {
	return nil
}

type testRelationshipDiscovery struct{}

func (t *testRelationshipDiscovery) DiscoverRelationships(_ context.Context, _, _ uuid.UUID) (*dag.RelationshipDiscoveryResult, error) {
	return nil, nil
}

type testRelationshipEnrichment struct{}

func (t *testRelationshipEnrichment) EnrichProject(_ context.Context, _ uuid.UUID, _ dag.ProgressCallback) (*dag.RelationshipEnrichmentResult, error) {
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
