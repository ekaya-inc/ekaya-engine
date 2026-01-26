package services

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// Mock implementations for testing

type mockMCPConfigRepository struct {
	config               *models.MCPConfig
	err                  error
	agentAPIKeyByProject map[uuid.UUID]string
}

func (m *mockMCPConfigRepository) Get(ctx context.Context, projectID uuid.UUID) (*models.MCPConfig, error) {
	return m.config, m.err
}

func (m *mockMCPConfigRepository) Upsert(ctx context.Context, config *models.MCPConfig) error {
	m.config = config
	return m.err
}

func (m *mockMCPConfigRepository) GetAgentAPIKey(ctx context.Context, projectID uuid.UUID) (string, error) {
	if m.agentAPIKeyByProject == nil {
		return "", m.err
	}
	return m.agentAPIKeyByProject[projectID], m.err
}

func (m *mockMCPConfigRepository) SetAgentAPIKey(ctx context.Context, projectID uuid.UUID, encryptedKey string) error {
	if m.agentAPIKeyByProject == nil {
		m.agentAPIKeyByProject = make(map[uuid.UUID]string)
	}
	m.agentAPIKeyByProject[projectID] = encryptedKey
	return m.err
}

type mockQueryServiceForMCP struct {
	hasEnabledQueries bool
	hasEnabledErr     error
}

func (m *mockQueryServiceForMCP) Create(ctx context.Context, projectID, datasourceID uuid.UUID, req *CreateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) Get(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) List(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) Update(ctx context.Context, projectID, queryID uuid.UUID, req *UpdateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) Delete(ctx context.Context, projectID, queryID uuid.UUID) error {
	return nil
}

func (m *mockQueryServiceForMCP) ListEnabled(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) ListEnabledByTags(ctx context.Context, projectID, datasourceID uuid.UUID, tags []string) ([]*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) HasEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (bool, error) {
	return m.hasEnabledQueries, m.hasEnabledErr
}

func (m *mockQueryServiceForMCP) SetEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error {
	return nil
}

func (m *mockQueryServiceForMCP) Execute(ctx context.Context, projectID, queryID uuid.UUID, req *ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) ExecuteWithParameters(ctx context.Context, projectID, queryID uuid.UUID, params map[string]any, req *ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) ExecuteModifyingWithParameters(ctx context.Context, projectID, queryID uuid.UUID, params map[string]any) (*datasource.ExecuteResult, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) (*ValidationResult, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) Test(ctx context.Context, projectID, datasourceID uuid.UUID, req *TestQueryRequest) (*datasource.QueryExecutionResult, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) ValidateParameterizedQuery(sqlQuery string, params []models.QueryParameter) error {
	return nil
}

func (m *mockQueryServiceForMCP) SuggestUpdate(ctx context.Context, projectID uuid.UUID, req *SuggestUpdateRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) DirectCreate(ctx context.Context, projectID, datasourceID uuid.UUID, req *CreateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) DirectUpdate(ctx context.Context, projectID, queryID uuid.UUID, req *UpdateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) ApproveQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) error {
	return nil
}

func (m *mockQueryServiceForMCP) RejectQuery(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string, reason string) error {
	return nil
}

func (m *mockQueryServiceForMCP) MoveToPending(ctx context.Context, projectID, queryID uuid.UUID) error {
	return nil
}

func (m *mockQueryServiceForMCP) ListPending(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) DeleteWithPendingRejection(ctx context.Context, projectID, queryID uuid.UUID, reviewerID string) (int, error) {
	return 0, nil
}

type mockProjectServiceForMCP struct {
	defaultDatasourceID uuid.UUID
	err                 error
}

func (m *mockProjectServiceForMCP) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForMCP) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectServiceForMCP) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectServiceForMCP) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectServiceForMCP) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockProjectServiceForMCP) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	return m.defaultDatasourceID, m.err
}

func (m *mockProjectServiceForMCP) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockProjectServiceForMCP) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
	// No-op for tests
}

func (m *mockProjectServiceForMCP) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	return "", nil
}

func (m *mockProjectServiceForMCP) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	return nil
}

func (m *mockProjectServiceForMCP) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*AutoApproveSettings, error) {
	return nil, nil
}

func (m *mockProjectServiceForMCP) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *AutoApproveSettings) error {
	return nil
}

func (m *mockProjectServiceForMCP) GetOntologySettings(ctx context.Context, projectID uuid.UUID) (*OntologySettings, error) {
	return &OntologySettings{UseLegacyPatternMatching: true}, nil
}

func (m *mockProjectServiceForMCP) SetOntologySettings(ctx context.Context, projectID uuid.UUID, settings *OntologySettings) error {
	return nil
}

// Tests

func TestMCPConfigService_Get_ReturnsStoredConfigState(t *testing.T) {
	// This test verifies that Get() returns the stored config state,
	// without overriding. The UI and ShouldShowApprovedQueriesTools
	// handle the validation of whether tools should actually be exposed.

	projectID := uuid.New()
	datasourceID := uuid.New()

	// Config has approved_queries enabled
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
		},
	}

	// Even with no enabled queries, the response should reflect stored state
	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: false,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Response should reflect stored state (enabled=true)
	approvedQueries, ok := resp.ToolGroups[ToolGroupApprovedQueries]
	require.True(t, ok, "approved_queries should be in response")
	assert.True(t, approvedQueries.Enabled, "response should reflect stored config state")
}

func TestMCPConfigService_Get_ApprovedQueriesEnabledWhenEnabledQueriesExist(t *testing.T) {
	// This test verifies that approved_queries shows as enabled
	// when the config has it enabled AND there are enabled queries.

	projectID := uuid.New()
	datasourceID := uuid.New()

	// Config has approved_queries enabled
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
		},
	}

	// Enabled queries exist
	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// approved_queries should be enabled
	approvedQueries, ok := resp.ToolGroups[ToolGroupApprovedQueries]
	require.True(t, ok, "approved_queries should be in response")
	assert.True(t, approvedQueries.Enabled, "approved_queries should be enabled when enabled queries exist")
}

func TestMCPConfigService_ShouldShowApprovedQueriesTools_FalseWhenNoEnabledQueries(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Config has approved_queries enabled
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
		},
	}

	// No enabled queries
	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: false,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	shouldShow, err := svc.ShouldShowApprovedQueriesTools(context.Background(), projectID)
	require.NoError(t, err)
	assert.False(t, shouldShow, "should return false when no enabled queries exist")
}

func TestMCPConfigService_ShouldShowApprovedQueriesTools_TrueWhenEnabledQueriesExist(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Config has approved_queries enabled
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
		},
	}

	// Enabled queries exist
	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	shouldShow, err := svc.ShouldShowApprovedQueriesTools(context.Background(), projectID)
	require.NoError(t, err)
	assert.True(t, shouldShow, "should return true when enabled queries exist")
}

func TestMCPConfigService_ShouldShowApprovedQueriesTools_ErrorFromHasEnabledQueries(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: false,
		hasEnabledErr:     assert.AnError,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	shouldShow, err := svc.ShouldShowApprovedQueriesTools(context.Background(), projectID)
	require.Error(t, err, "should propagate error from HasEnabledQueries")
	assert.False(t, shouldShow)
}

func TestMCPConfigService_Get_NoDatasource_StillReturnsStoredState(t *testing.T) {
	// Even when project has no default datasource, Get() should return stored state.
	// ShouldShowApprovedQueriesTools handles whether tools are exposed.
	projectID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	// Project has no default datasource
	projectService := &mockProjectServiceForMCP{
		err: assert.AnError,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Response should reflect stored state (enabled=true)
	approvedQueries, ok := resp.ToolGroups[ToolGroupApprovedQueries]
	require.True(t, ok)
	assert.True(t, approvedQueries.Enabled, "response should reflect stored config state")
}

func TestMCPConfigService_Update_ShouldPersistSubOptionState(t *testing.T) {
	// This test verifies that when Update() saves a sub-option,
	// the response should reflect that sub-option value.

	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: models.DefaultMCPConfig(projectID),
	}

	// Enabled queries exist
	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	// Update to set AllowOntologyMaintenance to false (default is true)
	allowOntologyMaintenance := false
	req := &UpdateMCPConfigRequest{
		AllowOntologyMaintenance: &allowOntologyMaintenance,
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The response should show AllowOntologyMaintenance=false
	userConfig, ok := resp.ToolGroups["user"]
	require.True(t, ok, "user config should be in response")
	assert.False(t, userConfig.AllowOntologyMaintenance, "AllowOntologyMaintenance should be false in response after Update")

	// Also verify the config was actually saved with AllowOntologyMaintenance=false
	assert.False(t, configRepo.config.ToolGroups["user"].AllowOntologyMaintenance,
		"config should be persisted with AllowOntologyMaintenance=false")
}

func TestMCPConfigService_Update_DeveloperSubOptions(t *testing.T) {
	// Test updating developer sub-options (AddQueryTools and AddOntologyMaintenance)

	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: models.DefaultMCPConfig(projectID),
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	// Update to disable AddQueryTools but keep AddOntologyMaintenance
	addQueryTools := false
	addOntologyMaintenance := true
	req := &UpdateMCPConfigRequest{
		AddQueryTools:          &addQueryTools,
		AddOntologyMaintenance: &addOntologyMaintenance,
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The config should be saved with the correct values
	require.NotNil(t, configRepo.config.ToolGroups["developer"])
	assert.False(t, configRepo.config.ToolGroups["developer"].AddQueryTools,
		"config should be persisted with AddQueryTools=false")
	assert.True(t, configRepo.config.ToolGroups["developer"].AddOntologyMaintenance,
		"config should be persisted with AddOntologyMaintenance=true")

	// Response should reflect saved state
	devConfig, ok := resp.ToolGroups["developer"]
	require.True(t, ok)
	assert.False(t, devConfig.AddQueryTools, "response should reflect AddQueryTools=false")
	assert.True(t, devConfig.AddOntologyMaintenance, "response should reflect AddOntologyMaintenance=true")
}

func TestMCPConfigService_Get_SubOptionsResetWhenDisabled(t *testing.T) {
	// When a tool group is disabled, sub-options should be reset to false in response
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Config has developer disabled but with ForceMode=true (inconsistent state)
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: false, ForceMode: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: false,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Developer should be disabled and sub-options should be reset
	developer, ok := resp.ToolGroups["developer"]
	require.True(t, ok)
	assert.False(t, developer.Enabled, "developer should be disabled")
	assert.False(t, developer.ForceMode, "ForceMode should be reset when group is disabled")
}

func TestMCPConfigService_Get_ServerURLConstruction(t *testing.T) {
	// Test that ServerURL is constructed correctly regardless of trailing slash on baseURL
	projectID := uuid.MustParse("12345678-1234-1234-1234-123456789abc")
	datasourceID := uuid.New()

	tests := []struct {
		name        string
		baseURL     string
		expectedURL string
	}{
		{
			name:        "base URL without trailing slash",
			baseURL:     "http://localhost:3443",
			expectedURL: "http://localhost:3443/mcp/12345678-1234-1234-1234-123456789abc",
		},
		{
			name:        "base URL with trailing slash",
			baseURL:     "http://localhost:3443/",
			expectedURL: "http://localhost:3443/mcp/12345678-1234-1234-1234-123456789abc",
		},
		{
			name:        "base URL with path",
			baseURL:     "https://example.com/api/v1",
			expectedURL: "https://example.com/api/v1/mcp/12345678-1234-1234-1234-123456789abc",
		},
		{
			name:        "base URL with path and trailing slash",
			baseURL:     "https://example.com/api/v1/",
			expectedURL: "https://example.com/api/v1/mcp/12345678-1234-1234-1234-123456789abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configRepo := &mockMCPConfigRepository{
				config: models.DefaultMCPConfig(projectID),
			}
			queryService := &mockQueryServiceForMCP{
				hasEnabledQueries: false,
			}
			projectService := &mockProjectServiceForMCP{
				defaultDatasourceID: datasourceID,
			}

			svc := NewMCPConfigService(
				configRepo,
				queryService,
				projectService,
				nil, // installedAppService - not needed for this test
				tt.baseURL,
				zap.NewNop(),
			)

			resp, err := svc.Get(context.Background(), projectID)
			require.NoError(t, err)
			require.NotNil(t, resp)
			assert.Equal(t, tt.expectedURL, resp.ServerURL, "ServerURL should be properly constructed")
		})
	}
}

func TestMCPConfigService_Get_EnabledToolsIncluded(t *testing.T) {
	// When no tool groups are enabled, only the health tool should be in EnabledTools
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: models.DefaultMCPConfig(projectID),
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: false,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// With no tool groups enabled, only health should be available
	require.Len(t, resp.EnabledTools, 1, "should have exactly one tool enabled (health)")
	assert.Equal(t, "health", resp.EnabledTools[0].Name)
	assert.Equal(t, "Server health check", resp.EnabledTools[0].Description)
}

func TestMCPConfigService_Get_EnabledToolsWithDeveloperEnabled(t *testing.T) {
	// When developer tools are enabled, only Developer Core tools are available
	// Query loadout tools require AddQueryTools option
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: false,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Developer Core tools only (Default + DeveloperCore loadouts)
	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// Should include Developer Core tools
	assert.Contains(t, toolNames, "health", "should include health tool (always available)")
	assert.Contains(t, toolNames, "echo", "should include echo tool (Developer Core)")
	assert.Contains(t, toolNames, "execute", "should include execute tool (Developer Core)")

	// Query loadout tools NOT included without AddQueryTools option
	assert.NotContains(t, toolNames, "validate", "validate requires AddQueryTools option")
	assert.NotContains(t, toolNames, "query", "query requires AddQueryTools option")
	assert.NotContains(t, toolNames, "explain_query", "explain_query requires AddQueryTools option")
	assert.NotContains(t, toolNames, "get_schema", "get_schema requires AddQueryTools option")
	assert.NotContains(t, toolNames, "sample", "sample requires AddQueryTools option")
}

func TestMCPConfigService_Get_EnabledToolsWithDeveloperCoreIncludesExecute(t *testing.T) {
	// When developer tools are enabled, execute tool is always included (part of Developer Core)
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: false,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	assert.Contains(t, toolNames, "execute", "should include execute tool in Developer Core")
}

func TestMCPConfigService_Get_EnabledToolsWithApprovedQueries(t *testing.T) {
	// When approved_queries is enabled, Query loadout tools are available
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// Query loadout tools (includes get_schema per spec)
	assert.Contains(t, toolNames, "query", "should include query tool")
	assert.Contains(t, toolNames, "sample", "should include sample tool")
	assert.Contains(t, toolNames, "validate", "should include validate tool")
	assert.Contains(t, toolNames, "get_ontology", "should include get_ontology tool")
	assert.Contains(t, toolNames, "list_glossary", "should include list_glossary tool")
	assert.Contains(t, toolNames, "get_glossary_sql", "should include get_glossary_sql tool")
	assert.Contains(t, toolNames, "list_approved_queries", "should include list_approved_queries tool")
	assert.Contains(t, toolNames, "execute_approved_query", "should include execute_approved_query tool")
	assert.Contains(t, toolNames, "health", "should include health tool")
	assert.Contains(t, toolNames, "get_schema", "get_schema IS in Query loadout per spec")
	assert.Contains(t, toolNames, "get_context", "should include get_context tool")

	// Developer-only tools should NOT be present
	assert.NotContains(t, toolNames, "echo", "should NOT include developer tool echo")
	assert.NotContains(t, toolNames, "execute", "should NOT include developer tool execute")

	// Ontology maintenance should NOT be present without AllowOntologyMaintenance
	assert.NotContains(t, toolNames, "update_entity", "should NOT include update_entity without option")
}

func TestMCPConfigService_Get_EnabledToolsWithAgentTools(t *testing.T) {
	// When only agent_tools is enabled (no developer or approved_queries),
	// the UI shows tools from the USER perspective, not agent perspective.
	// Since neither developer nor approved_queries is enabled for users,
	// only health should be available.
	// Agent-specific filtering only happens at MCP connection time.
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: false,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// When agent_tools is enabled, UI shows what agents would see (limited query tools)
	assert.Contains(t, toolNames, "health", "should include health tool")
	assert.Contains(t, toolNames, "list_approved_queries", "should include list_approved_queries for agents")
	assert.Contains(t, toolNames, "execute_approved_query", "should include execute_approved_query for agents")
	assert.Len(t, resp.EnabledTools, 3, "should have 3 tools (health + limited query) when agent_tools is enabled")
}

func TestMCPConfigService_Update_DeveloperToolsReflectNewState(t *testing.T) {
	// When Update() is called with developer sub-options, DeveloperTools should reflect the new state
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: models.DefaultMCPConfig(projectID),
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	// Update developer tools with AddQueryTools option (but not AddOntologyMaintenance)
	addQueryTools := true
	addOntologyMaintenance := false
	req := &UpdateMCPConfigRequest{
		AddQueryTools:          &addQueryTools,
		AddOntologyMaintenance: &addOntologyMaintenance,
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Check DeveloperTools (the new per-role field) instead of EnabledTools (deprecated)
	toolNames := make([]string, len(resp.DeveloperTools))
	for i, tool := range resp.DeveloperTools {
		toolNames[i] = tool.Name
	}

	// Developer Core + Query loadout (NO ontology maintenance - that requires AddOntologyMaintenance)
	assert.Contains(t, toolNames, "echo", "should include echo in developer tools")
	assert.Contains(t, toolNames, "execute", "should include execute in developer tools")
	assert.Contains(t, toolNames, "get_schema", "should include get_schema with AddQueryTools")
	assert.Contains(t, toolNames, "sample", "should include sample with AddQueryTools")
	assert.NotContains(t, toolNames, "update_entity", "should NOT include update_entity with AddQueryTools (requires AddOntologyMaintenance)")
}

func TestMCPConfigService_Update_AllowOntologyMaintenance(t *testing.T) {
	// Test that enabling AllowOntologyMaintenance for User Tools adds ontology maintenance tools
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Start with AllowOntologyMaintenance disabled
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"user": {AllowOntologyMaintenance: false},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: datasourceID,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	// Update to enable AllowOntologyMaintenance
	allowOntologyMaintenance := true
	req := &UpdateMCPConfigRequest{
		AllowOntologyMaintenance: &allowOntologyMaintenance,
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify the state includes AllowOntologyMaintenance
	userConfig := resp.ToolGroups["user"]
	require.NotNil(t, userConfig, "user config should exist")
	assert.True(t, userConfig.AllowOntologyMaintenance, "AllowOntologyMaintenance should be true")

	// User tools should now include ontology maintenance tools
	toolNames := make([]string, len(resp.UserTools))
	for i, tool := range resp.UserTools {
		toolNames[i] = tool.Name
	}

	// Should have Query tools + Ontology Maintenance tools for users
	assert.Contains(t, toolNames, "update_entity", "should include update_entity with AllowOntologyMaintenance")
	assert.Contains(t, toolNames, "update_column", "should include update_column with AllowOntologyMaintenance")
}

// mockInstalledAppServiceForMCP is a mock implementation of InstalledAppService for testing.
type mockInstalledAppServiceForMCP struct {
	installed map[string]bool // appID -> installed
	err       error
}

func (m *mockInstalledAppServiceForMCP) ListInstalled(ctx context.Context, projectID uuid.UUID) ([]*models.InstalledApp, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) IsInstalled(ctx context.Context, projectID uuid.UUID, appID string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.installed[appID], nil
}

func (m *mockInstalledAppServiceForMCP) Install(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*models.InstalledApp, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) Uninstall(ctx context.Context, projectID uuid.UUID, appID string) error {
	return nil
}

func (m *mockInstalledAppServiceForMCP) GetSettings(ctx context.Context, projectID uuid.UUID, appID string) (map[string]any, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) UpdateSettings(ctx context.Context, projectID uuid.UUID, appID string, settings map[string]any) error {
	return nil
}

func (m *mockInstalledAppServiceForMCP) GetApp(ctx context.Context, projectID uuid.UUID, appID string) (*models.InstalledApp, error) {
	return nil, nil
}

func TestMCPConfigService_Get_FiltersDataLiaisonTools_WhenNotInstalled(t *testing.T) {
	// When AI Data Liaison app is not installed, data liaison tools should not appear in EnabledTools

	projectID := uuid.New()
	datasourceID := uuid.New()

	// Enable developer tools with query tools AND ontology maintenance (which includes data liaison tools)
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupDeveloper: {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	// AI Data Liaison app is NOT installed
	installedAppService := &mockInstalledAppServiceForMCP{
		installed: map[string]bool{
			models.AppIDAIDataLiaison: false,
		},
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		installedAppService,
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// Should have developer tools but NOT data liaison tools
	assert.Contains(t, toolNames, "get_schema", "should include regular developer tools")
	assert.Contains(t, toolNames, "update_entity", "should include ontology maintenance tools")

	// Data liaison tools should be filtered out
	assert.NotContains(t, toolNames, "suggest_approved_query", "should NOT include suggest_approved_query")
	assert.NotContains(t, toolNames, "suggest_query_update", "should NOT include suggest_query_update")
	assert.NotContains(t, toolNames, "list_query_suggestions", "should NOT include list_query_suggestions")
	assert.NotContains(t, toolNames, "approve_query_suggestion", "should NOT include approve_query_suggestion")
	assert.NotContains(t, toolNames, "reject_query_suggestion", "should NOT include reject_query_suggestion")
	assert.NotContains(t, toolNames, "create_approved_query", "should NOT include create_approved_query")
	assert.NotContains(t, toolNames, "update_approved_query", "should NOT include update_approved_query")
	assert.NotContains(t, toolNames, "delete_approved_query", "should NOT include delete_approved_query")
}

func TestMCPConfigService_Get_IncludesDataLiaisonTools_WhenInstalled(t *testing.T) {
	// When AI Data Liaison app IS installed, data liaison tools should appear in EnabledTools

	projectID := uuid.New()
	datasourceID := uuid.New()

	// Enable developer tools with query tools AND ontology maintenance
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupDeveloper: {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	// AI Data Liaison app IS installed
	installedAppService := &mockInstalledAppServiceForMCP{
		installed: map[string]bool{
			models.AppIDAIDataLiaison: true,
		},
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		installedAppService,
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// Should have developer tools including data liaison tools
	assert.Contains(t, toolNames, "get_schema", "should include regular developer tools")
	assert.Contains(t, toolNames, "list_query_suggestions", "should include list_query_suggestions")
	assert.Contains(t, toolNames, "approve_query_suggestion", "should include approve_query_suggestion")
	assert.Contains(t, toolNames, "create_approved_query", "should include create_approved_query")
}

func TestMCPConfigService_Get_NilInstalledAppService_FiltersDataLiaisonTools(t *testing.T) {
	// When installedAppService is nil, data liaison tools should be filtered (fail closed)

	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupDeveloper: {Enabled: true, AddOntologyMaintenance: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	// No installedAppService passed (nil)
	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // No installed app service
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// Data liaison tools should be filtered (fail closed)
	assert.NotContains(t, toolNames, "suggest_approved_query", "should NOT include suggest_approved_query with nil service")
	assert.NotContains(t, toolNames, "list_query_suggestions", "should NOT include list_query_suggestions with nil service")
}

func TestMCPConfigService_Get_IncludesPerRoleToolLists(t *testing.T) {
	// Test that the response includes all three per-role tool lists
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: true},
				ToolGroupDeveloper:       {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil, // installedAppService - not needed for this test
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// All three role tool lists should be populated
	assert.NotEmpty(t, resp.UserTools, "UserTools should not be empty")
	assert.NotEmpty(t, resp.DeveloperTools, "DeveloperTools should not be empty")
	assert.NotEmpty(t, resp.AgentTools, "AgentTools should not be empty")

	// EnabledTools (deprecated) should still be populated for backward compatibility
	assert.NotEmpty(t, resp.EnabledTools, "EnabledTools should not be empty (backward compat)")
}

func TestMCPConfigService_Get_UserToolsContainsQueryTools(t *testing.T) {
	// Test that UserTools includes query tools and optionally ontology maintenance
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Config with AllowOntologyMaintenance enabled for user tools
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupUser: {AllowOntologyMaintenance: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil,
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	userToolNames := make([]string, len(resp.UserTools))
	for i, tool := range resp.UserTools {
		userToolNames[i] = tool.Name
	}

	// UserTools should include query tools
	assert.Contains(t, userToolNames, "health", "UserTools should include health")
	assert.Contains(t, userToolNames, "query", "UserTools should include query")
	assert.Contains(t, userToolNames, "list_approved_queries", "UserTools should include list_approved_queries")

	// With AllowOntologyMaintenance, should include ontology tools
	assert.Contains(t, userToolNames, "update_entity", "UserTools should include update_entity with AllowOntologyMaintenance")

	// UserTools should NOT include developer-specific tools
	assert.NotContains(t, userToolNames, "echo", "UserTools should NOT include echo (developer only)")
	assert.NotContains(t, userToolNames, "execute", "UserTools should NOT include execute (developer only)")
}

func TestMCPConfigService_Get_UserToolsWithoutOntologyMaintenance(t *testing.T) {
	// Test that UserTools excludes ontology maintenance when option is disabled
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: false},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil,
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	userToolNames := make([]string, len(resp.UserTools))
	for i, tool := range resp.UserTools {
		userToolNames[i] = tool.Name
	}

	// UserTools should include query tools
	assert.Contains(t, userToolNames, "query", "UserTools should include query")

	// Without AllowOntologyMaintenance, should NOT include ontology maintenance tools
	assert.NotContains(t, userToolNames, "update_entity", "UserTools should NOT include update_entity without AllowOntologyMaintenance")
}

func TestMCPConfigService_Get_DeveloperToolsContainsDevCore(t *testing.T) {
	// Test that DeveloperTools includes developer core tools
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupDeveloper: {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil,
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	devToolNames := make([]string, len(resp.DeveloperTools))
	for i, tool := range resp.DeveloperTools {
		devToolNames[i] = tool.Name
	}

	// DeveloperTools should include developer core tools
	assert.Contains(t, devToolNames, "health", "DeveloperTools should include health")
	assert.Contains(t, devToolNames, "echo", "DeveloperTools should include echo")
	assert.Contains(t, devToolNames, "execute", "DeveloperTools should include execute")

	// With AddQueryTools, should include query tools
	assert.Contains(t, devToolNames, "query", "DeveloperTools should include query with AddQueryTools")
	assert.Contains(t, devToolNames, "get_schema", "DeveloperTools should include get_schema with AddQueryTools")

	// With AddOntologyMaintenance, should include ontology tools
	assert.Contains(t, devToolNames, "update_entity", "DeveloperTools should include update_entity with AddOntologyMaintenance")
}

func TestMCPConfigService_Get_DeveloperToolsWithoutSubOptions(t *testing.T) {
	// Test that DeveloperTools only has core tools when sub-options are disabled
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupDeveloper: {Enabled: true, AddQueryTools: false, AddOntologyMaintenance: false},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil,
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	devToolNames := make([]string, len(resp.DeveloperTools))
	for i, tool := range resp.DeveloperTools {
		devToolNames[i] = tool.Name
	}

	// DeveloperTools should include developer core tools
	assert.Contains(t, devToolNames, "health", "DeveloperTools should include health")
	assert.Contains(t, devToolNames, "echo", "DeveloperTools should include echo")
	assert.Contains(t, devToolNames, "execute", "DeveloperTools should include execute")

	// Without AddQueryTools, should NOT include query tools
	assert.NotContains(t, devToolNames, "query", "DeveloperTools should NOT include query without AddQueryTools")

	// Without AddOntologyMaintenance, should NOT include ontology tools
	assert.NotContains(t, devToolNames, "update_entity", "DeveloperTools should NOT include update_entity without AddOntologyMaintenance")
}

func TestMCPConfigService_Get_AgentToolsIsLimited(t *testing.T) {
	// Test that AgentTools is always a limited set regardless of config
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Even with all options enabled, agent tools should be limited
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: true},
				ToolGroupDeveloper:       {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
				ToolGroupAgentTools:      {Enabled: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		nil,
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	agentToolNames := make([]string, len(resp.AgentTools))
	for i, tool := range resp.AgentTools {
		agentToolNames[i] = tool.Name
	}

	// AgentTools should be limited to: health, list_approved_queries, execute_approved_query
	assert.Contains(t, agentToolNames, "health", "AgentTools should include health")
	assert.Contains(t, agentToolNames, "list_approved_queries", "AgentTools should include list_approved_queries")
	assert.Contains(t, agentToolNames, "execute_approved_query", "AgentTools should include execute_approved_query")

	// AgentTools should NOT include any other tools
	assert.Len(t, resp.AgentTools, 3, "AgentTools should have exactly 3 tools")

	// Specifically verify it excludes developer and full query tools
	assert.NotContains(t, agentToolNames, "query", "AgentTools should NOT include query")
	assert.NotContains(t, agentToolNames, "echo", "AgentTools should NOT include echo")
	assert.NotContains(t, agentToolNames, "execute", "AgentTools should NOT include execute")
	assert.NotContains(t, agentToolNames, "update_entity", "AgentTools should NOT include update_entity")
}

func TestMCPConfigService_Get_PerRoleToolsFilterDataLiaison(t *testing.T) {
	// Test that per-role tool lists filter data liaison tools when app not installed
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: true},
				ToolGroupDeveloper:       {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	// AI Data Liaison app is NOT installed
	installedAppService := &mockInstalledAppServiceForMCP{
		installed: map[string]bool{
			models.AppIDAIDataLiaison: false,
		},
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		installedAppService,
		"http://localhost:3443",
		zap.NewNop(),
	)

	resp, err := svc.Get(context.Background(), projectID)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Collect all tool names from all role lists
	allToolNames := make(map[string]bool)
	for _, tool := range resp.UserTools {
		allToolNames[tool.Name] = true
	}
	for _, tool := range resp.DeveloperTools {
		allToolNames[tool.Name] = true
	}
	for _, tool := range resp.AgentTools {
		allToolNames[tool.Name] = true
	}

	// Data liaison tools should NOT be in any role's tool list
	assert.False(t, allToolNames["suggest_approved_query"], "suggest_approved_query should be filtered")
	assert.False(t, allToolNames["suggest_query_update"], "suggest_query_update should be filtered")
	assert.False(t, allToolNames["list_query_suggestions"], "list_query_suggestions should be filtered")
}
