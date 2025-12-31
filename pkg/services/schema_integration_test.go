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

// schemaServiceTestContext holds all dependencies for schema service integration tests.
type schemaServiceTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	service   SchemaService
	repo      repositories.SchemaRepository
	projectID uuid.UUID
	dsID      uuid.UUID
}

// setupSchemaServiceTest creates a test context with real database.
func setupSchemaServiceTest(t *testing.T) *schemaServiceTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	repo := repositories.NewSchemaRepository()
	logger := zap.NewNop()

	// Create service with nil dependencies (not needed for these tests)
	service := NewSchemaService(repo, nil, nil, nil, logger)

	// Use fixed IDs for consistent testing (different from repository tests)
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000102")
	dsID := uuid.MustParse("00000000-0000-0000-0000-000000000103")

	tc := &schemaServiceTestContext{
		t:         t,
		engineDB:  engineDB,
		service:   service,
		repo:      repo,
		projectID: projectID,
		dsID:      dsID,
	}

	// Ensure project and datasource exist
	tc.ensureTestProject()
	tc.ensureTestDatasource()

	return tc
}

// createTestContext creates a context with tenant scope and returns a cleanup function.
func (tc *schemaServiceTestContext) createTestContext() (context.Context, func()) {
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
func (tc *schemaServiceTestContext) ensureTestProject() {
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
	`, tc.projectID, "Schema Service Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestDatasource creates the test datasource if it doesn't exist.
func (tc *schemaServiceTestContext) ensureTestDatasource() {
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
	`, tc.dsID, tc.projectID, "Schema Service Test Datasource", "postgres", "{}")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
}

// cleanup removes all schema data for the test datasource.
func (tc *schemaServiceTestContext) cleanup() {
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
func (tc *schemaServiceTestContext) createTestTable(ctx context.Context, schemaName, tableName string, isSelected bool) *models.SchemaTable {
	tc.t.Helper()

	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   schemaName,
		TableName:    tableName,
		IsSelected:   isSelected,
	}

	if err := tc.repo.UpsertTable(ctx, table); err != nil {
		tc.t.Fatalf("Failed to create test table: %v", err)
	}

	return table
}

// createTestColumn creates a test column and returns it.
func (tc *schemaServiceTestContext) createTestColumn(ctx context.Context, tableID uuid.UUID, columnName, dataType string, ordinal int, isSelected bool) *models.SchemaColumn {
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

	if err := tc.repo.UpsertColumn(ctx, column); err != nil {
		tc.t.Fatalf("Failed to create test column: %v", err)
	}

	return column
}

// createTestRelationship creates a test relationship and returns it.
func (tc *schemaServiceTestContext) createTestRelationship(ctx context.Context, sourceTableID, sourceColumnID, targetTableID, targetColumnID uuid.UUID) *models.SchemaRelationship {
	tc.t.Helper()

	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    sourceTableID,
		SourceColumnID:   sourceColumnID,
		TargetTableID:    targetTableID,
		TargetColumnID:   targetColumnID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
	}

	if err := tc.repo.UpsertRelationship(ctx, rel); err != nil {
		tc.t.Fatalf("Failed to create test relationship: %v", err)
	}

	return rel
}

// ============================================================================
// UpdateTableMetadata Integration Tests
// ============================================================================

func TestSchemaService_UpdateTableMetadata_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a table
	table := tc.createTestTable(ctx, "public", "users", false)

	// Update metadata
	businessName := "User Accounts"
	description := "Stores registered user information"
	err := tc.service.UpdateTableMetadata(ctx, tc.projectID, table.ID, &businessName, &description)
	if err != nil {
		t.Fatalf("UpdateTableMetadata failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := tc.repo.GetTableByID(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("GetTableByID failed: %v", err)
	}

	if retrieved.BusinessName == nil || *retrieved.BusinessName != businessName {
		t.Errorf("expected BusinessName %q, got %v", businessName, retrieved.BusinessName)
	}
	if retrieved.Description == nil || *retrieved.Description != description {
		t.Errorf("expected Description %q, got %v", description, retrieved.Description)
	}
}

func TestSchemaService_UpdateTableMetadata_PartialUpdate_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a table with initial metadata
	table := tc.createTestTable(ctx, "public", "orders", false)
	initialName := "Orders"
	initialDesc := "Order records"
	err := tc.service.UpdateTableMetadata(ctx, tc.projectID, table.ID, &initialName, &initialDesc)
	if err != nil {
		t.Fatalf("Initial UpdateTableMetadata failed: %v", err)
	}

	// Verify initial metadata was set
	retrieved, err := tc.repo.GetTableByID(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("GetTableByID after initial update failed: %v", err)
	}
	if retrieved.Description == nil || *retrieved.Description != initialDesc {
		t.Fatalf("Initial Description not set correctly, got %v", retrieved.Description)
	}

	// Update only business_name (description should be preserved)
	newName := "Customer Orders"
	err = tc.service.UpdateTableMetadata(ctx, tc.projectID, table.ID, &newName, nil)
	if err != nil {
		t.Fatalf("Partial UpdateTableMetadata failed: %v", err)
	}

	// Verify
	retrieved, err = tc.repo.GetTableByID(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("GetTableByID failed: %v", err)
	}

	if retrieved.BusinessName == nil || *retrieved.BusinessName != newName {
		t.Errorf("expected BusinessName %q, got %v", newName, retrieved.BusinessName)
	}
	if retrieved.Description == nil || *retrieved.Description != initialDesc {
		t.Errorf("expected Description %q preserved, got %v", initialDesc, retrieved.Description)
	}
}

// ============================================================================
// UpdateColumnMetadata Integration Tests
// ============================================================================

func TestSchemaService_UpdateColumnMetadata_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create table and column
	table := tc.createTestTable(ctx, "public", "products", false)
	column := tc.createTestColumn(ctx, table.ID, "price", "numeric", 1, false)

	// Update metadata
	businessName := "Product Price"
	description := "The retail price in USD"
	err := tc.service.UpdateColumnMetadata(ctx, tc.projectID, column.ID, &businessName, &description)
	if err != nil {
		t.Fatalf("UpdateColumnMetadata failed: %v", err)
	}

	// Retrieve and verify
	retrieved, err := tc.repo.GetColumnByID(ctx, tc.projectID, column.ID)
	if err != nil {
		t.Fatalf("GetColumnByID failed: %v", err)
	}

	if retrieved.BusinessName == nil || *retrieved.BusinessName != businessName {
		t.Errorf("expected BusinessName %q, got %v", businessName, retrieved.BusinessName)
	}
	if retrieved.Description == nil || *retrieved.Description != description {
		t.Errorf("expected Description %q, got %v", description, retrieved.Description)
	}
}

// ============================================================================
// SaveSelections Integration Tests
// ============================================================================

func TestSchemaService_SaveSelections_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create two tables with columns
	usersTable := tc.createTestTable(ctx, "public", "users", false)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, false)
	usersEmailCol := tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2, false)
	tc.createTestColumn(ctx, usersTable.ID, "password_hash", "text", 3, false) // Should not be selected

	ordersTable := tc.createTestTable(ctx, "public", "orders", false)
	ordersIDCol := tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, false)
	ordersUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, false)

	// Save selections using table and column IDs
	tableSelections := map[uuid.UUID]bool{
		usersTable.ID:  true,
		ordersTable.ID: true,
	}
	columnSelections := map[uuid.UUID][]uuid.UUID{
		usersTable.ID:  {usersIDCol.ID, usersEmailCol.ID}, // password_hash excluded
		ordersTable.ID: {ordersIDCol.ID, ordersUserIDCol.ID},
	}

	err := tc.service.SaveSelections(ctx, tc.projectID, tc.dsID, tableSelections, columnSelections)
	if err != nil {
		t.Fatalf("SaveSelections failed: %v", err)
	}

	// Verify table selections
	tables, err := tc.repo.ListTablesByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListTablesByDatasource failed: %v", err)
	}

	for _, table := range tables {
		if !table.IsSelected {
			t.Errorf("expected table %s to be selected", table.TableName)
		}
	}

	// Verify column selections
	usersColumns, err := tc.repo.ListColumnsByTable(ctx, tc.projectID, usersTable.ID)
	if err != nil {
		t.Fatalf("ListColumnsByTable failed: %v", err)
	}

	selectedCount := 0
	for _, col := range usersColumns {
		if col.ColumnName == "password_hash" && col.IsSelected {
			t.Error("expected password_hash to NOT be selected")
		}
		if (col.ColumnName == "id" || col.ColumnName == "email") && !col.IsSelected {
			t.Errorf("expected %s to be selected", col.ColumnName)
		}
		if col.IsSelected {
			selectedCount++
		}
	}

	if selectedCount != 2 {
		t.Errorf("expected 2 selected columns in users, got %d", selectedCount)
	}
}

func TestSchemaService_SaveSelections_Deselect_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create a selected table
	table := tc.createTestTable(ctx, "public", "metrics", true)
	tc.createTestColumn(ctx, table.ID, "value", "numeric", 1, true)

	// Deselect via SaveSelections using table ID
	tableSelections := map[uuid.UUID]bool{
		table.ID: false,
	}
	columnSelections := map[uuid.UUID][]uuid.UUID{
		table.ID: {}, // Empty means deselect all
	}

	err := tc.service.SaveSelections(ctx, tc.projectID, tc.dsID, tableSelections, columnSelections)
	if err != nil {
		t.Fatalf("SaveSelections failed: %v", err)
	}

	// Verify table deselected
	retrieved, err := tc.repo.GetTableByID(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("GetTableByID failed: %v", err)
	}

	if retrieved.IsSelected {
		t.Error("expected table to be deselected")
	}

	// Verify column deselected
	columns, err := tc.repo.ListColumnsByTable(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("ListColumnsByTable failed: %v", err)
	}

	for _, col := range columns {
		if col.IsSelected {
			t.Errorf("expected column %s to be deselected", col.ColumnName)
		}
	}
}

func TestSchemaService_SaveSelections_RoundTrip_Integration(t *testing.T) {
	// This test verifies the full round-trip: save selections, then retrieve
	// schema via GetDatasourceSchema and verify is_selected flags are correct.
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables with columns (all unselected initially)
	usersTable := tc.createTestTable(ctx, "public", "users", false)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, false)
	usersEmailCol := tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2, false)
	tc.createTestColumn(ctx, usersTable.ID, "password_hash", "text", 3, false)

	ordersTable := tc.createTestTable(ctx, "public", "orders", false)
	ordersIDCol := tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, false)
	ordersTotalCol := tc.createTestColumn(ctx, ordersTable.ID, "total", "numeric", 2, false)

	// Save selections using table and column IDs
	tableSelections := map[uuid.UUID]bool{
		usersTable.ID:  true,
		ordersTable.ID: true,
	}
	columnSelections := map[uuid.UUID][]uuid.UUID{
		usersTable.ID:  {usersIDCol.ID, usersEmailCol.ID}, // password_hash excluded
		ordersTable.ID: {ordersIDCol.ID, ordersTotalCol.ID},
	}

	err := tc.service.SaveSelections(ctx, tc.projectID, tc.dsID, tableSelections, columnSelections)
	if err != nil {
		t.Fatalf("SaveSelections failed: %v", err)
	}

	// Now retrieve the schema via GetDatasourceSchema (simulates frontend reload)
	schema, err := tc.service.GetDatasourceSchema(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetDatasourceSchema failed: %v", err)
	}

	// Verify tables have is_selected=true
	for _, tbl := range schema.Tables {
		if !tbl.IsSelected {
			t.Errorf("expected table %s.%s to have is_selected=true after save", tbl.SchemaName, tbl.TableName)
		}
	}

	// Verify columns have correct is_selected state
	for _, tbl := range schema.Tables {
		for _, col := range tbl.Columns {
			// password_hash should not be selected
			if tbl.TableName == "users" && col.ColumnName == "password_hash" {
				if col.IsSelected {
					t.Errorf("expected users.password_hash to have is_selected=false")
				}
			} else {
				// All other columns should be selected
				if !col.IsSelected {
					t.Errorf("expected %s.%s to have is_selected=true", tbl.TableName, col.ColumnName)
				}
			}
		}
	}
}

// ============================================================================
// GetSelectedDatasourceSchema Integration Tests
// ============================================================================

func TestSchemaService_GetSelectedDatasourceSchema_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables - some selected, some not
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2, true)
	tc.createTestColumn(ctx, usersTable.ID, "internal_notes", "text", 3, false) // Not selected

	ordersTable := tc.createTestTable(ctx, "public", "orders", true)
	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, true)

	// Unselected table
	logsTable := tc.createTestTable(ctx, "public", "logs", false)
	tc.createTestColumn(ctx, logsTable.ID, "id", "uuid", 1, true)

	// Create relationship between selected tables
	orderUserIDCol, _ := tc.repo.GetColumnByName(ctx, ordersTable.ID, "user_id")
	usersIDCol, _ := tc.repo.GetColumnByName(ctx, usersTable.ID, "id")
	tc.createTestRelationship(ctx, ordersTable.ID, orderUserIDCol.ID, usersTable.ID, usersIDCol.ID)

	// Get selected schema
	schema, err := tc.service.GetSelectedDatasourceSchema(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetSelectedDatasourceSchema failed: %v", err)
	}

	// Should have 2 tables (users, orders) - logs excluded
	if len(schema.Tables) != 2 {
		t.Errorf("expected 2 selected tables, got %d", len(schema.Tables))
	}

	// Verify users table has 2 columns (internal_notes excluded)
	var usersInSchema *models.DatasourceTable
	for _, tbl := range schema.Tables {
		if tbl.TableName == "users" {
			usersInSchema = tbl
			break
		}
	}

	if usersInSchema == nil {
		t.Fatal("expected users table in schema")
	}

	if len(usersInSchema.Columns) != 2 {
		t.Errorf("expected 2 selected columns in users, got %d", len(usersInSchema.Columns))
	}

	// Verify relationship is included (both tables are selected)
	if len(schema.Relationships) != 1 {
		t.Errorf("expected 1 relationship, got %d", len(schema.Relationships))
	}
}

func TestSchemaService_GetSelectedDatasourceSchema_FiltersRelationships_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create selected and unselected tables
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	usersIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", false) // NOT selected
	orderUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 1, true)

	// Create relationship - should be filtered out because orders is not selected
	tc.createTestRelationship(ctx, ordersTable.ID, orderUserIDCol.ID, usersTable.ID, usersIDCol.ID)

	// Get selected schema
	schema, err := tc.service.GetSelectedDatasourceSchema(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("GetSelectedDatasourceSchema failed: %v", err)
	}

	// Only users should be in the schema
	if len(schema.Tables) != 1 {
		t.Errorf("expected 1 selected table, got %d", len(schema.Tables))
	}

	// Relationship should be filtered out
	if len(schema.Relationships) != 0 {
		t.Errorf("expected 0 relationships (filtered), got %d", len(schema.Relationships))
	}
}

// ============================================================================
// GetDatasourceSchemaForPrompt Integration Tests
// ============================================================================

func TestSchemaService_GetDatasourceSchemaForPrompt_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables with metadata
	usersTable := tc.createTestTable(ctx, "public", "users", true)
	usersTable.RowCount = ptr(int64(1500))
	_ = tc.repo.UpsertTable(ctx, usersTable)

	// Note: prompt format uses Description, not BusinessName
	description := "Registered user accounts"
	_ = tc.service.UpdateTableMetadata(ctx, tc.projectID, usersTable.ID, nil, &description)

	tc.createTestColumn(ctx, usersTable.ID, "id", "uuid", 1, true)
	tc.createTestColumn(ctx, usersTable.ID, "email", "text", 2, true)

	ordersTable := tc.createTestTable(ctx, "public", "orders", true)
	ordersTable.RowCount = ptr(int64(5000))
	_ = tc.repo.UpsertTable(ctx, ordersTable)

	tc.createTestColumn(ctx, ordersTable.ID, "id", "uuid", 1, true)
	userIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", "uuid", 2, true)

	// Create relationship
	usersIDCol, _ := tc.repo.GetColumnByName(ctx, usersTable.ID, "id")
	tc.createTestRelationship(ctx, ordersTable.ID, userIDCol.ID, usersTable.ID, usersIDCol.ID)

	// Get prompt (selected only)
	prompt, err := tc.service.GetDatasourceSchemaForPrompt(ctx, tc.projectID, tc.dsID, true)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaForPrompt failed: %v", err)
	}

	// Verify prompt contains expected content
	if !strings.Contains(prompt, "DATABASE SCHEMA:") {
		t.Error("expected prompt to contain 'DATABASE SCHEMA:'")
	}

	if !strings.Contains(prompt, "Table: users") {
		t.Error("expected prompt to contain 'Table: users'")
	}

	// Verify description is included (not business_name - prompt uses Description field)
	if !strings.Contains(prompt, "Registered user accounts") {
		t.Error("expected prompt to contain description 'Registered user accounts'")
	}

	if !strings.Contains(prompt, "RELATIONSHIPS:") {
		t.Error("expected prompt to contain 'RELATIONSHIPS:'")
	}

	// Relationship format uses schema.table.column: "public.orders.user_id -> public.users.id"
	if !strings.Contains(prompt, "public.orders.user_id -> public.users.id") {
		t.Errorf("expected prompt to contain relationship 'public.orders.user_id -> public.users.id', got:\n%s", prompt)
	}

	// Verify row count is included
	if !strings.Contains(prompt, "1500") {
		t.Error("expected prompt to contain row count 1500")
	}
}

func TestSchemaService_GetDatasourceSchemaForPrompt_Empty_Integration(t *testing.T) {
	tc := setupSchemaServiceTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Don't create any tables

	// Get prompt
	prompt, err := tc.service.GetDatasourceSchemaForPrompt(ctx, tc.projectID, tc.dsID, true)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaForPrompt failed: %v", err)
	}

	// Even with no tables, the header should be present
	if !strings.Contains(prompt, "DATABASE SCHEMA:") {
		t.Error("expected prompt to contain header even when empty")
	}

	// No tables means no "Table:" entries
	if strings.Contains(prompt, "Table:") {
		t.Error("expected prompt to NOT contain 'Table:' when empty")
	}

	// No relationships section when empty
	if strings.Contains(prompt, "RELATIONSHIPS:") {
		t.Error("expected prompt to NOT contain 'RELATIONSHIPS:' when empty")
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func ptr[T any](v T) *T {
	return &v
}
