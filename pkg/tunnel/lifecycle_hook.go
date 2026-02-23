package tunnel

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// LifecycleHook implements handlers.AppLifecycleHook to start/stop tunnels
// when the mcp-tunnel app is activated or uninstalled.
type LifecycleHook struct {
	manager *Manager
	logger  *zap.Logger
}

// NewLifecycleHook creates a new lifecycle hook for the tunnel manager.
func NewLifecycleHook(manager *Manager, logger *zap.Logger) *LifecycleHook {
	return &LifecycleHook{
		manager: manager,
		logger:  logger,
	}
}

// OnAppActivated is called when an app is successfully activated.
// Starts a tunnel if the activated app is mcp-tunnel.
func (h *LifecycleHook) OnAppActivated(ctx context.Context, projectID uuid.UUID, appID string) {
	if appID != models.AppIDMCPTunnel {
		return
	}

	h.logger.Info("mcp-tunnel activated, starting tunnel",
		zap.String("project_id", projectID.String()),
	)

	if err := h.manager.StartTunnel(ctx, projectID); err != nil {
		h.logger.Error("Failed to start tunnel after activation",
			zap.String("project_id", projectID.String()),
			zap.Error(err),
		)
	}
}

// OnAppUninstalled is called when an app is successfully uninstalled.
// Stops the tunnel if the uninstalled app is mcp-tunnel.
func (h *LifecycleHook) OnAppUninstalled(_ context.Context, projectID uuid.UUID, appID string) {
	if appID != models.AppIDMCPTunnel {
		return
	}

	h.logger.Info("mcp-tunnel uninstalled, stopping tunnel",
		zap.String("project_id", projectID.String()),
	)

	h.manager.StopTunnel(projectID)
}
