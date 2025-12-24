package datasource

import (
	"context"
	"database/sql"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgresPoolWrapper wraps *pgxpool.Pool to implement PoolConnector
type PostgresPoolWrapper struct {
	pool *pgxpool.Pool
}

// NewPostgresPoolWrapper creates a new PostgreSQL pool wrapper
func NewPostgresPoolWrapper(pool *pgxpool.Pool) *PostgresPoolWrapper {
	return &PostgresPoolWrapper{pool: pool}
}

// Ping verifies the PostgreSQL connection is alive
func (w *PostgresPoolWrapper) Ping(ctx context.Context) error {
	return w.pool.Ping(ctx)
}

// Close closes all connections in the PostgreSQL pool
func (w *PostgresPoolWrapper) Close() error {
	w.pool.Close()
	return nil
}

// GetType returns the database type
func (w *PostgresPoolWrapper) GetType() string {
	return "postgres"
}

// GetPool returns the underlying *pgxpool.Pool
func (w *PostgresPoolWrapper) GetPool() *pgxpool.Pool {
	return w.pool
}

// MSSQLPoolWrapper wraps *sql.DB to implement PoolConnector
type MSSQLPoolWrapper struct {
	db *sql.DB
}

// NewMSSQLPoolWrapper creates a new MSSQL pool wrapper
func NewMSSQLPoolWrapper(db *sql.DB) *MSSQLPoolWrapper {
	return &MSSQLPoolWrapper{db: db}
}

// Ping verifies the MSSQL connection is alive
func (w *MSSQLPoolWrapper) Ping(ctx context.Context) error {
	return w.db.PingContext(ctx)
}

// Close closes all connections in the MSSQL pool
func (w *MSSQLPoolWrapper) Close() error {
	return w.db.Close()
}

// GetType returns the database type
func (w *MSSQLPoolWrapper) GetType() string {
	return "mssql"
}

// GetDB returns the underlying *sql.DB
func (w *MSSQLPoolWrapper) GetDB() *sql.DB {
	return w.db
}
