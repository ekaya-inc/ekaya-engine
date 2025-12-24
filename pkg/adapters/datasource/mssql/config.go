package mssql

import (
	"fmt"
)

// Config contains SQL Server-specific connection options.
type Config struct {
	Host     string
	Port     int
	Database string

	// AuthMethod determines which authentication to use
	// Options: "sql", "service_principal", "user_delegation"
	AuthMethod string

	// SQL Authentication fields
	Username string
	Password string

	// Service Principal (Azure AD) fields
	TenantID     string
	ClientID     string
	ClientSecret string

	// User Delegation - token from JWT (injected at runtime)
	AzureAccessToken string

	// Connection options
	Encrypt                bool
	TrustServerCertificate bool
	ConnectionTimeout      int
}

// DefaultPort returns the default SQL Server port.
func DefaultPort() int {
	return 1433
}

// DefaultConnectionTimeout returns the default connection timeout in seconds.
func DefaultConnectionTimeout() int {
	return 30
}

// FromMap creates a Config from a generic config map and auto-detects auth method.
func FromMap(config map[string]any) (*Config, error) {
	cfg := &Config{
		Port:              DefaultPort(),
		Encrypt:           true,
		ConnectionTimeout: DefaultConnectionTimeout(),
	}

	// Required fields
	if host, ok := config["host"].(string); ok {
		cfg.Host = host
	} else {
		return nil, fmt.Errorf("host is required")
	}

	if port, ok := config["port"].(float64); ok { // JSON numbers are float64
		cfg.Port = int(port)
	} else if port, ok := config["port"].(int); ok {
		cfg.Port = port
	}

	if database, ok := config["database"].(string); ok {
		cfg.Database = database
	} else if name, ok := config["name"].(string); ok {
		// Support legacy "name" field
		cfg.Database = name
	} else {
		return nil, fmt.Errorf("database is required")
	}

	// Optional connection settings
	if encrypt, ok := config["encrypt"].(bool); ok {
		cfg.Encrypt = encrypt
	} else if encryptStr, ok := config["encrypt"].(string); ok {
		// Support string values: "true", "false", "strict"
		cfg.Encrypt = encryptStr == "true" || encryptStr == "strict"
	}

	if trust, ok := config["trust_server_certificate"].(bool); ok {
		cfg.TrustServerCertificate = trust
	}

	if timeout, ok := config["connection_timeout"].(float64); ok {
		cfg.ConnectionTimeout = int(timeout)
	} else if timeout, ok := config["connection_timeout"].(int); ok {
		cfg.ConnectionTimeout = timeout
	}

	// Auto-detect auth method or use explicitly provided
	if authMethod, ok := config["auth_method"].(string); ok && authMethod != "" {
		cfg.AuthMethod = authMethod
	} else {
		// Auto-detect based on provided fields
		// Priority: azure_access_token > client_id > username/user (non-empty)
		if _, hasAzureToken := config["azure_access_token"].(string); hasAzureToken {
			cfg.AuthMethod = "user_delegation"
		} else if _, hasClientID := config["client_id"].(string); hasClientID {
			cfg.AuthMethod = "service_principal"
		} else if username, hasUsername := config["username"].(string); hasUsername && username != "" {
			cfg.AuthMethod = "sql"
		} else if user, hasUser := config["user"].(string); hasUser && user != "" {
			cfg.AuthMethod = "sql"
		} else {
			return nil, fmt.Errorf("could not auto-detect auth method; no credentials provided")
		}
	}

	// Parse auth-specific fields based on detected method
	switch cfg.AuthMethod {
	case "sql":
		if username, ok := config["username"].(string); ok {
			cfg.Username = username
		} else if user, ok := config["user"].(string); ok {
			cfg.Username = user
		} else {
			return nil, fmt.Errorf("username is required for SQL authentication")
		}

		if password, ok := config["password"].(string); ok {
			cfg.Password = password
		}
		// Password can be empty for some scenarios

	case "service_principal":
		if tenantID, ok := config["tenant_id"].(string); ok {
			cfg.TenantID = tenantID
		} else {
			return nil, fmt.Errorf("tenant_id is required for service principal authentication")
		}

		if clientID, ok := config["client_id"].(string); ok {
			cfg.ClientID = clientID
		} else {
			return nil, fmt.Errorf("client_id is required for service principal authentication")
		}

		if clientSecret, ok := config["client_secret"].(string); ok {
			cfg.ClientSecret = clientSecret
		} else {
			return nil, fmt.Errorf("client_secret is required for service principal authentication")
		}

	case "user_delegation":
		// No config fields required - token extracted from context at connection time

	default:
		return nil, fmt.Errorf("invalid auth method: %s (must be sql, service_principal, or user_delegation)", cfg.AuthMethod)
	}

	return cfg, nil
}

// Validate checks if the config has all required fields for the selected auth method.
func (c *Config) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host is required")
	}
	if c.Database == "" {
		return fmt.Errorf("database is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}

	switch c.AuthMethod {
	case "sql":
		if c.Username == "" {
			return fmt.Errorf("username is required for SQL authentication")
		}
	case "service_principal":
		if c.TenantID == "" {
			return fmt.Errorf("tenant_id is required for service principal")
		}
		if c.ClientID == "" {
			return fmt.Errorf("client_id is required for service principal")
		}
		if c.ClientSecret == "" {
			return fmt.Errorf("client_secret is required for service principal")
		}
	case "user_delegation":
		if c.AzureAccessToken == "" {
			return fmt.Errorf("azure_access_token is required for user delegation")
		}
	default:
		return fmt.Errorf("invalid auth method: %s", c.AuthMethod)
	}

	return nil
}
