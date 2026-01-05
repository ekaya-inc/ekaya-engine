package services

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Mock Implementations for Entity Service Tests
// ============================================================================

type mockEntityRepo struct {
	entities        map[uuid.UUID]*models.OntologyEntity
	aliases         map[uuid.UUID][]*models.OntologyEntityAlias
	getByOntologyFn func(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error)
	getByIDErr      error
	getAliasesErr   error
}

func newMockEntityRepo() *mockEntityRepo {
	return &mockEntityRepo{
		entities: make(map[uuid.UUID]*models.OntologyEntity),
		aliases:  make(map[uuid.UUID][]*models.OntologyEntityAlias),
	}
}

func (m *mockEntityRepo) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	if m.getByIDErr != nil {
		return nil, m.getByIDErr
	}
	entity, ok := m.entities[entityID]
	if !ok {
		return nil, nil
	}
	return entity, nil
}

func (m *mockEntityRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	if m.getByOntologyFn != nil {
		return m.getByOntologyFn(ctx, ontologyID)
	}
	var result []*models.OntologyEntity
	for _, entity := range m.entities {
		if entity.OntologyID == ontologyID && !entity.IsDeleted {
			result = append(result, entity)
		}
	}
	return result, nil
}

func (m *mockEntityRepo) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	if m.getAliasesErr != nil {
		return nil, m.getAliasesErr
	}
	return m.aliases[entityID], nil
}

// Stub methods to satisfy interface
func (m *mockEntityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}
func (m *mockEntityRepo) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}
func (m *mockEntityRepo) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}
func (m *mockEntityRepo) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}
func (m *mockEntityRepo) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}
func (m *mockEntityRepo) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}
func (m *mockEntityRepo) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}
func (m *mockEntityRepo) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}
func (m *mockEntityRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityRepo) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockEntityRepo) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}
func (m *mockEntityRepo) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

type mockRelationshipRepo struct {
	relationships       map[uuid.UUID][]*models.EntityRelationship // keyed by target entity ID
	getByTargetEntityFn func(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error)
}

func newMockRelationshipRepo() *mockRelationshipRepo {
	return &mockRelationshipRepo{
		relationships: make(map[uuid.UUID][]*models.EntityRelationship),
	}
}

func (m *mockRelationshipRepo) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	if m.getByTargetEntityFn != nil {
		return m.getByTargetEntityFn(ctx, entityID)
	}
	return m.relationships[entityID], nil
}

// Stub methods to satisfy interface
func (m *mockRelationshipRepo) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}
func (m *mockRelationshipRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockRelationshipRepo) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	return m.relationships, nil
}
func (m *mockRelationshipRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockRelationshipRepo) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	return nil, nil
}
func (m *mockRelationshipRepo) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	return nil
}
func (m *mockRelationshipRepo) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	return nil
}
func (m *mockRelationshipRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

type mockOntologyRepo struct {
	ontologies  map[uuid.UUID]*models.TieredOntology
	getActiveFn func(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error)
}

func newMockOntologyRepo() *mockOntologyRepo {
	return &mockOntologyRepo{
		ontologies: make(map[uuid.UUID]*models.TieredOntology),
	}
}

func (m *mockOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getActiveFn != nil {
		return m.getActiveFn(ctx, projectID)
	}
	for _, onto := range m.ontologies {
		if onto.ProjectID == projectID && onto.IsActive {
			return onto, nil
		}
	}
	return nil, nil
}

// Stub methods to satisfy interface
func (m *mockOntologyRepo) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}
func (m *mockOntologyRepo) GetByVersion(ctx context.Context, projectID uuid.UUID, version int) (*models.TieredOntology, error) {
	return nil, nil
}
func (m *mockOntologyRepo) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}
func (m *mockOntologyRepo) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return nil
}
func (m *mockOntologyRepo) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}
func (m *mockOntologyRepo) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}
func (m *mockOntologyRepo) UpdateMetadata(ctx context.Context, projectID uuid.UUID, metadata map[string]any) error {
	return nil
}
func (m *mockOntologyRepo) SetActive(ctx context.Context, projectID uuid.UUID, version int) error {
	return nil
}
func (m *mockOntologyRepo) DeactivateAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}
func (m *mockOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}
func (m *mockOntologyRepo) WriteCleanOntology(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// ============================================================================
// Tests for computeOccurrences
// ============================================================================

func TestComputeOccurrences_NoRelationships(t *testing.T) {
	entityRepo := newMockEntityRepo()
	relationshipRepo := newMockRelationshipRepo()
	ontologyRepo := newMockOntologyRepo()

	service := NewEntityService(entityRepo, relationshipRepo, ontologyRepo, zap.NewNop()).(*entityService)

	entityID := uuid.New()

	// No relationships for this entity
	occurrences, err := service.computeOccurrences(context.Background(), entityID)

	require.NoError(t, err)
	assert.Empty(t, occurrences, "expected no occurrences when no relationships exist")
}

func TestComputeOccurrences_WithRelationships(t *testing.T) {
	entityRepo := newMockEntityRepo()
	relationshipRepo := newMockRelationshipRepo()
	ontologyRepo := newMockOntologyRepo()

	service := NewEntityService(entityRepo, relationshipRepo, ontologyRepo, zap.NewNop()).(*entityService)

	entityID := uuid.New()
	rel1ID := uuid.New()
	rel2ID := uuid.New()

	association1 := "placed_by"
	association2 := "owned_by"

	// Setup relationships where entityID is the target
	relationshipRepo.relationships[entityID] = []*models.EntityRelationship{
		{
			ID:                 rel1ID,
			TargetEntityID:     entityID,
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			Association:        &association1,
			Confidence:         0.95,
			CreatedAt:          time.Now(),
		},
		{
			ID:                 rel2ID,
			TargetEntityID:     entityID,
			SourceColumnSchema: "public",
			SourceColumnTable:  "accounts",
			SourceColumnName:   "owner_id",
			Association:        &association2,
			Confidence:         1.0,
			CreatedAt:          time.Now(),
		},
	}

	occurrences, err := service.computeOccurrences(context.Background(), entityID)

	require.NoError(t, err)
	require.Len(t, occurrences, 2, "expected 2 occurrences from 2 relationships")

	// Check first occurrence
	assert.Equal(t, rel1ID, occurrences[0].ID)
	assert.Equal(t, entityID, occurrences[0].EntityID)
	assert.Equal(t, "public", occurrences[0].SchemaName)
	assert.Equal(t, "orders", occurrences[0].TableName)
	assert.Equal(t, "user_id", occurrences[0].ColumnName)
	assert.Equal(t, &association1, occurrences[0].Association)
	assert.Equal(t, 0.95, occurrences[0].Confidence)

	// Check second occurrence
	assert.Equal(t, rel2ID, occurrences[1].ID)
	assert.Equal(t, entityID, occurrences[1].EntityID)
	assert.Equal(t, "public", occurrences[1].SchemaName)
	assert.Equal(t, "accounts", occurrences[1].TableName)
	assert.Equal(t, "owner_id", occurrences[1].ColumnName)
	assert.Equal(t, &association2, occurrences[1].Association)
	assert.Equal(t, 1.0, occurrences[1].Confidence)
}

func TestComputeOccurrences_WithNullAssociation(t *testing.T) {
	entityRepo := newMockEntityRepo()
	relationshipRepo := newMockRelationshipRepo()
	ontologyRepo := newMockOntologyRepo()

	service := NewEntityService(entityRepo, relationshipRepo, ontologyRepo, zap.NewNop()).(*entityService)

	entityID := uuid.New()
	relID := uuid.New()

	// Relationship with null association (not yet enriched)
	relationshipRepo.relationships[entityID] = []*models.EntityRelationship{
		{
			ID:                 relID,
			TargetEntityID:     entityID,
			SourceColumnSchema: "public",
			SourceColumnTable:  "visits",
			SourceColumnName:   "visitor_id",
			Association:        nil, // Not yet enriched
			Confidence:         1.0,
			CreatedAt:          time.Now(),
		},
	}

	occurrences, err := service.computeOccurrences(context.Background(), entityID)

	require.NoError(t, err)
	require.Len(t, occurrences, 1)
	assert.Nil(t, occurrences[0].Association, "expected nil association when association is nil")
}

func TestComputeOccurrences_RepositoryError(t *testing.T) {
	entityRepo := newMockEntityRepo()
	relationshipRepo := newMockRelationshipRepo()
	ontologyRepo := newMockOntologyRepo()

	service := NewEntityService(entityRepo, relationshipRepo, ontologyRepo, zap.NewNop()).(*entityService)

	entityID := uuid.New()

	// Setup error
	relationshipRepo.getByTargetEntityFn = func(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
		return nil, errors.New("database error")
	}

	occurrences, err := service.computeOccurrences(context.Background(), entityID)

	assert.Error(t, err)
	assert.Nil(t, occurrences)
	assert.Contains(t, err.Error(), "get relationships by target entity")
}

// ============================================================================
// Integration tests for ListByProject
// ============================================================================

func TestListByProject_ComputesOccurrences(t *testing.T) {
	entityRepo := newMockEntityRepo()
	relationshipRepo := newMockRelationshipRepo()
	ontologyRepo := newMockOntologyRepo()

	service := NewEntityService(entityRepo, relationshipRepo, ontologyRepo, zap.NewNop())

	projectID := uuid.New()
	ontologyID := uuid.New()
	entity1ID := uuid.New()
	entity2ID := uuid.New()

	// Setup active ontology
	ontologyRepo.ontologies[ontologyID] = &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
	}

	// Setup entities
	entityRepo.entities[entity1ID] = &models.OntologyEntity{
		ID:         entity1ID,
		OntologyID: ontologyID,
		Name:       "user",
		IsDeleted:  false,
	}
	entityRepo.entities[entity2ID] = &models.OntologyEntity{
		ID:         entity2ID,
		OntologyID: ontologyID,
		Name:       "order",
		IsDeleted:  false,
	}

	// Setup relationships (entity1 has 2 occurrences, entity2 has 0)
	association := "placed_by"
	relationshipRepo.relationships[entity1ID] = []*models.EntityRelationship{
		{
			ID:                 uuid.New(),
			TargetEntityID:     entity1ID,
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			Association:        &association,
			Confidence:         1.0,
			CreatedAt:          time.Now(),
		},
		{
			ID:                 uuid.New(),
			TargetEntityID:     entity1ID,
			SourceColumnSchema: "public",
			SourceColumnTable:  "visits",
			SourceColumnName:   "visitor_id",
			Association:        nil,
			Confidence:         1.0,
			CreatedAt:          time.Now(),
		},
	}

	// Setup aliases (empty for simplicity)
	entityRepo.aliases[entity1ID] = []*models.OntologyEntityAlias{}
	entityRepo.aliases[entity2ID] = []*models.OntologyEntityAlias{}

	result, err := service.ListByProject(context.Background(), projectID)

	require.NoError(t, err)
	require.Len(t, result, 2)

	// Find entities in result (order not guaranteed)
	var user, order *EntityWithDetails
	for _, ewd := range result {
		if ewd.Entity.Name == "user" {
			user = ewd
		} else if ewd.Entity.Name == "order" {
			order = ewd
		}
	}

	require.NotNil(t, user, "expected user entity in result")
	require.NotNil(t, order, "expected order entity in result")

	// Verify occurrence counts
	assert.Equal(t, 2, user.OccurrenceCount, "user should have 2 occurrences")
	assert.Len(t, user.Occurrences, 2)
	assert.Equal(t, 0, order.OccurrenceCount, "order should have 0 occurrences")
	assert.Empty(t, order.Occurrences)
}

func TestGetByID_ComputesOccurrences(t *testing.T) {
	entityRepo := newMockEntityRepo()
	relationshipRepo := newMockRelationshipRepo()
	ontologyRepo := newMockOntologyRepo()

	service := NewEntityService(entityRepo, relationshipRepo, ontologyRepo, zap.NewNop())

	entityID := uuid.New()

	// Setup entity
	entityRepo.entities[entityID] = &models.OntologyEntity{
		ID:        entityID,
		Name:      "user",
		IsDeleted: false,
	}

	// Setup relationship
	association := "placed_by"
	relationshipRepo.relationships[entityID] = []*models.EntityRelationship{
		{
			ID:                 uuid.New(),
			TargetEntityID:     entityID,
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			Association:        &association,
			Confidence:         1.0,
			CreatedAt:          time.Now(),
		},
	}

	// Setup aliases
	entityRepo.aliases[entityID] = []*models.OntologyEntityAlias{}

	result, err := service.GetByID(context.Background(), entityID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, 1, result.OccurrenceCount)
	assert.Len(t, result.Occurrences, 1)
	assert.Equal(t, "orders", result.Occurrences[0].TableName)
	assert.Equal(t, &association, result.Occurrences[0].Association)
}
