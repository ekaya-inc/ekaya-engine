package services

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// InstalledAppService orchestrates installed app management.
type InstalledAppService interface {
	// ListInstalled returns all installed apps for a project.
	// Always includes mcp-server as "always installed".
	ListInstalled(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error)

	// IsInstalled checks if a specific app is installed.
	IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error)

	// Install installs an app for a project.
	Install(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*models.InstalledApp, error)

	// Uninstall removes an app and clears its settings.
	Uninstall(ctx context.Context, projectID uuid.UUID, appID string) error

	// GetSettings returns app-specific settings.
	GetSettings(ctx context.Context, projectID uuid.UUID, appID string) (map[string]any, error)

	// UpdateSettings updates app-specific settings.
	UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]any) error

	// GetApp returns a specific installed app details.
	GetApp(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error)
}

type installedAppService struct {
	repo   repositories.InstalledAppRepository
	logger *zap.Logger
}

// NewInstalledAppService creates a new installed app service.
func NewInstalledAppService(repo repositories.InstalledAppRepository, logger *zap.Logger) InstalledAppService {
	return &installedAppService{
		repo:   repo,
		logger: logger,
	}
}

// ListInstalled returns all installed apps for a project.
// Always includes mcp-server as "always installed" (virtual app).
func (s *installedAppService) ListInstalled(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error) {
	apps, err := s.repo.List(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list installed apps: %w", err)
	}

	// Check if MCP Server is already in the list
	hasMCP := false
	for _, app := range apps {
		if app.AppID == models.AppIDMCPServer {
			hasMCP = true
			break
		}
	}

	// Always include MCP Server as "installed" (it's always available)
	if !hasMCP {
		mcpApp := &models.InstalledApp{
			ID:          uuid.Nil, // Zero UUID indicates virtual/always-installed
			ProjectID:   projectID,
			AppID:       models.AppIDMCPServer,
			InstalledAt: time.Time{}, // Zero time indicates "always installed"
			Settings:    make(map[string]any),
		}
		apps = append([]*models.InstalledApp{mcpApp}, apps...)
	}

	return apps, nil
}

// IsInstalled checks if a specific app is installed.
func (s *installedAppService) IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error) {
	// MCP Server is always installed
	if appID == models.AppIDMCPServer {
		return true, nil
	}

	return s.repo.IsInstalled(ctx, projectID, appID)
}

// Install installs an app for a project.
func (s *installedAppService) Install(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*models.InstalledApp, error) {
	// Validate appID is known
	if !models.KnownAppIDs[appID] {
		return nil, fmt.Errorf("unknown app: %s", appID)
	}

	// MCP Server cannot be installed (it's always available)
	if appID == models.AppIDMCPServer {
		return nil, fmt.Errorf("mcp-server is always installed")
	}

	// Check if already installed
	installed, err := s.repo.IsInstalled(ctx, projectID, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to check installation status: %w", err)
	}
	if installed {
		return nil, fmt.Errorf("app already installed")
	}

	app := &models.InstalledApp{
		ID:          uuid.New(),
		ProjectID:   projectID,
		AppID:       appID,
		InstalledAt: time.Now(),
		InstalledBy: userID,
		Settings:    make(map[string]any),
	}

	if err := s.repo.Install(ctx, app); err != nil {
		return nil, fmt.Errorf("failed to install app: %w", err)
	}

	s.logger.Info("App installed",
		zap.String("project_id", projectID.String()),
		zap.String("app_id", appID),
		zap.String("installed_by", userID),
	)

	return app, nil
}

// Uninstall removes an app and clears its settings.
func (s *installedAppService) Uninstall(ctx context.Context, projectID uuid.UUID, appID string) error {
	// MCP Server cannot be uninstalled
	if appID == models.AppIDMCPServer {
		return fmt.Errorf("mcp-server cannot be uninstalled")
	}

	if err := s.repo.Uninstall(ctx, projectID, appID); err != nil {
		return fmt.Errorf("failed to uninstall app: %w", err)
	}

	s.logger.Info("App uninstalled",
		zap.String("project_id", projectID.String()),
		zap.String("app_id", appID),
	)

	return nil
}

// GetSettings returns app-specific settings.
func (s *installedAppService) GetSettings(ctx context.Context, projectID uuid.UUID, appID string) (map[string]any, error) {
	app, err := s.repo.Get(ctx, projectID, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}
	if app == nil {
		// MCP Server returns empty settings even if not in DB
		if appID == models.AppIDMCPServer {
			return make(map[string]any), nil
		}
		return nil, fmt.Errorf("app not installed")
	}

	return app.Settings, nil
}

// UpdateSettings updates app-specific settings.
func (s *installedAppService) UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]any) error {
	// MCP Server settings are managed via mcp_config, not here
	if appID == models.AppIDMCPServer {
		return fmt.Errorf("mcp-server settings are managed via the MCP configuration API")
	}

	if err := s.repo.UpdateSettings(ctx, projectID, appID, settings); err != nil {
		return fmt.Errorf("failed to update settings: %w", err)
	}

	s.logger.Info("App settings updated",
		zap.String("project_id", projectID.String()),
		zap.String("app_id", appID),
	)

	return nil
}

// GetApp returns a specific installed app details.
func (s *installedAppService) GetApp(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error) {
	// MCP Server is always installed (virtual)
	if appID == models.AppIDMCPServer {
		return &models.InstalledApp{
			ID:          uuid.Nil,
			ProjectID:   projectID,
			AppID:       models.AppIDMCPServer,
			InstalledAt: time.Time{},
			Settings:    make(map[string]any),
		}, nil
	}

	app, err := s.repo.Get(ctx, projectID, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}
	if app == nil {
		return nil, fmt.Errorf("app not installed")
	}

	return app, nil
}

// Ensure installedAppService implements InstalledAppService at compile time.
var _ InstalledAppService = (*installedAppService)(nil)
