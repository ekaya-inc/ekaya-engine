package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
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

// TenantContextWithProvenanceFunc acquires a tenant-scoped database connection
// with inference provenance context. Use this for DAG tasks that create/modify
// ontology objects.
// Returns the scoped context with provenance, a cleanup function (MUST be called), and any error.
type TenantContextWithProvenanceFunc func(ctx context.Context, projectID uuid.UUID, userID uuid.UUID) (context.Context, func(), error)

// NewTenantContextWithProvenanceFunc creates a TenantContextWithProvenanceFunc
// that uses the given database and injects inference provenance.
func NewTenantContextWithProvenanceFunc(db *database.DB) TenantContextWithProvenanceFunc {
	return func(ctx context.Context, projectID uuid.UUID, userID uuid.UUID) (context.Context, func(), error) {
		scope, err := db.WithTenant(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		tenantCtx := database.SetTenantScope(ctx, scope)
		// Add inference provenance for DAG tasks
		tenantCtx = models.WithInferredProvenance(tenantCtx, userID)
		return tenantCtx, func() { scope.Close() }, nil
	}
}

// WithInferredProvenanceWrapper wraps a TenantContextFunc to add inference provenance.
// This allows existing DAG tasks to add provenance by wrapping their getTenantCtx function.
//
// Example usage in a DAG task:
//
//	wrappedGetTenantCtx := services.WithInferredProvenanceWrapper(t.getTenantCtx, userID)
//	tenantCtx, cleanup, err := wrappedGetTenantCtx(ctx, t.projectID)
func WithInferredProvenanceWrapper(getTenantCtx TenantContextFunc, userID uuid.UUID) TenantContextFunc {
	return func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		tenantCtx, cleanup, err := getTenantCtx(ctx, projectID)
		if err != nil {
			return nil, nil, err
		}
		// Add inference provenance
		tenantCtx = models.WithInferredProvenance(tenantCtx, userID)
		return tenantCtx, cleanup, nil
	}
}
