package handlers

import (
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/tunnel"
)

// TunnelStatusHandler handles tunnel status HTTP requests.
type TunnelStatusHandler struct {
	tunnelManager *tunnel.Manager
	logger        *zap.Logger
}

// NewTunnelStatusHandler creates a new tunnel status handler.
func NewTunnelStatusHandler(tunnelManager *tunnel.Manager, logger *zap.Logger) *TunnelStatusHandler {
	return &TunnelStatusHandler{
		tunnelManager: tunnelManager,
		logger:        logger,
	}
}

// RegisterRoutes registers the tunnel status endpoint.
func (h *TunnelStatusHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	mux.HandleFunc("GET /api/projects/{pid}/apps/mcp-tunnel/status",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Status)))
}

// Status handles GET /api/projects/{pid}/apps/mcp-tunnel/status
func (h *TunnelStatusHandler) Status(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	status := h.tunnelManager.Status(projectID)

	response := ApiResponse{Success: true, Data: status}
	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
