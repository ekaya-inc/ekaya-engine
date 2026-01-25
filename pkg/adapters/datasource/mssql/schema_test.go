//go:build mssql || all_adapters

package mssql

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// TestSchemaDiscoverer_NewSchemaDiscoverer_SQLAuth tests schema discovery
// with SQL authentication.
func TestSchemaDiscoverer_NewSchemaDiscoverer_SQLAuth(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check for required environment variables for SQL auth
	host := os.Getenv("MSSQL_HOST")
	user := os.Getenv("MSSQL_USER")
	password := os.Getenv("MSSQL_PASSWORD")
	database := os.Getenv("MSSQL_DATABASE")

	if host == "" || user == "" || password == "" || database == "" {
		t.Skip("skipping integration test: MSSQL_HOST, MSSQL_USER, MSSQL_PASSWORD, or MSSQL_DATABASE not set")
	}

	port := 1433
	if p := os.Getenv("MSSQL_PORT"); p != "" {
		var err error
		port, err = parseInt(p)
		if err != nil {
			t.Fatalf("invalid MSSQL_PORT: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := &Config{
		Host:       host,
		Port:       port,
		Database:   database,
		AuthMethod: "sql",
		Username:   user,
		Password:   password,
		Encrypt:    false,
	}

	// Test without connection manager
	discoverer, err := NewSchemaDiscoverer(ctx, cfg, nil, uuid.Nil, uuid.Nil, "", nil)
	require.NoError(t, err, "failed to create schema discoverer with SQL auth")
	require.NotNil(t, discoverer)
	defer discoverer.Close()

	// Verify we can discover tables (even if empty)
	tables, err := discoverer.DiscoverTables(ctx)
	require.NoError(t, err, "should be able to discover tables")
	assert.NotNil(t, tables, "tables should not be nil")
}

// TestSchemaDiscoverer_NewSchemaDiscoverer_WithConnectionManager tests
// schema discovery with connection manager.
func TestSchemaDiscoverer_NewSchemaDiscoverer_WithConnectionManager(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check for required environment variables for SQL auth
	host := os.Getenv("MSSQL_HOST")
	user := os.Getenv("MSSQL_USER")
	password := os.Getenv("MSSQL_PASSWORD")
	database := os.Getenv("MSSQL_DATABASE")

	if host == "" || user == "" || password == "" || database == "" {
		t.Skip("skipping integration test: MSSQL_HOST, MSSQL_USER, MSSQL_PASSWORD, or MSSQL_DATABASE not set")
	}

	port := 1433
	if p := os.Getenv("MSSQL_PORT"); p != "" {
		var err error
		port, err = parseInt(p)
		if err != nil {
			t.Fatalf("invalid MSSQL_PORT: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	logger := zaptest.NewLogger(t)
	connMgr := datasource.NewConnectionManager(datasource.ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}, logger)
	defer connMgr.Close()

	projectID := uuid.New()
	userID := "test-user"
	datasourceID := uuid.New()

	cfg := &Config{
		Host:       host,
		Port:       port,
		Database:   database,
		AuthMethod: "sql",
		Username:   user,
		Password:   password,
		Encrypt:    false,
	}

	// Test with connection manager
	discoverer, err := NewSchemaDiscoverer(ctx, cfg, connMgr, projectID, datasourceID, userID, logger)
	require.NoError(t, err, "failed to create schema discoverer with connection manager")
	require.NotNil(t, discoverer)
	defer discoverer.Close()

	// Verify we can discover tables
	tables, err := discoverer.DiscoverTables(ctx)
	require.NoError(t, err, "should be able to discover tables")
	assert.NotNil(t, tables, "tables should not be nil")

	// Verify connection is registered in connection manager
	stats := connMgr.GetStats()
	assert.Equal(t, 1, stats.TotalConnections, "connection should be registered")
}

// schemaTestContext holds test dependencies for schema discoverer tests.
type schemaTestContext struct {
	discoverer *SchemaDiscoverer
	cleanup    func()
}

// setupSchemaDiscovererTest creates a schema discoverer for testing.
func setupSchemaDiscovererTest(t *testing.T) *schemaTestContext {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	host := os.Getenv("MSSQL_HOST")
	user := os.Getenv("MSSQL_USER")
	password := os.Getenv("MSSQL_PASSWORD")
	database := os.Getenv("MSSQL_DATABASE")

	if host == "" || user == "" || password == "" || database == "" {
		t.Skip("skipping integration test: MSSQL_HOST, MSSQL_USER, MSSQL_PASSWORD, or MSSQL_DATABASE not set")
	}

	port := 1433
	if p := os.Getenv("MSSQL_PORT"); p != "" {
		var err error
		port, err = parseInt(p)
		if err != nil {
			t.Fatalf("invalid MSSQL_PORT: %v", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)

	logger := zaptest.NewLogger(t)
	cfg := &Config{
		Host:       host,
		Port:       port,
		Database:   database,
		AuthMethod: "sql",
		Username:   user,
		Password:   password,
		Encrypt:    false,
	}

	discoverer, err := NewSchemaDiscoverer(ctx, cfg, nil, uuid.Nil, uuid.Nil, "", logger)
	require.NoError(t, err, "failed to create schema discoverer")

	return &schemaTestContext{
		discoverer: discoverer,
		cleanup: func() {
			cancel()
			discoverer.Close()
		},
	}
}

func TestSchemaDiscoverer_AnalyzeColumnStats_PartialFailure(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	defer tc.cleanup()
	ctx := context.Background()

	// Create a temporary table with valid columns
	setupSQL := `
		CREATE TABLE #test_partial_failure (
			id INT IDENTITY(1,1) PRIMARY KEY,
			name NVARCHAR(100) NOT NULL
		);
		INSERT INTO #test_partial_failure (name) VALUES (N'alice'), (N'bob'), (N'charlie');
	`

	_, err := tc.discoverer.db.ExecContext(ctx, setupSQL)
	require.NoError(t, err, "failed to create test table")

	// Request stats for columns including a nonexistent one.
	// The function should:
	// 1. Try to get column type (will fail for nonexistent column)
	// 2. Fall back to simplified query (will also fail for nonexistent column)
	// 3. Continue processing remaining columns after failures
	columnNames := []string{"id", "nonexistent_column", "name"}
	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "tempdb", "#test_partial_failure", columnNames)

	// Should NOT return an error - partial failures are handled gracefully
	require.NoError(t, err, "AnalyzeColumnStats should handle partial failures")

	// Should return stats for all requested columns
	require.Len(t, stats, 3, "expected 3 stat results")

	// Verify column names are preserved in order
	assert.Equal(t, "id", stats[0].ColumnName)
	assert.Equal(t, "nonexistent_column", stats[1].ColumnName)
	assert.Equal(t, "name", stats[2].ColumnName)

	// Verify id column has accurate stats (3 rows, 3 distinct)
	assert.Equal(t, int64(3), stats[0].RowCount, "id row count")
	assert.Equal(t, int64(3), stats[0].DistinctCount, "id distinct count")

	// Verify nonexistent_column has zero stats (failed to analyze)
	assert.Equal(t, int64(0), stats[1].RowCount, "nonexistent_column row count should be 0")
	assert.Equal(t, int64(0), stats[1].DistinctCount, "nonexistent_column distinct count should be 0")
	assert.Nil(t, stats[1].MinLength, "nonexistent_column min_length should be nil")
	assert.Nil(t, stats[1].MaxLength, "nonexistent_column max_length should be nil")

	// Verify name column still has accurate stats (processed after the failure)
	assert.Equal(t, int64(3), stats[2].RowCount, "name row count")
	assert.Equal(t, int64(3), stats[2].DistinctCount, "name distinct count")
}

func TestSchemaDiscoverer_AnalyzeColumnStats_NonTextTypes(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	defer tc.cleanup()
	ctx := context.Background()

	// Create a temporary table with non-text column types.
	// Binary/varbinary/image columns should get NULL length stats, not errors.
	setupSQL := `
		CREATE TABLE #test_nontext_types (
			id INT IDENTITY(1,1) PRIMARY KEY,
			name NVARCHAR(100) NOT NULL,
			data VARBINARY(MAX),
			num INT NOT NULL
		);
		INSERT INTO #test_nontext_types (name, data, num)
		VALUES
			(N'alice', 0xDEADBEEF, 42),
			(N'bob', 0xCAFE, 100);
	`

	_, err := tc.discoverer.db.ExecContext(ctx, setupSQL)
	require.NoError(t, err, "failed to create test table")

	// Request stats for all columns including binary type
	columnNames := []string{"id", "name", "data", "num"}
	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "tempdb", "#test_nontext_types", columnNames)

	// Should NOT return an error - non-text types should be handled gracefully
	require.NoError(t, err, "AnalyzeColumnStats should handle non-text types")
	require.Len(t, stats, 4, "expected 4 stat results")

	// Verify all columns have correct basic stats (row_count, distinct_count)
	for _, s := range stats {
		assert.Equal(t, int64(2), s.RowCount, "column %s: row count", s.ColumnName)
		assert.Equal(t, int64(2), s.DistinctCount, "column %s: distinct count", s.ColumnName)
	}

	// Verify text column (name) has length stats
	nameStats := stats[1]
	assert.Equal(t, "name", nameStats.ColumnName)
	require.NotNil(t, nameStats.MinLength, "text column 'name' should have min_length")
	require.NotNil(t, nameStats.MaxLength, "text column 'name' should have max_length")
	assert.Equal(t, int64(3), *nameStats.MinLength, "name min_length (bob=3)")
	assert.Equal(t, int64(5), *nameStats.MaxLength, "name max_length (alice=5)")

	// Verify binary column (data) has NULL length stats
	dataStats := stats[2]
	assert.Equal(t, "data", dataStats.ColumnName)
	assert.Nil(t, dataStats.MinLength, "binary column 'data' should have nil min_length")
	assert.Nil(t, dataStats.MaxLength, "binary column 'data' should have nil max_length")

	// Verify integer column (num) has NULL length stats
	numStats := stats[3]
	assert.Equal(t, "num", numStats.ColumnName)
	assert.Nil(t, numStats.MinLength, "integer column 'num' should have nil min_length")
	assert.Nil(t, numStats.MaxLength, "integer column 'num' should have nil max_length")
}

func TestSchemaDiscoverer_AnalyzeColumnStats_RetryWithSimplifiedQuery(t *testing.T) {
	tc := setupSchemaDiscovererTest(t)
	defer tc.cleanup()
	ctx := context.Background()

	// This test verifies the retry mechanism works.
	// We create a scenario where valid and invalid columns are mixed, verifying:
	// 1. Valid columns get full stats
	// 2. Invalid columns trigger retry (both queries fail) and get zero values
	// 3. Processing continues after failures

	setupSQL := `
		CREATE TABLE #test_retry_behavior (
			id INT IDENTITY(1,1) PRIMARY KEY,
			name NVARCHAR(100) NOT NULL,
			count_val INT NOT NULL
		);
		INSERT INTO #test_retry_behavior (name, count_val) VALUES
			(N'alice', 10),
			(N'bob', 20),
			(N'charlie', 30);
	`

	_, err := tc.discoverer.db.ExecContext(ctx, setupSQL)
	require.NoError(t, err, "failed to create test table")

	// Mix of valid columns and invalid columns to test retry behavior
	// The nonexistent columns will fail both main and simplified queries
	columnNames := []string{"id", "invalid_col_1", "name", "invalid_col_2", "count_val"}
	stats, err := tc.discoverer.AnalyzeColumnStats(ctx, "tempdb", "#test_retry_behavior", columnNames)

	require.NoError(t, err, "AnalyzeColumnStats should not return error")
	require.Len(t, stats, 5, "expected 5 stat results")

	// Verify valid columns have correct stats
	// id column (index 0)
	assert.Equal(t, "id", stats[0].ColumnName)
	assert.Equal(t, int64(3), stats[0].RowCount)
	assert.Equal(t, int64(3), stats[0].DistinctCount)

	// invalid_col_1 (index 1) - should have zero values after retry fails
	assert.Equal(t, "invalid_col_1", stats[1].ColumnName)
	assert.Equal(t, int64(0), stats[1].RowCount, "invalid_col_1 should have zero row_count")
	assert.Equal(t, int64(0), stats[1].DistinctCount, "invalid_col_1 should have zero distinct_count")

	// name column (index 2) - should still have correct stats after previous failure
	assert.Equal(t, "name", stats[2].ColumnName)
	assert.Equal(t, int64(3), stats[2].RowCount)
	assert.Equal(t, int64(3), stats[2].DistinctCount)
	// Text column should have length stats
	require.NotNil(t, stats[2].MinLength, "name column should have min_length")
	require.NotNil(t, stats[2].MaxLength, "name column should have max_length")

	// invalid_col_2 (index 3) - should have zero values after retry fails
	assert.Equal(t, "invalid_col_2", stats[3].ColumnName)
	assert.Equal(t, int64(0), stats[3].RowCount, "invalid_col_2 should have zero row_count")
	assert.Equal(t, int64(0), stats[3].DistinctCount, "invalid_col_2 should have zero distinct_count")

	// count_val column (index 4) - should have correct stats
	assert.Equal(t, "count_val", stats[4].ColumnName)
	assert.Equal(t, int64(3), stats[4].RowCount)
	assert.Equal(t, int64(3), stats[4].DistinctCount)
}
