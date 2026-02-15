//go:build integration && (postgres || all_adapters)

package postgres

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// schemaDiscovererTestContext holds dependencies for schema discoverer tests.
type schemaDiscovererTestContext struct {
	t          *testing.T
	discoverer *SchemaDiscoverer
}

// setupSchemaDiscovererTest creates a SchemaDiscoverer connected to the test container.
func setupSchemaDiscovererTest(t *testing.T) *schemaDiscovererTestContext {
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

	// Parse port to int
	portInt := port.Int()

	cfg := &Config{
		Host:     host,
		Port:     portInt,
		User:     "ekaya",
		Password: "test_password",
		Database: "test_data",
		SSLMode:  "disable",
	}

	// Pass nil for connection manager, zero IDs, and nil logger for unmanaged pool (test mode)
	discoverer, err := NewSchemaDiscoverer(ctx, cfg, nil, uuid.Nil, uuid.Nil, "", nil)
	if err != nil {
		t.Fatalf("failed to create schema discoverer: %v", err)
	}

	t.Cleanup(func() {
		discoverer.Close()
	})

	return &schemaDiscovererTestContext{
		t:          t,
		discoverer: discoverer,
	}
}

func TestSchemaDiscoverer_DiscoverTables(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	tables, err := tc.discoverer.DiscoverTables(ctx)
	if err != nil {
		t.Fatalf("DiscoverTables failed: %v", err)
	}

	// Test database has 38 tables in public schema
	if len(tables) < 30 {
		t.Errorf("expected at least 30 tables, got %d", len(tables))
	}

	// Verify known tables exist
	foundEvents := false
	foundAccounts := false
	foundUsers := false

	for _, table := range tables {
		switch table.TableName {
		case "events":
			foundEvents = true
			if table.SchemaName != "public" {
				t.Errorf("events table: expected schema 'public', got %q", table.SchemaName)
			}
		case "accounts":
			foundAccounts = true
		case "users":
			foundUsers = true
		}
	}

	if !foundEvents {
		t.Error("expected to find 'events' table")
	}
	if !foundAccounts {
		t.Error("expected to find 'accounts' table")
	}
	if !foundUsers {
		t.Error("expected to find 'users' table")
	}
}

func TestSchemaDiscoverer_DiscoverTables_ExcludesSystemSchemas(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	tables, err := tc.discoverer.DiscoverTables(ctx)
	if err != nil {
		t.Fatalf("DiscoverTables failed: %v", err)
	}

	// Verify no system schema tables are included
	for _, table := range tables {
		switch table.SchemaName {
		case "pg_catalog", "information_schema", "pg_toast":
			t.Errorf("system schema table found: %s.%s", table.SchemaName, table.TableName)
		}
	}
}

func TestSchemaDiscoverer_DiscoverColumns(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Discover columns for the events table
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "events")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	if len(columns) == 0 {
		t.Fatal("expected at least one column in events table")
	}

	// Verify columns have required fields populated
	for _, col := range columns {
		if col.ColumnName == "" {
			t.Error("column has empty name")
		}
		if col.DataType == "" {
			t.Error("column has empty data type")
		}
		if col.OrdinalPosition < 1 {
			t.Errorf("column %s has invalid ordinal position: %d", col.ColumnName, col.OrdinalPosition)
		}
	}

	// Verify ordinal positions are sequential
	for i, col := range columns {
		if col.OrdinalPosition != i+1 {
			t.Errorf("column %s: expected ordinal position %d, got %d", col.ColumnName, i+1, col.OrdinalPosition)
		}
	}
}

func TestSchemaDiscoverer_DiscoverColumns_DetectsPrimaryKey(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Discover columns - most tables have an 'id' primary key
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "accounts")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	// Find any primary key column
	foundPK := false
	for _, col := range columns {
		if col.IsPrimaryKey {
			foundPK = true
			break
		}
	}

	if !foundPK {
		t.Error("expected to find at least one primary key column in accounts table")
	}
}

func TestSchemaDiscoverer_DiscoverColumns_NonexistentTable(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "nonexistent_table_xyz")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	// Should return empty slice, not error
	if len(columns) != 0 {
		t.Errorf("expected 0 columns for nonexistent table, got %d", len(columns))
	}
}

func TestSchemaDiscoverer_DiscoverColumns_DetectsUniqueConstraint(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// The events table has a UNIQUE constraint on ev_id
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "events")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	// Find the ev_id column and verify it's marked as unique
	var evIDColumn *struct {
		name     string
		isUnique bool
	}
	for _, col := range columns {
		if col.ColumnName == "ev_id" {
			evIDColumn = &struct {
				name     string
				isUnique bool
			}{col.ColumnName, col.IsUnique}
			break
		}
	}

	if evIDColumn == nil {
		t.Fatal("expected to find ev_id column in events table")
	}
	if !evIDColumn.isUnique {
		t.Error("expected ev_id column to have is_unique=true")
	}
}

func TestSchemaDiscoverer_DiscoverColumns_DetectsDefaultValue(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// The accounts table has columns with default values
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "accounts")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	// Find the id column - should have nextval default
	var idDefault *string
	var isBannedDefault *string
	for _, col := range columns {
		if col.ColumnName == "id" {
			idDefault = col.DefaultValue
		}
		if col.ColumnName == "is_banned" {
			isBannedDefault = col.DefaultValue
		}
	}

	// id column should have a sequence default
	if idDefault == nil {
		t.Error("expected id column to have a default value")
	} else if *idDefault != "nextval('accounts_id_seq'::regclass)" {
		t.Errorf("expected id default to be nextval sequence, got: %s", *idDefault)
	}

	// is_banned column should have boolean default
	if isBannedDefault == nil {
		t.Error("expected is_banned column to have a default value")
	} else if *isBannedDefault != "false" {
		t.Errorf("expected is_banned default to be 'false', got: %s", *isBannedDefault)
	}
}

func TestSchemaDiscoverer_DiscoverColumns_ExcludesMultiColumnUnique(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Primary keys are often multi-column in junction tables, and we exclude
	// multi-column unique constraints. Verify that regular columns in tables
	// with composite keys don't incorrectly show as unique.

	// The accounts table id is a PK (single column) - should NOT be marked unique
	// because PKs are tracked separately from unique constraints
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "accounts")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	for _, col := range columns {
		if col.ColumnName == "id" {
			// id is a PK, not a separate UNIQUE constraint
			if col.IsUnique {
				t.Error("expected id column (PK) to have is_unique=false since it's a PK, not a UNIQUE constraint")
			}
			if !col.IsPrimaryKey {
				t.Error("expected id column to be marked as primary key")
			}
			break
		}
	}
}

func TestSchemaDiscoverer_DiscoverForeignKeys(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	fks, err := tc.discoverer.DiscoverForeignKeys(ctx)
	if err != nil {
		t.Fatalf("DiscoverForeignKeys failed: %v", err)
	}

	// Test database should have foreign keys
	if len(fks) == 0 {
		t.Skip("no foreign keys found in test database - skipping FK verification")
	}

	// Verify FK structure is populated
	for _, fk := range fks {
		if fk.ConstraintName == "" {
			t.Error("FK has empty constraint name")
		}
		if fk.SourceSchema == "" || fk.SourceTable == "" || fk.SourceColumn == "" {
			t.Errorf("FK %s has empty source fields", fk.ConstraintName)
		}
		if fk.TargetSchema == "" || fk.TargetTable == "" || fk.TargetColumn == "" {
			t.Errorf("FK %s has empty target fields", fk.ConstraintName)
		}
	}
}

func TestSchemaDiscoverer_SupportsForeignKeys(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)

	if !tc.discoverer.SupportsForeignKeys() {
		t.Error("PostgreSQL should support foreign keys")
	}
}

func TestSchemaDiscoverer_AnalyzeColumnStats(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// First discover columns to get valid column names
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "events")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	if len(columns) == 0 {
		t.Fatal("no columns found in events table")
	}

	// Analyze first column
	columnNames := []string{columns[0].ColumnName}
	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "public", "events", columnNames)
	if err != nil {
		t.Fatalf("AnalyzeColumnStats failed: %v", err)
	}

	if len(stats) != 1 {
		t.Fatalf("expected 1 stat result, got %d", len(stats))
	}

	stat := stats[0]
	if stat.ColumnName != columnNames[0] {
		t.Errorf("expected column name %q, got %q", columnNames[0], stat.ColumnName)
	}

	// events table has 100 rows
	if stat.RowCount != 100 {
		t.Errorf("expected row count 100, got %d", stat.RowCount)
	}

	// Distinct count should be <= row count
	if stat.DistinctCount > stat.RowCount {
		t.Errorf("distinct count %d exceeds row count %d", stat.DistinctCount, stat.RowCount)
	}

	// Non-null count should be <= row count
	if stat.NonNullCount > stat.RowCount {
		t.Errorf("non-null count %d exceeds row count %d", stat.NonNullCount, stat.RowCount)
	}
}

func TestSchemaDiscoverer_AnalyzeColumnStats_MultipleColumns(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Discover columns
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "accounts")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	if len(columns) < 2 {
		t.Skip("need at least 2 columns to test multiple column stats")
	}

	// Analyze first two columns
	columnNames := []string{columns[0].ColumnName, columns[1].ColumnName}
	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "public", "accounts", columnNames)
	if err != nil {
		t.Fatalf("AnalyzeColumnStats failed: %v", err)
	}

	if len(stats) != 2 {
		t.Errorf("expected 2 stat results, got %d", len(stats))
	}
}

func TestSchemaDiscoverer_AnalyzeColumnStats_EmptyList(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "public", "events", []string{})
	if err != nil {
		t.Fatalf("AnalyzeColumnStats with empty list failed: %v", err)
	}

	if stats != nil && len(stats) != 0 {
		t.Errorf("expected nil or empty slice for empty column list, got %d", len(stats))
	}
}

func TestSchemaDiscoverer_AnalyzeColumnStats_PartialFailure(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Create a temporary table with valid columns
	setupSQL := `
		CREATE TEMP TABLE test_partial_failure (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL
		);
		INSERT INTO test_partial_failure (name) VALUES ('alice'), ('bob'), ('charlie');
	`

	_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	// Request stats for columns including a nonexistent one.
	// This simulates the scenario where a column might fail due to permissions, type issues, etc.
	// The function should:
	// 1. Try the main query (with pg_typeof/length detection) - will fail for nonexistent column
	// 2. Retry with simplified query (just COUNT stats) - will also fail for nonexistent column
	// 3. Continue processing remaining columns after both queries fail
	columnNames := []string{"id", "nonexistent_column", "name"}
	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "pg_temp", "test_partial_failure", columnNames)

	// Should NOT return an error - partial failures are handled gracefully
	if err != nil {
		t.Fatalf("AnalyzeColumnStats should handle partial failures, got error: %v", err)
	}

	// Should return stats for all requested columns
	if len(stats) != 3 {
		t.Fatalf("expected 3 stat results, got %d", len(stats))
	}

	// Verify column names are preserved in order
	if stats[0].ColumnName != "id" {
		t.Errorf("expected first column to be 'id', got %q", stats[0].ColumnName)
	}
	if stats[1].ColumnName != "nonexistent_column" {
		t.Errorf("expected second column to be 'nonexistent_column', got %q", stats[1].ColumnName)
	}
	if stats[2].ColumnName != "name" {
		t.Errorf("expected third column to be 'name', got %q", stats[2].ColumnName)
	}

	// Verify id column has accurate stats (3 rows, 3 distinct)
	if stats[0].RowCount != 3 {
		t.Errorf("expected id row count 3, got %d", stats[0].RowCount)
	}
	if stats[0].DistinctCount != 3 {
		t.Errorf("expected id distinct count 3, got %d", stats[0].DistinctCount)
	}

	// Verify nonexistent_column has zero stats (failed to analyze)
	if stats[1].RowCount != 0 {
		t.Errorf("expected nonexistent_column row count 0, got %d", stats[1].RowCount)
	}
	if stats[1].DistinctCount != 0 {
		t.Errorf("expected nonexistent_column distinct count 0, got %d", stats[1].DistinctCount)
	}
	if stats[1].MinLength != nil {
		t.Errorf("expected nonexistent_column min_length nil, got %d", *stats[1].MinLength)
	}
	if stats[1].MaxLength != nil {
		t.Errorf("expected nonexistent_column max_length nil, got %d", *stats[1].MaxLength)
	}

	// Verify name column still has accurate stats (processed after the failure)
	if stats[2].RowCount != 3 {
		t.Errorf("expected name row count 3, got %d", stats[2].RowCount)
	}
	if stats[2].DistinctCount != 3 {
		t.Errorf("expected name distinct count 3, got %d", stats[2].DistinctCount)
	}
}

func TestSchemaDiscoverer_AnalyzeColumnStats_NonTextTypes(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Create a temporary table with non-text column types that would fail ::text casting.
	// Array columns cannot be cast to text with LENGTH() in PostgreSQL.
	setupSQL := `
		CREATE TEMP TABLE test_nonttext_types (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			tags TEXT[] NOT NULL,
			data BYTEA,
			num INTEGER NOT NULL
		);
		INSERT INTO test_nonttext_types (name, tags, data, num)
		VALUES
			('alice', ARRAY['a', 'b'], E'\\xDEADBEEF', 42),
			('bob', ARRAY['c'], E'\\xCAFE', 100);
	`

	_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	// Request stats for all columns including array and bytea types
	columnNames := []string{"id", "name", "tags", "data", "num"}
	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "pg_temp", "test_nonttext_types", columnNames)

	// Should NOT return an error - non-text types should be handled gracefully
	if err != nil {
		t.Fatalf("AnalyzeColumnStats should handle non-text types, got error: %v", err)
	}

	if len(stats) != 5 {
		t.Fatalf("expected 5 stat results, got %d", len(stats))
	}

	// Verify all columns have correct basic stats (row_count, distinct_count)
	for _, s := range stats {
		if s.RowCount != 2 {
			t.Errorf("column %s: expected row count 2, got %d", s.ColumnName, s.RowCount)
		}
		// All columns in our test data have 2 distinct values
		if s.DistinctCount != 2 {
			t.Errorf("column %s: expected distinct count 2, got %d", s.ColumnName, s.DistinctCount)
		}
	}

	// Verify text column has length stats
	nameStats := stats[1] // name column
	if nameStats.ColumnName != "name" {
		t.Fatalf("expected second column to be 'name', got %q", nameStats.ColumnName)
	}
	if nameStats.MinLength == nil {
		t.Error("expected text column 'name' to have min_length, got nil")
	} else if *nameStats.MinLength != 3 { // "bob" = 3
		t.Errorf("expected name min_length 3, got %d", *nameStats.MinLength)
	}
	if nameStats.MaxLength == nil {
		t.Error("expected text column 'name' to have max_length, got nil")
	} else if *nameStats.MaxLength != 5 { // "alice" = 5
		t.Errorf("expected name max_length 5, got %d", *nameStats.MaxLength)
	}

	// Verify array column has NULL length stats (not a type cast error)
	tagsStats := stats[2] // tags column (TEXT[])
	if tagsStats.ColumnName != "tags" {
		t.Fatalf("expected third column to be 'tags', got %q", tagsStats.ColumnName)
	}
	if tagsStats.MinLength != nil {
		t.Errorf("expected array column 'tags' to have nil min_length, got %d", *tagsStats.MinLength)
	}
	if tagsStats.MaxLength != nil {
		t.Errorf("expected array column 'tags' to have nil max_length, got %d", *tagsStats.MaxLength)
	}

	// Verify bytea column has NULL length stats
	dataStats := stats[3] // data column (BYTEA)
	if dataStats.ColumnName != "data" {
		t.Fatalf("expected fourth column to be 'data', got %q", dataStats.ColumnName)
	}
	if dataStats.MinLength != nil {
		t.Errorf("expected bytea column 'data' to have nil min_length, got %d", *dataStats.MinLength)
	}
	if dataStats.MaxLength != nil {
		t.Errorf("expected bytea column 'data' to have nil max_length, got %d", *dataStats.MaxLength)
	}

	// Verify integer column has NULL length stats
	numStats := stats[4] // num column (INTEGER)
	if numStats.ColumnName != "num" {
		t.Fatalf("expected fifth column to be 'num', got %q", numStats.ColumnName)
	}
	if numStats.MinLength != nil {
		t.Errorf("expected integer column 'num' to have nil min_length, got %d", *numStats.MinLength)
	}
	if numStats.MaxLength != nil {
		t.Errorf("expected integer column 'num' to have nil max_length, got %d", *numStats.MaxLength)
	}
}

func TestSchemaDiscoverer_AnalyzeColumnStats_RetryWithSimplifiedQuery(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// This test verifies the retry mechanism works.
	// We create a scenario where the main query works but a nonexistent column triggers retry.
	// Both valid and invalid columns are processed, verifying:
	// 1. Valid columns get full stats
	// 2. Invalid columns trigger retry (both queries fail) and get zero values
	// 3. Processing continues after failures

	setupSQL := `
		CREATE TEMP TABLE test_retry_behavior (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			count INTEGER NOT NULL
		);
		INSERT INTO test_retry_behavior (name, count) VALUES
			('alice', 10),
			('bob', 20),
			('charlie', 30);
	`

	_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
	if err != nil {
		t.Fatalf("failed to create test table: %v", err)
	}

	// Mix of valid columns and invalid columns to test retry behavior
	// The nonexistent column will fail both main and simplified queries
	columnNames := []string{"id", "invalid_col_1", "name", "invalid_col_2", "count"}
	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "pg_temp", "test_retry_behavior", columnNames)

	if err != nil {
		t.Fatalf("AnalyzeColumnStats should not return error, got: %v", err)
	}

	if len(stats) != 5 {
		t.Fatalf("expected 5 stat results, got %d", len(stats))
	}

	// Verify valid columns have correct stats
	// id column (index 0)
	if stats[0].ColumnName != "id" {
		t.Errorf("expected column 0 to be 'id', got %q", stats[0].ColumnName)
	}
	if stats[0].RowCount != 3 || stats[0].DistinctCount != 3 {
		t.Errorf("id column: expected row_count=3, distinct_count=3, got %d, %d",
			stats[0].RowCount, stats[0].DistinctCount)
	}

	// invalid_col_1 (index 1) - should have zero values after retry fails
	if stats[1].ColumnName != "invalid_col_1" {
		t.Errorf("expected column 1 to be 'invalid_col_1', got %q", stats[1].ColumnName)
	}
	if stats[1].RowCount != 0 || stats[1].DistinctCount != 0 {
		t.Errorf("invalid_col_1: expected zero values after retry, got row_count=%d, distinct_count=%d",
			stats[1].RowCount, stats[1].DistinctCount)
	}

	// name column (index 2) - should still have correct stats after previous failure
	if stats[2].ColumnName != "name" {
		t.Errorf("expected column 2 to be 'name', got %q", stats[2].ColumnName)
	}
	if stats[2].RowCount != 3 || stats[2].DistinctCount != 3 {
		t.Errorf("name column: expected row_count=3, distinct_count=3, got %d, %d",
			stats[2].RowCount, stats[2].DistinctCount)
	}
	// Text column should have length stats
	if stats[2].MinLength == nil || stats[2].MaxLength == nil {
		t.Error("name column should have length stats")
	}

	// invalid_col_2 (index 3) - should have zero values after retry fails
	if stats[3].ColumnName != "invalid_col_2" {
		t.Errorf("expected column 3 to be 'invalid_col_2', got %q", stats[3].ColumnName)
	}
	if stats[3].RowCount != 0 || stats[3].DistinctCount != 0 {
		t.Errorf("invalid_col_2: expected zero values after retry, got row_count=%d, distinct_count=%d",
			stats[3].RowCount, stats[3].DistinctCount)
	}

	// count column (index 4) - should still have correct stats after previous failures
	if stats[4].ColumnName != "count" {
		t.Errorf("expected column 4 to be 'count', got %q", stats[4].ColumnName)
	}
	if stats[4].RowCount != 3 || stats[4].DistinctCount != 3 {
		t.Errorf("count column: expected row_count=3, distinct_count=3, got %d, %d",
			stats[4].RowCount, stats[4].DistinctCount)
	}
}

func TestSchemaDiscoverer_CheckValueOverlap(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Check overlap between a column and itself (should be 100% match)
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "events")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	if len(columns) == 0 {
		t.Fatal("no columns found")
	}

	// Find a column that's likely to have values
	colName := columns[0].ColumnName

	result, err := tc.discoverer.CheckValueOverlap(ctx,
		"public", "events", colName,
		"public", "events", colName,
		1000)
	if err != nil {
		t.Fatalf("CheckValueOverlap failed: %v", err)
	}

	// Same column should have 100% overlap
	if result.MatchRate < 0.99 {
		t.Errorf("expected ~100%% match rate for same column, got %.2f%%", result.MatchRate*100)
	}

	if result.SourceDistinct != result.TargetDistinct {
		t.Errorf("source and target distinct should be equal for same column: %d vs %d",
			result.SourceDistinct, result.TargetDistinct)
	}

	if result.MatchedCount != result.SourceDistinct {
		t.Errorf("matched count should equal distinct count for same column: %d vs %d",
			result.MatchedCount, result.SourceDistinct)
	}
}

func TestSchemaDiscoverer_CheckValueOverlap_DifferentTables(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// This tests the mechanism works, not necessarily that there's overlap
	result, err := tc.discoverer.CheckValueOverlap(ctx,
		"public", "events", "id",
		"public", "accounts", "id",
		1000)
	if err != nil {
		t.Fatalf("CheckValueOverlap failed: %v", err)
	}

	// Just verify the result structure is valid
	if result.MatchRate < 0 || result.MatchRate > 1 {
		t.Errorf("match rate should be between 0 and 1, got %f", result.MatchRate)
	}
}

func TestSchemaDiscoverer_AnalyzeJoin(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Test join analysis between a column and itself (should join all rows)
	columns, err := tc.discoverer.DiscoverColumns(ctx, "public", "events")
	if err != nil {
		t.Fatalf("DiscoverColumns failed: %v", err)
	}

	if len(columns) == 0 {
		t.Fatal("no columns found")
	}

	colName := columns[0].ColumnName

	result, err := tc.discoverer.AnalyzeJoin(ctx,
		"public", "events", colName,
		"public", "events", colName)
	if err != nil {
		t.Fatalf("AnalyzeJoin failed: %v", err)
	}

	// Self-join should have zero orphans
	if result.OrphanCount != 0 {
		t.Errorf("expected 0 orphans for self-join, got %d", result.OrphanCount)
	}

	// Join count should be > 0
	if result.JoinCount == 0 {
		t.Error("expected non-zero join count for self-join")
	}
}

func TestSchemaDiscoverer_AnalyzeJoin_NoMatch(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Get columns from two different tables
	eventsColumns, err := tc.discoverer.DiscoverColumns(ctx, "public", "events")
	if err != nil {
		t.Fatalf("DiscoverColumns for events failed: %v", err)
	}

	accountsColumns, err := tc.discoverer.DiscoverColumns(ctx, "public", "accounts")
	if err != nil {
		t.Fatalf("DiscoverColumns for accounts failed: %v", err)
	}

	if len(eventsColumns) == 0 || len(accountsColumns) == 0 {
		t.Skip("need columns in both tables")
	}

	// Try to join on columns that likely don't match
	result, err := tc.discoverer.AnalyzeJoin(ctx,
		"public", "events", eventsColumns[0].ColumnName,
		"public", "accounts", accountsColumns[0].ColumnName)
	if err != nil {
		t.Fatalf("AnalyzeJoin failed: %v", err)
	}

	// Just verify the result structure is valid
	if result.JoinCount < 0 {
		t.Errorf("join count should be non-negative, got %d", result.JoinCount)
	}
	if result.OrphanCount < 0 {
		t.Errorf("orphan count should be non-negative, got %d", result.OrphanCount)
	}
}

func TestSchemaDiscoverer_AnalyzeJoin_CrossTypeComparison(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Create temporary tables with mismatched column types to test cross-type joins.
	// This simulates real-world scenarios where a text column (e.g., external_id)
	// might reference a bigint column (e.g., id) or vice versa.
	setupSQL := `
		CREATE TEMP TABLE test_text_col (id SERIAL PRIMARY KEY, ref_id TEXT);
		CREATE TEMP TABLE test_bigint_col (id BIGINT PRIMARY KEY);
		INSERT INTO test_text_col (ref_id) VALUES ('1'), ('2'), ('3');
		INSERT INTO test_bigint_col (id) VALUES (1), (2), (3);
	`

	// Get underlying pool to execute setup SQL
	_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
	if err != nil {
		t.Fatalf("failed to create test tables: %v", err)
	}

	// Attempt to join text column to bigint column.
	// The values match semantically ('1' = 1, '2' = 2, etc.) but types differ.
	result, err := tc.discoverer.AnalyzeJoin(ctx,
		"pg_temp", "test_text_col", "ref_id",
		"pg_temp", "test_bigint_col", "id")

	// Should succeed (not fail with type mismatch error)
	if err != nil {
		t.Fatalf("AnalyzeJoin should handle cross-type comparison (text vs bigint), got error: %v", err)
	}

	// Verify the join found matches (all 3 rows should match)
	if result.JoinCount != 3 {
		t.Errorf("expected 3 matched rows, got %d", result.JoinCount)
	}
	if result.OrphanCount != 0 {
		t.Errorf("expected 0 orphans, got %d", result.OrphanCount)
	}
	// Both tables have same values {1, 2, 3}, so reverse orphans should also be 0
	if result.ReverseOrphanCount != 0 {
		t.Errorf("expected 0 reverse orphans (tables have same values), got %d", result.ReverseOrphanCount)
	}
}

// TestSchemaDiscoverer_AnalyzeJoin_ReverseOrphans tests the bidirectional validation
// that catches false positive relationships like identity_provider → jobs.id.
// When source has few values that coincidentally exist in target (which has many more values),
// the reverse orphan count should be high, indicating this is not a real FK relationship.
func TestSchemaDiscoverer_AnalyzeJoin_ReverseOrphans(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Create temporary tables to simulate the identity_provider → jobs.id false positive:
	// - "small_lookup" has 3 values: {1, 2, 3} (like identity_provider)
	// - "large_table" has IDs 1-100 (like jobs.id)
	// Source→target: all 3 values from small_lookup exist in large_table → 0 orphans
	// Target→source: 97 values (4-100) don't exist in small_lookup → high reverse orphans
	setupSQL := `
		CREATE TEMP TABLE test_small_lookup (id INT PRIMARY KEY);
		CREATE TEMP TABLE test_large_table (id INT PRIMARY KEY);
		INSERT INTO test_small_lookup (id) VALUES (1), (2), (3);
		INSERT INTO test_large_table (id) SELECT generate_series(1, 100);
	`

	_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
	if err != nil {
		t.Fatalf("failed to create test tables: %v", err)
	}

	// Analyze join from small_lookup.id → large_table.id
	result, err := tc.discoverer.AnalyzeJoin(ctx,
		"pg_temp", "test_small_lookup", "id",
		"pg_temp", "test_large_table", "id")
	if err != nil {
		t.Fatalf("AnalyzeJoin failed: %v", err)
	}

	// Source→target: all 3 values exist in large_table → 0 orphans
	if result.OrphanCount != 0 {
		t.Errorf("expected 0 source orphans (all small values exist in large table), got %d", result.OrphanCount)
	}

	// Target→source: 97 values (4-100) don't exist in small_lookup
	expectedReverseOrphans := int64(97)
	if result.ReverseOrphanCount != expectedReverseOrphans {
		t.Errorf("expected %d reverse orphans (values 4-100), got %d", expectedReverseOrphans, result.ReverseOrphanCount)
	}

	// Source matched should be 3
	if result.SourceMatched != 3 {
		t.Errorf("expected 3 source matched, got %d", result.SourceMatched)
	}

	// Target matched should be 3 (only values 1-3 from large table match)
	if result.TargetMatched != 3 {
		t.Errorf("expected 3 target matched, got %d", result.TargetMatched)
	}

	// Calculate the reverse orphan rate (97 / 100 = 0.97 = 97%)
	// This is well above the 50% threshold, so this should be rejected
	targetDistinct := result.TargetMatched + result.ReverseOrphanCount
	reverseOrphanRate := float64(result.ReverseOrphanCount) / float64(targetDistinct)
	if reverseOrphanRate < 0.5 {
		t.Errorf("expected reverse orphan rate > 50%% for false positive detection, got %.2f%%", reverseOrphanRate*100)
	}
}

// TestSchemaDiscoverer_AnalyzeJoin_ValidFK tests that a valid FK relationship is correctly
// identified with low orphan counts in both directions.
func TestSchemaDiscoverer_AnalyzeJoin_ValidFK(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Create a valid FK relationship:
	// - orders table references customers table
	// - All order.customer_id values exist in customers.id
	// - Most customers have at least one order
	setupSQL := `
		CREATE TEMP TABLE test_customers (id INT PRIMARY KEY);
		CREATE TEMP TABLE test_orders (id SERIAL PRIMARY KEY, customer_id INT);
		-- 10 customers
		INSERT INTO test_customers (id) SELECT generate_series(1, 10);
		-- 30 orders spread across customers 1-10 (3 orders per customer on average)
		INSERT INTO test_orders (customer_id) VALUES
			(1), (1), (1), (2), (2), (3), (3), (3), (4), (4),
			(5), (5), (6), (6), (6), (7), (7), (8), (8), (8),
			(9), (9), (10), (10), (10), (1), (2), (3), (4), (5);
	`

	_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
	if err != nil {
		t.Fatalf("failed to create test tables: %v", err)
	}

	// Analyze join from orders.customer_id → customers.id
	result, err := tc.discoverer.AnalyzeJoin(ctx,
		"pg_temp", "test_orders", "customer_id",
		"pg_temp", "test_customers", "id")
	if err != nil {
		t.Fatalf("AnalyzeJoin failed: %v", err)
	}

	// Source→target: all orders reference valid customers → 0 orphans
	if result.OrphanCount != 0 {
		t.Errorf("expected 0 source orphans (all orders have valid customer), got %d", result.OrphanCount)
	}

	// Target→source: all 10 customers have at least one order → 0 reverse orphans
	if result.ReverseOrphanCount != 0 {
		t.Errorf("expected 0 reverse orphans (all customers have orders), got %d", result.ReverseOrphanCount)
	}

	// Join count should be 30 (one per order)
	if result.JoinCount != 30 {
		t.Errorf("expected 30 joined rows, got %d", result.JoinCount)
	}

	// Source matched should be 10 (10 distinct customer_ids in orders)
	if result.SourceMatched != 10 {
		t.Errorf("expected 10 source matched (distinct customer_ids), got %d", result.SourceMatched)
	}

	// Target matched should be 10 (all customers referenced)
	if result.TargetMatched != 10 {
		t.Errorf("expected 10 target matched, got %d", result.TargetMatched)
	}

	// Reverse orphan rate should be 0% - well below 50% threshold
	if result.TargetMatched > 0 {
		reverseOrphanRate := float64(result.ReverseOrphanCount) / float64(result.TargetMatched+result.ReverseOrphanCount)
		if reverseOrphanRate >= 0.5 {
			t.Errorf("expected reverse orphan rate < 50%% for valid FK, got %.2f%%", reverseOrphanRate*100)
		}
	}
}

// TestSchemaDiscoverer_AnalyzeJoin_PartialFK tests a partial FK where source has some orphans
// but the reverse check still passes (most target values exist in source).
func TestSchemaDiscoverer_AnalyzeJoin_PartialFK(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	ctx := context.Background()

	// Create a partial FK relationship:
	// - Most source values exist in target (high match rate)
	// - A few source values are orphans (new customers not yet in lookup)
	// - Most target values are referenced (low reverse orphan rate)
	setupSQL := `
		CREATE TEMP TABLE test_customer_types (id INT PRIMARY KEY, name TEXT);
		CREATE TEMP TABLE test_customer_records (id SERIAL PRIMARY KEY, type_id INT);
		-- 5 customer types
		INSERT INTO test_customer_types (id, name) VALUES
			(1, 'Individual'), (2, 'Business'), (3, 'Enterprise'), (4, 'Partner'), (5, 'Reseller');
		-- 50 customers, mostly using types 1-5, but 2 have type_id=6 (orphan)
		INSERT INTO test_customer_records (type_id) VALUES
			(1), (1), (1), (1), (1), (1), (1), (1), (1), (1),
			(2), (2), (2), (2), (2), (2), (2), (2), (2), (2),
			(3), (3), (3), (3), (3), (3), (3), (3), (3), (3),
			(4), (4), (4), (4), (4), (4), (4), (4),
			(5), (5), (5), (5), (5), (5), (5), (5),
			(6), (6);  -- Orphans: type 6 doesn't exist in customer_types
	`

	_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
	if err != nil {
		t.Fatalf("failed to create test tables: %v", err)
	}

	result, err := tc.discoverer.AnalyzeJoin(ctx,
		"pg_temp", "test_customer_records", "type_id",
		"pg_temp", "test_customer_types", "id")
	if err != nil {
		t.Fatalf("AnalyzeJoin failed: %v", err)
	}

	// Source→target: 1 orphan value (type_id=6 doesn't exist in customer_types)
	if result.OrphanCount != 1 {
		t.Errorf("expected 1 source orphan (type_id=6), got %d", result.OrphanCount)
	}

	// Source matched: 5 distinct values (1-5) that exist in target
	if result.SourceMatched != 5 {
		t.Errorf("expected 5 source matched, got %d", result.SourceMatched)
	}

	// Target→source: 0 reverse orphans (all 5 types are referenced in records)
	if result.ReverseOrphanCount != 0 {
		t.Errorf("expected 0 reverse orphans (all types have records), got %d", result.ReverseOrphanCount)
	}

	// Reverse orphan rate: 0 / 5 = 0% - well below 50% threshold
	// This partial FK should still be accepted by the service layer
	if result.TargetMatched > 0 {
		reverseOrphanRate := float64(result.ReverseOrphanCount) / float64(result.TargetMatched+result.ReverseOrphanCount)
		if reverseOrphanRate >= 0.5 {
			t.Errorf("expected reverse orphan rate < 50%% for partial FK, got %.2f%%", reverseOrphanRate*100)
		}
	}
}

// TestSchemaDiscoverer_AnalyzeJoin_BoundaryThreshold tests boundary conditions around
// the reverse orphan rate threshold. This helps verify the metrics are correct for
// edge cases that the service layer uses for rejection decisions.
func TestSchemaDiscoverer_AnalyzeJoin_BoundaryThreshold(t *testing.T) {
	testCases := []struct {
		name                      string
		sourceValues              string // SQL VALUES list
		targetValues              string // SQL VALUES list
		expectedReverseOrphans    int64
		expectedTargetMatched     int64
		expectedReverseOrphanRate float64
	}{
		{
			name:                      "exactly 50% reverse orphans",
			sourceValues:              "(1), (2), (3), (4), (5)",                           // 5 values
			targetValues:              "(1), (2), (3), (4), (5), (6), (7), (8), (9), (10)", // 10 values
			expectedReverseOrphans:    5,                                                   // 6,7,8,9,10 don't exist in source
			expectedTargetMatched:     5,                                                   // 1-5 exist in both
			expectedReverseOrphanRate: 0.5,                                                 // 5/10 = 50%
		},
		{
			name:                      "just under 50% reverse orphans",
			sourceValues:              "(1), (2), (3), (4), (5), (6)",                      // 6 values
			targetValues:              "(1), (2), (3), (4), (5), (6), (7), (8), (9), (10)", // 10 values
			expectedReverseOrphans:    4,                                                   // 7,8,9,10 don't exist in source
			expectedTargetMatched:     6,                                                   // 1-6 exist in both
			expectedReverseOrphanRate: 0.4,                                                 // 4/10 = 40%
		},
		{
			name:                      "just over 50% reverse orphans",
			sourceValues:              "(1), (2), (3), (4)",                                // 4 values
			targetValues:              "(1), (2), (3), (4), (5), (6), (7), (8), (9), (10)", // 10 values
			expectedReverseOrphans:    6,                                                   // 5,6,7,8,9,10 don't exist in source
			expectedTargetMatched:     4,                                                   // 1-4 exist in both
			expectedReverseOrphanRate: 0.6,                                                 // 6/10 = 60%
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tc := setupSchemaDiscovererTest(t)
			ctx := context.Background()

			// Use unique table names per test case to avoid conflicts
			setupSQL := fmt.Sprintf(`
				DROP TABLE IF EXISTS test_boundary_source CASCADE;
				DROP TABLE IF EXISTS test_boundary_target CASCADE;
				CREATE TEMP TABLE test_boundary_source (val INT);
				CREATE TEMP TABLE test_boundary_target (val INT PRIMARY KEY);
				INSERT INTO test_boundary_source (val) VALUES %s;
				INSERT INTO test_boundary_target (val) VALUES %s;
			`, testCase.sourceValues, testCase.targetValues)

			_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
			if err != nil {
				t.Fatalf("failed to create test tables: %v", err)
			}

			result, err := tc.discoverer.AnalyzeJoin(ctx,
				"pg_temp", "test_boundary_source", "val",
				"pg_temp", "test_boundary_target", "val")
			if err != nil {
				t.Fatalf("AnalyzeJoin failed: %v", err)
			}

			if result.ReverseOrphanCount != testCase.expectedReverseOrphans {
				t.Errorf("expected %d reverse orphans, got %d", testCase.expectedReverseOrphans, result.ReverseOrphanCount)
			}

			if result.TargetMatched != testCase.expectedTargetMatched {
				t.Errorf("expected %d target matched, got %d", testCase.expectedTargetMatched, result.TargetMatched)
			}

			// Verify the reverse orphan rate calculation
			totalTarget := result.TargetMatched + result.ReverseOrphanCount
			actualRate := float64(result.ReverseOrphanCount) / float64(totalTarget)
			tolerance := 0.01 // 1% tolerance for floating point comparison
			if actualRate < testCase.expectedReverseOrphanRate-tolerance || actualRate > testCase.expectedReverseOrphanRate+tolerance {
				t.Errorf("expected reverse orphan rate ~%.2f, got %.2f", testCase.expectedReverseOrphanRate, actualRate)
			}
		})
	}
}

// TestSchemaDiscoverer_AnalyzeJoin_MaxSourceValue tests that the MaxSourceValue field
// is correctly populated for semantic validation (detecting small integer columns like ratings).
func TestSchemaDiscoverer_AnalyzeJoin_MaxSourceValue(t *testing.T) {
	testCases := []struct {
		name         string
		sourceValues string
		targetValues string
		expectedMax  *int64
	}{
		{
			name:         "small integers (likely rating)",
			sourceValues: "(1), (2), (3), (4), (5)",
			targetValues: "(1), (2), (3), (4), (5), (6), (7), (8), (9), (10)",
			expectedMax:  ptrInt64(5),
		},
		{
			name:         "larger integers (likely FK)",
			sourceValues: "(100), (200), (300)",
			targetValues: "(100), (200), (300), (400), (500)",
			expectedMax:  ptrInt64(300),
		},
		{
			name:         "non-numeric values",
			sourceValues: "('abc'), ('def'), ('ghi')",
			targetValues: "('abc'), ('def'), ('ghi'), ('jkl')",
			expectedMax:  nil, // Non-numeric should return NULL
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tc := setupSchemaDiscovererTest(t)
			ctx := context.Background()

			// Create tables with appropriate types
			var setupSQL string
			if testCase.name == "non-numeric values" {
				setupSQL = fmt.Sprintf(`
					DROP TABLE IF EXISTS test_max_source CASCADE;
					DROP TABLE IF EXISTS test_max_target CASCADE;
					CREATE TEMP TABLE test_max_source (val TEXT);
					CREATE TEMP TABLE test_max_target (val TEXT);
					INSERT INTO test_max_source (val) VALUES %s;
					INSERT INTO test_max_target (val) VALUES %s;
				`, testCase.sourceValues, testCase.targetValues)
			} else {
				setupSQL = fmt.Sprintf(`
					DROP TABLE IF EXISTS test_max_source CASCADE;
					DROP TABLE IF EXISTS test_max_target CASCADE;
					CREATE TEMP TABLE test_max_source (val INT);
					CREATE TEMP TABLE test_max_target (val INT);
					INSERT INTO test_max_source (val) VALUES %s;
					INSERT INTO test_max_target (val) VALUES %s;
				`, testCase.sourceValues, testCase.targetValues)
			}

			_, err := tc.discoverer.pool.Exec(ctx, setupSQL)
			if err != nil {
				t.Fatalf("failed to create test tables: %v", err)
			}

			result, err := tc.discoverer.AnalyzeJoin(ctx,
				"pg_temp", "test_max_source", "val",
				"pg_temp", "test_max_target", "val")
			if err != nil {
				t.Fatalf("AnalyzeJoin failed: %v", err)
			}

			if testCase.expectedMax == nil {
				if result.MaxSourceValue != nil {
					t.Errorf("expected nil MaxSourceValue for non-numeric, got %d", *result.MaxSourceValue)
				}
			} else {
				if result.MaxSourceValue == nil {
					t.Errorf("expected MaxSourceValue %d, got nil", *testCase.expectedMax)
				} else if *result.MaxSourceValue != *testCase.expectedMax {
					t.Errorf("expected MaxSourceValue %d, got %d", *testCase.expectedMax, *result.MaxSourceValue)
				}
			}
		})
	}
}

// ptrInt64 is a helper to create *int64 from int64 value
func ptrInt64(v int64) *int64 {
	return &v
}

func TestSchemaDiscoverer_Close(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	host, _ := testDB.Container.Host(ctx)
	port, _ := testDB.Container.MappedPort(ctx, "5432")

	cfg := &Config{
		Host:     host,
		Port:     port.Int(),
		User:     "ekaya",
		Password: "test_password",
		Database: "test_data",
		SSLMode:  "disable",
	}

	// Pass nil for connection manager, zero IDs, and nil logger for unmanaged pool (test mode)
	discoverer, err := NewSchemaDiscoverer(ctx, cfg, nil, uuid.Nil, uuid.Nil, "", nil)
	if err != nil {
		t.Fatalf("failed to create discoverer: %v", err)
	}

	// Close should not error
	if err := discoverer.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Operations after close should fail
	_, err = discoverer.DiscoverTables(ctx)
	if err == nil {
		t.Error("expected error after Close, got nil")
	}
}

// TestSchemaDiscoverer_AnalyzeColumnStats_DebugLogging verifies that debug logging
// is working correctly for column stats collection. This test uses a development logger
// to capture and display debug output from the AnalyzeColumnStats function.
//
// Run with: go test -v -tags='integration postgres' -run 'DebugLogging' ./pkg/adapters/datasource/postgres/...
func TestSchemaDiscoverer_AnalyzeColumnStats_DebugLogging(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	// Use development logger to see debug output
	logger, _ := zap.NewDevelopment()

	// Create discoverer WITH logger to see debug output
	discoverer, err := NewSchemaDiscoverer(ctx, cfg, nil, uuid.Nil, uuid.Nil, "", logger)
	if err != nil {
		t.Fatalf("failed to create schema discoverer: %v", err)
	}
	defer discoverer.Close()

	// Test with users table which has text and integer columns
	// This should trigger the debug logging we added
	t.Log("===== Analyzing column stats for public.users =====")
	columnNames := []string{"user_id", "username", "profile_url", "agent_type", "avg_rating"}
	stats, err := discoverer.AnalyzeColumnStats(ctx, "public", "users", columnNames)
	if err != nil {
		t.Fatalf("AnalyzeColumnStats failed: %v", err)
	}

	t.Logf("Got %d stats results:", len(stats))
	for _, s := range stats {
		t.Logf("  Column: %s, row_count=%d, non_null_count=%d, distinct_count=%d",
			s.ColumnName, s.RowCount, s.NonNullCount, s.DistinctCount)
	}

	// Verify all columns have distinct_count populated
	for _, s := range stats {
		if s.DistinctCount == 0 && s.NonNullCount > 0 {
			t.Errorf("Column %s has non_null_count=%d but distinct_count=0 - this indicates a bug!",
				s.ColumnName, s.NonNullCount)
		}
	}
}
