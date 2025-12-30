package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_EnvOverridesYAML(t *testing.T) {
	// Create a temp directory with a config.yaml
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
port: "3443"
env: "test"
database:
  host: "db.example.com"
  port: 5432
  user: "testuser"
  database: "testdb"
redis:
  host: "redis.example.com"
  port: 6379
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Change to temp directory so Load() finds config.yaml
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalDir)
	})

	// Clear env vars that might interfere with test
	os.Unsetenv("PGHOST")
	os.Unsetenv("BASE_URL")

	// Set env vars to override YAML values
	t.Setenv("PORT", "4443")
	t.Setenv("ENVIRONMENT", "production")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify env vars override YAML
	if cfg.Port != "4443" {
		t.Errorf("expected Port=4443 (from env), got %s", cfg.Port)
	}
	if cfg.Env != "production" {
		t.Errorf("expected Env=production (from env), got %s", cfg.Env)
	}

	// Verify version was set
	if cfg.Version != "test-version" {
		t.Errorf("expected Version=test-version, got %s", cfg.Version)
	}

	// Verify BaseURL was auto-derived from PORT
	if cfg.BaseURL != "http://localhost:4443" {
		t.Errorf("expected BaseURL=http://localhost:4443 (auto-derived from PORT), got %s", cfg.BaseURL)
	}

	// Verify YAML value used for database host (proves YAML was read)
	if cfg.Database.Host != "db.example.com" {
		t.Errorf("expected Database.Host=db.example.com (from yaml), got %s", cfg.Database.Host)
	}
}

func TestLoad_BaseURLAutoDerive(t *testing.T) {
	// Create a temp directory with a minimal config.yaml
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
port: "5678"
env: "test"
database:
  host: "localhost"
redis:
  host: "localhost"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalDir)
	})

	// Clear BASE_URL to test auto-derivation
	os.Unsetenv("BASE_URL")
	os.Unsetenv("PORT")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify BaseURL was auto-derived from port in YAML
	if cfg.BaseURL != "http://localhost:5678" {
		t.Errorf("expected BaseURL=http://localhost:5678 (auto-derived), got %s", cfg.BaseURL)
	}
}

func TestLoad_BaseURLExplicit(t *testing.T) {
	// Create a temp directory with a config that sets base_url explicitly
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
port: "3443"
env: "test"
base_url: "http://my-server.internal:8080"
database:
  host: "localhost"
redis:
  host: "localhost"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalDir)
	})

	// Clear env vars
	os.Unsetenv("BASE_URL")
	os.Unsetenv("PORT")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify explicit BaseURL is used (not auto-derived)
	if cfg.BaseURL != "http://my-server.internal:8080" {
		t.Errorf("expected BaseURL=http://my-server.internal:8080 (explicit), got %s", cfg.BaseURL)
	}
}

func TestLoad_MissingConfigFile(t *testing.T) {
	tmpDir := t.TempDir()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalDir)
	})

	_, err = Load("test-version")
	if err == nil {
		t.Error("expected error when config.yaml is missing")
	}
}

func TestLoad_DatasourceConfigDefaults(t *testing.T) {
	// Create a temp directory with minimal config.yaml
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
port: "3443"
env: "test"
database:
  host: "localhost"
redis:
  host: "localhost"
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalDir)
	})

	// Clear any env vars that might interfere
	os.Unsetenv("DATASOURCE_CONNECTION_TTL_MINUTES")
	os.Unsetenv("DATASOURCE_MAX_CONNECTIONS_PER_USER")
	os.Unsetenv("DATASOURCE_POOL_MAX_CONNS")
	os.Unsetenv("DATASOURCE_POOL_MIN_CONNS")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify datasource config defaults
	if cfg.Datasource.ConnectionTTLMinutes != 5 {
		t.Errorf("expected ConnectionTTLMinutes=5 (default), got %d", cfg.Datasource.ConnectionTTLMinutes)
	}
	if cfg.Datasource.MaxConnectionsPerUser != 10 {
		t.Errorf("expected MaxConnectionsPerUser=10 (default), got %d", cfg.Datasource.MaxConnectionsPerUser)
	}
	if cfg.Datasource.PoolMaxConns != 10 {
		t.Errorf("expected PoolMaxConns=10 (default), got %d", cfg.Datasource.PoolMaxConns)
	}
	if cfg.Datasource.PoolMinConns != 1 {
		t.Errorf("expected PoolMinConns=1 (default), got %d", cfg.Datasource.PoolMinConns)
	}
}

func TestLoad_DatasourceConfigFromYAML(t *testing.T) {
	// Create a temp directory with datasource config in YAML
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
port: "3443"
env: "test"
database:
  host: "localhost"
redis:
  host: "localhost"
datasource:
  connection_ttl_minutes: 10
  max_connections_per_user: 20
  pool_max_conns: 15
  pool_min_conns: 2
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalDir)
	})

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify datasource config from YAML
	if cfg.Datasource.ConnectionTTLMinutes != 10 {
		t.Errorf("expected ConnectionTTLMinutes=10 (from yaml), got %d", cfg.Datasource.ConnectionTTLMinutes)
	}
	if cfg.Datasource.MaxConnectionsPerUser != 20 {
		t.Errorf("expected MaxConnectionsPerUser=20 (from yaml), got %d", cfg.Datasource.MaxConnectionsPerUser)
	}
	if cfg.Datasource.PoolMaxConns != 15 {
		t.Errorf("expected PoolMaxConns=15 (from yaml), got %d", cfg.Datasource.PoolMaxConns)
	}
	if cfg.Datasource.PoolMinConns != 2 {
		t.Errorf("expected PoolMinConns=2 (from yaml), got %d", cfg.Datasource.PoolMinConns)
	}
}

func TestLoad_DatasourceConfigFromEnv(t *testing.T) {
	// Create a temp directory with minimal config.yaml
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	yamlContent := `
port: "3443"
env: "test"
database:
  host: "localhost"
redis:
  host: "localhost"
datasource:
  connection_ttl_minutes: 5
  max_connections_per_user: 10
  pool_max_conns: 10
  pool_min_conns: 1
`
	if err := os.WriteFile(configPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}
	t.Cleanup(func() {
		os.Chdir(originalDir)
	})

	// Set env vars to override YAML values
	t.Setenv("DATASOURCE_CONNECTION_TTL_MINUTES", "15")
	t.Setenv("DATASOURCE_MAX_CONNECTIONS_PER_USER", "25")
	t.Setenv("DATASOURCE_POOL_MAX_CONNS", "20")
	t.Setenv("DATASOURCE_POOL_MIN_CONNS", "3")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify env vars override YAML
	if cfg.Datasource.ConnectionTTLMinutes != 15 {
		t.Errorf("expected ConnectionTTLMinutes=15 (from env), got %d", cfg.Datasource.ConnectionTTLMinutes)
	}
	if cfg.Datasource.MaxConnectionsPerUser != 25 {
		t.Errorf("expected MaxConnectionsPerUser=25 (from env), got %d", cfg.Datasource.MaxConnectionsPerUser)
	}
	if cfg.Datasource.PoolMaxConns != 20 {
		t.Errorf("expected PoolMaxConns=20 (from env), got %d", cfg.Datasource.PoolMaxConns)
	}
	if cfg.Datasource.PoolMinConns != 3 {
		t.Errorf("expected PoolMinConns=3 (from env), got %d", cfg.Datasource.PoolMinConns)
	}
}
