//go:build integration

package postgres

import (
	"context"
	"testing"
	"time"

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

	discoverer, err := NewSchemaDiscoverer(ctx, cfg)
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

	discoverer, err := NewSchemaDiscoverer(ctx, cfg)
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
