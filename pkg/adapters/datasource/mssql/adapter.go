package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"

	_ "github.com/microsoft/go-mssqldb"         // SQL Server driver
	_ "github.com/microsoft/go-mssqldb/azuread" // Azure AD support

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// Adapter provides SQL Server connectivity with support for multiple authentication methods.
type Adapter struct {
	config       *Config
	db           *sql.DB
	connMgr      *datasource.ConnectionManager
	projectID    uuid.UUID
	userID       string
	datasourceID uuid.UUID
	ownedDB      bool // true if we created the DB (for tests or TestConnection case)
}

// extractAndSetAzureToken extracts the Azure access token from context and sets it in the config.
// It also validates token expiry if available in claims, and automatically refreshes expired tokens.
// This mirrors how JWT expiration is handled automatically by the jwt-go library.
// This function is idempotent - if token is already set in config, it does nothing.
func extractAndSetAzureToken(ctx context.Context, cfg *Config) error {
	// If token is already set, skip extraction (idempotent)
	if cfg.AzureAccessToken != "" {
		return nil
	}

	azureToken, hasToken := auth.GetAzureAccessToken(ctx)
	if !hasToken || azureToken == "" {
		return fmt.Errorf("user_delegation auth requires Azure access token in request context")
	}

	// Check token expiry if available in claims
	claims, hasClaims := auth.GetClaims(ctx)
	if hasClaims && claims != nil {
		if claims.AzureTokenExpiry > 0 {
			expiry := time.Unix(claims.AzureTokenExpiry, 0)
			now := time.Now()
			if expiry.Before(now) {
				// Token expired - automatically refresh it (same pattern as JWT expiration handling)
				refreshedToken, refreshedExp, err := auth.RefreshAzureToken(ctx)
				if err != nil {
					return fmt.Errorf("Azure access token expired and refresh failed: %w", err)
				}
				// Use the refreshed token
				cfg.AzureAccessToken = refreshedToken
				// Update claims with new expiry for future checks (optional, but helpful)
				claims.AzureTokenExpiry = refreshedExp
				return nil
			}
			// Warn if token expires within 5 minutes
			if expiry.Before(now.Add(5 * time.Minute)) {
				// Log warning but don't fail - token is still valid
				// The connection will fail later with a clearer error if token actually expires during connection
			}
		}
		// If azure_exp is not set in claims, we can't validate expiry
		// The connection attempt will reveal if token is expired
	}
	// If claims are not available, proceed with connection attempt
	// Azure SQL Database will return an error if token is expired

	cfg.AzureAccessToken = azureToken
	return nil
}

// NewAdapter creates a SQL Server adapter with the given config.
// Supports three authentication methods:
//  1. SQL Authentication (username/password)
//  2. Service Principal (Azure AD with client credentials)
//  3. User Delegation (Azure AD with access token from JWT)
//
// Uses connection manager for connection pooling when provided.
func NewAdapter(ctx context.Context, cfg *Config, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (*Adapter, error) {
	// Extract Azure token from context for user_delegation if not already set
	// This makes NewAdapter idempotent - it can be called after token extraction
	if cfg.AuthMethod == "user_delegation" {
		if err := extractAndSetAzureToken(ctx, cfg); err != nil {
			return nil, err
		}
		cfg.AzureAccessToken = azureToken
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	var db *sql.DB
	var err error

	if connMgr == nil {
		// Fallback for direct instantiation (tests, TestConnection)
		// Create connection directly without pooling
		switch cfg.AuthMethod {
		case "sql":
			db, err = createSQLAuthConnection(cfg)
		case "service_principal":
			db, err = createServicePrincipalConnection(cfg)
		case "user_delegation":
			db, err = createUserDelegationConnection(cfg)
		default:
			return nil, fmt.Errorf("unsupported auth method: %s", cfg.AuthMethod)
		}

		if err != nil {
			return nil, fmt.Errorf("create connection: %w", err)
		}

		// Test the connection immediately
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, fmt.Errorf("connection test failed: %w", err)
		}

		return &Adapter{
			config:  cfg,
			db:      db,
			ownedDB: true,
		}, nil
	}

	// Use connection manager for reusable pool
	// For MSSQL, we need to create the connection first (due to auth complexity),
	// then wrap and register it
	switch cfg.AuthMethod {
	case "sql":
		db, err = createSQLAuthConnection(cfg)
	case "service_principal":
		db, err = createServicePrincipalConnection(cfg)
	case "user_delegation":
		db, err = createUserDelegationConnection(cfg)
	default:
		return nil, fmt.Errorf("unsupported auth method: %s", cfg.AuthMethod)
	}

	if err != nil {
		return nil, fmt.Errorf("create connection: %w", err)
	}

	// Wrap the connection and register with connection manager
	wrapper := datasource.NewMSSQLPoolWrapper(db)
	connector, err := connMgr.RegisterConnection(ctx, projectID, userID, datasourceID, wrapper)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to register connection: %w", err)
	}

	// Extract underlying DB from connector (should be the same wrapper we registered)
	mssqlDB, err := datasource.GetMSSQLDB(connector)
	if err != nil {
		return nil, fmt.Errorf("failed to extract mssql db: %w", err)
	}

	// Test the connection immediately
	if err := mssqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connection test failed: %w", err)
	}

	return &Adapter{
		config:       cfg,
		db:           mssqlDB,
		connMgr:      connMgr,
		projectID:    projectID,
		userID:       userID,
		datasourceID: datasourceID,
		ownedDB:      false,
	}, nil
}

// createSQLAuthConnection creates a connection using SQL Server authentication.
func createSQLAuthConnection(cfg *Config) (*sql.DB, error) {
	query := url.Values{}
	query.Add("database", cfg.Database)

	if cfg.Encrypt {
		query.Add("encrypt", "true")
	} else {
		query.Add("encrypt", "false")
	}

	if cfg.TrustServerCertificate {
		query.Add("TrustServerCertificate", "true")
	}

	if cfg.ConnectionTimeout > 0 {
		query.Add("connection timeout", fmt.Sprintf("%d", cfg.ConnectionTimeout))
	}

	connStr := fmt.Sprintf("sqlserver://%s:%s@%s:%d?%s",
		url.QueryEscape(cfg.Username),
		url.QueryEscape(cfg.Password),
		cfg.Host,
		cfg.Port,
		query.Encode(),
	)

	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return nil, fmt.Errorf("open SQL auth connection: %w", err)
	}

	return db, nil
}

// createServicePrincipalConnection creates a connection using Azure AD Service Principal.
// Uses connection string with fedauth parameter for Azure AD authentication.
func createServicePrincipalConnection(cfg *Config) (*sql.DB, error) {
	query := url.Values{}
	query.Add("database", cfg.Database)
	query.Add("fedauth", "ActiveDirectoryServicePrincipal")
	query.Add("user id", cfg.ClientID)
	query.Add("password", cfg.ClientSecret)
	query.Add("tenant id", cfg.TenantID)

	if cfg.Encrypt {
		query.Add("encrypt", "true")
	}
	if cfg.TrustServerCertificate {
		query.Add("TrustServerCertificate", "true")
	}
	if cfg.ConnectionTimeout > 0 {
		query.Add("connection timeout", fmt.Sprintf("%d", cfg.ConnectionTimeout))
	}

	// For Azure AD, use azuresql driver
	connStr := fmt.Sprintf("sqlserver://%s:%d?%s",
		cfg.Host,
		cfg.Port,
		query.Encode(),
	)

	db, err := sql.Open("azuresql", connStr)
	if err != nil {
		return nil, fmt.Errorf("open service principal connection: %w", err)
	}

	return db, nil
}

// createUserDelegationConnection creates a connection using an Azure AD access token.
// The token is extracted from context by NewAdapter() and stored in cfg.AzureAccessToken.
// Uses sqlserver driver with fedauth=ActiveDirectoryAccessToken and token as password.
func createUserDelegationConnection(cfg *Config) (*sql.DB, error) {
	query := url.Values{}
	query.Add("database", cfg.Database)
	query.Add("fedauth", "ActiveDirectoryAccessToken")

	if cfg.Encrypt {
		query.Add("encrypt", "true")
	} else {
		query.Add("encrypt", "false")
	}
	if cfg.TrustServerCertificate {
		query.Add("TrustServerCertificate", "true")
	}
	if cfg.ConnectionTimeout > 0 {
		query.Add("connection timeout", fmt.Sprintf("%d", cfg.ConnectionTimeout))
	}

	// For ActiveDirectoryAccessToken, pass token as password parameter
	// The sqlserver driver (not azuresql) supports this fedauth type
	query.Add("password", cfg.AzureAccessToken)

	connStr := fmt.Sprintf("sqlserver://%s:%d?%s",
		cfg.Host,
		cfg.Port,
		query.Encode(),
	)

	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		return nil, fmt.Errorf("open user delegation connection: %w", err)
	}

	return db, nil
}

// TestConnection verifies the database is reachable with valid credentials.
func (a *Adapter) TestConnection(ctx context.Context) error {
	if err := a.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Run a simple query to ensure we have database access
	var result int
	err := a.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return fmt.Errorf("test query failed: %w", err)
	}

	return nil
}

// Close releases the adapter (but NOT the DB if managed).
func (a *Adapter) Close() error {
	if a.ownedDB && a.db != nil {
		return a.db.Close()
	}
	// If using connection manager, don't close the DB - it's managed by TTL
	return nil
}

// DB returns the underlying *sql.DB for use by schema discoverer and query executor.
func (a *Adapter) DB() *sql.DB {
	return a.db
}

// Ensure Adapter implements ConnectionTester at compile time.
var _ datasource.ConnectionTester = (*Adapter)(nil)
