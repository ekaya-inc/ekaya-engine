package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupConfigTest creates config.yaml in a temp directory and changes to it.
// If dir is empty, creates a new temp directory. Returns the directory path.
// Cleanup is registered automatically.
func setupConfigTest(t *testing.T, yamlContent string, dir ...string) string {
	t.Helper()
	var tmpDir string
	if len(dir) > 0 && dir[0] != "" {
		tmpDir = dir[0]
	} else {
		tmpDir = t.TempDir()
	}
	configPath := filepath.Join(tmpDir, "config.yaml")

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

	return tmpDir
}

// tlsTestFiles holds paths to TLS test files created by setupTLSFiles.
type tlsTestFiles struct {
	CertPath string
	KeyPath  string
}

// setupTLSFiles creates dummy cert and/or key files in the given directory.
// Pass empty string to skip creating that file.
func setupTLSFiles(t *testing.T, dir string, createCert, createKey bool) tlsTestFiles {
	t.Helper()
	files := tlsTestFiles{
		CertPath: filepath.Join(dir, "test-cert.pem"),
		KeyPath:  filepath.Join(dir, "test-key.pem"),
	}

	if createCert {
		if err := os.WriteFile(files.CertPath, []byte("fake-cert-content"), 0644); err != nil {
			t.Fatalf("failed to write test cert: %v", err)
		}
	}
	if createKey {
		if err := os.WriteFile(files.KeyPath, []byte("fake-key-content"), 0644); err != nil {
			t.Fatalf("failed to write test key: %v", err)
		}
	}

	return files
}

func TestLoad_EnvOverridesYAML(t *testing.T) {
	setupConfigTest(t, `
port: "3443"
env: "test"
engine_database:
  pg_host: "db.example.com"
  pg_port: 5432
  pg_user: "testuser"
  pg_database: "testdb"
`)

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
	if cfg.EngineDatabase.Host != "db.example.com" {
		t.Errorf("expected EngineDatabase.Host=db.example.com (from yaml), got %s", cfg.EngineDatabase.Host)
	}
}

func TestLoad_BaseURLAutoDerive(t *testing.T) {
	setupConfigTest(t, `
port: "5678"
env: "test"
engine_database:
  pg_host: "localhost"
`)

	// Clear env vars to test auto-derivation from YAML
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
	setupConfigTest(t, `
port: "3443"
env: "test"
base_url: "http://my-server.internal:8080"
engine_database:
  pg_host: "localhost"
`)

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

func TestLoad_BaseURLAutoDeriveTLS(t *testing.T) {
	// Create TLS files first (need paths for config)
	tmpDir := t.TempDir()
	tls := setupTLSFiles(t, tmpDir, true, true)

	setupConfigTest(t, fmt.Sprintf(`
port: "8443"
env: "test"
tls_cert_path: "%s"
tls_key_path: "%s"
engine_database:
  pg_host: "localhost"
`, tls.CertPath, tls.KeyPath), tmpDir)

	// Clear env vars to test auto-derivation from YAML
	os.Unsetenv("BASE_URL")
	os.Unsetenv("PORT")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify BaseURL uses HTTPS when TLS is configured
	if cfg.BaseURL != "https://localhost:8443" {
		t.Errorf("expected BaseURL=https://localhost:8443 (auto-derived with TLS), got %s", cfg.BaseURL)
	}
}

func TestLoad_MissingConfigFile(t *testing.T) {
	tmpDir := t.TempDir()
	originalDir, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	t.Cleanup(func() { os.Chdir(originalDir) })

	// Override HOME so the fallback doesn't find a real ~/.ekaya/config.yaml
	t.Setenv("HOME", tmpDir)

	_, err := Load("test-version")
	if err == nil {
		t.Error("expected error when config.yaml is missing")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestLoad_FallbackToHomeDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create ~/.ekaya/config.yaml in a fake home directory
	ekayaDir := filepath.Join(tmpDir, ".ekaya")
	if err := os.MkdirAll(ekayaDir, 0755); err != nil {
		t.Fatalf("failed to create .ekaya dir: %v", err)
	}
	configContent := `
port: "9999"
env: "test"
engine_database:
  pg_host: "home-host"
`
	if err := os.WriteFile(filepath.Join(ekayaDir, "config.yaml"), []byte(configContent), 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// CWD has no config.yaml
	cwdDir := t.TempDir()
	originalDir, _ := os.Getwd()
	_ = os.Chdir(cwdDir)
	t.Cleanup(func() { os.Chdir(originalDir) })

	// Point HOME to our fake home
	t.Setenv("HOME", tmpDir)
	os.Unsetenv("PORT")
	os.Unsetenv("PGHOST")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Port != "9999" {
		t.Errorf("expected Port=9999 (from ~/.ekaya/config.yaml), got %s", cfg.Port)
	}
	if cfg.EngineDatabase.Host != "home-host" {
		t.Errorf("expected Host=home-host (from ~/.ekaya/config.yaml), got %s", cfg.EngineDatabase.Host)
	}
}

func TestLoad_CWDTakesPrecedenceOverHomeDir(t *testing.T) {
	tmpDir := t.TempDir()

	// Create ~/.ekaya/config.yaml in a fake home directory
	ekayaDir := filepath.Join(tmpDir, ".ekaya")
	if err := os.MkdirAll(ekayaDir, 0755); err != nil {
		t.Fatalf("failed to create .ekaya dir: %v", err)
	}
	homeConfig := `
port: "1111"
env: "test"
engine_database:
  pg_host: "home-host"
`
	if err := os.WriteFile(filepath.Join(ekayaDir, "config.yaml"), []byte(homeConfig), 0644); err != nil {
		t.Fatalf("failed to write home config: %v", err)
	}

	// CWD config.yaml takes precedence
	cwdConfig := `
port: "2222"
env: "test"
engine_database:
  pg_host: "cwd-host"
`
	cwdDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwdDir, "config.yaml"), []byte(cwdConfig), 0644); err != nil {
		t.Fatalf("failed to write cwd config: %v", err)
	}

	originalDir, _ := os.Getwd()
	_ = os.Chdir(cwdDir)
	t.Cleanup(func() { os.Chdir(originalDir) })

	t.Setenv("HOME", tmpDir)
	os.Unsetenv("PORT")
	os.Unsetenv("PGHOST")

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	if cfg.Port != "2222" {
		t.Errorf("expected Port=2222 (from CWD config.yaml), got %s", cfg.Port)
	}
	if cfg.EngineDatabase.Host != "cwd-host" {
		t.Errorf("expected Host=cwd-host (from CWD config.yaml), got %s", cfg.EngineDatabase.Host)
	}
}

func TestLoad_DatasourceConfigDefaults(t *testing.T) {
	setupConfigTest(t, `
port: "3443"
env: "test"
engine_database:
  pg_host: "localhost"
`)

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
	setupConfigTest(t, `
port: "3443"
env: "test"
engine_database:
  pg_host: "localhost"
datasource:
  connection_ttl_minutes: 10
  max_connections_per_user: 20
  pool_max_conns: 15
  pool_min_conns: 2
`)

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
	setupConfigTest(t, `
port: "3443"
env: "test"
engine_database:
  pg_host: "localhost"
datasource:
  connection_ttl_minutes: 5
  max_connections_per_user: 10
  pool_max_conns: 10
  pool_min_conns: 1
`)

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
	setupConfigTest(t, `
port: "3443"
env: "test"
engine_database:
  pg_host: "localhost"
`)

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
	tmpDir := t.TempDir()
	tls := setupTLSFiles(t, tmpDir, true, true)

	setupConfigTest(t, fmt.Sprintf(`
port: "3443"
env: "test"
tls_cert_path: "%s"
tls_key_path: "%s"
engine_database:
  pg_host: "localhost"
`, tls.CertPath, tls.KeyPath), tmpDir)

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify TLS paths are set correctly
	if cfg.TLSCertPath != tls.CertPath {
		t.Errorf("expected TLSCertPath=%s, got %s", tls.CertPath, cfg.TLSCertPath)
	}
	if cfg.TLSKeyPath != tls.KeyPath {
		t.Errorf("expected TLSKeyPath=%s, got %s", tls.KeyPath, cfg.TLSKeyPath)
	}
}

func TestValidateTLS_OnlyCertProvided(t *testing.T) {
	tmpDir := t.TempDir()
	tls := setupTLSFiles(t, tmpDir, true, false) // cert only

	setupConfigTest(t, fmt.Sprintf(`
port: "3443"
env: "test"
tls_cert_path: "%s"
engine_database:
  pg_host: "localhost"
`, tls.CertPath), tmpDir)

	_, err := Load("test-version")
	if err == nil {
		t.Fatal("expected error when only cert provided, got nil")
	}

	// Verify error message mentions both must be provided
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("expected error to mention 'both', got: %v", err)
	}
}

func TestValidateTLS_OnlyKeyProvided(t *testing.T) {
	tmpDir := t.TempDir()
	tls := setupTLSFiles(t, tmpDir, false, true) // key only

	setupConfigTest(t, fmt.Sprintf(`
port: "3443"
env: "test"
tls_key_path: "%s"
engine_database:
  pg_host: "localhost"
`, tls.KeyPath), tmpDir)

	_, err := Load("test-version")
	if err == nil {
		t.Fatal("expected error when only key provided, got nil")
	}

	// Verify error message mentions both must be provided
	if !strings.Contains(err.Error(), "both") {
		t.Errorf("expected error to mention 'both', got: %v", err)
	}
}

func TestValidateTLS_CertFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tls := setupTLSFiles(t, tmpDir, false, true) // key only, cert missing
	nonexistentCert := filepath.Join(tmpDir, "nonexistent-cert.pem")

	setupConfigTest(t, fmt.Sprintf(`
port: "3443"
env: "test"
tls_cert_path: "%s"
tls_key_path: "%s"
engine_database:
  pg_host: "localhost"
`, nonexistentCert, tls.KeyPath), tmpDir)

	_, err := Load("test-version")
	if err == nil {
		t.Fatal("expected error when cert file not found, got nil")
	}

	// Verify error message mentions cert file does not exist
	if !strings.Contains(err.Error(), "cert file does not exist") {
		t.Errorf("expected error to mention 'cert file does not exist', got: %v", err)
	}
}

func TestValidateTLS_KeyFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	tls := setupTLSFiles(t, tmpDir, true, false) // cert only, key missing
	nonexistentKey := filepath.Join(tmpDir, "nonexistent-key.pem")

	setupConfigTest(t, fmt.Sprintf(`
port: "3443"
env: "test"
tls_cert_path: "%s"
tls_key_path: "%s"
engine_database:
  pg_host: "localhost"
`, tls.CertPath, nonexistentKey), tmpDir)

	_, err := Load("test-version")
	if err == nil {
		t.Fatal("expected error when key file not found, got nil")
	}

	// Verify error message mentions key file does not exist
	if !strings.Contains(err.Error(), "key file does not exist") {
		t.Errorf("expected error to mention 'key file does not exist', got: %v", err)
	}
}

// Note: We don't test unreadable files (e.g., files with 0000 permissions) because:
// 1. os.Stat() succeeds even on unreadable files (it only checks metadata)
// 2. Actual readability errors will be caught by tls.LoadX509KeyPair() at server startup
// 3. Testing true read permissions would require OS-specific setups that are fragile
// The file existence checks (tested above) are sufficient for config validation.

func TestValidateTLS_TLSFromEnv(t *testing.T) {
	tmpDir := t.TempDir()
	tls := setupTLSFiles(t, tmpDir, true, true)

	setupConfigTest(t, `
port: "3443"
env: "test"
engine_database:
  pg_host: "localhost"
`, tmpDir)

	// Set TLS paths via environment variables
	t.Setenv("TLS_CERT_PATH", tls.CertPath)
	t.Setenv("TLS_KEY_PATH", tls.KeyPath)

	cfg, err := Load("test-version")
	if err != nil {
		t.Fatalf("Load() failed: %v", err)
	}

	// Verify TLS paths from env
	if cfg.TLSCertPath != tls.CertPath {
		t.Errorf("expected TLSCertPath=%s (from env), got %s", tls.CertPath, cfg.TLSCertPath)
	}
	if cfg.TLSKeyPath != tls.KeyPath {
		t.Errorf("expected TLSKeyPath=%s (from env), got %s", tls.KeyPath, cfg.TLSKeyPath)
	}
}
