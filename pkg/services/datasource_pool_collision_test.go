//go:build integration

package services_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	_ "github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource/postgres" // Register postgres adapter
	"github.com/ekaya-inc/ekaya-engine/pkg/crypto"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// testEncryptionKey is a base64-encoded 32-byte key for testing
const poolTestEncryptionKey = "dGVzdC1rZXktZm9yLXVuaXQtdGVzdHMtMzItYnl0ZXM="

// TestDatasourceService_MCPHealthPoolCollision reproduces the HOTFIX-wrong-database bug
// at the service layer, simulating the exact flow that the MCP health check uses.
//
// The bug flow:
// 1. Project A exists with datasource pointing to database_a
// 2. Project B exists with datasource pointing to database_b
// 3. User calls MCP health for Project A → TestConnection creates pool for database_a with uuid.Nil key
// 4. User calls MCP health for Project B → TestConnection reuses pool (wrong database!)
// 5. Health check detects mismatch and reports error
func TestDatasourceService_MCPHealthPoolCollision(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create two test databases to simulate different customer databases
	databaseA := "mcp_health_db_a"
	databaseB := "mcp_health_db_b"

	adminPool := testDB.Pool
	_, err := adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", databaseA))
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", databaseB))
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", databaseA))
	require.NoError(t, err)
	_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", databaseB))
	require.NoError(t, err)

	t.Cleanup(func() {
		_, _ = adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", databaseA))
		_, _ = adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", databaseB))
	})

	// Get host:port from test container
	host, err := testDB.Container.Host(ctx)
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Create connection manager (simulates production server with shared pool)
	connMgrCfg := datasource.ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	connMgr := datasource.NewConnectionManager(connMgrCfg, logger)
	defer connMgr.Close()

	// Create adapter factory with connection manager
	adapterFactory := datasource.NewDatasourceAdapterFactory(connMgr)

	// Create encryptor for datasource configs
	encryptor, err := crypto.NewCredentialEncryptor(poolTestEncryptionKey)
	require.NoError(t, err)

	// Create datasource service - this is what MCP health uses
	// We pass nil for repos/projectService since we only need TestConnection
	dsService := services.NewDatasourceService(nil, nil, encryptor, adapterFactory, nil, logger)

	// Create datasource configs pointing to different databases
	configA := map[string]any{
		"host":     host,
		"port":     float64(port.Int()), // JSON numbers are float64
		"database": databaseA,
		"user":     "ekaya",
		"password": "test_password",
		"ssl_mode": "disable",
	}
	configB := map[string]any{
		"host":     host,
		"port":     float64(port.Int()),
		"database": databaseB,
		"user":     "ekaya",
		"password": "test_password",
		"ssl_mode": "disable",
	}

	// === SIMULATE MCP HEALTH CHECK FLOW ===

	// Step 1: MCP health check for Project A (first user)
	// This mimics: deps.DatasourceService.TestConnection(ctx, ds.DatasourceType, ds.Config)
	t.Log("Step 1: Testing connection to Project A (database_a)")
	err = dsService.TestConnection(ctx, "postgres", configA, uuid.Nil)
	require.NoError(t, err, "TestConnection to Project A should succeed")

	// Step 2: MCP health check for Project B (second user or same user switching projects)
	// THE BUG: Connection pool returns the connection from Step 1 (to database_a)
	// because TestConnection uses uuid.Nil keys, causing pool key collision
	t.Log("Step 2: Testing connection to Project B (database_b)")
	err = dsService.TestConnection(ctx, "postgres", configB, uuid.Nil)

	if err != nil {
		// Bug reproduced - document the failure
		assert.Contains(t, err.Error(), "connected to wrong database",
			"BUG CONFIRMED: Pool key collision causes wrong database")
		t.Logf("BUG REPRODUCED in service layer: %v", err)
		t.Errorf("HOTFIX-wrong-database.md: MCP health check returns wrong database due to TestConnection pool collision")
	} else {
		t.Log("Bug appears to be fixed - TestConnection correctly connects to Project B's database")
	}

	// Log connection pool stats to help debug
	stats := connMgr.GetStats()
	t.Logf("Connection pool stats: total=%d, by_project=%v, by_user=%v",
		stats.TotalConnections, stats.ConnectionsByProject, stats.ConnectionsByUser)

	// The bug symptom: should have 1 connection (shared incorrectly) instead of 0 (test connections shouldn't pool)
	// or 2 (if each connection string had its own pool)
}

// TestDatasourceService_SequentialTestConnections verifies the exact sequence from the HOTFIX:
// 1. Test Connection for tikr-all → Creates pool with uuid.Nil key
// 2. Test Connection for tikr-production → Pool returns tikr-all connection (BUG!)
// The bug manifests on the SECOND test connection because the pool key collision
// causes the first pool (tikr-all) to be returned for all subsequent test connections.
func TestDatasourceService_SequentialTestConnections(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)
	ctx := context.Background()

	// Create databases simulating tikr_all and tikr_production
	tikrAll := "tikr_all_test"
	tikrProduction := "tikr_production_test"

	adminPool := testDB.Pool
	for _, db := range []string{tikrAll, tikrProduction} {
		_, err := adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", db))
		require.NoError(t, err)
		_, err = adminPool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", db))
		require.NoError(t, err)
	}

	t.Cleanup(func() {
		_, _ = adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", tikrAll))
		_, _ = adminPool.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", tikrProduction))
	})

	host, err := testDB.Container.Host(ctx)
	require.NoError(t, err)
	port, err := testDB.Container.MappedPort(ctx, "5432")
	require.NoError(t, err)

	// Create connection manager and adapter factory
	connMgr := datasource.NewConnectionManager(datasource.ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}, logger)
	defer connMgr.Close()

	adapterFactory := datasource.NewDatasourceAdapterFactory(connMgr)
	encryptor, err := crypto.NewCredentialEncryptor(poolTestEncryptionKey)
	require.NoError(t, err)

	// Create minimal datasource service (no repos needed for TestConnection)
	dsService := services.NewDatasourceService(nil, nil, encryptor, adapterFactory, nil, logger)

	// Configs matching the HOTFIX scenario
	configTikrAll := map[string]any{
		"host":     host,
		"port":     float64(port.Int()),
		"database": tikrAll,
		"user":     "ekaya",
		"password": "test_password",
		"ssl_mode": "disable",
	}
	configTikrProduction := map[string]any{
		"host":     host,
		"port":     float64(port.Int()),
		"database": tikrProduction,
		"user":     "ekaya",
		"password": "test_password",
		"ssl_mode": "disable",
	}

	// === EXACT REPRO SEQUENCE FROM HOTFIX ===

	// Step 1: "Test Connection" for tikr-all (first project)
	// This creates a pool with key: "00000000-0000-0000-0000-000000000000::00000000-0000-0000-0000-000000000000"
	t.Log("REPRO Step 1: Test Connection for tikr-all (first time)")
	err = dsService.TestConnection(ctx, "postgres", configTikrAll, uuid.Nil)
	require.NoError(t, err, "First test of tikr-all should succeed")
	t.Log("✓ tikr-all: connected successfully (pool created)")

	// Step 2: "Test Connection" for tikr-production (second project)
	// THE BUG MANIFESTS HERE:
	// - Pool lookup uses same key: "00000000-0000-0000-0000-000000000000::00000000-0000-0000-0000-000000000000"
	// - Connection manager returns the pool from Step 1 (connected to tikr_all)
	// - TestConnection detects mismatch: expected "tikr_production" but connected to "tikr_all"
	t.Log("REPRO Step 2: Test Connection for tikr-production - BUG CHECK")
	err = dsService.TestConnection(ctx, "postgres", configTikrProduction, uuid.Nil)

	if err != nil {
		// Bug reproduced - document the behavior
		assert.Contains(t, err.Error(), "connected to wrong database",
			"BUG CONFIRMED: Pool key collision causes wrong database")
		assert.Contains(t, err.Error(), tikrProduction, "Error should mention expected database")
		assert.Contains(t, err.Error(), tikrAll, "Error should mention actual (wrong) database")

		t.Logf("BUG REPRODUCED exactly as documented in HOTFIX-wrong-database.md")
		t.Logf("Expected: %s, Got: %s", tikrProduction, tikrAll)
		t.Logf("Error: %v", err)

		t.Errorf("CRITICAL SECURITY BUG: Connection pool returned wrong database. " +
			"This causes cross-project data leakage!")
	} else {
		t.Log("✓ Bug appears to be FIXED - tikr-production correctly connected")
	}

	// Verify pool stats show only 1 connection (proof of bug)
	stats := connMgr.GetStats()
	t.Logf("Final pool stats: total=%d, by_project=%v", stats.TotalConnections, stats.ConnectionsByProject)

	if stats.TotalConnections == 1 {
		t.Log("BUG SYMPTOM CONFIRMED: Only 1 pooled connection exists for 2 different databases")
	}
}
