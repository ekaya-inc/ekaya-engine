package services

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// TestNewLLMRelationshipDiscoveryService tests service creation
func TestNewLLMRelationshipDiscoveryService(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, // candidateCollector - tested in relationship_candidate_collector_test.go
		nil, // validator - tested in relationship_validator_test.go
		nil, // datasourceService
		nil, // adapterFactory
		nil, // ontologyRepo
		nil, // entityRepo
		nil, // relationshipRepo
		nil, // schemaRepo
		logger,
	)

	require.NotNil(t, svc, "NewLLMRelationshipDiscoveryService should return non-nil service")
}

// TestLLMRelationshipDiscoveryResult verifies the result structure
func TestLLMRelationshipDiscoveryResult(t *testing.T) {
	result := &LLMRelationshipDiscoveryResult{
		CandidatesEvaluated:   100,
		RelationshipsCreated:  15,
		RelationshipsRejected: 85,
		PreservedDBFKs:        10,
		PreservedColumnFKs:    5,
		DurationMs:            1500,
	}

	assert.Equal(t, 100, result.CandidatesEvaluated)
	assert.Equal(t, 15, result.RelationshipsCreated)
	assert.Equal(t, 85, result.RelationshipsRejected)
	assert.Equal(t, 10, result.PreservedDBFKs)
	assert.Equal(t, 5, result.PreservedColumnFKs)
	assert.Equal(t, int64(1500), result.DurationMs)
}

// TestBuildEntityByTableMap verifies entity-to-table mapping
func TestBuildEntityByTableMap(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), Name: "User", PrimaryTable: "users"},
		{ID: uuid.New(), Name: "Order", PrimaryTable: "orders"},
		{ID: uuid.New(), Name: "Orphan", PrimaryTable: ""}, // No primary table
	}

	entityMap := svc.buildEntityByTableMap(entities)

	require.Len(t, entityMap, 2, "should only include entities with primary tables")
	assert.NotNil(t, entityMap["users"])
	assert.NotNil(t, entityMap["orders"])
	assert.Nil(t, entityMap[""], "should not include empty table name")
}

// TestBuildExistingRelationshipSet verifies deduplication set creation
func TestBuildExistingRelationshipSet(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	relationships := []*models.EntityRelationship{
		{
			SourceColumnTable: "orders",
			SourceColumnName:  "user_id",
			TargetColumnTable: "users",
			TargetColumnName:  "id",
		},
		{
			SourceColumnTable: "orders",
			SourceColumnName:  "product_id",
			TargetColumnTable: "products",
			TargetColumnName:  "id",
		},
	}

	relSet := svc.buildExistingRelationshipSet(relationships)

	require.Len(t, relSet, 2)
	assert.True(t, relSet["orders.user_id->users.id"])
	assert.True(t, relSet["orders.product_id->products.id"])
	assert.False(t, relSet["orders.status_id->statuses.id"], "non-existent relationship should not be in set")
}

// TestBuildEntityByTableMap_EmptyInput tests handling of empty entity list
func TestBuildEntityByTableMap_EmptyInput(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	entityMap := svc.buildEntityByTableMap([]*models.OntologyEntity{})

	require.Empty(t, entityMap, "empty input should produce empty map")
}

// TestBuildEntityByTableMap_NilInput tests handling of nil entity list
func TestBuildEntityByTableMap_NilInput(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	entityMap := svc.buildEntityByTableMap(nil)

	require.Empty(t, entityMap, "nil input should produce empty map")
}

// TestBuildExistingRelationshipSet_EmptyInput tests handling of empty relationship list
func TestBuildExistingRelationshipSet_EmptyInput(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	relSet := svc.buildExistingRelationshipSet([]*models.EntityRelationship{})

	require.Empty(t, relSet, "empty input should produce empty set")
}

// TestBuildExistingRelationshipSet_NilInput tests handling of nil relationship list
func TestBuildExistingRelationshipSet_NilInput(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	relSet := svc.buildExistingRelationshipSet(nil)

	require.Empty(t, relSet, "nil input should produce empty set")
}

// TestBuildEntityByTableMap_DuplicateTables tests that later entities override earlier ones
func TestBuildEntityByTableMap_DuplicateTables(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	firstEntity := &models.OntologyEntity{ID: uuid.New(), Name: "First", PrimaryTable: "shared_table"}
	secondEntity := &models.OntologyEntity{ID: uuid.New(), Name: "Second", PrimaryTable: "shared_table"}

	entityMap := svc.buildEntityByTableMap([]*models.OntologyEntity{firstEntity, secondEntity})

	require.Len(t, entityMap, 1, "duplicate tables should result in single entry")
	assert.Equal(t, secondEntity.ID, entityMap["shared_table"].ID, "later entity should override earlier one")
}

// TestBuildExistingRelationshipSet_KeyFormat tests the relationship key format
func TestBuildExistingRelationshipSet_KeyFormat(t *testing.T) {
	logger := zap.NewNop()

	svc := NewLLMRelationshipDiscoveryService(
		nil, nil, nil, nil, nil, nil, nil, nil, logger,
	).(*llmRelationshipDiscoveryService)

	rel := &models.EntityRelationship{
		SourceColumnTable: "source_table",
		SourceColumnName:  "source_col",
		TargetColumnTable: "target_table",
		TargetColumnName:  "target_col",
	}

	relSet := svc.buildExistingRelationshipSet([]*models.EntityRelationship{rel})

	// Key format should be "source_table.source_col->target_table.target_col"
	expectedKey := "source_table.source_col->target_table.target_col"
	assert.True(t, relSet[expectedKey], "key format should match expected pattern")
}
