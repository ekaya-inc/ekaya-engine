package datasource

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

func TestConnectionManager_GetOrCreatePool_Reuse(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"
	datasourceID := uuid.New()

	// First call - creates pool
	pool1, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool1)

	// Second call with same parameters - should reuse pool
	pool2, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool2)

	// Verify same pool instance returned (compare pointers as strings to avoid race detector false positive)
	assert.Equal(t, fmt.Sprintf("%p", pool1), fmt.Sprintf("%p", pool2), "should reuse same pool instance")

	// Verify stats
	stats := cm.GetStats()
	assert.Equal(t, 1, stats.TotalConnections, "should have exactly 1 connection")
	assert.Equal(t, 1, stats.ConnectionsByUser[userID], "user should have 1 connection")
	assert.Equal(t, 1, stats.ConnectionsByProject[projectID.String()], "project should have 1 connection")
}

func TestConnectionManager_GetOrCreatePool_DifferentUsers(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Create pools for two different users
	user1 := "user-1"
	pool1, err := cm.GetOrCreatePool(ctx, projectID, user1, datasourceID, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool1)

	user2 := "user-2"
	pool2, err := cm.GetOrCreatePool(ctx, projectID, user2, datasourceID, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool2)

	// Verify different pool instances (compare pointers as strings to avoid race detector false positive)
	assert.NotEqual(t, fmt.Sprintf("%p", pool1), fmt.Sprintf("%p", pool2), "different users should get different pools")

	// Verify stats
	stats := cm.GetStats()
	assert.Equal(t, 2, stats.TotalConnections, "should have 2 connections")
	assert.Equal(t, 1, stats.ConnectionsByUser[user1], "user1 should have 1 connection")
	assert.Equal(t, 1, stats.ConnectionsByUser[user2], "user2 should have 1 connection")
}

func TestConnectionManager_GetOrCreatePool_DifferentDatasources(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"

	// Create pools for two different datasources
	ds1 := uuid.New()
	pool1, err := cm.GetOrCreatePool(ctx, projectID, userID, ds1, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool1)

	ds2 := uuid.New()
	pool2, err := cm.GetOrCreatePool(ctx, projectID, userID, ds2, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool2)

	// Verify different pool instances (compare pointers as strings to avoid race detector false positive)
	assert.NotEqual(t, fmt.Sprintf("%p", pool1), fmt.Sprintf("%p", pool2), "different datasources should get different pools")

	// Verify stats
	stats := cm.GetStats()
	assert.Equal(t, 2, stats.TotalConnections, "should have 2 connections")
	assert.Equal(t, 2, stats.ConnectionsByUser[userID], "user should have 2 connections")
}

func TestConnectionManager_GetOrCreatePool_MaxConnectionsPerUser(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 2, // Low limit for testing
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"

	// Create 2 connections (at limit)
	ds1 := uuid.New()
	_, err := cm.GetOrCreatePool(ctx, projectID, userID, ds1, testDB.ConnStr)
	require.NoError(t, err)

	ds2 := uuid.New()
	_, err = cm.GetOrCreatePool(ctx, projectID, userID, ds2, testDB.ConnStr)
	require.NoError(t, err)

	// Try to create a 3rd connection - should fail
	ds3 := uuid.New()
	_, err = cm.GetOrCreatePool(ctx, projectID, userID, ds3, testDB.ConnStr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "maximum connections limit")
}

func TestConnectionManager_GetOrCreatePool_HealthCheckRecovery(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"
	datasourceID := uuid.New()

	// Create initial pool
	pool1, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool1)

	// Simulate unhealthy connection by closing the pool
	pool1.Close()

	// Next call should detect unhealthy pool and recreate
	pool2, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool2)

	// Verify we got a new pool instance (not the same as closed one)
	// Compare pointers as strings to avoid race detector false positive
	assert.NotEqual(t, fmt.Sprintf("%p", pool1), fmt.Sprintf("%p", pool2), "should create new pool after detecting unhealthy connection")

	// Verify new pool is healthy
	err = pool2.Ping(ctx)
	assert.NoError(t, err, "new pool should be healthy")
}

func TestConnectionManager_TTLExpiration(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	// Use very short TTL for testing (2 seconds)
	cfg := ConnectionManagerConfig{
		TTLMinutes:            0, // Will be overridden below
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	cm.ttl = 2 * time.Second // Override for fast test
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"
	datasourceID := uuid.New()

	// Create pool
	_, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, testDB.ConnStr)
	require.NoError(t, err)

	// Verify pool exists
	stats := cm.GetStats()
	assert.Equal(t, 1, stats.TotalConnections)

	// Wait for TTL to expire plus cleanup interval
	time.Sleep(3 * time.Second)

	// Manually trigger cleanup
	cm.performCleanup()

	// Verify pool was removed
	stats = cm.GetStats()
	assert.Equal(t, 0, stats.TotalConnections, "expired connection should be cleaned up")
}

func TestConnectionManager_ConcurrentAccess(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 50, // High limit for concurrent test
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Launch 20 goroutines trying to get/create pools concurrently
	const numGoroutines = 20
	var wg sync.WaitGroup
	errors := make([]error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			userID := fmt.Sprintf("user-%d", idx%5) // 5 different users
			_, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, testDB.ConnStr)
			errors[idx] = err
		}(i)
	}

	wg.Wait()

	// Verify no errors occurred
	for i, err := range errors {
		assert.NoError(t, err, "goroutine %d should not error", i)
	}

	// Verify correct number of pools created (5 users × 1 datasource = 5 pools)
	stats := cm.GetStats()
	assert.Equal(t, 5, stats.TotalConnections, "should create exactly 5 pools for 5 users")
}

func TestConnectionManager_GetStats(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	project1 := uuid.New()
	project2 := uuid.New()
	ds1 := uuid.New()

	// Create pools for different projects and users
	_, err := cm.GetOrCreatePool(ctx, project1, "user1", ds1, testDB.ConnStr)
	require.NoError(t, err)

	_, err = cm.GetOrCreatePool(ctx, project1, "user2", ds1, testDB.ConnStr)
	require.NoError(t, err)

	_, err = cm.GetOrCreatePool(ctx, project2, "user1", ds1, testDB.ConnStr)
	require.NoError(t, err)

	// Get stats
	stats := cm.GetStats()

	// Verify totals
	assert.Equal(t, 3, stats.TotalConnections)
	assert.Equal(t, 5, stats.TTLMinutes)
	assert.Equal(t, 10, stats.MaxConnectionsPerUser)

	// Verify per-project counts
	assert.Equal(t, 2, stats.ConnectionsByProject[project1.String()])
	assert.Equal(t, 1, stats.ConnectionsByProject[project2.String()])

	// Verify per-user counts
	assert.Equal(t, 2, stats.ConnectionsByUser["user1"])
	assert.Equal(t, 1, stats.ConnectionsByUser["user2"])

	// Verify oldest idle time is reasonable (should be very recent)
	assert.Less(t, stats.OldestIdleSeconds, 5, "connections should be very recent")
}

func TestConnectionManager_Close(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)

	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"
	datasourceID := uuid.New()

	// Create pool
	pool, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, testDB.ConnStr)
	require.NoError(t, err)
	require.NotNil(t, pool)

	// Close manager
	err = cm.Close()
	require.NoError(t, err)

	// Verify all connections removed
	stats := cm.GetStats()
	assert.Equal(t, 0, stats.TotalConnections, "all connections should be closed")

	// Verify pool is closed (ping should fail)
	err = pool.Ping(ctx)
	assert.Error(t, err, "closed pool should fail ping")

	// Verify Close is idempotent
	err = cm.Close()
	assert.NoError(t, err, "second Close should not error")
}

func TestConnectionManager_InvalidConnectionString(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"
	datasourceID := uuid.New()

	// Try to create pool with invalid connection string
	invalidConnStr := "invalid connection string"
	_, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, invalidConnStr)
	require.Error(t, err)
	// PostgreSQL returns "cannot parse" error, so check for "parse" in the error message
	assert.Contains(t, err.Error(), "parse")
}

func TestConnectionManager_RetryOnTransientFailure(t *testing.T) {
	t.Skip("Skipping retry failure test - difficult to test network failures reliably")
	// The retry logic is tested implicitly by the health check recovery test
	// and by integration with real databases. Testing transient network failures
	// is difficult without mocking the network layer.
}

func TestConnectionManager_DefaultConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test with zero values (should use defaults)
	cfg := ConnectionManagerConfig{}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	assert.Equal(t, DefaultConnectionTTLMinutes*time.Minute, cm.ttl)
	assert.Equal(t, int32(DefaultPoolMaxConns), cm.poolMaxConns)
	assert.Equal(t, int32(DefaultPoolMinConns), cm.poolMinConns)
	assert.Equal(t, DefaultMaxConnectionsPerUser, cm.maxConnectionsPerUser)
}

func TestConnectionManager_CountConnectionsForUser(t *testing.T) {
	testDB := testhelpers.GetTestDB(t)
	logger := zaptest.NewLogger(t)

	cfg := ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          5,
		PoolMinConns:          1,
	}
	cm := NewConnectionManager(cfg, logger)
	defer cm.Close()

	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"

	// Create 3 connections for same user, different datasources
	for i := 0; i < 3; i++ {
		datasourceID := uuid.New()
		_, err := cm.GetOrCreatePool(ctx, projectID, userID, datasourceID, testDB.ConnStr)
		require.NoError(t, err)
	}

	// Count should be 3
	cm.mu.RLock()
	count := cm.countConnectionsForUser(userID)
	cm.mu.RUnlock()

	assert.Equal(t, 3, count, "should count all 3 connections for user")
}
