//go:build integration

package services

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test IDs for ontology builder tests (unique range 0x401-0x4xx)
var (
	ontologyBuilderTestProjectID = uuid.MustParse("00000000-0000-0000-0000-000000000401")
	ontologyBuilderTestDSID      = uuid.MustParse("00000000-0000-0000-0000-000000000402")
)

// ontologyBuilderTestContext holds all dependencies for ontology builder integration tests.
type ontologyBuilderTestContext struct {
	t          *testing.T
	engineDB   *testhelpers.EngineDB
	service    *ontologyBuilderService // concrete type to access helper methods
	schemaRepo repositories.SchemaRepository
	projectID  uuid.UUID
	dsID       uuid.UUID
}

// setupOntologyBuilderTest creates a test context with real database.
func setupOntologyBuilderTest(t *testing.T) *ontologyBuilderTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	schemaRepo := repositories.NewSchemaRepository()
	logger := zap.NewNop()

	// Create concrete service (not interface) to access helper methods
	service := &ontologyBuilderService{
		schemaRepo: schemaRepo,
		logger:     logger,
	}

	tc := &ontologyBuilderTestContext{
		t:          t,
		engineDB:   engineDB,
		service:    service,
		schemaRepo: schemaRepo,
		projectID:  ontologyBuilderTestProjectID,
		dsID:       ontologyBuilderTestDSID,
	}

	// Ensure project and datasource exist
	tc.ensureTestProject()
	tc.ensureTestDatasource()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *ontologyBuilderTestContext) createTestContext() (context.Context, func()) {
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

// ensureTestProject creates the test project if it doesn't exist.
func (tc *ontologyBuilderTestContext) ensureTestProject() {
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
	`, tc.projectID, "Ontology Builder Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestDatasource creates the test datasource if it doesn't exist.
func (tc *ontologyBuilderTestContext) ensureTestDatasource() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for datasource setup: %v", err)
	}
	defer scope.Close()

	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.dsID, tc.projectID, "Ontology Builder Test Datasource", "postgres", "{}")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
}

// cleanup removes all schema data for the test datasource.
func (tc *ontologyBuilderTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete in reverse order of dependencies
	_, err = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_relationships WHERE project_id = $1`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup relationships: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_columns WHERE project_id = $1`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup columns: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `DELETE FROM engine_schema_tables WHERE project_id = $1`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup tables: %v", err)
	}
}

// createTestTable creates a test table and returns it.
func (tc *ontologyBuilderTestContext) createTestTable(ctx context.Context, schemaName, tableName string, isSelected bool) *models.SchemaTable {
	tc.t.Helper()

	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   schemaName,
		TableName:    tableName,
		IsSelected:   isSelected,
	}

	if err := tc.schemaRepo.UpsertTable(ctx, table); err != nil {
		tc.t.Fatalf("Failed to create test table: %v", err)
	}

	return table
}

// createTestColumn creates a test column and returns it.
func (tc *ontologyBuilderTestContext) createTestColumn(ctx context.Context, tableID uuid.UUID, columnName, dataType string, ordinal int, isSelected bool) *models.SchemaColumn {
	tc.t.Helper()

	column := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   tableID,
		ColumnName:      columnName,
		DataType:        dataType,
		IsNullable:      true,
		IsPrimaryKey:    columnName == "id",
		IsSelected:      isSelected,
		OrdinalPosition: ordinal,
	}

	if err := tc.schemaRepo.UpsertColumn(ctx, column); err != nil {
		tc.t.Fatalf("Failed to create test column: %v", err)
	}

	return column
}

// ============================================================================
// loadTablesWithColumns Integration Tests
// ============================================================================

// TestLoadTablesWithColumns_Integration tests that loadTablesWithColumns returns
// tables with their columns populated. This is the fix for the bug where
// BuildTieredOntology was calling ListTablesByDatasource which returns tables
// WITHOUT columns, causing the LLM to hallucinate column names.
func TestLoadTablesWithColumns_Integration(t *testing.T) {
	tc := setupOntologyBuilderTest(t)
	tc.cleanup()
	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create test table with columns that match the real-world bug scenario
	// The LLM was hallucinating "user_id" instead of "owner_id" and
	// "offer_value" instead of "fee_per_minute"
	table := tc.createTestTable(ctx, "public", "offers", true)
	tc.createTestColumn(ctx, table.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, table.ID, "owner_id", "uuid", 2, true)
	tc.createTestColumn(ctx, table.ID, "fee_per_minute", "numeric", 3, true)

	// Call the helper method that should load tables WITH columns
	tables, err := tc.service.loadTablesWithColumns(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("loadTablesWithColumns failed: %v", err)
	}

	// ASSERTIONS
	if len(tables) != 1 {
		t.Fatalf("Expected 1 table, got %d", len(tables))
	}

	offersTable := tables[0]
	if len(offersTable.Columns) != 3 {
		t.Errorf("Expected 3 columns, got %d", len(offersTable.Columns))
	}

	// Verify specific columns exist (these get hallucinated without the fix)
	columnNames := make(map[string]bool)
	for _, col := range offersTable.Columns {
		columnNames[col.ColumnName] = true
	}

	if !columnNames["owner_id"] {
		t.Error("Missing column 'owner_id' - LLM hallucinates this as 'user_id' without fix")
	}
	if !columnNames["fee_per_minute"] {
		t.Error("Missing column 'fee_per_minute' - LLM hallucinates this as 'offer_value' without fix")
	}
}

// TestBuildSchemaSummaryForDescription_IncludesColumns tests that the schema summary
// for ProcessProjectDescription includes column names with data types and flags.
// This is the first LLM call and was missing columns, causing the assessment to
// show columns_included: false and input_assessment score of 45.
func TestBuildSchemaSummaryForDescription_IncludesColumns(t *testing.T) {
	// Create service (no DB needed for this unit test)
	service := &ontologyBuilderService{}

	// Create test tables with columns including PK and nullable flags
	tables := []*models.SchemaTable{
		{
			TableName: "offers",
			Columns: []models.SchemaColumn{
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ColumnName: "owner_id", DataType: "uuid", IsNullable: true},
				{ColumnName: "fee_per_minute", DataType: "numeric"},
			},
		},
		{
			TableName: "users",
			Columns: []models.SchemaColumn{
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ColumnName: "email", DataType: "text"},
			},
		},
	}

	// Build schema summary
	summary := service.buildSchemaSummaryForDescription(tables)

	// Verify column names are included with data types
	if !strings.Contains(summary, "owner_id: uuid") {
		t.Error("Schema summary missing 'owner_id: uuid' - must include data type")
	}
	if !strings.Contains(summary, "fee_per_minute: numeric") {
		t.Error("Schema summary missing 'fee_per_minute: numeric' - must include data type")
	}
	if !strings.Contains(summary, "email: text") {
		t.Error("Schema summary missing 'email: text' - must include data type")
	}

	// Verify PK flags are included
	if !strings.Contains(summary, "[PK]") {
		t.Error("Schema summary missing [PK] flag for primary key columns")
	}

	// Verify nullable flags are included
	if !strings.Contains(summary, "[nullable]") && !strings.Contains(summary, "nullable") {
		t.Error("Schema summary missing nullable flag for nullable columns")
	}

	// Verify column count is shown (regression test for "Columns (0):" bug)
	if !strings.Contains(summary, "Columns (3)") {
		t.Errorf("Schema summary should show 'Columns (3)' for offers table.\nGot: %s", summary)
	}
	if !strings.Contains(summary, "Columns (2)") {
		t.Errorf("Schema summary should show 'Columns (2)' for users table.\nGot: %s", summary)
	}
}

// TestBuildSchemaSummaryForDescription_EmptyColumns tests that the schema summary
// shows "Columns (0):" when a table has no columns. This is a regression test for
// the bug where ProcessDescription used ListTablesByDatasource (which doesn't load
// columns) instead of loadTablesWithColumns.
func TestBuildSchemaSummaryForDescription_EmptyColumns(t *testing.T) {
	service := &ontologyBuilderService{}

	// Simulate the bug: table loaded without columns
	tables := []*models.SchemaTable{
		{
			TableName: "offers",
			Columns:   nil, // No columns loaded - this was the bug!
		},
	}

	summary := service.buildSchemaSummaryForDescription(tables)

	// This test documents the buggy behavior - if you see "Columns (0):" in production,
	// it means columns weren't loaded properly
	if strings.Contains(summary, "Columns (0):") {
		t.Log("WARNING: Table has no columns - this indicates a data loading bug")
		t.Log("ProcessDescription must use loadTablesWithColumns, not ListTablesByDatasource")
	}
}

// TestBuildTier1PromptWithContext_IncludesColumns tests that the Tier 1 prompt
// for BuildTieredOntology includes column names with data types.
func TestBuildTier1PromptWithContext_IncludesColumns(t *testing.T) {
	// Create service (no DB needed for this unit test)
	service := &ontologyBuilderService{}

	// Create test tables with columns
	tables := []*models.SchemaTable{
		{
			TableName: "offers",
			Columns: []models.SchemaColumn{
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
				{ColumnName: "owner_id", DataType: "uuid"},
				{ColumnName: "fee_per_minute", DataType: "numeric"},
			},
		},
	}

	// Build Tier 1 prompt
	prompt := service.buildTier1PromptWithContext(tables, nil, nil)

	// Verify column names are included with data types
	if !strings.Contains(prompt, "owner_id (uuid)") {
		t.Error("Tier 1 prompt missing 'owner_id (uuid)' - LLM will hallucinate column names")
	}
	if !strings.Contains(prompt, "fee_per_minute (numeric)") {
		t.Error("Tier 1 prompt missing 'fee_per_minute (numeric)' - LLM will hallucinate column names")
	}
	if !strings.Contains(prompt, "id (uuid) [PK]") {
		t.Error("Tier 1 prompt missing 'id (uuid) [PK]' - primary key marker missing")
	}

	// Verify "Columns:" section exists
	if !strings.Contains(prompt, "Columns:") {
		t.Error("Tier 1 prompt missing 'Columns:' section header")
	}
}

// TestLoadTablesWithColumns_MultipleTablesAndColumns_Integration tests that
// loadTablesWithColumns correctly associates columns with their respective tables.
func TestLoadTablesWithColumns_MultipleTablesAndColumns_Integration(t *testing.T) {
	tc := setupOntologyBuilderTest(t)
	tc.cleanup()
	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create multiple tables with different column counts
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2, true)

	channelsTable := tc.createTestTable(ctx, "public", "channels", true)
	tc.createTestColumn(ctx, channelsTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, channelsTable.ID, "owner_id", "uuid", 2, true)
	tc.createTestColumn(ctx, channelsTable.ID, "name", "text", 3, true)
	tc.createTestColumn(ctx, channelsTable.ID, "description", "text", 4, true)

	// Load tables with columns
	tables, err := tc.service.loadTablesWithColumns(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("loadTablesWithColumns failed: %v", err)
	}

	if len(tables) != 2 {
		t.Fatalf("Expected 2 tables, got %d", len(tables))
	}

	// Build map for easier lookup
	tablesByName := make(map[string]*models.SchemaTable)
	for _, tbl := range tables {
		tablesByName[tbl.TableName] = tbl
	}

	// Verify users table has 2 columns
	if users, ok := tablesByName["users"]; ok {
		if len(users.Columns) != 2 {
			t.Errorf("users table: expected 2 columns, got %d", len(users.Columns))
		}
	} else {
		t.Error("users table not found")
	}

	// Verify channels table has 4 columns
	if channels, ok := tablesByName["channels"]; ok {
		if len(channels.Columns) != 4 {
			t.Errorf("channels table: expected 4 columns, got %d", len(channels.Columns))
		}
	} else {
		t.Error("channels table not found")
	}
}
