package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
)

// NonceRepository provides data access for callback nonces.
type NonceRepository interface {
	Create(ctx context.Context, nonce, action string, projectID uuid.UUID, appID string, expiresAt time.Time) error
	ValidateAndConsume(ctx context.Context, nonce, action string, projectID uuid.UUID, appID string) (bool, error)
	DeleteExpired(ctx context.Context) (int64, error)
}

type nonceRepository struct{}

// NewNonceRepository creates a new NonceRepository.
func NewNonceRepository() NonceRepository {
	return &nonceRepository{}
}

var _ NonceRepository = (*nonceRepository)(nil)

func (r *nonceRepository) Create(ctx context.Context, nonce, action string, projectID uuid.UUID, appID string, expiresAt time.Time) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		INSERT INTO engine_nonces (
			nonce, project_id, action, app_id, expires_at
		) VALUES ($1, $2, $3, $4, $5)`

	if _, err := scope.Conn.Exec(ctx, query, nonce, projectID, action, appID, expiresAt); err != nil {
		return fmt.Errorf("failed to create nonce: %w", err)
	}

	return nil
}

func (r *nonceRepository) ValidateAndConsume(ctx context.Context, nonce, action string, projectID uuid.UUID, appID string) (bool, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return false, fmt.Errorf("no tenant scope in context")
	}

	query := `
		DELETE FROM engine_nonces
		WHERE nonce = $1
		  AND action = $2
		  AND project_id = $3
		  AND app_id = $4
		RETURNING expires_at > NOW()`

	var valid bool
	err := scope.Conn.QueryRow(ctx, query, nonce, action, projectID, appID).Scan(&valid)
	if err != nil {
		if err == pgx.ErrNoRows {
			return false, nil
		}
		return false, fmt.Errorf("failed to validate nonce: %w", err)
	}

	return valid, nil
}

func (r *nonceRepository) DeleteExpired(ctx context.Context) (int64, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	tag, err := scope.Conn.Exec(ctx, `DELETE FROM engine_nonces WHERE expires_at <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("failed to delete expired nonces: %w", err)
	}

	return tag.RowsAffected(), nil
}
