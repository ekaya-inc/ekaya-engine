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

func (m *mockOntologyRepository) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockOntologyRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

type mockOntologyEntityRepository struct {
	entities        []*models.OntologyEntity
	aliases         map[uuid.UUID][]*models.OntologyEntityAlias
	keyColumns      map[uuid.UUID][]*models.OntologyEntityKeyColumn
	getByProjectErr error
	getAliasesErr   error
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
	relationships         []*models.EntityRelationship
	getByProjectErr       error
	getByTablesErr        error
	getByTargetEntityErr  error
	relationshipsByTarget map[uuid.UUID][]*models.EntityRelationship
}

func (m *mockEntityRelationshipRepository) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (m *mockEntityRelationshipRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return m.relationships, nil
}

func (m *mockEntityRelationshipRepository) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	result := make(map[uuid.UUID][]*models.EntityRelationship)
	for _, rel := range m.relationships {
		result[rel.TargetEntityID] = append(result[rel.TargetEntityID], rel)
	}
	return result, nil
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

func (m *mockEntityRelationshipRepository) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	return nil
}

func (m *mockEntityRelationshipRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRelationshipRepository) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	if m.getByTargetEntityErr != nil {
		return nil, m.getByTargetEntityErr
	}
	if m.relationshipsByTarget != nil {
		return m.relationshipsByTarget[entityID], nil
	}
	return []*models.EntityRelationship{}, nil
}

func (m *mockEntityRelationshipRepository) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockEntityRelationshipRepository) Upsert(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (m *mockEntityRelationshipRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockEntityRelationshipRepository) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (m *mockEntityRelationshipRepository) Update(ctx context.Context, rel *models.EntityRelationship) error {
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

func (m *mockProjectServiceForOntology) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	return "", nil
}

func (m *mockProjectServiceForOntology) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	return nil
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

	// Create relationships for testing
	// For occurrence counts: each inbound relationship = 1 occurrence
	// entityID1 (user) should have 2 occurrences:
	//   1. orders.user_id -> users.id (user is target)
	//   2. visits.visitor_id -> users.id (user is target)
	desc1 := "order placed by user"
	desc2 := "visit by visitor"
	relID1 := uuid.New()
	relID2 := uuid.New()
	relationships := []*models.EntityRelationship{
		{
			ID:                 relID1,
			SourceEntityID:     entityID2,
			TargetEntityID:     entityID1,
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
			Description:        &desc1,
			Confidence:         1.0,
		},
		{
			ID:                 relID2,
			SourceEntityID:     entityID1, // visits -> users (for display in relationship graph)
			TargetEntityID:     entityID1,
			SourceColumnSchema: "public",
			SourceColumnTable:  "visits",
			SourceColumnName:   "visitor_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
			Description:        &desc2,
			Confidence:         1.0,
		},
	}

	ontologyRepo := &mockOntologyRepository{
		activeOntology: ontology,
	}
	entityRepo := &mockOntologyEntityRepository{
		entities: entities,
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
	assert.Equal(t, 2, result.Entities[0].OccurrenceCount) // 2 inbound relationships to user

	// Verify relationships from normalized table
	assert.Len(t, result.Relationships, 2)
	// First relationship: order -> user
	assert.Equal(t, "order", result.Relationships[0].From)
	assert.Equal(t, "user", result.Relationships[0].To)
	assert.Equal(t, "order placed by user", result.Relationships[0].Label)
	// Second relationship: user -> user (self-reference from visits)
	assert.Equal(t, "user", result.Relationships[1].From)
	assert.Equal(t, "user", result.Relationships[1].To)
	assert.Equal(t, "visit by visitor", result.Relationships[1].Label)
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

	// Create relationships for testing occurrences
	// entityID1 (user) has 2 inbound relationships:
	//   1. orders.user_id -> users.id (no role/association)
	//   2. orders.customer_id -> users.id (with "customer" association)
	customerAssoc := "customer"
	rel1ID := uuid.New()
	rel2ID := uuid.New()
	relationships := []*models.EntityRelationship{
		{
			ID:                 rel1ID,
			SourceEntityID:     uuid.New(), // order entity
			TargetEntityID:     entityID1,
			SourceColumnSchema: "public",
			SourceColumnTable:  "users",
			SourceColumnName:   "id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
			Confidence:         1.0,
		},
		{
			ID:                 rel2ID,
			SourceEntityID:     uuid.New(), // order entity
			TargetEntityID:     entityID1,
			SourceColumnSchema: "public",
			SourceColumnTable:  "orders",
			SourceColumnName:   "user_id",
			TargetColumnSchema: "public",
			TargetColumnTable:  "users",
			TargetColumnName:   "id",
			Association:        &customerAssoc,
			Confidence:         1.0,
		},
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
		entities:   entities,
		aliases:    aliases,
		keyColumns: keyColumns,
	}
	relationshipRepo := &mockEntityRelationshipRepository{
		relationships: relationships,
		relationshipsByTarget: map[uuid.UUID][]*models.EntityRelationship{
			entityID1: relationships,
		},
	}
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

	// Verify occurrences - computed from inbound relationships
	assert.Len(t, userEntity.Occurrences, 2)
	// First occurrence: users.id -> users.id (no association)
	assert.Equal(t, "users", userEntity.Occurrences[0].Table)
	assert.Equal(t, "id", userEntity.Occurrences[0].Column)
	assert.Nil(t, userEntity.Occurrences[0].Association)
	// Second occurrence: orders.user_id -> users.id (with "customer" association)
	assert.Equal(t, "orders", userEntity.Occurrences[1].Table)
	assert.Equal(t, "user_id", userEntity.Occurrences[1].Column)
	assert.Equal(t, "customer", *userEntity.Occurrences[1].Association)
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

func TestGetTablesContext_FKRoles(t *testing.T) {
	// Tests that FK roles and analytical roles from enriched column_details are exposed at tables depth
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New()
	tableID := uuid.New()

	// Ontology with enriched column_details containing FK roles and analytical roles
	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
		ColumnDetails: map[string][]models.ColumnDetail{
			"billing_engagements": {
				{Name: "id", IsPrimaryKey: true, Role: models.ColumnRoleIdentifier},
				{Name: "host_id", IsForeignKey: true, ForeignTable: "users", FKAssociation: "host", Role: models.ColumnRoleDimension},
				{Name: "visitor_id", IsForeignKey: true, ForeignTable: "users", FKAssociation: "visitor", Role: models.ColumnRoleDimension},
				{Name: "status", EnumValues: []models.EnumValue{{Value: "active"}, {Value: "completed"}}, Role: models.ColumnRoleDimension},
				{Name: "amount", Role: models.ColumnRoleMeasure},
			},
		},
	}

	entities := []*models.OntologyEntity{
		{
			ID:            entityID1,
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          "billing_engagement",
			Description:   "A paid session between host and visitor",
			PrimarySchema: "public",
			PrimaryTable:  "billing_engagements",
			PrimaryColumn: "id",
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{entities: entities}
	relationshipRepo := &mockEntityRelationshipRepository{}

	// Mock schema columns for the table
	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"billing_engagements": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "host_id", DataType: "uuid", IsPrimaryKey: false},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "visitor_id", DataType: "uuid", IsPrimaryKey: false},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "status", DataType: "varchar", IsPrimaryKey: false},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "amount", DataType: "numeric", IsPrimaryKey: false},
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	result, err := svc.GetTablesContext(ctx, projectID, []string{"billing_engagements"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	table := result.Tables["billing_engagements"]
	assert.Equal(t, "billing_engagement", table.BusinessName)
	assert.Len(t, table.Columns, 5)

	// Verify FK roles and analytical roles are exposed
	columnByName := make(map[string]models.ColumnOverview)
	for _, col := range table.Columns {
		columnByName[col.Name] = col
	}

	// host_id should have FKAssociation = "host" and Role = "dimension"
	hostCol := columnByName["host_id"]
	assert.Equal(t, "host", hostCol.FKAssociation, "host_id should have FK association 'host'")
	assert.Equal(t, models.ColumnRoleDimension, hostCol.Role, "host_id should have analytical role 'dimension'")

	// visitor_id should have FKAssociation = "visitor" and Role = "dimension"
	visitorCol := columnByName["visitor_id"]
	assert.Equal(t, "visitor", visitorCol.FKAssociation, "visitor_id should have FK association 'visitor'")
	assert.Equal(t, models.ColumnRoleDimension, visitorCol.Role, "visitor_id should have analytical role 'dimension'")

	// id should have Role = "identifier" but no FKAssociation (it's a PK, not FK)
	idCol := columnByName["id"]
	assert.Empty(t, idCol.FKAssociation, "primary key should not have FK association")
	assert.Equal(t, models.ColumnRoleIdentifier, idCol.Role, "id should have analytical role 'identifier'")

	// status should have HasEnumValues = true and Role = "dimension"
	statusCol := columnByName["status"]
	assert.True(t, statusCol.HasEnumValues, "status should have enum values")
	assert.Empty(t, statusCol.FKAssociation, "status should not have FK association")
	assert.Equal(t, models.ColumnRoleDimension, statusCol.Role, "status should have analytical role 'dimension'")

	// amount should have Role = "measure"
	amountCol := columnByName["amount"]
	assert.Equal(t, models.ColumnRoleMeasure, amountCol.Role, "amount should have analytical role 'measure'")
	assert.Empty(t, amountCol.FKAssociation, "amount should not have FK association")
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

func TestGetDomainContext_DeduplicatesRelationships(t *testing.T) {
	// Tests that duplicate relationships (same source→target pair) are deduplicated,
	// keeping the relationship with the longest description for more context.
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	entityID1 := uuid.New() // user
	entityID2 := uuid.New() // billing_engagement

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
	}

	entities := []*models.OntologyEntity{
		{
			ID:            entityID1,
			Name:          "user",
			PrimarySchema: "public",
			PrimaryTable:  "users",
			PrimaryColumn: "id",
		},
		{
			ID:            entityID2,
			Name:          "billing_engagement",
			PrimarySchema: "public",
			PrimaryTable:  "billing_engagements",
			PrimaryColumn: "id",
		},
	}

	// Create duplicate relationships: same user→billing_engagement pair via host_id and visitor_id
	// This simulates the scenario where both FKs create separate relationship rows
	shortDesc := "via FK"
	longDesc := "User participates in billing engagement as either host or visitor"
	relationships := []*models.EntityRelationship{
		{
			ID:                uuid.New(),
			SourceEntityID:    entityID1,
			TargetEntityID:    entityID2,
			SourceColumnTable: "users",
			SourceColumnName:  "id",
			TargetColumnTable: "billing_engagements",
			TargetColumnName:  "host_id",
			Description:       &shortDesc, // Short description
		},
		{
			ID:                uuid.New(),
			SourceEntityID:    entityID1,
			TargetEntityID:    entityID2,
			SourceColumnTable: "users",
			SourceColumnName:  "id",
			TargetColumnTable: "billing_engagements",
			TargetColumnName:  "visitor_id",
			Description:       &longDesc, // Longer, more descriptive - should be kept
		},
		{
			ID:                uuid.New(),
			SourceEntityID:    entityID1,
			TargetEntityID:    entityID2,
			SourceColumnTable: "users",
			SourceColumnName:  "id",
			TargetColumnTable: "billing_engagements",
			TargetColumnName:  "created_by",
			Description:       nil, // No description
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{
		entities: entities,
	}
	relationshipRepo := &mockEntityRelationshipRepository{relationships: relationships}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Should only have 1 relationship (deduplicated from 3)
	assert.Len(t, result.Relationships, 1, "Expected 3 duplicate relationships to be deduplicated to 1")

	// Should keep the longest description
	assert.Equal(t, "user", result.Relationships[0].From)
	assert.Equal(t, "billing_engagement", result.Relationships[0].To)
	assert.Equal(t, longDesc, result.Relationships[0].Label, "Should keep the longest description for more context")
}

func TestGetDomainContext_DeduplicatesRelationships_FirstWinsWhenSameLength(t *testing.T) {
	// When descriptions have the same length, the first one encountered should be kept
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

	entities := []*models.OntologyEntity{
		{ID: entityID1, Name: "user", PrimaryTable: "users"},
		{ID: entityID2, Name: "order", PrimaryTable: "orders"},
	}

	// Same-length descriptions
	desc1 := "first"
	desc2 := "later"
	relationships := []*models.EntityRelationship{
		{
			ID:             uuid.New(),
			SourceEntityID: entityID1,
			TargetEntityID: entityID2,
			Description:    &desc1,
		},
		{
			ID:             uuid.New(),
			SourceEntityID: entityID1,
			TargetEntityID: entityID2,
			Description:    &desc2,
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{
		entities: entities,
	}
	relationshipRepo := &mockEntityRelationshipRepository{relationships: relationships}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, relationshipRepo, schemaRepo, projectService, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.NoError(t, err)
	assert.Len(t, result.Relationships, 1)
	// When same length, first one wins (no update happens)
	assert.Equal(t, "first", result.Relationships[0].Label)
}
