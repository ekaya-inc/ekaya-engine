package mssql

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"

	mssql "github.com/microsoft/go-mssqldb"
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

// extractAndSetAzureToken extracts the Azure access token from context using token reference ID.
// It fetches the token from ekaya-central API and caches it in memory.
// This function is idempotent - if token is already set in config, it does nothing.
func extractAndSetAzureToken(ctx context.Context, cfg *Config) error {
	// If token is already set, skip extraction (idempotent)
	if cfg.AzureAccessToken != "" {
		return nil
	}

	claims, hasClaims := auth.GetClaims(ctx)
	if !hasClaims || claims == nil {
		return fmt.Errorf("user_delegation auth requires claims in request context")
	}

	if claims.AzureTokenRefID == "" {
		return fmt.Errorf("user_delegation auth requires Azure token reference ID in JWT claims")
	}

	// Get JWT token from context for API call
	jwtToken, hasJWT := auth.GetToken(ctx)
	if !hasJWT || jwtToken == "" {
		return fmt.Errorf("user_delegation auth requires JWT token in context for token reference lookup")
	}

	if claims.PAPI == "" {
		return fmt.Errorf("user_delegation auth requires PAPI (auth server URL) in claims")
	}

	// Check cache first
	tokenCache := auth.GetTokenCache()
	cacheKey := fmt.Sprintf("%s:%s", claims.Subject, claims.AzureTokenRefID)
	if cachedToken, found := tokenCache.Get(cacheKey); found {
		cfg.AzureAccessToken = cachedToken
		return nil
	}

	// Fetch from API
	tokenClient := auth.GetTokenClient()
	token, err := tokenClient.FetchTokenByReference(ctx, claims.AzureTokenRefID, claims.PAPI, jwtToken)
	if err != nil {
		return fmt.Errorf("fetch token by reference: %w", err)
	}

	// Cache token
	var expiresAt time.Time
	if claims.AzureTokenExpiry > 0 {
		expiresAt = time.Unix(claims.AzureTokenExpiry, 0)
	} else {
		// Default to 1 hour if expiry not available
		expiresAt = time.Now().Add(1 * time.Hour)
	}
	// Cache with 5 minute buffer before expiry
	cacheExpiry := expiresAt.Add(-5 * time.Minute)
	tokenCache.Set(cacheKey, token, cacheExpiry)

	cfg.AzureAccessToken = token
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
// Ported from C# pattern: dbConnection.AccessToken = token
// Uses a custom connector that sets the access token on the connection.
func createUserDelegationConnection(cfg *Config) (*sql.DB, error) {
	// Build connection string without authentication
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

	connStr := fmt.Sprintf("sqlserver://%s:%d?%s",
		cfg.Host,
		cfg.Port,
		query.Encode(),
	)

	// Create connector with access token
	connector, err := mssql.NewAccessTokenConnector(connStr, func() (string, error) {
		return cfg.AzureAccessToken, nil
	})
	if err != nil {
		return nil, fmt.Errorf("create access token connector: %w", err)
	}

	// Open database using the connector
	db := sql.OpenDB(connector)
	return db, nil
}

// TestConnection verifies the database is reachable with valid credentials.
// It checks:
// 1. Server connectivity (ping)
// 2. Database access (simple query)
// 3. Correct database name (to prevent connecting to wrong/default database)
func (a *Adapter) TestConnection(ctx context.Context) error {
	if err := a.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Run a simple query to ensure we have database access
	var result int
	if err := a.db.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("test query failed: %w", err)
	}

	// Verify we're connected to the correct database
	// SQL Server might connect to 'master' or another database if the specified one doesn't exist
	var currentDB string
	if err := a.db.QueryRowContext(ctx, "SELECT DB_NAME()").Scan(&currentDB); err != nil {
		return fmt.Errorf("failed to get current database name: %w", err)
	}

	expectedDB := a.config.Database
	if currentDB != expectedDB {
		return fmt.Errorf("connected to wrong database: expected %q but connected to %q", expectedDB, currentDB)
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
