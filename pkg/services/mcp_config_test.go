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

func (m *mockQueryServiceForMCP) Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) (*ValidationResult, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) Test(ctx context.Context, projectID, datasourceID uuid.UUID, req *TestQueryRequest) (*datasource.QueryExecutionResult, error) {
	return nil, nil
}

func (m *mockQueryServiceForMCP) ValidateParameterizedQuery(sqlQuery string, params []models.QueryParameter) error {
	return nil
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

func TestMCPConfigService_Update_ShouldPersistEnabledState(t *testing.T) {
	// This test verifies that when Update() saves enabled=true,
	// the response should reflect enabled=true (when queries exist).
	// This catches the bug where buildResponse was overriding saved state.

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
		"http://localhost:3443",
		zap.NewNop(),
	)

	// Update to enable approved_queries
	req := &UpdateMCPConfigRequest{
		ToolGroups: map[string]*models.ToolGroupConfig{
			ToolGroupApprovedQueries: {Enabled: true},
		},
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The response should show enabled=true
	approvedQueries, ok := resp.ToolGroups[ToolGroupApprovedQueries]
	require.True(t, ok, "approved_queries should be in response")
	assert.True(t, approvedQueries.Enabled, "approved_queries should be enabled in response after Update")

	// Also verify the config was actually saved with enabled=true
	assert.True(t, configRepo.config.ToolGroups[ToolGroupApprovedQueries].Enabled,
		"config should be persisted with enabled=true")
}

func TestMCPConfigService_Update_NoDefaultDatasource_ResponseReflectsSavedState(t *testing.T) {
	// Even when GetDefaultDatasourceID returns nil, the response should
	// reflect the saved state. This was previously a bug where buildResponse
	// would override enabled=false when hasEnabledQueries returned false.

	projectID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: models.DefaultMCPConfig(projectID),
	}

	queryService := &mockQueryServiceForMCP{
		hasEnabledQueries: true,
	}

	// No default datasource configured
	projectService := &mockProjectServiceForMCP{
		defaultDatasourceID: uuid.Nil,
	}

	svc := NewMCPConfigService(
		configRepo,
		queryService,
		projectService,
		"http://localhost:3443",
		zap.NewNop(),
	)

	// Update to enable approved_queries
	req := &UpdateMCPConfigRequest{
		ToolGroups: map[string]*models.ToolGroupConfig{
			ToolGroupApprovedQueries: {Enabled: true},
		},
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// The config should be saved with enabled=true
	require.NotNil(t, configRepo.config.ToolGroups[ToolGroupApprovedQueries])
	assert.True(t, configRepo.config.ToolGroups[ToolGroupApprovedQueries].Enabled,
		"config should be persisted with enabled=true")

	// Response should reflect saved state (enabled=true)
	approvedQueries, ok := resp.ToolGroups[ToolGroupApprovedQueries]
	require.True(t, ok)
	assert.True(t, approvedQueries.Enabled, "response should reflect saved state")
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

func TestMCPConfigService_Update_EnabledToolsReflectNewState(t *testing.T) {
	// When Update() is called, the response EnabledTools should reflect the new state
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
		"http://localhost:3443",
		zap.NewNop(),
	)

	// Update to enable developer tools with AddQueryTools option
	req := &UpdateMCPConfigRequest{
		ToolGroups: map[string]*models.ToolGroupConfig{
			"developer": {Enabled: true, AddQueryTools: true},
		},
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// Developer Core + Query + Ontology Maintenance tools
	assert.Contains(t, toolNames, "echo", "should include echo after enabling developer")
	assert.Contains(t, toolNames, "execute", "should include execute after enabling developer")
	assert.Contains(t, toolNames, "get_schema", "should include get_schema with AddQueryTools")
	assert.Contains(t, toolNames, "sample", "should include sample with AddQueryTools")
	assert.Contains(t, toolNames, "update_entity", "should include update_entity with AddQueryTools")
}

func TestMCPConfigService_Update_AllowOntologyMaintenance(t *testing.T) {
	// Test that enabling AllowOntologyMaintenance for approved_queries adds ontology maintenance tools
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
		"http://localhost:3443",
		zap.NewNop(),
	)

	// Update to enable AllowOntologyMaintenance
	req := &UpdateMCPConfigRequest{
		ToolGroups: map[string]*models.ToolGroupConfig{
			ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: true},
		},
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Verify the state includes AllowOntologyMaintenance
	aqConfig := resp.ToolGroups[ToolGroupApprovedQueries]
	require.NotNil(t, aqConfig, "approved_queries config should exist")
	assert.True(t, aqConfig.AllowOntologyMaintenance, "AllowOntologyMaintenance should be true")

	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// Should have Query tools + Ontology Maintenance tools
	assert.Contains(t, toolNames, "query", "should include query")
	assert.Contains(t, toolNames, "update_entity", "should include update_entity with AllowOntologyMaintenance")
	assert.Contains(t, toolNames, "update_column", "should include update_column with AllowOntologyMaintenance")
}
