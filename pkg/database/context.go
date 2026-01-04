package database

import (
	"context"

	"github.com/google/uuid"
)

type contextKey string

const (
	// TenantScopeKey is the context key for storing the tenant-scoped database connection.
	TenantScopeKey contextKey = "tenantScope"
)

// GetTenantScope retrieves the tenant-scoped database connection from context.
// Returns nil and false if not present.
func GetTenantScope(ctx context.Context) (*TenantScope, bool) {
	scope, ok := ctx.Value(TenantScopeKey).(*TenantScope)
	return scope, ok
}

// SetTenantScope stores the tenant-scoped database connection in context.
func SetTenantScope(ctx context.Context, scope *TenantScope) context.Context {
	return context.WithValue(ctx, TenantScopeKey, scope)
}

// TenantScopeProvider creates tenant-scoped contexts for database operations.
// This satisfies mcpauth.TenantScopeProvider via Go's implicit interfaces.
type TenantScopeProvider struct {
	db *DB
}

// NewTenantScopeProvider creates a TenantScopeProvider for the given database.
func NewTenantScopeProvider(db *DB) *TenantScopeProvider {
	return &TenantScopeProvider{db: db}
}

// WithTenantScope returns a context with tenant scope set for the given project.
// The cleanup function must be called when the scope is no longer needed.
func (p *TenantScopeProvider) WithTenantScope(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
	scope, err := p.db.WithTenant(ctx, projectID)
	if err != nil {
		return nil, nil, err
	}
	tenantCtx := SetTenantScope(ctx, scope)
	return tenantCtx, func() { scope.Close() }, nil
}
