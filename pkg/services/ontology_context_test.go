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

// mockOntologyRepository is a mock for OntologyRepository (shared across service tests).
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

// mockColumnMetadataRepository is a mock for ColumnMetadataRepository.
type mockColumnMetadataRepository struct {
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata
}

func (m *mockColumnMetadataRepository) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	return nil
}

func (m *mockColumnMetadataRepository) UpsertFromExtraction(ctx context.Context, meta *models.ColumnMetadata) error {
	return nil
}

func (m *mockColumnMetadataRepository) GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
	if m.metadataByColumnID == nil {
		return nil, nil
	}
	return m.metadataByColumnID[schemaColumnID], nil
}

func (m *mockColumnMetadataRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	var result []*models.ColumnMetadata
	for _, meta := range m.metadataByColumnID {
		result = append(result, meta)
	}
	return result, nil
}

func (m *mockColumnMetadataRepository) GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error) {
	var result []*models.ColumnMetadata
	for _, id := range schemaColumnIDs {
		if meta, ok := m.metadataByColumnID[id]; ok {
			result = append(result, meta)
		}
	}
	return result, nil
}

func (m *mockColumnMetadataRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockColumnMetadataRepository) DeleteBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) error {
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
	datasourceID  uuid.UUID
	project       *models.Project
	err           error
}

func (m *mockProjectServiceForOntology) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForOntology) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForOntology) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.project != nil {
		return m.project, nil
	}
	// Return a minimal project if none set
	return &models.Project{ID: id}, nil
}

func (m *mockProjectServiceForOntology) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectServiceForOntology) Delete(ctx context.Context, id uuid.UUID) (*DeleteResult, error) {
	return &DeleteResult{}, nil
}

func (m *mockProjectServiceForOntology) CompleteDeleteCallback(ctx context.Context, projectID uuid.UUID, action, status, nonce string) (*DeleteCallbackResult, error) {
	return &DeleteCallbackResult{}, nil
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

	projectService := &mockProjectServiceForOntology{
		project: &models.Project{
			ID: projectID,
			DomainSummary: &models.DomainSummary{
				Description: "E-commerce platform",
				Domains:     []string{"sales", "customer"},
			},
		},
	}
	schemaRepo := &mockSchemaRepository{
		tableCount: 2,
		columns: []*models.SchemaColumn{
			{ColumnName: "id"}, {ColumnName: "email"},
			{ColumnName: "id"}, {ColumnName: "user_id"}, {ColumnName: "total"},
		},
	}

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Verify domain info
	assert.Equal(t, "E-commerce platform", result.Domain.Description)
	assert.Equal(t, []string{"sales", "customer"}, result.Domain.PrimaryDomains)
	assert.Equal(t, 2, result.Domain.TableCount)  // From schema table count
	assert.Equal(t, 5, result.Domain.ColumnCount) // From schema columns
}

func TestGetDomainContext_NoDomainSummary(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	projectService := &mockProjectServiceForOntology{
		project: &models.Project{ID: projectID},
	}
	schemaRepo := &mockSchemaRepository{
		tableCount: 0,
	}

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetDomainContext(ctx, projectID)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Empty(t, result.Domain.Description)
	assert.Equal(t, 0, result.Domain.TableCount)
	assert.Equal(t, 0, result.Domain.ColumnCount)
}

func TestGetTablesContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	// Schema columns with IDs for metadata lookup
	idColID := uuid.New()
	emailColID := uuid.New()
	statusColID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: idColID, SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: emailColID, SchemaTableID: tableID, ColumnName: "email", DataType: "varchar"},
				{ID: statusColID, SchemaTableID: tableID, ColumnName: "status", DataType: "varchar"},
			},
		},
	}

	// Column metadata with enrichment
	identifierRole := models.ColumnRoleIdentifier
	attributeRole := models.ColumnRoleAttribute
	columnMetadataRepo := &mockColumnMetadataRepository{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			idColID: {
				SchemaColumnID: idColID,
				Role:           &identifierRole,
			},
			emailColID: {
				SchemaColumnID: emailColID,
				Role:           &attributeRole,
			},
			statusColID: {
				SchemaColumnID: statusColID,
				Features: models.ColumnMetadataFeatures{
					EnumFeatures: &models.EnumFeatures{
						Values: []models.ColumnEnumValue{{Value: "active"}},
					},
				},
			},
		},
	}

	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, columnMetadataRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetTablesContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	usersTable := result.Tables["users"]
	assert.Equal(t, 3, usersTable.ColumnCount)
	assert.Len(t, usersTable.Columns, 3)

	// Verify column roles from metadata
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

	schemaRepo := &mockSchemaRepository{
		selectedTableNames: []string{"users", "orders"},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	// Test without filter - should return all selected tables
	result, err := svc.GetTablesContext(ctx, projectID, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 2)
}

func TestGetTablesContext_FKRoles(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	idColID := uuid.New()
	hostColID := uuid.New()
	visitorColID := uuid.New()
	statusColID := uuid.New()
	amountColID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"billing_engagements": {
				{ID: idColID, SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: hostColID, SchemaTableID: tableID, ColumnName: "host_id", DataType: "uuid"},
				{ID: visitorColID, SchemaTableID: tableID, ColumnName: "visitor_id", DataType: "uuid"},
				{ID: statusColID, SchemaTableID: tableID, ColumnName: "status", DataType: "varchar"},
				{ID: amountColID, SchemaTableID: tableID, ColumnName: "amount", DataType: "numeric"},
			},
		},
	}

	identifierRole := models.ColumnRoleIdentifier
	dimensionRole := models.ColumnRoleDimension
	measureRole := models.ColumnRoleMeasure
	columnMetadataRepo := &mockColumnMetadataRepository{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			idColID: {
				SchemaColumnID: idColID,
				Role:           &identifierRole,
			},
			hostColID: {
				SchemaColumnID: hostColID,
				Role:           &dimensionRole,
				Features: models.ColumnMetadataFeatures{
					IdentifierFeatures: &models.IdentifierFeatures{
						FKAssociation: "host",
						FKTargetTable: "users",
					},
				},
			},
			visitorColID: {
				SchemaColumnID: visitorColID,
				Role:           &dimensionRole,
				Features: models.ColumnMetadataFeatures{
					IdentifierFeatures: &models.IdentifierFeatures{
						FKAssociation: "visitor",
						FKTargetTable: "users",
					},
				},
			},
			statusColID: {
				SchemaColumnID: statusColID,
				Role:           &dimensionRole,
				Features: models.ColumnMetadataFeatures{
					EnumFeatures: &models.EnumFeatures{
						Values: []models.ColumnEnumValue{{Value: "active"}, {Value: "completed"}},
					},
				},
			},
			amountColID: {
				SchemaColumnID: amountColID,
				Role:           &measureRole,
			},
		},
	}

	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, columnMetadataRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetTablesContext(ctx, projectID, []string{"billing_engagements"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	table := result.Tables["billing_engagements"]
	assert.Len(t, table.Columns, 5)

	columnByName := make(map[string]models.ColumnOverview)
	for _, col := range table.Columns {
		columnByName[col.Name] = col
	}

	// host_id should have FKAssociation = "host" and Role = "dimension"
	hostCol := columnByName["host_id"]
	assert.Equal(t, "host", hostCol.FKAssociation)
	assert.Equal(t, models.ColumnRoleDimension, hostCol.Role)

	// visitor_id should have FKAssociation = "visitor" and Role = "dimension"
	visitorCol := columnByName["visitor_id"]
	assert.Equal(t, "visitor", visitorCol.FKAssociation)
	assert.Equal(t, models.ColumnRoleDimension, visitorCol.Role)

	// id should have Role = "identifier" but no FKAssociation
	idCol := columnByName["id"]
	assert.Empty(t, idCol.FKAssociation)
	assert.Equal(t, models.ColumnRoleIdentifier, idCol.Role)

	// status should have HasEnumValues = true and Role = "dimension"
	statusCol := columnByName["status"]
	assert.True(t, statusCol.HasEnumValues)
	assert.Empty(t, statusCol.FKAssociation)
	assert.Equal(t, models.ColumnRoleDimension, statusCol.Role)

	// amount should have Role = "measure"
	amountCol := columnByName["amount"]
	assert.Equal(t, models.ColumnRoleMeasure, amountCol.Role)
	assert.Empty(t, amountCol.FKAssociation)
}

func TestGetColumnsContext(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	idColID := uuid.New()
	emailColID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: idColID, SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: emailColID, SchemaTableID: tableID, ColumnName: "email", DataType: "varchar"},
			},
		},
	}

	idDesc := "Unique user ID"
	emailDesc := "User email address"
	columnMetadataRepo := &mockColumnMetadataRepository{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			idColID: {
				SchemaColumnID: idColID,
				Description:    &idDesc,
			},
			emailColID: {
				SchemaColumnID: emailColID,
				Description:    &emailDesc,
			},
		},
	}

	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, columnMetadataRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	usersTable := result.Tables["users"]
	assert.Len(t, usersTable.Columns, 2)

	// Verify enriched column details
	var idCol, emailCol *models.ColumnDetailInfo
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

	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, nil)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "table names required")
}

func TestGetColumnsContext_TooManyTables(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	schemaRepo := &mockSchemaRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, &mockTableMetadataRepository{}, projectService, zap.NewNop())

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
	// Columns without metadata should still work (schema-only fallback)
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "name", DataType: "varchar"},
			},
		},
	}

	// No column metadata — empty repo
	columnMetadataRepo := &mockColumnMetadataRepository{}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, columnMetadataRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	usersTable := result.Tables["users"]
	assert.Len(t, usersTable.Columns, 2)

	var idCol *models.ColumnDetailInfo
	for i := range usersTable.Columns {
		if usersTable.Columns[i].Name == "id" {
			idCol = &usersTable.Columns[i]
			break
		}
	}
	assert.NotNil(t, idCol)
	assert.True(t, idCol.IsPrimaryKey)
	assert.Empty(t, idCol.Description)
}

func TestGetTablesContext_WithTableMetadata(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

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

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, tableMetadataRepo, projectService, zap.NewNop())

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
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

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

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, tableMetadataRepo, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"orders"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 1)

	ordersTable := result.Tables["orders"]
	assert.Equal(t, "Customer order records", ordersTable.Description)
	assert.Equal(t, "Join with customers table for full details", ordersTable.UsageNotes)
}

func TestGetTablesContext_RowCountPopulated(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	usersRowCount := int64(95)
	ordersRowCount := int64(250)
	schemaRepo := &mockSchemaRepository{
		selectedTableNames: []string{"users", "orders"},
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: uuid.New(), SchemaTableID: usersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			},
			"orders": {
				{ID: uuid.New(), SchemaTableID: ordersTableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			},
		},
		tablesByName: map[string]*models.SchemaTable{
			"users": {
				ID:        usersTableID,
				ProjectID: projectID,
				TableName: "users",
				RowCount:  &usersRowCount,
			},
			"orders": {
				ID:        ordersTableID,
				ProjectID: projectID,
				TableName: "orders",
				RowCount:  &ordersRowCount,
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetTablesContext(ctx, projectID, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Len(t, result.Tables, 2)

	assert.Equal(t, int64(95), result.Tables["users"].RowCount)
	assert.Equal(t, int64(250), result.Tables["orders"].RowCount)
}

func TestGetTablesContext_RowCountNil(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: uuid.New(), SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
			},
		},
		tablesByName: map[string]*models.SchemaTable{
			"users": {
				ID:        tableID,
				ProjectID: projectID,
				TableName: "users",
				RowCount:  nil,
			},
		},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, &mockColumnMetadataRepository{}, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetTablesContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.Tables["users"].RowCount)
}

func TestGetTablesContext_HasDescriptionFlag(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	idColID := uuid.New()
	emailColID := uuid.New()
	statusColID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": {
				{ID: idColID, SchemaTableID: tableID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ID: emailColID, SchemaTableID: tableID, ColumnName: "email", DataType: "varchar"},
				{ID: statusColID, SchemaTableID: tableID, ColumnName: "status", DataType: "varchar"},
			},
		},
	}

	idDesc := "Unique user identifier"
	emailDesc := "User email address"
	columnMetadataRepo := &mockColumnMetadataRepository{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			idColID: {
				SchemaColumnID: idColID,
				Description:    &idDesc,
			},
			emailColID: {
				SchemaColumnID: emailColID,
				Description:    &emailDesc,
			},
			// statusColID has no metadata — no description
		},
	}
	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, columnMetadataRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetTablesContext(ctx, projectID, []string{"users"})

	assert.NoError(t, err)
	assert.NotNil(t, result)

	columnByName := make(map[string]models.ColumnOverview)
	for _, col := range result.Tables["users"].Columns {
		columnByName[col.Name] = col
	}

	assert.True(t, columnByName["id"].HasDescription)
	assert.True(t, columnByName["email"].HasDescription)
	assert.False(t, columnByName["status"].HasDescription)
}

func TestGetColumnsContext_FKAndEnumDetails(t *testing.T) {
	// Test that FK info and enum values are properly conveyed in ColumnDetailInfo
	ctx := context.Background()
	projectID := uuid.New()
	tableID := uuid.New()

	hostColID := uuid.New()
	statusColID := uuid.New()

	schemaRepo := &mockSchemaRepository{
		columnsByTable: map[string][]*models.SchemaColumn{
			"visits": {
				{ID: hostColID, SchemaTableID: tableID, ColumnName: "host_id", DataType: "uuid"},
				{ID: statusColID, SchemaTableID: tableID, ColumnName: "status", DataType: "varchar"},
			},
		},
	}

	dimensionRole := models.ColumnRoleDimension
	columnMetadataRepo := &mockColumnMetadataRepository{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			hostColID: {
				SchemaColumnID: hostColID,
				Role:           &dimensionRole,
				Features: models.ColumnMetadataFeatures{
					IdentifierFeatures: &models.IdentifierFeatures{
						FKAssociation: "host",
						FKTargetTable: "users",
					},
				},
			},
			statusColID: {
				SchemaColumnID: statusColID,
				Role:           &dimensionRole,
				Features: models.ColumnMetadataFeatures{
					EnumFeatures: &models.EnumFeatures{
						Values: []models.ColumnEnumValue{
							{Value: "active", Label: "Active"},
							{Value: "completed", Label: "Completed"},
						},
					},
				},
			},
		},
	}

	projectService := &mockProjectServiceForOntology{}

	svc := NewOntologyContextService(schemaRepo, columnMetadataRepo, &mockTableMetadataRepository{}, projectService, zap.NewNop())

	result, err := svc.GetColumnsContext(ctx, projectID, []string{"visits"})

	assert.NoError(t, err)
	assert.NotNil(t, result)

	visitsTable := result.Tables["visits"]
	assert.Len(t, visitsTable.Columns, 2)

	var hostCol, statusCol *models.ColumnDetailInfo
	for i := range visitsTable.Columns {
		switch visitsTable.Columns[i].Name {
		case "host_id":
			hostCol = &visitsTable.Columns[i]
		case "status":
			statusCol = &visitsTable.Columns[i]
		}
	}

	// Verify FK info
	assert.NotNil(t, hostCol)
	assert.True(t, hostCol.IsForeignKey)
	assert.Equal(t, "users", hostCol.ForeignTable)
	assert.Equal(t, "host", hostCol.FKAssociation)

	// Verify enum values
	assert.NotNil(t, statusCol)
	assert.Len(t, statusCol.EnumValues, 2)
	assert.Equal(t, "active", statusCol.EnumValues[0].Value)
	assert.Equal(t, "Active", statusCol.EnumValues[0].Label)
}
