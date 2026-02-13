package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

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
