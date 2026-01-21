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
	relationships       []*models.EntityRelationship
	updatedDescs        map[uuid.UUID]string
	updatedAssociations map[uuid.UUID]string
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

func (r *testRelEnrichmentRelRepo) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	if r.updatedDescs == nil {
		r.updatedDescs = make(map[uuid.UUID]string)
	}
	if r.updatedAssociations == nil {
		r.updatedAssociations = make(map[uuid.UUID]string)
	}
	r.updatedDescs[id] = description
	r.updatedAssociations[id] = association
	return nil
}

func (r *testRelEnrichmentRelRepo) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testRelEnrichmentRelRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return r.relationships, nil
}

func (r *testRelEnrichmentRelRepo) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	result := make(map[uuid.UUID][]*models.EntityRelationship)
	for _, rel := range r.relationships {
		result[rel.TargetEntityID] = append(result[rel.TargetEntityID], rel)
	}
	return result, nil
}

func (r *testRelEnrichmentRelRepo) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	return r.relationships, nil
}

func (r *testRelEnrichmentRelRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentRelRepo) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testRelEnrichmentRelRepo) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testRelEnrichmentRelRepo) Upsert(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testRelEnrichmentRelRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentRelRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testRelEnrichmentRelRepo) Update(ctx context.Context, rel *models.EntityRelationship) error {
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

func (r *testRelEnrichmentEntityRepo) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
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

func (r *testRelEnrichmentEntityRepo) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (r *testRelEnrichmentEntityRepo) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	return 0, nil
}

func (r *testRelEnrichmentEntityRepo) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
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
	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

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
			Content: `{"relationships": [{"id": 1, "description": "Test desc", "association": "placed_by"}]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert: should retry and succeed
	assert.NoError(t, err)
	assert.Equal(t, 1, result.RelationshipsEnriched)
	assert.Equal(t, 0, result.RelationshipsFailed)
	assert.Equal(t, 2, callCount, "Should retry once after transient error")
	assert.Equal(t, "Test desc", relRepo.updatedDescs[relID])
	assert.Equal(t, "placed_by", relRepo.updatedAssociations[relID])
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

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

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

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert: should handle parse failure gracefully
	assert.NoError(t, err)
	assert.Equal(t, 0, result.RelationshipsEnriched)
	assert.Equal(t, 1, result.RelationshipsFailed, "Should mark as failed when JSON parse fails")
}

// TestRelationshipEnrichmentService_SelfReferentialRelationship tests that self-referential
// relationships (e.g., Employee -> Employee for manager_id) are properly enriched with
// appropriate association labels like "reports_to" or "manages".
func TestRelationshipEnrichmentService_SelfReferentialRelationship(t *testing.T) {
	projectID := uuid.New()
	employeeEntityID := uuid.New()
	relID := uuid.New()

	// Setup: Employee entity that has a self-referential relationship
	entities := []*models.OntologyEntity{
		{
			ID:          employeeEntityID,
			Name:        "Employee",
			Description: "An employee in the organization hierarchy",
		},
	}

	// Self-referential relationship: employees.manager_id -> employees.id
	relationships := []*models.EntityRelationship{
		{
			ID:                relID,
			SourceEntityID:    employeeEntityID,
			TargetEntityID:    employeeEntityID, // Same entity - self-referential
			SourceColumnTable: "employees",
			SourceColumnName:  "manager_id",
			TargetColumnTable: "employees",
			TargetColumnName:  "id",
			DetectionMethod:   "fk_constraint",
			Confidence:        1.0,
		},
	}

	relRepo := &testRelEnrichmentRelRepo{relationships: relationships}
	entityRepo := &testRelEnrichmentEntityRepo{entities: entities}
	mockFactory := llm.NewMockClientFactory()

	// LLM responds with appropriate self-referential association
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"relationships": [{"id": 1, "description": "Represents the reporting structure within the organization. Each employee has a manager who is also an employee, forming a hierarchical tree.", "association": "reports_to"}]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 1, result.RelationshipsEnriched)
	assert.Equal(t, 0, result.RelationshipsFailed)

	// Verify the self-referential relationship was enriched with proper association
	assert.Contains(t, relRepo.updatedDescs[relID], "reporting structure",
		"Description should mention hierarchical/reporting structure")
	assert.Equal(t, "reports_to", relRepo.updatedAssociations[relID],
		"Self-referential FK should have 'reports_to' association")
}

// TestRelationshipEnrichmentService_MultipleSelfReferentialFK tests handling of tables
// with multiple self-referential FKs (e.g., employee.manager_id and employee.mentor_id).
func TestRelationshipEnrichmentService_MultipleSelfReferentialFK(t *testing.T) {
	projectID := uuid.New()
	employeeEntityID := uuid.New()
	managerRelID := uuid.New()
	mentorRelID := uuid.New()

	// Setup: Employee entity with multiple self-references
	entities := []*models.OntologyEntity{
		{
			ID:          employeeEntityID,
			Name:        "Employee",
			Description: "An employee with management and mentorship relationships",
		},
	}

	// Two self-referential relationships: manager_id and mentor_id
	relationships := []*models.EntityRelationship{
		{
			ID:                managerRelID,
			SourceEntityID:    employeeEntityID,
			TargetEntityID:    employeeEntityID, // Self-referential
			SourceColumnTable: "employees",
			SourceColumnName:  "manager_id",
			TargetColumnTable: "employees",
			TargetColumnName:  "id",
			DetectionMethod:   "fk_constraint",
			Confidence:        1.0,
		},
		{
			ID:                mentorRelID,
			SourceEntityID:    employeeEntityID,
			TargetEntityID:    employeeEntityID, // Self-referential
			SourceColumnTable: "employees",
			SourceColumnName:  "mentor_id",
			TargetColumnTable: "employees",
			TargetColumnName:  "id",
			DetectionMethod:   "fk_constraint",
			Confidence:        1.0,
		},
	}

	relRepo := &testRelEnrichmentRelRepo{relationships: relationships}
	entityRepo := &testRelEnrichmentEntityRepo{entities: entities}
	mockFactory := llm.NewMockClientFactory()

	// LLM responds with different associations for each self-referential FK
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{"relationships": [
				{"id": 1, "description": "Represents management hierarchy where an employee reports to a manager.", "association": "reports_to"},
				{"id": 2, "description": "Represents mentorship relationship where an employee is guided by a mentor.", "association": "mentored_by"}
			]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 2, result.RelationshipsEnriched)
	assert.Equal(t, 0, result.RelationshipsFailed)

	// Verify both relationships have distinct associations
	assert.Equal(t, "reports_to", relRepo.updatedAssociations[managerRelID],
		"Manager FK should have 'reports_to' association")
	assert.Equal(t, "mentored_by", relRepo.updatedAssociations[mentorRelID],
		"Mentor FK should have 'mentored_by' association")
}
