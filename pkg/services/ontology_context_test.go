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

func (m *mockOntologyRepository) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}

func (m *mockOntologyRepository) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockOntologyRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// mockTableMetadataRepository is a mock for TableMetadataRepository.
type mockTableMetadataRepository struct {
	metadataByTableName map[string]*models.TableMetadata
}

func (m *mockTableMetadataRepository) GetBySchemaTableID(ctx context.Context, schemaTableID uuid.UUID) (*models.TableMetadata, error) {
	return nil, nil
}

func (m *mockTableMetadataRepository) UpsertFromExtraction(ctx context.Context, meta *models.TableMetadata) error {
	return nil
}

func (m *mockTableMetadataRepository) Upsert(ctx context.Context, meta *models.TableMetadata) error {
	return nil
}

func (m *mockTableMetadataRepository) List(ctx context.Context, projectID uuid.UUID) ([]*models.TableMetadata, error) {
	return nil, nil
}

func (m *mockTableMetadataRepository) ListByTableNames(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string]*models.TableMetadata, error) {
	if m.metadataByTableName == nil {
		return make(map[string]*models.TableMetadata), nil
	}
	result := make(map[string]*models.TableMetadata)
	for _, name := range tableNames {
		if meta, ok := m.metadataByTableName[name]; ok {
			result[name] = meta
		}
	}
	return result, nil
}

func (m *mockTableMetadataRepository) Delete(ctx context.Context, schemaTableID uuid.UUID) error {
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

func (m *mockProjectServiceForOntology) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*AutoApproveSettings, error) {
	return nil, nil
}

func (m *mockProjectServiceForOntology) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *AutoApproveSettings) error {
	return nil
}

func (m *mockProjectServiceForOntology) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*OntologySettings, error) {
	return &OntologySettings{UseLegacyPatternMatching: true}, nil
}

func (m *mockProjectServiceForOntology) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *OntologySettings) error {
	return nil
}

func (m *mockProjectServiceForOntology) SyncServerURL(ctx context.Context, projectID uuid.UUID, papiURL, token string) error {
	return nil
}

// ============================================================================
// Tests
// ============================================================================

func TestGetDomainContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		Version:   1,
		IsActive:  true,
		DomainSummary: &models.DomainSummary{
			Description: "E-commerce platform",
			Domains:     []string{"sales", "customer"},
		},
		ColumnDetails: map[string][]models.ColumnDetail{
			"users":  {{Name: "id"}, {Name: "email"}},
			"orders": {{Name: "id"}, {Name: "user_id"}, {Name: "total"}},
		},
	}

	ontologyRepo := &mockOntologyRepository{
		activeOntology: ontology,
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

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify domain info
	assert.Equal(t, "E-commerce platform", result.Domain.Description)
	assert.Equal(t, []string{"sales", "customer"}, result.Domain.PrimaryDomains)
	assert.Equal(t, 2, result.Domain.TableCount)  // Number of tables from column_details
	assert.Equal(t, 5, result.Domain.ColumnCount) // From schema columns
}

func TestGetDomainContext_NoActiveOntology(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepository{
		activeOntology: nil,
	}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no active ontology found")
}

func TestGetTablesContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	tableID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
		ColumnDetails: map[string][]models.ColumnDetail{
			"users": {
				{Name: "id", IsPrimaryKey: true, Role: models.ColumnRoleIdentifier},
				{Name: "email", Role: models.ColumnRoleAttribute},
				{Name: "status", EnumValues: []models.EnumValue{{Value: "active"}}},
			},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "email", DataType: "varchar"},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "status", DataType: "varchar"},
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	// Test with specific table filter
	result, err := svc.GetTablesContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	// Verify users table
	usersTable := result.Tables["users"]
	assert.Equal(t, 3, usersTable.ColumnCount)
	assert.Len(t, usersTable.Columns, 3)

	// Verify column roles from enriched data
	columnByName := make(map[string]models.ColumnOverview)
	for _, col := range usersTable.Columns {
		columnByName[col.Name] = col
	}

	idCol := columnByName["id"]
	assert.True(t, idCol.IsPrimaryKey)
	assert.Equal(t, models.ColumnRoleIdentifier, idCol.Role)

	statusCol := columnByName["status"]
	assert.True(t, statusCol.HasEnumValues)
}

func TestGetTablesContext_AllTables(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
		ColumnDetails: map[string][]models.ColumnDetail{
			"users":  {{Name: "id"}},
			"orders": {{Name: "id"}},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	// Test without filter - should return all tables from column_details
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

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}

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

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetTablesContext(ctx, projectID, []string{"billing_engagements"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	table := result.Tables["billing_engagements"]
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
	tableID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
		ColumnDetails: map[string][]models.ColumnDetail{
			"users": {
				{Name: "id", Description: "Unique user ID", IsPrimaryKey: true},
				{Name: "email", Description: "User email address"},
			},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "email", DataType: "varchar"},
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	// Verify users table
	usersTable := result.Tables["users"]
	assert.Len(t, usersTable.Columns, 2)

	// Verify enriched column details
	var idCol, emailCol *models.ColumnDetail
	for i := range usersTable.Columns {
		if usersTable.Columns[i].Name == "id" {
			idCol = &usersTable.Columns[i]
		}
		if usersTable.Columns[i].Name == "email" {
			emailCol = &usersTable.Columns[i]
		}
	}

	assert.NotNil(t, idCol)
	assert.Equal(t, "Unique user ID", idCol.Description)
	assert.True(t, idCol.IsPrimaryKey)

	assert.NotNil(t, emailCol)
	assert.Equal(t, "User email address", emailCol.Description)
}

func TestGetColumnsContext_RequiresTableFilter(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "table names required")
}

func TestGetColumnsContext_TooManyTables(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepository{}
	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

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

func TestGetColumnsContext_NoEnrichment(t *testing.T) {
	// Test that columns without enrichment still work (schema-only fallback)
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	tableID := uuid.New()

	// Ontology with empty column_details (no enrichment yet)
	ontology := &models.TieredOntology{
		ID:            ontologyID,
		ProjectID:     projectID,
		IsActive:      true,
		ColumnDetails: map[string][]models.ColumnDetail{},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "name", DataType: "varchar"},
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	usersTable := result.Tables["users"]
	assert.Len(t, usersTable.Columns, 2)

	// Columns should have basic schema info even without enrichment
	var idCol *models.ColumnDetail
	for i := range usersTable.Columns {
		if usersTable.Columns[i].Name == "id" {
			idCol = &usersTable.Columns[i]
			break
		}
	}
	assert.NotNil(t, idCol)
	assert.True(t, idCol.IsPrimaryKey)
	assert.Empty(t, idCol.Description) // No enrichment
}

func TestGetTablesContext_WithTableMetadata(t *testing.T) {
	// Test that table metadata from engine_ontology_table_metadata is merged correctly
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	tableID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
		ColumnDetails: map[string][]models.ColumnDetail{
			"orders": {{Name: "id", IsPrimaryKey: true}},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	// Set up table metadata with description and usage notes
	description := "Customer order records"
	usageNotes := "Contains payment information - handle with care"
	preferredAlternative := "orders_v2"
	tableMetadataRepo := &mockTableMetadataRepository{
		metadataByTableName: map[string]*models.TableMetadata{
			"orders": {
				SchemaTableID:        tableID,
				Description:          &description,
				UsageNotes:           &usageNotes,
				IsEphemeral:          true,
				PreferredAlternative: &preferredAlternative,
			},
		},
	}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, tableMetadataRepo, projectService, zap.NewNop())

	result, err := svc.GetTablesContext(ctx, projectID, []string{"orders"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	ordersTable := result.Tables["orders"]
	assert.Equal(t, "Customer order records", ordersTable.Description)
	assert.Equal(t, "Contains payment information - handle with care", ordersTable.UsageNotes)
	assert.True(t, ordersTable.IsEphemeral)
	assert.Equal(t, "orders_v2", ordersTable.PreferredAlternative)
}

func TestGetColumnsContext_WithTableMetadata(t *testing.T) {
	// Test that table metadata from engine_ontology_table_metadata is merged at columns depth
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()
	tableID := uuid.New()

	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: projectID,
		IsActive:  true,
		ColumnDetails: map[string][]models.ColumnDetail{
			"orders": {{Name: "id", IsPrimaryKey: true}},
		},
	}

	ontologyRepo := &mockOntologyRepository{activeOntology: ontology}
	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	// Set up table metadata
	description := "Customer order records"
	usageNotes := "Join with customers table for full details"
	tableMetadataRepo := &mockTableMetadataRepository{
		metadataByTableName: map[string]*models.TableMetadata{
			"orders": {
				SchemaTableID: tableID,
				Description:   &description,
				UsageNotes:    &usageNotes,
			},
		},
	}

	svc := NewOntologyContextService(ontologyRepo, schemaRepo, tableMetadataRepo, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"orders"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	ordersTable := result.Tables["orders"]
	assert.Equal(t, "Customer order records", ordersTable.Description)
	assert.Equal(t, "Join with customers table for full details", ordersTable.UsageNotes)
}
