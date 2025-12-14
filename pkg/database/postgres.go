package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps a pgxpool connection pool.
type DB struct {
	*pgxpool.Pool
}

// Config holds database connection configuration.
type Config struct {
	URL             string
	MaxConnections  int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// NewConnection creates a new database connection pool.
func NewConnection(ctx context.Context, cfg *Config) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database URL: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConnections
	if poolConfig.MaxConns == 0 {
		poolConfig.MaxConns = 25
	}

	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	if poolConfig.MaxConnLifetime == 0 {
		poolConfig.MaxConnLifetime = time.Hour
	}

	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	if poolConfig.MaxConnIdleTime == 0 {
		poolConfig.MaxConnIdleTime = time.Minute * 30
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{Pool: pool}, nil
}

// Close closes the connection pool.
func (db *DB) Close() {
	db.Pool.Close()
}
