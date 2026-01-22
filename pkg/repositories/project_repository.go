package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ProjectRepository defines the interface for project data access.
type ProjectRepository interface {
	Create(ctx context.Context, project *models.Project) error
	Get(ctx context.Context, id uuid.UUID) (*models.Project, error)
	Update(ctx context.Context, project *models.Project) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// projectRepository implements ProjectRepository using PostgreSQL.
type projectRepository struct{}

// NewProjectRepository creates a new project repository.
func NewProjectRepository() ProjectRepository {
	return &projectRepository{}
}

// Create inserts a new project or updates if it already exists (idempotent).
// Uses ON CONFLICT for safe retry behavior during provisioning.
func (r *projectRepository) Create(ctx context.Context, project *models.Project) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	// Generate UUID v4 if not provided
	if project.ID == uuid.Nil {
		project.ID = uuid.New()
	}

	now := time.Now()
	project.CreatedAt = now
	project.UpdatedAt = now
	if project.Status == "" {
		project.Status = "active"
	}
	if project.IndustryType == "" {
		project.IndustryType = models.IndustryGeneral
	}

	params, err := json.Marshal(project.Parameters)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	query := `
		INSERT INTO engine_projects (id, name, parameters, created_at, updated_at, status, industry_type)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE
		SET name = EXCLUDED.name,
		    parameters = EXCLUDED.parameters,
		    updated_at = EXCLUDED.updated_at,
		    status = EXCLUDED.status,
		    industry_type = EXCLUDED.industry_type`

	_, err = scope.Conn.Exec(ctx, query,
		project.ID,
		project.Name,
		params,
		project.CreatedAt,
		project.UpdatedAt,
		project.Status,
		project.IndustryType,
	)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}

	return nil
}

// Get retrieves a project by ID.
func (r *projectRepository) Get(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return nil, fmt.Errorf("no tenant scope in context")
	}

	query := `
		SELECT id, name, parameters, created_at, updated_at, status, COALESCE(industry_type, 'general')
		FROM engine_projects
		WHERE id = $1`

	var project models.Project
	var params []byte

	err := scope.Conn.QueryRow(ctx, query, id).Scan(
		&project.ID,
		&project.Name,
		&params,
		&project.CreatedAt,
		&project.UpdatedAt,
		&project.Status,
		&project.IndustryType,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, apperrors.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	if err := json.Unmarshal(params, &project.Parameters); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	return &project, nil
}

// Update updates an existing project's parameters.
func (r *projectRepository) Update(ctx context.Context, project *models.Project) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	project.UpdatedAt = time.Now()
	if project.IndustryType == "" {
		project.IndustryType = models.IndustryGeneral
	}

	params, err := json.Marshal(project.Parameters)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	query := `
		UPDATE engine_projects
		SET name = $2, parameters = $3, updated_at = $4, industry_type = $5
		WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, project.ID, project.Name, params, project.UpdatedAt, project.IndustryType)
	if err != nil {
		return fmt.Errorf("failed to update project: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

// Delete removes a project by ID.
// Related users are automatically deleted via CASCADE.
func (r *projectRepository) Delete(ctx context.Context, id uuid.UUID) error {
	scope, ok := database.GetTenantScope(ctx)
	if !ok {
		return fmt.Errorf("no tenant scope in context")
	}

	query := `DELETE FROM engine_projects WHERE id = $1`

	result, err := scope.Conn.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete project: %w", err)
	}

	if result.RowsAffected() == 0 {
		return apperrors.ErrNotFound
	}

	return nil
}

// Ensure projectRepository implements ProjectRepository at compile time.
var _ ProjectRepository = (*projectRepository)(nil)
