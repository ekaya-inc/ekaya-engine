package datasource

import "context"

// PoolConnector is an interface that abstracts connection pool operations
// across different database types (PostgreSQL, MSSQL, MySQL, etc.)
type PoolConnector interface {
	// Ping verifies the connection is alive
	Ping(ctx context.Context) error

	// Close closes all connections in the pool
	Close() error

	// GetType returns the database type for logging/stats
	GetType() string
}
