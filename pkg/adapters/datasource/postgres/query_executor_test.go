//go:build integration

package postgres

import (
	"context"
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
