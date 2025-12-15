package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// DatasourceResponse matches frontend Datasource interface.
type DatasourceResponse struct {
	DatasourceID string         `json:"datasource_id"`
	ProjectID    string         `json:"project_id"`
	Type         string         `json:"type"`
	Config       map[string]any `json:"config"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

// ListDatasourcesResponse wraps array for frontend compatibility.
type ListDatasourcesResponse struct {
	Datasources []DatasourceResponse `json:"datasources"`
}

// CreateDatasourceRequest for POST body.
type CreateDatasourceRequest struct {
	ProjectID string         `json:"project_id"`
	Type      string         `json:"type"`
	Config    map[string]any `json:"config"`
}

// UpdateDatasourceRequest for PUT body.
type UpdateDatasourceRequest struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// TestConnectionRequest for connection testing.
type TestConnectionRequest struct {
	Type   string         `json:"type"`
	Config map[string]any `json:"config"`
}

// TestConnectionResponse for connection test result.
type TestConnectionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// DeleteDatasourceResponse for delete result.
type DeleteDatasourceResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// DatasourcesHandler handles datasource-related HTTP requests.
type DatasourcesHandler struct {
	datasourceService services.DatasourceService
	logger            *zap.Logger
}

// NewDatasourcesHandler creates a new datasources handler.
func NewDatasourcesHandler(datasourceService services.DatasourceService, logger *zap.Logger) *DatasourcesHandler {
	return &DatasourcesHandler{
		datasourceService: datasourceService,
		logger:            logger,
	}
}

// RegisterRoutes registers the datasources handler's routes on the given mux.
func (h *DatasourcesHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	// All datasource routes are project-scoped and require authentication + tenant context
	mux.HandleFunc("GET /api/projects/{pid}/datasources",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.List)))
	mux.HandleFunc("POST /api/projects/{pid}/datasources",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Create)))
	mux.HandleFunc("GET /api/projects/{pid}/datasources/{id}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("PUT /api/projects/{pid}/datasources/{id}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Update)))
	mux.HandleFunc("DELETE /api/projects/{pid}/datasources/{id}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Delete)))
	mux.HandleFunc("POST /api/projects/{pid}/datasources/test",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.TestConnection)))
}

// List handles GET /api/projects/{pid}/datasources
// Returns all datasources for the project.
func (h *DatasourcesHandler) List(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	datasources, err := h.datasourceService.List(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to list datasources",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to list datasources"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ListDatasourcesResponse{
		Datasources: make([]DatasourceResponse, len(datasources)),
	}
	for i, ds := range datasources {
		response.Datasources[i] = DatasourceResponse{
			DatasourceID: ds.ID.String(),
			ProjectID:    ds.ProjectID.String(),
			Type:         ds.DatasourceType,
			Config:       maskSensitiveConfig(ds.Config),
			CreatedAt:    ds.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:    ds.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Create handles POST /api/projects/{pid}/datasources
// Creates a new datasource for the project.
func (h *DatasourcesHandler) Create(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req CreateDatasourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Type == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_type", "Datasource type is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Extract name from config (frontend sends database name in config.name)
	name := ""
	if req.Config != nil {
		if n, ok := req.Config["name"].(string); ok {
			name = n
		}
	}
	if name == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_name", "Datasource name is required in config"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	ds, err := h.datasourceService.Create(r.Context(), projectID, name, req.Type, req.Config)
	if err != nil {
		h.logger.Error("Failed to create datasource",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "create_failed", "Failed to create datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := DatasourceResponse{
		DatasourceID: ds.ID.String(),
		ProjectID:    ds.ProjectID.String(),
		Type:         ds.DatasourceType,
		Config:       maskSensitiveConfig(ds.Config),
		CreatedAt:    ds.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    ds.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if err := WriteJSON(w, http.StatusCreated, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Get handles GET /api/projects/{pid}/datasources/{id}
// Returns a single datasource by ID.
func (h *DatasourcesHandler) Get(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	idStr := r.PathValue("id")

	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	datasourceID, err := uuid.Parse(idStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_datasource_id", "Invalid datasource ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	ds, err := h.datasourceService.Get(r.Context(), projectID, datasourceID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Datasource not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to get datasource",
			zap.String("project_id", projectID.String()),
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := DatasourceResponse{
		DatasourceID: ds.ID.String(),
		ProjectID:    ds.ProjectID.String(),
		Type:         ds.DatasourceType,
		Config:       maskSensitiveConfig(ds.Config),
		CreatedAt:    ds.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    ds.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Update handles PUT /api/projects/{pid}/datasources/{id}
// Updates an existing datasource.
func (h *DatasourcesHandler) Update(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	idStr := r.PathValue("id")

	_, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	datasourceID, err := uuid.Parse(idStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_datasource_id", "Invalid datasource ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req UpdateDatasourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Type == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_type", "Datasource type is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Extract name from config
	name := ""
	if req.Config != nil {
		if n, ok := req.Config["name"].(string); ok {
			name = n
		}
	}
	if name == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_name", "Datasource name is required in config"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.datasourceService.Update(r.Context(), datasourceID, name, req.Type, req.Config); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Datasource not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to update datasource",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "update_failed", "Failed to update datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Return success response (frontend expects UpdateDatasourceResponse without created_at)
	response := map[string]any{
		"datasource_id": datasourceID.String(),
		"type":          req.Type,
		"config":        maskSensitiveConfig(req.Config),
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Delete handles DELETE /api/projects/{pid}/datasources/{id}
// Deletes a datasource.
func (h *DatasourcesHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	idStr := r.PathValue("id")

	_, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	datasourceID, err := uuid.Parse(idStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_datasource_id", "Invalid datasource ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.datasourceService.Delete(r.Context(), datasourceID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			if err := ErrorResponse(w, http.StatusNotFound, "not_found", "Datasource not found"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to delete datasource",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "delete_failed", "Failed to delete datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := DeleteDatasourceResponse{
		Success: true,
		Message: "Datasource deleted successfully",
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// TestConnection handles POST /api/projects/{pid}/datasources/test
// Tests connection to a datasource without saving it.
func (h *DatasourcesHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")

	_, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req TestConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Type == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_type", "Datasource type is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.datasourceService.TestConnection(r.Context(), req.Type, req.Config); err != nil {
		response := TestConnectionResponse{
			Success: false,
			Message: err.Error(),
		}
		if err := WriteJSON(w, http.StatusOK, response); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	response := TestConnectionResponse{
		Success: true,
		Message: "Connection successful",
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// maskSensitiveConfig removes sensitive fields from config before sending to client.
func maskSensitiveConfig(config map[string]any) map[string]any {
	if config == nil {
		return nil
	}
	masked := make(map[string]any)
	for k, v := range config {
		if k == "password" {
			masked[k] = "********"
		} else {
			masked[k] = v
		}
	}
	return masked
}
