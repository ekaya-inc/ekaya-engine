package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Mock Implementations
// ============================================================================

type mockOntologyRepository struct {
	activeOntology *models.TieredOntology
	getActiveErr   error
}

func (m *mockOntologyRepository) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockOntologyRepository) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getActiveErr != nil {
		return nil, m.getActiveErr
	}
	return m.activeOntology, nil
}

func (m *mockOntologyRepository) GetByVersion(ctx context.Context, projectID uuid.UUID, version int) (*models.TieredOntology, error) {
	return nil, nil
}

func (m *mockOntologyRepository) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}

func (m *mockOntologyRepository) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return nil
}

func (m *mockOntologyRepository) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}

func (m *mockOntologyRepository) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}

func (m *mockOntologyRepository) UpdateMetadata(ctx context.Context, projectID uuid.UUID, metadata map[string]any) error {
	return nil
}

func (m *mockOntologyRepository) SetActive(ctx context.Context, projectID uuid.UUID, version int) error {
	return nil
}

func (m *mockOntologyRepository) DeactivateAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockOntologyRepository) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockOntologyRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockOntologyRepository) WriteCleanOntology(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

type mockOntologyEntityRepository struct {
	entities          []*models.OntologyEntity
	occurrences       []*models.OntologyEntityOccurrence
	aliases           map[uuid.UUID][]*models.OntologyEntityAlias
	keyColumns        map[uuid.UUID][]*models.OntologyEntityKeyColumn
	getByProjectErr   error
	getOccurrencesErr error
	getAliasesErr     error
}

func (m *mockOntologyEntityRepository) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockOntologyEntityRepository) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockOntologyEntityRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return m.entities, nil
}

func (m *mockOntologyEntityRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	if m.getByProjectErr != nil {
		return nil, m.getByProjectErr
	}
	return m.entities, nil
}

func (m *mockOntologyEntityRepository) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockOntologyEntityRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockOntologyEntityRepository) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockOntologyEntityRepository) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (m *mockOntologyEntityRepository) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockOntologyEntityRepository) CreateOccurrence(ctx context.Context, occ *models.OntologyEntityOccurrence) error {
	return nil
}

func (m *mockOntologyEntityRepository) GetOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (m *mockOntologyEntityRepository) GetOccurrencesByTable(ctx context.Context, ontologyID uuid.UUID, schema, table string) ([]*models.OntologyEntityOccurrence, error) {
	return nil, nil
}

func (m *mockOntologyEntityRepository) GetAllOccurrencesByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntityOccurrence, error) {
	if m.getOccurrencesErr != nil {
		return nil, m.getOccurrencesErr
	}
	return m.occurrences, nil
}

func (m *mockOntologyEntityRepository) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}

func (m *mockOntologyEntityRepository) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	if m.getAliasesErr != nil {
		return nil, m.getAliasesErr
	}
	if m.aliases != nil {
		return m.aliases[entityID], nil
	}
	return []*models.OntologyEntityAlias{}, nil
}

func (m *mockOntologyEntityRepository) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}

func (m *mockOntologyEntityRepository) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	if m.getAliasesErr != nil {
		return nil, m.getAliasesErr
	}
	if m.aliases != nil {
		return m.aliases, nil
	}
	return make(map[uuid.UUID][]*models.OntologyEntityAlias), nil
}

func (m *mockOntologyEntityRepository) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}

func (m *mockOntologyEntityRepository) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (m *mockOntologyEntityRepository) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	if m.keyColumns != nil {
		return m.keyColumns, nil
	}
	return make(map[uuid.UUID][]*models.OntologyEntityKeyColumn), nil
}

// mockEntityRelationshipRepository is a mock for EntityRelationshipRepository.
type mockEntityRelationshipRepository struct {
	relationships   []*models.EntityRelationship
	getByProjectErr error
	getByTablesErr  error
}

func (m *mockEntityRelationshipRepository) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (m *mockEntityRelationshipRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return m.relationships, nil
}

func (m *mockEntityRelationshipRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	if m.getByProjectErr != nil {
		return nil, m.getByProjectErr
	}
	return m.relationships, nil
}

func (m *mockEntityRelationshipRepository) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	if m.getByTablesErr != nil {
		return nil, m.getByTablesErr
	}
	return m.relationships, nil
}

func (m *mockEntityRelationshipRepository) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	return nil
}

func (m *mockEntityRelationshipRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

// mockProjectServiceForOntology is a mock for ProjectService in ontology context tests.
type mockProjectServiceForOntology struct {
	datasourceID uuid.UUID
	err          error
}

func (m *mockProjectServiceForOntology) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForOntology) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForOntology) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectServiceForOntology) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectServiceForOntology) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockProjectServiceForOntology) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	if m.err != nil {
		return uuid.Nil, m.err
	}
	return m.datasourceID, nil
}

func (m *mockProjectServiceForOntology) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockProjectServiceForOntology) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
	// No-op for tests
}

// ============================================================================
// Tests
// ============================================================================

func TestGetDomainContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()
	entityID2 := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		Version:   1,
		IsActive:  true,
		DomainSummary: &models.DomainSummary{
			Description: "E-commerce platform",
			Domains:     []string{"sales", "customer"},
			RelationshipGraph: []models.RelationshipEdge{
				{From: "user", To: "order", Label: "places", Cardinality: "1:N"},
			},
		},
		EntitySummaries: map[string]*models.EntitySummary{
			"users":  {TableName: "users", BusinessName: "Users"},
			"orders": {TableName: "orders", BusinessName: "Orders"},
		},
		ColumnDetails: map[string][]models.ColumnDetail{
			"users":  {{Name: "id"}, {Name: "email"}},
			"orders": {{Name: "id"}, {Name: "user_id"}, {Name: "total"}},
		},
	}

	entities := []*models.OntologyEntity{
		{
			ID:            entityID1,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "user",
			Description:   "Platform user",
			PrimarySchema: "public",
			PrimaryTable:  "users",
			PrimaryColumn: "id",
		},
		{
			ID:            entityID2,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "order",
			Description:   "Customer order",
			PrimarySchema: "public",
			PrimaryTable:  "orders",
			PrimaryColumn: "id",
		},
	}

	occurrences := []*models.OntologyEntityOccurrence{
		{ID: uuid.New(), EntityID: entityID1, TableName: "users", ColumnName: "id"},
		{ID: uuid.New(), EntityID: entityID1, TableName: "orders", ColumnName: "user_id"},
		{ID: uuid.New(), EntityID: entityID2, TableName: "orders", ColumnName: "id"},
	}

	// Create relationship for testing
	desc := "user places order"
	relationships := []*models.EntityRelationship{
		{
			ID:                uuid.New(),
			SourceEntityID:    entityID1,
			TargetEntityID:    entityID2,
			SourceColumnTable: "users",
			SourceColumnName:  "id",
			TargetColumnTable: "orders",
			TargetColumnName:  "user_id",
			Description:       &desc,
		},
	}

	ontologyRepo := &mockOntologyRepository{
		activeOntology: ontology,
	}
	entityRepo := &mockOntologyEntityRepository{
		entities:    entities,
		occurrences: occurrences,
	}
	relationshipRepo := &mockEntityRelationshipRepository{
		relationships: relationships,
	}
	schemaRepo := &mockSchemaRepository{
		columns: []*models.SchemaColumn{
			{ColumnName: "id", SchemaTableID: uuid.New()},
			{ColumnName: "email", SchemaTableID: uuid.New()},
			{ColumnName: "id", SchemaTableID: uuid.New()},
			{ColumnName: "user_id", SchemaTableID: uuid.New()},
			{ColumnName: "total", SchemaTableID: uuid.New()},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify domain info
	assert.Equal(t, "E-commerce platform", result.Domain.Description)
	assert.Equal(t, []string{"sales", "customer"}, result.Domain.PrimaryDomains)
	assert.Equal(t, 2, result.Domain.TableCount)  // Number of entities
	assert.Equal(t, 5, result.Domain.ColumnCount) // From schema columns

	// Verify entities
	assert.Len(t, result.Entities, 2)
	assert.Equal(t, "user", result.Entities[0].Name)
	assert.Equal(t, "Platform user", result.Entities[0].Description)
	assert.Equal(t, "users", result.Entities[0].PrimaryTable)
	assert.Equal(t, 2, result.Entities[0].OccurrenceCount)

	// Verify relationships from normalized table
	assert.Len(t, result.Relationships, 1)
	assert.Equal(t, "user", result.Relationships[0].From)
	assert.Equal(t, "order", result.Relationships[0].To)
	assert.Equal(t, "user places order", result.Relationships[0].Label)
}

func TestGetDomainContext_NoActiveOntology(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepository{
		activeOntology: nil,
	}
	entityRepo := &mockOntologyEntityRepository{}
	relationshipRepo := &mockEntityRelationshipRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no active ontology found")
}

func TestGetEntitiesContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		EntitySummaries: map[string]*models.EntitySummary{
			"users": {
				TableName:    "users",
				BusinessName: "Users",
				KeyColumns: []models.KeyColumn{
					{Name: "id", Synonyms: []string{"user_id"}},
					{Name: "email", Synonyms: []string{"email_address"}},
				},
			},
		},
	}

	entities := []*models.OntologyEntity{
		{
			ID:            entityID1,
			Name:          "user",
			Description:   "Platform user",
			PrimaryTable:  "users",
			PrimaryColumn: "id",
		},
	}

	role := "customer"
	occurrences := []*models.OntologyEntityOccurrence{
		{ID: uuid.New(), EntityID: entityID1, TableName: "users", ColumnName: "id"},
		{ID: uuid.New(), EntityID: entityID1, TableName: "orders", ColumnName: "user_id", Role: &role},
	}

	aliases := map[uuid.UUID][]*models.OntologyEntityAlias{
		entityID1: {
			{ID: uuid.New(), EntityID: entityID1, Alias: "customer"},
			{ID: uuid.New(), EntityID: entityID1, Alias: "account"},
		},
	}

	keyColumns := map[uuid.UUID][]*models.OntologyEntityKeyColumn{
		entityID1: {
			{ID: uuid.New(), EntityID: entityID1, ColumnName: "id", Synonyms: []string{"user_id"}},
			{ID: uuid.New(), EntityID: entityID1, ColumnName: "email", Synonyms: []string{"email_address"}},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{
		entities:    entities,
		occurrences: occurrences,
		aliases:     aliases,
		keyColumns:  keyColumns,
	}
	relationshipRepo := &mockEntityRelationshipRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	result, err := svc.GetEntitiesContext(ctx, projectID)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify entity details
	assert.Len(t, result.Entities, 1)
	userEntity := result.Entities["user"]
	assert.Equal(t, "users", userEntity.PrimaryTable)
	assert.Equal(t, "Platform user", userEntity.Description)
	assert.Equal(t, []string{"customer", "account"}, userEntity.Synonyms)

	// Verify key columns
	assert.Len(t, userEntity.KeyColumns, 2)
	assert.Equal(t, "id", userEntity.KeyColumns[0].Name)
	assert.Equal(t, []string{"user_id"}, userEntity.KeyColumns[0].Synonyms)

	// Verify occurrences
	assert.Len(t, userEntity.Occurrences, 2)
	assert.Equal(t, "users", userEntity.Occurrences[0].Table)
	assert.Equal(t, "id", userEntity.Occurrences[0].Column)
	assert.Nil(t, userEntity.Occurrences[0].Role)
	assert.Equal(t, "customer", *userEntity.Occurrences[1].Role)
}

func TestGetTablesContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
	}

	// Entities provide business name and description
	entities := []*models.OntologyEntity{
		{
			ID:            entityID1,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "user",
			Description:   "Platform users",
			PrimarySchema: "public",
			PrimaryTable:  "users",
			PrimaryColumn: "id",
		},
	}

	// Entity aliases provide synonyms
	aliases := map[uuid.UUID][]*models.OntologyEntityAlias{
		entityID1: {
			{ID: uuid.New(), EntityID: entityID1, Alias: "accounts"},
			{ID: uuid.New(), EntityID: entityID1, Alias: "members"},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{
		entities: entities,
		aliases:  aliases,
	}
	relationshipRepo := &mockEntityRelationshipRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	// Test with specific table filter
	result, err := svc.GetTablesContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	// Verify users table - business name comes from entity name, description from entity
	usersTable := result.Tables["users"]
	assert.Equal(t, "user", usersTable.BusinessName)          // Entity name
	assert.Equal(t, "Platform users", usersTable.Description) // Entity description
	assert.Equal(t, "public", usersTable.Schema)
	assert.Equal(t, []string{"accounts", "members"}, usersTable.Synonyms) // From aliases
}

func TestGetTablesContext_AllTables(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()
	entityID2 := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
	}

	// Entities - when no filter, all entity tables are returned
	entities := []*models.OntologyEntity{
		{
			ID:            entityID1,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "user",
			Description:   "Users",
			PrimarySchema: "public",
			PrimaryTable:  "users",
			PrimaryColumn: "id",
		},
		{
			ID:            entityID2,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "order",
			Description:   "Orders",
			PrimarySchema: "public",
			PrimaryTable:  "orders",
			PrimaryColumn: "id",
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{entities: entities}
	relationshipRepo := &mockEntityRelationshipRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	// Test without filter - should return all entity tables
	result, err := svc.GetTablesContext(ctx, projectID, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 2)
}

func TestGetColumnsContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
	}

	// Entity provides business name and description
	entities := []*models.OntologyEntity{
		{
			ID:            entityID1,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "user",
			Description:   "Platform users",
			PrimarySchema: "public",
			PrimaryTable:  "users",
			PrimaryColumn: "id",
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{entities: entities}
	relationshipRepo := &mockEntityRelationshipRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	// Verify users table - business name/description from entity
	usersTable := result.Tables["users"]
	assert.Equal(t, "user", usersTable.BusinessName)
	assert.Equal(t, "Platform users", usersTable.Description)
	assert.Equal(t, "public", usersTable.Schema)
	// Note: Columns come from schema_columns via GetColumnsByTables mock which returns empty map
	// Semantic fields (description, synonyms, etc.) are empty until Column Workflow (Phase 4)
}

func TestGetColumnsContext_RequiresTableFilter(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepository{}
	entityRepo := &mockOntologyEntityRepository{}
	relationshipRepo := &mockEntityRelationshipRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "table names required")
}

func TestGetColumnsContext_MissingTable(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
	}

	// No entities - requested table won't match any entity
	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{entities: []*models.OntologyEntity{}}
	relationshipRepo := &mockEntityRelationshipRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	// Should not error, but table won't be in results (no entity matches)
	result, err := svc.GetColumnsContext(ctx, projectID, []string{"nonexistent"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 0)
}

func TestGetColumnsContext_TooManyTables(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepository{}
	entityRepo := &mockOntologyEntityRepository{}
	relationshipRepo := &mockEntityRelationshipRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	// Create list of tables exceeding the limit
	tables := make([]string, MaxColumnsDepthTables+1)
	for i := range tables {
		tables[i] = fmt.Sprintf("table_%d", i)
	}

	result, err := svc.GetColumnsContext(ctx, projectID, tables)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "too many tables requested")
	assert.Contains(t, err.Error(), fmt.Sprintf("maximum %d tables allowed", MaxColumnsDepthTables))
}
