//go:build integration

package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// schemaTestContext holds all dependencies for schema repository integration tests.
type schemaTestContext struct {
	t         *testing.T
	engineDB  *testhelpers.EngineDB
	repo      SchemaRepository
	projectID uuid.UUID
	dsID      uuid.UUID // test datasource ID
}

// setupSchemaTest creates a test context with real database.
func setupSchemaTest(t *testing.T) *schemaTestContext {
	t.Helper()

	engineDB := testhelpers.GetEngineDB(t)
	repo := NewSchemaRepository()

	// Use fixed IDs for consistent testing
	projectID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	dsID := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	tc := &schemaTestContext{
		t:         t,
		engineDB:  engineDB,
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
func (tc *schemaTestContext) createTestContext() (context.Context, func()) {
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
func (tc *schemaTestContext) ensureTestProject() {
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
	`, tc.projectID, "Schema Test Project")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test project: %v", err)
	}
}

// ensureTestDatasource creates the test datasource if it doesn't exist.
func (tc *schemaTestContext) ensureTestDatasource() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for datasource setup: %v", err)
	}
	defer scope.Close()

	// Create test datasource using correct column names
	_, err = scope.Conn.Exec(ctx, `
		INSERT INTO engine_datasources (id, project_id, name, datasource_type, datasource_config)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (id) DO NOTHING
	`, tc.dsID, tc.projectID, "Schema Test Datasource", "postgres", "{}")
	if err != nil {
		tc.t.Fatalf("Failed to ensure test datasource: %v", err)
	}
}

// cleanup removes all schema data for the test datasource.
func (tc *schemaTestContext) cleanup() {
	tc.t.Helper()

	ctx := context.Background()
	scope, err := tc.engineDB.DB.WithTenant(ctx, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to create tenant scope for cleanup: %v", err)
	}
	defer scope.Close()

	// Delete in reverse order of dependencies
	_, err = scope.Conn.Exec(ctx, `
		DELETE FROM engine_schema_relationships
		WHERE project_id = $1
	`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup relationships: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `
		DELETE FROM engine_schema_columns
		WHERE project_id = $1
	`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup columns: %v", err)
	}

	_, err = scope.Conn.Exec(ctx, `
		DELETE FROM engine_schema_tables
		WHERE project_id = $1
	`, tc.projectID)
	if err != nil {
		tc.t.Fatalf("Failed to cleanup tables: %v", err)
	}
}

// createTestTable creates a test table and returns it.
func (tc *schemaTestContext) createTestTable(ctx context.Context, schemaName, tableName string) *models.SchemaTable {
	tc.t.Helper()

	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   schemaName,
		TableName:    tableName,
		IsSelected:   false,
	}

	if err := tc.repo.UpsertTable(ctx, table); err != nil {
		tc.t.Fatalf("Failed to create test table: %v", err)
	}

	return table
}

// createTestColumn creates a test column and returns it.
func (tc *schemaTestContext) createTestColumn(ctx context.Context, tableID uuid.UUID, columnName string, ordinal int) *models.SchemaColumn {
	tc.t.Helper()

	column := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   tableID,
		ColumnName:      columnName,
		DataType:        "text",
		IsNullable:      true,
		IsPrimaryKey:    false,
		IsSelected:      false,
		OrdinalPosition: ordinal,
	}

	if err := tc.repo.UpsertColumn(ctx, column); err != nil {
		tc.t.Fatalf("Failed to create test column: %v", err)
	}

	return column
}

// ============================================================================
// Table Operations Tests
// ============================================================================

func TestSchemaRepository_UpsertTable_Create(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   "public",
		TableName:    "users",
		IsSelected:   false,
		RowCount:     ptr(int64(100)),
	}

	err := tc.repo.UpsertTable(ctx, table)
	if err != nil {
		t.Fatalf("UpsertTable failed: %v", err)
	}

	// Verify ID was assigned
	if table.ID == uuid.Nil {
		t.Error("expected ID to be assigned")
	}

	// Verify timestamps were set
	if table.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}
	if table.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set")
	}

	// Verify data was persisted
	retrieved, err := tc.repo.GetTableByID(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("GetTableByID failed: %v", err)
	}

	if retrieved.TableName != "users" {
		t.Errorf("expected TableName 'users', got %q", retrieved.TableName)
	}
	if *retrieved.RowCount != 100 {
		t.Errorf("expected RowCount 100, got %d", *retrieved.RowCount)
	}
}

func TestSchemaRepository_UpsertTable_Update(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create initial table with business_name
	table := tc.createTestTable(ctx, "public", "products")
	businessName := "Product Catalog"
	table.BusinessName = &businessName

	// Update business_name via direct SQL to simulate user modification
	scope, _ := database.GetTenantScope(ctx)
	_, err := scope.Conn.Exec(ctx, `
		UPDATE engine_schema_tables SET business_name = $1 WHERE id = $2
	`, businessName, table.ID)
	if err != nil {
		t.Fatalf("Failed to set business_name: %v", err)
	}

	originalID := table.ID
	originalCreatedAt := table.CreatedAt

	// Upsert with updated row_count (simulating schema refresh)
	table.RowCount = ptr(int64(500))
	table.ID = uuid.Nil // Clear ID to test upsert behavior

	err = tc.repo.UpsertTable(ctx, table)
	if err != nil {
		t.Fatalf("UpsertTable failed: %v", err)
	}

	// Verify ID was preserved (same record)
	if table.ID != originalID {
		t.Errorf("expected ID to be preserved (%s), got %s", originalID, table.ID)
	}

	// Verify CreatedAt was preserved
	if !table.CreatedAt.Equal(originalCreatedAt) {
		t.Errorf("expected CreatedAt to be preserved")
	}

	// Verify business_name was preserved
	if table.BusinessName == nil || *table.BusinessName != businessName {
		t.Errorf("expected business_name to be preserved, got %v", table.BusinessName)
	}

	// Verify row_count was updated
	retrieved, err := tc.repo.GetTableByID(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("GetTableByID failed: %v", err)
	}

	if retrieved.RowCount == nil || *retrieved.RowCount != 500 {
		t.Errorf("expected RowCount 500, got %v", retrieved.RowCount)
	}
}

func TestSchemaRepository_UpsertTable_ReactivateSoftDeleted(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create and soft-delete a table
	table := tc.createTestTable(ctx, "public", "archived_table")
	businessName := "Important Table"
	description := "This table has important data"

	// Set business metadata
	err := tc.repo.UpdateTableMetadata(ctx, tc.projectID, table.ID, &businessName, &description)
	if err != nil {
		t.Fatalf("UpdateTableMetadata failed: %v", err)
	}

	originalID := table.ID

	// Soft-delete the table
	_, err = tc.repo.SoftDeleteRemovedTables(ctx, tc.projectID, tc.dsID, []TableKey{})
	if err != nil {
		t.Fatalf("SoftDeleteRemovedTables failed: %v", err)
	}

	// Verify it's no longer visible
	_, err = tc.repo.GetTableByName(ctx, tc.projectID, tc.dsID, "public", "archived_table")
	if err == nil {
		t.Error("expected table to be not found after soft-delete")
	}

	// Reactivate by upserting
	newTable := &models.SchemaTable{
		ProjectID:    tc.projectID,
		DatasourceID: tc.dsID,
		SchemaName:   "public",
		TableName:    "archived_table",
		RowCount:     ptr(int64(200)),
	}

	err = tc.repo.UpsertTable(ctx, newTable)
	if err != nil {
		t.Fatalf("UpsertTable (reactivate) failed: %v", err)
	}

	// Verify original ID was preserved
	if newTable.ID != originalID {
		t.Errorf("expected ID to be preserved (%s), got %s", originalID, newTable.ID)
	}

	// Verify business metadata was preserved
	if newTable.BusinessName == nil || *newTable.BusinessName != businessName {
		t.Errorf("expected BusinessName to be preserved, got %v", newTable.BusinessName)
	}
	if newTable.Description == nil || *newTable.Description != description {
		t.Errorf("expected Description to be preserved, got %v", newTable.Description)
	}

	// Verify row_count was updated
	if newTable.RowCount == nil || *newTable.RowCount != 200 {
		t.Errorf("expected RowCount 200, got %v", newTable.RowCount)
	}
}

func TestSchemaRepository_ListTablesByDatasource(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create multiple tables
	tc.createTestTable(ctx, "public", "users")
	tc.createTestTable(ctx, "public", "orders")
	tc.createTestTable(ctx, "analytics", "events")

	tables, err := tc.repo.ListTablesByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListTablesByDatasource failed: %v", err)
	}

	if len(tables) != 3 {
		t.Errorf("expected 3 tables, got %d", len(tables))
	}

	// Verify ordering: analytics.events, public.orders, public.users
	expectedOrder := []string{"analytics.events", "public.orders", "public.users"}
	for i, table := range tables {
		actual := table.SchemaName + "." + table.TableName
		if actual != expectedOrder[i] {
			t.Errorf("expected table[%d] to be %q, got %q", i, expectedOrder[i], actual)
		}
	}
}

func TestSchemaRepository_GetTableByName(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	created := tc.createTestTable(ctx, "inventory", "products")

	retrieved, err := tc.repo.GetTableByName(ctx, tc.projectID, tc.dsID, "inventory", "products")
	if err != nil {
		t.Fatalf("GetTableByName failed: %v", err)
	}

	if retrieved.ID != created.ID {
		t.Errorf("expected ID %s, got %s", created.ID, retrieved.ID)
	}
	if retrieved.SchemaName != "inventory" {
		t.Errorf("expected SchemaName 'inventory', got %q", retrieved.SchemaName)
	}
	if retrieved.TableName != "products" {
		t.Errorf("expected TableName 'products', got %q", retrieved.TableName)
	}
}

func TestSchemaRepository_SoftDeleteRemovedTables(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create three tables
	tc.createTestTable(ctx, "public", "keep_me")
	tc.createTestTable(ctx, "public", "delete_me")
	tc.createTestTable(ctx, "public", "also_delete")

	// Soft-delete tables not in the active list
	activeKeys := []TableKey{
		{SchemaName: "public", TableName: "keep_me"},
	}

	deleted, err := tc.repo.SoftDeleteRemovedTables(ctx, tc.projectID, tc.dsID, activeKeys)
	if err != nil {
		t.Fatalf("SoftDeleteRemovedTables failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("expected 2 tables deleted, got %d", deleted)
	}

	// Verify only keep_me is still visible
	tables, err := tc.repo.ListTablesByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListTablesByDatasource failed: %v", err)
	}

	if len(tables) != 1 {
		t.Errorf("expected 1 table remaining, got %d", len(tables))
	}

	if tables[0].TableName != "keep_me" {
		t.Errorf("expected 'keep_me' to remain, got %q", tables[0].TableName)
	}
}

func TestSchemaRepository_UpdateTableSelection(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestTable(ctx, "public", "selectable")

	// Initially not selected
	if table.IsSelected {
		t.Error("expected IsSelected to be false initially")
	}

	// Update to selected
	err := tc.repo.UpdateTableSelection(ctx, tc.projectID, table.ID, true)
	if err != nil {
		t.Fatalf("UpdateTableSelection failed: %v", err)
	}

	// Verify
	retrieved, err := tc.repo.GetTableByID(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("GetTableByID failed: %v", err)
	}

	if !retrieved.IsSelected {
		t.Error("expected IsSelected to be true after update")
	}
}

// ============================================================================
// Column Operations Tests
// ============================================================================

func TestSchemaRepository_UpsertColumn_Create(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestTable(ctx, "public", "users")

	column := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   table.ID,
		ColumnName:      "email",
		DataType:        "varchar(255)",
		IsNullable:      false,
		IsPrimaryKey:    false,
		IsSelected:      false,
		OrdinalPosition: 1,
	}

	err := tc.repo.UpsertColumn(ctx, column)
	if err != nil {
		t.Fatalf("UpsertColumn failed: %v", err)
	}

	// Verify ID was assigned
	if column.ID == uuid.Nil {
		t.Error("expected ID to be assigned")
	}

	// Verify timestamps
	if column.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify data persisted
	retrieved, err := tc.repo.GetColumnByID(ctx, tc.projectID, column.ID)
	if err != nil {
		t.Fatalf("GetColumnByID failed: %v", err)
	}

	if retrieved.DataType != "varchar(255)" {
		t.Errorf("expected DataType 'varchar(255)', got %q", retrieved.DataType)
	}
}

func TestSchemaRepository_UpsertColumn_Update(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestTable(ctx, "public", "products")
	column := tc.createTestColumn(ctx, table.ID, "price", 1)

	// Set stats and business metadata via direct SQL
	scope, _ := database.GetTenantScope(ctx)
	businessName := "Product Price"
	distinctCount := int64(1000)
	_, err := scope.Conn.Exec(ctx, `
		UPDATE engine_schema_columns
		SET business_name = $1, distinct_count = $2
		WHERE id = $3
	`, businessName, distinctCount, column.ID)
	if err != nil {
		t.Fatalf("Failed to set column metadata: %v", err)
	}

	originalID := column.ID
	originalCreatedAt := column.CreatedAt

	// Upsert with updated data_type (simulating schema refresh)
	column.DataType = "numeric(10,2)"
	column.ID = uuid.Nil // Clear to test upsert behavior

	err = tc.repo.UpsertColumn(ctx, column)
	if err != nil {
		t.Fatalf("UpsertColumn failed: %v", err)
	}

	// Verify ID preserved
	if column.ID != originalID {
		t.Errorf("expected ID to be preserved")
	}

	// Verify CreatedAt preserved
	if !column.CreatedAt.Equal(originalCreatedAt) {
		t.Error("expected CreatedAt to be preserved")
	}

	// Verify business metadata preserved
	if column.BusinessName == nil || *column.BusinessName != businessName {
		t.Errorf("expected BusinessName preserved, got %v", column.BusinessName)
	}

	// Verify stats preserved
	if column.DistinctCount == nil || *column.DistinctCount != distinctCount {
		t.Errorf("expected DistinctCount preserved, got %v", column.DistinctCount)
	}

	// Verify data_type updated
	retrieved, err := tc.repo.GetColumnByID(ctx, tc.projectID, column.ID)
	if err != nil {
		t.Fatalf("GetColumnByID failed: %v", err)
	}

	if retrieved.DataType != "numeric(10,2)" {
		t.Errorf("expected DataType 'numeric(10,2)', got %q", retrieved.DataType)
	}
}

func TestSchemaRepository_UpsertColumn_ReactivateSoftDeleted(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestTable(ctx, "public", "metrics")
	column := tc.createTestColumn(ctx, table.ID, "value", 1)

	// Set stats
	distinctCount := int64(500)
	nullCount := int64(10)
	err := tc.repo.UpdateColumnStats(ctx, column.ID, &distinctCount, &nullCount)
	if err != nil {
		t.Fatalf("UpdateColumnStats failed: %v", err)
	}

	originalID := column.ID

	// Soft-delete
	_, err = tc.repo.SoftDeleteRemovedColumns(ctx, table.ID, []string{})
	if err != nil {
		t.Fatalf("SoftDeleteRemovedColumns failed: %v", err)
	}

	// Verify not visible
	_, err = tc.repo.GetColumnByName(ctx, table.ID, "value")
	if err == nil {
		t.Error("expected column to be not found after soft-delete")
	}

	// Reactivate
	newColumn := &models.SchemaColumn{
		ProjectID:       tc.projectID,
		SchemaTableID:   table.ID,
		ColumnName:      "value",
		DataType:        "float",
		IsNullable:      true,
		OrdinalPosition: 1,
	}

	err = tc.repo.UpsertColumn(ctx, newColumn)
	if err != nil {
		t.Fatalf("UpsertColumn (reactivate) failed: %v", err)
	}

	// Verify ID preserved
	if newColumn.ID != originalID {
		t.Errorf("expected ID preserved")
	}

	// Verify stats preserved
	if newColumn.DistinctCount == nil || *newColumn.DistinctCount != distinctCount {
		t.Errorf("expected DistinctCount preserved, got %v", newColumn.DistinctCount)
	}
	if newColumn.NullCount == nil || *newColumn.NullCount != nullCount {
		t.Errorf("expected NullCount preserved, got %v", newColumn.NullCount)
	}
}

func TestSchemaRepository_ListColumnsByTable(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestTable(ctx, "public", "orders")
	tc.createTestColumn(ctx, table.ID, "id", 1)
	tc.createTestColumn(ctx, table.ID, "created_at", 3)
	tc.createTestColumn(ctx, table.ID, "user_id", 2)

	columns, err := tc.repo.ListColumnsByTable(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("ListColumnsByTable failed: %v", err)
	}

	if len(columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(columns))
	}

	// Verify ordering by ordinal_position
	expectedOrder := []string{"id", "user_id", "created_at"}
	for i, col := range columns {
		if col.ColumnName != expectedOrder[i] {
			t.Errorf("expected column[%d] to be %q, got %q", i, expectedOrder[i], col.ColumnName)
		}
	}
}

func TestSchemaRepository_ListColumnsByDatasource(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table1 := tc.createTestTable(ctx, "public", "users")
	table2 := tc.createTestTable(ctx, "public", "orders")

	tc.createTestColumn(ctx, table1.ID, "id", 1)
	tc.createTestColumn(ctx, table1.ID, "name", 2)
	tc.createTestColumn(ctx, table2.ID, "id", 1)
	tc.createTestColumn(ctx, table2.ID, "user_id", 2)

	columns, err := tc.repo.ListColumnsByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListColumnsByDatasource failed: %v", err)
	}

	if len(columns) != 4 {
		t.Errorf("expected 4 columns, got %d", len(columns))
	}
}

func TestSchemaRepository_SoftDeleteRemovedColumns(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestTable(ctx, "public", "test_table")
	tc.createTestColumn(ctx, table.ID, "keep_col", 1)
	tc.createTestColumn(ctx, table.ID, "delete_col", 2)
	tc.createTestColumn(ctx, table.ID, "also_delete", 3)

	deleted, err := tc.repo.SoftDeleteRemovedColumns(ctx, table.ID, []string{"keep_col"})
	if err != nil {
		t.Fatalf("SoftDeleteRemovedColumns failed: %v", err)
	}

	if deleted != 2 {
		t.Errorf("expected 2 columns deleted, got %d", deleted)
	}

	columns, err := tc.repo.ListColumnsByTable(ctx, tc.projectID, table.ID)
	if err != nil {
		t.Fatalf("ListColumnsByTable failed: %v", err)
	}

	if len(columns) != 1 {
		t.Errorf("expected 1 column remaining, got %d", len(columns))
	}

	if columns[0].ColumnName != "keep_col" {
		t.Errorf("expected 'keep_col' to remain, got %q", columns[0].ColumnName)
	}
}

func TestSchemaRepository_UpdateColumnStats(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestTable(ctx, "public", "stats_table")
	column := tc.createTestColumn(ctx, table.ID, "value", 1)

	distinctCount := int64(1500)
	nullCount := int64(25)

	err := tc.repo.UpdateColumnStats(ctx, column.ID, &distinctCount, &nullCount)
	if err != nil {
		t.Fatalf("UpdateColumnStats failed: %v", err)
	}

	retrieved, err := tc.repo.GetColumnByID(ctx, tc.projectID, column.ID)
	if err != nil {
		t.Fatalf("GetColumnByID failed: %v", err)
	}

	if retrieved.DistinctCount == nil || *retrieved.DistinctCount != distinctCount {
		t.Errorf("expected DistinctCount %d, got %v", distinctCount, retrieved.DistinctCount)
	}

	if retrieved.NullCount == nil || *retrieved.NullCount != nullCount {
		t.Errorf("expected NullCount %d, got %v", nullCount, retrieved.NullCount)
	}
}

func TestSchemaRepository_UpdateColumnSelection(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table := tc.createTestTable(ctx, "public", "selectable_cols")
	column := tc.createTestColumn(ctx, table.ID, "selectable_col", 1)

	// Initially not selected
	if column.IsSelected {
		t.Error("expected IsSelected to be false initially")
	}

	// Update to selected
	err := tc.repo.UpdateColumnSelection(ctx, tc.projectID, column.ID, true)
	if err != nil {
		t.Fatalf("UpdateColumnSelection failed: %v", err)
	}

	// Verify
	retrieved, err := tc.repo.GetColumnByID(ctx, tc.projectID, column.ID)
	if err != nil {
		t.Fatalf("GetColumnByID failed: %v", err)
	}

	if !retrieved.IsSelected {
		t.Error("expected IsSelected to be true after update")
	}

	// Update back to not selected
	err = tc.repo.UpdateColumnSelection(ctx, tc.projectID, column.ID, false)
	if err != nil {
		t.Fatalf("UpdateColumnSelection (to false) failed: %v", err)
	}

	retrieved, err = tc.repo.GetColumnByID(ctx, tc.projectID, column.ID)
	if err != nil {
		t.Fatalf("GetColumnByID failed: %v", err)
	}

	if retrieved.IsSelected {
		t.Error("expected IsSelected to be false after second update")
	}
}

func TestSchemaRepository_UpdateColumnSelection_NotFound(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.New()
	err := tc.repo.UpdateColumnSelection(ctx, tc.projectID, nonExistentID, true)
	if err == nil || err.Error() != "column not found" {
		t.Errorf("expected 'column not found' error, got %v", err)
	}
}

// ============================================================================
// Relationship Operations Tests
// ============================================================================

func TestSchemaRepository_UpsertRelationship_Create(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create source and target tables/columns
	usersTable := tc.createTestTable(ctx, "public", "users")
	userIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", 1)

	ordersTable := tc.createTestTable(ctx, "public", "orders")
	orderUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", 2)

	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    ordersTable.ID,
		SourceColumnID:   orderUserIDCol.ID,
		TargetTableID:    usersTable.ID,
		TargetColumnID:   userIDCol.ID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
		IsValidated:      false,
	}

	err := tc.repo.UpsertRelationship(ctx, rel)
	if err != nil {
		t.Fatalf("UpsertRelationship failed: %v", err)
	}

	if rel.ID == uuid.Nil {
		t.Error("expected ID to be assigned")
	}

	if rel.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set")
	}

	// Verify persisted
	retrieved, err := tc.repo.GetRelationshipByID(ctx, tc.projectID, rel.ID)
	if err != nil {
		t.Fatalf("GetRelationshipByID failed: %v", err)
	}

	if retrieved.RelationshipType != models.RelationshipTypeFK {
		t.Errorf("expected RelationshipType 'fk', got %q", retrieved.RelationshipType)
	}

	if retrieved.Cardinality != models.CardinalityNTo1 {
		t.Errorf("expected Cardinality 'N:1', got %q", retrieved.Cardinality)
	}
}

func TestSchemaRepository_UpsertRelationship_ReactivateSoftDeleted(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables/columns
	table1 := tc.createTestTable(ctx, "public", "a")
	col1 := tc.createTestColumn(ctx, table1.ID, "x", 1)

	table2 := tc.createTestTable(ctx, "public", "b")
	col2 := tc.createTestColumn(ctx, table2.ID, "y", 1)

	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    table1.ID,
		SourceColumnID:   col1.ID,
		TargetTableID:    table2.ID,
		TargetColumnID:   col2.ID,
		RelationshipType: models.RelationshipTypeInferred,
		Cardinality:      models.Cardinality1To1,
		Confidence:       0.85,
		IsValidated:      true,
	}

	err := tc.repo.UpsertRelationship(ctx, rel)
	if err != nil {
		t.Fatalf("UpsertRelationship failed: %v", err)
	}

	originalID := rel.ID

	// Soft-delete
	err = tc.repo.SoftDeleteRelationship(ctx, tc.projectID, rel.ID)
	if err != nil {
		t.Fatalf("SoftDeleteRelationship failed: %v", err)
	}

	// Verify not visible
	_, err = tc.repo.GetRelationshipByID(ctx, tc.projectID, rel.ID)
	if err == nil {
		t.Error("expected relationship to be not found after soft-delete")
	}

	// Reactivate with updated confidence
	newRel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    table1.ID,
		SourceColumnID:   col1.ID,
		TargetTableID:    table2.ID,
		TargetColumnID:   col2.ID,
		RelationshipType: models.RelationshipTypeManual,
		Cardinality:      models.Cardinality1To1,
		Confidence:       1.0,
		IsValidated:      true,
	}

	err = tc.repo.UpsertRelationship(ctx, newRel)
	if err != nil {
		t.Fatalf("UpsertRelationship (reactivate) failed: %v", err)
	}

	// Verify ID preserved
	if newRel.ID != originalID {
		t.Errorf("expected ID to be preserved")
	}

	// Verify data updated
	retrieved, err := tc.repo.GetRelationshipByID(ctx, tc.projectID, newRel.ID)
	if err != nil {
		t.Fatalf("GetRelationshipByID failed: %v", err)
	}

	if retrieved.RelationshipType != models.RelationshipTypeManual {
		t.Errorf("expected RelationshipType 'manual', got %q", retrieved.RelationshipType)
	}
}

func TestSchemaRepository_SoftDeleteRelationship(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	table1 := tc.createTestTable(ctx, "public", "t1")
	col1 := tc.createTestColumn(ctx, table1.ID, "c1", 1)

	table2 := tc.createTestTable(ctx, "public", "t2")
	col2 := tc.createTestColumn(ctx, table2.ID, "c2", 1)

	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    table1.ID,
		SourceColumnID:   col1.ID,
		TargetTableID:    table2.ID,
		TargetColumnID:   col2.ID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
	}

	err := tc.repo.UpsertRelationship(ctx, rel)
	if err != nil {
		t.Fatalf("UpsertRelationship failed: %v", err)
	}

	// Soft-delete
	err = tc.repo.SoftDeleteRelationship(ctx, tc.projectID, rel.ID)
	if err != nil {
		t.Fatalf("SoftDeleteRelationship failed: %v", err)
	}

	// Verify not found
	_, err = tc.repo.GetRelationshipByID(ctx, tc.projectID, rel.ID)
	if err == nil {
		t.Error("expected relationship to be not found after soft-delete")
	}

	// Verify not in list
	rels, err := tc.repo.ListRelationshipsByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListRelationshipsByDatasource failed: %v", err)
	}

	if len(rels) != 0 {
		t.Errorf("expected 0 relationships, got %d", len(rels))
	}
}

func TestSchemaRepository_SoftDeleteOrphanedRelationships(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables and columns
	usersTable := tc.createTestTable(ctx, "public", "users")
	userIDCol := tc.createTestColumn(ctx, usersTable.ID, "id", 1)

	ordersTable := tc.createTestTable(ctx, "public", "orders")
	orderUserIDCol := tc.createTestColumn(ctx, ordersTable.ID, "user_id", 1)

	productsTable := tc.createTestTable(ctx, "public", "products")
	productIDCol := tc.createTestColumn(ctx, productsTable.ID, "id", 1)

	orderItemsTable := tc.createTestTable(ctx, "public", "order_items")
	itemProductIDCol := tc.createTestColumn(ctx, orderItemsTable.ID, "product_id", 1)

	// Create two relationships
	rel1 := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    ordersTable.ID,
		SourceColumnID:   orderUserIDCol.ID,
		TargetTableID:    usersTable.ID,
		TargetColumnID:   userIDCol.ID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
	}
	err := tc.repo.UpsertRelationship(ctx, rel1)
	if err != nil {
		t.Fatalf("UpsertRelationship 1 failed: %v", err)
	}

	rel2 := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    orderItemsTable.ID,
		SourceColumnID:   itemProductIDCol.ID,
		TargetTableID:    productsTable.ID,
		TargetColumnID:   productIDCol.ID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
	}
	err = tc.repo.UpsertRelationship(ctx, rel2)
	if err != nil {
		t.Fatalf("UpsertRelationship 2 failed: %v", err)
	}

	// Verify 2 relationships exist
	rels, _ := tc.repo.ListRelationshipsByDatasource(ctx, tc.projectID, tc.dsID)
	if len(rels) != 2 {
		t.Fatalf("expected 2 relationships, got %d", len(rels))
	}

	// Soft-delete the user_id column from orders
	_, err = tc.repo.SoftDeleteRemovedColumns(ctx, ordersTable.ID, []string{})
	if err != nil {
		t.Fatalf("SoftDeleteRemovedColumns failed: %v", err)
	}

	// Cascade soft-delete orphaned relationships
	deleted, err := tc.repo.SoftDeleteOrphanedRelationships(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("SoftDeleteOrphanedRelationships failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 relationship orphaned, got %d", deleted)
	}

	// Verify only rel2 remains
	rels, err = tc.repo.ListRelationshipsByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListRelationshipsByDatasource failed: %v", err)
	}

	if len(rels) != 1 {
		t.Errorf("expected 1 relationship remaining, got %d", len(rels))
	}

	if rels[0].ID != rel2.ID {
		t.Errorf("expected rel2 to remain, got %s", rels[0].ID)
	}
}

func TestSchemaRepository_GetRelationshipByColumns_Success(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	// Create tables and columns
	table1 := tc.createTestTable(ctx, "public", "lookup_source")
	col1 := tc.createTestColumn(ctx, table1.ID, "fk_col", 1)

	table2 := tc.createTestTable(ctx, "public", "lookup_target")
	col2 := tc.createTestColumn(ctx, table2.ID, "pk_col", 1)

	// Create relationship
	rel := &models.SchemaRelationship{
		ProjectID:        tc.projectID,
		SourceTableID:    table1.ID,
		SourceColumnID:   col1.ID,
		TargetTableID:    table2.ID,
		TargetColumnID:   col2.ID,
		RelationshipType: models.RelationshipTypeFK,
		Cardinality:      models.CardinalityNTo1,
		Confidence:       1.0,
	}
	err := tc.repo.UpsertRelationship(ctx, rel)
	if err != nil {
		t.Fatalf("UpsertRelationship failed: %v", err)
	}

	// Lookup by column pair
	retrieved, err := tc.repo.GetRelationshipByColumns(ctx, col1.ID, col2.ID)
	if err != nil {
		t.Fatalf("GetRelationshipByColumns failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected relationship to be found")
	}

	if retrieved.ID != rel.ID {
		t.Errorf("expected ID %s, got %s", rel.ID, retrieved.ID)
	}

	if retrieved.RelationshipType != models.RelationshipTypeFK {
		t.Errorf("expected RelationshipType 'fk', got %q", retrieved.RelationshipType)
	}
}

func TestSchemaRepository_GetRelationshipByColumns_NotFound(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID1 := uuid.New()
	nonExistentID2 := uuid.New()

	// GetRelationshipByColumns returns nil, nil when not found (not an error)
	retrieved, err := tc.repo.GetRelationshipByColumns(ctx, nonExistentID1, nonExistentID2)
	if err != nil {
		t.Fatalf("GetRelationshipByColumns should not return error for not found, got: %v", err)
	}

	if retrieved != nil {
		t.Error("expected nil result when relationship not found")
	}
}

// ============================================================================
// Edge Case Tests
// ============================================================================

func TestSchemaRepository_NoTenantScope(t *testing.T) {
	tc := setupSchemaTest(t)

	// Use context WITHOUT tenant scope
	ctx := context.Background()

	_, err := tc.repo.ListTablesByDatasource(ctx, tc.projectID, tc.dsID)
	if err == nil {
		t.Error("expected error when no tenant scope")
	}

	expectedErr := "no tenant scope in context"
	if err.Error() != expectedErr {
		t.Errorf("expected error %q, got %q", expectedErr, err.Error())
	}
}

func TestSchemaRepository_NotFound(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	nonExistentID := uuid.New()

	// GetTableByID
	_, err := tc.repo.GetTableByID(ctx, tc.projectID, nonExistentID)
	if err == nil || err.Error() != "table not found" {
		t.Errorf("expected 'table not found' error, got %v", err)
	}

	// GetTableByName
	_, err = tc.repo.GetTableByName(ctx, tc.projectID, tc.dsID, "nonexistent", "table")
	if err == nil || err.Error() != "table not found" {
		t.Errorf("expected 'table not found' error, got %v", err)
	}

	// GetColumnByID
	_, err = tc.repo.GetColumnByID(ctx, tc.projectID, nonExistentID)
	if err == nil || err.Error() != "column not found" {
		t.Errorf("expected 'column not found' error, got %v", err)
	}

	// GetColumnByName
	_, err = tc.repo.GetColumnByName(ctx, nonExistentID, "nonexistent")
	if err == nil || err.Error() != "column not found" {
		t.Errorf("expected 'column not found' error, got %v", err)
	}

	// GetRelationshipByID
	_, err = tc.repo.GetRelationshipByID(ctx, tc.projectID, nonExistentID)
	if err == nil || err.Error() != "relationship not found" {
		t.Errorf("expected 'relationship not found' error, got %v", err)
	}

	// UpdateTableSelection with non-existent table
	err = tc.repo.UpdateTableSelection(ctx, tc.projectID, nonExistentID, true)
	if err == nil || err.Error() != "table not found" {
		t.Errorf("expected 'table not found' error, got %v", err)
	}

	// UpdateColumnStats with non-existent column
	distinctCount := int64(100)
	err = tc.repo.UpdateColumnStats(ctx, nonExistentID, &distinctCount, nil)
	if err == nil || err.Error() != "column not found" {
		t.Errorf("expected 'column not found' error, got %v", err)
	}

	// SoftDeleteRelationship with non-existent relationship
	err = tc.repo.SoftDeleteRelationship(ctx, tc.projectID, nonExistentID)
	if err == nil || err.Error() != "relationship not found" {
		t.Errorf("expected 'relationship not found' error, got %v", err)
	}
}

// ============================================================================
// Helper Functions
// ============================================================================

func ptr[T any](v T) *T {
	return &v
}

// Verify tests completed within reasonable time
func TestSchemaRepository_PerformanceBaseline(t *testing.T) {
	tc := setupSchemaTest(t)
	tc.cleanup()

	ctx, cleanup := tc.createTestContext()
	defer cleanup()

	start := time.Now()

	// Create 50 tables with 5 columns each
	for i := 0; i < 50; i++ {
		table := tc.createTestTable(ctx, "public", "table_"+string(rune('a'+i)))
		for j := 0; j < 5; j++ {
			tc.createTestColumn(ctx, table.ID, "col_"+string(rune('a'+j)), j+1)
		}
	}

	// List all tables
	tables, err := tc.repo.ListTablesByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListTablesByDatasource failed: %v", err)
	}

	if len(tables) != 50 {
		t.Errorf("expected 50 tables, got %d", len(tables))
	}

	// List all columns
	columns, err := tc.repo.ListColumnsByDatasource(ctx, tc.projectID, tc.dsID)
	if err != nil {
		t.Fatalf("ListColumnsByDatasource failed: %v", err)
	}

	if len(columns) != 250 {
		t.Errorf("expected 250 columns, got %d", len(columns))
	}

	elapsed := time.Since(start)
	if elapsed > 10*time.Second {
		t.Errorf("test took too long: %v", elapsed)
	}

	t.Logf("Created 50 tables with 250 columns and listed them in %v", elapsed)
}
