//go:build integration

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

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

	executor, err := NewQueryExecutor(ctx, cfg)
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

	result, err := tc.executor.ExecuteQuery(ctx, "SELECT 1 as num, 'hello' as greeting", 0)
	if err != nil {
		t.Fatalf("ExecuteQuery failed: %v", err)
	}

	if len(result.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(result.Columns))
	}
	if result.Columns[0] != "num" {
		t.Errorf("expected first column 'num', got %q", result.Columns[0])
	}
	if result.Columns[1] != "greeting" {
		t.Errorf("expected second column 'greeting', got %q", result.Columns[1])
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
	result, err := tc.executor.ExecuteQuery(ctx, "SELECT * FROM events LIMIT 5", 0)
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
	resultNoLimit, err := tc.executor.ExecuteQuery(ctx, "SELECT * FROM events", 0)
	if err != nil {
		t.Fatalf("ExecuteQuery without limit failed: %v", err)
	}

	if resultNoLimit.RowCount < 3 {
		t.Skipf("need at least 3 rows to test limit, got %d", resultNoLimit.RowCount)
	}

	// Now query with limit
	result, err := tc.executor.ExecuteQuery(ctx, "SELECT * FROM events", 2)
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

	result, err := tc.executor.ExecuteQuery(ctx, "SELECT * FROM events WHERE 1=0", 0)
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

	_, err := tc.executor.ExecuteQuery(ctx, "SELECT * FROM nonexistent_table_xyz", 0)
	if err == nil {
		t.Fatal("expected error for invalid SQL")
	}
}

func TestQueryExecutor_ExecuteQuery_SyntaxError(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	_, err := tc.executor.ExecuteQuery(ctx, "SELEC * FORM events", 0)
	if err == nil {
		t.Fatal("expected error for SQL syntax error")
	}
}

func TestQueryExecutor_ExecuteQuery_Join(t *testing.T) {
	tc := setupQueryExecutorTest(t)
	ctx := context.Background()

	// Test a simple self-join query using events table
	result, err := tc.executor.ExecuteQuery(ctx, `
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

	result, err := tc.executor.ExecuteQuery(ctx, `
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
	result, err := tc.executor.ExecuteQuery(ctx, `
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

	_, err := tc.executor.ExecuteQuery(ctx, "SELECT pg_sleep(10)", 0)
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
	_, err = tc.executor.ExecuteQuery(ctx, "SELECT * FROM test_drop_table", 0)
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
