package handlers

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// TenantMiddleware is a function that wraps a handler with tenant context.
type TenantMiddleware func(http.HandlerFunc) http.HandlerFunc

// ProjectResponse is the standard response for project endpoints.
type ProjectResponse struct {
	Status  string `json:"status"`
	PID     string `json:"pid"`
	Name    string `json:"name,omitempty"`
	PAPIURL string `json:"papi_url,omitempty"`
}

// ProjectsHandler handles project-related HTTP requests.
type ProjectsHandler struct {
	projectService services.ProjectService
	logger         *zap.Logger
}

// NewProjectsHandler creates a new projects handler.
func NewProjectsHandler(projectService services.ProjectService, logger *zap.Logger) *ProjectsHandler {
	return &ProjectsHandler{
		projectService: projectService,
		logger:         logger,
	}
}

// RegisterRoutes registers the projects handler's routes on the given mux.
func (h *ProjectsHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// Frontend routes (no /api prefix)
	mux.HandleFunc("GET /projects", authMiddleware.RequireAuth(h.GetCurrent))
	mux.HandleFunc("POST /projects", authMiddleware.RequireAuth(h.Provision))

	// API routes
	mux.HandleFunc("GET /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("DELETE /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Delete)))
}

// GetCurrent handles GET /projects
// Returns project info using project ID from JWT claims.
func (h *ProjectsHandler) GetCurrent(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetClaims(r.Context())
	if !ok || claims.ProjectID == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_project_id", "Project ID required in token"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	projectID, err := uuid.Parse(claims.ProjectID)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format in token"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	project, err := h.projectService.GetByIDWithoutTenant(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Project not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to get project",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get project"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := h.buildProjectResponse(project)
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Provision handles POST /projects
// Provisions project and user from JWT claims. Idempotent.
func (h *ProjectsHandler) Provision(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		if err := ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "Missing authentication"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	result, err := h.projectService.ProvisionFromClaims(r.Context(), claims)
	if err != nil {
		h.logger.Error("Failed to provision project", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "provision_failed", "Failed to provision project"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ProjectResponse{
		Status:  "success",
		PID:     result.ProjectID.String(),
		Name:    result.Name,
		PAPIURL: result.PAPIURL,
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Get handles GET /api/projects/{pid}
// Returns the project details.
func (h *ProjectsHandler) Get(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	project, err := h.projectService.GetByID(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Project not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to get project",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get project"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := h.buildProjectResponse(project)
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}
// Deletes a project and all associated data.
func (h *ProjectsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.projectService.Delete(r.Context(), projectID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Project not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to delete project",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to delete project"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// buildProjectResponse creates a ProjectResponse from a Project model.
func (h *ProjectsHandler) buildProjectResponse(project *models.Project) ProjectResponse {
	var papiURL string
	if project.Parameters != nil {
		if papi, ok := project.Parameters["papi_url"].(string); ok {
			papiURL = papi
		}
	}

	return ProjectResponse{
		Status:  "success",
		PID:     project.ID.String(),
		Name:    project.Name,
		PAPIURL: papiURL,
	}
}
