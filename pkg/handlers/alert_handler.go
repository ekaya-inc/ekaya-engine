package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// AlertHandler handles audit alert HTTP requests.
type AlertHandler struct {
	alertService  services.AlertService
	mcpConfigRepo repositories.MCPConfigRepository
	logger        *zap.Logger
}

// NewAlertHandler creates a new alert handler.
func NewAlertHandler(alertService services.AlertService, mcpConfigRepo repositories.MCPConfigRepository, logger *zap.Logger) *AlertHandler {
	return &AlertHandler{
		alertService:  alertService,
		mcpConfigRepo: mcpConfigRepo,
		logger:        logger,
	}
}

// RegisterRoutes registers the alert handler's routes on the given mux.
func (h *AlertHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/audit/alerts"

	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListAlerts)))
	mux.HandleFunc("POST "+base+"/{alert_id}/resolve",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin, models.RoleData)(tenantMiddleware(h.ResolveAlert))))

	configBase := "/api/projects/{pid}/audit/alert-config"
	mux.HandleFunc("GET "+configBase,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetAlertConfig)))
	mux.HandleFunc("PUT "+configBase,
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.SetAlertConfig))))
}

// ListAlerts handles GET /api/projects/{pid}/audit/alerts
func (h *AlertHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	filters := models.AlertFilters{
		AuditPageFilters: parsePageFilters(r),
		Status:           r.URL.Query().Get("status"),
		Severity:         r.URL.Query().Get("severity"),
	}

	// Default to open alerts if no status filter provided
	if filters.Status == "" {
		filters.Status = models.AlertStatusOpen
	}

	results, total, err := h.alertService.ListAlerts(r.Context(), projectID, filters)
	if err != nil {
		h.logger.Error("Failed to list alerts", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "list_alerts_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if results == nil {
		results = make([]*models.AuditAlert, 0)
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data: PaginatedResponse{
			Items:  results,
			Total:  total,
			Limit:  filters.Limit,
			Offset: filters.Offset,
		},
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

type resolveAlertRequest struct {
	Resolution string `json:"resolution"` // "resolved" or "dismissed"
	Notes      string `json:"notes"`
}

// ResolveAlert handles POST /api/projects/{pid}/audit/alerts/{alert_id}/resolve
func (h *AlertHandler) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	alertIDStr := r.PathValue("alert_id")
	alertID, err := uuid.Parse(alertIDStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_alert_id", "Invalid alert ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req resolveAlertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if !models.ValidAlertResolution(req.Resolution) {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_resolution", "Resolution must be 'resolved' or 'dismissed'"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	resolvedBy := auth.GetUserIDFromContext(r.Context())
	if resolvedBy == "" {
		if err := ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "User ID not found in context"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.alertService.ResolveAlert(r.Context(), projectID, alertID, resolvedBy, req.Resolution, req.Notes); err != nil {
		h.logger.Error("Failed to resolve alert", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "resolve_alert_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Message: "Alert resolved successfully",
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// GetAlertConfig handles GET /api/projects/{pid}/audit/alert-config
func (h *AlertHandler) GetAlertConfig(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	config, err := h.mcpConfigRepo.GetAlertConfig(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get alert config", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get alert configuration"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if config == nil {
		config = models.DefaultAlertConfig()
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data:    config,
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// SetAlertConfig handles PUT /api/projects/{pid}/audit/alert-config
func (h *AlertHandler) SetAlertConfig(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	var config models.AlertConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate severity values in alert settings
	for alertType, setting := range config.AlertSettings {
		if setting.Severity != "" && !models.ValidAlertSeverity(setting.Severity) {
			if err := ErrorResponse(w, http.StatusBadRequest, "invalid_severity",
				"Invalid severity for alert type "+alertType+": "+setting.Severity); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
	}

	if err := h.mcpConfigRepo.SetAlertConfig(r.Context(), projectID, &config); err != nil {
		h.logger.Error("Failed to set alert config", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to update alert configuration"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{
		Success: true,
		Data:    &config,
	}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
