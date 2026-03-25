package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

type mockMCPConfigRepository struct {
	config               *models.MCPConfig
	err                  error
	agentAPIKeyByProject map[uuid.UUID]string
}

func (m *mockMCPConfigRepository) Get(context.Context, uuid.UUID) (*models.MCPConfig, error) {
	return m.config, m.err
}

func (m *mockMCPConfigRepository) Upsert(_ context.Context, config *models.MCPConfig) error {
	m.config = config
	return m.err
}

func (m *mockMCPConfigRepository) GetAgentAPIKey(_ context.Context, projectID uuid.UUID) (string, error) {
	if m.agentAPIKeyByProject == nil {
		return "", m.err
	}
	return m.agentAPIKeyByProject[projectID], m.err
}

func (m *mockMCPConfigRepository) SetAgentAPIKey(_ context.Context, projectID uuid.UUID, encryptedKey string) error {
	if m.agentAPIKeyByProject == nil {
		m.agentAPIKeyByProject = make(map[uuid.UUID]string)
	}
	m.agentAPIKeyByProject[projectID] = encryptedKey
	return m.err
}

func (m *mockMCPConfigRepository) GetAuditRetentionDays(_ context.Context, _ uuid.UUID) (*int, error) {
	if m.config == nil {
		return nil, m.err
	}
	return m.config.AuditRetentionDays, m.err
}

func (m *mockMCPConfigRepository) SetAuditRetentionDays(_ context.Context, _ uuid.UUID, days *int) error {
	if m.config != nil {
		m.config.AuditRetentionDays = days
	}
	return m.err
}

func (m *mockMCPConfigRepository) GetAlertConfig(context.Context, uuid.UUID) (*models.AlertConfig, error) {
	return nil, m.err
}

func (m *mockMCPConfigRepository) SetAlertConfig(context.Context, uuid.UUID, *models.AlertConfig) error {
	return m.err
}

type mockInstalledAppServiceForMCP struct {
	installed map[string]bool
}

func newMockInstalledAppServiceForMCP(installedApps ...string) *mockInstalledAppServiceForMCP {
	m := &mockInstalledAppServiceForMCP{installed: map[string]bool{models.AppIDMCPServer: true}}
	for _, appID := range installedApps {
		m.installed[appID] = true
	}
	return m
}

func (m *mockInstalledAppServiceForMCP) ListInstalled(context.Context, uuid.UUID) ([]*models.InstalledApp, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) IsInstalled(_ context.Context, _ uuid.UUID, appID string) (bool, error) {
	return m.installed[appID], nil
}

func (m *mockInstalledAppServiceForMCP) Install(context.Context, uuid.UUID, string, string) (*AppActionResult, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) Activate(context.Context, uuid.UUID, string) (*AppActionResult, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) Uninstall(context.Context, uuid.UUID, string) (*AppActionResult, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) CompleteCallback(context.Context, uuid.UUID, string, string, string, string, string) error {
	return nil
}

func (m *mockInstalledAppServiceForMCP) GetSettings(context.Context, uuid.UUID, string) (map[string]any, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) UpdateSettings(context.Context, uuid.UUID, string, map[string]any) error {
	return nil
}

func (m *mockInstalledAppServiceForMCP) GetApp(context.Context, uuid.UUID, string) (*models.InstalledApp, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) EnsureInstalled(context.Context, uuid.UUID, string) error {
	return nil
}

func enabledToolNames(tools []EnabledToolInfo) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

func responseToolGroupNames(groups map[string]*models.ToolGroupConfig) []string {
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	return names
}

func TestMCPConfigService_Get_DefaultResponseUsesSupportedGroups(t *testing.T) {
	projectID := uuid.New()
	repo := &mockMCPConfigRepository{}

	svc := NewMCPConfigService(repo, nil, nil, nil, "https://engine.example.com", zap.NewNop())
	resp, err := svc.Get(context.Background(), projectID)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{ToolGroupTools, ToolGroupAgentTools}, responseToolGroupNames(resp.ToolGroups))
	assert.True(t, resp.ToolGroups[ToolGroupTools].AddDirectDatabaseAccess)
	assert.True(t, resp.ToolGroups[ToolGroupTools].AddOntologyMaintenanceTools)
	assert.True(t, resp.ToolGroups[ToolGroupTools].AddOntologySuggestions)
	assert.True(t, resp.ToolGroups[ToolGroupTools].AddApprovalTools)
	assert.True(t, resp.ToolGroups[ToolGroupTools].AddRequestTools)
	assert.True(t, resp.ToolGroups[ToolGroupAgentTools].Enabled)
}

func TestMCPConfigService_Update_DropsLegacyGroups(t *testing.T) {
	projectID := uuid.New()
	repo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupTools:      {AddRequestTools: true},
				ToolGroupAgentTools: {Enabled: true},
				"developer":         {Enabled: true},
				"user":              {Enabled: true},
				"approved_queries":  {Enabled: true},
			},
		},
	}

	svc := NewMCPConfigService(repo, nil, nil, nil, "https://engine.example.com", zap.NewNop())
	disableRequestTools := false
	enableDirectDB := true
	resp, err := svc.Update(context.Background(), projectID, &UpdateMCPConfigRequest{
		AddDirectDatabaseAccess: &enableDirectDB,
		AddRequestTools:         &disableRequestTools,
	})

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{ToolGroupTools, ToolGroupAgentTools}, responseToolGroupNames(repo.config.ToolGroups))
	assert.True(t, repo.config.ToolGroups[ToolGroupTools].AddDirectDatabaseAccess)
	assert.False(t, repo.config.ToolGroups[ToolGroupTools].AddRequestTools)
	assert.True(t, repo.config.ToolGroups[ToolGroupAgentTools].Enabled)
	assert.ElementsMatch(t, []string{ToolGroupTools, ToolGroupAgentTools}, responseToolGroupNames(resp.ToolGroups))
}

func TestMCPConfigService_GetToolGroupsState_SanitizesLegacyGroups(t *testing.T) {
	projectID := uuid.New()
	repo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupTools:     {AddApprovalTools: true},
				"developer":        {Enabled: true},
				"approved_queries": {Enabled: true},
			},
		},
	}

	svc := NewMCPConfigService(repo, nil, nil, nil, "https://engine.example.com", zap.NewNop())
	state, err := svc.GetToolGroupsState(context.Background(), projectID)

	require.NoError(t, err)
	assert.Len(t, state, 1)
	assert.Contains(t, state, ToolGroupTools)
	assert.NotContains(t, state, "developer")
	assert.NotContains(t, state, "approved_queries")
}

func TestMCPConfigService_Get_FiltersToolsByInstalledApps(t *testing.T) {
	projectID := uuid.New()
	repo := &mockMCPConfigRepository{
		config: models.DefaultMCPConfig(projectID),
	}
	installedApps := newMockInstalledAppServiceForMCP(models.AppIDOntologyForge)

	svc := NewMCPConfigService(repo, nil, nil, installedApps, "https://engine.example.com", zap.NewNop())
	resp, err := svc.Get(context.Background(), projectID)

	require.NoError(t, err)
	assert.Contains(t, enabledToolNames(resp.DeveloperTools), "get_schema")
	assert.Contains(t, enabledToolNames(resp.UserTools), "get_context")
	assert.NotContains(t, enabledToolNames(resp.UserTools), "list_approved_queries")
	assert.NotContains(t, enabledToolNames(resp.DeveloperTools), "create_approved_query")
}
