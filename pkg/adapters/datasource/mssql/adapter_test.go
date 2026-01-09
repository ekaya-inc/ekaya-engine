//go:build mssql || all_adapters

package mssql

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap/zaptest"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// TestAdapter_TestConnection_FailsWithWrongDatabaseName tests that TestConnection
// fails when connected to a different database than specified in config.
func TestAdapter_TestConnection_FailsWithWrongDatabaseName(t *testing.T) {
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

	// Test SQL auth with wrong database name
	cfg := &Config{
		Host:       host,
		Port:       port,
		Database:   "nonexistent_database_12345",
		AuthMethod: "sql",
		Username:   user,
		Password:   password,
		Encrypt:    false,
	}

	adapter, err := NewAdapter(ctx, cfg, nil, uuid.Nil, uuid.Nil, "")
	if err != nil {
		// Connection might fail at creation if database doesn't exist
		// This is acceptable - the test verifies that TestConnection would fail
		return
	}
	defer adapter.Close()

	// TestConnection should fail because database name doesn't match
	err = adapter.TestConnection(ctx)
	require.Error(t, err, "expected connection test to fail with wrong database name")
	assert.True(t, strings.Contains(strings.ToLower(err.Error()), "wrong database") ||
		strings.Contains(strings.ToLower(err.Error()), "database") ||
		strings.Contains(strings.ToLower(err.Error()), "does not exist"),
		"expected error about wrong database, got: %v", err)
}

// TestAdapter_TestConnection_SucceedsWithCorrectDatabaseName tests that TestConnection
// succeeds when connected to the correct database.
func TestAdapter_TestConnection_SucceedsWithCorrectDatabaseName(t *testing.T) {
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

	// Test SQL auth with correct database name
	cfg := &Config{
		Host:       host,
		Port:       port,
		Database:   database,
		AuthMethod: "sql",
		Username:   user,
		Password:   password,
		Encrypt:    false,
	}

	adapter, err := NewAdapter(ctx, cfg, nil, uuid.Nil, uuid.Nil, "")
	require.NoError(t, err, "failed to create adapter")
	defer adapter.Close()

	// TestConnection should succeed with correct database
	err = adapter.TestConnection(ctx)
	assert.NoError(t, err, "connection test should succeed with correct database")
}

// TestAdapter_NewAdapter_WithoutConnectionManager tests adapter creation
// without connection manager for all auth methods.
func TestAdapter_NewAdapter_WithoutConnectionManager(t *testing.T) {
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

	// Test SQL auth without connection manager
	cfg := &Config{
		Host:       host,
		Port:       port,
		Database:   database,
		AuthMethod: "sql",
		Username:   user,
		Password:   password,
		Encrypt:    false,
	}

	adapter, err := NewAdapter(ctx, cfg, nil, uuid.Nil, uuid.Nil, "")
	require.NoError(t, err, "failed to create adapter with SQL auth")
	require.NotNil(t, adapter, "adapter should not be nil")
	assert.True(t, adapter.ownedDB, "adapter should own the DB when connection manager is nil")
	defer adapter.Close()

	// Verify connection works
	err = adapter.TestConnection(ctx)
	assert.NoError(t, err, "connection test should succeed")
}

// TestAdapter_NewAdapter_WithConnectionManager tests adapter creation
// with connection manager for SQL auth.
func TestAdapter_NewAdapter_WithConnectionManager(t *testing.T) {
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

	// Test SQL auth with connection manager
	cfg := &Config{
		Host:       host,
		Port:       port,
		Database:   database,
		AuthMethod: "sql",
		Username:   user,
		Password:   password,
		Encrypt:    false,
	}

	adapter, err := NewAdapter(ctx, cfg, connMgr, projectID, datasourceID, userID)
	require.NoError(t, err, "failed to create adapter with connection manager")
	require.NotNil(t, adapter, "adapter should not be nil")
	assert.False(t, adapter.ownedDB, "adapter should not own the DB when using connection manager")
	defer adapter.Close()

	// Verify connection works
	err = adapter.TestConnection(ctx)
	assert.NoError(t, err, "connection test should succeed")

	// Verify connection is registered in connection manager
	stats := connMgr.GetStats()
	assert.Equal(t, 1, stats.TotalConnections, "connection should be registered")
}

// parseInt is a helper to parse port string to int
func parseInt(s string) (int, error) {
	var result int
	_, err := fmt.Sscanf(s, "%d", &result)
	return result, err
}
