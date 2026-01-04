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

// Tests

func TestMCPConfigService_Get_ApprovedQueriesDisabledWhenNoEnabledQueries(t *testing.T) {
	// This test verifies that when approved_queries is enabled in config,
	// but there are no enabled queries (e.g., all deleted), the response
	// should show approved_queries as disabled.

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

	// But no enabled queries exist (e.g., all were deleted)
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

	// approved_queries should be disabled because no enabled queries exist
	approvedQueries, ok := resp.ToolGroups[ToolGroupApprovedQueries]
	require.True(t, ok, "approved_queries should be in response")
	assert.False(t, approvedQueries.Enabled, "approved_queries should be disabled when no enabled queries exist")
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

func TestMCPConfigService_Get_NoDatasourceFallsBackToDisabled(t *testing.T) {
	// When project has no default datasource, approved_queries should be disabled
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

	// approved_queries should be disabled because no datasource exists
	approvedQueries, ok := resp.ToolGroups[ToolGroupApprovedQueries]
	require.True(t, ok)
	assert.False(t, approvedQueries.Enabled, "approved_queries should be disabled when no datasource exists")
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
