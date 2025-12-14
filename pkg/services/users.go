package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// UserService defines the interface for user operations.
type UserService interface {
	Add(ctx context.Context, projectID, userID uuid.UUID, role string) error
	Remove(ctx context.Context, projectID, userID uuid.UUID) error
	Update(ctx context.Context, projectID, userID uuid.UUID, newRole string) error
	GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.User, error)
}

// userService implements UserService.
type userService struct {
	userRepo repositories.UserRepository
	logger   *zap.Logger
}

// NewUserService creates a new user service with dependencies.
func NewUserService(userRepo repositories.UserRepository, logger *zap.Logger) UserService {
	return &userService{
		userRepo: userRepo,
		logger:   logger,
	}
}

// Add adds a user to a project with the specified role.
func (s *userService) Add(ctx context.Context, projectID, userID uuid.UUID, role string) error {
	if !models.IsValidRole(role) {
		return fmt.Errorf("invalid role: %s", role)
	}

	user := &models.User{
		ProjectID: projectID,
		UserID:    userID,
		Role:      role,
	}

	return s.userRepo.Add(ctx, user)
}

// Remove removes a user from a project.
// Returns ErrLastAdmin if attempting to remove the last admin.
func (s *userService) Remove(ctx context.Context, projectID, userID uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Start transaction for atomic check-and-delete
	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Check if user is an admin
	user, err := s.userRepo.GetByID(ctx, projectID, userID)
	if err != nil {
		return err
	}

	// If user is admin, check if they're the last one
	if user.Role == models.RoleAdmin {
		adminCount, err := s.userRepo.CountAdmins(ctx, projectID)
		if err != nil {
			return fmt.Errorf("failed to count admins: %w", err)
		}

		if adminCount <= 1 {
			return apperrors.ErrLastAdmin
		}
	}

	// Remove the user
	if err = s.userRepo.Remove(ctx, projectID, userID); err != nil {
		return err
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Update updates a user's role in a project.
// Returns ErrLastAdmin if attempting to demote the last admin.
func (s *userService) Update(ctx context.Context, projectID, userID uuid.UUID, newRole string) error {
	if !models.IsValidRole(newRole) {
		return fmt.Errorf("invalid role: %s", newRole)
	}

	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Start transaction for atomic check-and-update
	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Get current user
	user, err := s.userRepo.GetByID(ctx, projectID, userID)
	if err != nil {
		return err
	}

	// If demoting from admin, check if they're the last one
	if user.Role == models.RoleAdmin && newRole != models.RoleAdmin {
		adminCount, err := s.userRepo.CountAdmins(ctx, projectID)
		if err != nil {
			return fmt.Errorf("failed to count admins: %w", err)
		}

		if adminCount <= 1 {
			return apperrors.ErrLastAdmin
		}
	}

	// Update the user's role
	if err = s.userRepo.Update(ctx, projectID, userID, newRole); err != nil {
		return err
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// GetByProject retrieves all users for a project.
func (s *userService) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.User, error) {
	return s.userRepo.GetByProject(ctx, projectID)
}

// Ensure userService implements UserService at compile time.
var _ UserService = (*userService)(nil)
