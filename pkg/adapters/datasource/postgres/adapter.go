package postgres

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// Adapter provides PostgreSQL connectivity.
type Adapter struct {
	config       *Config
	pool         *pgxpool.Pool
	connMgr      *datasource.ConnectionManager
	projectID    uuid.UUID
	userID       string
	datasourceID uuid.UUID
	ownedPool    bool // true if we created the pool (for TestConnection case)
}

// buildConnectionString builds a PostgreSQL URL with proper escaping.
// IMPORTANT: All user-provided fields must be URL-escaped to handle special characters
// in passwords (e.g., @, /, #, ?) that would otherwise break URL parsing.
// When running in Docker, localhost is automatically resolved to host.docker.internal
// to allow connections to databases running on the host machine.
func buildConnectionString(cfg *Config) string {
	sslMode := cfg.SSLMode
	if sslMode == "" {
		sslMode = "require"
	}

	// Resolve localhost to host.docker.internal when running in Docker
	host := config.ResolveHostForDocker(cfg.Host)

	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(cfg.User),
		url.QueryEscape(cfg.Password),
		host,
		cfg.Port,
		url.QueryEscape(cfg.Database),
		sslMode,
	)
}

// NewAdapter creates a PostgreSQL adapter using the connection manager.
// If connMgr is nil, creates an unmanaged pool (for tests or TestConnection).
func NewAdapter(ctx context.Context, cfg *Config, connMgr *datasource.ConnectionManager, projectID, datasourceID uuid.UUID, userID string) (*Adapter, error) {
	connStr := buildConnectionString(cfg)

	if connMgr == nil {
		// Fallback for direct instantiation (tests, TestConnection)
		pool, err := pgxpool.New(ctx, connStr)
		if err != nil {
			return nil, fmt.Errorf("connect to postgres: %w", err)
		}

		return &Adapter{
			config:    cfg,
			pool:      pool,
			ownedPool: true,
		}, nil
	}

	// Use connection manager for reusable pool
	connector, err := connMgr.GetOrCreateConnection(ctx, "postgres", projectID, userID, datasourceID, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get pooled connection: %w", err)
	}

	// Extract underlying PostgreSQL pool from connector
	pool, err := datasource.GetPostgresPool(connector)
	if err != nil {
		return nil, fmt.Errorf("failed to extract postgres pool: %w", err)
	}

	return &Adapter{
		config:       cfg,
		pool:         pool,
		connMgr:      connMgr,
		projectID:    projectID,
		userID:       userID,
		datasourceID: datasourceID,
		ownedPool:    false,
	}, nil
}

// TestConnection verifies the database is reachable with valid credentials.
// It checks:
// 1. Server connectivity (ping)
// 2. Database access (simple query)
// 3. Correct database name (to prevent connecting to wrong/default database)
func (a *Adapter) TestConnection(ctx context.Context) error {
	if err := a.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	// Run a simple query to ensure we have database access
	var result int
	if err := a.pool.QueryRow(ctx, "SELECT 1").Scan(&result); err != nil {
		return fmt.Errorf("test query failed: %w", err)
	}

	// Verify we're connected to the correct database
	// PostgreSQL connection strings include the database, but we should verify
	var currentDB string
	if err := a.pool.QueryRow(ctx, "SELECT current_database()").Scan(&currentDB); err != nil {
		return fmt.Errorf("failed to get current database name: %w", err)
	}

	expectedDB := a.config.Database
	// PostgreSQL database names are case-sensitive, but we'll do case-insensitive comparison
	// to match MSSQL behavior and handle common configuration issues
	if !strings.EqualFold(currentDB, expectedDB) {
		return fmt.Errorf("connected to wrong database: expected %q but connected to %q", expectedDB, currentDB)
	}

	return nil
}

// Close releases the adapter (but NOT the pool if managed).
func (a *Adapter) Close() error {
	if a.ownedPool && a.pool != nil {
		a.pool.Close()
	}
	// If using connection manager, don't close the pool - it's managed by TTL
	return nil
}

// Ensure Adapter implements ConnectionTester at compile time.
var _ datasource.ConnectionTester = (*Adapter)(nil)
