package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// UserRepository defines the interface for user data access.
type UserRepository interface {
	Add(ctx context.Context, user *models.User) error
	Remove(ctx context.Context, projectID, userID uuid.UUID) error
	Update(ctx context.Context, projectID, userID uuid.UUID, newRole string) error
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.User, error)
	GetByID(ctx context.Context, projectID, userID uuid.UUID) (*models.User, error)
	CountAdmins(ctx context.Context, projectID uuid.UUID) (int, error)
}

// userRepository implements UserRepository using PostgreSQL.
type userRepository struct{}

// NewUserRepository creates a new user repository.
func NewUserRepository() UserRepository {
	return &userRepository{}
}

// Add adds a user to a project with the specified role.
func (r *userRepository) Add(ctx context.Context, user *models.User) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	now := time.Now()
	user.CreatedAt = now
	user.UpdatedAt = now

	query := `
		INSERT INTO engine_users (project_id, user_id, role, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, user_id) DO UPDATE
		SET role = EXCLUDED.role,
		    updated_at = EXCLUDED.updated_at`

	_, err := scope.Conn.Exec(ctx, query,
		user.ProjectID,
		user.UserID,
		user.Role,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to add user: %w", err)
	}

	return nil
}

// Remove removes a user from a project.
func (r *userRepository) Remove(ctx context.Context, projectID, userID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_users WHERE project_id = $1 AND user_id = $2`

	result, err := scope.Conn.Exec(ctx, query, projectID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// Update updates a user's role in a project.
func (r *userRepository) Update(ctx context.Context, projectID, userID uuid.UUID, newRole string) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `
		UPDATE engine_users
		SET role = $1, updated_at = $2
		WHERE project_id = $3 AND user_id = $4`

	result, err := scope.Conn.Exec(ctx, query, newRole, time.Now(), projectID, userID)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("user not found")
	}

	return nil
}

// GetByProject retrieves all users for a project.
func (r *userRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.User, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT project_id, user_id, role, created_at, updated_at
		FROM engine_users
		WHERE project_id = $1
		ORDER BY created_at`

	rows, err := scope.Conn.Query(ctx, query, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		var user models.User
		err := rows.Scan(
			&user.ProjectID,
			&user.UserID,
			&user.Role,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, &user)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating users: %w", err)
	}

	return users, nil
}

// GetByID retrieves a specific user from a project.
func (r *userRepository) GetByID(ctx context.Context, projectID, userID uuid.UUID) (*models.User, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT project_id, user_id, role, created_at, updated_at
		FROM engine_users
		WHERE project_id = $1 AND user_id = $2`

	var user models.User
	err := scope.Conn.QueryRow(ctx, query, projectID, userID).Scan(
		&user.ProjectID,
		&user.UserID,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return &user, nil
}

// CountAdmins returns the number of admin users in a project.
func (r *userRepository) CountAdmins(ctx context.Context, projectID uuid.UUID) (int, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return 0, fmt.Errorf("no tenant scope in context")
	}

	query := `SELECT COUNT(*) FROM engine_users WHERE project_id = $1 AND role = 'admin'`

	var count int
	err := scope.Conn.QueryRow(ctx, query, projectID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count admins: %w", err)
	}

	return count, nil
}

// Ensure userRepository implements UserRepository at compile time.
var _ UserRepository = (*userRepository)(nil)
