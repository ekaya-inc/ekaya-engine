package database

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TenantScope wraps a connection with tenant context and ensures cleanup.
// The connection has app.current_project_id set for RLS policy evaluation.
type TenantScope struct {
	Conn *pgxpool.Conn
}

// Close resets tenant context and releases connection to pool.
// This MUST be called to prevent tenant context from leaking to the next request.
func (s *TenantScope) Close() {
	if s.Conn == nil {
		return
	}
	// Reset the tenant context before returning connection to pool
	_, _ = s.Conn.Exec(context.Background(), "RESET app.current_project_id")
	s.Conn.Release()
}

// WithTenant acquires a connection and sets the tenant context for RLS.
// The returned TenantScope MUST be closed with defer scope.Close().
func (db *DB) WithTenant(ctx context.Context, projectID uuid.UUID) (*TenantScope, error) {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}

	_, err = conn.Exec(ctx, "SELECT set_config('app.current_project_id', $1, false)", projectID.String())
	if err != nil {
		conn.Release()
		return nil, err
	}

	return &TenantScope{Conn: conn}, nil
}

// WithoutTenant acquires a connection without tenant context.
// Use this for central service operations that need full access (e.g., project creation).
// The returned TenantScope MUST be closed with defer scope.Close().
func (db *DB) WithoutTenant(ctx context.Context) (*TenantScope, error) {
	conn, err := db.Pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	return &TenantScope{Conn: conn}, nil
}
