package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
)

// Adapter provides PostgreSQL connectivity.
type Adapter struct {
	pool *pgxpool.Pool
}

// NewAdapter creates a PostgreSQL adapter with the given config.
func NewAdapter(ctx context.Context, cfg *Config) (*Adapter, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.Database, cfg.SSLMode,
	)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		return nil, fmt.Errorf("connect to postgres: %w", err)
	}

	return &Adapter{pool: pool}, nil
}

// TestConnection verifies the database is reachable with valid credentials.
func (a *Adapter) TestConnection(ctx context.Context) error {
	return a.pool.Ping(ctx)
}

// Close releases the connection pool.
func (a *Adapter) Close() error {
	a.pool.Close()
	return nil
}

// Ensure Adapter implements ConnectionTester at compile time.
var _ datasource.ConnectionTester = (*Adapter)(nil)
