package tools

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockMCPConfigService implements services.MCPConfigService for testing.
type mockMCPConfigService struct {
	config                    *models.ToolGroupConfig
	err                       error
	shouldShowApprovedQueries bool
}

func (m *mockMCPConfigService) Get(ctx context.Context, projectID uuid.UUID) (*services.MCPConfigResponse, error) {
	return nil, nil
}

func (m *mockMCPConfigService) Update(ctx context.Context, projectID uuid.UUID, req *services.UpdateMCPConfigRequest) (*services.MCPConfigResponse, error) {
	return nil, nil
}

func (m *mockMCPConfigService) IsToolGroupEnabled(ctx context.Context, projectID uuid.UUID, toolGroup string) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if m.config == nil {
		return false, nil
	}
	return m.config.Enabled, nil
}

func (m *mockMCPConfigService) GetToolGroupConfig(ctx context.Context, projectID uuid.UUID, toolGroup string) (*models.ToolGroupConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

func (m *mockMCPConfigService) ShouldShowApprovedQueriesTools(ctx context.Context, projectID uuid.UUID) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	return m.shouldShowApprovedQueries, nil
}

// mockQueryService implements services.QueryService for testing.
type mockQueryService struct {
	enabledQueries []*models.Query
	executeResult  *datasource.QueryExecutionResult
	executeError   error
}

func (m *mockQueryService) Create(ctx context.Context, projectID, datasourceID uuid.UUID, req *services.CreateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) Get(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) List(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) Update(ctx context.Context, projectID, queryID uuid.UUID, req *services.UpdateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) Delete(ctx context.Context, projectID, queryID uuid.UUID) error {
	return nil
}

func (m *mockQueryService) ListEnabled(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.Query, error) {
	return m.enabledQueries, nil
}

func (m *mockQueryService) HasEnabledQueries(ctx context.Context, projectID, datasourceID uuid.UUID) (bool, error) {
	return len(m.enabledQueries) > 0, nil
}

func (m *mockQueryService) SetEnabledStatus(ctx context.Context, projectID, queryID uuid.UUID, isEnabled bool) error {
	return nil
}

func (m *mockQueryService) Execute(ctx context.Context, projectID, queryID uuid.UUID, req *services.ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	return m.executeResult, m.executeError
}

func (m *mockQueryService) ExecuteWithParameters(ctx context.Context, projectID, queryID uuid.UUID, params map[string]any, req *services.ExecuteQueryRequest) (*datasource.QueryExecutionResult, error) {
	return m.executeResult, m.executeError
}

func (m *mockQueryService) Test(ctx context.Context, projectID, datasourceID uuid.UUID, req *services.TestQueryRequest) (*datasource.QueryExecutionResult, error) {
	return nil, nil
}

func (m *mockQueryService) Validate(ctx context.Context, projectID, datasourceID uuid.UUID, sqlQuery string) (*services.ValidationResult, error) {
	return &services.ValidationResult{Valid: true, Message: "SQL is valid"}, nil
}

func (m *mockQueryService) ValidateParameterizedQuery(sqlQuery string, params []models.QueryParameter) error {
	return nil
}

// mockProjectService implements services.ProjectService for testing.
type mockProjectService struct {
	defaultDatasourceID    uuid.UUID
	defaultDatasourceError error
}

func (m *mockProjectService) Provision(ctx context.Context, projectID uuid.UUID, name string, params map[string]interface{}) (*services.ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectService) ProvisionFromClaims(ctx context.Context, claims *auth.Claims) (*services.ProvisionResult, error) {
	return nil, nil
}

func (m *mockProjectService) GetByID(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectService) GetByIDWithoutTenant(ctx context.Context, id uuid.UUID) (*models.Project, error) {
	return nil, nil
}

func (m *mockProjectService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockProjectService) GetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID) (uuid.UUID, error) {
	if m.defaultDatasourceError != nil {
		return uuid.Nil, m.defaultDatasourceError
	}
	return m.defaultDatasourceID, nil
}

func (m *mockProjectService) SetDefaultDatasourceID(ctx context.Context, projectID uuid.UUID, datasourceID uuid.UUID) error {
	return nil
}

// mockDatasourceService implements services.DatasourceService for testing.
type mockDatasourceService struct {
	datasource      *models.Datasource
	getError        error
	connectionError error
}

func (m *mockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockDatasourceService) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	return m.datasource, m.getError
}

func (m *mockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	return nil, nil
}

func (m *mockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error {
	return nil
}

func (m *mockDatasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any) error {
	return m.connectionError
}
