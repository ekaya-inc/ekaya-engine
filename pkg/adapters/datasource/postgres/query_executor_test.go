//go:build integration && (postgres || all_adapters)

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// queryExecutorTestContext holds dependencies for query executor tests.
type queryExecutorTestContext struct {
	t        *testing.T
	executor *QueryExecutor
}

// setupQueryExecutorTest creates a QueryExecutor connected to the test container.
func setupQueryExecutorTest(t *testing.T) *queryExecutorTestContext {
	t.Helper()

	testDB := testhelpers.GetTestDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get container connection info
	host, err := testDB.Container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}

	port, err := testDB.Container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	cfg := &Config{
		Host:     host,
		Port:     port.Int(),
		User:     "ekaya",
		Password: "test_password",
		Database: "test_data",
		SSLMode:  "disable",
	}

	// Pass nil for connection manager and zero IDs for unmanaged pool (test mode)
	executor, err := NewQueryExecutor(ctx, cfg, nil, uuid.Nil, uuid.Nil, "")
	if err != nil {
		t.Fatalf("failed to create query executor: %v", err)
	}

	t.Cleanup(func() {
		executor.Close()
	})

	return &queryExecutorTestContext{
		t:        t,
		executor: executor,
	}
}

// ============================================================================
// Execution Tests
// ============================================================================

func TestQueryExecutor_ExecuteQuery_Simple(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	result, err := tc.executor.Query(ctx, "SELECT 1 as num, 'hello' as greeting", 0)
	if err != nil {
		t.Fatalf("ExecuteQuery failed: %v", err)
	}

	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(result.Columns))
	}
	if result.Columns[0].Name != "num" {
		t.Errorf("expected first column 'num', got %q", result.Columns[0].Name)
	}
	if result.Columns[1].Name != "greeting" {
		t.Errorf("expected second column 'greeting', got %q", result.Columns[1].Name)
	}

	if result.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", result.RowCount)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row in Rows slice, got %d", len(result.Rows))
	}

	// Check row values
	row := result.Rows[0]
	if row["greeting"] != "hello" {
		t.Errorf("expected greeting 'hello', got %v", row["greeting"])
	}
}

func TestQueryExecutor_ExecuteQuery_FromTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Query the events table which should exist in test data
	result, err := tc.executor.Query(ctx, "SELECT * FROM events LIMIT 5", 0)
	if err != nil {
		t.Fatalf("ExecuteQuery failed: %v", err)
	}

	if len(result.Columns) == 0 {
		t.Error("expected at least one column")
	}
	if result.RowCount > 5 {
		t.Errorf("expected at most 5 rows, got %d", result.RowCount)
	}
}

func TestQueryExecutor_ExecuteQuery_WithLimit(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Query without limit first to see how many rows exist
	resultNoLimit, err := tc.executor.Query(ctx, "SELECT * FROM events", 0)
	if err != nil {
		t.Fatalf("ExecuteQuery without limit failed: %v", err)
	}

	if resultNoLimit.RowCount < 3 {
		t.Skipf("need at least 3 rows to test limit, got %d", resultNoLimit.RowCount)
	}

	// Now query with limit
	result, err := tc.executor.Query(ctx, "SELECT * FROM events", 2)
	if err != nil {
		t.Fatalf("ExecuteQuery with limit failed: %v", err)
	}

	if result.RowCount != 2 {
		t.Errorf("expected 2 rows with limit, got %d", result.RowCount)
	}
}

func TestQueryExecutor_ExecuteQuery_NoResults(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	result, err := tc.executor.Query(ctx, "SELECT * FROM events WHERE 1=0", 0)
	if err != nil {
		t.Fatalf("ExecuteQuery failed: %v", err)
	}

	if result.RowCount != 0 {
		t.Errorf("expected 0 rows, got %d", result.RowCount)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected empty Rows slice, got %d", len(result.Rows))
	}
	// Columns should still be populated even with no results
	if len(result.Columns) == 0 {
		t.Error("expected columns even with no results")
	}
}

func TestQueryExecutor_ExecuteQuery_InvalidSQL(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.Query(ctx, "SELECT * FROM nonexistent_table_xyz", 0)
	if err == nil {
		t.Fatal("expected error for invalid SQL")
	}
}

func TestQueryExecutor_ExecuteQuery_SyntaxError(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.Query(ctx, "SELEC * FORM events", 0)
	if err == nil {
		t.Fatal("expected error for SQL syntax error")
	}
}

func TestQueryExecutor_ExecuteQuery_Join(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Test a simple self-join query using events table
	result, err := tc.executor.Query(ctx, `
		SELECT e1.id, e2.id as e2_id
		FROM events e1
		CROSS JOIN events e2
		LIMIT 5
	`, 0)
	if err != nil {
		t.Fatalf("ExecuteQuery with join failed: %v", err)
	}

	// Should have 2 columns from the join
	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns from join, got %d: %v", len(result.Columns), result.Columns)
	}
}

func TestQueryExecutor_ExecuteQuery_Aggregation(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	result, err := tc.executor.Query(ctx, `
		SELECT COUNT(*) as total, MIN(created_at) as earliest
		FROM events
	`, 0)
	if err != nil {
		t.Fatalf("ExecuteQuery with aggregation failed: %v", err)
	}

	if result.RowCount != 1 {
		t.Errorf("expected 1 row from aggregation, got %d", result.RowCount)
	}
	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(result.Columns))
	}
}

// ============================================================================
// Validation Tests
// ============================================================================

func TestQueryExecutor_ValidateQuery_Valid(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	err := tc.executor.ValidateQuery(ctx, "SELECT * FROM events")
	if err != nil {
		t.Errorf("expected valid SQL to pass validation, got error: %v", err)
	}
}

func TestQueryExecutor_ValidateQuery_ValidComplex(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Use a self-referential query to avoid schema-specific column type issues
	err := tc.executor.ValidateQuery(ctx, `
		SELECT id, COUNT(*) as event_count
		FROM events
		WHERE created_at > '2020-01-01'
		GROUP BY id
		HAVING COUNT(*) > 0
		ORDER BY event_count DESC
	`)
	if err != nil {
		t.Errorf("expected complex valid SQL to pass validation, got error: %v", err)
	}
}

func TestQueryExecutor_ValidateQuery_InvalidSyntax(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	err := tc.executor.ValidateQuery(ctx, "SELEC * FORM events")
	if err == nil {
		t.Error("expected syntax error to fail validation")
	}
}

func TestQueryExecutor_ValidateQuery_NonExistentTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	err := tc.executor.ValidateQuery(ctx, "SELECT * FROM nonexistent_table_xyz")
	if err == nil {
		t.Error("expected non-existent table to fail validation")
	}
}

func TestQueryExecutor_ValidateQuery_NonExistentColumn(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	err := tc.executor.ValidateQuery(ctx, "SELECT nonexistent_column FROM events")
	if err == nil {
		t.Error("expected non-existent column to fail validation")
	}
}

// ============================================================================
// Data Type Tests
// ============================================================================

func TestQueryExecutor_DataTypes(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Test various PostgreSQL data types
	result, err := tc.executor.Query(ctx, `
		SELECT
			1::integer as int_val,
			1.5::numeric as numeric_val,
			'text'::text as text_val,
			true::boolean as bool_val,
			NULL::text as null_val,
			NOW()::timestamptz as timestamp_val,
			gen_random_uuid() as uuid_val
	`, 0)
	if err != nil {
		t.Fatalf("ExecuteQuery with various types failed: %v", err)
	}

	if result.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", result.RowCount)
	}

	row := result.Rows[0]

	// Check that values are present (exact type checking depends on pgx driver behavior)
	if row["int_val"] == nil {
		t.Error("expected int_val to be non-nil")
	}
	if row["text_val"] != "text" {
		t.Errorf("expected text_val 'text', got %v", row["text_val"])
	}
	if row["bool_val"] != true {
		t.Errorf("expected bool_val true, got %v", row["bool_val"])
	}
	if row["null_val"] != nil {
		t.Errorf("expected null_val to be nil, got %v", row["null_val"])
	}
	if row["timestamp_val"] == nil {
		t.Error("expected timestamp_val to be non-nil")
	}
	if row["uuid_val"] == nil {
		t.Error("expected uuid_val to be non-nil")
	}
}

// ============================================================================
// Edge Cases
// ============================================================================

func TestQueryExecutor_ContextCancellation(t *testing.T) {
	tc := setupQueryExecutorTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := tc.executor.Query(ctx, "SELECT pg_sleep(10)", 0)
	if err == nil {
		t.Error("expected error when context is cancelled")
	}
}

// ============================================================================
// Execute() Tests - DDL/DML Execution
// ============================================================================

func TestQueryExecutor_Execute_CreateTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Clean up first in case previous test failed
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_execute_table")

	result, err := tc.executor.Execute(ctx, `
		CREATE TABLE test_execute_table (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("Execute CREATE TABLE failed: %v", err)
	}

	// DDL doesn't affect rows
	if result.RowsAffected != 0 {
		t.Errorf("expected 0 rows affected for CREATE TABLE, got %d", result.RowsAffected)
	}

	// Verify table exists by inserting
	insertResult, err := tc.executor.Execute(ctx, "INSERT INTO test_execute_table (name) VALUES ('test')")
	if err != nil {
		t.Fatalf("INSERT after CREATE failed: %v", err)
	}
	if insertResult.RowsAffected != 1 {
		t.Errorf("expected 1 row affected for INSERT, got %d", insertResult.RowsAffected)
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE test_execute_table")
}

func TestQueryExecutor_Execute_Insert(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Setup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_insert_table")
	_, err := tc.executor.Execute(ctx, "CREATE TABLE test_insert_table (id SERIAL PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("setup CREATE TABLE failed: %v", err)
	}

	// Test INSERT
	result, err := tc.executor.Execute(ctx, "INSERT INTO test_insert_table (value) VALUES ('one'), ('two'), ('three')")
	if err != nil {
		t.Fatalf("Execute INSERT failed: %v", err)
	}

	if result.RowsAffected != 3 {
		t.Errorf("expected 3 rows affected, got %d", result.RowsAffected)
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE test_insert_table")
}

func TestQueryExecutor_Execute_InsertReturning(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Setup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_returning_table")
	_, err := tc.executor.Execute(ctx, "CREATE TABLE test_returning_table (id SERIAL PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("setup CREATE TABLE failed: %v", err)
	}

	// Test INSERT with RETURNING
	result, err := tc.executor.Execute(ctx, "INSERT INTO test_returning_table (value) VALUES ('test') RETURNING id, value")
	if err != nil {
		t.Fatalf("Execute INSERT RETURNING failed: %v", err)
	}

	if result.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", result.RowsAffected)
	}
	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns returned, got %d", len(result.Columns))
	}
	if result.RowCount != 1 {
		t.Errorf("expected 1 row returned, got %d", result.RowCount)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row in Rows, got %d", len(result.Rows))
	}
	if result.Rows[0]["value"] != "test" {
		t.Errorf("expected value 'test', got %v", result.Rows[0]["value"])
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE test_returning_table")
}

func TestQueryExecutor_Execute_Update(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Setup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_update_table")
	_, _ = tc.executor.Execute(ctx, "CREATE TABLE test_update_table (id SERIAL PRIMARY KEY, value TEXT)")
	_, _ = tc.executor.Execute(ctx, "INSERT INTO test_update_table (value) VALUES ('old1'), ('old2')")

	// Test UPDATE
	result, err := tc.executor.Execute(ctx, "UPDATE test_update_table SET value = 'new' WHERE value LIKE 'old%'")
	if err != nil {
		t.Fatalf("Execute UPDATE failed: %v", err)
	}

	if result.RowsAffected != 2 {
		t.Errorf("expected 2 rows affected, got %d", result.RowsAffected)
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE test_update_table")
}

func TestQueryExecutor_Execute_Delete(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Setup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_delete_table")
	_, _ = tc.executor.Execute(ctx, "CREATE TABLE test_delete_table (id SERIAL PRIMARY KEY, value TEXT)")
	_, _ = tc.executor.Execute(ctx, "INSERT INTO test_delete_table (value) VALUES ('one'), ('two'), ('three')")

	// Test DELETE
	result, err := tc.executor.Execute(ctx, "DELETE FROM test_delete_table WHERE value IN ('one', 'two')")
	if err != nil {
		t.Fatalf("Execute DELETE failed: %v", err)
	}

	if result.RowsAffected != 2 {
		t.Errorf("expected 2 rows affected, got %d", result.RowsAffected)
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE test_delete_table")
}

func TestQueryExecutor_Execute_DropTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Create a table first
	_, err := tc.executor.Execute(ctx, "CREATE TABLE test_drop_table (id INT)")
	if err != nil {
		t.Fatalf("setup CREATE TABLE failed: %v", err)
	}

	// Drop it
	result, err := tc.executor.Execute(ctx, "DROP TABLE test_drop_table")
	if err != nil {
		t.Fatalf("Execute DROP TABLE failed: %v", err)
	}

	if result.RowsAffected != 0 {
		t.Errorf("expected 0 rows affected for DROP TABLE, got %d", result.RowsAffected)
	}

	// Verify it's gone by trying to query it
	_, err = tc.executor.Query(ctx, "SELECT * FROM test_drop_table", 0)
	if err == nil {
		t.Error("expected error querying dropped table")
	}
}

// TestQueryExecutor_Execute_DropNonExistentTable verifies that dropping a
// non-existent table returns an error. This is a critical test - PostgreSQL
// returns "ERROR: table does not exist" but the bug causes this to succeed.
func TestQueryExecutor_Execute_DropNonExistentTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Ensure table doesn't exist
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS definitely_does_not_exist_xyz")

	// Try to drop non-existent table - THIS MUST RETURN AN ERROR
	_, err := tc.executor.Execute(ctx, "DROP TABLE definitely_does_not_exist_xyz")
	if err == nil {
		t.Fatal("CRITICAL BUG: Execute DROP TABLE on non-existent table should return error, but got success")
	}

	// Verify error message mentions the table
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected error to mention 'does not exist', got: %v", err)
	}
}

// TestQueryExecutor_Execute_InvalidSQL verifies that syntax errors are returned.
func TestQueryExecutor_Execute_InvalidSQL(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.Execute(ctx, "CREAT TABL broken_syntax")
	if err == nil {
		t.Fatal("expected error for invalid SQL syntax")
	}
}

// TestQueryExecutor_Execute_InsertIntoNonExistentTable verifies errors are
// returned when inserting into a table that doesn't exist.
func TestQueryExecutor_Execute_InsertIntoNonExistentTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.Execute(ctx, "INSERT INTO nonexistent_table_xyz (col) VALUES ('test')")
	if err == nil {
		t.Fatal("expected error inserting into non-existent table")
	}
}

// TestQueryExecutor_Execute_AlterNonExistentTable verifies ALTER on non-existent
// table returns an error.
func TestQueryExecutor_Execute_AlterNonExistentTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.Execute(ctx, "ALTER TABLE nonexistent_table_xyz ADD COLUMN new_col TEXT")
	if err == nil {
		t.Fatal("expected error for ALTER on non-existent table")
	}
}

// TestQueryExecutor_Execute_TruncateNonExistentTable verifies TRUNCATE on
// non-existent table returns an error.
func TestQueryExecutor_Execute_TruncateNonExistentTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.Execute(ctx, "TRUNCATE TABLE nonexistent_table_xyz")
	if err == nil {
		t.Fatal("expected error for TRUNCATE on non-existent table")
	}
}

// ============================================================================
// ExecuteQueryWithParams Tests - Parameterized Queries
// ============================================================================

func TestQueryExecutor_ExecuteQueryWithParams_Simple(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := "SELECT $1::integer as num, $2::text as greeting"
	params := []any{42, "hello"}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams failed: %v", err)
	}

	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(result.Columns))
	}
	if result.Columns[0].Name != "num" {
		t.Errorf("expected first column 'num', got %q", result.Columns[0].Name)
	}
	if result.Columns[1].Name != "greeting" {
		t.Errorf("expected second column 'greeting', got %q", result.Columns[1].Name)
	}

	if result.RowCount != 1 {
		t.Errorf("expected 1 row, got %d", result.RowCount)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row in Rows slice, got %d", len(result.Rows))
	}

	row := result.Rows[0]
	if row["greeting"] != "hello" {
		t.Errorf("expected greeting 'hello', got %v", row["greeting"])
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_WhereClause(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Query events table with parameterized WHERE clause using id (guaranteed to exist)
	sql := "SELECT * FROM events WHERE id > $1 LIMIT 5"
	params := []any{0}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with WHERE failed: %v", err)
	}

	// Should get some results
	if len(result.Columns) == 0 {
		t.Error("expected at least one column")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_MultipleParams(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := `
		SELECT *
		FROM events
		WHERE id > $1
		  AND created_at >= $2
		LIMIT 10
	`
	params := []any{0, "2020-01-01"}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with multiple params failed: %v", err)
	}

	if len(result.Columns) == 0 {
		t.Error("expected at least one column")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_NumericTypes(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := `
		SELECT
			$1::integer as int_val,
			$2::bigint as bigint_val,
			$3::numeric as numeric_val,
			$4::float as float_val
	`
	params := []any{42, int64(9999999999), 123.45, 67.89}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with numeric types failed: %v", err)
	}

	if result.RowCount != 1 {
		t.Fatalf("expected 1 row, got %d", result.RowCount)
	}

	row := result.Rows[0]
	if row["int_val"] == nil {
		t.Error("expected int_val to be non-nil")
	}
	if row["bigint_val"] == nil {
		t.Error("expected bigint_val to be non-nil")
	}
	if row["numeric_val"] == nil {
		t.Error("expected numeric_val to be non-nil")
	}
	if row["float_val"] == nil {
		t.Error("expected float_val to be non-nil")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_BooleanType(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := "SELECT $1::boolean as bool_true, $2::boolean as bool_false"
	params := []any{true, false}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with boolean failed: %v", err)
	}

	row := result.Rows[0]
	if row["bool_true"] != true {
		t.Errorf("expected bool_true to be true, got %v", row["bool_true"])
	}
	if row["bool_false"] != false {
		t.Errorf("expected bool_false to be false, got %v", row["bool_false"])
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_DateTypes(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := `
		SELECT
			$1::date as date_val,
			$2::timestamp as timestamp_val,
			$3::timestamptz as timestamptz_val
	`
	params := []any{"2024-01-15", "2024-01-15 10:30:00", "2024-01-15T10:30:00Z"}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with date types failed: %v", err)
	}

	row := result.Rows[0]
	if row["date_val"] == nil {
		t.Error("expected date_val to be non-nil")
	}
	if row["timestamp_val"] == nil {
		t.Error("expected timestamp_val to be non-nil")
	}
	if row["timestamptz_val"] == nil {
		t.Error("expected timestamptz_val to be non-nil")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_UUIDType(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	testUUID := "550e8400-e29b-41d4-a716-446655440000"
	sql := "SELECT $1::uuid as uuid_val"
	params := []any{testUUID}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with UUID failed: %v", err)
	}

	row := result.Rows[0]
	if row["uuid_val"] == nil {
		t.Error("expected uuid_val to be non-nil")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_ArrayTypes(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := `
		SELECT
			$1::text[] as text_array,
			$2::integer[] as int_array
	`
	params := []any{[]string{"one", "two", "three"}, []int64{1, 2, 3}}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with arrays failed: %v", err)
	}

	row := result.Rows[0]
	if row["text_array"] == nil {
		t.Error("expected text_array to be non-nil")
	}
	if row["int_array"] == nil {
		t.Error("expected int_array to be non-nil")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_INClause(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := "SELECT * FROM events WHERE id = ANY($1::integer[]) LIMIT 10"
	params := []any{[]int64{1, 2, 3}}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with IN clause failed: %v", err)
	}

	// Verify we got results (if ids 1,2,3 exist in test data)
	if len(result.Columns) == 0 {
		t.Error("expected at least one column")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_NullValue(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := "SELECT $1::text as null_val, $2::text as non_null_val"
	params := []any{nil, "not null"}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with null failed: %v", err)
	}

	row := result.Rows[0]
	if row["null_val"] != nil {
		t.Errorf("expected null_val to be nil, got %v", row["null_val"])
	}
	if row["non_null_val"] != "not null" {
		t.Errorf("expected non_null_val 'not null', got %v", row["non_null_val"])
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_WithLimit(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// First check we have enough rows
	checkSQL := "SELECT COUNT(*) as cnt FROM events WHERE id > $1"
	checkResult, err := tc.executor.QueryWithParams(ctx, checkSQL, []any{0}, 0)
	if err != nil {
		t.Fatalf("check query failed: %v", err)
	}

	// Skip if not enough rows
	if len(checkResult.Rows) == 0 {
		t.Skip("no rows to test limit")
	}

	// Now test with limit
	sql := "SELECT * FROM events WHERE id > $1"
	params := []any{0}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 3)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with limit failed: %v", err)
	}

	if result.RowCount > 3 {
		t.Errorf("expected at most 3 rows with limit, got %d", result.RowCount)
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_SameParamMultipleTimes(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Use same parameter twice in query
	sql := "SELECT $1::text as first, $1::text as second, $2::integer as num"
	params := []any{"repeated", 42}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with repeated param failed: %v", err)
	}

	row := result.Rows[0]
	if row["first"] != "repeated" {
		t.Errorf("expected first 'repeated', got %v", row["first"])
	}
	if row["second"] != "repeated" {
		t.Errorf("expected second 'repeated', got %v", row["second"])
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_NoResults(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := "SELECT * FROM events WHERE id = $1 AND 1=0"
	params := []any{99999}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams failed: %v", err)
	}

	if result.RowCount != 0 {
		t.Errorf("expected 0 rows, got %d", result.RowCount)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected empty Rows slice, got %d", len(result.Rows))
	}
	// Columns should still be populated
	if len(result.Columns) == 0 {
		t.Error("expected columns even with no results")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_WrongParamCount(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// SQL expects 2 params but we provide 1
	sql := "SELECT $1::integer as first, $2::integer as second"
	params := []any{42}

	_, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err == nil {
		t.Fatal("expected error when param count doesn't match")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_InvalidType(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Try to pass string where integer expected
	sql := "SELECT $1::integer as num"
	params := []any{"not a number"}

	_, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err == nil {
		t.Fatal("expected error for type mismatch")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_SQLInjectionPrevention(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// This should be treated as a literal integer value, not SQL injection
	// If parameterization is broken, this would cause a syntax error
	maliciousInput := "1; DROP TABLE events; --"

	sql := "SELECT * FROM events WHERE id::text = $1 LIMIT 1"
	params := []any{maliciousInput}

	// This should NOT drop the table - it should just find no results
	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams failed: %v", err)
	}

	// Should return 0 results (no id matches the malicious string as text)
	if result.RowCount != 0 {
		t.Logf("note: got %d rows, which is fine - means the string matched some data", result.RowCount)
	}

	// Verify the events table still exists by querying it
	verifyResult, err := tc.executor.Query(ctx, "SELECT COUNT(*) as cnt FROM events", 0)
	if err != nil {
		t.Fatalf("SECURITY FAILURE: events table was affected by injection attempt: %v", err)
	}
	if verifyResult.RowCount != 1 {
		t.Error("SECURITY FAILURE: events table appears to be missing or corrupted")
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_ComplexQuery(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	sql := `
		SELECT
			id,
			COUNT(*) as event_count,
			MIN(created_at) as earliest,
			MAX(created_at) as latest
		FROM events
		WHERE id > $1
		  AND created_at >= $2
		GROUP BY id
		HAVING COUNT(*) > $3
		ORDER BY event_count DESC
		LIMIT 10
	`
	params := []any{0, "2020-01-01", 0}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err != nil {
		t.Fatalf("ExecuteQueryWithParams with complex query failed: %v", err)
	}

	if len(result.Columns) != 4 {
		t.Errorf("expected 4 columns, got %d", len(result.Columns))
	}
}

func TestQueryExecutor_ExecuteQueryWithParams_ContextCancellation(t *testing.T) {
	tc := setupQueryExecutorTest(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	sql := "SELECT pg_sleep(10), $1::integer"
	params := []any{42}

	_, err := tc.executor.QueryWithParams(ctx, sql, params, 0)
	if err == nil {
		t.Error("expected error when context is cancelled")
	}
}

// ============================================================================
// ExplainQuery Tests
// ============================================================================

func TestQueryExecutor_ExplainQuery_Valid(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	result, err := tc.executor.ExplainQuery(ctx, "SELECT * FROM events WHERE id < 100")
	if err != nil {
		t.Fatalf("expected valid query to succeed, got error: %v", err)
	}

	if result.Plan == "" {
		t.Error("expected non-empty execution plan")
	}

	// Execution time should be non-negative (0 is ok for very fast queries)
	if result.ExecutionTimeMs < 0 {
		t.Errorf("expected non-negative execution time, got %.2f", result.ExecutionTimeMs)
	}

	// Planning time should be non-negative
	if result.PlanningTimeMs < 0 {
		t.Errorf("expected non-negative planning time, got %.2f", result.PlanningTimeMs)
	}

	// Should have at least one hint
	if len(result.PerformanceHints) == 0 {
		t.Error("expected at least one performance hint")
	}
}

func TestQueryExecutor_ExplainQuery_Complex(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Test with a more complex query
	sql := `
		SELECT e.id, COUNT(*) as count
		FROM events e
		WHERE e.created_at > NOW() - INTERVAL '1 day'
		GROUP BY e.id
		ORDER BY count DESC
		LIMIT 10
	`

	result, err := tc.executor.ExplainQuery(ctx, sql)
	if err != nil {
		t.Fatalf("expected complex query to succeed, got error: %v", err)
	}

	if result.Plan == "" {
		t.Error("expected non-empty execution plan")
	}

	// Execution and planning times should be non-negative (can be 0 for very fast queries)
	if result.ExecutionTimeMs < 0 {
		t.Errorf("expected non-negative execution time, got %.2f", result.ExecutionTimeMs)
	}
	if result.PlanningTimeMs < 0 {
		t.Errorf("expected non-negative planning time, got %.2f", result.PlanningTimeMs)
	}

	// Should have hints
	if len(result.PerformanceHints) == 0 {
		t.Error("expected at least one performance hint")
	}
}

func TestQueryExecutor_ExplainQuery_InvalidSQL(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.ExplainQuery(ctx, "SELEC * FORM events")
	if err == nil {
		t.Error("expected invalid SQL to fail")
	}

	if !strings.Contains(err.Error(), "EXPLAIN ANALYZE failed") {
		t.Errorf("expected error to mention EXPLAIN ANALYZE failure, got: %v", err)
	}
}

func TestQueryExecutor_ExplainQuery_NonExistentTable(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.ExplainQuery(ctx, "SELECT * FROM nonexistent_table_xyz")
	if err == nil {
		t.Error("expected non-existent table to fail")
	}
}

func TestQueryExecutor_ExplainQuery_WithJoin(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Self-join to test join plan analysis
	sql := `
		SELECT e1.id, e2.id
		FROM events e1
		JOIN events e2 ON e1.id = e2.id
		WHERE e1.id < 10
	`

	result, err := tc.executor.ExplainQuery(ctx, sql)
	if err != nil {
		t.Fatalf("expected join query to succeed, got error: %v", err)
	}

	if result.Plan == "" {
		t.Error("expected non-empty execution plan")
	}

	// Should have timing information
	if result.ExecutionTimeMs < 0 {
		t.Errorf("expected non-negative execution time, got %.2f", result.ExecutionTimeMs)
	}

	// Should have hints
	if len(result.PerformanceHints) == 0 {
		t.Error("expected at least one performance hint")
	}
}

func TestQueryExecutor_ExplainQuery_PerformanceHints(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Query that will likely trigger a sequential scan
	result, err := tc.executor.ExplainQuery(ctx, "SELECT * FROM events")
	if err != nil {
		t.Fatalf("expected query to succeed, got error: %v", err)
	}

	// Should have hints
	if len(result.PerformanceHints) == 0 {
		t.Error("expected performance hints to be generated")
	}

	// Verify hints are non-empty strings
	for i, hint := range result.PerformanceHints {
		if hint == "" {
			t.Errorf("hint %d is empty", i)
		}
	}
}

// ============================================================================
// Modifying Statement Tests (INSERT/UPDATE/DELETE with parameters)
// These tests verify that QueryWithParams and Query correctly handle
// data-modifying statements, which cannot be wrapped in SELECT * FROM (...).
// ============================================================================

func TestQueryExecutor_QueryWithParams_INSERT_WithReturning(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Create a test table for this test
	_, err := tc.executor.pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_insert_params;
		CREATE TABLE test_insert_params (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			value INTEGER DEFAULT 0,
			created_at TIMESTAMP DEFAULT now()
		);
	`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		_, _ = tc.executor.pool.Exec(ctx, "DROP TABLE IF EXISTS test_insert_params")
	}()

	// INSERT with parameters and RETURNING clause
	sql := "INSERT INTO test_insert_params (name, value) VALUES ($1, $2) RETURNING id, name, value"
	params := []any{"test-name", 42}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 10)
	if err != nil {
		t.Fatalf("INSERT with RETURNING should succeed, got error: %v", err)
	}

	// Should return exactly one row
	if result.RowCount != 1 {
		t.Errorf("expected 1 row returned, got %d", result.RowCount)
	}

	// Should have 3 columns: id, name, value
	if len(result.Columns) != 3 {
		t.Errorf("expected 3 columns, got %d", len(result.Columns))
	}

	// Verify returned data
	if len(result.Rows) > 0 {
		row := result.Rows[0]
		if name, ok := row["name"].(string); !ok || name != "test-name" {
			t.Errorf("expected name='test-name', got %v", row["name"])
		}
	}

	// Verify data was actually inserted
	var count int
	err = tc.executor.pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_insert_params").Scan(&count)
	if err != nil {
		t.Fatalf("failed to verify insert: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row in table, got %d", count)
	}
}

func TestQueryExecutor_QueryWithParams_UPDATE_WithReturning(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Create and populate test table
	_, err := tc.executor.pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_update_params;
		CREATE TABLE test_update_params (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			value INTEGER DEFAULT 0
		);
		INSERT INTO test_update_params (name, value) VALUES ('original', 10), ('other', 20);
	`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		_, _ = tc.executor.pool.Exec(ctx, "DROP TABLE IF EXISTS test_update_params")
	}()

	// UPDATE with parameters and RETURNING clause
	sql := "UPDATE test_update_params SET value = $1 WHERE name = $2 RETURNING name, value"
	params := []any{99, "original"}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 10)
	if err != nil {
		t.Fatalf("UPDATE with RETURNING should succeed, got error: %v", err)
	}

	// Should return exactly one row (the updated row)
	if result.RowCount != 1 {
		t.Errorf("expected 1 row returned, got %d", result.RowCount)
	}

	// Verify returned data shows updated value
	if len(result.Rows) > 0 {
		row := result.Rows[0]
		// PostgreSQL INTEGER returns as int32
		var valueMatches bool
		switch v := row["value"].(type) {
		case int32:
			valueMatches = v == 99
		case int64:
			valueMatches = v == 99
		}
		if !valueMatches {
			t.Errorf("expected value=99, got %v (type: %T)", row["value"], row["value"])
		}
	}

	// Verify data was actually updated
	var newValue int
	err = tc.executor.pool.QueryRow(ctx, "SELECT value FROM test_update_params WHERE name = 'original'").Scan(&newValue)
	if err != nil {
		t.Fatalf("failed to verify update: %v", err)
	}
	if newValue != 99 {
		t.Errorf("expected value=99, got %d", newValue)
	}

	// Verify other row was not affected
	var otherValue int
	err = tc.executor.pool.QueryRow(ctx, "SELECT value FROM test_update_params WHERE name = 'other'").Scan(&otherValue)
	if err != nil {
		t.Fatalf("failed to verify other row: %v", err)
	}
	if otherValue != 20 {
		t.Errorf("expected other value=20, got %d", otherValue)
	}
}

func TestQueryExecutor_QueryWithParams_DELETE_WithReturning(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Create and populate test table
	_, err := tc.executor.pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_delete_params;
		CREATE TABLE test_delete_params (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		INSERT INTO test_delete_params (name) VALUES ('to-delete'), ('keep');
	`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		_, _ = tc.executor.pool.Exec(ctx, "DROP TABLE IF EXISTS test_delete_params")
	}()

	// DELETE with parameters and RETURNING clause
	sql := "DELETE FROM test_delete_params WHERE name = $1 RETURNING id, name"
	params := []any{"to-delete"}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 10)
	if err != nil {
		t.Fatalf("DELETE with RETURNING should succeed, got error: %v", err)
	}

	// Should return exactly one row (the deleted row)
	if result.RowCount != 1 {
		t.Errorf("expected 1 row returned, got %d", result.RowCount)
	}

	// Verify returned data
	if len(result.Rows) > 0 {
		row := result.Rows[0]
		if name, ok := row["name"].(string); !ok || name != "to-delete" {
			t.Errorf("expected name='to-delete', got %v", row["name"])
		}
	}

	// Verify data was actually deleted
	var count int
	err = tc.executor.pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_delete_params").Scan(&count)
	if err != nil {
		t.Fatalf("failed to verify delete: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row remaining, got %d", count)
	}
}

func TestQueryExecutor_QueryWithParams_INSERT_WithoutReturning(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Create test table
	_, err := tc.executor.pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_insert_no_return;
		CREATE TABLE test_insert_no_return (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		_, _ = tc.executor.pool.Exec(ctx, "DROP TABLE IF EXISTS test_insert_no_return")
	}()

	// INSERT without RETURNING clause - should still work but return empty result
	sql := "INSERT INTO test_insert_no_return (name) VALUES ($1)"
	params := []any{"test-name"}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 10)
	if err != nil {
		t.Fatalf("INSERT without RETURNING should succeed, got error: %v", err)
	}

	// Should return zero rows (no RETURNING clause)
	if result.RowCount != 0 {
		t.Errorf("expected 0 rows returned (no RETURNING), got %d", result.RowCount)
	}

	// Verify data was actually inserted
	var count int
	err = tc.executor.pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_insert_no_return").Scan(&count)
	if err != nil {
		t.Fatalf("failed to verify insert: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row in table, got %d", count)
	}
}

func TestQueryExecutor_Query_INSERT_WithReturning(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Create test table
	_, err := tc.executor.pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_insert_query;
		CREATE TABLE test_insert_query (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		_, _ = tc.executor.pool.Exec(ctx, "DROP TABLE IF EXISTS test_insert_query")
	}()

	// INSERT with RETURNING (no parameters) via Query method
	sql := "INSERT INTO test_insert_query (name) VALUES ('hardcoded-name') RETURNING id, name"

	result, err := tc.executor.Query(ctx, sql, 10)
	if err != nil {
		t.Fatalf("INSERT with RETURNING via Query should succeed, got error: %v", err)
	}

	// Should return exactly one row
	if result.RowCount != 1 {
		t.Errorf("expected 1 row returned, got %d", result.RowCount)
	}

	// Verify data was actually inserted
	var count int
	err = tc.executor.pool.QueryRow(ctx, "SELECT COUNT(*) FROM test_insert_query").Scan(&count)
	if err != nil {
		t.Fatalf("failed to verify insert: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 row in table, got %d", count)
	}
}

func TestQueryExecutor_QueryWithParams_INSERT_UUID_Generation(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Create test table with UUID primary key
	_, err := tc.executor.pool.Exec(ctx, `
		DROP TABLE IF EXISTS test_insert_uuid;
		CREATE TABLE test_insert_uuid (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			name TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT now()
		);
	`)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}
	defer func() {
		_, _ = tc.executor.pool.Exec(ctx, "DROP TABLE IF EXISTS test_insert_uuid")
	}()

	// INSERT letting the database generate the UUID
	sql := "INSERT INTO test_insert_uuid (name) VALUES ($1) RETURNING id, name, created_at"
	params := []any{"uuid-test"}

	result, err := tc.executor.QueryWithParams(ctx, sql, params, 10)
	if err != nil {
		t.Fatalf("INSERT with UUID generation should succeed, got error: %v", err)
	}

	// Should return exactly one row
	if result.RowCount != 1 {
		t.Errorf("expected 1 row returned, got %d", result.RowCount)
	}

	// Verify UUID was generated
	if len(result.Rows) > 0 {
		row := result.Rows[0]
		id := row["id"]
		if id == nil {
			t.Error("expected id to be generated, got nil")
		}
		// UUID should be parseable
		switch v := id.(type) {
		case [16]byte:
			// pgx returns UUID as [16]byte
			parsedUUID := uuid.UUID(v)
			if parsedUUID == uuid.Nil {
				t.Error("expected non-nil UUID")
			}
		case string:
			if _, err := uuid.Parse(v); err != nil {
				t.Errorf("expected valid UUID string, got %v: %v", v, err)
			}
		default:
			t.Errorf("unexpected UUID type: %T", id)
		}
	}
}

// ============================================================================
// Multi-Statement Execution Tests
// ============================================================================

func TestQueryExecutor_Execute_MultiStatement_DDL(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_multi_b")
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_multi_a")
	_, _ = tc.executor.Execute(ctx, "DROP TYPE IF EXISTS test_multi_enum")

	// Execute multiple DDL statements in one call
	result, err := tc.executor.Execute(ctx, `
		CREATE TYPE test_multi_enum AS ENUM ('x', 'y', 'z');
		CREATE TABLE test_multi_a (id SERIAL PRIMARY KEY, status test_multi_enum NOT NULL);
		CREATE TABLE test_multi_b (id SERIAL PRIMARY KEY, a_id INT REFERENCES test_multi_a(id))
	`)
	if err != nil {
		t.Fatalf("multi-statement DDL failed: %v", err)
	}

	// DDL doesn't affect rows
	if result.RowsAffected != 0 {
		t.Errorf("expected 0 rows affected for DDL batch, got %d", result.RowsAffected)
	}

	// Verify all objects were created by using them
	insertResult, err := tc.executor.Execute(ctx, "INSERT INTO test_multi_a (status) VALUES ('x')")
	if err != nil {
		t.Fatalf("insert into table created by batch failed: %v", err)
	}
	if insertResult.RowsAffected != 1 {
		t.Errorf("expected 1 row affected, got %d", insertResult.RowsAffected)
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_multi_b")
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_multi_a")
	_, _ = tc.executor.Execute(ctx, "DROP TYPE IF EXISTS test_multi_enum")
}

func TestQueryExecutor_Execute_MultiStatement_Rollback(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_rollback_table")

	// Create a table first (single statement)
	_, err := tc.executor.Execute(ctx, "CREATE TABLE test_rollback_table (id SERIAL PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("setup CREATE TABLE failed: %v", err)
	}

	// Execute a batch where the second statement fails — should rollback the first
	_, err = tc.executor.Execute(ctx, `
		INSERT INTO test_rollback_table (value) VALUES ('should_be_rolled_back');
		INSERT INTO nonexistent_table_xyz (col) VALUES ('fail')
	`)
	if err == nil {
		t.Fatal("expected error from multi-statement batch with invalid table")
	}

	// Verify the first INSERT was rolled back
	queryResult, err := tc.executor.Query(ctx, "SELECT COUNT(*) as cnt FROM test_rollback_table", 0)
	if err != nil {
		t.Fatalf("query after rollback failed: %v", err)
	}
	count := queryResult.Rows[0]["cnt"]
	if count != int64(0) {
		t.Errorf("expected 0 rows after rollback, got %v", count)
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_rollback_table")
}

func TestQueryExecutor_Execute_MultiStatement_DML(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Setup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_multi_dml")
	_, err := tc.executor.Execute(ctx, "CREATE TABLE test_multi_dml (id SERIAL PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Execute multiple DML statements
	_, err = tc.executor.Execute(ctx, `
		INSERT INTO test_multi_dml (value) VALUES ('one');
		INSERT INTO test_multi_dml (value) VALUES ('two');
		INSERT INTO test_multi_dml (value) VALUES ('three')
	`)
	if err != nil {
		t.Fatalf("multi-statement DML failed: %v", err)
	}

	// Verify all inserts succeeded
	queryResult, err := tc.executor.Query(ctx, "SELECT COUNT(*) as cnt FROM test_multi_dml", 0)
	if err != nil {
		t.Fatalf("verify query failed: %v", err)
	}
	count := queryResult.Rows[0]["cnt"]
	if count != int64(3) {
		t.Errorf("expected 3 rows, got %v", count)
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_multi_dml")
}

func TestQueryExecutor_Execute_SingleStatement_Unchanged(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Verify single statements still work through the original path (with RETURNING)
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_single_stmt")
	_, err := tc.executor.Execute(ctx, "CREATE TABLE test_single_stmt (id SERIAL PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Single statement with RETURNING should still return rows
	result, err := tc.executor.Execute(ctx, "INSERT INTO test_single_stmt (value) VALUES ('test') RETURNING id, value")
	if err != nil {
		t.Fatalf("single statement RETURNING failed: %v", err)
	}

	if result.RowCount != 1 {
		t.Errorf("expected 1 row returned, got %d", result.RowCount)
	}
	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(result.Columns))
	}
	if result.Rows[0]["value"] != "test" {
		t.Errorf("expected value 'test', got %v", result.Rows[0]["value"])
	}

	// Cleanup
	_, _ = tc.executor.Execute(ctx, "DROP TABLE IF EXISTS test_single_stmt")
}
