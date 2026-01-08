package postgres

import (
	"context"
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// Adapter provides PostgreSQL connectivity.
type Adapter struct {
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
			pool:      pool,
			ownedPool: true,
		}, nil
	}

	// Use connection manager for reusable pool
	pool, err := connMgr.GetOrCreatePool(ctx, projectID, userID, datasourceID, connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to get pooled connection: %w", err)
	}

	return &Adapter{
		pool:         pool,
		connMgr:      connMgr,
		projectID:    projectID,
		userID:       userID,
		datasourceID: datasourceID,
		ownedPool:    false,
	}, nil
}

// TestConnection verifies the database is reachable with valid credentials.
func (a *Adapter) TestConnection(ctx context.Context) error {
	return a.pool.Ping(ctx)
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
