package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/services/workflow"
)

// TenantContextFunc is an alias for workflow.TenantContextFunc for backwards compatibility.
// New code should import from pkg/services/workflow directly.
type TenantContextFunc = workflow.TenantContextFunc

// NewTenantContextFunc creates a TenantContextFunc that uses the given database.
func NewTenantContextFunc(db *database.DB) workflow.TenantContextFunc {
	return func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		scope, err := db.WithTenant(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		tenantCtx := database.SetTenantScope(ctx, scope)
		return tenantCtx, func() { scope.Close() }, nil
	}
}
