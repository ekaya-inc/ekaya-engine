package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

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
	return s.userRepo.RemoveWithOwnerCheck(ctx, projectID, userID)
}

// Update updates a user's role in a project.
// Returns ErrLastAdmin if attempting to demote the last admin.
func (s *userService) Update(ctx context.Context, projectID, userID uuid.UUID, newRole string) error {
	if !models.IsValidRole(newRole) {
		return fmt.Errorf("invalid role: %s", newRole)
	}

	return s.userRepo.UpdateRoleWithOwnerCheck(ctx, projectID, userID, newRole)
}

// GetByProject retrieves all users for a project.
func (s *userService) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.User, error) {
	return s.userRepo.GetByProject(ctx, projectID)
}

// Ensure userService implements UserService at compile time.
var _ UserService = (*userService)(nil)
