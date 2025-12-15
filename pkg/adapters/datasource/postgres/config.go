package postgres

import "fmt"

// Config contains PostgreSQL-specific connection options.
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
	SSLMode  string // "disable", "require", "verify-ca", "verify-full"
}

// DefaultPort returns the default PostgreSQL port.
func DefaultPort() int {
	return 5432
}

// DefaultSSLMode returns the default SSL mode.
func DefaultSSLMode() string {
	return "require"
}

// FromMap creates a Config from a generic config map.
func FromMap(config map[string]any) (*Config, error) {
	cfg := &Config{
		Port:    DefaultPort(),
		SSLMode: DefaultSSLMode(),
	}

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

	if user, ok := config["user"].(string); ok {
		cfg.User = user
	} else {
		return nil, fmt.Errorf("user is required")
	}

	if password, ok := config["password"].(string); ok {
		cfg.Password = password
	}

	if database, ok := config["database"].(string); ok {
		cfg.Database = database
	} else if name, ok := config["name"].(string); ok {
		// Support legacy "name" field
		cfg.Database = name
	} else {
		return nil, fmt.Errorf("database is required")
	}

	if sslMode, ok := config["ssl_mode"].(string); ok {
		cfg.SSLMode = sslMode
	}

	return cfg, nil
}
