package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"
)

// Config holds all configuration for ekaya-engine.
// Configuration can come from YAML file (config.yaml) or environment variables.
// Environment variables always override YAML values for fields that support both.
// Secrets (passwords, keys) must only come from environment variables.
type Config struct {
	// Server configuration
	BindAddr string `yaml:"bind_addr" env:"BIND_ADDR" env-default:"127.0.0.1"`
	Port     string `yaml:"port" env:"PORT" env-default:"3443"`
	Env      string `yaml:"env" env:"ENVIRONMENT" env-default:"local"`
	BaseURL  string `yaml:"base_url" env:"BASE_URL" env-default:""` // Auto-derived from Port if empty
	Version  string `yaml:"-"`                                      // Set at load time, not from config

	// TLS configuration (optional - if both provided, server uses HTTPS)
	TLSCertPath string `yaml:"tls_cert_path" env:"TLS_CERT_PATH" env-default:""`
	TLSKeyPath  string `yaml:"tls_key_path" env:"TLS_KEY_PATH" env-default:""`

	// Authentication configuration
	Auth AuthConfig `yaml:"auth"`

	// OAuth configuration
	OAuth OAuthConfig `yaml:"oauth"`

	// AuthServerURL is the OAuth authorization server base URL.
	// Used for constructing OAuth redirect URLs.
	AuthServerURL string `yaml:"auth_server_url" env:"AUTH_SERVER_URL" env-default:""`

	// CookieDomain is the domain for auth cookies (optional).
	// If empty, it will be auto-derived from BaseURL.
	CookieDomain string `yaml:"cookie_domain" env:"COOKIE_DOMAIN" env-default:""`

	// Database configuration (PostgreSQL)
	Database DatabaseConfig `yaml:"database"`

	// Datasource connection management configuration
	Datasource DatasourceConfig `yaml:"datasource"`

	// Pre-configured AI model endpoints (server-level)
	CommunityAI CommunityAIConfig `yaml:"community_ai"`
	EmbeddedAI  EmbeddedAIConfig  `yaml:"embedded_ai"`

	// Credential encryption key for project secrets (AI keys, database passwords, etc.)
	// Must be a 32-byte key, base64 encoded. Generate with: openssl rand -base64 32
	// Server will fail to start if this is not set.
	ProjectCredentialsKey string `yaml:"-" env:"PROJECT_CREDENTIALS_KEY"` // Secret - not in YAML
}

// OAuthConfig holds OAuth client configuration.
type OAuthConfig struct {
	// ClientID is the OAuth client ID registered with the auth server.
	ClientID string `yaml:"client_id" env:"OAUTH_CLIENT_ID" env-default:"ekaya-engine"`
}

// AuthConfig holds authentication-related configuration.
type AuthConfig struct {
	// EnableVerification controls whether JWT tokens are validated.
	// Set to false for local development without auth server.
	EnableVerification bool `yaml:"enable_verification" env:"AUTH_ENABLE_VERIFICATION" env-default:"true"`

	// JWKSEndpointsStr is a comma-separated list of issuer=jwks_url pairs.
	// Format: "issuer1=url1,issuer2=url2"
	JWKSEndpointsStr string `yaml:"jwks_endpoints" env:"JWKS_ENDPOINTS" env-default:"https://auth.ekaya.ai=https://auth.ekaya.ai/.well-known/jwks.json"`

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

// DatasourceConfig holds datasource connection management settings.
type DatasourceConfig struct {
	// ConnectionTTLMinutes is how long idle datasource connections are kept alive.
	ConnectionTTLMinutes int `yaml:"connection_ttl_minutes" env:"DATASOURCE_CONNECTION_TTL_MINUTES" env-default:"5"`
	// MaxConnectionsPerUser limits concurrent datasource connections per user.
	MaxConnectionsPerUser int `yaml:"max_connections_per_user" env:"DATASOURCE_MAX_CONNECTIONS_PER_USER" env-default:"10"`
	// PoolMaxConns is the maximum number of connections per datasource pool.
	PoolMaxConns int32 `yaml:"pool_max_conns" env:"DATASOURCE_POOL_MAX_CONNS" env-default:"10"`
	// PoolMinConns is the minimum number of connections per datasource pool.
	PoolMinConns int32 `yaml:"pool_min_conns" env:"DATASOURCE_POOL_MIN_CONNS" env-default:"1"`
}

// CommunityAIConfig holds endpoints for free community AI models.
// These are server-level settings that projects can opt into.
type CommunityAIConfig struct {
	LLMBaseURL     string `yaml:"llm_base_url" env:"COMMUNITY_AI_LLM_BASE_URL" env-default:""`
	LLMModel       string `yaml:"llm_model" env:"COMMUNITY_AI_LLM_MODEL" env-default:""`
	EmbeddingURL   string `yaml:"embedding_url" env:"COMMUNITY_AI_EMBEDDING_URL" env-default:""`
	EmbeddingModel string `yaml:"embedding_model" env:"COMMUNITY_AI_EMBEDDING_MODEL" env-default:""`
}

// IsAvailable returns true if community AI is configured.
func (c *CommunityAIConfig) IsAvailable() bool {
	return c.LLMBaseURL != "" && c.LLMModel != ""
}

// EmbeddedAIConfig holds endpoints for licensed embedded AI models.
// These are server-level settings that projects can opt into.
type EmbeddedAIConfig struct {
	LLMBaseURL     string `yaml:"llm_base_url" env:"EMBEDDED_AI_LLM_BASE_URL" env-default:""`
	LLMModel       string `yaml:"llm_model" env:"EMBEDDED_AI_LLM_MODEL" env-default:""`
	EmbeddingURL   string `yaml:"embedding_url" env:"EMBEDDED_AI_EMBEDDING_URL" env-default:""`
	EmbeddingModel string `yaml:"embedding_model" env:"EMBEDDED_AI_EMBEDDING_MODEL" env-default:""`
}

// IsAvailable returns true if embedded AI is configured.
func (c *EmbeddedAIConfig) IsAvailable() bool {
	return c.LLMBaseURL != "" && c.LLMModel != ""
}

// Load reads configuration from config.yaml with environment variable overrides.
// The version parameter is injected at build time and set on the returned Config.
// Environment variables override YAML values. Secrets (PGPASSWORD,
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

	// Validate TLS configuration
	if err := cfg.validateTLS(); err != nil {
		return nil, fmt.Errorf("invalid TLS configuration: %w", err)
	}

	// Auto-derive BaseURL from Port if not explicitly set
	// Use HTTPS scheme if TLS is configured
	if cfg.BaseURL == "" {
		scheme := "http"
		if cfg.TLSCertPath != "" {
			scheme = "https"
		}
		cfg.BaseURL = (&url.URL{
			Scheme: scheme,
			Host:   "localhost:" + cfg.Port,
		}).String()
	}

	return cfg, nil
}

// parseComplexFields handles fields that need post-processing after loading.
func (c *Config) parseComplexFields() error {
	// Parse JWKS endpoints from string to map
	c.Auth.JWKSEndpoints = parseJWKSEndpoints(c.Auth.JWKSEndpointsStr)
	return nil
}

// validateTLS ensures TLS configuration is valid if provided.
// Both cert and key must be provided together, and files must exist and be readable.
func (c *Config) validateTLS() error {
	certSet := c.TLSCertPath != ""
	keySet := c.TLSKeyPath != ""

	// Both must be provided together or both empty
	if certSet != keySet {
		return fmt.Errorf("both tls_cert_path and tls_key_path must be provided together")
	}

	// If both provided, verify files exist (actual readability checked by tls.LoadX509KeyPair at startup)
	if certSet {
		if _, err := os.Stat(c.TLSCertPath); err != nil {
			return fmt.Errorf("TLS cert file does not exist: %w", err)
		}
		if _, err := os.Stat(c.TLSKeyPath); err != nil {
			return fmt.Errorf("TLS key file does not exist: %w", err)
		}
	}

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

// ValidateAuthURL validates an auth_url against the JWKS endpoints whitelist.
// Returns the validated auth URL and an empty error string on success.
// If authURL is empty, returns the default AuthServerURL.
// If authURL is provided but not in the whitelist, returns empty string and error message.
func (c *Config) ValidateAuthURL(authURL string) (string, string) {
	// If no auth_url provided, use default
	if authURL == "" {
		return c.AuthServerURL, ""
	}

	// Check if auth_url is in the JWKS endpoints whitelist
	if _, ok := c.Auth.JWKSEndpoints[authURL]; ok {
		return authURL, ""
	}

	// auth_url provided but not in whitelist - reject
	return "", "auth_url not in allowed list"
}
