//go:build postgres || all_adapters

package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
	"github.com/google/uuid"
)

func TestFromMap_ValidConfig(t *testing.T) {
	config := map[string]any{
		"host":     "localhost",
		"port":     float64(5432), // JSON numbers are float64
		"user":     "testuser",
		"password": "testpass",
		"database": "testdb",
		"ssl_mode": "disable",
	}

	cfg, err := FromMap(config)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Host != "localhost" {
		t.Errorf("expected host 'localhost', got '%s'", cfg.Host)
	}
	if cfg.Port != 5432 {
		t.Errorf("expected port 5432, got %d", cfg.Port)
	}
	if cfg.User != "testuser" {
		t.Errorf("expected user 'testuser', got '%s'", cfg.User)
	}
	if cfg.Password != "testpass" {
		t.Errorf("expected password 'testpass', got '%s'", cfg.Password)
	}
	if cfg.Database != "testdb" {
		t.Errorf("expected database 'testdb', got '%s'", cfg.Database)
	}
	if cfg.SSLMode != "disable" {
		t.Errorf("expected ssl_mode 'disable', got '%s'", cfg.SSLMode)
	}
}

func TestFromMap_IntPort(t *testing.T) {
	config := map[string]any{
		"host":     "localhost",
		"port":     5433, // int instead of float64
		"user":     "testuser",
		"database": "testdb",
	}

	cfg, err := FromMap(config)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Port != 5433 {
		t.Errorf("expected port 5433, got %d", cfg.Port)
	}
}

func TestFromMap_Defaults(t *testing.T) {
	config := map[string]any{
		"host":     "localhost",
		"user":     "testuser",
		"database": "testdb",
	}

	cfg, err := FromMap(config)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if cfg.Port != DefaultPort() {
		t.Errorf("expected default port %d, got %d", DefaultPort(), cfg.Port)
	}
	if cfg.SSLMode != DefaultSSLMode() {
		t.Errorf("expected default ssl_mode '%s', got '%s'", DefaultSSLMode(), cfg.SSLMode)
	}
}

func TestFromMap_MissingHost(t *testing.T) {
	config := map[string]any{
		"user":     "testuser",
		"database": "testdb",
	}

	_, err := FromMap(config)
	if err == nil {
		t.Fatal("expected error for missing host")
	}
}

func TestFromMap_MissingUser(t *testing.T) {
	config := map[string]any{
		"host":     "localhost",
		"database": "testdb",
	}

	_, err := FromMap(config)
	if err == nil {
		t.Fatal("expected error for missing user")
	}
}

func TestFromMap_MissingDatabase(t *testing.T) {
	config := map[string]any{
		"host": "localhost",
		"user": "testuser",
	}

	_, err := FromMap(config)
	if err == nil {
		t.Fatal("expected error for missing database")
	}
}

func TestDefaultPort(t *testing.T) {
	if DefaultPort() != 5432 {
		t.Errorf("expected default port 5432, got %d", DefaultPort())
	}
}

func TestDefaultSSLMode(t *testing.T) {
	if DefaultSSLMode() != "require" {
		t.Errorf("expected default ssl_mode 'require', got '%s'", DefaultSSLMode())
	}
}

// Integration test - uses test container for isolation
func TestAdapter_Integration(t *testing.T) {
	// GetTestDB handles short mode skip internally
	testDB := testhelpers.GetTestDB(t)

	// Parse connection info from the test container's connection string
	// Format: postgres://ekaya:test_password@host:port/test_data?sslmode=disable
	connStr := testDB.ConnStr
	// Extract host:port from connection string
	hostPortDB := strings.TrimPrefix(connStr, "postgres://ekaya:test_password@")
	hostPortDB = strings.Split(hostPortDB, "?")[0] // Remove query params
	parts := strings.Split(hostPortDB, "/")
	hostPort := parts[0]
	database := parts[1]

	hostParts := strings.Split(hostPort, ":")
	host := hostParts[0]
	port := 5432
	if len(hostParts) > 1 {
		// Parse port
		for i, c := range hostParts[1] {
			if c < '0' || c > '9' {
				break
			}
			if i == 0 {
				port = 0
			}
			port = port*10 + int(c-'0')
		}
	}

	config := map[string]any{
		"host":     host,
		"port":     float64(port),
		"user":     "ekaya",
		"password": "test_password",
		"database": database,
		"ssl_mode": "disable",
	}

	cfg, err := FromMap(config)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Pass nil for connection manager in tests (creates unmanaged pool)
	adapter, err := NewAdapter(ctx, cfg, nil, uuid.Nil, uuid.Nil, "")
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer adapter.Close()

	if err := adapter.TestConnection(ctx); err != nil {
		t.Fatalf("connection test failed: %v", err)
	}
}

func TestAdapter_ConnectionFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping connection failure test in short mode")
	}

	cfg := &Config{
		Host:     "localhost",
		Port:     59999, // unlikely to be listening
		User:     "nonexistent",
		Password: "wrong",
		Database: "nodb",
		SSLMode:  "disable",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Pass nil for connection manager in tests (creates unmanaged pool)
	adapter, err := NewAdapter(ctx, cfg, nil, uuid.Nil, uuid.Nil, "")
	if err != nil {
		// Connection failed at creation time - this is expected
		return
	}
	defer adapter.Close()

	// If adapter was created, ping should fail
	if err := adapter.TestConnection(ctx); err == nil {
		t.Error("expected connection test to fail with invalid credentials")
	}
}

// TestAdapter_TestConnection_VerifiesDatabaseName tests that TestConnection
// verifies the database name matches the configured database.
func TestAdapter_TestConnection_VerifiesDatabaseName(t *testing.T) {
	// GetTestDB handles short mode skip internally
	testDB := testhelpers.GetTestDB(t)

	// Parse connection info from the test container's connection string
	connStr := testDB.ConnStr
	hostPortDB := strings.TrimPrefix(connStr, "postgres://ekaya:test_password@")
	hostPortDB = strings.Split(hostPortDB, "?")[0]
	parts := strings.Split(hostPortDB, "/")
	hostPort := parts[0]
	database := parts[1]

	hostParts := strings.Split(hostPort, ":")
	host := hostParts[0]
	port := 5432
	if len(hostParts) > 1 {
		for i, c := range hostParts[1] {
			if c < '0' || c > '9' {
				break
			}
			if i == 0 {
				port = 0
			}
			port = port*10 + int(c-'0')
		}
	}

	// Test with correct database name
	cfg := &Config{
		Host:     host,
		Port:     port,
		User:     "ekaya",
		Password: "test_password",
		Database: database,
		SSLMode:  "disable",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	adapter, err := NewAdapter(ctx, cfg, nil, uuid.Nil, uuid.Nil, "")
	if err != nil {
		t.Fatalf("failed to create adapter: %v", err)
	}
	defer adapter.Close()

	// TestConnection should succeed with correct database
	if err := adapter.TestConnection(ctx); err != nil {
		t.Fatalf("connection test should succeed with correct database: %v", err)
	}

	// Test with wrong database name (should fail)
	wrongCfg := &Config{
		Host:     host,
		Port:     port,
		User:     "ekaya",
		Password: "test_password",
		Database: "nonexistent_database_12345",
		SSLMode:  "disable",
	}

	// Try to connect to wrong database - connection might succeed but TestConnection should fail
	wrongAdapter, err := NewAdapter(ctx, wrongCfg, nil, uuid.Nil, uuid.Nil, "")
	if err != nil {
		// Connection failed at creation - this is acceptable
		return
	}
	defer wrongAdapter.Close()

	// TestConnection should fail because database name doesn't match
	if err := wrongAdapter.TestConnection(ctx); err == nil {
		t.Error("expected connection test to fail with wrong database name")
	} else {
		errLower := strings.ToLower(err.Error())
		// Accept either our custom "wrong database" error or PostgreSQL's native "does not exist" error
		if !strings.Contains(errLower, "wrong database") && !strings.Contains(errLower, "does not exist") {
			t.Errorf("expected error about wrong database, got: %v", err)
		}
	}
}
