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

func (m *mockMCPConfigService) GetToolGroupsState(ctx context.Context, projectID uuid.UUID) (map[string]*models.ToolGroupConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Return a map with just the configured tool group
	if m.config != nil {
		return map[string]*models.ToolGroupConfig{
			"developer": m.config,
		}, nil
	}
	return map[string]*models.ToolGroupConfig{}, nil
}

// mockQueryService implements services.QueryService for testing.
type mockQueryService struct {
	query          *models.Query
	enabledQueries []*models.Query
	executeResult  *datasource.QueryExecutionResult
	execResult     *datasource.QueryExecutionResult
	executeError   error
}

func (m *mockQueryService) Create(ctx context.Context, projectID, datasourceID uuid.UUID, req *services.CreateQueryRequest) (*models.Query, error) {
	return nil, nil
}

func (m *mockQueryService) Get(ctx context.Context, projectID, queryID uuid.UUID) (*models.Query, error) {
	if m.query != nil {
		return m.query, nil
	}
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

func (m *mockQueryService) ListEnabledByTags(ctx context.Context, projectID, datasourceID uuid.UUID, tags []string) ([]*models.Query, error) {
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
	if m.execResult != nil {
		return m.execResult, m.executeError
	}
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

func (m *mockProjectService) SyncFromCentralAsync(projectID uuid.UUID, papiURL, token string) {
	// No-op for tests
}

func (m *mockProjectService) GetAuthServerURL(ctx context.Context, projectID uuid.UUID) (string, error) {
	return "", nil
}

func (m *mockProjectService) UpdateAuthServerURL(ctx context.Context, projectID uuid.UUID, authServerURL string) error {
	return nil
}

// mockDatasourceService implements services.DatasourceService for testing.
type mockDatasourceService struct {
	datasource      *models.Datasource
	getError        error
	connectionError error
}

func (m *mockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType, provider string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockDatasourceService) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	return m.datasource, m.getError
}

func (m *mockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (m *mockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.DatasourceWithStatus, error) {
	return nil, nil
}

func (m *mockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType, provider string, config map[string]any) error {
	return nil
}

func (m *mockDatasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any, datasourceID uuid.UUID) error {
	return m.connectionError
}

// mockSchemaService implements services.SchemaService for testing.
type mockSchemaService struct {
	refreshResult         *models.RefreshResult
	refreshError          error
	selectAllTablesError  error
	relationshipsResponse *models.RelationshipsResponse
}

func (m *mockSchemaService) RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RefreshResult, error) {
	if m.refreshError != nil {
		return nil, m.refreshError
	}
	return m.refreshResult, nil
}

func (m *mockSchemaService) GetDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error) {
	return nil, nil
}

func (m *mockSchemaService) GetDatasourceTable(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.DatasourceTable, error) {
	return nil, nil
}

func (m *mockSchemaService) AddManualRelationship(ctx context.Context, projectID, datasourceID uuid.UUID, req *models.AddRelationshipRequest) (*models.SchemaRelationship, error) {
	return nil, nil
}

func (m *mockSchemaService) RemoveRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}

func (m *mockSchemaService) GetRelationshipsForDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}

func (m *mockSchemaService) GetRelationshipsResponse(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RelationshipsResponse, error) {
	return m.relationshipsResponse, nil
}

func (m *mockSchemaService) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}

func (m *mockSchemaService) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}

func (m *mockSchemaService) SaveSelections(ctx context.Context, projectID, datasourceID uuid.UUID, tableSelections map[uuid.UUID]bool, columnSelections map[uuid.UUID][]uuid.UUID) error {
	return nil
}

func (m *mockSchemaService) GetSelectedDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error) {
	return nil, nil
}

func (m *mockSchemaService) GetDatasourceSchemaForPrompt(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error) {
	return "", nil
}

func (m *mockSchemaService) GetDatasourceSchemaWithEntities(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error) {
	return "", nil
}

func (m *mockSchemaService) SelectAllTables(ctx context.Context, datasourceID uuid.UUID) error {
	return m.selectAllTablesError
}
