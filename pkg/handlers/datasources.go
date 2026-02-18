package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// DatasourceResponse matches frontend Datasource interface.
type DatasourceResponse struct {
	DatasourceID     string         `json:"datasource_id"`
	ProjectID        string         `json:"project_id"`
	Name             string         `json:"name"`
	Type             string         `json:"type"`
	Provider         string         `json:"provider,omitempty"`
	Config           map[string]any `json:"config"`
	CreatedAt        string         `json:"created_at"`
	UpdatedAt        string         `json:"updated_at"`
	DecryptionFailed bool           `json:"decryption_failed,omitempty"`
	ErrorMessage     string         `json:"error_message,omitempty"`
}

// ListDatasourcesResponse wraps array for frontend compatibility.
type ListDatasourcesResponse struct {
	Datasources []DatasourceResponse `json:"datasources"`
}

// CreateDatasourceRequest for POST body.
type CreateDatasourceRequest struct {
	ProjectID string         `json:"project_id"`
	Name      string         `json:"name"`
	Type      string         `json:"type"`
	Provider  string         `json:"provider,omitempty"`
	Config    map[string]any `json:"config"`
}

// UpdateDatasourceRequest for PUT body.
type UpdateDatasourceRequest struct {
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Provider string         `json:"provider,omitempty"`
	Config   map[string]any `json:"config"`
}

// RenameDatasourceRequest for PATCH name body.
type RenameDatasourceRequest struct {
	Name string `json:"name"`
}

// TestConnectionRequest for connection testing.
// Matches frontend's flat structure (type + config fields at top level).
type TestConnectionRequest struct {
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Name     string `json:"name"`
	SSLMode  string `json:"ssl_mode"`

	// MSSQL-specific fields
	AuthMethod             string `json:"auth_method"`
	TenantID               string `json:"tenant_id"`
	ClientID               string `json:"client_id"`
	ClientSecret           string `json:"client_secret"`
	Encrypt                *bool  `json:"encrypt"`
	TrustServerCertificate *bool  `json:"trust_server_certificate"`
	ConnectionTimeout      *int   `json:"connection_timeout"`
}

// ToConfig converts the flat request to a config map for the service layer.
func (r *TestConnectionRequest) ToConfig() map[string]any {
	config := map[string]any{
		"host":     r.Host,
		"port":     r.Port,
		"user":     r.User,
		"password": r.Password,
		"database": r.Name,
		"name":     r.Name,
		"ssl_mode": r.SSLMode,
	}

	// Add user/password if provided (for SQL auth or PostgreSQL)
	if r.User != "" {
		config["user"] = r.User
	}
	if r.Password != "" {
		config["password"] = r.Password
	}

	// Add MSSQL-specific fields if provided
	if r.AuthMethod != "" {
		config["auth_method"] = r.AuthMethod
	}
	if r.TenantID != "" {
		config["tenant_id"] = r.TenantID
	}
	if r.ClientID != "" {
		config["client_id"] = r.ClientID
	}
	if r.ClientSecret != "" {
		config["client_secret"] = r.ClientSecret
	}
	if r.Encrypt != nil {
		config["encrypt"] = *r.Encrypt
	}
	if r.TrustServerCertificate != nil {
		config["trust_server_certificate"] = *r.TrustServerCertificate
	}
	if r.ConnectionTimeout != nil {
		config["connection_timeout"] = *r.ConnectionTimeout
	}

	return config
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

// ApiResponse wraps data in the format expected by the frontend.
type ApiResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
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
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Create))))
	mux.HandleFunc("GET /api/projects/{pid}/datasources/{id}",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("PUT /api/projects/{pid}/datasources/{id}",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Update))))
	mux.HandleFunc("PATCH /api/projects/{pid}/datasources/{id}/name",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Rename))))
	mux.HandleFunc("DELETE /api/projects/{pid}/datasources/{id}",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Delete))))
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

	data := ListDatasourcesResponse{
		Datasources: make([]DatasourceResponse, len(datasources)),
	}
	for i, dsWithStatus := range datasources {
		data.Datasources[i] = DatasourceResponse{
			DatasourceID:     dsWithStatus.ID.String(),
			ProjectID:        dsWithStatus.ProjectID.String(),
			Name:             dsWithStatus.Name,
			Type:             dsWithStatus.DatasourceType,
			Provider:         dsWithStatus.Provider,
			Config:           dsWithStatus.Config,
			CreatedAt:        dsWithStatus.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
			UpdatedAt:        dsWithStatus.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			DecryptionFailed: dsWithStatus.DecryptionFailed,
			ErrorMessage:     dsWithStatus.ErrorMessage,
		}
	}

	response := ApiResponse{Success: true, Data: data}
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

	if req.Name == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_name", "Datasource name is required"); err != nil {
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

	ds, err := h.datasourceService.Create(r.Context(), projectID, req.Name, req.Type, req.Provider, req.Config)
	if err != nil {
		if errors.Is(err, apperrors.ErrDatasourceLimitReached) {
			if err := ErrorResponse(w, http.StatusConflict, "datasource_limit_reached", "Only one datasource per project is currently supported"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		if errors.Is(err, apperrors.ErrConflict) {
			if err := ErrorResponse(w, http.StatusConflict, "duplicate_name", "A datasource with this name already exists"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Failed to create datasource",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "create_failed", "Failed to create datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	data := DatasourceResponse{
		DatasourceID: ds.ID.String(),
		ProjectID:    ds.ProjectID.String(),
		Name:         ds.Name,
		Type:         ds.DatasourceType,
		Provider:     ds.Provider,
		Config:       ds.Config,
		CreatedAt:    ds.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    ds.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	response := ApiResponse{Success: true, Data: data}
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

	data := DatasourceResponse{
		DatasourceID: ds.ID.String(),
		ProjectID:    ds.ProjectID.String(),
		Name:         ds.Name,
		Type:         ds.DatasourceType,
		Provider:     ds.Provider,
		Config:       ds.Config,
		CreatedAt:    ds.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:    ds.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}

	response := ApiResponse{Success: true, Data: data}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Rename handles PATCH /api/projects/{pid}/datasources/{id}/name
// Renames a datasource without requiring a connection test.
func (h *DatasourcesHandler) Rename(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")

	datasourceID, err := uuid.Parse(idStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_datasource_id", "Invalid datasource ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req RenameDatasourceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Name == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_name", "Datasource name is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.datasourceService.Rename(r.Context(), datasourceID, req.Name); err != nil {
		h.logger.Error("Failed to rename datasource",
			zap.String("datasource_id", datasourceID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "rename_failed", "Failed to rename datasource"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{Success: true, Data: map[string]string{
		"datasource_id": datasourceID.String(),
		"name":          req.Name,
	}}
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

	if req.Name == "" {
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_name", "Datasource name is required"); err != nil {
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

	if err := h.datasourceService.Update(r.Context(), datasourceID, req.Name, req.Type, req.Provider, req.Config); err != nil {
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

	// Return success response
	data := map[string]any{
		"datasource_id": datasourceID.String(),
		"name":          req.Name,
		"type":          req.Type,
		"provider":      req.Provider,
		"config":        req.Config,
	}

	response := ApiResponse{Success: true, Data: data}
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

	data := DeleteDatasourceResponse{
		Success: true,
		Message: "Datasource deleted successfully",
	}

	response := ApiResponse{Success: true, Data: data}
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
		h.logger.Debug("Invalid project ID", zap.String("pid", pidStr), zap.Error(err))
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req TestConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error("Failed to decode request body", zap.Error(err))
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if req.Type == "" {
		h.logger.Debug("Missing datasource type in request")
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_type", "Datasource type is required"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	config := req.ToConfig()

	// Pass uuid.Nil for datasourceID since this is testing a new/unsaved datasource config
	if err := h.datasourceService.TestConnection(r.Context(), req.Type, config, uuid.Nil); err != nil {
		h.logger.Info("Connection test failed",
			zap.String("type", req.Type),
			zap.String("host", req.Host),
			zap.Error(err))
		if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: false, Error: err.Error()}); err != nil {
			h.logger.Error("Failed to write response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Message: "Connection successful"}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
