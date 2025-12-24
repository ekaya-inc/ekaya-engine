package datasource

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreatePostgresPool creates a PostgreSQL connection pool
func CreatePostgresPool(ctx context.Context, connString string, config ConnectionManagerConfig) (PoolConnector, error) {
	poolConfig, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}

	// Apply connection manager settings
	poolConfig.MaxConns = config.PoolMaxConns
	poolConfig.MinConns = config.PoolMinConns
	poolConfig.MaxConnIdleTime = time.Duration(config.TTLMinutes) * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}

	return NewPostgresPoolWrapper(pool), nil
}

// GetPostgresPool extracts the underlying *pgxpool.Pool from a PoolConnector.
// Returns an error if the connector is not a PostgreSQL pool.
func GetPostgresPool(connector PoolConnector) (*pgxpool.Pool, error) {
	wrapper, ok := connector.(*PostgresPoolWrapper)
	if !ok {
		return nil, fmt.Errorf("connector is not a PostgreSQL pool wrapper")
	}
	return wrapper.GetPool(), nil
}

// CreateMSSQLPool creates an MSSQL connection pool.
// Note: This is a simple wrapper factory. The actual connection creation logic
// (handling different auth methods) should be done by the MSSQL adapter before
// calling this factory. This factory expects a pre-created *sql.DB.
// For a proper implementation, MSSQL adapters should create the connection
// and wrap it, or we need to pass auth metadata through the connection string.
//
// For now, this is a placeholder. MSSQL connection creation is handled
// directly in the adapter due to auth complexity (user_delegation requires
// access token from context, different drivers for different auth methods).
func CreateMSSQLPool(ctx context.Context, connString string, config ConnectionManagerConfig) (PoolConnector, error) {
	// This is a simplified version. In practice, MSSQL connection creation
	// is handled by the adapter because it needs access to context for tokens
	// and different drivers for different auth methods.
	// The adapter will create the *sql.DB and wrap it directly.
	return nil, fmt.Errorf("CreateMSSQLPool not implemented - MSSQL adapters create connections directly")
}

// GetMSSQLDB extracts the underlying *sql.DB from a PoolConnector.
// Returns an error if the connector is not an MSSQL pool.
func GetMSSQLDB(connector PoolConnector) (*sql.DB, error) {
	wrapper, ok := connector.(*MSSQLPoolWrapper)
	if !ok {
		return nil, fmt.Errorf("connector is not an MSSQL pool wrapper")
	}
	return wrapper.GetDB(), nil
}
