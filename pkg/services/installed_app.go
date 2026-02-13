package services

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/central"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// AppActionResult is returned by Install, Activate, and Uninstall when central
// may require a redirect for user interaction (e.g., billing flow).
type AppActionResult struct {
	// App is set when the action completed immediately (no redirect needed).
	App *models.InstalledApp
	// RedirectUrl is set when the user must be redirected to central to complete the action.
	RedirectUrl string
	// Status is the action status (e.g., "installed", "activated", "pending_activation").
	Status string
}

// InstalledAppService orchestrates installed app management.
type InstalledAppService interface {
	// ListInstalled returns all installed apps for a project.
	// Always includes mcp-server as "always installed".
	ListInstalled(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error)

	// IsInstalled checks if a specific app is installed.
	IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error)

	// Install installs an app for a project. May return a redirect URL if central
	// requires user interaction.
	Install(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*AppActionResult, error)

	// Activate activates an installed app (billing begins). May return a redirect URL
	// if central requires user interaction.
	Activate(ctx context.Context, projectID uuid.UUID, appID string) (*AppActionResult, error)

	// Uninstall removes an app. May return a redirect URL if central requires
	// user interaction (e.g., billing cancellation confirmation).
	Uninstall(ctx context.Context, projectID uuid.UUID, appID string) (*AppActionResult, error)

	// CompleteCallback processes the callback from central after a redirect flow.
	// Returns the action that was completed (for redirect target determination).
	CompleteCallback(ctx context.Context, projectID uuid.UUID, appID, action, status, nonce, userID string) error

	// GetSettings returns app-specific settings.
	GetSettings(ctx context.Context, projectID uuid.UUID, appID string) (map[string]any, error)

	// UpdateSettings updates app-specific settings.
	UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]any) error

	// GetApp returns a specific installed app details.
	GetApp(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error)
}

type installedAppService struct {
	repo          repositories.InstalledAppRepository
	centralClient *central.Client
	nonceStore    NonceStore
	baseURL       string
	logger        *zap.Logger
}

// NewInstalledAppService creates a new installed app service.
func NewInstalledAppService(
	repo repositories.InstalledAppRepository,
	centralClient *central.Client,
	nonceStore NonceStore,
	baseURL string,
	logger *zap.Logger,
) InstalledAppService {
	return &installedAppService{
		repo:          repo,
		centralClient: centralClient,
		nonceStore:    nonceStore,
		baseURL:       baseURL,
		logger:        logger,
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
// Calls central first, then saves locally if no redirect is needed.
func (s *installedAppService) Install(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*AppActionResult, error) {
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

	// Notify central first
	callbackUrl := s.buildCallbackURL(projectID.String(), appID, "install")
	token, papiURL, err := s.getAuthContext(ctx)
	if err != nil {
		return nil, err
	}

	centralResp, err := s.centralClient.InstallApp(ctx, papiURL, projectID.String(), appID, token, callbackUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to notify central: %w", err)
	}

	// If central requires a redirect, return it without saving to DB
	if centralResp.RedirectUrl != "" {
		return &AppActionResult{
			RedirectUrl: centralResp.RedirectUrl,
			Status:      centralResp.Status,
		}, nil
	}

	// No redirect needed — save to DB immediately
	app, err := s.saveInstall(ctx, projectID, appID, userID)
	if err != nil {
		return nil, err
	}

	return &AppActionResult{
		App:    app,
		Status: centralResp.Status,
	}, nil
}

// Activate activates an installed app.
// Calls central first, then updates locally if no redirect is needed.
func (s *installedAppService) Activate(ctx context.Context, projectID uuid.UUID, appID string) (*AppActionResult, error) {
	// MCP Server cannot be activated
	if appID == models.AppIDMCPServer {
		return nil, fmt.Errorf("mcp-server does not require activation")
	}

	// Notify central first
	callbackUrl := s.buildCallbackURL(projectID.String(), appID, "activate")
	token, papiURL, err := s.getAuthContext(ctx)
	if err != nil {
		return nil, err
	}

	centralResp, err := s.centralClient.ActivateApp(ctx, papiURL, projectID.String(), appID, token, callbackUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to notify central: %w", err)
	}

	// If central requires a redirect, return it
	if centralResp.RedirectUrl != "" {
		return &AppActionResult{
			RedirectUrl: centralResp.RedirectUrl,
			Status:      centralResp.Status,
		}, nil
	}

	// No redirect needed — activate locally
	if err := s.repo.Activate(ctx, projectID, appID); err != nil {
		return nil, fmt.Errorf("failed to activate app: %w", err)
	}

	s.logger.Info("App activated",
		zap.String("project_id", projectID.String()),
		zap.String("app_id", appID),
	)

	return &AppActionResult{Status: centralResp.Status}, nil
}

// Uninstall removes an app.
// Calls central first, then deletes locally if no redirect is needed.
func (s *installedAppService) Uninstall(ctx context.Context, projectID uuid.UUID, appID string) (*AppActionResult, error) {
	// MCP Server cannot be uninstalled
	if appID == models.AppIDMCPServer {
		return nil, fmt.Errorf("mcp-server cannot be uninstalled")
	}

	// Notify central first
	callbackUrl := s.buildCallbackURL(projectID.String(), appID, "uninstall")
	token, papiURL, err := s.getAuthContext(ctx)
	if err != nil {
		return nil, err
	}

	centralResp, err := s.centralClient.UninstallApp(ctx, papiURL, projectID.String(), appID, token, callbackUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to notify central: %w", err)
	}

	// If central requires a redirect, return it without deleting
	if centralResp.RedirectUrl != "" {
		return &AppActionResult{
			RedirectUrl: centralResp.RedirectUrl,
			Status:      centralResp.Status,
		}, nil
	}

	// No redirect needed — delete locally
	if err := s.repo.Uninstall(ctx, projectID, appID); err != nil {
		return nil, fmt.Errorf("failed to uninstall app: %w", err)
	}

	s.logger.Info("App uninstalled",
		zap.String("project_id", projectID.String()),
		zap.String("app_id", appID),
	)

	return &AppActionResult{Status: centralResp.Status}, nil
}

// CompleteCallback processes the callback from central after a redirect flow.
func (s *installedAppService) CompleteCallback(ctx context.Context, projectID uuid.UUID, appID, action, status, nonce, userID string) error {
	// Validate nonce
	if !s.nonceStore.Validate(nonce, action, projectID.String(), appID) {
		return fmt.Errorf("invalid or expired callback nonce")
	}

	// If user cancelled, nothing to do
	if status == "cancelled" {
		s.logger.Info("App action cancelled by user",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
			zap.String("action", action),
		)
		return nil
	}

	if status != "success" {
		return fmt.Errorf("unexpected callback status: %s", status)
	}

	// Complete the action locally
	switch action {
	case "install":
		if _, err := s.saveInstall(ctx, projectID, appID, userID); err != nil {
			return fmt.Errorf("failed to complete install: %w", err)
		}
	case "activate":
		if err := s.repo.Activate(ctx, projectID, appID); err != nil {
			return fmt.Errorf("failed to complete activation: %w", err)
		}
		s.logger.Info("App activated via callback",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
		)
	case "uninstall":
		if err := s.repo.Uninstall(ctx, projectID, appID); err != nil {
			return fmt.Errorf("failed to complete uninstall: %w", err)
		}
		s.logger.Info("App uninstalled via callback",
			zap.String("project_id", projectID.String()),
			zap.String("app_id", appID),
		)
	default:
		return fmt.Errorf("unknown callback action: %s", action)
	}

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

// saveInstall creates the installed app record in the database.
func (s *installedAppService) saveInstall(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*models.InstalledApp, error) {
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

// buildCallbackURL constructs the engine callback URL for central redirects.
func (s *installedAppService) buildCallbackURL(projectID, appID, action string) string {
	nonce := s.nonceStore.Generate(action, projectID, appID)
	callbackURL := fmt.Sprintf("%s/api/projects/%s/apps/%s/callback", s.baseURL, projectID, appID)

	params := url.Values{}
	params.Set("action", action)
	params.Set("state", nonce)

	return callbackURL + "?" + params.Encode()
}

// getAuthContext extracts the JWT token and central API URL from the request context.
func (s *installedAppService) getAuthContext(ctx context.Context) (token, papiURL string, err error) {
	t, ok := auth.GetToken(ctx)
	if !ok {
		return "", "", fmt.Errorf("no auth token in context")
	}

	claims, ok := auth.GetClaims(ctx)
	if !ok {
		return "", "", fmt.Errorf("no auth claims in context")
	}

	if claims.PAPI == "" {
		return "", "", fmt.Errorf("no central API URL in token claims")
	}

	return t, claims.PAPI, nil
}

// Ensure installedAppService implements InstalledAppService at compile time.
var _ InstalledAppService = (*installedAppService)(nil)
