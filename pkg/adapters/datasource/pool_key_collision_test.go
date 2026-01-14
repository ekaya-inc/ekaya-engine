//go:build integration

package datasource_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// TestPoolKeyCollision_IndependentPools verifies that connections with DIFFERENT
// projectID/datasourceID correctly use separate pools. This is the expected behavior
// after the HOTFIX-wrong-database.md fix where TestConnection generates unique
// datasource IDs for each test connection.
func TestPoolKeyCollision_IndependentPools(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create two test databases
	projectADB := "independent_a_db"
	projectBDB := "independent_b_db"

	adminPool := testDB.Pool
	_, err := adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", projectADB))
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", projectBDB))
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", projectADB))
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", projectBDB))
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", projectADB))
		_, _ = adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", projectBDB))
	})

	host, err := testDB.Container.Host(ctx)
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	cfg := datasource.ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	connMgr := datasource.NewConnectionManager(cfg, logger)
	defer connMgr.Close()

	projectAConfig := &postgres.Config{
		Host:     host,
		Port:     port.Int(),
		Database: projectADB,
		User:     "ekaya",
		Password: "test_password",
		SSLMode:  "disable",
	}
	projectBConfig := &postgres.Config{
		Host:     host,
		Port:     port.Int(),
		Database: projectBDB,
		User:     "ekaya",
		Password: "test_password",
		SSLMode:  "disable",
	}

	// Use DIFFERENT project and datasource IDs (as would happen for actual datasources)
	projectAID := uuid.New()
	datasourceAID := uuid.New()
	projectBID := uuid.New()
	datasourceBID := uuid.New()
	userID := "test-user"

	// Test connection to Project A with real IDs
	adapterA, err := postgres.NewAdapter(ctx, projectAConfig, connMgr, projectAID, datasourceAID, userID)
	require.NoError(t, err)
	defer adapterA.Close()
	err = adapterA.TestConnection(ctx)
	require.NoError(t, err, "Connection to Project A should succeed")

	// Test connection to Project B with real IDs
	adapterB, err := postgres.NewAdapter(ctx, projectBConfig, connMgr, projectBID, datasourceBID, userID)
	require.NoError(t, err)
	defer adapterB.Close()
	err = adapterB.TestConnection(ctx)
	require.NoError(t, err, "Connection to Project B should succeed")

	// Test connection to Project A again - should work because pool keys are different
	adapterA2, err := postgres.NewAdapter(ctx, projectAConfig, connMgr, projectAID, datasourceAID, userID)
	require.NoError(t, err)
	defer adapterA2.Close()
	err = adapterA2.TestConnection(ctx)
	require.NoError(t, err, "Second connection to Project A should succeed with correct database")

	// Verify we have 2 separate pools
	stats := connMgr.GetStats()
	assert.Equal(t, 2, stats.TotalConnections, "Should have 2 separate connection pools")
}

// TestPoolKeyCollision_VerifyConnectionStringNotInKey documents that connection strings
// are NOT part of the pool key. This is by design - the datasourceID is what differentiates
// connections to different databases. The HOTFIX-wrong-database.md fix ensures that
// TestConnection always uses unique datasource IDs to prevent collisions.
func TestPoolKeyCollision_VerifyConnectionStringNotInKey(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	cfg := datasource.ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	connMgr := datasource.NewConnectionManager(cfg, logger)
	defer connMgr.Close()

	// Get host:port from test container
	host, err := testDB.Container.Host(ctx)
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Two different connection strings with same pool key (uuid.Nil:empty:uuid.Nil)
	connStr1 := fmt.Sprintf("postgresql://ekaya:test_password@%s:%s/test_data?sslmode=disable",
		host, port.Port())
	connStr2 := fmt.Sprintf("postgresql://ekaya:test_password@%s:%s/ekaya_engine_test?sslmode=disable",
		host, port.Port())

	// Create first connection
	pool1, err := connMgr.GetOrCreateConnection(ctx, "postgres", uuid.Nil, "", uuid.Nil, connStr1)
	require.NoError(t, err)

	// Create second connection with different connection string but same key
	pool2, err := connMgr.GetOrCreateConnection(ctx, "postgres", uuid.Nil, "", uuid.Nil, connStr2)
	require.NoError(t, err)

	// With same pool key, pool1 and pool2 are the same object (by design)
	pgPool1, err := datasource.GetPostgresPool(pool1)
	require.NoError(t, err)
	pgPool2, err := datasource.GetPostgresPool(pool2)
	require.NoError(t, err)

	// Compare underlying pool pointers - they should be the same (same key = same pool)
	samePool := fmt.Sprintf("%p", pgPool1) == fmt.Sprintf("%p", pgPool2)
	assert.True(t, samePool, "Same pool key should return same pool (connection string is not in key)")

	// Verify both query the SAME database (the first one created)
	var db1, db2 string
	err = pgPool1.QueryRow(ctx, "SELECT current_database()").Scan(&db1)
	require.NoError(t, err)
	err = pgPool2.QueryRow(ctx, "SELECT current_database()").Scan(&db2)
	require.NoError(t, err)
	assert.Equal(t, db1, db2, "Same pool means same database")

	// Stats should show only 1 connection (correct behavior for same key)
	stats := connMgr.GetStats()
	assert.Equal(t, 1, stats.TotalConnections,
		"Same pool key should result in 1 connection (by design)")
}
