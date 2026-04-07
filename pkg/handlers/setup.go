package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

type SetupHandler struct {
	setupStateService services.SetupStateService
	logger            *zap.Logger
}

func NewSetupHandler(setupStateService services.SetupStateService, logger *zap.Logger) *SetupHandler {
	return &SetupHandler{
		setupStateService: setupStateService,
		logger:            logger,
	}
}

func (h *SetupHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	mux.HandleFunc("GET /api/projects/{pid}/setup-status",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.GetStatus)))
}

func (h *SetupHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	status, err := h.setupStateService.GetSetupStatus(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get setup status",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "get_setup_status_failed", "Failed to get setup status"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: status}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
