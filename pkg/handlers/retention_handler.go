package handlers

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// RetentionHandler handles audit retention configuration requests.
type RetentionHandler struct {
	mcpConfigRepo repositories.MCPConfigRepository
	logger        *zap.Logger
}

// NewRetentionHandler creates a new retention handler.
func NewRetentionHandler(mcpConfigRepo repositories.MCPConfigRepository, logger *zap.Logger) *RetentionHandler {
	return &RetentionHandler{
		mcpConfigRepo: mcpConfigRepo,
		logger:        logger,
	}
}

// RegisterRoutes registers the retention handler's routes on the given mux.
func (h *RetentionHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/audit/retention"

	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetRetention)))
	mux.HandleFunc("PUT "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.SetRetention))))
}

type retentionResponse struct {
	RetentionDays int  `json:"retention_days"`
	IsDefault     bool `json:"is_default"`
}

type setRetentionRequest struct {
	RetentionDays *int `json:"retention_days"` // nil = reset to default
}

// GetRetention handles GET /api/projects/{pid}/audit/retention
func (h *RetentionHandler) GetRetention(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	days, err := h.mcpConfigRepo.GetAuditRetentionDays(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get audit retention days",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get retention configuration")
		return
	}

	resp := retentionResponse{
		RetentionDays: services.DefaultRetentionDays,
		IsDefault:     true,
	}
	if days != nil {
		resp.RetentionDays = *days
		resp.IsDefault = false
	}

	if err := WriteJSON(w, http.StatusOK, resp); err != nil {
		h.logger.Error("Failed to write retention response", zap.Error(err))
	}
}

// SetRetention handles PUT /api/projects/{pid}/audit/retention
func (h *RetentionHandler) SetRetention(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var req setRetentionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	// Validate: if provided, must be between 1 and 365
	if req.RetentionDays != nil {
		days := *req.RetentionDays
		if days < 1 || days > 365 {
			ErrorResponse(w, http.StatusBadRequest, "invalid_parameters", "retention_days must be between 1 and 365")
			return
		}
	}

	if err := h.mcpConfigRepo.SetAuditRetentionDays(r.Context(), projectID, req.RetentionDays); err != nil {
		h.logger.Error("Failed to set audit retention days",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to update retention configuration")
		return
	}

	resp := retentionResponse{
		RetentionDays: services.DefaultRetentionDays,
		IsDefault:     true,
	}
	if req.RetentionDays != nil {
		resp.RetentionDays = *req.RetentionDays
		resp.IsDefault = false
	}

	if err := WriteJSON(w, http.StatusOK, resp); err != nil {
		h.logger.Error("Failed to write retention response", zap.Error(err))
	}
}
