package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// ProjectResponse contains the response for a project request.
type ProjectResponse struct {
	Project   interface{} `json:"project"`
	UserID    string      `json:"user_id"`
	UserEmail string      `json:"user_email,omitempty"`
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
// Uses Go 1.22+ path parameters: {pid}
func (h *ProjectsHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware) {
	// GET /api/projects/{pid} - requires auth with path validation
	mux.HandleFunc("GET /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(h.Get))
}

// Get handles GET /api/projects/{pid}
// Returns the project details along with the authenticated user's info.
func (h *ProjectsHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Get project ID from path (already validated by middleware)
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		// This shouldn't happen if middleware validated properly
		h.errorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format")
		return
	}

	// Get claims from context (set by middleware)
	claims, ok := auth.GetClaims(r.Context())
	if !ok {
		h.errorResponse(w, http.StatusInternalServerError, "internal_error", "Claims not found in context")
		return
	}

	// Call service to get project
	project, err := h.projectService.GetByID(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get project",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		h.errorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get project")
		return
	}

	// Return project with user info from claims
	response := ProjectResponse{
		Project:   project,
		UserID:    claims.Subject,
		UserEmail: claims.Email,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.logger.Error("Failed to encode response", zap.Error(err))
		return
	}
}

// errorResponse writes a JSON error response.
func (h *ProjectsHandler) errorResponse(w http.ResponseWriter, statusCode int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   errorCode,
		"message": message,
	})
}
