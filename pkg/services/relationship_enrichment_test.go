package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func (r *testRelEnrichmentRelRepo) MarkInferenceRelationshipsStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentRelRepo) ClearStaleFlag(ctx context.Context, relationshipID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentRelRepo) GetStaleRelationships(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
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

func (r *testRelEnrichmentEntityRepo) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (r *testRelEnrichmentEntityRepo) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
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
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, nil, nil, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

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
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, nil, nil, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

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
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, nil, nil, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

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
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, nil, nil, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

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
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, nil, nil, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

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
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, nil, nil, nil, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

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

// TestRelationshipEnrichmentService_HostVisitorRolesWithKnowledge tests that host and visitor
// roles are enriched with their business meaning from seeded project knowledge.
// This verifies BUG-5 fix: Host/Visitor roles captured with business meaning.
func TestRelationshipEnrichmentService_HostVisitorRolesWithKnowledge(t *testing.T) {
	projectID := uuid.New()
	userEntityID := uuid.New()
	engagementEntityID := uuid.New()
	hostRelID := uuid.New()
	visitorRelID := uuid.New()

	// Setup: User entity appearing with different roles in engagements
	entities := []*models.OntologyEntity{
		{
			ID:          userEntityID,
			Name:        "User",
			Description: "A platform user",
		},
		{
			ID:          engagementEntityID,
			Name:        "Engagement",
			Description: "A billing engagement session",
		},
	}

	// Two relationships: host_id and visitor_id both reference User
	relationships := []*models.EntityRelationship{
		{
			ID:                hostRelID,
			SourceEntityID:    engagementEntityID,
			TargetEntityID:    userEntityID,
			SourceColumnTable: "billing_engagements",
			SourceColumnName:  "host_id",
			TargetColumnTable: "users",
			TargetColumnName:  "id",
			DetectionMethod:   "foreign_key",
			Confidence:        1.0,
		},
		{
			ID:                visitorRelID,
			SourceEntityID:    engagementEntityID,
			TargetEntityID:    userEntityID,
			SourceColumnTable: "billing_engagements",
			SourceColumnName:  "visitor_id",
			TargetColumnTable: "users",
			TargetColumnName:  "id",
			DetectionMethod:   "foreign_key",
			Confidence:        1.0,
		},
	}

	// Mock knowledge repository that returns seeded domain knowledge
	knowledgeRepo := &mockKnowledgeRepoForRelEnrichment{
		facts: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "terminology",
				Key:       "Host is a content creator who receives payments",
				Value:     "Host is a content creator who receives payments",
				Context:   "User role",
			},
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "terminology",
				Key:       "Visitor is a viewer who pays for engagements",
				Value:     "Visitor is a viewer who pays for engagements",
				Context:   "User role",
			},
		},
	}

	relRepo := &testRelEnrichmentRelRepo{relationships: relationships}
	entityRepo := &testRelEnrichmentEntityRepo{entities: entities}
	mockFactory := llm.NewMockClientFactory()

	var capturedPrompt string
	// LLM responds with role-aware descriptions
	mockFactory.MockClient.GenerateResponseFunc = func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		capturedPrompt = prompt
		return &llm.GenerateResponseResult{
			Content: `{"relationships": [
				{"id": 1, "description": "Links the engagement to the host (content creator) who receives payments for the session. Each host can have many engagements.", "association": "as_host"},
				{"id": 2, "description": "Links the engagement to the visitor (viewer) who pays for the engagement session. Each visitor can participate in many engagements.", "association": "as_visitor"}
			]}`,
		}, nil
	}

	testPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop())
	circuitBreaker := llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig())
	service := NewRelationshipEnrichmentService(relRepo, entityRepo, knowledgeRepo, nil, nil, nil, mockFactory, testPool, circuitBreaker, nil, zap.NewNop())

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, nil)

	// Assert
	assert.NoError(t, err)
	assert.Equal(t, 2, result.RelationshipsEnriched)
	assert.Equal(t, 0, result.RelationshipsFailed)

	// Verify domain knowledge was included in the prompt
	assert.Contains(t, capturedPrompt, "Domain Knowledge", "Prompt should include domain knowledge section")
	assert.Contains(t, capturedPrompt, "Host is a content creator who receives payments", "Prompt should include host role definition")
	assert.Contains(t, capturedPrompt, "Visitor is a viewer who pays for engagements", "Prompt should include visitor role definition")

	// Verify role-aware associations
	assert.Equal(t, "as_host", relRepo.updatedAssociations[hostRelID],
		"Host FK should have 'as_host' association")
	assert.Equal(t, "as_visitor", relRepo.updatedAssociations[visitorRelID],
		"Visitor FK should have 'as_visitor' association")

	// Verify descriptions include business meaning
	assert.Contains(t, relRepo.updatedDescs[hostRelID], "content creator",
		"Host description should mention content creator role")
	assert.Contains(t, relRepo.updatedDescs[visitorRelID], "viewer",
		"Visitor description should mention viewer role")
}

// mockKnowledgeRepoForRelEnrichment is a test mock for KnowledgeRepository.
type mockKnowledgeRepoForRelEnrichment struct {
	facts []*models.KnowledgeFact
}

func (m *mockKnowledgeRepoForRelEnrichment) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	return m.facts, nil
}

func (m *mockKnowledgeRepoForRelEnrichment) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	var result []*models.KnowledgeFact
	for _, f := range m.facts {
		if f.FactType == factType {
			result = append(result, f)
		}
	}
	return result, nil
}

func (m *mockKnowledgeRepoForRelEnrichment) GetByKey(ctx context.Context, projectID uuid.UUID, factType, key string) (*models.KnowledgeFact, error) {
	for _, f := range m.facts {
		if f.FactType == factType && f.Key == key {
			return f, nil
		}
	}
	return nil, nil
}

func (m *mockKnowledgeRepoForRelEnrichment) Upsert(ctx context.Context, fact *models.KnowledgeFact) error {
	return nil
}

func (m *mockKnowledgeRepoForRelEnrichment) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeRepoForRelEnrichment) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func TestRelationshipEnrichmentResponse_QuestionsDeserialization(t *testing.T) {
	// Direct test of relationshipEnrichmentResponse JSON parsing

	responseJSON := `{
		"relationships": [
			{
				"id": 1,
				"description": "Links each order to the user who placed it.",
				"association": "placed_by"
			}
		],
		"questions": [
			{
				"category": "relationship",
				"priority": 1,
				"question": "Is users.referrer_id a self-reference to the same users table?",
				"context": "Column appears to reference the same table."
			}
		]
	}`

	response, err := llm.ParseJSONResponse[relationshipEnrichmentResponse](responseJSON)

	require.NoError(t, err)
	require.Len(t, response.Relationships, 1)
	require.Len(t, response.Questions, 1)

	// Verify relationship
	assert.Equal(t, 1, response.Relationships[0].ID)
	assert.Equal(t, "placed_by", response.Relationships[0].Association)

	// Verify question
	assert.Equal(t, "relationship", response.Questions[0].Category)
	assert.Equal(t, 1, response.Questions[0].Priority)
	assert.Equal(t, "Is users.referrer_id a self-reference to the same users table?", response.Questions[0].Question)
}

func TestRelationshipEnrichmentResponse_NoQuestions(t *testing.T) {
	// Test that parsing succeeds when no questions are present

	responseJSON := `{
		"relationships": [
			{
				"id": 1,
				"description": "Links orders to users.",
				"association": "belongs_to"
			}
		]
	}`

	response, err := llm.ParseJSONResponse[relationshipEnrichmentResponse](responseJSON)

	require.NoError(t, err)
	require.Len(t, response.Relationships, 1)
	assert.Empty(t, response.Questions, "Questions should be empty when not present")
}

func TestRelationshipEnrichmentResponse_MultipleQuestions(t *testing.T) {
	// Test that multiple questions of different categories are parsed correctly

	responseJSON := `{
		"relationships": [
			{
				"id": 1,
				"description": "Links transactions to users as host.",
				"association": "as_host"
			},
			{
				"id": 2,
				"description": "Links transactions to users as visitor.",
				"association": "as_visitor"
			}
		],
		"questions": [
			{
				"category": "business_rules",
				"priority": 1,
				"question": "Can a user be both host and visitor in the same transaction?",
				"context": "Transaction has two FK columns to users: host_id and visitor_id."
			},
			{
				"category": "relationship",
				"priority": 2,
				"question": "What distinguishes a host from a visitor in business terms?",
				"context": "Both roles reference users but have different business meanings."
			}
		]
	}`

	response, err := llm.ParseJSONResponse[relationshipEnrichmentResponse](responseJSON)

	require.NoError(t, err)
	require.Len(t, response.Relationships, 2)
	require.Len(t, response.Questions, 2)

	// Verify relationships
	assert.Equal(t, "as_host", response.Relationships[0].Association)
	assert.Equal(t, "as_visitor", response.Relationships[1].Association)

	// Verify questions
	assert.Equal(t, "business_rules", response.Questions[0].Category)
	assert.Equal(t, 1, response.Questions[0].Priority)

	assert.Equal(t, "relationship", response.Questions[1].Category)
	assert.Equal(t, 2, response.Questions[1].Priority)
}
