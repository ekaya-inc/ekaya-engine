package services

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"

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

	ontologyRepo := &mockOntologyRepository{
		activeOntology: ontology,
	}
	entityRepo := &mockOntologyEntityRepository{
		entities:    entities,
		occurrences: occurrences,
	}
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify domain info
	assert.Equal(t, "E-commerce platform", result.Domain.Description)
	assert.Equal(t, []string{"sales", "customer"}, result.Domain.PrimaryDomains)
	assert.Equal(t, 2, result.Domain.TableCount)
	assert.Equal(t, 5, result.Domain.ColumnCount)

	// Verify entities
	assert.Len(t, result.Entities, 2)
	assert.Equal(t, "user", result.Entities[0].Name)
	assert.Equal(t, "Platform user", result.Entities[0].Description)
	assert.Equal(t, "users", result.Entities[0].PrimaryTable)
	assert.Equal(t, 2, result.Entities[0].OccurrenceCount)

	// Verify relationships
	assert.Len(t, result.Relationships, 1)
	assert.Equal(t, "user", result.Relationships[0].From)
	assert.Equal(t, "order", result.Relationships[0].To)
}

func TestGetDomainContext_NoActiveOntology(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepository{
		activeOntology: nil,
	}
	entityRepo := &mockOntologyEntityRepository{}
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

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

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{
		entities:    entities,
		occurrences: occurrences,
		aliases:     aliases,
	}
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

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

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		EntitySummaries: map[string]*models.EntitySummary{
			"users": {
				TableName:    "users",
				BusinessName: "Users",
				Description:  "Platform users",
				Domain:       "customer",
				Synonyms:     []string{"accounts", "members"},
				ColumnCount:  3,
			},
			"orders": {
				TableName:    "orders",
				BusinessName: "Orders",
				Description:  "Customer orders",
				Domain:       "sales",
				ColumnCount:  5,
			},
		},
		ColumnDetails: map[string][]models.ColumnDetail{
			"users": {
				{Name: "id", Role: "identifier", IsPrimaryKey: true},
				{Name: "email", Role: "attribute"},
				{Name: "created_at", Role: "dimension"},
			},
			"orders": {
				{Name: "id", Role: "identifier", IsPrimaryKey: true},
				{Name: "user_id", Role: "identifier", IsForeignKey: true},
				{Name: "total", Role: "measure"},
				{Name: "status", Role: "dimension", EnumValues: []models.EnumValue{{Value: "pending"}, {Value: "complete"}}},
				{Name: "created_at", Role: "dimension"},
			},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{}
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

	// Test with specific table filter
	result, err := svc.GetTablesContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	// Verify users table
	usersTable := result.Tables["users"]
	assert.Equal(t, "Users", usersTable.BusinessName)
	assert.Equal(t, "Platform users", usersTable.Description)
	assert.Equal(t, "customer", usersTable.Domain)
	assert.Equal(t, 3, usersTable.ColumnCount)
	assert.Equal(t, []string{"accounts", "members"}, usersTable.Synonyms)

	// Verify columns
	assert.Len(t, usersTable.Columns, 3)
	assert.Equal(t, "id", usersTable.Columns[0].Name)
	assert.Equal(t, "identifier", usersTable.Columns[0].Role)
	assert.True(t, usersTable.Columns[0].IsPrimaryKey)
}

func TestGetTablesContext_AllTables(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		EntitySummaries: map[string]*models.EntitySummary{
			"users":  {TableName: "users", BusinessName: "Users"},
			"orders": {TableName: "orders", BusinessName: "Orders"},
		},
		ColumnDetails: map[string][]models.ColumnDetail{
			"users":  {{Name: "id"}},
			"orders": {{Name: "id"}},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{}
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

	// Test without filter - should return all tables
	result, err := svc.GetTablesContext(ctx, projectID, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 2)
}

func TestGetColumnsContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		EntitySummaries: map[string]*models.EntitySummary{
			"users": {
				TableName:    "users",
				BusinessName: "Users",
				Description:  "Platform users",
			},
		},
		ColumnDetails: map[string][]models.ColumnDetail{
			"users": {
				{
					Name:         "id",
					Description:  "Unique user identifier",
					SemanticType: "identifier",
					Role:         "identifier",
					IsPrimaryKey: true,
					Synonyms:     []string{"user_id"},
				},
				{
					Name:         "email",
					Description:  "User email address",
					SemanticType: "email",
					Role:         "attribute",
					Synonyms:     []string{"email_address"},
				},
				{
					Name:         "status",
					Description:  "Account status",
					SemanticType: "category",
					Role:         "dimension",
					EnumValues: []models.EnumValue{
						{Value: "active", Label: "Active", Description: "Account is active"},
						{Value: "suspended", Label: "Suspended", Description: "Account is suspended"},
					},
				},
			},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{}
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	// Verify users table
	usersTable := result.Tables["users"]
	assert.Equal(t, "Users", usersTable.BusinessName)
	assert.Equal(t, "Platform users", usersTable.Description)

	// Verify columns
	assert.Len(t, usersTable.Columns, 3)

	// Verify id column
	idCol := usersTable.Columns[0]
	assert.Equal(t, "id", idCol.Name)
	assert.Equal(t, "Unique user identifier", idCol.Description)
	assert.Equal(t, "identifier", idCol.SemanticType)
	assert.True(t, idCol.IsPrimaryKey)
	assert.Equal(t, []string{"user_id"}, idCol.Synonyms)

	// Verify status column with enum values
	statusCol := usersTable.Columns[2]
	assert.Equal(t, "status", statusCol.Name)
	assert.Len(t, statusCol.EnumValues, 2)
	assert.Equal(t, "active", statusCol.EnumValues[0].Value)
	assert.Equal(t, "Active", statusCol.EnumValues[0].Label)
}

func TestGetColumnsContext_RequiresTableFilter(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepository{}
	entityRepo := &mockOntologyEntityRepository{}
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

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
		ID:              ontologyID,
		ProjectID:       projectID,
		EntitySummaries: map[string]*models.EntitySummary{},
		ColumnDetails:   map[string][]models.ColumnDetail{},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	entityRepo := &mockOntologyEntityRepository{}
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

	// Should not error, but table won't be in results
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
	schemaRepo := &mockSchemaRepository{}

	svc := NewOntologyContextService(ontologyRepo, entityRepo, schemaRepo, zap.NewNop())

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
