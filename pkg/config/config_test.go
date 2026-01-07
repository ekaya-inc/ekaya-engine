package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// TLS Configuration Tests

func TestLoad_NoTLS(t *testing.T) {
	// Create a temp directory with config.yaml that has no TLS settings
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

	// Clear TLS env vars
	os.Unsetenv("TLS_CERT_PATH")
	os.Unsetenv("TLS_KEY_PATH")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify TLS fields are empty
	if cfg.TLSCertPath != "" {
		t.Errorf("expected empty TLSCertPath, got %s", cfg.TLSCertPath)
	}
	if cfg.TLSKeyPath != "" {
		t.Errorf("expected empty TLSKeyPath, got %s", cfg.TLSKeyPath)
	}
}

func TestValidateTLS_BothProvided(t *testing.T) {
	// Create a temp directory with valid cert and key files
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	certPath := filepath.Join(tmpDir, "test-cert.pem")
	keyPath := filepath.Join(tmpDir, "test-key.pem")

	// Create dummy cert and key files
	if err := os.WriteFile(certPath, []byte("fake-cert-content"), 0644); err != nil {
		t.Fatalf("failed to write test cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("fake-key-content"), 0644); err != nil {
		t.Fatalf("failed to write test key: %v", err)
	}

	yamlContent := fmt.Sprintf(`
port: "3443"
env: "test"
tls_cert_path: "%s"
tls_key_path: "%s"
database:
  host: "localhost"
redis:
  host: "localhost"
`, certPath, keyPath)
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

	// Verify TLS paths are set correctly
	if cfg.TLSCertPath != certPath {
		t.Errorf("expected TLSCertPath=%s, got %s", certPath, cfg.TLSCertPath)
	}
	if cfg.TLSKeyPath != keyPath {
		t.Errorf("expected TLSKeyPath=%s, got %s", keyPath, cfg.TLSKeyPath)
	}
}

func TestValidateTLS_OnlyCertProvided(t *testing.T) {
	// Create a temp directory with only cert file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	certPath := filepath.Join(tmpDir, "test-cert.pem")

	// Create dummy cert file
	if err := os.WriteFile(certPath, []byte("fake-cert-content"), 0644); err != nil {
		t.Fatalf("failed to write test cert: %v", err)
	}

	yamlContent := fmt.Sprintf(`
port: "3443"
env: "test"
tls_cert_path: "%s"
database:
  host: "localhost"
redis:
  host: "localhost"
`, certPath)
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

	_, err = Load("test-version")
	if err == nil {
		t.Fatal("expected error when only cert provided, got nil")
	}

	// Verify error message mentions both must be provided
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("expected error to mention 'both', got: %v", err)
	}
}

func TestValidateTLS_OnlyKeyProvided(t *testing.T) {
	// Create a temp directory with only key file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	keyPath := filepath.Join(tmpDir, "test-key.pem")

	// Create dummy key file
	if err := os.WriteFile(keyPath, []byte("fake-key-content"), 0644); err != nil {
		t.Fatalf("failed to write test key: %v", err)
	}

	yamlContent := fmt.Sprintf(`
port: "3443"
env: "test"
tls_key_path: "%s"
database:
  host: "localhost"
redis:
  host: "localhost"
`, keyPath)
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

	_, err = Load("test-version")
	if err == nil {
		t.Fatal("expected error when only key provided, got nil")
	}

	// Verify error message mentions both must be provided
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("expected error to mention 'both', got: %v", err)
	}
}

func TestValidateTLS_CertFileNotFound(t *testing.T) {
	// Create a temp directory with config that references non-existent cert
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	certPath := filepath.Join(tmpDir, "nonexistent-cert.pem")
	keyPath := filepath.Join(tmpDir, "test-key.pem")

	// Create only the key file (cert file intentionally missing)
	if err := os.WriteFile(keyPath, []byte("fake-key-content"), 0644); err != nil {
		t.Fatalf("failed to write test key: %v", err)
	}

	yamlContent := fmt.Sprintf(`
port: "3443"
env: "test"
tls_cert_path: "%s"
tls_key_path: "%s"
database:
  host: "localhost"
redis:
  host: "localhost"
`, certPath, keyPath)
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

	_, err = Load("test-version")
	if err == nil {
		t.Fatal("expected error when cert file not found, got nil")
	}

	// Verify error message mentions cert file
	if !strings.Contains(err.Error(), "cert") {
		t.Errorf("expected error to mention 'cert', got: %v", err)
	}
}

func TestValidateTLS_KeyFileNotFound(t *testing.T) {
	// Create a temp directory with config that references non-existent key
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	certPath := filepath.Join(tmpDir, "test-cert.pem")
	keyPath := filepath.Join(tmpDir, "nonexistent-key.pem")

	// Create only the cert file (key file intentionally missing)
	if err := os.WriteFile(certPath, []byte("fake-cert-content"), 0644); err != nil {
		t.Fatalf("failed to write test cert: %v", err)
	}

	yamlContent := fmt.Sprintf(`
port: "3443"
env: "test"
tls_cert_path: "%s"
tls_key_path: "%s"
database:
  host: "localhost"
redis:
  host: "localhost"
`, certPath, keyPath)
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

	_, err = Load("test-version")
	if err == nil {
		t.Fatal("expected error when key file not found, got nil")
	}

	// Verify error message mentions key file
	if !strings.Contains(err.Error(), "key") {
		t.Errorf("expected error to mention 'key', got: %v", err)
	}
}

// Note: We don't test unreadable files (e.g., files with 0000 permissions) because:
// 1. os.Stat() succeeds even on unreadable files (it only checks metadata)
// 2. Actual readability errors will be caught by tls.LoadX509KeyPair() at server startup
// 3. Testing true read permissions would require OS-specific setups that are fragile
// The file existence checks (tested above) are sufficient for config validation.

func TestValidateTLS_TLSFromEnv(t *testing.T) {
	// Test that TLS config can come from environment variables
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	certPath := filepath.Join(tmpDir, "test-cert.pem")
	keyPath := filepath.Join(tmpDir, "test-key.pem")

	// Create cert and key files
	if err := os.WriteFile(certPath, []byte("fake-cert-content"), 0644); err != nil {
		t.Fatalf("failed to write test cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte("fake-key-content"), 0644); err != nil {
		t.Fatalf("failed to write test key: %v", err)
	}

	// Create minimal config without TLS in YAML
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

	// Set TLS paths via environment variables
	t.Setenv("TLS_CERT_PATH", certPath)
	t.Setenv("TLS_KEY_PATH", keyPath)

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify TLS paths from env
	if cfg.TLSCertPath != certPath {
		t.Errorf("expected TLSCertPath=%s (from env), got %s", certPath, cfg.TLSCertPath)
	}
	if cfg.TLSKeyPath != keyPath {
		t.Errorf("expected TLSKeyPath=%s (from env), got %s", keyPath, cfg.TLSKeyPath)
	}
}
