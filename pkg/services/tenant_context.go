package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
)

// TenantContextFunc acquires a tenant-scoped database connection.
// Returns the scoped context, a cleanup function (MUST be called), and any error.
type TenantContextFunc func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error)

// NewTenantContextFunc creates a TenantContextFunc that uses the given database.
func NewTenantContextFunc(db *database.DB) TenantContextFunc {
	return func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		scope, err := db.WithTenant(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		tenantCtx := database.SetTenantScope(ctx, scope)
		return tenantCtx, func() { scope.Close() }, nil
	}
}
