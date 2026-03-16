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

func (m *mockMCPConfigRepository) GetAuditRetentionDays(_ context.Context, _ uuid.UUID) (*int, error) {
	if m.config != nil {
		return m.config.AuditRetentionDays, m.err
	}
	return nil, m.err
}

func (m *mockMCPConfigRepository) SetAuditRetentionDays(_ context.Context, _ uuid.UUID, days *int) error {
	if m.config != nil {
		m.config.AuditRetentionDays = days
	}
	return m.err
}

func (m *mockMCPConfigRepository) GetAlertConfig(_ context.Context, _ uuid.UUID) (*models.AlertConfig, error) {
	return nil, m.err
}

func (m *mockMCPConfigRepository) SetAlertConfig(_ context.Context, _ uuid.UUID, _ *models.AlertConfig) error {
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

func (m *mockQueryServiceForMCP) ListRejected(ctx context.Context, projectID uuid.UUID) ([]*models.Query, error) {
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

func (m *mockProjectServiceForMCP) Delete(ctx context.Context, id uuid.UUID) (*DeleteResult, error) {
	return &DeleteResult{}, nil
}

func (m *mockProjectServiceForMCP) CompleteDeleteCallback(ctx context.Context, projectID uuid.UUID, action, status, nonce string) (*DeleteCallbackResult, error) {
	return &DeleteCallbackResult{}, nil
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

func (m *mockProjectServiceForMCP) SyncServerURL(ctx context.Context, projectID uuid.UUID, papiURL, token string) error {
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
	// DefaultMCPConfig has "tools" key with all toggles enabled
	// So it includes all tools from all per-app toggles
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

	// DefaultMCPConfig enables all toggles, so many tools should be available
	toolNames := make([]string, len(resp.EnabledTools))
	for i, tool := range resp.EnabledTools {
		toolNames[i] = tool.Name
	}

	// Should include health (always) and tools from all toggles
	assert.Contains(t, toolNames, "health", "should include health tool (always available)")
	assert.Contains(t, toolNames, "echo", "should include echo tool (Direct Database Access)")
	assert.Contains(t, toolNames, "execute", "should include execute tool (Direct Database Access)")
	assert.Contains(t, toolNames, "query", "should include query tool (Direct Database Access / Request)")
	assert.Contains(t, toolNames, "list_approved_queries", "should include list_approved_queries (Request)")
}

func TestMCPConfigService_Get_EnabledToolsWithDeveloperEnabled(t *testing.T) {
	// With new architecture, "developer": {Enabled: true} alone does not enable any tools
	// because no per-app toggles are set. Only health is available.
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

	// Only health - no per-app toggles set
	assert.Contains(t, toolNames, "health", "should include health tool (always available)")
	assert.Len(t, resp.EnabledTools, 1, "only health without per-app toggles")
}

func TestMCPConfigService_Get_EnabledToolsWithDirectDatabaseAccess(t *testing.T) {
	// When AddDirectDatabaseAccess is enabled, execute/echo/query are included
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {AddDirectDatabaseAccess: true},
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

	assert.Contains(t, toolNames, "execute", "should include execute tool with Direct Database Access")
	assert.Contains(t, toolNames, "echo", "should include echo tool with Direct Database Access")
	assert.Contains(t, toolNames, "query", "should include query tool with Direct Database Access")
}

func TestMCPConfigService_Get_EnabledToolsWithApprovedQueries(t *testing.T) {
	// Legacy approved_queries.Enabled has no effect on new toggle system.
	// Without per-app toggles, only health is returned.
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

	// Only health - legacy Enabled flag doesn't set per-app toggles
	assert.Contains(t, toolNames, "health", "should include health tool")
	assert.Len(t, resp.EnabledTools, 1, "only health without per-app toggles")
}

func TestMCPConfigService_Get_EnabledToolsWithAgentTools(t *testing.T) {
	// agent_tools.Enabled doesn't affect EnabledTools (user perspective).
	// Without per-app toggles, only health is returned.
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

	// Only health - no per-app toggles set
	assert.Contains(t, toolNames, "health", "should include health tool")
	assert.Len(t, resp.EnabledTools, 1, "only health without per-app toggles")
}

func TestMCPConfigService_Update_DeveloperToolsReflectNewState(t *testing.T) {
	// When Update() is called with per-app toggles, DeveloperTools should reflect the new state
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

	// Update with new per-app toggles: enable Direct Database Access but disable Ontology Maintenance
	addDirectDB := true
	addOntologyMaint := false
	req := &UpdateMCPConfigRequest{
		AddDirectDatabaseAccess:     &addDirectDB,
		AddOntologyMaintenanceTools: &addOntologyMaint,
	}

	resp, err := svc.Update(context.Background(), projectID, req)
	require.NoError(t, err)
	require.NotNil(t, resp)

	// Check DeveloperTools (the new per-role field) instead of EnabledTools (deprecated)
	toolNames := make([]string, len(resp.DeveloperTools))
	for i, tool := range resp.DeveloperTools {
		toolNames[i] = tool.Name
	}

	// Direct Database Access tools present
	assert.Contains(t, toolNames, "echo", "should include echo in developer tools")
	assert.Contains(t, toolNames, "execute", "should include execute in developer tools")
	assert.Contains(t, toolNames, "query", "should include query with AddDirectDatabaseAccess")

	// Ontology Maintenance tools NOT present (toggle disabled)
	assert.NotContains(t, toolNames, "update_table", "should NOT include update_table when AddOntologyMaintenanceTools=false")
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

	// User tools are restricted to Default + LimitedQuery (health, list_approved_queries, execute_approved_query)
	// regardless of AllowOntologyMaintenance — ontology tools are for admin/data roles only
	toolNames := make([]string, len(resp.UserTools))
	for i, tool := range resp.UserTools {
		toolNames[i] = tool.Name
	}

	assert.Contains(t, toolNames, "health", "should include health in user tools")
	assert.NotContains(t, toolNames, "update_table", "user role should NOT include update_table")
	assert.NotContains(t, toolNames, "update_column", "user role should NOT include update_column")
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

func (m *mockInstalledAppServiceForMCP) Install(ctx context.Context, projectID uuid.UUID, appID string, userID string) (*AppActionResult, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) Activate(ctx context.Context, projectID uuid.UUID, appID string) (*AppActionResult, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) Uninstall(ctx context.Context, projectID uuid.UUID, appID string) (*AppActionResult, error) {
	return nil, nil
}

func (m *mockInstalledAppServiceForMCP) CompleteCallback(ctx context.Context, projectID uuid.UUID, appID, action, status, nonce, userID string) error {
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

func (m *mockInstalledAppServiceForMCP) EnsureInstalled(ctx context.Context, projectID uuid.UUID, appID string) error {
	return nil
}

func TestMCPConfigService_Get_FiltersDataLiaisonTools_WhenNotInstalled(t *testing.T) {
	// When AI Data Liaison app is not installed, its tools should not appear in EnabledTools

	projectID := uuid.New()
	datasourceID := uuid.New()

	// Enable all toggles including approval tools (AI Data Liaison)
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {
					AddDirectDatabaseAccess:     true,
					AddOntologyMaintenanceTools: true,
					AddApprovalTools:            true,
					AddOntologySuggestions:      true,
					AddRequestTools:             true,
				},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	// AI Data Liaison NOT installed, Ontology Forge IS installed
	installedAppService := &mockInstalledAppServiceForMCP{
		installed: map[string]bool{
			models.AppIDAIDataLiaison: false,
			models.AppIDOntologyForge: true,
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

	// Ontology Forge tools should be present (app is installed)
	assert.Contains(t, toolNames, "get_schema", "should include ontology forge tools")
	assert.Contains(t, toolNames, "update_table", "should include ontology maintenance tools")

	// AI Data Liaison tools should be filtered out (app not installed)
	assert.NotContains(t, toolNames, "list_query_suggestions", "should NOT include list_query_suggestions")
	assert.NotContains(t, toolNames, "approve_query_suggestion", "should NOT include approve_query_suggestion")
	assert.NotContains(t, toolNames, "create_approved_query", "should NOT include create_approved_query")
}

func TestMCPConfigService_Get_IncludesDataLiaisonTools_WhenInstalled(t *testing.T) {
	// When AI Data Liaison app IS installed, its tools should appear in EnabledTools

	projectID := uuid.New()
	datasourceID := uuid.New()

	// Enable all toggles
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {
					AddDirectDatabaseAccess:     true,
					AddOntologyMaintenanceTools: true,
					AddApprovalTools:            true,
					AddOntologySuggestions:      true,
					AddRequestTools:             true,
				},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}

	// Both apps installed
	installedAppService := &mockInstalledAppServiceForMCP{
		installed: map[string]bool{
			models.AppIDAIDataLiaison: true,
			models.AppIDOntologyForge: true,
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

	// Should have tools from both apps
	assert.Contains(t, toolNames, "get_schema", "should include ontology forge tools")
	assert.Contains(t, toolNames, "list_query_suggestions", "should include list_query_suggestions")
	assert.Contains(t, toolNames, "approve_query_suggestion", "should include approve_query_suggestion")
	assert.Contains(t, toolNames, "create_approved_query", "should include create_approved_query")
}

func TestMCPConfigService_Get_NilInstalledAppService_FiltersNonMCPServerTools(t *testing.T) {
	// When installedAppService is nil, only MCP Server tools pass (fail closed for other apps)

	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {
					AddDirectDatabaseAccess:     true,
					AddOntologyMaintenanceTools: true,
					AddApprovalTools:            true,
				},
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

	// MCP Server tools should be present (always available)
	assert.Contains(t, toolNames, "health", "health should be present with nil service")
	assert.Contains(t, toolNames, "echo", "echo should be present (MCP Server app)")
	assert.Contains(t, toolNames, "execute", "execute should be present (MCP Server app)")

	// Ontology Forge tools should be filtered (fail closed)
	assert.NotContains(t, toolNames, "get_schema", "should NOT include get_schema with nil service")

	// AI Data Liaison tools should be filtered (fail closed)
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
	// Test that UserTools includes request tools when AddRequestTools toggle is enabled
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {AddOntologySuggestions: true, AddRequestTools: true},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}
	installedAppService := &mockInstalledAppServiceForMCP{
		installed: map[string]bool{
			models.AppIDOntologyForge: true,
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

	userToolNames := make([]string, len(resp.UserTools))
	for i, tool := range resp.UserTools {
		userToolNames[i] = tool.Name
	}

	// AddRequestTools enables AI Data Liaison user tools
	assert.Contains(t, userToolNames, "health", "UserTools should include health")
	assert.Contains(t, userToolNames, "list_approved_queries", "UserTools should include list_approved_queries")
	assert.Contains(t, userToolNames, "execute_approved_query", "UserTools should include execute_approved_query")

	// AddOntologySuggestions enables Ontology Forge user tools
	assert.Contains(t, userToolNames, "get_context", "UserTools should include get_context")
	assert.Contains(t, userToolNames, "get_ontology", "UserTools should include get_ontology")

	// UserTools should NOT include developer-only tools
	assert.NotContains(t, userToolNames, "update_table", "UserTools should NOT include update_table (developer only)")
	assert.NotContains(t, userToolNames, "echo", "UserTools should NOT include echo (developer only)")
	assert.NotContains(t, userToolNames, "execute", "UserTools should NOT include execute (developer only)")
}

func TestMCPConfigService_Get_UserToolsWithoutOntologyMaintenance(t *testing.T) {
	// Test that UserTools excludes ontology suggestions when toggle is disabled
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Only AddRequestTools enabled, not AddOntologySuggestions
	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {AddRequestTools: true, AddOntologySuggestions: false},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}
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

	userToolNames := make([]string, len(resp.UserTools))
	for i, tool := range resp.UserTools {
		userToolNames[i] = tool.Name
	}

	// AddRequestTools enables AI Data Liaison user tools
	assert.Contains(t, userToolNames, "health", "UserTools should include health")
	assert.Contains(t, userToolNames, "list_approved_queries", "UserTools should include list_approved_queries")

	// Ontology suggestions NOT enabled, so no Ontology Forge user tools
	assert.NotContains(t, userToolNames, "get_context", "UserTools should NOT include get_context")
	assert.NotContains(t, userToolNames, "get_ontology", "UserTools should NOT include get_ontology")

	// Developer-only tools should NOT be present
	assert.NotContains(t, userToolNames, "update_table", "UserTools should NOT include update_table (developer only)")
}

func TestMCPConfigService_Get_DeveloperToolsContainsDevCore(t *testing.T) {
	// Test that DeveloperTools includes all developer tools when all toggles are enabled
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {
					AddDirectDatabaseAccess:     true,
					AddOntologyMaintenanceTools: true,
					AddApprovalTools:            true,
				},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}
	installedAppService := &mockInstalledAppServiceForMCP{
		installed: map[string]bool{
			models.AppIDOntologyForge: true,
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

	devToolNames := make([]string, len(resp.DeveloperTools))
	for i, tool := range resp.DeveloperTools {
		devToolNames[i] = tool.Name
	}

	// Direct Database Access tools (MCP Server)
	assert.Contains(t, devToolNames, "health", "DeveloperTools should include health")
	assert.Contains(t, devToolNames, "echo", "DeveloperTools should include echo")
	assert.Contains(t, devToolNames, "execute", "DeveloperTools should include execute")
	assert.Contains(t, devToolNames, "query", "DeveloperTools should include query")

	// Ontology Maintenance tools (Ontology Forge)
	assert.Contains(t, devToolNames, "get_schema", "DeveloperTools should include get_schema")
	assert.Contains(t, devToolNames, "update_table", "DeveloperTools should include update_table")

	// Approval tools (AI Data Liaison)
	assert.Contains(t, devToolNames, "list_query_suggestions", "DeveloperTools should include list_query_suggestions")
}

func TestMCPConfigService_Get_GlossaryToolsOwnedByAIDataLiaison(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {
					AddOntologyMaintenanceTools: true,
					AddOntologySuggestions:      true,
					AddApprovalTools:            true,
					AddRequestTools:             true,
				},
			},
		},
	}

	queryService := &mockQueryServiceForMCP{hasEnabledQueries: true}
	projectService := &mockProjectServiceForMCP{defaultDatasourceID: datasourceID}
	installedAppService := &mockInstalledAppServiceForMCP{
		installed: map[string]bool{
			models.AppIDOntologyForge: true,
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

	userToolApps := make(map[string]string, len(resp.UserTools))
	for _, tool := range resp.UserTools {
		userToolApps[tool.Name] = tool.AppID
	}

	devToolApps := make(map[string]string, len(resp.DeveloperTools))
	for _, tool := range resp.DeveloperTools {
		devToolApps[tool.Name] = tool.AppID
	}

	assert.Equal(t, models.AppIDOntologyForge, userToolApps["get_context"])
	assert.Equal(t, models.AppIDOntologyForge, userToolApps["get_ontology"])
	assert.Equal(t, models.AppIDAIDataLiaison, userToolApps["list_glossary"])
	assert.Equal(t, models.AppIDAIDataLiaison, userToolApps["get_glossary_sql"])
	assert.Equal(t, models.AppIDAIDataLiaison, devToolApps["create_glossary_term"])
	assert.Equal(t, models.AppIDAIDataLiaison, devToolApps["update_glossary_term"])
	assert.Equal(t, models.AppIDAIDataLiaison, devToolApps["delete_glossary_term"])
}

func TestMCPConfigService_Get_DeveloperToolsWithoutSubOptions(t *testing.T) {
	// Test that DeveloperTools only has health when no toggles are enabled
	projectID := uuid.New()
	datasourceID := uuid.New()

	configRepo := &mockMCPConfigRepository{
		config: &models.MCPConfig{
			ProjectID: projectID,
			ToolGroups: map[string]*models.ToolGroupConfig{
				"tools": {
					AddDirectDatabaseAccess:     false,
					AddOntologyMaintenanceTools: false,
					AddApprovalTools:            false,
				},
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

	// Only health when no toggles are enabled
	assert.Contains(t, devToolNames, "health", "DeveloperTools should include health")

	// No other tools without any toggles
	assert.NotContains(t, devToolNames, "echo", "DeveloperTools should NOT include echo without toggles")
	assert.NotContains(t, devToolNames, "execute", "DeveloperTools should NOT include execute without toggles")
	assert.NotContains(t, devToolNames, "query", "DeveloperTools should NOT include query without toggles")
	assert.NotContains(t, devToolNames, "update_table", "DeveloperTools should NOT include update_table without toggles")
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
	assert.NotContains(t, agentToolNames, "update_table", "AgentTools should NOT include update_table")
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
