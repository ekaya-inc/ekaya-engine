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
	discoverer, err := NewSchemaDiscoverer(ctx, cfg, nil, uuid.Nil, uuid.Nil, "")
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
	discoverer, err := NewSchemaDiscoverer(ctx, cfg, connMgr, projectID, datasourceID, userID)
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
