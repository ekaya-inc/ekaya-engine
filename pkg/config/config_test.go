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
base_url: "http://localhost:3443"
region_domain: "yaml-region.example.com"
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

	// Clear env vars that might interfere, then set test values
	// (unset first, since empty string counts as "set" for cleanenv)
	os.Unsetenv("REGION_DOMAIN")
	t.Cleanup(func() { os.Unsetenv("REGION_DOMAIN") })

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

	// Verify YAML value used for field without env override
	// (yaml-region.example.com differs from env-default "localhost", proving YAML was read)
	if cfg.RegionDomain != "yaml-region.example.com" {
		t.Errorf("expected RegionDomain=yaml-region.example.com (from yaml), got %s", cfg.RegionDomain)
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
