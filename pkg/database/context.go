package database

import "context"

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
