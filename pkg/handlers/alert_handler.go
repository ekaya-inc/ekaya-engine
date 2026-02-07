package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// AlertHandler handles audit alert HTTP requests.
type AlertHandler struct {
	alertService services.AlertService
	logger       *zap.Logger
}

// NewAlertHandler creates a new alert handler.
func NewAlertHandler(alertService services.AlertService, logger *zap.Logger) *AlertHandler {
	return &AlertHandler{
		alertService: alertService,
		logger:       logger,
	}
}

// RegisterRoutes registers the alert handler's routes on the given mux.
func (h *AlertHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	base := "/api/projects/{pid}/audit/alerts"

	mux.HandleFunc("GET "+base,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ListAlerts)))
	mux.HandleFunc("POST "+base+"/{alert_id}/resolve",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.ResolveAlert)))
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
