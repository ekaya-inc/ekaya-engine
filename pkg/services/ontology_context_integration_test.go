//go:build integration

package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test IDs for ontology context tests (unique range 0x501-0x5xx)
var (
	ontologyContextTestProjectID = uuid.MustParse("00000000-0000-0000-0000-000000000501")
	ontologyContextTestDSID      = uuid.MustParse("00000000-0000-0000-0000-000000000502")
)

// ontologyContextTestContext holds all dependencies for ontology context integration tests.
type ontologyContextTestContext struct {
	t                  *testing.T
	engineDB           *testhelpers.EngineDB
	service            OntologyContextService
	schemaRepo         repositories.SchemaRepository
	columnMetadataRepo repositories.ColumnMetadataRepository
	projectMock        *mockProjectServiceForIntegration
	projectID          uuid.UUID
	dsID               uuid.UUID
}

// mockProjectServiceForIntegration implements ProjectService for integration tests.
type mockProjectServiceForIntegration struct {
	dsID          uuid.UUID
	domainSummary *models.DomainSummary
}

func (m *mockProjectServiceForIntegration) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForIntegration) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForIntegration) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return &models.Project{
		ID:            id,
		Name:          "Ontology Context Test Project",
		Status:        "active",
		DomainSummary: m.domainSummary,
	}, nil
}

func (m *mockProjectServiceForIntegration) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectServiceForIntegration) Delete(ctx context.Context, id uuid.UUID) (*DeleteResult, error) {
	return &DeleteResult{}, nil
}

func (m *mockProjectServiceForIntegration) CompleteDeleteCallback(ctx context.Context, projectID uuid.UUID, action, status, nonce string) (*DeleteCallbackResult, error) {
	return &DeleteCallbackResult{}, nil
}

func (m *mockProjectServiceForIntegration) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	return m.dsID, nil
}

func (m *mockProjectServiceForIntegration) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockProjectServiceForIntegration) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
	// No-op for tests
}

func (m *mockProjectServiceForIntegration) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	return "", nil
}

func (m *mockProjectServiceForIntegration) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	return nil
}

func (m *mockProjectServiceForIntegration) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*AutoApproveSettings, error) {
	return nil, nil
}

func (m *mockProjectServiceForIntegration) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *AutoApproveSettings) error {
	return nil
}

func (m *mockProjectServiceForIntegration) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*OntologySettings, error) {
	return &OntologySettings{UseLegacyPatternMatching: true}, nil
}

func (m *mockProjectServiceForIntegration) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *OntologySettings) error {
	return nil
}

func (m *mockProjectServiceForIntegration) SyncServerURL(ctx context.Context, projectID uuid.UUID, papiURL, token string) error {
	return nil
}

// setupOntologyContextTest creates a test context with real database.
func setupOntologyContextTest(t *testing.T) *ontologyContextTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	schemaRepo := repositories.NewSchemaRepository()
	columnMetadataRepo := repositories.NewColumnMetadataRepository()
	projectService := &mockProjectServiceForIntegration{dsID: ontologyContextTestDSID}
	logger := zap.NewNop()

	service := NewOntologyContextService(schemaRepo, columnMetadataRepo, nil, projectService, logger)

	tc := &ontologyContextTestContext{
		t:                  t,
		engineDB:           engineDB,
		service:            service,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: columnMetadataRepo,
		projectMock:        projectService,
		projectID:          ontologyContextTestProjectID,
		dsID:               ontologyContextTestDSID,
	}

	// Ensure project exists
	tc.ensureTestProject()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *ontologyContextTestContext) createTestContext() (context.Context, func()) {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope: %v", err)
	}

	ctx = database.SetTenantScope(ctx, scope)

	return ctx, func() {
		scope.Close()
	}
}

// ensureTestProject creates the test project and datasource if they don't exist.
func (tc *ontologyContextTestContext) ensureTestProject() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithoutTenant(ctx)
	if err != nil {
		tc.t.Fatalf("Failed to create scope for project setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_projects (id, name, status)
		VALUES ($1, $2, 'active')
		ON CONFLICT (id) DO NOTHING
	`, tc.projectID, "Ontology Context Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}

	// Create test datasource for schema tables
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config, created_at, updated_at)
		VALUES ($1, $2, 'Test Datasource', 'postgres', '{}', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, tc.dsID, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
}

// cleanup removes all ontology data for the test project.
func (tc *ontologyContextTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete in reverse order of dependencies
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontology_column_metadata WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_columns WHERE schema_table_id IN (SELECT id FROM engine_schema_tables WHERE project_id = $1)`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_tables WHERE project_id = $1`, tc.projectID)
}

// createTestOntology sets up domain summary on the project mock, creates schema tables/columns,
// and writes column metadata records for enriched data.
func (tc *ontologyContextTestContext) createTestOntology(ctx context.Context) uuid.UUID {
	tc.t.Helper()

	ontologyID := uuid.New()

	// Set domain summary on the project mock (replaces old engine_ontologies insert)
	tc.projectMock.domainSummary = &models.DomainSummary{
		Description: "E-commerce platform for online retail",
		Domains:     []string{"sales", "customer", "product"},
	}

	// Create schema tables and columns for column count
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	usersRowCount := int64(95)
	ordersRowCount := int64(250)

	err := tc.schemaRepo.UpsertTable(ctx, &models.SchemaTable{
		ID:           usersTableID,
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   "public",
		TableName:    "users",
		IsSelected:   true,
		RowCount:     &usersRowCount,
	})
	require.NoError(tc.t, err)

	err = tc.schemaRepo.UpsertTable(ctx, &models.SchemaTable{
		ID:           ordersTableID,
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   "public",
		TableName:    "orders",
		IsSelected:   true,
		RowCount:     &ordersRowCount,
	})
	require.NoError(tc.t, err)

	// Create columns for users table (3 columns) and track IDs for metadata
	columnIDs := make(map[string]uuid.UUID)
	for _, colName := range []string{"id", "email", "status"} {
		colID := uuid.New()
		columnIDs["users."+colName] = colID
		isPK := colName == "id"
		err = tc.schemaRepo.UpsertColumn(ctx, &models.SchemaColumn{
			ID:            colID,
			ProjectID:     tc.projectID,
			SchemaTableID: usersTableID,
			ColumnName:    colName,
			DataType:      "text",
			IsPrimaryKey:  isPK,
			IsSelected:    true,
		})
		require.NoError(tc.t, err)
	}

	// Create columns for orders table (4 columns)
	for _, colName := range []string{"id", "user_id", "total_amount", "status"} {
		colID := uuid.New()
		columnIDs["orders."+colName] = colID
		isPK := colName == "id"
		err = tc.schemaRepo.UpsertColumn(ctx, &models.SchemaColumn{
			ID:            colID,
			ProjectID:     tc.projectID,
			SchemaTableID: ordersTableID,
			ColumnName:    colName,
			DataType:      "text",
			IsPrimaryKey:  isPK,
			IsSelected:    true,
		})
		require.NoError(tc.t, err)
	}

	// Create column metadata for enriched columns
	identifierRole := "identifier"
	err = tc.columnMetadataRepo.Upsert(ctx, &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: columnIDs["users.id"],
		Role:           &identifierRole,
		Source:         "inferred",
	})
	require.NoError(tc.t, err)

	err = tc.columnMetadataRepo.Upsert(ctx, &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: columnIDs["users.status"],
		Source:         "inferred",
		Features: models.ColumnMetadataFeatures{
			EnumFeatures: &models.EnumFeatures{
				Values: []models.ColumnEnumValue{
					{Value: "active", Label: "Active user"},
					{Value: "inactive", Label: "Inactive user"},
				},
			},
		},
	})
	require.NoError(tc.t, err)

	totalAmountDesc := "Total order value"
	totalAmountSemType := "currency"
	measureRole := "measure"
	err = tc.columnMetadataRepo.Upsert(ctx, &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: columnIDs["orders.total_amount"],
		Description:    &totalAmountDesc,
		SemanticType:   &totalAmountSemType,
		Role:           &measureRole,
		Source:         "inferred",
		Features: models.ColumnMetadataFeatures{
			Synonyms: []string{"revenue", "order_total"},
		},
	})
	require.NoError(tc.t, err)

	fkRole := "foreign_key"
	err = tc.columnMetadataRepo.Upsert(ctx, &models.ColumnMetadata{
		ProjectID:      tc.projectID,
		SchemaColumnID: columnIDs["orders.user_id"],
		Role:           &fkRole,
		Source:         "inferred",
		Features: models.ColumnMetadataFeatures{
			IdentifierFeatures: &models.IdentifierFeatures{
				IdentifierType: "foreign_key",
				FKTargetTable:  "users",
				FKTargetColumn: "id",
			},
		},
	})
	require.NoError(tc.t, err)

	return ontologyID
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestOntologyContextService_Integration_GetDomainContext(t *testing.T) {
	tc := setupOntologyContextTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test ontology
	tc.createTestOntology(ctx)

	// Test GetDomainContext
	result, err := tc.service.GetDomainContext(ctx, tc.projectID)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify domain info
	assert.Equal(t, "E-commerce platform for online retail", result.Domain.Description)
	assert.Contains(t, result.Domain.PrimaryDomains, "sales")
	assert.Contains(t, result.Domain.PrimaryDomains, "customer")
	assert.Equal(t, 2, result.Domain.TableCount)
	assert.Equal(t, 7, result.Domain.ColumnCount) // 3 users + 4 orders
}

func TestOntologyContextService_Integration_GetTablesContext(t *testing.T) {
	tc := setupOntologyContextTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test ontology
	tc.createTestOntology(ctx)

	// Test GetTablesContext with filter
	result, err := tc.service.GetTablesContext(ctx, tc.projectID, []string{"users"})

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should only have users table
	assert.Len(t, result.Tables, 1)

	usersTable, ok := result.Tables["users"]
	require.True(t, ok, "Users table should be present")
	assert.Equal(t, 3, usersTable.ColumnCount)

	// Check columns overview
	assert.Len(t, usersTable.Columns, 3)

	// Verify column roles from enriched data
	columnByName := make(map[string]models.ColumnOverview)
	for _, col := range usersTable.Columns {
		columnByName[col.Name] = col
	}

	idCol := columnByName["id"]
	assert.True(t, idCol.IsPrimaryKey)
	assert.Equal(t, "identifier", idCol.Role)

	statusCol := columnByName["status"]
	assert.True(t, statusCol.HasEnumValues)
}

func TestOntologyContextService_Integration_GetTablesContext_AllTables(t *testing.T) {
	tc := setupOntologyContextTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test ontology
	tc.createTestOntology(ctx)

	// Test GetTablesContext without filter (all tables)
	result, err := tc.service.GetTablesContext(ctx, tc.projectID, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have both tables
	assert.Len(t, result.Tables, 2)
	assert.Contains(t, result.Tables, "users")
	assert.Contains(t, result.Tables, "orders")
}

func TestOntologyContextService_Integration_GetColumnsContext(t *testing.T) {
	tc := setupOntologyContextTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test ontology
	tc.createTestOntology(ctx)

	// Test GetColumnsContext
	result, err := tc.service.GetColumnsContext(ctx, tc.projectID, []string{"orders"})

	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have orders table
	assert.Len(t, result.Tables, 1)

	ordersTable, ok := result.Tables["orders"]
	require.True(t, ok, "Orders table should be present")

	// Check column count - structural data from schema_columns
	assert.Len(t, ordersTable.Columns, 4)

	// Find total_amount column - only structural fields populated in Phase 1
	var totalAmountCol *models.ColumnDetailInfo
	for i := range ordersTable.Columns {
		if ordersTable.Columns[i].Name == "total_amount" {
			totalAmountCol = &ordersTable.Columns[i]
			break
		}
	}
	require.NotNil(t, totalAmountCol, "total_amount column should be present")
	// Semantic fields are populated from column_details
	assert.Equal(t, "Total order value", totalAmountCol.Description)
	assert.Equal(t, "currency", totalAmountCol.SemanticType)
	assert.Equal(t, "measure", totalAmountCol.Role)
	assert.Equal(t, []string{"revenue", "order_total"}, totalAmountCol.Synonyms)

	// Find user_id column to verify FK detection from enriched data
	var userIDCol *models.ColumnDetailInfo
	for i := range ordersTable.Columns {
		if ordersTable.Columns[i].Name == "user_id" {
			userIDCol = &ordersTable.Columns[i]
			break
		}
	}
	require.NotNil(t, userIDCol, "user_id column should be present")
	// FK info comes from enriched column_details
	assert.True(t, userIDCol.IsForeignKey)
	assert.Equal(t, "users", userIDCol.ForeignTable)
}

func TestOntologyContextService_Integration_GetTablesContext_RowCount(t *testing.T) {
	tc := setupOntologyContextTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test ontology (sets users=95, orders=250 row counts)
	tc.createTestOntology(ctx)

	// Test GetTablesContext returns row counts
	result, err := tc.service.GetTablesContext(ctx, tc.projectID, nil)

	require.NoError(t, err)
	require.NotNil(t, result)

	usersTable := result.Tables["users"]
	assert.Equal(t, int64(95), usersTable.RowCount, "users table should have row_count from schema")

	ordersTable := result.Tables["orders"]
	assert.Equal(t, int64(250), ordersTable.RowCount, "orders table should have row_count from schema")
}

func TestOntologyContextService_Integration_NoDomainSummary(t *testing.T) {
	tc := setupOntologyContextTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Don't set domain summary — returns empty but valid context
	result, err := tc.service.GetDomainContext(ctx, tc.projectID)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Domain.Description)
	assert.Equal(t, 0, result.Domain.TableCount)
}
