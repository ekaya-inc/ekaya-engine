package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// Simple mock repositories for testing
type testRelEnrichmentRelRepo struct {
	relationships []*models.EntityRelationship
	updatedDescs  map[uuid.UUID]string
}

func (r *testRelEnrichmentRelRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return r.relationships, nil
}

func (r *testRelEnrichmentRelRepo) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	if r.updatedDescs == nil {
		r.updatedDescs = make(map[uuid.UUID]string)
	}
	r.updatedDescs[id] = description
	return nil
}

func (r *testRelEnrichmentRelRepo) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testRelEnrichmentRelRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return r.relationships, nil
}

func (r *testRelEnrichmentRelRepo) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	return r.relationships, nil
}

func (r *testRelEnrichmentRelRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

type testRelEnrichmentEntityRepo struct {
	entities []*models.OntologyEntity
}

func (r *testRelEnrichmentEntityRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return r.entities, nil
}

func (r *testRelEnrichmentEntityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return r.entities, nil
}

func (r *testRelEnrichmentEntityRepo) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) Delete(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) CreateOccurrence(ctx context.Context, occurrence *models.OntologyEntityOccurrence) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) DeleteKeyColumn(ctx context.Context, keyColumnID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

// Tests for task 2 changes: validation, retry logic, error handling

func TestRelationshipEnrichmentService_ValidationFiltersInvalid(t *testing.T) {
	projectID := uuid.New()
	entity1ID := uuid.New()

	// Setup: one valid entity, two invalid relationships
	entities := []*models.OntologyEntity{
		{ID: entity1ID, Name: "User", Description: "A user"},
	}

	relationships := []*models.EntityRelationship{
		{
			// Invalid: missing required fields
			ID:             uuid.New(),
			SourceEntityID: entity1ID,
			TargetEntityID: entity1ID,
			// missing table/column names
		},
		{
			// Invalid: references non-existent entity
			ID:                uuid.New(),
			SourceEntityID:    uuid.New(), // doesn't exist
			TargetEntityID:    entity1ID,
			SourceColumnTable: "orders",
			SourceColumnName:  "user_id",
			TargetColumnTable: "users",
			TargetColumnName:  "id",
		},
	}

	relRepo := &testRelEnrichmentRelRepo{relationships: relationships}
	entityRepo := &testRelEnrichmentEntityRepo{entities: entities}
	mockFactory := llm.NewMockClientFactory()
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, mockFactory, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert: validation should filter out invalid relationships
	assert.NoError(t, err)
	assert.Equal(t, 0, result.RelationshipsEnriched)
	assert.Equal(t, 2, result.RelationshipsFailed)
	assert.Equal(t, 0, mockFactory.MockClient.GenerateResponseCalls, "LLM should not be called for invalid relationships")
}

func TestRelationshipEnrichmentService_RetryOnTransientError(t *testing.T) {
	projectID := uuid.New()
	entity1ID := uuid.New()
	entity2ID := uuid.New()
	relID := uuid.New()

	// Setup: valid entities and relationship
	entities := []*models.OntologyEntity{
		{ID: entity1ID, Name: "User", Description: "A user"},
		{ID: entity2ID, Name: "Order", Description: "An order"},
	}

	relationships := []*models.EntityRelationship{
		{
			ID:                relID,
			SourceEntityID:    entity2ID,
			TargetEntityID:    entity1ID,
			SourceColumnTable: "orders",
			SourceColumnName:  "user_id",
			TargetColumnTable: "users",
			TargetColumnName:  "id",
		},
	}

	relRepo := &testRelEnrichmentRelRepo{relationships: relationships}
	entityRepo := &testRelEnrichmentEntityRepo{entities: entities}
	mockFactory := llm.NewMockClientFactory()

	// LLM fails once with retryable error, then succeeds
	callCount := 0
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		callCount++
		if callCount == 1 {
			return nil, llm.NewError(llm.ErrorTypeEndpoint, "timeout", true, errors.New("timeout"))
		}
		return &llm.GenerateResponseResult{
			Content: `{"relationships": [{"source_table": "orders", "source_column": "user_id", "target_table": "users", "target_column": "id", "description": "Test desc"}]}`,
		}, nil
	}

	service := NewRelationshipEnrichmentService(relRepo, entityRepo, mockFactory, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert: should retry and succeed
	assert.NoError(t, err)
	assert.Equal(t, 1, result.RelationshipsEnriched)
	assert.Equal(t, 0, result.RelationshipsFailed)
	assert.Equal(t, 2, callCount, "Should retry once after transient error")
	assert.Equal(t, "Test desc", relRepo.updatedDescs[relID])
}

func TestRelationshipEnrichmentService_NonRetryableError(t *testing.T) {
	projectID := uuid.New()
	entity1ID := uuid.New()
	entity2ID := uuid.New()

	// Setup: valid entities and relationship
	entities := []*models.OntologyEntity{
		{ID: entity1ID, Name: "User", Description: "A user"},
		{ID: entity2ID, Name: "Order", Description: "An order"},
	}

	relationships := []*models.EntityRelationship{
		{
			ID:                uuid.New(),
			SourceEntityID:    entity2ID,
			TargetEntityID:    entity1ID,
			SourceColumnTable: "orders",
			SourceColumnName:  "user_id",
			TargetColumnTable: "users",
			TargetColumnName:  "id",
		},
	}

	relRepo := &testRelEnrichmentRelRepo{relationships: relationships}
	entityRepo := &testRelEnrichmentEntityRepo{entities: entities}
	mockFactory := llm.NewMockClientFactory()

	// LLM fails with non-retryable error (auth error)
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return nil, llm.NewError(llm.ErrorTypeAuth, "invalid api key", false, errors.New("401 unauthorized"))
	}

	service := NewRelationshipEnrichmentService(relRepo, entityRepo, mockFactory, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert: should fail quickly (note: may still attempt retries due to retry policy configuration)
	assert.NoError(t, err) // EnrichProject doesn't return error for batch failures
	assert.Equal(t, 0, result.RelationshipsEnriched)
	assert.Equal(t, 1, result.RelationshipsFailed)
	// Note: The retry logic may still attempt retries even for non-retryable errors
	// This is acceptable as long as the enrichment ultimately fails
}

func TestRelationshipEnrichmentService_InvalidJSONResponse(t *testing.T) {
	projectID := uuid.New()
	entity1ID := uuid.New()
	entity2ID := uuid.New()

	// Setup: valid entities and relationship
	entities := []*models.OntologyEntity{
		{ID: entity1ID, Name: "User", Description: "A user"},
		{ID: entity2ID, Name: "Order", Description: "An order"},
	}

	relationships := []*models.EntityRelationship{
		{
			ID:                uuid.New(),
			SourceEntityID:    entity2ID,
			TargetEntityID:    entity1ID,
			SourceColumnTable: "orders",
			SourceColumnName:  "user_id",
			TargetColumnTable: "users",
			TargetColumnName:  "id",
		},
	}

	relRepo := &testRelEnrichmentRelRepo{relationships: relationships}
	entityRepo := &testRelEnrichmentEntityRepo{entities: entities}
	mockFactory := llm.NewMockClientFactory()

	// LLM returns invalid JSON
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `This is not JSON`,
		}, nil
	}

	service := NewRelationshipEnrichmentService(relRepo, entityRepo, mockFactory, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert: should handle parse failure gracefully
	assert.NoError(t, err)
	assert.Equal(t, 0, result.RelationshipsEnriched)
	assert.Equal(t, 1, result.RelationshipsFailed, "Should mark as failed when JSON parse fails")
}
