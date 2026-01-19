//go:build integration

package database_test

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// Test_Migrations_InsufficientPermissions verifies that migrations fail fast with
// a clear error when the database user lacks required permissions.
//
// This test reproduces the scenario where:
// 1. A new database is created
// 2. A user is granted CONNECT and basic privileges but NOT schema CREATE privileges
// 3. Migrations should fail immediately with a permission error, not hang
//
// See: https://github.com/ekaya-inc/ekaya-engine issue - migrations hang on permission errors
func Test_Migrations_InsufficientPermissions(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create a unique database and user for this test
	testDBName := "test_migration_perms"
	testUser := "restricted_user"
	testPassword := "test_password"

	// Clean up first in case previous test run failed
	_, _ = testDB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName)
	_, _ = testDB.Pool.Exec(ctx, "DROP USER IF EXISTS "+testUser)

	// Create the test database
	_, err := testDB.Pool.Exec(ctx, "CREATE DATABASE "+testDBName)
	require.NoError(t, err, "Failed to create test database")

	// Create a restricted user
	_, err = testDB.Pool.Exec(ctx, "CREATE USER "+testUser+" WITH PASSWORD '"+testPassword+"'")
	require.NoError(t, err, "Failed to create test user")

	// Grant CONNECT privilege (user can connect to the database)
	_, err = testDB.Pool.Exec(ctx, "GRANT CONNECT ON DATABASE "+testDBName+" TO "+testUser)
	require.NoError(t, err, "Failed to grant CONNECT")

	// NOTE: We intentionally do NOT grant CREATE on schema public
	// This simulates the scenario where a user has database access but
	// cannot create tables (missing: GRANT ALL ON SCHEMA public TO user)

	// Cleanup on test completion
	defer func() {
		// Force disconnect any remaining connections
		_, _ = testDB.Pool.Exec(ctx, `
			SELECT pg_terminate_backend(pg_stat_activity.pid)
			FROM pg_stat_activity
			WHERE pg_stat_activity.datname = $1
			AND pid <> pg_backend_pid()
		`, testDBName)
		time.Sleep(100 * time.Millisecond) // Give connections time to close

		_, _ = testDB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName)
		_, _ = testDB.Pool.Exec(ctx, "DROP USER IF EXISTS "+testUser)
	}()

	// Get host and port from the test container
	host, err := testDB.Container.Host(ctx)
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Build connection string for the restricted user
	connStr := "postgres://" + testUser + ":" + testPassword + "@" + host + ":" + port.Port() + "/" + testDBName + "?sslmode=disable"

	// Open connection as the restricted user
	restrictedDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err, "Failed to open connection as restricted user")
	defer restrictedDB.Close()

	// Verify connection works
	err = restrictedDB.Ping()
	require.NoError(t, err, "Restricted user should be able to connect")

	// Verify the user cannot create tables (confirm our test setup is correct)
	_, err = restrictedDB.Exec("CREATE TABLE test_table (id int)")
	require.Error(t, err, "Restricted user should NOT be able to create tables")
	assert.Contains(t, err.Error(), "permission denied", "Error should indicate permission denied")

	// Now test the actual migration behavior
	// This should fail fast with a permission error, not hang
	logger := zap.NewNop()

	// Use a timeout to detect if migrations hang
	done := make(chan error, 1)
	go func() {
		done <- database.RunMigrations(restrictedDB, logger)
	}()

	select {
	case err := <-done:
		// Migrations completed (with error, as expected)
		require.Error(t, err, "Migrations should fail with insufficient permissions")
		assert.Contains(t, err.Error(), "permission denied",
			"Error should indicate permission denied for schema operations")
		t.Logf("Migration failed as expected with error: %v", err)

	case <-time.After(30 * time.Second):
		t.Fatal("TIMEOUT: Migrations hung instead of failing with permission error. " +
			"This indicates the migration system does not properly handle permission errors.")
	}
}

// Test_Migrations_SuccessWithProperPermissions verifies migrations work when
// the user has proper permissions (control test to ensure our permission setup is correct).
func Test_Migrations_SuccessWithProperPermissions(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create a unique database and user for this test
	testDBName := "test_migration_success"
	testUser := "full_perms_user"
	testPassword := "test_password"

	// Clean up first
	_, _ = testDB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName)
	_, _ = testDB.Pool.Exec(ctx, "DROP USER IF EXISTS "+testUser)

	// Create the test database
	_, err := testDB.Pool.Exec(ctx, "CREATE DATABASE "+testDBName)
	require.NoError(t, err, "Failed to create test database")

	// Create user with full privileges
	_, err = testDB.Pool.Exec(ctx, "CREATE USER "+testUser+" WITH PASSWORD '"+testPassword+"'")
	require.NoError(t, err, "Failed to create test user")

	// Grant all necessary privileges
	_, err = testDB.Pool.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+testDBName+" TO "+testUser)
	require.NoError(t, err, "Failed to grant database privileges")

	// Connect to the new database to grant schema privileges
	host, err := testDB.Container.Host(ctx)
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Connect as superuser to the new database to grant schema permissions
	superConnStr := "postgres://ekaya:test_password@" + host + ":" + port.Port() + "/" + testDBName + "?sslmode=disable"
	superDB, err := sql.Open("pgx", superConnStr)
	require.NoError(t, err)
	defer superDB.Close()

	// Grant schema permissions
	_, err = superDB.Exec("GRANT ALL ON SCHEMA public TO " + testUser)
	require.NoError(t, err, "Failed to grant schema privileges")

	// Cleanup on test completion
	defer func() {
		// Force disconnect any remaining connections
		_, _ = testDB.Pool.Exec(ctx, `
			SELECT pg_terminate_backend(pg_stat_activity.pid)
			FROM pg_stat_activity
			WHERE pg_stat_activity.datname = $1
			AND pid <> pg_backend_pid()
		`, testDBName)
		time.Sleep(100 * time.Millisecond)

		_, _ = testDB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName)
		_, _ = testDB.Pool.Exec(ctx, "DROP USER IF EXISTS "+testUser)
	}()

	// Build connection string for the user with full permissions
	connStr := "postgres://" + testUser + ":" + testPassword + "@" + host + ":" + port.Port() + "/" + testDBName + "?sslmode=disable"

	// Open connection
	userDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err, "Failed to open connection")
	defer userDB.Close()

	// Run migrations - should succeed
	logger := zap.NewNop()

	done := make(chan error, 1)
	go func() {
		done <- database.RunMigrations(userDB, logger)
	}()

	select {
	case err := <-done:
		require.NoError(t, err, "Migrations should succeed with proper permissions")
		t.Log("Migrations completed successfully with proper permissions")

	case <-time.After(60 * time.Second):
		t.Fatal("TIMEOUT: Migrations took too long even with proper permissions")
	}

	// Verify migrations actually ran by checking for a table
	// Note: RunMigrations closes the connection, so we need a fresh one for verification
	verifyDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err, "Failed to open verification connection")
	defer verifyDB.Close()

	var tableExists bool
	err = verifyDB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_name = 'engine_projects'
		)
	`).Scan(&tableExists)
	require.NoError(t, err)
	assert.True(t, tableExists, "engine_projects table should exist after migrations")
}

// Test_Migrations_ConcurrentAccessBlocking verifies behavior when another connection
// holds a lock on the schema_migrations table (simulating concurrent migration attempts).
func Test_Migrations_ConcurrentAccessBlocking(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create a unique database for this test
	testDBName := "test_migration_concurrent"
	testUser := "concurrent_user"
	testPassword := "test_password"

	// Clean up first
	_, _ = testDB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName)
	_, _ = testDB.Pool.Exec(ctx, "DROP USER IF EXISTS "+testUser)

	// Create the test database and user
	_, err := testDB.Pool.Exec(ctx, "CREATE DATABASE "+testDBName)
	require.NoError(t, err)
	_, err = testDB.Pool.Exec(ctx, "CREATE USER "+testUser+" WITH PASSWORD '"+testPassword+"'")
	require.NoError(t, err)
	_, err = testDB.Pool.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+testDBName+" TO "+testUser)
	require.NoError(t, err)

	// Get container connection details
	host, err := testDB.Container.Host(ctx)
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Connect as superuser to grant schema permissions
	superConnStr := "postgres://ekaya:test_password@" + host + ":" + port.Port() + "/" + testDBName + "?sslmode=disable"
	superDB, err := sql.Open("pgx", superConnStr)
	require.NoError(t, err)
	defer superDB.Close()

	_, err = superDB.Exec("GRANT ALL ON SCHEMA public TO " + testUser)
	require.NoError(t, err)

	// Cleanup on test completion
	defer func() {
		_, _ = testDB.Pool.Exec(ctx, `
			SELECT pg_terminate_backend(pg_stat_activity.pid)
			FROM pg_stat_activity
			WHERE pg_stat_activity.datname = $1
			AND pid <> pg_backend_pid()
		`, testDBName)
		time.Sleep(100 * time.Millisecond)
		_, _ = testDB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName)
		_, _ = testDB.Pool.Exec(ctx, "DROP USER IF EXISTS "+testUser)
	}()

	// User connection string
	connStr := "postgres://" + testUser + ":" + testPassword + "@" + host + ":" + port.Port() + "/" + testDBName + "?sslmode=disable"

	// First, run migrations successfully to create schema_migrations table
	firstDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer firstDB.Close()

	err = database.RunMigrations(firstDB, zap.NewNop())
	require.NoError(t, err, "First migration run should succeed")

	// Now open a connection and lock the schema_migrations table
	lockDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer lockDB.Close()

	// Start a transaction and lock the schema_migrations table
	tx, err := lockDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer tx.Rollback()

	// Acquire an exclusive lock on the schema_migrations table
	_, err = tx.Exec("LOCK TABLE schema_migrations IN ACCESS EXCLUSIVE MODE")
	require.NoError(t, err, "Should be able to lock schema_migrations table")

	// Now try to run migrations from another connection
	// This should block waiting for the lock
	secondDB, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	defer secondDB.Close()

	// Set a statement timeout on the second connection to prevent indefinite blocking
	_, err = secondDB.Exec("SET statement_timeout = '5s'")
	require.NoError(t, err)

	logger := zap.NewNop()
	var wg sync.WaitGroup
	var migrationErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		migrationErr = database.RunMigrations(secondDB, logger)
	}()

	// Wait for migration to complete (with timeout)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// The migration completed (either succeeded because no changes, or failed with timeout)
		if migrationErr != nil {
			// Expected: either "no change" or a timeout error
			t.Logf("Concurrent migration result: %v", migrationErr)
			// The error should indicate lock timeout or cancellation, not hang
			assert.True(t,
				migrationErr.Error() == "no change" ||
					containsAny(migrationErr.Error(), "timeout", "lock", "cancel"),
				"Error should indicate timeout, lock, or cancellation, got: %v", migrationErr)
		} else {
			// No error means database was already migrated (ErrNoChange converted to nil)
			t.Log("Concurrent migration completed with no changes (expected for already-migrated database)")
		}

	case <-time.After(30 * time.Second):
		t.Fatal("TIMEOUT: Migrations hung when table was locked. " +
			"The migration system should respect statement_timeout and not block indefinitely.")
	}
}

// containsAny checks if s contains any of the substrings
func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if len(sub) > 0 && len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// Test_Migrations_InsufficientSchemaPermissions_AppPath mimics the exact application
// startup path: pgxpool -> stdlib.OpenDBFromPool -> RunMigrations.
//
// This reproduces the scenario where:
// 1. GRANT ALL PRIVILEGES ON DATABASE was run (grants CONNECT, CREATE SCHEMA, TEMP)
// 2. GRANT ALL ON SCHEMA public was NOT run (missing CREATE on public schema)
// 3. Application tries to run migrations and should fail (not hang)
//
// KNOWN BUG: This test WILL HANG without the fix. The issue is that when using
// stdlib.OpenDBFromPool (app path), migrations hang on permission errors, whereas
// raw sql.Open fails fast. This is due to how golang-migrate's postgres driver
// interacts with the pgx stdlib adapter.
//
// The fix is to add a statement_timeout or connection timeout to the migration connection.
func Test_Migrations_InsufficientSchemaPermissions_AppPath(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	ctx := context.Background()

	// Create a unique database and user for this test
	testDBName := "test_migration_app_path"
	testUser := "app_path_user"
	testPassword := "test_password"

	// Clean up first in case previous test run failed
	_, _ = testDB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName)
	_, _ = testDB.Pool.Exec(ctx, "DROP USER IF EXISTS "+testUser)

	// Create the test database
	_, err := testDB.Pool.Exec(ctx, "CREATE DATABASE "+testDBName)
	require.NoError(t, err, "Failed to create test database")

	// Create a user (simulating: CREATE USER damon WITH PASSWORD 'xxx')
	_, err = testDB.Pool.Exec(ctx, "CREATE USER "+testUser+" WITH PASSWORD '"+testPassword+"'")
	require.NoError(t, err, "Failed to create test user")

	// Grant database privileges (simulating: GRANT ALL PRIVILEGES ON DATABASE damon_engine TO damon)
	// This grants CONNECT, CREATE (schema), TEMPORARY - but NOT create on public schema
	_, err = testDB.Pool.Exec(ctx, "GRANT ALL PRIVILEGES ON DATABASE "+testDBName+" TO "+testUser)
	require.NoError(t, err, "Failed to grant database privileges")

	// NOTE: We intentionally do NOT run: GRANT ALL ON SCHEMA public TO user
	// This is the exact scenario the user reported - they thought GRANT ALL ON DATABASE was enough

	// Cleanup on test completion
	defer func() {
		// Force disconnect any remaining connections
		_, _ = testDB.Pool.Exec(ctx, `
			SELECT pg_terminate_backend(pg_stat_activity.pid)
			FROM pg_stat_activity
			WHERE pg_stat_activity.datname = $1
			AND pid <> pg_backend_pid()
		`, testDBName)
		time.Sleep(100 * time.Millisecond) // Give connections time to close

		_, _ = testDB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+testDBName)
		_, _ = testDB.Pool.Exec(ctx, "DROP USER IF EXISTS "+testUser)
	}()

	// Get host and port from the test container
	host, err := testDB.Container.Host(ctx)
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Build connection URL exactly like the app does in main.go:setupDatabase
	databaseURL := fmt.Sprintf("postgresql://%s:%s@%s:%s/%s?sslmode=disable",
		testUser, testPassword, host, port.Port(), testDBName)

	// Use pgxpool exactly like the app does
	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	require.NoError(t, err, "Failed to parse database URL")

	poolConfig.MaxConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	require.NoError(t, err, "Failed to create connection pool")
	defer pool.Close()

	// Verify connection works (app does pool.Ping)
	err = pool.Ping(ctx)
	require.NoError(t, err, "Pool ping should succeed - user can connect")

	// Convert pgxpool to *sql.DB exactly like the app does: stdlib.OpenDBFromPool
	stdDB := stdlib.OpenDBFromPool(pool)
	defer stdDB.Close()

	// Verify the user cannot create tables (confirm test setup is correct)
	_, err = stdDB.Exec("CREATE TABLE test_table (id int)")
	require.Error(t, err, "User should NOT be able to create tables without schema permissions")
	assert.Contains(t, err.Error(), "permission denied", "Error should indicate permission denied")

	// Close the pgxpool connection - we'll use the fixed approach instead
	stdDB.Close()
	pool.Close()

	// Apply the fix: Use a direct sql.Open connection with statement_timeout in the URL
	// This replicates what main.go:runMigrations now does
	const migrationTimeout = 5 * time.Second // Use shorter timeout for tests
	timeoutMS := int(migrationTimeout.Milliseconds())
	migrationURL := fmt.Sprintf("%s&statement_timeout=%d", databaseURL, timeoutMS)

	migrationDB, err := sql.Open("pgx", migrationURL)
	require.NoError(t, err, "Should be able to open migration connection")
	defer migrationDB.Close()

	// Verify connection works
	err = migrationDB.Ping()
	require.NoError(t, err, "Migration connection ping should succeed")

	// Now test the actual migration behavior using the fixed code path
	// With the fix, this should timeout and return an error, not hang
	logger := zap.NewNop()

	t.Log("Starting migrations with direct sql.Open + statement_timeout in URL")
	t.Log("With the fix, this should fail within the timeout period")

	// Use a timeout to detect if migrations still hang despite the fix
	done := make(chan error, 1)
	go func() {
		done <- database.RunMigrations(migrationDB, logger)
	}()

	select {
	case err := <-done:
		// Migrations completed (with error, as expected)
		require.Error(t, err, "Migrations should fail with insufficient schema permissions")

		// The error should contain either "permission denied" or "timeout"
		errStr := err.Error()
		hasPermissionDenied := containsAny(errStr, "permission denied")
		hasTimeout := containsAny(errStr, "timeout", "canceling statement")

		assert.True(t, hasPermissionDenied || hasTimeout,
			"Error should indicate permission denied or timeout, got: %v", err)
		t.Logf("Migration failed as expected with error: %v", err)

	case <-time.After(30 * time.Second):
		t.Fatal("TIMEOUT: Migrations hung despite statement_timeout fix. " +
			"The fix should prevent indefinite hangs.")
	}
}
