package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/central"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// Mock repository for testing
type mockInstalledAppRepository struct {
	apps           map[string]*models.InstalledApp // keyed by projectID:appID
	installErr     error
	uninstallErr   error
	updateErr      error
	getErr         error
	listErr        error
	isInstalledErr error
	activateErr    error
}

func newMockInstalledAppRepository() *mockInstalledAppRepository {
	return &mockInstalledAppRepository{
		apps: make(map[string]*models.InstalledApp),
	}
}

func (m *mockInstalledAppRepository) key(projectID uuid.UUID, appID string) string {
	return projectID.String() + ":" + appID
}

func (m *mockInstalledAppRepository) List(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	var apps []*models.InstalledApp
	prefix := projectID.String() + ":"
	for k, app := range m.apps {
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			apps = append(apps, app)
		}
	}
	return apps, nil
}

func (m *mockInstalledAppRepository) Get(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.apps[m.key(projectID, appID)], nil
}

func (m *mockInstalledAppRepository) IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error) {
	if m.isInstalledErr != nil {
		return false, m.isInstalledErr
	}
	_, exists := m.apps[m.key(projectID, appID)]
	return exists, nil
}

func (m *mockInstalledAppRepository) Install(ctx context.Context, app *models.InstalledApp) error {
	if m.installErr != nil {
		return m.installErr
	}
	m.apps[m.key(app.ProjectID, app.AppID)] = app
	return nil
}

func (m *mockInstalledAppRepository) Activate(ctx context.Context, projectID uuid.UUID, appID string) error {
	if m.activateErr != nil {
		return m.activateErr
	}
	key := m.key(projectID, appID)
	if app, exists := m.apps[key]; exists {
		now := time.Now()
		app.ActivatedAt = &now
		return nil
	}
	return assert.AnError
}

func (m *mockInstalledAppRepository) Uninstall(ctx context.Context, projectID uuid.UUID, appID string) error {
	if m.uninstallErr != nil {
		return m.uninstallErr
	}
	key := m.key(projectID, appID)
	if _, exists := m.apps[key]; !exists {
		return assert.AnError
	}
	delete(m.apps, key)
	return nil
}

func (m *mockInstalledAppRepository) UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]any) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	key := m.key(projectID, appID)
	if app, exists := m.apps[key]; exists {
		app.Settings = settings
		return nil
	}
	return assert.AnError
}

// newTestInstalledAppService creates an InstalledAppService with nil central client (tests that don't
// call Install/Activate/Uninstall don't need it).
func newTestInstalledAppService(repo *mockInstalledAppRepository) InstalledAppService {
	return NewInstalledAppService(repo, nil, NewNonceStore(), "http://localhost:3443", zap.NewNop())
}

// Mock central client for testing app lifecycle methods.
type mockCentralClient struct {
	installResp   *central.AppActionResponse
	installErr    error
	activateResp  *central.AppActionResponse
	activateErr   error
	uninstallResp *central.AppActionResponse
	uninstallErr  error
}

func (m *mockCentralClient) InstallApp(_ context.Context, _, _, _, _, _ string) (*central.AppActionResponse, error) {
	return m.installResp, m.installErr
}

func (m *mockCentralClient) ActivateApp(_ context.Context, _, _, _, _, _ string) (*central.AppActionResponse, error) {
	return m.activateResp, m.activateErr
}

func (m *mockCentralClient) UninstallApp(_ context.Context, _, _, _, _, _ string) (*central.AppActionResponse, error) {
	return m.uninstallResp, m.uninstallErr
}

func newTestInstalledAppServiceWithCentral(repo *mockInstalledAppRepository, cc CentralAppClient) InstalledAppService {
	return NewInstalledAppService(repo, cc, NewNonceStore(), "http://localhost:3443", zap.NewNop())
}

func testAuthContext() context.Context {
	ctx := context.WithValue(context.Background(), auth.TokenKey, "test-token")
	return context.WithValue(ctx, auth.ClaimsKey, &auth.Claims{PAPI: "https://central.example.com"})
}

// Tests

func TestInstalledAppService_ListInstalled_IncludesMCPServerAlways(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	apps, err := svc.ListInstalled(context.Background(), projectID)
	require.NoError(t, err)
	require.Len(t, apps, 1, "should include virtual MCP Server")

	assert.Equal(t, models.AppIDMCPServer, apps[0].AppID)
	assert.Equal(t, projectID, apps[0].ProjectID)
	assert.True(t, apps[0].InstalledAt.IsZero(), "virtual MCP Server should have zero time")
}

func TestInstalledAppService_ListInstalled_ReturnsInstalledApps(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()

	// Pre-install an app
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:          uuid.New(),
		ProjectID:   projectID,
		AppID:       models.AppIDAIDataLiaison,
		InstalledAt: time.Now(),
		Settings:    make(map[string]any),
	}

	svc := newTestInstalledAppService(repo)

	apps, err := svc.ListInstalled(context.Background(), projectID)
	require.NoError(t, err)
	require.Len(t, apps, 2, "should include MCP Server and AI Data Liaison")

	// First should be MCP Server (prepended)
	assert.Equal(t, models.AppIDMCPServer, apps[0].AppID)
	assert.Equal(t, models.AppIDAIDataLiaison, apps[1].AppID)
}

func TestInstalledAppService_IsInstalled_MCPServerAlwaysTrue(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	installed, err := svc.IsInstalled(context.Background(), projectID, models.AppIDMCPServer)
	require.NoError(t, err)
	assert.True(t, installed, "MCP Server should always be installed")
}

func TestInstalledAppService_IsInstalled_ReturnsCorrectStatus(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	// Not installed
	installed, err := svc.IsInstalled(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)
	assert.False(t, installed)

	// Install it
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
	}

	// Now should be installed
	installed, err = svc.IsInstalled(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)
	assert.True(t, installed)
}

func TestInstalledAppService_Install_RejectsUnknownApp(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	_, err := svc.Install(context.Background(), projectID, "unknown-app", "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown app")
}

func TestInstalledAppService_Install_MCPServerRejected(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	_, err := svc.Install(context.Background(), projectID, models.AppIDMCPServer, "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "always installed")
}

func TestInstalledAppService_Install_AlreadyInstalled(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()

	// Pre-install
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
	}

	svc := newTestInstalledAppService(repo)

	_, err := svc.Install(context.Background(), projectID, models.AppIDAIDataLiaison, "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already installed")
}

func TestInstalledAppService_Uninstall_MCPServerRejected(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	_, err := svc.Uninstall(context.Background(), projectID, models.AppIDMCPServer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be uninstalled")
}

func TestInstalledAppService_GetSettings_ReturnsSettings(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()

	expectedSettings := map[string]any{"key": "value"}
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
		Settings:  expectedSettings,
	}

	svc := newTestInstalledAppService(repo)

	settings, err := svc.GetSettings(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)
	assert.Equal(t, expectedSettings, settings)
}

func TestInstalledAppService_GetSettings_MCPServerReturnsEmpty(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	settings, err := svc.GetSettings(context.Background(), projectID, models.AppIDMCPServer)
	require.NoError(t, err)
	assert.Empty(t, settings)
}

func TestInstalledAppService_GetSettings_NotInstalled(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	_, err := svc.GetSettings(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

func TestInstalledAppService_UpdateSettings_Success(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()

	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
		Settings:  make(map[string]any),
	}

	svc := newTestInstalledAppService(repo)

	newSettings := map[string]any{"newKey": "newValue"}
	err := svc.UpdateSettings(context.Background(), projectID, models.AppIDAIDataLiaison, newSettings)
	require.NoError(t, err)

	// Verify settings updated
	app := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	assert.Equal(t, newSettings, app.Settings)
}

func TestInstalledAppService_UpdateSettings_MCPServerRejected(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	err := svc.UpdateSettings(context.Background(), projectID, models.AppIDMCPServer, map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MCP configuration API")
}

func TestInstalledAppService_GetApp_MCPServerReturnsVirtual(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	app, err := svc.GetApp(context.Background(), projectID, models.AppIDMCPServer)
	require.NoError(t, err)
	require.NotNil(t, app)

	assert.Equal(t, models.AppIDMCPServer, app.AppID)
	assert.Equal(t, projectID, app.ProjectID)
	assert.Equal(t, uuid.Nil, app.ID, "virtual app should have nil ID")
	assert.True(t, app.InstalledAt.IsZero(), "virtual app should have zero time")
}

func TestInstalledAppService_GetApp_ReturnsInstalledApp(t *testing.T) {
	projectID := uuid.New()
	appID := uuid.New()
	installedAt := time.Now()
	repo := newMockInstalledAppRepository()

	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:          appID,
		ProjectID:   projectID,
		AppID:       models.AppIDAIDataLiaison,
		InstalledAt: installedAt,
		InstalledBy: "user@example.com",
		Settings:    map[string]any{"key": "value"},
	}

	svc := newTestInstalledAppService(repo)

	app, err := svc.GetApp(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)
	require.NotNil(t, app)

	assert.Equal(t, appID, app.ID)
	assert.Equal(t, models.AppIDAIDataLiaison, app.AppID)
	assert.Equal(t, "user@example.com", app.InstalledBy)
}

func TestInstalledAppService_GetApp_NotInstalled(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	_, err := svc.GetApp(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not installed")
}

// --- Activate tests ---

func TestInstalledAppService_Activate_RejectsMCPServer(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	_, err := svc.Activate(context.Background(), projectID, models.AppIDMCPServer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not require activation")
}

func TestInstalledAppService_Activate_ReturnsRedirectFromCentral(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	cc := &mockCentralClient{
		activateResp: &central.AppActionResponse{
			Status:      "pending_activation",
			RedirectUrl: "https://central.example.com/billing",
		},
	}
	svc := newTestInstalledAppServiceWithCentral(repo, cc)

	result, err := svc.Activate(testAuthContext(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)
	assert.Equal(t, "https://central.example.com/billing", result.RedirectUrl)
	assert.Equal(t, "pending_activation", result.Status)
}

func TestInstalledAppService_Activate_CompletesLocallyWhenNoRedirect(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
		Settings:  make(map[string]any),
	}
	cc := &mockCentralClient{
		activateResp: &central.AppActionResponse{Status: "activated"},
	}
	svc := newTestInstalledAppServiceWithCentral(repo, cc)

	result, err := svc.Activate(testAuthContext(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)
	assert.Empty(t, result.RedirectUrl)
	assert.Equal(t, "activated", result.Status)

	app := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	assert.NotNil(t, app.ActivatedAt)
}

// --- Uninstall tests ---

func TestInstalledAppService_Uninstall_ReturnsRedirectFromCentral(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	cc := &mockCentralClient{
		uninstallResp: &central.AppActionResponse{
			Status:      "pending_uninstall",
			RedirectUrl: "https://central.example.com/cancel",
		},
	}
	svc := newTestInstalledAppServiceWithCentral(repo, cc)

	result, err := svc.Uninstall(testAuthContext(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)
	assert.Equal(t, "https://central.example.com/cancel", result.RedirectUrl)
	assert.Equal(t, "pending_uninstall", result.Status)
}

func TestInstalledAppService_Uninstall_CompletesLocallyWhenNoRedirect(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
		Settings:  make(map[string]any),
	}
	cc := &mockCentralClient{
		uninstallResp: &central.AppActionResponse{Status: "uninstalled"},
	}
	svc := newTestInstalledAppServiceWithCentral(repo, cc)

	result, err := svc.Uninstall(testAuthContext(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)
	assert.Empty(t, result.RedirectUrl)
	assert.Equal(t, "uninstalled", result.Status)

	_, exists := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	assert.False(t, exists)
}

// --- CompleteCallback tests ---

func TestInstalledAppService_CompleteCallback_RejectsInvalidNonce(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	err := svc.CompleteCallback(context.Background(), projectID, models.AppIDAIDataLiaison, "activate", "success", "invalid-nonce", "user@example.com")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid or expired")
}

func TestInstalledAppService_CompleteCallback_CompletesActivate(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
		Settings:  make(map[string]any),
	}

	nonceStore := NewNonceStore()
	svc := NewInstalledAppService(repo, nil, nonceStore, "http://localhost:3443", zap.NewNop())
	nonce := nonceStore.Generate("activate", projectID.String(), models.AppIDAIDataLiaison)

	err := svc.CompleteCallback(context.Background(), projectID, models.AppIDAIDataLiaison, "activate", "success", nonce, "user@example.com")
	require.NoError(t, err)

	app := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	assert.NotNil(t, app.ActivatedAt)
}

func TestInstalledAppService_CompleteCallback_CompletesUninstall(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
		Settings:  make(map[string]any),
	}

	nonceStore := NewNonceStore()
	svc := NewInstalledAppService(repo, nil, nonceStore, "http://localhost:3443", zap.NewNop())
	nonce := nonceStore.Generate("uninstall", projectID.String(), models.AppIDAIDataLiaison)

	err := svc.CompleteCallback(context.Background(), projectID, models.AppIDAIDataLiaison, "uninstall", "success", nonce, "user@example.com")
	require.NoError(t, err)

	_, exists := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	assert.False(t, exists)
}

// --- EnsureInstalled tests ---

func TestInstalledAppService_EnsureInstalled_InstallsNewApp(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	err := svc.EnsureInstalled(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)

	app, exists := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	require.True(t, exists, "app should be installed")
	assert.Equal(t, models.AppIDAIDataLiaison, app.AppID)
	assert.Equal(t, "central-provision", app.InstalledBy)
	assert.False(t, app.InstalledAt.IsZero())
}

func TestInstalledAppService_EnsureInstalled_AlreadyInstalledIsNoop(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()

	originalApp := &models.InstalledApp{
		ID:          uuid.New(),
		ProjectID:   projectID,
		AppID:       models.AppIDAIDataLiaison,
		InstalledAt: time.Now().Add(-24 * time.Hour),
		InstalledBy: "original-user",
		Settings:    map[string]any{"key": "value"},
	}
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = originalApp

	svc := newTestInstalledAppService(repo)

	err := svc.EnsureInstalled(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)

	// Verify original app unchanged
	app := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	assert.Equal(t, originalApp.ID, app.ID)
	assert.Equal(t, "original-user", app.InstalledBy)
	assert.Equal(t, map[string]any{"key": "value"}, app.Settings)
}

func TestInstalledAppService_EnsureInstalled_UnknownAppSkipped(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	err := svc.EnsureInstalled(context.Background(), projectID, "unknown-app")
	require.NoError(t, err)

	assert.Empty(t, repo.apps, "no app should be installed")
}

func TestInstalledAppService_EnsureInstalled_MCPServerSkipped(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	svc := newTestInstalledAppService(repo)

	err := svc.EnsureInstalled(context.Background(), projectID, models.AppIDMCPServer)
	require.NoError(t, err)

	assert.Empty(t, repo.apps, "mcp-server should not be persisted")
}

func TestInstalledAppService_EnsureInstalled_DoesNotCallCentral(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	// Use nil central client — if EnsureInstalled tried to call central, it would panic
	svc := newTestInstalledAppService(repo)

	err := svc.EnsureInstalled(context.Background(), projectID, models.AppIDAIDataLiaison)
	require.NoError(t, err)

	_, exists := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	assert.True(t, exists, "app should be installed without central interaction")
}

func TestInstalledAppService_CompleteCallback_CancelledNoOps(t *testing.T) {
	projectID := uuid.New()
	repo := newMockInstalledAppRepository()
	repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)] = &models.InstalledApp{
		ID:        uuid.New(),
		ProjectID: projectID,
		AppID:     models.AppIDAIDataLiaison,
		Settings:  make(map[string]any),
	}

	nonceStore := NewNonceStore()
	svc := NewInstalledAppService(repo, nil, nonceStore, "http://localhost:3443", zap.NewNop())
	nonce := nonceStore.Generate("activate", projectID.String(), models.AppIDAIDataLiaison)

	err := svc.CompleteCallback(context.Background(), projectID, models.AppIDAIDataLiaison, "activate", "cancelled", nonce, "user@example.com")
	require.NoError(t, err)

	// App should NOT have been activated
	app := repo.apps[repo.key(projectID, models.AppIDAIDataLiaison)]
	assert.Nil(t, app.ActivatedAt)
}
