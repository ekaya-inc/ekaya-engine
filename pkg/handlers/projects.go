package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// TenantMiddleware is a function that wraps a handler with tenant context.
type TenantMiddleware func(http.HandlerFunc) http.HandlerFunc

// ProjectResponse is the standard response for project endpoints.
type ProjectResponse struct {
	Status          string `json:"status"`
	PID             string `json:"pid"`
	Name            string `json:"name,omitempty"`
	PAPIURL         string `json:"papi_url,omitempty"`
	ProjectsPageURL string `json:"projects_page_url,omitempty"`
	ProjectPageURL  string `json:"project_page_url,omitempty"`
}

// ProjectsHandler handles project-related HTTP requests.
type ProjectsHandler struct {
	projectService services.ProjectService
	cfg            *config.Config
	logger         *zap.Logger
}

// NewProjectsHandler creates a new projects handler.
func NewProjectsHandler(projectService services.ProjectService, cfg *config.Config, logger *zap.Logger) *ProjectsHandler {
	return &ProjectsHandler{
		projectService: projectService,
		cfg:            cfg,
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
	mux.HandleFunc("PATCH /api/projects/{pid}/auth-server-url",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.UpdateAuthServerURL)))
	mux.HandleFunc("POST /api/projects/{pid}/sync-server-url",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.SyncServerURL)))
}

// GetCurrent handles GET /projects
// Returns project info using project ID from JWT claims.
// Triggers async sync from ekaya-central to update project name if changed.
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

	// Trigger async sync from ekaya-central if we have PAPI URL and token
	if token, hasToken := auth.GetToken(r.Context()); hasToken && claims.PAPI != "" {
		h.projectService.SyncFromCentralAsync(projectID, claims.PAPI, token)
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
		Status:          "success",
		PID:             result.ProjectID.String(),
		Name:            result.Name,
		PAPIURL:         result.PAPIURL,
		ProjectsPageURL: result.ProjectsPageURL,
		ProjectPageURL:  result.ProjectPageURL,
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

// UpdateAuthServerURLRequest is the request body for updating auth server URL.
type UpdateAuthServerURLRequest struct {
	AuthServerURL string `json:"auth_server_url"`
}

// UpdateAuthServerURL handles PATCH /api/projects/{pid}/auth-server-url
// Updates the auth_server_url in project parameters after validation.
func (h *ProjectsHandler) UpdateAuthServerURL(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req UpdateAuthServerURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate auth_server_url against whitelist
	_, errMsg := h.cfg.ValidateAuthURL(req.AuthServerURL)
	if errMsg != "" {
		h.logger.Warn("Auth server URL validation failed",
			zap.String("project_id", projectID.String()),
			zap.String("auth_server_url", req.AuthServerURL),
			zap.String("error", errMsg))
		if err := ErrorResponse(w, http.StatusForbidden, "auth_url_not_allowed", "Auth server URL not in allowed list"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.projectService.UpdateAuthServerURL(r.Context(), projectID, req.AuthServerURL); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Project not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to update auth server URL",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to update auth server URL"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// SyncServerURL handles POST /api/projects/{pid}/sync-server-url
// Pushes the engine's current base_url to ekaya-central so redirect URLs
// and MCP setup links reflect the server's actual address.
func (h *ProjectsHandler) SyncServerURL(w http.ResponseWriter, r *http.Request) {
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		if err := ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "Missing authentication"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if claims.PAPI == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_papi", "JWT does not contain ekaya-central API URL"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	token, hasToken := auth.GetToken(r.Context())
	if !hasToken {
		if err := ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "Missing token"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	pidStr := r.PathValue("pid")
	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.projectService.SyncServerURL(r.Context(), projectID, claims.PAPI, token); err != nil {
		h.logger.Error("Failed to sync server URL to ekaya-central",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "sync_failed", "Failed to sync server URL to ekaya-central"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, map[string]string{
		"status":     "success",
		"server_url": h.cfg.BaseURL,
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// buildProjectResponse creates a ProjectResponse from a Project model.
func (h *ProjectsHandler) buildProjectResponse(project *models.Project) ProjectResponse {
	var papiURL, projectsPageURL, projectPageURL string
	if project.Parameters != nil {
		if v, ok := project.Parameters["papi_url"].(string); ok {
			papiURL = v
		}
		if v, ok := project.Parameters["projects_page_url"].(string); ok {
			projectsPageURL = v
		}
		if v, ok := project.Parameters["project_page_url"].(string); ok {
			projectPageURL = v
		}
	}

	return ProjectResponse{
		Status:          "success",
		PID:             project.ID.String(),
		Name:            project.Name,
		PAPIURL:         papiURL,
		ProjectsPageURL: projectsPageURL,
		ProjectPageURL:  projectPageURL,
	}
}
