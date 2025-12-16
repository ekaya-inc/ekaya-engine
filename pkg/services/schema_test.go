package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

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

func (m *mockSchemaRepository) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
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

func (m *mockSchemaRepository) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
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

func (m *mockSchemaRepository) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount *int64) error {
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

// mockDatasourceService is a mock for DatasourceService.
type mockDatasourceService struct {
	datasource *models.Datasource
	getErr     error
}

func (m *mockDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType string, config map[string]any) (*models.Datasource, error) {
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

func (m *mockDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.Datasource, error) {
	return nil, errors.New("not implemented")
}

func (m *mockDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType string, config map[string]any) error {
	return errors.New("not implemented")
}

func (m *mockDatasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	return errors.New("not implemented")
}

func (m *mockDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any) error {
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

func (m *mockSchemaDiscoverer) Close() error {
	return nil
}

// mockSchemaAdapterFactory is a mock for DatasourceAdapterFactory.
type mockSchemaAdapterFactory struct {
	discoverer    *mockSchemaDiscoverer
	discovererErr error
}

func (m *mockSchemaAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any) (datasource.ConnectionTester, error) {
	return nil, errors.New("not implemented")
}

func (m *mockSchemaAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any) (datasource.SchemaDiscoverer, error) {
	if m.discovererErr != nil {
		return nil, m.discovererErr
	}
	return m.discoverer, nil
}

func (m *mockSchemaAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func newTestSchemaService(repo *mockSchemaRepository, dsSvc *mockDatasourceService, factory *mockSchemaAdapterFactory) SchemaService {
	return NewSchemaService(repo, dsSvc, factory, zap.NewNop())
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

	result, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID)
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

	result, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID)
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

	_, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID)
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

	_, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID)
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

	_, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID)
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

	result, err := service.RefreshDatasourceSchema(context.Background(), projectID, datasourceID)
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

	tableSelections := map[string]bool{
		"public.users": true,
	}
	columnSelections := map[string][]string{
		"public.users": {"id", "email"},
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

	repo := &mockSchemaRepository{
		tables: []*models.SchemaTable{}, // No tables
	}
	dsSvc := &mockDatasourceService{}
	factory := &mockSchemaAdapterFactory{}

	service := newTestSchemaService(repo, dsSvc, factory)

	tableSelections := map[string]bool{
		"public.nonexistent": true,
	}

	// Should not error, just skip unknown tables
	err := service.SaveSelections(context.Background(), projectID, datasourceID, tableSelections, nil)
	if err != nil {
		t.Fatalf("SaveSelections should not error for unknown tables: %v", err)
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
	if !contains(prompt, "Table: public.users") {
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

	// Should only contain users table
	if !contains(prompt, "public.users") {
		t.Error("expected users table in prompt")
	}
	if contains(prompt, "public.orders") {
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
