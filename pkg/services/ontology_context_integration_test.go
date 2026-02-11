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
	t            *testing.T
	engineDB     *testhelpers.EngineDB
	service      OntologyContextService
	ontologyRepo repositories.OntologyRepository
	schemaRepo   repositories.SchemaRepository
	projectID    uuid.UUID
	dsID         uuid.UUID
}

// mockProjectServiceForIntegration implements ProjectService for integration tests.
type mockProjectServiceForIntegration struct {
	dsID uuid.UUID
}

func (m *mockProjectServiceForIntegration) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForIntegration) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForIntegration) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectServiceForIntegration) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectServiceForIntegration) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
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
	ontologyRepo := repositories.NewOntologyRepository()
	schemaRepo := repositories.NewSchemaRepository()
	projectService := &mockProjectServiceForIntegration{dsID: ontologyContextTestDSID}
	logger := zap.NewNop()

	service := NewOntologyContextService(ontologyRepo, schemaRepo, nil, projectService, logger)

	tc := &ontologyContextTestContext{
		t:            t,
		engineDB:     engineDB,
		service:      service,
		ontologyRepo: ontologyRepo,
		schemaRepo:   schemaRepo,
		projectID:    ontologyContextTestProjectID,
		dsID:         ontologyContextTestDSID,
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
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_ontologies WHERE project_id = $1`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_columns WHERE schema_table_id IN (SELECT id FROM engine_schema_tables WHERE project_id = $1)`, tc.projectID)
	_, _ = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_tables WHERE project_id = $1`, tc.projectID)
}

// createTestOntology creates a complete ontology with enriched column details.
func (tc *ontologyContextTestContext) createTestOntology(ctx context.Context) uuid.UUID {
	tc.t.Helper()

	ontologyID := uuid.New()

	// Create ontology with domain summary and column details
	ontology := &models.TieredOntology{
		ID:        ontologyID,
		ProjectID: tc.projectID,
		Version:   1,
		IsActive:  true,
		DomainSummary: &models.DomainSummary{
			Description: "E-commerce platform for online retail",
			Domains:     []string{"sales", "customer", "product"},
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
						{Value: "inactive", Label: "Inactive", Description: "Account is inactive"},
					},
				},
			},
			"orders": {
				{
					Name:         "id",
					Description:  "Unique order identifier",
					SemanticType: "identifier",
					Role:         "identifier",
					IsPrimaryKey: true,
					Synonyms:     []string{"order_id"},
				},
				{
					Name:         "user_id",
					Description:  "Reference to customer",
					SemanticType: "identifier",
					Role:         "identifier",
					IsForeignKey: true,
					ForeignTable: "users",
				},
				{
					Name:         "total_amount",
					Description:  "Total order value",
					SemanticType: "currency",
					Role:         "measure",
					Synonyms:     []string{"revenue", "order_total"},
				},
				{
					Name:         "status",
					Description:  "Order status",
					SemanticType: "category",
					Role:         "dimension",
					EnumValues: []models.EnumValue{
						{Value: "pending", Label: "Pending"},
						{Value: "shipped", Label: "Shipped"},
						{Value: "delivered", Label: "Delivered"},
					},
				},
			},
		},
	}

	err := tc.ontologyRepo.Create(ctx, ontology)
	require.NoError(tc.t, err, "Failed to create test ontology")

	// Create schema tables and columns for column count
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	err = tc.schemaRepo.UpsertTable(ctx, &models.SchemaTable{
		ID:           usersTableID,
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   "public",
		TableName:    "users",
		IsSelected:   true,
	})
	require.NoError(tc.t, err)

	err = tc.schemaRepo.UpsertTable(ctx, &models.SchemaTable{
		ID:           ordersTableID,
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   "public",
		TableName:    "orders",
		IsSelected:   true,
	})
	require.NoError(tc.t, err)

	// Create columns for users table (3 columns)
	for _, colName := range []string{"id", "email", "status"} {
		isPK := colName == "id"
		err = tc.schemaRepo.UpsertColumn(ctx, &models.SchemaColumn{
			ID:            uuid.New(),
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
		isPK := colName == "id"
		err = tc.schemaRepo.UpsertColumn(ctx, &models.SchemaColumn{
			ID:            uuid.New(),
			ProjectID:     tc.projectID,
			SchemaTableID: ordersTableID,
			ColumnName:    colName,
			DataType:      "text",
			IsPrimaryKey:  isPK,
			IsSelected:    true,
		})
		require.NoError(tc.t, err)
	}

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
	var totalAmountCol *models.ColumnDetail
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
	var userIDCol *models.ColumnDetail
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

func TestOntologyContextService_Integration_NoOntology(t *testing.T) {
	tc := setupOntologyContextTest(t)
	defer tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Don't create any ontology - test error handling

	result, err := tc.service.GetDomainContext(ctx, tc.projectID)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "no active ontology found")
}
