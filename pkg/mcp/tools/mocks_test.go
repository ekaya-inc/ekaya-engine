package tools

import (
	"context"

	"github.com/google/uuid"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
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

func (m *mockProjectService) GetAutoApproveSettings(ctx context.Context, projectID uuid.UUID) (*services.AutoApproveSettings, error) {
	return nil, nil
}

func (m *mockProjectService) SetAutoApproveSettings(ctx context.Context, projectID uuid.UUID, settings *services.AutoApproveSettings) error {
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
	relationshipsResponse *models.RelationshipsResponse
}

func (m *mockSchemaService) RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID, autoSelect bool) (*models.RefreshResult, error) {
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

func (m *mockSchemaService) SelectAllTables(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

// mockSchemaRepository implements repositories.SchemaRepository for testing.
type mockSchemaRepository struct {
	tables        []*models.SchemaTable
	columns       []*models.SchemaColumn
	relationships []*models.SchemaRelationship
}

func (m *mockSchemaRepository) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	return m.tables, nil
}
func (m *mockSchemaRepository) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepository) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepository) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepository) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return nil
}
func (m *mockSchemaRepository) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepository) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepository) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepository) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepository) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, nil
}
func (m *mockSchemaRepository) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepository) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockSchemaRepository) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepository) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepository) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}
func (m *mockSchemaRepository) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepository) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepository) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64, sampleValues []string) error {
	return nil
}
func (m *mockSchemaRepository) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepository) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return m.relationships, nil
}
func (m *mockSchemaRepository) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepository) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepository) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	return nil
}
func (m *mockSchemaRepository) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepository) DeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepository) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return nil
}
func (m *mockSchemaRepository) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepository) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}
func (m *mockSchemaRepository) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepository) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepository) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	return nil
}
func (m *mockSchemaRepository) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepository) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}
func (m *mockSchemaRepository) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepository) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepository) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

// mockColumnMetadataRepository implements repositories.ColumnMetadataRepository for testing.
type mockColumnMetadataRepository struct {
	metadata map[string]*models.ColumnMetadata // key is "table.column"
}

func newMockColumnMetadataRepository() *mockColumnMetadataRepository {
	return &mockColumnMetadataRepository{
		metadata: make(map[string]*models.ColumnMetadata),
	}
}

func (m *mockColumnMetadataRepository) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	key := meta.TableName + "." + meta.ColumnName
	m.metadata[key] = meta
	return nil
}

func (m *mockColumnMetadataRepository) GetByTableColumn(ctx context.Context, projectID uuid.UUID, tableName, columnName string) (*models.ColumnMetadata, error) {
	key := tableName + "." + columnName
	if meta, ok := m.metadata[key]; ok {
		return meta, nil
	}
	return nil, nil
}

func (m *mockColumnMetadataRepository) GetByTable(ctx context.Context, projectID uuid.UUID, tableName string) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockColumnMetadataRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

func (m *mockColumnMetadataRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockColumnMetadataRepository) DeleteByTableColumn(ctx context.Context, projectID uuid.UUID, tableName, columnName string) error {
	key := tableName + "." + columnName
	delete(m.metadata, key)
	return nil
}
