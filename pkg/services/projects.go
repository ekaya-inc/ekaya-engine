package services

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ProjectService defines the interface for project operations.
type ProjectService interface {
	// GetByID returns a project by its ID.
	GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error)
}

// projectService is a dummy implementation of ProjectService.
// It returns placeholder data until database support is added.
type projectService struct{}

// NewProjectService creates a new project service.
func NewProjectService() ProjectService {
	return &projectService{}
}

// GetByID returns a dummy project with the given ID.
// This is a placeholder implementation until database support is added.
func (s *projectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	// Return a placeholder project
	return &models.Project{
		ID:   id,
		Name: "Project " + id.String()[:8],
	}, nil
}

// Ensure projectService implements ProjectService at compile time.
var _ ProjectService = (*projectService)(nil)
