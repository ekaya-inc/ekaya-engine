//go:build postgres || all_adapters

package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

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

// Integration test - skipped if no database is available
func TestAdapter_Integration(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check for required environment variables
	host := os.Getenv("PGHOST")
	user := os.Getenv("PGUSER")
	database := os.Getenv("PGDATABASE")

	if host == "" || user == "" || database == "" {
		t.Skip("skipping integration test: PGHOST, PGUSER, or PGDATABASE not set")
	}

	config := map[string]any{
		"host":     host,
		"user":     user,
		"password": os.Getenv("PGPASSWORD"),
		"database": database,
		"ssl_mode": "disable",
	}

	if port := os.Getenv("PGPORT"); port != "" {
		// Parse port
		var p int
		if _, err := os.Stat(port); err == nil {
			p = 5432
		} else {
			p = 5432 // default
		}
		config["port"] = float64(p)
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
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Check for required environment variables
	host := os.Getenv("PGHOST")
	user := os.Getenv("PGUSER")
	database := os.Getenv("PGDATABASE")

	if host == "" || user == "" || database == "" {
		t.Skip("skipping integration test: PGHOST, PGUSER, or PGDATABASE not set")
	}

	// Test with correct database name
	cfg := &Config{
		Host:     host,
		Port:     5432,
		User:     user,
		Password: os.Getenv("PGPASSWORD"),
		Database: database,
		SSLMode:  "disable",
	}

	if port := os.Getenv("PGPORT"); port != "" {
		// Parse port if provided
		var p int
		if _, err := os.Stat(port); err == nil {
			p = 5432
		} else {
			p = 5432 // default
		}
		cfg.Port = p
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
		Port:     cfg.Port,
		User:     user,
		Password: os.Getenv("PGPASSWORD"),
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
	} else if !strings.Contains(strings.ToLower(err.Error()), "wrong database") {
		t.Errorf("expected error about wrong database, got: %v", err)
	}
}
