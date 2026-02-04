package services

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// testContextWithAuth creates a context with JWT claims for testing
func testContextWithAuth(projectID, userID string) context.Context {
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID,
		},
		ProjectID: projectID,
	}
	return context.WithValue(context.Background(), auth.ClaimsKey, claims)
}

// ============================================================================
// Mock Implementations
// ============================================================================

// mockSchemaRepository is a configurable mock for testing.
type mockSchemaRepository struct {
	// Tables
	tables            []*models.SchemaTable
	tableByName       *models.SchemaTable
	upsertTableErr    error
	softDeletedTables int64

	// Columns
	columns            []*models.SchemaColumn
	columnByName       *models.SchemaColumn
	columnsByTable     map[string][]*models.SchemaColumn
	upsertColumnErr    error
	softDeletedColumns int64

	// Relationships
	relationships         []*models.SchemaRelationship
	upsertRelationshipErr error
	softDeletedRels       int64

	// Error returns
	listTablesErr              error
	getTableByNameErr          error
	listColumnsErr             error
	getColumnByNameErr         error
	listRelationshipsErr       error
	softDeleteTablesErr        error
	softDeleteColumnsErr       error
	softDeleteOrphanedErr      error
	getRelationshipByIDErr     error
	getRelationshipByColsErr   error
	updateApprovalErr          error
	relationshipByColsResponse *models.SchemaRelationship
	updateTableMetadataErr     error
	updateColumnMetadataErr    error
	updateTableSelectionErr    error
	updateColumnSelectionErr   error

	// Capture for verification
	upsertedTables        []*models.SchemaTable
	upsertedColumns       []*models.SchemaColumn
	upsertedRelationships []*models.SchemaRelationship
}

func (m *mockSchemaRepository) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	if m.listTablesErr != nil {
		return nil, m.listTablesErr
	}
	return m.tables, nil
}

func (m *mockSchemaRepository) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	for _, t := range m.tables {
		if t.ID == tableID {
			return t, nil
		}
	}
	return nil, errors.New("table not found")
}

func (m *mockSchemaRepository) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	if m.getTableByNameErr != nil {
		return nil, m.getTableByNameErr
	}
	if m.tableByName != nil {
		return m.tableByName, nil
	}
	// Search in tables list
	for _, t := range m.tables {
		if t.SchemaName == schemaName && t.TableName == tableName {
			return t, nil
		}
	}
	// Also check upserted tables
	for _, t := range m.upsertedTables {
		if t.SchemaName == schemaName && t.TableName == tableName {
			return t, nil
		}
	}
	return nil, errors.New("table not found")
}

func (m *mockSchemaRepository) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	if m.getTableByNameErr != nil {
		return nil, m.getTableByNameErr
	}
	// Search in tables list by table name only (schema-agnostic)
	for _, t := range m.tables {
		if t.TableName == tableName {
			return t, nil
		}
	}
	// Also check upserted tables
	for _, t := range m.upsertedTables {
		if t.TableName == tableName {
			return t, nil
		}
	}
	return nil, errors.New("table not found")
}

func (m *mockSchemaRepository) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	if m.upsertTableErr != nil {
		return m.upsertTableErr
	}
	if table.ID == uuid.Nil {
		table.ID = uuid.New()
	}
	m.upsertedTables = append(m.upsertedTables, table)
	return nil
}

func (m *mockSchemaRepository) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	if m.softDeleteTablesErr != nil {
		return 0, m.softDeleteTablesErr
	}
	return m.softDeletedTables, nil
}

func (m *mockSchemaRepository) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	if m.updateTableSelectionErr != nil {
		return m.updateTableSelectionErr
	}
	return nil
}

func (m *mockSchemaRepository) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	if m.updateTableMetadataErr != nil {
		return m.updateTableMetadataErr
	}
	return nil
}

func (m *mockSchemaRepository) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
	if m.listColumnsErr != nil {
		return nil, m.listColumnsErr
	}
	var result []*models.SchemaColumn
	for _, c := range m.columns {
		if c.SchemaTableID == tableID {
			result = append(result, c)
		}
	}
	return result, nil
}

func (m *mockSchemaRepository) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	if m.listColumnsErr != nil {
		return nil, m.listColumnsErr
	}
	return m.columns, nil
}

func (m *mockSchemaRepository) GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockSchemaRepository) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	for _, c := range m.columns {
		if c.ID == columnID {
			return c, nil
		}
	}
	return nil, errors.New("column not found")
}

func (m *mockSchemaRepository) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	if m.getColumnByNameErr != nil {
		return nil, m.getColumnByNameErr
	}
	if m.columnByName != nil {
		return m.columnByName, nil
	}
	// Search in columns list
	for _, c := range m.columns {
		if c.SchemaTableID == tableID && c.ColumnName == columnName {
			return c, nil
		}
	}
	// Also check upserted columns
	for _, c := range m.upsertedColumns {
		if c.SchemaTableID == tableID && c.ColumnName == columnName {
			return c, nil
		}
	}
	return nil, errors.New("column not found")
}

func (m *mockSchemaRepository) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	if m.upsertColumnErr != nil {
		return m.upsertColumnErr
	}
	if column.ID == uuid.Nil {
		column.ID = uuid.New()
	}
	m.upsertedColumns = append(m.upsertedColumns, column)
	return nil
}

func (m *mockSchemaRepository) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	if m.softDeleteColumnsErr != nil {
		return 0, m.softDeleteColumnsErr
	}
	return m.softDeletedColumns, nil
}

func (m *mockSchemaRepository) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	if m.updateColumnSelectionErr != nil {
		return m.updateColumnSelectionErr
	}
	return nil
}

func (m *mockSchemaRepository) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64) error {
	return nil
}

func (m *mockSchemaRepository) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	if m.updateColumnMetadataErr != nil {
		return m.updateColumnMetadataErr
	}
	return nil
}

func (m *mockSchemaRepository) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	if m.listRelationshipsErr != nil {
		return nil, m.listRelationshipsErr
	}
	return m.relationships, nil
}

func (m *mockSchemaRepository) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	if m.getRelationshipByIDErr != nil {
		return nil, m.getRelationshipByIDErr
	}
	for _, r := range m.relationships {
		if r.ID == relationshipID {
			return r, nil
		}
	}
	return nil, errors.New("relationship not found")
}

func (m *mockSchemaRepository) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	if m.getRelationshipByColsErr != nil {
		return nil, m.getRelationshipByColsErr
	}
	if m.relationshipByColsResponse != nil {
		return m.relationshipByColsResponse, nil
	}
	for _, r := range m.relationships {
		if r.SourceColumnID == sourceColumnID && r.TargetColumnID == targetColumnID {
			return r, nil
		}
	}
	return nil, nil
}

func (m *mockSchemaRepository) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	if m.upsertRelationshipErr != nil {
		return m.upsertRelationshipErr
	}
	if rel.ID == uuid.Nil {
		rel.ID = uuid.New()
	}
	m.upsertedRelationships = append(m.upsertedRelationships, rel)
	return nil
}

func (m *mockSchemaRepository) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	if m.updateApprovalErr != nil {
		return m.updateApprovalErr
	}
	return nil
}

func (m *mockSchemaRepository) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}

func (m *mockSchemaRepository) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	if m.softDeleteOrphanedErr != nil {
		return 0, m.softDeleteOrphanedErr
	}
	return m.softDeletedRels, nil
}

// Relationship Discovery methods (Phase 3)

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
	if rel.ID == uuid.Nil {
		rel.ID = uuid.New()
	}
	m.upsertedRelationships = append(m.upsertedRelationships, rel)
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

func (m *mockSchemaRepository) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
	if m.columnsByTable != nil {
		return m.columnsByTable, nil
	}
	return make(map[string][]*models.SchemaColumn), nil
}

func (m *mockSchemaRepository) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return len(m.columns), nil
}

func (m *mockSchemaRepository) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockSchemaRepository) GetRelationshipsByMethod(ctx context.Context, projectID, datasourceID uuid.UUID, method string) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepository) DeleteInferredRelationshipsByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	return 0, nil
}

// mockDatasourceService is a mock for DatasourceService.
type mockDatasourceService struct {
	datasource *models.Datasource
	getErr     error
}

func (m *mockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType, provider string, config map[string]any) (*models.Datasource, error) {
	return nil, errors.New("not implemented")
}

func (m *mockDatasourceService) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.datasource != nil {
		return m.datasource, nil
	}
	return &models.Datasource{
		ID:             id,
		ProjectID:      projectID,
		DatasourceType: "postgres",
		Config:         map[string]any{"host": "localhost"},
	}, nil
}

func (m *mockDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, errors.New("not implemented")
}

func (m *mockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.DatasourceWithStatus, error) {
	return nil, errors.New("not implemented")
}

func (m *mockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType, provider string, config map[string]any) error {
	return errors.New("not implemented")
}

func (m *mockDatasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.New("not implemented")
}

func (m *mockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any, datasourceID uuid.UUID) error {
	return errors.New("not implemented")
}

// mockSchemaDiscoverer is a mock for datasource.SchemaDiscoverer.
type mockSchemaDiscoverer struct {
	tables            []datasource.TableMetadata
	columns           map[string][]datasource.ColumnMetadata // key: schema.table
	foreignKeys       []datasource.ForeignKeyMetadata
	supportsFKs       bool
	discoverTablesErr error
	discoverColsErr   error
	discoverFKsErr    error
}

func (m *mockSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	if m.discoverTablesErr != nil {
		return nil, m.discoverTablesErr
	}
	return m.tables, nil
}

func (m *mockSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	if m.discoverColsErr != nil {
		return nil, m.discoverColsErr
	}
	key := schemaName + "." + tableName
	return m.columns[key], nil
}

func (m *mockSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	if m.discoverFKsErr != nil {
		return nil, m.discoverFKsErr
	}
	return m.foreignKeys, nil
}

func (m *mockSchemaDiscoverer) SupportsForeignKeys() bool {
	return m.supportsFKs
}

func (m *mockSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	return nil, nil
}

func (m *mockSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	return nil, nil
}

func (m *mockSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	return nil, nil
}

func (m *mockSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockSchemaDiscoverer) GetEnumValueDistribution(ctx context.Context, schemaName, tableName, columnName string, completionTimestampCol string, limit int) (*datasource.EnumDistributionResult, error) {
	return nil, nil
}

func (m *mockSchemaDiscoverer) Close() error {
	return nil
}

// mockSchemaAdapterFactory is a mock for DatasourceAdapterFactory.
type mockSchemaAdapterFactory struct {
	discoverer    *mockSchemaDiscoverer
	discovererErr error
}

func (m *mockSchemaAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSchemaAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	if m.discovererErr != nil {
		return nil, m.discovererErr
	}
	return m.discoverer, nil
}

func (m *mockSchemaAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

func (m *mockSchemaAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, errors.New("not implemented in mock")
}

// ============================================================================
// Helper Functions
// ============================================================================

func newTestSchemaService(repo *mockSchemaRepository, dsSvc *mockDatasourceService, factory *mockSchemaAdapterFactory) SchemaService {
	return NewSchemaService(repo, nil, nil, nil, dsSvc, factory, zap.NewNop())
}

// ============================================================================
// Tests
// ============================================================================

func TestSchemaService_RefreshDatasourceSchema_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost"},
		},
	}
	discoverer := &mockSchemaDiscoverer{
		tables: []datasource.TableMetadata{
			{SchemaName: "public", TableName: "users", RowCount: 100},
			{SchemaName: "public", TableName: "orders", RowCount: 500},
		},
		columns: map[string][]datasource.ColumnMetadata{
			"public.users": {
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1},
				{ColumnName: "email", DataType: "text", IsNullable: false, OrdinalPosition: 2},
			},
			"public.orders": {
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1},
				{ColumnName: "user_id", DataType: "uuid", IsNullable: false, OrdinalPosition: 2},
			},
		},
		supportsFKs: true,
		foreignKeys: []datasource.ForeignKeyMetadata{
			{
				ConstraintName: "fk_orders_user",
				SourceSchema:   "public",
				SourceTable:    "orders",
				SourceColumn:   "user_id",
				TargetSchema:   "public",
				TargetTable:    "users",
				TargetColumn:   "id",
			},
		},
	}
	factory := &mockSchemaAdapterFactory{discoverer: discoverer}

	service := newTestSchemaService(repo, dsSvc, factory)

	ctx := testContextWithAuth(projectID.String(), "test-user-id")
	result, err := service.RefreshDatasourceSchema(ctx, projectID, datasourceID, false)
	if err != nil {
		t.Fatalf("RefreshDatasourceSchema failed: %v", err)
	}

	if result.TablesUpserted != 2 {
		t.Errorf("expected 2 tables upserted, got %d", result.TablesUpserted)
	}
	if result.ColumnsUpserted != 4 {
		t.Errorf("expected 4 columns upserted, got %d", result.ColumnsUpserted)
	}
	if result.RelationshipsCreated != 1 {
		t.Errorf("expected 1 relationship created, got %d", result.RelationshipsCreated)
	}

	// Verify tables were upserted
	if len(repo.upsertedTables) != 2 {
		t.Errorf("expected 2 tables in repo, got %d", len(repo.upsertedTables))
	}

	// Verify columns were upserted
	if len(repo.upsertedColumns) != 4 {
		t.Errorf("expected 4 columns in repo, got %d", len(repo.upsertedColumns))
	}

	// Verify relationships were upserted
	if len(repo.upsertedRelationships) != 1 {
		t.Errorf("expected 1 relationship in repo, got %d", len(repo.upsertedRelationships))
	}
}

func TestSchemaService_RefreshDatasourceSchema_NoTables(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	discoverer := &mockSchemaDiscoverer{
		tables:      []datasource.TableMetadata{},
		columns:     map[string][]datasource.ColumnMetadata{},
		supportsFKs: false,
	}
	factory := &mockSchemaAdapterFactory{discoverer: discoverer}

	service := newTestSchemaService(repo, dsSvc, factory)

	ctx := testContextWithAuth(projectID.String(), "test-user-id")
	result, err := service.RefreshDatasourceSchema(ctx, projectID, datasourceID, false)
	if err != nil {
		t.Fatalf("RefreshDatasourceSchema failed: %v", err)
	}

	if result.TablesUpserted != 0 {
		t.Errorf("expected 0 tables upserted, got %d", result.TablesUpserted)
	}
	if result.ColumnsUpserted != 0 {
		t.Errorf("expected 0 columns upserted, got %d", result.ColumnsUpserted)
	}
	if result.RelationshipsCreated != 0 {
		t.Errorf("expected 0 relationships created, got %d", result.RelationshipsCreated)
	}
}

func TestSchemaService_RefreshDatasourceSchema_DatasourceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{
		getErr: errors.New("datasource not found"),
	}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	_, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID, false)
	if err == nil {
		t.Fatal("expected error from datasource service")
	}
}

func TestSchemaService_RefreshDatasourceSchema_AdapterError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{
		discovererErr: errors.New("connection refused"),
	}

	service := newTestSchemaService(repo, dsSvc, factory)

	_, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID, false)
	if err == nil {
		t.Fatal("expected error from adapter factory")
	}
}

func TestSchemaService_RefreshDatasourceSchema_DiscoverTablesError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	discoverer := &mockSchemaDiscoverer{
		discoverTablesErr: errors.New("query failed"),
	}
	factory := &mockSchemaAdapterFactory{discoverer: discoverer}

	service := newTestSchemaService(repo, dsSvc, factory)

	_, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID, false)
	if err == nil {
		t.Fatal("expected error from discover tables")
	}
}

func TestSchemaService_RefreshDatasourceSchema_NoFKSupport(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	discoverer := &mockSchemaDiscoverer{
		tables: []datasource.TableMetadata{
			{SchemaName: "public", TableName: "users", RowCount: 100},
		},
		columns: map[string][]datasource.ColumnMetadata{
			"public.users": {
				{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1},
			},
		},
		supportsFKs: false, // No FK support
	}
	factory := &mockSchemaAdapterFactory{discoverer: discoverer}

	service := newTestSchemaService(repo, dsSvc, factory)

	ctx := testContextWithAuth(projectID.String(), "test-user-id")
	result, err := service.RefreshDatasourceSchema(ctx, projectID, datasourceID, false)
	if err != nil {
		t.Fatalf("RefreshDatasourceSchema failed: %v", err)
	}

	if result.RelationshipsCreated != 0 {
		t.Errorf("expected 0 relationships when FK not supported, got %d", result.RelationshipsCreated)
	}
}

func TestSchemaService_GetDatasourceSchema_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()
	columnID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID,
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
			},
		},
		relationships: []*models.SchemaRelationship{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	schema, err := service.GetDatasourceSchema(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("GetDatasourceSchema failed: %v", err)
	}

	if schema.ProjectID != projectID {
		t.Errorf("expected project ID %s, got %s", projectID, schema.ProjectID)
	}
	if schema.DatasourceID != datasourceID {
		t.Errorf("expected datasource ID %s, got %s", datasourceID, schema.DatasourceID)
	}
	if len(schema.Tables) != 1 {
		t.Fatalf("expected 1 table, got %d", len(schema.Tables))
	}
	if schema.Tables[0].TableName != "users" {
		t.Errorf("expected table name 'users', got %q", schema.Tables[0].TableName)
	}
	if len(schema.Tables[0].Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(schema.Tables[0].Columns))
	}
	if schema.Tables[0].Columns[0].ColumnName != "id" {
		t.Errorf("expected column name 'id', got %q", schema.Tables[0].Columns[0].ColumnName)
	}
}

func TestSchemaService_GetDatasourceSchema_Empty(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{
		tables:        []*models.SchemaTable{},
		columns:       []*models.SchemaColumn{},
		relationships: []*models.SchemaRelationship{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	schema, err := service.GetDatasourceSchema(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("GetDatasourceSchema failed: %v", err)
	}

	if len(schema.Tables) != 0 {
		t.Errorf("expected 0 tables, got %d", len(schema.Tables))
	}
	if len(schema.Relationships) != 0 {
		t.Errorf("expected 0 relationships, got %d", len(schema.Relationships))
	}
}

func TestSchemaService_GetDatasourceTable_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()
	columnID := uuid.New()

	rowCount := int64(100)
	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
				RowCount:     &rowCount,
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID,
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "email",
				DataType:      "text",
				IsNullable:    false,
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	table, err := service.GetDatasourceTable(context.Background(), projectID, datasourceID, "public.users")
	if err != nil {
		t.Fatalf("GetDatasourceTable failed: %v", err)
	}

	if table.TableName != "users" {
		t.Errorf("expected table name 'users', got %q", table.TableName)
	}
	if table.SchemaName != "public" {
		t.Errorf("expected schema name 'public', got %q", table.SchemaName)
	}
	if table.RowCount != 100 {
		t.Errorf("expected row count 100, got %d", table.RowCount)
	}
	if len(table.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(table.Columns))
	}
}

func TestSchemaService_GetDatasourceTable_DefaultSchema(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
			},
		},
		columns: []*models.SchemaColumn{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	// Call with just table name (no schema prefix)
	table, err := service.GetDatasourceTable(context.Background(), projectID, datasourceID, "users")
	if err != nil {
		t.Fatalf("GetDatasourceTable failed: %v", err)
	}

	if table.SchemaName != "public" {
		t.Errorf("expected default schema 'public', got %q", table.SchemaName)
	}
}

func TestSchemaService_GetDatasourceTable_NotFound(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{
		getTableByNameErr: errors.New("table not found"),
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	_, err := service.GetDatasourceTable(context.Background(), projectID, datasourceID, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent table")
	}
}

func TestSchemaService_Interface(t *testing.T) {
	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}
	service := newTestSchemaService(repo, dsSvc, factory)
	var _ SchemaService = service
}

// ============================================================================
// Phase 7: Relationship Management Tests
// ============================================================================

func TestSchemaService_AddManualRelationship_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	sourceTableID := uuid.New()
	targetTableID := uuid.New()
	sourceColumnID := uuid.New()
	targetColumnID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           sourceTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
			},
			{
				ID:           targetTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            sourceColumnID,
				ProjectID:     projectID,
				SchemaTableID: sourceTableID,
				ColumnName:    "user_id",
				DataType:      "uuid",
			},
			{
				ID:            targetColumnID,
				ProjectID:     projectID,
				SchemaTableID: targetTableID,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	req := &models.AddRelationshipRequest{
		SourceTableName:  "orders",
		SourceColumnName: "user_id",
		TargetTableName:  "users",
		TargetColumnName: "id",
	}

	rel, err := service.AddManualRelationship(context.Background(), projectID, datasourceID, req)
	if err != nil {
		t.Fatalf("AddManualRelationship failed: %v", err)
	}

	if rel.RelationshipType != models.RelationshipTypeManual {
		t.Errorf("expected relationship type %q, got %q", models.RelationshipTypeManual, rel.RelationshipType)
	}
	if rel.Confidence != 1.0 {
		t.Errorf("expected confidence 1.0, got %f", rel.Confidence)
	}
	if rel.IsApproved == nil || !*rel.IsApproved {
		t.Error("expected is_approved to be true")
	}
	if rel.Cardinality != models.CardinalityUnknown {
		t.Errorf("expected cardinality %q, got %q", models.CardinalityUnknown, rel.Cardinality)
	}

	// Verify relationship was upserted
	if len(repo.upsertedRelationships) != 1 {
		t.Errorf("expected 1 relationship upserted, got %d", len(repo.upsertedRelationships))
	}
}

func TestSchemaService_AddManualRelationship_WithSchemaPrefix(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	sourceTableID := uuid.New()
	targetTableID := uuid.New()
	sourceColumnID := uuid.New()
	targetColumnID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           sourceTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "sales",
				TableName:    "orders",
			},
			{
				ID:           targetTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            sourceColumnID,
				ProjectID:     projectID,
				SchemaTableID: sourceTableID,
				ColumnName:    "user_id",
				DataType:      "uuid",
			},
			{
				ID:            targetColumnID,
				ProjectID:     projectID,
				SchemaTableID: targetTableID,
				ColumnName:    "id",
				DataType:      "uuid",
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	req := &models.AddRelationshipRequest{
		SourceTableName:  "sales.orders",
		SourceColumnName: "user_id",
		TargetTableName:  "public.users",
		TargetColumnName: "id",
	}

	rel, err := service.AddManualRelationship(context.Background(), projectID, datasourceID, req)
	if err != nil {
		t.Fatalf("AddManualRelationship failed: %v", err)
	}

	if rel.SourceTableID != sourceTableID {
		t.Errorf("expected source table ID %s, got %s", sourceTableID, rel.SourceTableID)
	}
	if rel.TargetTableID != targetTableID {
		t.Errorf("expected target table ID %s, got %s", targetTableID, rel.TargetTableID)
	}
}

func TestSchemaService_AddManualRelationship_SourceTableNotFound(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{
		getTableByNameErr: errors.New("table not found"),
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	req := &models.AddRelationshipRequest{
		SourceTableName:  "nonexistent",
		SourceColumnName: "user_id",
		TargetTableName:  "users",
		TargetColumnName: "id",
	}

	_, err := service.AddManualRelationship(context.Background(), projectID, datasourceID, req)
	if err == nil {
		t.Fatal("expected error for source table not found")
	}
	if !contains(err.Error(), "source table not found") {
		t.Errorf("expected error to contain 'source table not found', got: %v", err)
	}
}

func TestSchemaService_AddManualRelationship_SourceColumnNotFound(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	sourceTableID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           sourceTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
			},
		},
		getColumnByNameErr: errors.New("column not found"),
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	req := &models.AddRelationshipRequest{
		SourceTableName:  "orders",
		SourceColumnName: "nonexistent",
		TargetTableName:  "users",
		TargetColumnName: "id",
	}

	_, err := service.AddManualRelationship(context.Background(), projectID, datasourceID, req)
	if err == nil {
		t.Fatal("expected error for source column not found")
	}
	if !contains(err.Error(), "source column not found") {
		t.Errorf("expected error to contain 'source column not found', got: %v", err)
	}
}

func TestSchemaService_AddManualRelationship_TargetTableNotFound(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	sourceTableID := uuid.New()
	sourceColumnID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           sourceTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            sourceColumnID,
				ProjectID:     projectID,
				SchemaTableID: sourceTableID,
				ColumnName:    "user_id",
				DataType:      "uuid",
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	req := &models.AddRelationshipRequest{
		SourceTableName:  "orders",
		SourceColumnName: "user_id",
		TargetTableName:  "nonexistent",
		TargetColumnName: "id",
	}

	_, err := service.AddManualRelationship(context.Background(), projectID, datasourceID, req)
	if err == nil {
		t.Fatal("expected error for target table not found")
	}
	if !contains(err.Error(), "target table not found") {
		t.Errorf("expected error to contain 'target table not found', got: %v", err)
	}
}

func TestSchemaService_AddManualRelationship_TargetColumnNotFound(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	sourceTableID := uuid.New()
	targetTableID := uuid.New()
	sourceColumnID := uuid.New()

	// Create a mock that returns success for source table/column but fails on target column
	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           sourceTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
			},
			{
				ID:           targetTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            sourceColumnID,
				ProjectID:     projectID,
				SchemaTableID: sourceTableID,
				ColumnName:    "user_id",
				DataType:      "uuid",
			},
			// No target column - will cause not found
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	req := &models.AddRelationshipRequest{
		SourceTableName:  "orders",
		SourceColumnName: "user_id",
		TargetTableName:  "users",
		TargetColumnName: "id",
	}

	_, err := service.AddManualRelationship(context.Background(), projectID, datasourceID, req)
	if err == nil {
		t.Fatal("expected error for target column not found")
	}
	if !contains(err.Error(), "target column not found") {
		t.Errorf("expected error to contain 'target column not found', got: %v", err)
	}
}

func TestSchemaService_AddManualRelationship_AlreadyExists(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	sourceTableID := uuid.New()
	targetTableID := uuid.New()
	sourceColumnID := uuid.New()
	targetColumnID := uuid.New()
	existingRelID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           sourceTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
			},
			{
				ID:           targetTableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            sourceColumnID,
				ProjectID:     projectID,
				SchemaTableID: sourceTableID,
				ColumnName:    "user_id",
				DataType:      "uuid",
			},
			{
				ID:            targetColumnID,
				ProjectID:     projectID,
				SchemaTableID: targetTableID,
				ColumnName:    "id",
				DataType:      "uuid",
			},
		},
		// Return an existing relationship
		relationshipByColsResponse: &models.SchemaRelationship{
			ID:             existingRelID,
			ProjectID:      projectID,
			SourceColumnID: sourceColumnID,
			TargetColumnID: targetColumnID,
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	req := &models.AddRelationshipRequest{
		SourceTableName:  "orders",
		SourceColumnName: "user_id",
		TargetTableName:  "users",
		TargetColumnName: "id",
	}

	_, err := service.AddManualRelationship(context.Background(), projectID, datasourceID, req)
	if err == nil {
		t.Fatal("expected error for existing relationship")
	}
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict, got: %v", err)
	}
}

func TestSchemaService_AddManualRelationship_EmptySourceTable(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	req := &models.AddRelationshipRequest{
		SourceTableName:  "",
		SourceColumnName: "user_id",
		TargetTableName:  "users",
		TargetColumnName: "id",
	}

	_, err := service.AddManualRelationship(context.Background(), projectID, datasourceID, req)
	if err == nil {
		t.Fatal("expected error for empty source table")
	}
}

func TestSchemaService_RemoveRelationship_Success(t *testing.T) {
	projectID := uuid.New()
	relationshipID := uuid.New()

	repo := &mockSchemaRepository{
		relationships: []*models.SchemaRelationship{
			{
				ID:        relationshipID,
				ProjectID: projectID,
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	err := service.RemoveRelationship(context.Background(), projectID, relationshipID)
	if err != nil {
		t.Fatalf("RemoveRelationship failed: %v", err)
	}
}

func TestSchemaService_RemoveRelationship_NotFound(t *testing.T) {
	projectID := uuid.New()
	relationshipID := uuid.New()

	repo := &mockSchemaRepository{
		relationships: []*models.SchemaRelationship{}, // Empty - no relationships
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	err := service.RemoveRelationship(context.Background(), projectID, relationshipID)
	if err == nil {
		t.Fatal("expected error for relationship not found")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestSchemaService_RemoveRelationship_WrongProject(t *testing.T) {
	projectID := uuid.New()
	otherProjectID := uuid.New()
	relationshipID := uuid.New()

	repo := &mockSchemaRepository{
		relationships: []*models.SchemaRelationship{
			{
				ID:        relationshipID,
				ProjectID: otherProjectID, // Different project
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	err := service.RemoveRelationship(context.Background(), projectID, relationshipID)
	if err == nil {
		t.Fatal("expected error for wrong project")
	}
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestSchemaService_GetRelationshipsForDatasource_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	rel1ID := uuid.New()
	rel2ID := uuid.New()

	repo := &mockSchemaRepository{
		relationships: []*models.SchemaRelationship{
			{
				ID:               rel1ID,
				ProjectID:        projectID,
				RelationshipType: models.RelationshipTypeFK,
			},
			{
				ID:               rel2ID,
				ProjectID:        projectID,
				RelationshipType: models.RelationshipTypeManual,
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	relationships, err := service.GetRelationshipsForDatasource(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("GetRelationshipsForDatasource failed: %v", err)
	}

	if len(relationships) != 2 {
		t.Errorf("expected 2 relationships, got %d", len(relationships))
	}
}

func TestSchemaService_GetRelationshipsForDatasource_Empty(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{
		relationships: []*models.SchemaRelationship{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	relationships, err := service.GetRelationshipsForDatasource(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("GetRelationshipsForDatasource failed: %v", err)
	}

	if len(relationships) != 0 {
		t.Errorf("expected 0 relationships, got %d", len(relationships))
	}
}

func TestSchemaService_GetRelationshipsForDatasource_Error(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{
		listRelationshipsErr: errors.New("database error"),
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	_, err := service.GetRelationshipsForDatasource(context.Background(), projectID, datasourceID)
	if err == nil {
		t.Fatal("expected error from repository")
	}
}

// ============================================================================
// Phase 8: Selection & Metadata Tests
// ============================================================================

func TestSchemaService_UpdateTableMetadata_Success(t *testing.T) {
	projectID := uuid.New()
	tableID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	businessName := "Users Table"
	description := "Contains user accounts"

	err := service.UpdateTableMetadata(context.Background(), projectID, tableID, &businessName, &description)
	if err != nil {
		t.Fatalf("UpdateTableMetadata failed: %v", err)
	}
}

func TestSchemaService_UpdateTableMetadata_PartialUpdate(t *testing.T) {
	projectID := uuid.New()
	tableID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	// Update only business name
	businessName := "Users"
	err := service.UpdateTableMetadata(context.Background(), projectID, tableID, &businessName, nil)
	if err != nil {
		t.Fatalf("UpdateTableMetadata with partial update failed: %v", err)
	}
}

func TestSchemaService_UpdateTableMetadata_NotFound(t *testing.T) {
	projectID := uuid.New()
	tableID := uuid.New()

	repo := &mockSchemaRepository{
		updateTableMetadataErr: errors.New("table not found"),
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	businessName := "Users"
	err := service.UpdateTableMetadata(context.Background(), projectID, tableID, &businessName, nil)
	if err == nil {
		t.Fatal("expected error for table not found")
	}
}

func TestSchemaService_UpdateColumnMetadata_Success(t *testing.T) {
	// TODO: Re-enable when UpdateColumnMetadata is implemented for ColumnMetadataRepository.
	// See PLAN-column-schema-refactor.md for details.
	t.Skip("UpdateColumnMetadata not yet implemented for new schema")

	projectID := uuid.New()
	columnID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	businessName := "Email Address"
	description := "User's email address"

	err := service.UpdateColumnMetadata(context.Background(), projectID, columnID, &businessName, &description)
	if err != nil {
		t.Fatalf("UpdateColumnMetadata failed: %v", err)
	}
}

func TestSchemaService_UpdateColumnMetadata_NotFound(t *testing.T) {
	projectID := uuid.New()
	columnID := uuid.New()

	repo := &mockSchemaRepository{
		updateColumnMetadataErr: errors.New("column not found"),
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	businessName := "Email"
	err := service.UpdateColumnMetadata(context.Background(), projectID, columnID, &businessName, nil)
	if err == nil {
		t.Fatal("expected error for column not found")
	}
}

func TestSchemaService_SaveSelections_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()
	columnID1 := uuid.New()
	columnID2 := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID1,
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "id",
			},
			{
				ID:            columnID2,
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "email",
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	// Use table and column IDs instead of names
	tableSelections := map[uuid.UUID]bool{
		tableID: true,
	}
	columnSelections := map[uuid.UUID][]uuid.UUID{
		tableID: {columnID1, columnID2},
	}

	err := service.SaveSelections(context.Background(), projectID, datasourceID, tableSelections, columnSelections)
	if err != nil {
		t.Fatalf("SaveSelections failed: %v", err)
	}
}

func TestSchemaService_SaveSelections_EmptyMaps(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	err := service.SaveSelections(context.Background(), projectID, datasourceID, nil, nil)
	if err != nil {
		t.Fatalf("SaveSelections with empty maps failed: %v", err)
	}
}

func TestSchemaService_SaveSelections_UnknownTable(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	unknownTableID := uuid.New() // This ID doesn't exist in the repo

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{}, // No tables
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	// Use an unknown table ID
	tableSelections := map[uuid.UUID]bool{
		unknownTableID: true,
	}

	// Should not error, just skip unknown table IDs
	err := service.SaveSelections(context.Background(), projectID, datasourceID, tableSelections, nil)
	if err != nil {
		t.Fatalf("SaveSelections should not error for unknown table IDs: %v", err)
	}
}

func TestSchemaService_GetSelectedDatasourceSchema_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID1 := uuid.New()
	tableID2 := uuid.New()
	columnID1 := uuid.New()
	columnID2 := uuid.New()
	columnID3 := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID1,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
			},
			{
				ID:           tableID2,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
				IsSelected:   false, // Not selected
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID1,
				ProjectID:     projectID,
				SchemaTableID: tableID1,
				ColumnName:    "id",
				IsSelected:    true,
			},
			{
				ID:            columnID2,
				ProjectID:     projectID,
				SchemaTableID: tableID1,
				ColumnName:    "email",
				IsSelected:    false, // Not selected
			},
			{
				ID:            columnID3,
				ProjectID:     projectID,
				SchemaTableID: tableID2,
				ColumnName:    "id",
				IsSelected:    true,
			},
		},
		relationships: []*models.SchemaRelationship{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	schema, err := service.GetSelectedDatasourceSchema(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("GetSelectedDatasourceSchema failed: %v", err)
	}

	// Should only have 1 table (users is selected, orders is not)
	if len(schema.Tables) != 1 {
		t.Errorf("expected 1 selected table, got %d", len(schema.Tables))
	}

	if schema.Tables[0].TableName != "users" {
		t.Errorf("expected users table, got %s", schema.Tables[0].TableName)
	}

	// Should only have 1 column (id is selected, email is not)
	if len(schema.Tables[0].Columns) != 1 {
		t.Errorf("expected 1 selected column, got %d", len(schema.Tables[0].Columns))
	}

	if schema.Tables[0].Columns[0].ColumnName != "id" {
		t.Errorf("expected id column, got %s", schema.Tables[0].Columns[0].ColumnName)
	}
}

func TestSchemaService_GetSelectedDatasourceSchema_NothingSelected(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   false, // Not selected
			},
		},
		columns:       []*models.SchemaColumn{},
		relationships: []*models.SchemaRelationship{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	schema, err := service.GetSelectedDatasourceSchema(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("GetSelectedDatasourceSchema failed: %v", err)
	}

	if len(schema.Tables) != 0 {
		t.Errorf("expected 0 selected tables, got %d", len(schema.Tables))
	}
}

func TestSchemaService_GetSelectedDatasourceSchema_FiltersRelationships(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID1 := uuid.New()
	tableID2 := uuid.New()
	columnID1 := uuid.New()
	columnID2 := uuid.New()
	relID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID1,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
			},
			{
				ID:           tableID2,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
				IsSelected:   false, // Not selected
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID1,
				ProjectID:     projectID,
				SchemaTableID: tableID1,
				ColumnName:    "id",
				IsSelected:    true,
			},
			{
				ID:            columnID2,
				ProjectID:     projectID,
				SchemaTableID: tableID2,
				ColumnName:    "user_id",
				IsSelected:    true,
			},
		},
		relationships: []*models.SchemaRelationship{
			{
				ID:            relID,
				ProjectID:     projectID,
				SourceTableID: tableID2, // orders (not selected)
				TargetTableID: tableID1, // users (selected)
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	schema, err := service.GetSelectedDatasourceSchema(context.Background(), projectID, datasourceID)
	if err != nil {
		t.Fatalf("GetSelectedDatasourceSchema failed: %v", err)
	}

	// Relationship should be filtered out because orders table is not selected
	if len(schema.Relationships) != 0 {
		t.Errorf("expected 0 relationships (orders not selected), got %d", len(schema.Relationships))
	}
}

func TestSchemaService_GetDatasourceSchemaForPrompt_Full(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()
	columnID := uuid.New()

	rowCount := int64(100)
	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
				RowCount:     &rowCount,
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID,
				ProjectID:     projectID,
				SchemaTableID: tableID,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
				IsNullable:    false,
				IsSelected:    true,
			},
		},
		relationships: []*models.SchemaRelationship{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	prompt, err := service.GetDatasourceSchemaForPrompt(context.Background(), projectID, datasourceID, false)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaForPrompt failed: %v", err)
	}

	// Check key elements are present
	if !contains(prompt, "DATABASE SCHEMA:") {
		t.Error("expected DATABASE SCHEMA header")
	}
	if !contains(prompt, "Table: users") {
		t.Error("expected table name")
	}
	if !contains(prompt, "Row count: 100") {
		t.Error("expected row count")
	}
	if !contains(prompt, "id: uuid") {
		t.Error("expected column definition")
	}
	if !contains(prompt, "PRIMARY KEY") {
		t.Error("expected PRIMARY KEY attribute")
	}
	if !contains(prompt, "NOT NULL") {
		t.Error("expected NOT NULL attribute")
	}
}

func TestSchemaService_GetDatasourceSchemaForPrompt_SelectedOnly(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID1 := uuid.New()
	tableID2 := uuid.New()
	columnID1 := uuid.New()
	columnID2 := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID1,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
			},
			{
				ID:           tableID2,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
				IsSelected:   false, // Not selected
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID1,
				ProjectID:     projectID,
				SchemaTableID: tableID1,
				ColumnName:    "id",
				DataType:      "uuid",
				IsSelected:    true,
			},
			{
				ID:            columnID2,
				ProjectID:     projectID,
				SchemaTableID: tableID2,
				ColumnName:    "id",
				DataType:      "uuid",
				IsSelected:    true,
			},
		},
		relationships: []*models.SchemaRelationship{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	prompt, err := service.GetDatasourceSchemaForPrompt(context.Background(), projectID, datasourceID, true)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaForPrompt failed: %v", err)
	}

	// Should only contain users table (without schema prefix)
	if !contains(prompt, "Table: users") {
		t.Error("expected users table in prompt")
	}
	if contains(prompt, "Table: orders") {
		t.Error("orders table should not be in prompt (not selected)")
	}
}

func TestSchemaService_GetDatasourceSchemaForPrompt_Empty(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{
		tables:        []*models.SchemaTable{},
		columns:       []*models.SchemaColumn{},
		relationships: []*models.SchemaRelationship{},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	prompt, err := service.GetDatasourceSchemaForPrompt(context.Background(), projectID, datasourceID, false)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaForPrompt failed: %v", err)
	}

	// Should still have header
	if !contains(prompt, "DATABASE SCHEMA:") {
		t.Error("expected DATABASE SCHEMA header even for empty schema")
	}
}

func TestSchemaService_GetDatasourceSchemaForPrompt_WithRelationships(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID1 := uuid.New()
	tableID2 := uuid.New()
	columnID1 := uuid.New()
	columnID2 := uuid.New()
	relID := uuid.New()

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID1,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
			},
			{
				ID:           tableID2,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "orders",
				IsSelected:   true,
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID1,
				ProjectID:     projectID,
				SchemaTableID: tableID1,
				ColumnName:    "id",
				DataType:      "uuid",
				IsSelected:    true,
			},
			{
				ID:            columnID2,
				ProjectID:     projectID,
				SchemaTableID: tableID2,
				ColumnName:    "user_id",
				DataType:      "uuid",
				IsSelected:    true,
			},
		},
		relationships: []*models.SchemaRelationship{
			{
				ID:               relID,
				ProjectID:        projectID,
				SourceTableID:    tableID2,
				SourceColumnID:   columnID2,
				TargetTableID:    tableID1,
				TargetColumnID:   columnID1,
				RelationshipType: "fk",
				Cardinality:      "N:1",
			},
		},
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	prompt, err := service.GetDatasourceSchemaForPrompt(context.Background(), projectID, datasourceID, false)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaForPrompt failed: %v", err)
	}

	// Should have relationships section
	if !contains(prompt, "RELATIONSHIPS:") {
		t.Error("expected RELATIONSHIPS section")
	}
	if !contains(prompt, "->") {
		t.Error("expected relationship arrow")
	}
	if !contains(prompt, "(N:1)") {
		t.Error("expected cardinality")
	}
}

// ============================================================================
// Tests for RefreshDatasourceSchema Auth Extraction
// ============================================================================

func TestSchemaService_RefreshDatasourceSchema_NoAuthContext(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost"},
		},
	}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	// Context without auth claims
	_, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID, false)

	if err == nil {
		t.Fatal("expected error when context has no auth claims, got nil")
	}
	if err.Error() != "user ID not found in context: user ID not found in context" {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================================
// Tests for shouldAutoSelect (Table Exclusion Patterns)
// ============================================================================

func TestShouldAutoSelect_NormalTables(t *testing.T) {
	// Normal business tables should be auto-selected
	normalTables := []string{
		"users",
		"orders",
		"billing_transactions",
		"user_profiles",
		"my_table",
		"s_table",     // just starts with s, no digit
		"test",        // just "test", not test_ prefix
		"testing",     // doesn't match test_ pattern
		"backup",      // just "backup", not _backup suffix
		"temp",        // just "temp", not temp_ prefix
		"temporary",   // doesn't match temp_ pattern
		"tests_data",  // tests_ not test_
		"sample_data", // sample_ not s\d+_
	}

	for _, tableName := range normalTables {
		if !shouldAutoSelect(tableName) {
			t.Errorf("expected table %q to be auto-selected, but it was not", tableName)
		}
	}
}

func TestShouldAutoSelect_SamplePrefixes(t *testing.T) {
	// Tables with s1_, s2_, etc. prefixes should NOT be auto-selected
	sampleTables := []string{
		"s1_customers",
		"s2_orders",
		"s10_products",
		"s123_test_data",
		"S1_CUSTOMERS", // uppercase should also be excluded
		"S10_Products", // mixed case
	}

	for _, tableName := range sampleTables {
		if shouldAutoSelect(tableName) {
			t.Errorf("expected table %q to NOT be auto-selected (sample prefix), but it was", tableName)
		}
	}
}

func TestShouldAutoSelect_TestPatterns(t *testing.T) {
	// Tables with test patterns should NOT be auto-selected
	testTables := []string{
		"test_users",
		"test_orders",
		"TEST_DATA", // uppercase
		"Test_Table",
		"users_test",
		"orders_test",
		"USERS_TEST", // uppercase suffix
	}

	for _, tableName := range testTables {
		if shouldAutoSelect(tableName) {
			t.Errorf("expected table %q to NOT be auto-selected (test pattern), but it was", tableName)
		}
	}
}

func TestShouldAutoSelect_TempPatterns(t *testing.T) {
	// Tables with temp patterns should NOT be auto-selected
	tempTables := []string{
		"tmp_users",
		"tmp_staging",
		"TMP_DATA", // uppercase
		"temp_orders",
		"temp_backup",
		"TEMP_STAGING", // uppercase
	}

	for _, tableName := range tempTables {
		if shouldAutoSelect(tableName) {
			t.Errorf("expected table %q to NOT be auto-selected (temp pattern), but it was", tableName)
		}
	}
}

func TestShouldAutoSelect_BackupSuffix(t *testing.T) {
	// Tables with _backup suffix should NOT be auto-selected
	backupTables := []string{
		"users_backup",
		"orders_backup",
		"data_backup",
		"USERS_BACKUP", // uppercase
	}

	for _, tableName := range backupTables {
		if shouldAutoSelect(tableName) {
			t.Errorf("expected table %q to NOT be auto-selected (backup suffix), but it was", tableName)
		}
	}
}

func TestShouldAutoSelect_EdgeCases(t *testing.T) {
	testCases := []struct {
		name     string
		expected bool
	}{
		{"s_users", true},            // "s" followed by non-digit
		{"s0_data", false},           // s0_ matches pattern
		{"test_", false},             // test_ with nothing after
		{"_test", false},             // ends with _test, so excluded
		{"my_test_table", true},      // _test_ in middle, not at end
		{"production_backup", false}, // _backup at end
		{"backup_data", true},        // backup_ at start, not _backup at end
		{"TMP_", false},              // tmp_ with nothing after (case insensitive)
	}

	for _, tc := range testCases {
		result := shouldAutoSelect(tc.name)
		if result != tc.expected {
			t.Errorf("shouldAutoSelect(%q) = %v, expected %v", tc.name, result, tc.expected)
		}
	}
}

func TestSchemaService_RefreshDatasourceSchema_AutoSelectWithExclusion(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost"},
		},
	}
	discoverer := &mockSchemaDiscoverer{
		tables: []datasource.TableMetadata{
			{SchemaName: "public", TableName: "users", RowCount: 100},                // normal - should be selected
			{SchemaName: "public", TableName: "s1_sample", RowCount: 10},             // sample - should NOT be selected
			{SchemaName: "public", TableName: "test_data", RowCount: 5},              // test - should NOT be selected
			{SchemaName: "public", TableName: "orders_backup", RowCount: 50},         // backup - should NOT be selected
			{SchemaName: "public", TableName: "billing_transactions", RowCount: 200}, // normal - should be selected
		},
		columns: map[string][]datasource.ColumnMetadata{
			"public.users":                {{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1}},
			"public.s1_sample":            {{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1}},
			"public.test_data":            {{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1}},
			"public.orders_backup":        {{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1}},
			"public.billing_transactions": {{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1}},
		},
		supportsFKs: false,
	}
	factory := &mockSchemaAdapterFactory{discoverer: discoverer}

	service := newTestSchemaService(repo, dsSvc, factory)

	ctx := testContextWithAuth(projectID.String(), "test-user-id")
	result, err := service.RefreshDatasourceSchema(ctx, projectID, datasourceID, true) // autoSelect = true
	if err != nil {
		t.Fatalf("RefreshDatasourceSchema failed: %v", err)
	}

	if result.TablesUpserted != 5 {
		t.Errorf("expected 5 tables upserted, got %d", result.TablesUpserted)
	}

	// Check that the right tables are selected/not selected
	expectedSelections := map[string]bool{
		"users":                true,  // normal table
		"s1_sample":            false, // sample prefix
		"test_data":            false, // test prefix
		"orders_backup":        false, // backup suffix
		"billing_transactions": true,  // normal table
	}

	for _, table := range repo.upsertedTables {
		expected, ok := expectedSelections[table.TableName]
		if !ok {
			t.Errorf("unexpected table %q in upserted tables", table.TableName)
			continue
		}
		if table.IsSelected != expected {
			t.Errorf("table %q: expected IsSelected=%v, got %v", table.TableName, expected, table.IsSelected)
		}
	}
}

func TestSchemaService_RefreshDatasourceSchema_AutoSelectFalse(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepository{}
	dsSvc := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost"},
		},
	}
	discoverer := &mockSchemaDiscoverer{
		tables: []datasource.TableMetadata{
			{SchemaName: "public", TableName: "users", RowCount: 100},
			{SchemaName: "public", TableName: "s1_sample", RowCount: 10},
		},
		columns: map[string][]datasource.ColumnMetadata{
			"public.users":     {{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1}},
			"public.s1_sample": {{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, OrdinalPosition: 1}},
		},
		supportsFKs: false,
	}
	factory := &mockSchemaAdapterFactory{discoverer: discoverer}

	service := newTestSchemaService(repo, dsSvc, factory)

	ctx := testContextWithAuth(projectID.String(), "test-user-id")
	_, err := service.RefreshDatasourceSchema(ctx, projectID, datasourceID, false) // autoSelect = false
	if err != nil {
		t.Fatalf("RefreshDatasourceSchema failed: %v", err)
	}

	// When autoSelect is false, no tables should be selected regardless of name
	for _, table := range repo.upsertedTables {
		if table.IsSelected {
			t.Errorf("table %q should not be selected when autoSelect=false", table.TableName)
		}
	}
}

// ============================================================================
// Mock Entity Repository
// ============================================================================

// mockEntityRepository is a mock for OntologyEntityRepository.
type mockEntityRepository struct {
	entities     []*models.OntologyEntity
	getByProjErr error
}

func (m *mockEntityRepository) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockEntityRepository) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepository) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	if m.getByProjErr != nil {
		return nil, m.getByProjErr
	}
	return m.entities, nil
}

func (m *mockEntityRepository) GetPromotedByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	if m.getByProjErr != nil {
		return nil, m.getByProjErr
	}
	var promoted []*models.OntologyEntity
	for _, e := range m.entities {
		if e.IsPromoted {
			promoted = append(promoted, e)
		}
	}
	return promoted, nil
}

func (m *mockEntityRepository) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepository) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepository) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepository) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepository) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

func (m *mockEntityRepository) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (m *mockEntityRepository) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (m *mockEntityRepository) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepository) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}

func (m *mockEntityRepository) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockEntityRepository) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (m *mockEntityRepository) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepository) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}

func (m *mockEntityRepository) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (m *mockEntityRepository) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (m *mockEntityRepository) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockEntityRepository) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
	return nil, nil
}

func (m *mockEntityRepository) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepository) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockEntityRepository) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

func (m *mockEntityRepository) TransferAliasesToEntity(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

func (m *mockEntityRepository) TransferKeyColumnsToEntity(ctx context.Context, fromEntityID, toEntityID uuid.UUID) (int, error) {
	return 0, nil
}

// newTestSchemaServiceWithEntityRepo creates a test schema service with entity repo support.
func newTestSchemaServiceWithEntityRepo(repo *mockSchemaRepository, entityRepo *mockEntityRepository, dsSvc *mockDatasourceService, factory *mockSchemaAdapterFactory) SchemaService {
	return NewSchemaService(repo, entityRepo, nil, nil, dsSvc, factory, zap.NewNop())
}

// ============================================================================
// Tests for GetDatasourceSchemaWithEntities Entity Filtering
// ============================================================================

func TestSchemaService_GetDatasourceSchemaWithEntities_FiltersBySelectedTables(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID1 := uuid.New()
	tableID2 := uuid.New()
	columnID1 := uuid.New()
	columnID2 := uuid.New()

	// Create schema with two tables - one selected, one not
	schemaRepo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID1,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users", // Selected
				IsSelected:   true,
			},
			{
				ID:           tableID2,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "sample_data", // Not selected
				IsSelected:   false,
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID1,
				ProjectID:     projectID,
				SchemaTableID: tableID1,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
				IsSelected:    true,
			},
			{
				ID:            columnID2,
				ProjectID:     projectID,
				SchemaTableID: tableID2,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
				IsSelected:    true,
			},
		},
		relationships: []*models.SchemaRelationship{},
	}

	// Create entities - one for each table
	entityID1 := uuid.New()
	entityID2 := uuid.New()
	entityRepo := &mockEntityRepository{
		entities: []*models.OntologyEntity{
			{
				ID:            entityID1,
				Name:          "User",
				Description:   "A user in the system",
				PrimarySchema: "public",
				PrimaryTable:  "users", // Matches selected table
				PrimaryColumn: "id",
			},
			{
				ID:            entityID2,
				Name:          "SampleEntity",
				Description:   "A sample entity",
				PrimarySchema: "public",
				PrimaryTable:  "sample_data", // Matches deselected table
				PrimaryColumn: "id",
			},
		},
	}

	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaServiceWithEntityRepo(schemaRepo, entityRepo, dsSvc, factory)

	// Test with selectedOnly=true - should filter entities
	result, err := service.GetDatasourceSchemaWithEntities(context.Background(), projectID, datasourceID, true)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaWithEntities failed: %v", err)
	}

	// Should include User entity (from selected table)
	if !contains(result, "User:") {
		t.Error("expected User entity in output (from selected table)")
	}

	// Should NOT include SampleEntity (from deselected table)
	if contains(result, "SampleEntity:") {
		t.Error("SampleEntity should not be in output (from deselected table)")
	}

	// Should include users table
	if !contains(result, "Table: public.users") {
		t.Error("expected users table in output")
	}

	// Should NOT include sample_data table
	if contains(result, "sample_data") {
		t.Error("sample_data table should not be in output")
	}
}

func TestSchemaService_GetDatasourceSchemaWithEntities_AllEntitiesWhenNotFiltered(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID1 := uuid.New()
	tableID2 := uuid.New()
	columnID1 := uuid.New()
	columnID2 := uuid.New()

	// Create schema with two tables
	schemaRepo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID1,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
			},
			{
				ID:           tableID2,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "sample_data",
				IsSelected:   false,
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID1,
				ProjectID:     projectID,
				SchemaTableID: tableID1,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
				IsSelected:    true,
			},
			{
				ID:            columnID2,
				ProjectID:     projectID,
				SchemaTableID: tableID2,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
				IsSelected:    true,
			},
		},
		relationships: []*models.SchemaRelationship{},
	}

	// Create entities for both tables
	entityID1 := uuid.New()
	entityID2 := uuid.New()
	entityRepo := &mockEntityRepository{
		entities: []*models.OntologyEntity{
			{
				ID:            entityID1,
				Name:          "User",
				Description:   "A user in the system",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
			},
			{
				ID:            entityID2,
				Name:          "SampleEntity",
				Description:   "A sample entity",
				PrimarySchema: "public",
				PrimaryTable:  "sample_data",
				PrimaryColumn: "id",
			},
		},
	}

	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaServiceWithEntityRepo(schemaRepo, entityRepo, dsSvc, factory)

	// Test with selectedOnly=false - should include ALL entities
	result, err := service.GetDatasourceSchemaWithEntities(context.Background(), projectID, datasourceID, false)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaWithEntities failed: %v", err)
	}

	// Should include BOTH entities
	if !contains(result, "User:") {
		t.Error("expected User entity in output")
	}
	if !contains(result, "SampleEntity:") {
		t.Error("expected SampleEntity in output when selectedOnly=false")
	}

	// Should include both tables
	if !contains(result, "Table: public.users") {
		t.Error("expected users table in output")
	}
	if !contains(result, "Table: public.sample_data") {
		t.Error("expected sample_data table in output when selectedOnly=false")
	}
}

func TestSchemaService_GetDatasourceSchemaWithEntities_NoEntitiesMatchSelectedTables(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID1 := uuid.New()
	columnID1 := uuid.New()

	// Create schema with one selected table
	schemaRepo := &mockSchemaRepository{
		tables: []*models.SchemaTable{
			{
				ID:           tableID1,
				ProjectID:    projectID,
				DatasourceID: datasourceID,
				SchemaName:   "public",
				TableName:    "users",
				IsSelected:   true,
			},
		},
		columns: []*models.SchemaColumn{
			{
				ID:            columnID1,
				ProjectID:     projectID,
				SchemaTableID: tableID1,
				ColumnName:    "id",
				DataType:      "uuid",
				IsPrimaryKey:  true,
				IsSelected:    true,
			},
		},
		relationships: []*models.SchemaRelationship{},
	}

	// Create entity for a different table (not in selected tables)
	entityID1 := uuid.New()
	entityRepo := &mockEntityRepository{
		entities: []*models.OntologyEntity{
			{
				ID:            entityID1,
				Name:          "Order",
				Description:   "An order",
				PrimarySchema: "public",
				PrimaryTable:  "orders", // This table is not in the schema
				PrimaryColumn: "id",
			},
		},
	}

	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaServiceWithEntityRepo(schemaRepo, entityRepo, dsSvc, factory)

	// Test with selectedOnly=true - should filter out all entities
	result, err := service.GetDatasourceSchemaWithEntities(context.Background(), projectID, datasourceID, true)
	if err != nil {
		t.Fatalf("GetDatasourceSchemaWithEntities failed: %v", err)
	}

	// Should NOT include Order entity (its table is not selected)
	if contains(result, "Order:") {
		t.Error("Order entity should not be in output (its table is not in selected tables)")
	}

	// Should NOT have DOMAIN ENTITIES section if no entities match
	if contains(result, "DOMAIN ENTITIES:") {
		t.Error("DOMAIN ENTITIES section should not appear when no entities match selected tables")
	}

	// Should still include the schema
	if !contains(result, "Table: public.users") {
		t.Error("expected users table in output")
	}
}
