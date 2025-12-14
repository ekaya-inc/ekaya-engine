package config

import (
	"fmt"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config holds all configuration for ekaya-engine.
// Configuration can come from YAML file (config.yaml) or environment variables.
// Environment variables always override YAML values for fields that support both.
// Secrets (passwords, keys) must only come from environment variables.
type Config struct {
	// Server configuration
	Port         string `yaml:"port" env:"PORT" env-default:"3443"`
	Env          string `yaml:"env" env:"ENVIRONMENT" env-default:"local"`
	BaseURL      string `yaml:"base_url" env:"BASE_URL" env-default:"http://localhost:3443"`
	RegionDomain string `yaml:"region_domain" env:"REGION_DOMAIN" env-default:"localhost"`
	Version      string `yaml:"-"` // Set at load time, not from config

	// Authentication configuration
	Auth AuthConfig `yaml:"auth"`

	// Database configuration (PostgreSQL)
	Database DatabaseConfig `yaml:"database"`

	// Redis configuration
	Redis RedisConfig `yaml:"redis"`

	// Credential encryption key for project secrets (AI keys, database passwords, etc.)
	// Must be a 32-byte key, base64 encoded. Generate with: openssl rand -base64 32
	// Server will fail to start if this is not set.
	ProjectCredentialsKey string `yaml:"-" env:"PROJECT_CREDENTIALS_KEY"` // Secret - not in YAML
}

// AuthConfig holds authentication-related configuration.
type AuthConfig struct {
	// EnableVerification controls whether JWT tokens are validated.
	// Set to false for local development without auth server.
	EnableVerification bool `yaml:"enable_verification" env:"AUTH_ENABLE_VERIFICATION" env-default:"true"`

	// JWKSEndpointsStr is a comma-separated list of issuer=jwks_url pairs.
	// Format: "issuer1=url1,issuer2=url2"
	JWKSEndpointsStr string `yaml:"jwks_endpoints" env:"JWKS_ENDPOINTS" env-default:""`

	// JWKSEndpoints is the parsed map from JWKSEndpointsStr (not from config file).
	JWKSEndpoints map[string]string `yaml:"-"`
}

// DatabaseConfig holds PostgreSQL database configuration.
type DatabaseConfig struct {
	Host           string `yaml:"host" env:"PGHOST" env-default:"localhost"`
	Port           int    `yaml:"port" env:"PGPORT" env-default:"5432"`
	User           string `yaml:"user" env:"PGUSER" env-default:"ekaya"`
	Password       string `yaml:"-" env:"PGPASSWORD"` // Secret - not in YAML
	Database       string `yaml:"database" env:"PGDATABASE" env-default:"ekaya_engine"`
	MaxConnections int32  `yaml:"max_connections" env:"PGMAX_CONNECTIONS" env-default:"25"`
	MaxIdleConns   int32  `yaml:"max_idle_conns" env:"PGMAX_IDLE_CONNS" env-default:"5"`
	Type           string `yaml:"type" env:"PGTYPE" env-default:"postgres"`
	SSLMode        string `yaml:"ssl_mode" env:"PGSSLMODE" env-default:"disable"`
}

// RedisConfig holds Redis configuration.
type RedisConfig struct {
	Host      string `yaml:"host" env:"REDIS_HOST" env-default:"localhost"`
	Port      int    `yaml:"port" env:"REDIS_PORT" env-default:"6379"`
	DB        int    `yaml:"db" env:"REDIS_DB" env-default:"0"`
	Password  string `yaml:"-" env:"REDIS_PASSWORD" env-default:""` // Secret - not in YAML
	KeyPrefix string `yaml:"key_prefix" env:"REDIS_KEY_PREFIX" env-default:"project:"`
}

// Load reads configuration from config.yaml with environment variable overrides.
// The version parameter is injected at build time and set on the returned Config.
// Environment variables override YAML values. Secrets (PGPASSWORD, REDIS_PASSWORD,
// PROJECT_CREDENTIALS_KEY) must come from environment variables (yaml:"-" fields).
func Load(version string) (*Config, error) {
	cfg := &Config{
		Version: version,
	}

	// Load config from YAML file with environment variable overrides
	if err := cleanenv.ReadConfig("config.yaml", cfg); err != nil {
		return nil, fmt.Errorf("failed to read config.yaml: %w", err)
	}

	// Parse complex fields
	if err := cfg.parseComplexFields(); err != nil {
		return nil, fmt.Errorf("failed to parse config fields: %w", err)
	}

	return cfg, nil
}

// parseComplexFields handles fields that need post-processing after loading.
func (c *Config) parseComplexFields() error {
	// Parse JWKS endpoints from string to map
	c.Auth.JWKSEndpoints = parseJWKSEndpoints(c.Auth.JWKSEndpointsStr)
	return nil
}

// parseJWKSEndpoints parses the JWKS endpoints string into a map.
// Format: "issuer1=url1,issuer2=url2"
func parseJWKSEndpoints(value string) map[string]string {
	endpoints := make(map[string]string)
	if value == "" {
		return endpoints
	}

	pairs := strings.Split(value, ",")
	for _, pair := range pairs {
		parts := strings.Split(pair, "=")
		if len(parts) == 2 {
			endpoints[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return endpoints
}

// ConnectionString returns a PostgreSQL connection string.
func (c *DatabaseConfig) ConnectionString() string {
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Database, c.SSLMode,
	)
}

// RedisAddr returns the Redis address in host:port format.
func (c *RedisConfig) RedisAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
