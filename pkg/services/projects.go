package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// CreateProjectResult contains the result of creating a project.
type CreateProjectResult struct {
	ProjectID  uuid.UUID
	ProjectURL string
}

// ProvisionResult contains the result of provisioning a project.
type ProvisionResult struct {
	ProjectID uuid.UUID
	Name      string
	PAPIURL   string
	Created   bool // true if project was created, false if already existed
}

// ProjectService defines the interface for project operations.
type ProjectService interface {
	Create(ctx context.Context, name string, adminUserID uuid.UUID, params map[string]interface{}) (*CreateProjectResult, error)
	Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error)
	ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error)
	GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error)
	GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

// projectService implements ProjectService.
type projectService struct {
	db          *database.DB
	projectRepo repositories.ProjectRepository
	userRepo    repositories.UserRepository
	redis       *redis.Client
	baseURL     string
	logger      *zap.Logger
}

// NewProjectService creates a new project service with dependencies.
func NewProjectService(
	db *database.DB,
	projectRepo repositories.ProjectRepository,
	userRepo repositories.UserRepository,
	redisClient *redis.Client,
	baseURL string,
	logger *zap.Logger,
) ProjectService {
	return &projectService{
		db:          db,
		projectRepo: projectRepo,
		userRepo:    userRepo,
		redis:       redisClient,
		baseURL:     baseURL,
		logger:      logger,
	}
}

// Create creates a new project with an admin user.
// This is called by central service (no tenant context) so we use WithoutTenant.
func (s *projectService) Create(ctx context.Context, name string, adminUserID uuid.UUID, params map[string]interface{}) (*CreateProjectResult, error) {
	// Acquire connection without tenant context (central service mode)
	scope, err := s.db.WithoutTenant(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer scope.Close()

	// Set tenant scope in context for repositories
	ctx = database.SetTenantScope(ctx, scope)

	// Start transaction
	tx, err := scope.Conn.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	// Create project
	project := &models.Project{
		ID:         uuid.New(),
		Name:       name,
		Parameters: params,
		Status:     "active",
	}

	if err = s.projectRepo.Create(ctx, project); err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	// Add admin user
	user := &models.User{
		ProjectID: project.ID,
		UserID:    adminUserID,
		Role:      models.RoleAdmin,
	}

	if err = s.userRepo.Add(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to add admin user: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Cache project config in Redis
	if s.redis != nil {
		s.cacheProjectConfig(ctx, project)
	}

	return &CreateProjectResult{
		ProjectID:  project.ID,
		ProjectURL: fmt.Sprintf("%s/projects/%s", s.baseURL, project.ID),
	}, nil
}

// Provision ensures a project exists with the given ID (idempotent).
// This is called during user's first access to provision the project locally.
// Uses WithoutTenant since the project may not exist yet.
func (s *projectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	// Acquire connection without tenant context (project may not exist yet)
	scope, err := s.db.WithoutTenant(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer scope.Close()

	// Set tenant scope in context for repositories
	ctx = database.SetTenantScope(ctx, scope)

	// Check if project already exists
	existingProject, err := s.projectRepo.Get(ctx, projectID)
	if err == nil {
		// Project exists - return its info
		var papiURL string
		if existingProject.Parameters != nil {
			if papi, ok := existingProject.Parameters["papi_url"].(string); ok {
				papiURL = papi
			}
		}
		return &ProvisionResult{
			ProjectID: existingProject.ID,
			Name:      existingProject.Name,
			PAPIURL:   papiURL,
			Created:   false,
		}, nil
	}

	// Check if it's a "not found" error or a real error
	if !errors.Is(err, apperrors.ErrNotFound) {
		return nil, fmt.Errorf("failed to check project existence: %w", err)
	}

	// Project doesn't exist - create it
	s.logger.Info("Provisioning new project",
		zap.String("project_id", projectID.String()),
		zap.String("name", name))

	project := &models.Project{
		ID:         projectID,
		Name:       name,
		Parameters: params,
		Status:     "active",
	}

	if err = s.projectRepo.Create(ctx, project); err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	// Cache project config in Redis
	if s.redis != nil {
		s.cacheProjectConfig(ctx, project)
	}

	// Extract papi_url from parameters
	var papiURL string
	if params != nil {
		if papi, ok := params["papi_url"].(string); ok {
			papiURL = papi
		}
	}

	return &ProvisionResult{
		ProjectID: project.ID,
		Name:      project.Name,
		PAPIURL:   papiURL,
		Created:   true,
	}, nil
}

// GetByID returns a project by its ID.
// Assumes tenant context is already set by middleware.
func (s *projectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return s.projectRepo.Get(ctx, id)
}

// GetByIDWithoutTenant returns a project by its ID, managing its own connection.
// Used when tenant context is not available (e.g., checking if project exists before provisioning).
func (s *projectService) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	scope, err := s.db.WithoutTenant(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer scope.Close()

	ctx = database.SetTenantScope(ctx, scope)
	return s.projectRepo.Get(ctx, id)
}

// Delete removes a project and all associated data.
// Assumes tenant context is already set by middleware.
func (s *projectService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.projectRepo.Delete(ctx, id); err != nil {
		return err
	}

	// Clear Redis cache
	if s.redis != nil {
		s.clearProjectCache(ctx, id)
	}

	return nil
}

// cacheProjectConfig caches project configuration in Redis.
func (s *projectService) cacheProjectConfig(ctx context.Context, project *models.Project) {
	configKey := fmt.Sprintf("project:%s:config", project.ID)
	configData, err := json.Marshal(map[string]interface{}{
		"name":       project.Name,
		"created_at": project.CreatedAt,
		"parameters": project.Parameters,
	})
	if err != nil {
		s.logger.Error("Failed to marshal project config for cache",
			zap.String("project_id", project.ID.String()),
			zap.Error(err))
		return
	}

	if err := s.redis.Set(ctx, configKey, configData, 0).Err(); err != nil {
		s.logger.Error("Failed to cache project config",
			zap.String("project_id", project.ID.String()),
			zap.Error(err))
	}
}

// clearProjectCache removes all cached data for a project.
func (s *projectService) clearProjectCache(ctx context.Context, projectID uuid.UUID) {
	// Delete main config key
	configKey := fmt.Sprintf("project:%s:config", projectID)
	if err := s.redis.Del(ctx, configKey).Err(); err != nil {
		s.logger.Error("Failed to delete project config cache",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
	}

	// Delete any sub-keys with pattern project:<id>:*
	pattern := fmt.Sprintf("project:%s:*", projectID)
	iter := s.redis.Scan(ctx, 0, pattern, 0).Iterator()
	var keysToDelete []string
	for iter.Next(ctx) {
		keysToDelete = append(keysToDelete, iter.Val())
	}
	if len(keysToDelete) > 0 {
		if err := s.redis.Del(ctx, keysToDelete...).Err(); err != nil {
			s.logger.Error("Failed to delete project cache keys",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
		}
	}
}

// ProvisionFromClaims provisions a project and user from JWT claims.
// This consolidates all provisioning logic - validates claims, creates project, and adds user.
// Idempotent - safe to call multiple times.
func (s *projectService) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	// Validate required claims
	if claims.ProjectID == "" {
		return nil, fmt.Errorf("missing project ID in claims")
	}
	if claims.Email == "" {
		return nil, fmt.Errorf("missing email in claims")
	}
	if claims.Subject == "" {
		return nil, fmt.Errorf("missing subject in claims")
	}

	// Parse project ID
	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("invalid project ID format: %w", err)
	}

	// Parse user ID from subject
	userID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return nil, fmt.Errorf("invalid user ID in subject: %w", err)
	}

	// Build parameters from claims
	params := map[string]interface{}{
		"roles": claims.Roles,
	}
	if claims.PAPI != "" {
		params["papi_url"] = claims.PAPI
	}

	// Use email as project name (ekaya-central has the actual name)
	projectName := claims.Email

	s.logger.Info("Provisioning project from claims",
		zap.String("project_id", claims.ProjectID),
		zap.String("email", claims.Email),
		zap.String("papi", claims.PAPI),
		zap.Strings("roles", claims.Roles))

	// Ensure project exists (idempotent)
	result, err := s.Provision(ctx, projectID, projectName, params)
	if err != nil {
		return nil, fmt.Errorf("failed to provision project: %w", err)
	}

	// Determine user role from claims
	userRole := models.RoleUser
	if len(claims.Roles) > 0 {
		userRole = claims.Roles[0]
	}

	// Validate role
	if !models.IsValidRole(userRole) {
		return nil, fmt.Errorf("%w: %s", apperrors.ErrInvalidRole, userRole)
	}

	// Ensure user exists (idempotent) - needs tenant context
	scope, err := s.db.WithTenant(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire tenant connection: %w", err)
	}
	defer scope.Close()

	ctxWithScope := database.SetTenantScope(ctx, scope)

	user := &models.User{
		ProjectID: projectID,
		UserID:    userID,
		Role:      userRole,
	}

	if err := s.userRepo.Add(ctxWithScope, user); err != nil {
		return nil, fmt.Errorf("failed to add user to project: %w", err)
	}

	s.logger.Info("User ensured in project",
		zap.String("project_id", projectID.String()),
		zap.String("user_id", userID.String()),
		zap.String("role", userRole))

	return result, nil
}

// Ensure projectService implements ProjectService at compile time.
var _ ProjectService = (*projectService)(nil)
