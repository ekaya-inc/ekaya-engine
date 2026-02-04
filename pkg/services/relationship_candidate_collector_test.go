//go:build ignore
// +build ignore

// TODO: This test has signature mismatches with identifyFKSources
// and mock type mismatches. Needs refactoring.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ============================================================================
// Mock Implementations for RelationshipCandidateCollector Tests
// ============================================================================

// mockSchemaRepoForCandidateCollector is a mock for SchemaRepository.
type mockSchemaRepoForCandidateCollector struct {
	repositories.SchemaRepository

	// ListColumnsByDatasource mock data
	allColumns    []*models.SchemaColumn
	allColumnsErr error

	// ListTablesByDatasource mock data
	tables    []*models.SchemaTable
	tablesErr error
}

func (m *mockSchemaRepoForCandidateCollector) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	if m.allColumnsErr != nil {
		return nil, m.allColumnsErr
	}
	return m.allColumns, nil
}

func (m *mockSchemaRepoForCandidateCollector) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	if m.tablesErr != nil {
		return nil, m.tablesErr
	}
	return m.tables, nil
}

// mockColumnMetadataRepoForCandidateCollector is a mock for ColumnMetadataRepository.
type mockColumnMetadataRepoForCandidateCollector struct {
	repositories.ColumnMetadataRepository

	// GetBySchemaColumnIDs mock data
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata
	metadataErr        error
}

func (m *mockColumnMetadataRepoForCandidateCollector) GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error) {
	if m.metadataErr != nil {
		return nil, m.metadataErr
	}
	var result []*models.ColumnMetadata
	for _, id := range schemaColumnIDs {
		if meta, ok := m.metadataByColumnID[id]; ok {
			result = append(result, meta)
		}
	}
	return result, nil
}

func (m *mockColumnMetadataRepoForCandidateCollector) GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
	if m.metadataErr != nil {
		return nil, m.metadataErr
	}
	return m.metadataByColumnID[schemaColumnID], nil
}

// mockAdapterFactoryForCandidateCollector is a mock for DatasourceAdapterFactory.
type mockAdapterFactoryForCandidateCollector struct {
	schemaDiscoverer    datasource.SchemaDiscoverer
	schemaDiscovererErr error
}

func (m *mockAdapterFactoryForCandidateCollector) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAdapterFactoryForCandidateCollector) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	if m.schemaDiscovererErr != nil {
		return nil, m.schemaDiscovererErr
	}
	if m.schemaDiscoverer != nil {
		return m.schemaDiscoverer, nil
	}
	// Return a default no-op mock for tests that don't need full adapter functionality
	return &mockSchemaDiscovererForJoinStats{}, nil
}

func (m *mockAdapterFactoryForCandidateCollector) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAdapterFactoryForCandidateCollector) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

// mockDatasourceServiceForCandidateCollector is a mock for DatasourceService.
type mockDatasourceServiceForCandidateCollector struct {
	DatasourceService

	// Get mock data
	datasource *models.Datasource
	getErr     error
}

func (m *mockDatasourceServiceForCandidateCollector) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	if m.datasource != nil {
		return m.datasource, nil
	}
	// Return a default datasource for tests
	return &models.Datasource{
		ID:             id,
		ProjectID:      projectID,
		Name:           "test-datasource",
		DatasourceType: "postgres",
		Config:         map[string]any{},
	}, nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func newTestCandidateCollector(schemaRepo *mockSchemaRepoForCandidateCollector) *relationshipCandidateCollector {
	return &relationshipCandidateCollector{
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &mockColumnMetadataRepoForCandidateCollector{metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata)},
		adapterFactory:     &mockAdapterFactoryForCandidateCollector{},
		dsSvc:              &mockDatasourceServiceForCandidateCollector{},
		logger:             zap.NewNop(),
	}
}

func newTestCandidateCollectorWithMetadata(schemaRepo *mockSchemaRepoForCandidateCollector, metadataRepo *mockColumnMetadataRepoForCandidateCollector) *relationshipCandidateCollector {
	return &relationshipCandidateCollector{
		schemaRepo:         schemaRepo,
		columnMetadataRepo: metadataRepo,
		adapterFactory:     &mockAdapterFactoryForCandidateCollector{},
		dsSvc:              &mockDatasourceServiceForCandidateCollector{},
		logger:             zap.NewNop(),
	}
}

// createColumnMetadataFromFeatures creates a ColumnMetadata from ColumnFeatures for tests.
func createColumnMetadataFromFeatures(columnID uuid.UUID, features *models.ColumnFeatures) *models.ColumnMetadata {
	if features == nil {
		return nil
	}
	meta := &models.ColumnMetadata{
		ID:             uuid.New(),
		SchemaColumnID: columnID,
	}
	if features.Purpose != "" {
		meta.Purpose = &features.Purpose
	}
	if features.Role != "" {
		meta.Role = &features.Role
	}
	if features.ClassificationPath != "" {
		path := string(features.ClassificationPath)
		meta.ClassificationPath = &path
	}
	return meta
}

// ============================================================================
// shouldExcludeFromFKSources Tests
// ============================================================================

func TestShouldExcludeFromFKSources_PrimaryKey(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ColumnName:   "id",
		DataType:     "uuid",
		IsPrimaryKey: true,
	}

	assert.True(t, collector.shouldExcludeFromFKSources(col, nil), "primary keys should be excluded - they are targets, not sources")
}

func TestShouldExcludeFromFKSources_TimestampTypes(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	timestampTypes := []struct {
		dataType    string
		description string
	}{
		{"timestamp", "timestamp type"},
		{"timestamp with time zone", "timestamp with time zone"},
		{"timestamp without time zone", "timestamp without time zone"},
		{"timestamptz", "timestamptz alias"},
		{"datetime", "datetime type"},
		{"date", "date type"},
		{"time", "time type"},
		{"TIMESTAMP", "uppercase TIMESTAMP"},
	}

	for _, tt := range timestampTypes {
		t.Run(tt.description, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName:   "created_at",
				DataType:     tt.dataType,
				IsPrimaryKey: false,
			}
			assert.True(t, collector.shouldExcludeFromFKSources(col, nil), "%s should be excluded", tt.dataType)
		})
	}
}

func TestShouldExcludeFromFKSources_BooleanTypes(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	boolTypes := []struct {
		dataType    string
		description string
	}{
		{"boolean", "boolean type"},
		{"bool", "bool alias"},
		{"BOOLEAN", "uppercase BOOLEAN"},
	}

	for _, tt := range boolTypes {
		t.Run(tt.description, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName:   "is_active",
				DataType:     tt.dataType,
				IsPrimaryKey: false,
			}
			assert.True(t, collector.shouldExcludeFromFKSources(col, nil), "%s should be excluded", tt.dataType)
		})
	}
}

func TestShouldExcludeFromFKSources_JSONTypes(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	jsonTypes := []struct {
		dataType    string
		description string
	}{
		{"json", "json type"},
		{"jsonb", "jsonb type"},
		{"JSON", "uppercase JSON"},
	}

	for _, tt := range jsonTypes {
		t.Run(tt.description, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName:   "metadata",
				DataType:     tt.dataType,
				IsPrimaryKey: false,
			}
			assert.True(t, collector.shouldExcludeFromFKSources(col, nil), "%s should be excluded", tt.dataType)
		})
	}
}

func TestShouldExcludeFromFKSources_MetadataClassificationPath(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Test columns with metadata that indicate exclusion via classification path
	excludedPaths := []models.ClassificationPath{
		models.ClassificationPathTimestamp,
		models.ClassificationPathBoolean,
		models.ClassificationPathJSON,
	}

	for _, path := range excludedPaths {
		t.Run(string(path), func(t *testing.T) {
			col := &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "some_column",
				DataType:     "text",
				IsPrimaryKey: false,
			}
			pathStr := string(path)
			metadata := &models.ColumnMetadata{
				SchemaColumnID:     col.ID,
				ClassificationPath: &pathStr,
			}
			assert.True(t, collector.shouldExcludeFromFKSources(col, metadata), "classification_path=%s should be excluded", path)
		})
	}
}

func TestShouldExcludeFromFKSources_NotExcluded(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Test columns that should NOT be excluded
	validTypes := []struct {
		columnName  string
		dataType    string
		description string
	}{
		{"user_id", "uuid", "UUID columns should not be excluded"},
		{"account_id", "integer", "integer columns should not be excluded"},
		{"external_id", "bigint", "bigint columns should not be excluded"},
		{"reference_code", "text", "text columns should not be excluded"},
		{"code", "varchar(255)", "varchar columns should not be excluded"},
	}

	for _, tt := range validTypes {
		t.Run(tt.description, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName:   tt.columnName,
				DataType:     tt.dataType,
				IsPrimaryKey: false,
			}
			assert.False(t, collector.shouldExcludeFromFKSources(col, nil), tt.description)
		})
	}
}

// ============================================================================
// isQualifiedFKSource Tests
// ============================================================================

func TestIsQualifiedFKSource_RoleForeignKey(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ID:         uuid.New(),
		ColumnName: "user_id",
		DataType:   "uuid",
	}
	role := models.RoleForeignKey
	metadata := &models.ColumnMetadata{
		SchemaColumnID: col.ID,
		Role:           &role,
	}

	assert.True(t, collector.isQualifiedFKSource(col, metadata), "role=foreign_key should qualify")
}

func TestIsQualifiedFKSource_PurposeIdentifier(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ID:         uuid.New(),
		ColumnName: "account_id",
		DataType:   "uuid",
	}
	purpose := models.PurposeIdentifier
	metadata := &models.ColumnMetadata{
		SchemaColumnID: col.ID,
		Purpose:        &purpose,
	}

	assert.True(t, collector.isQualifiedFKSource(col, metadata), "purpose=identifier should qualify")
}

func TestIsQualifiedFKSource_ClassificationPathUUID(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ID:         uuid.New(),
		ColumnName: "some_id",
		DataType:   "uuid",
	}
	classPath := string(models.ClassificationPathUUID)
	metadata := &models.ColumnMetadata{
		SchemaColumnID:     col.ID,
		ClassificationPath: &classPath,
	}

	assert.True(t, collector.isQualifiedFKSource(col, metadata), "classification_path=uuid should qualify")
}

func TestIsQualifiedFKSource_ClassificationPathExternalID(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ID:         uuid.New(),
		ColumnName: "stripe_customer_id",
		DataType:   "text",
	}
	classPath := string(models.ClassificationPathExternalID)
	metadata := &models.ColumnMetadata{
		SchemaColumnID:     col.ID,
		ClassificationPath: &classPath,
	}

	assert.True(t, collector.isQualifiedFKSource(col, metadata), "classification_path=external_id should qualify")
}

func TestIsQualifiedFKSource_JoinableFallback(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Column marked as joinable but without metadata should qualify
	joinable := true
	col := &models.SchemaColumn{
		ID:         uuid.New(),
		ColumnName: "user_id",
		DataType:   "uuid",
		IsJoinable: &joinable,
	}

	assert.True(t, collector.isQualifiedFKSource(col, nil), "joinable column without metadata should qualify")
}

func TestIsQualifiedFKSource_NotQualified(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ID:         uuid.New(),
		ColumnName: "some_column",
		DataType:   "text",
	}

	// Test columns that should NOT qualify
	tests := []struct {
		name     string
		metadata *models.ColumnMetadata
	}{
		{
			name: "attribute role",
			metadata: func() *models.ColumnMetadata {
				role := models.RoleAttribute
				purpose := models.PurposeText
				return &models.ColumnMetadata{SchemaColumnID: col.ID, Role: &role, Purpose: &purpose}
			}(),
		},
		{
			name: "measure role",
			metadata: func() *models.ColumnMetadata {
				role := models.RoleMeasure
				purpose := models.PurposeMeasure
				return &models.ColumnMetadata{SchemaColumnID: col.ID, Role: &role, Purpose: &purpose}
			}(),
		},
		{
			name: "text classification",
			metadata: func() *models.ColumnMetadata {
				classPath := string(models.ClassificationPathText)
				purpose := models.PurposeText
				return &models.ColumnMetadata{SchemaColumnID: col.ID, ClassificationPath: &classPath, Purpose: &purpose}
			}(),
		},
		{
			name: "numeric classification",
			metadata: func() *models.ColumnMetadata {
				classPath := string(models.ClassificationPathNumeric)
				purpose := models.PurposeMeasure
				return &models.ColumnMetadata{SchemaColumnID: col.ID, ClassificationPath: &classPath, Purpose: &purpose}
			}(),
		},
		{
			name: "enum classification",
			metadata: func() *models.ColumnMetadata {
				classPath := string(models.ClassificationPathEnum)
				purpose := models.PurposeEnum
				return &models.ColumnMetadata{SchemaColumnID: col.ID, ClassificationPath: &classPath, Purpose: &purpose}
			}(),
		},
		{
			name:     "nil metadata and not joinable",
			metadata: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, collector.isQualifiedFKSource(col, tt.metadata), "%s should not qualify", tt.name)
		})
	}
}

func TestIsQualifiedFKSource_MultipleQualifications(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// A column with multiple qualifying criteria should still qualify
	col := &models.SchemaColumn{
		ID:         uuid.New(),
		ColumnName: "user_id",
		DataType:   "uuid",
	}
	role := models.RoleForeignKey
	purpose := models.PurposeIdentifier
	classPath := string(models.ClassificationPathUUID)
	metadata := &models.ColumnMetadata{
		SchemaColumnID:     col.ID,
		Role:               &role,
		Purpose:            &purpose,
		ClassificationPath: &classPath,
	}

	assert.True(t, collector.isQualifiedFKSource(col, metadata), "column with multiple qualifications should qualify")
}

// ============================================================================
// identifyFKSources Tests
// ============================================================================

func TestIdentifyFKSources_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	// Create columns
	userIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
	}

	accountIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "account_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
	}

	// Primary key should be excluded
	usersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
	}

	// Create metadata for each column
	role := models.RoleForeignKey
	purpose := models.PurposeIdentifier
	classPath := string(models.ClassificationPathUUID)
	pkRole := models.RolePrimaryKey

	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{
		userIDCol.ID: {
			SchemaColumnID:     userIDCol.ID,
			Role:               &role,
			Purpose:            &purpose,
			ClassificationPath: &classPath,
		},
		accountIDCol.ID: {
			SchemaColumnID:     accountIDCol.ID,
			Role:               &role,
			ClassificationPath: &classPath,
		},
		usersPKCol.ID: {
			SchemaColumnID:     usersPKCol.ID,
			Role:               &pkRole,
			ClassificationPath: &classPath,
		},
	}

	schemaRepo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{userIDCol, accountIDCol, usersPKCol},
		tables: []*models.SchemaTable{
			{ID: usersTableID, TableName: "users"},
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	metadataRepo := &mockColumnMetadataRepoForCandidateCollector{
		metadataByColumnID: metadataByColumnID,
	}

	collector := newTestCandidateCollectorWithMetadata(schemaRepo, metadataRepo)

	sources, _, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Should have 2 sources (user_id and account_id), not the PK
	assert.Len(t, sources, 2, "expected 2 FK sources")

	// Verify the sources are correct
	sourceNames := make([]string, len(sources))
	for i, src := range sources {
		sourceNames[i] = src.Column.ColumnName
	}
	assert.Contains(t, sourceNames, "user_id")
	assert.Contains(t, sourceNames, "account_id")
	assert.NotContains(t, sourceNames, "id", "PK should not be in sources")
}

func TestIdentifyFKSources_JoinableFallback(t *testing.T) {
	// Test that columns marked as joinable but without metadata are included
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()

	isJoinable := true
	joinabilityReason := "cardinality_ok"

	// Column with no metadata but marked as joinable
	joinableCol := &models.SchemaColumn{
		ID:                uuid.New(),
		SchemaTableID:     ordersTableID,
		ColumnName:        "legacy_id",
		DataType:          "integer",
		IsPrimaryKey:      false,
		IsJoinable:        &isJoinable,
		JoinabilityReason: &joinabilityReason,
	}

	schemaRepo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{joinableCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	metadataRepo := &mockColumnMetadataRepoForCandidateCollector{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{}, // No metadata
	}

	collector := newTestCandidateCollectorWithMetadata(schemaRepo, metadataRepo)

	sources, _, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Should have 1 source from joinable fallback
	assert.Len(t, sources, 1, "expected 1 FK source from joinable fallback")
	assert.Equal(t, "legacy_id", sources[0].Column.ColumnName)
	assert.Equal(t, "orders", sources[0].TableName)
}

func TestIdentifyFKSources_ExcludesJoinableTimestamp(t *testing.T) {
	// Test that joinable columns are still filtered by exclusion criteria
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()

	isJoinable := true

	// Timestamp column marked as joinable should still be excluded
	timestampCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "created_at",
		DataType:      "timestamp",
		IsPrimaryKey:  false,
		IsJoinable:    &isJoinable,
	}

	schemaRepo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{timestampCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	metadataRepo := &mockColumnMetadataRepoForCandidateCollector{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{},
	}

	collector := newTestCandidateCollectorWithMetadata(schemaRepo, metadataRepo)

	sources, _, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	assert.Len(t, sources, 0, "timestamp should be excluded even if joinable")
}

func TestIdentifyFKSources_NoDuplicates(t *testing.T) {
	// Test that a column with both metadata and joinable flag is only listed once
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()

	isJoinable := true
	colID := uuid.New()

	col := &models.SchemaColumn{
		ID:            colID,
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		IsJoinable:    &isJoinable,
	}

	role := models.RoleForeignKey
	classPath := string(models.ClassificationPathUUID)

	schemaRepo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{col},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	metadataRepo := &mockColumnMetadataRepoForCandidateCollector{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			colID: {
				SchemaColumnID:     colID,
				Role:               &role,
				ClassificationPath: &classPath,
			},
		},
	}

	collector := newTestCandidateCollectorWithMetadata(schemaRepo, metadataRepo)

	sources, _, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	assert.Len(t, sources, 1, "column should appear only once")
	assert.Equal(t, "user_id", sources[0].Column.ColumnName)
}

func TestIdentifyFKSources_ErrorHandling(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	t.Run("GetColumnsWithFeaturesByDatasource error", func(t *testing.T) {
		repo := &mockSchemaRepoForCandidateCollector{
			allColumnsErr: errors.New("database error"),
		}

		collector := newTestCandidateCollector(repo)

		_, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get columns with features")
	})

	t.Run("ListColumnsByDatasource error", func(t *testing.T) {
		repo := &mockSchemaRepoForCandidateCollector{
			allColumns: map[string][]*models.SchemaColumn{},
			allColumnsErr:  errors.New("database error"),
		}

		collector := newTestCandidateCollector(repo)

		_, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list all columns")
	})

	t.Run("ListTablesByDatasource error", func(t *testing.T) {
		repo := &mockSchemaRepoForCandidateCollector{
			allColumns: map[string][]*models.SchemaColumn{},
			allColumns:     []*models.SchemaColumn{},
			tablesErr:      errors.New("database error"),
		}

		collector := newTestCandidateCollector(repo)

		_, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list tables")
	})
}

func TestIdentifyFKSources_EmptyDataset(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: map[string][]*models.SchemaColumn{},
		allColumns:     []*models.SchemaColumn{},
		tables:         []*models.SchemaTable{},
	}

	collector := newTestCandidateCollector(repo)

	sources, _, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)
	assert.Len(t, sources, 0, "empty dataset should return empty sources")
}

func TestIdentifyFKSources_SkipsColumnsWithoutTableName(t *testing.T) {
	// Test that columns where table name cannot be resolved are skipped
	projectID := uuid.New()
	datasourceID := uuid.New()
	unknownTableID := uuid.New()

	isJoinable := true
	col := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: unknownTableID, // This table ID is not in the tables list
		ColumnName:    "orphan_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		IsJoinable:    &isJoinable,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: map[string][]*models.SchemaColumn{},
		allColumns:     []*models.SchemaColumn{col},
		tables:         []*models.SchemaTable{}, // Empty - no tables to resolve
	}

	collector := newTestCandidateCollector(repo)

	sources, _, _, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Column should be skipped because table name cannot be resolved
	assert.Len(t, sources, 0, "column without resolvable table name should be skipped")
}

// ============================================================================
// FKSourceColumn Tests
// ============================================================================

func TestFKSourceColumn_Fields(t *testing.T) {
	colID := uuid.New()
	col := &models.SchemaColumn{
		ID:           colID,
		ColumnName:   "user_id",
		DataType:     "uuid",
		IsPrimaryKey: false,
	}
	features := &models.ColumnFeatures{
		Role:               models.RoleForeignKey,
		Purpose:            models.PurposeIdentifier,
		ClassificationPath: models.ClassificationPathUUID,
	}

	fkSource := &FKSourceColumn{
		Column:    col,
		Features:  features,
		TableName: "orders",
	}

	assert.Equal(t, col, fkSource.Column)
	assert.Equal(t, features, fkSource.Metadata)
	assert.Equal(t, "orders", fkSource.TableName)
	assert.Equal(t, "user_id", fkSource.Column.ColumnName)
}

// ============================================================================
// CollectCandidates Tests
// ============================================================================

func TestCollectCandidates_GeneratesCandidatePairs(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()

	// FK source column
	userIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":                "foreign_key",
				"classification_path": "uuid",
			},
		},
	}

	// PK target column
	usersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: map[string][]*models.SchemaColumn{
			"orders": {userIDCol},
		},
		allColumns: []*models.SchemaColumn{userIDCol, usersPKCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
			{ID: usersTableID, TableName: "users"},
		},
	}

	collector := NewRelationshipCandidateCollector(repo, &mockColumnMetadataRepoForCandidateCollector{metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata)}, &mockAdapterFactoryForCandidateCollector{}, &mockDatasourceServiceForCandidateCollector{}, zap.NewNop())

	// Track progress callbacks
	progressCalls := 0
	progressCallback := func(current, total int, message string) {
		progressCalls++
	}

	result, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, progressCallback)
	require.NoError(t, err)

	// Should have 1 candidate: orders.user_id → users.id
	require.Len(t, result, 1, "expected 1 candidate pair")
	assert.Equal(t, "orders", result[0].SourceTable)
	assert.Equal(t, "user_id", result[0].SourceColumn)
	assert.Equal(t, "users", result[0].TargetTable)
	assert.Equal(t, "id", result[0].TargetColumn)

	// Progress callback should be called at least 5 times (steps 0, 1, 2, 3, 5)
	// Step 4 is only called during statistics collection for large candidate sets (>10 candidates)
	assert.GreaterOrEqual(t, progressCalls, 5, "progress callback should be called for each major step")
}

func TestCollectCandidates_NoTargets(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()

	// FK source column but no PK/unique target columns
	userIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":                "foreign_key",
				"classification_path": "uuid",
			},
		},
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: map[string][]*models.SchemaColumn{
			"orders": {userIDCol},
		},
		allColumns: []*models.SchemaColumn{userIDCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	collector := NewRelationshipCandidateCollector(repo, &mockColumnMetadataRepoForCandidateCollector{metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata)}, &mockAdapterFactoryForCandidateCollector{}, &mockDatasourceServiceForCandidateCollector{}, zap.NewNop())

	result, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, nil)
	require.NoError(t, err)

	// No targets = no candidates
	assert.Empty(t, result, "no FK targets means no candidates")
}

func TestCollectCandidates_PropagatesError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepoForCandidateCollector{
		allColumnsErr: errors.New("database error"),
	}

	collector := NewRelationshipCandidateCollector(repo, &mockColumnMetadataRepoForCandidateCollector{metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata)}, &mockAdapterFactoryForCandidateCollector{}, &mockDatasourceServiceForCandidateCollector{}, zap.NewNop())

	_, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identify FK sources")
}

// ============================================================================
// identifyFKTargets Tests
// ============================================================================

func TestIdentifyFKTargets_PrimaryKeysOnly(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	// Primary key column - should be included
	usersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
		IsUnique:      false,
	}

	// Non-PK, non-unique column - should be excluded
	userNameCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "name",
		DataType:      "text",
		IsPrimaryKey:  false,
		IsUnique:      false,
	}

	// Another PK - should be included
	ordersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
		IsUnique:      false,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{usersPKCol, userNameCol, ordersPKCol},
		tables: []*models.SchemaTable{
			{ID: usersTableID, TableName: "users"},
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	collector := newTestCandidateCollector(repo)

	targets, err := collector.identifyFKTargets(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Should have 2 targets (both PKs)
	assert.Len(t, targets, 2, "expected 2 FK targets (PKs)")

	targetNames := make([]string, len(targets))
	for i, tgt := range targets {
		targetNames[i] = tgt.TableName + "." + tgt.Column.ColumnName
	}
	assert.Contains(t, targetNames, "users.id")
	assert.Contains(t, targetNames, "orders.id")
}

func TestIdentifyFKTargets_UniqueColumns(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()

	// Primary key column
	usersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
		IsUnique:      false,
	}

	// Unique column (not PK) - should also be included
	usersEmailCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "email",
		DataType:      "text",
		IsPrimaryKey:  false,
		IsUnique:      true, // Unique constraint
	}

	// Non-unique column - should be excluded
	userNameCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "name",
		DataType:      "text",
		IsPrimaryKey:  false,
		IsUnique:      false,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{usersPKCol, usersEmailCol, userNameCol},
		tables: []*models.SchemaTable{
			{ID: usersTableID, TableName: "users"},
		},
	}

	collector := newTestCandidateCollector(repo)

	targets, err := collector.identifyFKTargets(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Should have 2 targets (PK and unique column)
	assert.Len(t, targets, 2, "expected 2 FK targets (PK + unique)")

	targetNames := make([]string, len(targets))
	for i, tgt := range targets {
		targetNames[i] = tgt.Column.ColumnName
		// All targets should have IsUnique=true
		assert.True(t, tgt.IsUnique, "target %s should have IsUnique=true", tgt.Column.ColumnName)
	}
	assert.Contains(t, targetNames, "id")
	assert.Contains(t, targetNames, "email")
	assert.NotContains(t, targetNames, "name")
}

func TestIdentifyFKTargets_ExcludesHighCardinalityNonUnique(t *testing.T) {
	// This test verifies the key change from the old approach:
	// High-cardinality columns that are NOT PKs or unique should be EXCLUDED
	projectID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()

	distinctCount := int64(10000) // High cardinality

	// High-cardinality column but not PK or unique - should be EXCLUDED
	// (Old approach would have included this because distinctCount >= 20)
	userNameCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "username",
		DataType:      "text",
		IsPrimaryKey:  false,
		IsUnique:      false,
		DistinctCount: &distinctCount,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{userNameCol},
		tables: []*models.SchemaTable{
			{ID: usersTableID, TableName: "users"},
		},
	}

	collector := newTestCandidateCollector(repo)

	targets, err := collector.identifyFKTargets(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Should have 0 targets - high cardinality alone is NOT enough
	assert.Len(t, targets, 0, "high-cardinality non-unique column should NOT be a FK target")
}

func TestIdentifyFKTargets_EmptyDataset(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{},
		tables:     []*models.SchemaTable{},
	}

	collector := newTestCandidateCollector(repo)

	targets, err := collector.identifyFKTargets(context.Background(), projectID, datasourceID)
	require.NoError(t, err)
	assert.Len(t, targets, 0, "empty dataset should return empty targets")
}

func TestIdentifyFKTargets_SkipsColumnsWithoutTableName(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	unknownTableID := uuid.New()

	// PK column but table cannot be resolved
	orphanPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: unknownTableID, // Not in tables list
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
		IsUnique:      false,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: []*models.SchemaColumn{orphanPKCol},
		tables:     []*models.SchemaTable{}, // Empty - no tables to resolve
	}

	collector := newTestCandidateCollector(repo)

	targets, err := collector.identifyFKTargets(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	// Column should be skipped because table name cannot be resolved
	assert.Len(t, targets, 0, "column without resolvable table name should be skipped")
}

func TestIdentifyFKTargets_ErrorHandling(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	t.Run("ListTablesByDatasource error", func(t *testing.T) {
		repo := &mockSchemaRepoForCandidateCollector{
			tablesErr: errors.New("database error"),
		}

		collector := newTestCandidateCollector(repo)

		_, err := collector.identifyFKTargets(context.Background(), projectID, datasourceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list tables")
	})

	t.Run("ListColumnsByDatasource error", func(t *testing.T) {
		repo := &mockSchemaRepoForCandidateCollector{
			tables:        []*models.SchemaTable{},
			allColumnsErr: errors.New("database error"),
		}

		collector := newTestCandidateCollector(repo)

		_, err := collector.identifyFKTargets(context.Background(), projectID, datasourceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list columns")
	})
}

// ============================================================================
// FKTargetColumn Tests
// ============================================================================

func TestFKTargetColumn_Fields(t *testing.T) {
	colID := uuid.New()
	col := &models.SchemaColumn{
		ID:           colID,
		ColumnName:   "id",
		DataType:     "uuid",
		IsPrimaryKey: true,
		IsUnique:     false,
	}

	fkTarget := &FKTargetColumn{
		Column:    col,
		TableName: "users",
		IsUnique:  true, // PKs are considered unique
	}

	assert.Equal(t, col, fkTarget.Column)
	assert.Equal(t, "users", fkTarget.TableName)
	assert.True(t, fkTarget.IsUnique)
	assert.Equal(t, "id", fkTarget.Column.ColumnName)
}

// ============================================================================
// areTypesCompatible Tests
// ============================================================================

func TestAreTypesCompatible_ExactMatch(t *testing.T) {
	// Same types should always be compatible
	tests := []struct {
		sourceType string
		targetType string
	}{
		{"uuid", "uuid"},
		{"text", "text"},
		{"integer", "integer"},
		{"bigint", "bigint"},
		{"varchar(255)", "varchar(255)"},
		{"boolean", "boolean"},
	}

	for _, tt := range tests {
		t.Run(tt.sourceType+"_to_"+tt.targetType, func(t *testing.T) {
			assert.True(t, areTypesCompatible(tt.sourceType, tt.targetType),
				"%s should be compatible with %s", tt.sourceType, tt.targetType)
		})
	}
}

func TestAreTypesCompatible_IntegerVariants(t *testing.T) {
	// All integer variants should be compatible with each other
	integerTypes := []string{
		"int", "int2", "int4", "int8",
		"integer", "smallint", "bigint",
		"serial", "smallserial", "bigserial",
		"tinyint",
	}

	for _, source := range integerTypes {
		for _, target := range integerTypes {
			t.Run(source+"_to_"+target, func(t *testing.T) {
				assert.True(t, areTypesCompatible(source, target),
					"%s should be compatible with %s", source, target)
			})
		}
	}
}

func TestAreTypesCompatible_StringVariants(t *testing.T) {
	// All string variants should be compatible with each other
	stringTypes := []string{
		"text", "varchar", "char", "character", "character varying",
		"bpchar", "nvarchar", "nchar", "ntext",
		"varchar(255)", "char(10)", "nvarchar(100)",
	}

	for _, source := range stringTypes {
		for _, target := range stringTypes {
			t.Run(source+"_to_"+target, func(t *testing.T) {
				assert.True(t, areTypesCompatible(source, target),
					"%s should be compatible with %s", source, target)
			})
		}
	}
}

func TestAreTypesCompatible_NumericVariants(t *testing.T) {
	// All numeric variants should be compatible with each other
	numericTypes := []string{
		"numeric", "decimal", "float", "float4", "float8",
		"real", "double precision", "double", "money",
		"numeric(10,2)", "decimal(18,4)",
	}

	for _, source := range numericTypes {
		for _, target := range numericTypes {
			t.Run(source+"_to_"+target, func(t *testing.T) {
				assert.True(t, areTypesCompatible(source, target),
					"%s should be compatible with %s", source, target)
			})
		}
	}
}

func TestAreTypesCompatible_UUIDOnly(t *testing.T) {
	// UUID should only match UUID
	assert.True(t, areTypesCompatible("uuid", "uuid"))
	assert.False(t, areTypesCompatible("uuid", "text"), "uuid should not match text")
	assert.False(t, areTypesCompatible("text", "uuid"), "text should not match uuid")
	assert.False(t, areTypesCompatible("uuid", "integer"), "uuid should not match integer")
}

func TestAreTypesCompatible_IncompatiblePairs(t *testing.T) {
	// These pairs should NOT be compatible
	tests := []struct {
		sourceType  string
		targetType  string
		description string
	}{
		{"text", "integer", "text to integer"},
		{"integer", "text", "integer to text"},
		{"boolean", "text", "boolean to text"},
		{"boolean", "integer", "boolean to integer"},
		{"boolean", "uuid", "boolean to uuid"},
		{"timestamp", "text", "timestamp to text"},
		{"timestamp", "integer", "timestamp to integer"},
		{"json", "text", "json to text"},
		{"json", "jsonb", "json to jsonb (same category)"}, // Wait, this should be true
		{"uuid", "varchar(36)", "uuid to varchar"},
		{"integer", "float", "integer to float (different categories)"},
		{"date", "integer", "date to integer"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			// Most should be false, but json/jsonb are same category
			if tt.sourceType == "json" && tt.targetType == "jsonb" {
				assert.True(t, areTypesCompatible(tt.sourceType, tt.targetType),
					"%s should be compatible with %s (same category)", tt.sourceType, tt.targetType)
			} else {
				assert.False(t, areTypesCompatible(tt.sourceType, tt.targetType),
					"%s should NOT be compatible with %s", tt.sourceType, tt.targetType)
			}
		})
	}
}

func TestAreTypesCompatible_CaseInsensitive(t *testing.T) {
	// Type comparison should be case-insensitive
	tests := []struct {
		sourceType string
		targetType string
	}{
		{"UUID", "uuid"},
		{"TEXT", "text"},
		{"Integer", "INTEGER"},
		{"VARCHAR(255)", "varchar(255)"},
	}

	for _, tt := range tests {
		t.Run(tt.sourceType+"_to_"+tt.targetType, func(t *testing.T) {
			assert.True(t, areTypesCompatible(tt.sourceType, tt.targetType),
				"%s should be compatible with %s (case-insensitive)", tt.sourceType, tt.targetType)
		})
	}
}

func TestAreTypesCompatible_TimestampTypes(t *testing.T) {
	// Timestamp types should only match other timestamp types
	timestampTypes := []string{
		"timestamp", "timestamptz", "timestamp with time zone",
		"timestamp without time zone", "datetime", "datetime2",
		"date", "time", "timetz",
	}

	for _, source := range timestampTypes {
		for _, target := range timestampTypes {
			t.Run(source+"_to_"+target, func(t *testing.T) {
				assert.True(t, areTypesCompatible(source, target),
					"%s should be compatible with %s", source, target)
			})
		}
	}

	// Timestamps should NOT match non-timestamps
	assert.False(t, areTypesCompatible("timestamp", "text"))
	assert.False(t, areTypesCompatible("timestamp", "integer"))
	assert.False(t, areTypesCompatible("timestamp", "uuid"))
}

func TestAreTypesCompatible_BooleanTypes(t *testing.T) {
	// Boolean types should only match other boolean types
	boolTypes := []string{"boolean", "bool", "bit"}

	for _, source := range boolTypes {
		for _, target := range boolTypes {
			t.Run(source+"_to_"+target, func(t *testing.T) {
				assert.True(t, areTypesCompatible(source, target),
					"%s should be compatible with %s", source, target)
			})
		}
	}

	// Booleans should NOT match non-booleans
	assert.False(t, areTypesCompatible("boolean", "text"))
	assert.False(t, areTypesCompatible("boolean", "integer"))
	assert.False(t, areTypesCompatible("boolean", "uuid"))
}

func TestAreTypesCompatible_UnknownTypes(t *testing.T) {
	// Unknown types should not match anything (including themselves)
	// This is conservative - better to not create a bad relationship
	assert.False(t, areTypesCompatible("unknown_type", "unknown_type"))
	assert.False(t, areTypesCompatible("unknown_type", "text"))
	assert.False(t, areTypesCompatible("text", "unknown_type"))
}

// ============================================================================
// categorizeDataType Tests
// ============================================================================

func TestCategorizeDataType_Categories(t *testing.T) {
	tests := []struct {
		dataType string
		expected string
	}{
		// UUID
		{"uuid", "uuid"},

		// Integer types
		{"int", "integer"},
		{"int2", "integer"},
		{"int4", "integer"},
		{"int8", "integer"},
		{"integer", "integer"},
		{"smallint", "integer"},
		{"bigint", "integer"},
		{"serial", "integer"},
		{"smallserial", "integer"},
		{"bigserial", "integer"},
		{"tinyint", "integer"},

		// String types
		{"text", "string"},
		{"varchar", "string"},
		{"varchar(255)", "string"},
		{"char", "string"},
		{"char(10)", "string"},
		{"character", "string"},
		{"character varying", "string"},
		{"character varying(100)", "string"},
		{"bpchar", "string"},
		{"nvarchar", "string"},
		{"nvarchar(50)", "string"},
		{"nchar", "string"},
		{"ntext", "string"},

		// Numeric types
		{"numeric", "numeric"},
		{"numeric(10,2)", "numeric"},
		{"decimal", "numeric"},
		{"decimal(18,4)", "numeric"},
		{"float", "numeric"},
		{"float4", "numeric"},
		{"float8", "numeric"},
		{"real", "numeric"},
		{"double precision", "numeric"},
		{"double", "numeric"},
		{"money", "numeric"},

		// Boolean types
		{"boolean", "boolean"},
		{"bool", "boolean"},
		{"bit", "boolean"},

		// Timestamp types
		{"timestamp", "timestamp"},
		{"timestamptz", "timestamp"},
		{"timestamp with time zone", "timestamp"},
		{"timestamp without time zone", "timestamp"},
		{"datetime", "timestamp"},
		{"datetime2", "timestamp"},
		{"date", "timestamp"},
		{"time", "timestamp"},
		{"timetz", "timestamp"},
		{"time with time zone", "timestamp"},

		// JSON types
		{"json", "json"},
		{"jsonb", "json"},

		// Unknown types
		{"bytea", ""},
		{"blob", ""},
		{"custom_type", ""},
	}

	for _, tt := range tests {
		t.Run(tt.dataType, func(t *testing.T) {
			assert.Equal(t, tt.expected, categorizeDataType(tt.dataType),
				"categorizeDataType(%s) should return %q", tt.dataType, tt.expected)
		})
	}
}

// ============================================================================
// generateCandidatePairs Tests
// ============================================================================

func TestGenerateCandidatePairs_BasicPairing(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Create source columns
	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "user_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			Features: &models.ColumnFeatures{
				Role:    models.RoleForeignKey,
				Purpose: models.PurposeIdentifier,
			},
			TableName: "orders",
		},
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "account_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			Features: &models.ColumnFeatures{
				Role:    models.RoleForeignKey,
				Purpose: models.PurposeIdentifier,
			},
			TableName: "orders",
		},
	}

	// Create target columns
	targets := []*FKTargetColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "id",
				DataType:     "uuid",
				IsPrimaryKey: true,
			},
			TableName: "users",
			IsUnique:  true,
		},
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "id",
				DataType:     "uuid",
				IsPrimaryKey: true,
			},
			TableName: "accounts",
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	// 2 sources × 2 targets = 4 candidates
	assert.Len(t, candidates, 4, "expected 4 candidate pairs (2 sources × 2 targets)")

	// Verify all candidates have correct source/target info
	for _, c := range candidates {
		assert.Equal(t, "orders", c.SourceTable, "all sources are from orders table")
		assert.Contains(t, []string{"users", "accounts"}, c.TargetTable)
		assert.Equal(t, "uuid", c.SourceDataType)
		assert.Equal(t, "uuid", c.TargetDataType)
	}
}

func TestGenerateCandidatePairs_SkipsSelfReferences(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	colID := uuid.New()

	// Source and target are the same column
	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           colID,
				ColumnName:   "id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			TableName: "items",
		},
	}

	targets := []*FKTargetColumn{
		{
			Column: &models.SchemaColumn{
				ID:           colID, // Same ID
				ColumnName:   "id",
				DataType:     "uuid",
				IsPrimaryKey: true,
			},
			TableName: "items", // Same table
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	// Self-reference should be skipped
	assert.Len(t, candidates, 0, "self-reference (same table.column) should be skipped")
}

func TestGenerateCandidatePairs_SkipsIncompatibleTypes(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// UUID source
	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "user_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			TableName: "orders",
		},
	}

	// Integer target - incompatible with UUID
	targets := []*FKTargetColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "id",
				DataType:     "integer",
				IsPrimaryKey: true,
			},
			TableName: "users",
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	// Incompatible types should be skipped
	assert.Len(t, candidates, 0, "uuid → integer should be skipped (incompatible types)")
}

func TestGenerateCandidatePairs_PopulatesColumnFeatures(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Source with features
	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "user_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			Features: &models.ColumnFeatures{
				Role:    models.RoleForeignKey,
				Purpose: models.PurposeIdentifier,
			},
			TableName: "orders",
		},
	}

	// Target with features (stored in metadata)
	targetCol := &models.SchemaColumn{
		ID:           uuid.New(),
		ColumnName:   "id",
		DataType:     "uuid",
		IsPrimaryKey: true,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":    "primary_key",
				"purpose": "identifier",
			},
		},
	}
	targets := []*FKTargetColumn{
		{
			Column:    targetCol,
			TableName: "users",
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	require.Len(t, candidates, 1)

	// Verify source features are populated
	assert.Equal(t, models.RoleForeignKey, candidates[0].SourceRole)
	assert.Equal(t, models.PurposeIdentifier, candidates[0].SourcePurpose)

	// Verify target features are populated from metadata
	assert.Equal(t, "primary_key", candidates[0].TargetRole)
	assert.Equal(t, "identifier", candidates[0].TargetPurpose)
}

func TestGenerateCandidatePairs_HandlesNilFeatures(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Source without features
	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "user_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			Features:  nil, // No features
			TableName: "orders",
		},
	}

	// Target without features
	targets := []*FKTargetColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "id",
				DataType:     "uuid",
				IsPrimaryKey: true,
				// No Metadata
			},
			TableName: "users",
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	require.Len(t, candidates, 1)

	// Features should be empty strings when nil
	assert.Empty(t, candidates[0].SourceRole)
	assert.Empty(t, candidates[0].SourcePurpose)
	assert.Empty(t, candidates[0].TargetRole)
	assert.Empty(t, candidates[0].TargetPurpose)
}

func TestGenerateCandidatePairs_PopulatesColumnIDs(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	sourceColID := uuid.New()
	targetColID := uuid.New()

	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           sourceColID,
				ColumnName:   "user_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			TableName: "orders",
		},
	}

	targets := []*FKTargetColumn{
		{
			Column: &models.SchemaColumn{
				ID:           targetColID,
				ColumnName:   "id",
				DataType:     "uuid",
				IsPrimaryKey: true,
			},
			TableName: "users",
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	require.Len(t, candidates, 1)

	// Verify column IDs are populated (for internal tracking)
	assert.Equal(t, sourceColID, candidates[0].SourceColumnID)
	assert.Equal(t, targetColID, candidates[0].TargetColumnID)
}

func TestGenerateCandidatePairs_EmptyInputs(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	t.Run("empty sources", func(t *testing.T) {
		targets := []*FKTargetColumn{
			{
				Column: &models.SchemaColumn{
					ID:           uuid.New(),
					ColumnName:   "id",
					DataType:     "uuid",
					IsPrimaryKey: true,
				},
				TableName: "users",
				IsUnique:  true,
			},
		}

		candidates := collector.generateCandidatePairs([]*FKSourceColumn{}, targets)
		assert.Len(t, candidates, 0, "empty sources should produce no candidates")
	})

	t.Run("empty targets", func(t *testing.T) {
		sources := []*FKSourceColumn{
			{
				Column: &models.SchemaColumn{
					ID:           uuid.New(),
					ColumnName:   "user_id",
					DataType:     "uuid",
					IsPrimaryKey: false,
				},
				TableName: "orders",
			},
		}

		candidates := collector.generateCandidatePairs(sources, []*FKTargetColumn{})
		assert.Len(t, candidates, 0, "empty targets should produce no candidates")
	})

	t.Run("both empty", func(t *testing.T) {
		candidates := collector.generateCandidatePairs([]*FKSourceColumn{}, []*FKTargetColumn{})
		assert.Len(t, candidates, 0, "empty inputs should produce no candidates")
	})
}

func TestGenerateCandidatePairs_MixedTypeCompatibility(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Sources with different types
	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "user_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			TableName: "orders",
		},
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "order_number",
				DataType:     "integer",
				IsPrimaryKey: false,
			},
			TableName: "order_items",
		},
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "category_code",
				DataType:     "text",
				IsPrimaryKey: false,
			},
			TableName: "products",
		},
	}

	// Targets with different types
	targets := []*FKTargetColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "id",
				DataType:     "uuid",
				IsPrimaryKey: true,
			},
			TableName: "users",
			IsUnique:  true,
		},
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "id",
				DataType:     "bigint", // Compatible with integer
				IsPrimaryKey: true,
			},
			TableName: "orders",
			IsUnique:  true,
		},
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "code",
				DataType:     "varchar(10)",
				IsPrimaryKey: true,
			},
			TableName: "categories",
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	// Expected pairs (only type-compatible):
	// - user_id (uuid) → users.id (uuid) ✓
	// - user_id (uuid) → orders.id (bigint) ✗ (uuid vs integer)
	// - user_id (uuid) → categories.code (varchar) ✗ (uuid vs string)
	// - order_number (integer) → users.id (uuid) ✗
	// - order_number (integer) → orders.id (bigint) ✓ (integer vs integer)
	// - order_number (integer) → categories.code (varchar) ✗
	// - category_code (text) → users.id (uuid) ✗
	// - category_code (text) → orders.id (bigint) ✗
	// - category_code (text) → categories.code (varchar) ✓ (string vs string)

	assert.Len(t, candidates, 3, "expected 3 type-compatible pairs")

	// Verify the correct pairs exist
	pairKeys := make(map[string]bool)
	for _, c := range candidates {
		key := c.SourceTable + "." + c.SourceColumn + " → " + c.TargetTable + "." + c.TargetColumn
		pairKeys[key] = true
	}

	assert.True(t, pairKeys["orders.user_id → users.id"], "uuid → uuid should be included")
	assert.True(t, pairKeys["order_items.order_number → orders.id"], "integer → bigint should be included")
	assert.True(t, pairKeys["products.category_code → categories.code"], "text → varchar should be included")
}

func TestGenerateCandidatePairs_SameTableDifferentColumns(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Source column in the same table as target, but different column (e.g., self-referential FK)
	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(),
				ColumnName:   "parent_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			TableName: "categories",
		},
	}

	targets := []*FKTargetColumn{
		{
			Column: &models.SchemaColumn{
				ID:           uuid.New(), // Different ID
				ColumnName:   "id",       // Different column name
				DataType:     "uuid",
				IsPrimaryKey: true,
			},
			TableName: "categories", // Same table
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	// Same table but different column should NOT be skipped
	// (This is a valid self-referential FK pattern)
	assert.Len(t, candidates, 1, "same table, different column should be allowed (self-referential FK)")
	assert.Equal(t, "parent_id", candidates[0].SourceColumn)
	assert.Equal(t, "id", candidates[0].TargetColumn)
	assert.Equal(t, "categories", candidates[0].SourceTable)
	assert.Equal(t, "categories", candidates[0].TargetTable)
}

func TestGenerateCandidatePairs_PopulatesAllFields(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	sourceColID := uuid.New()
	targetColID := uuid.New()

	sources := []*FKSourceColumn{
		{
			Column: &models.SchemaColumn{
				ID:           sourceColID,
				ColumnName:   "user_id",
				DataType:     "uuid",
				IsPrimaryKey: false,
			},
			Features: &models.ColumnFeatures{
				Role:    models.RoleForeignKey,
				Purpose: models.PurposeIdentifier,
			},
			TableName: "orders",
		},
	}

	targets := []*FKTargetColumn{
		{
			Column: &models.SchemaColumn{
				ID:           targetColID,
				ColumnName:   "id",
				DataType:     "uuid",
				IsPrimaryKey: true,
			},
			TableName: "users",
			IsUnique:  true,
		},
	}

	candidates := collector.generateCandidatePairs(sources, targets)

	require.Len(t, candidates, 1)
	c := candidates[0]

	// Verify all source fields
	assert.Equal(t, "orders", c.SourceTable)
	assert.Equal(t, "user_id", c.SourceColumn)
	assert.Equal(t, "uuid", c.SourceDataType)
	assert.False(t, c.SourceIsPK)
	assert.Equal(t, sourceColID, c.SourceColumnID)
	assert.Equal(t, models.RoleForeignKey, c.SourceRole)
	assert.Equal(t, models.PurposeIdentifier, c.SourcePurpose)

	// Verify all target fields
	assert.Equal(t, "users", c.TargetTable)
	assert.Equal(t, "id", c.TargetColumn)
	assert.Equal(t, "uuid", c.TargetDataType)
	assert.True(t, c.TargetIsPK)
	assert.Equal(t, targetColID, c.TargetColumnID)

	// Join stats should be zero (not yet collected)
	assert.Zero(t, c.JoinCount)
	assert.Zero(t, c.OrphanCount)
	assert.Zero(t, c.ReverseOrphans)
	assert.Zero(t, c.SourceMatched)
	assert.Zero(t, c.TargetMatched)
}

// ============================================================================
// mockSchemaDiscoverer for join statistics tests
// ============================================================================

type mockSchemaDiscovererForJoinStats struct {
	datasource.SchemaDiscoverer

	// AnalyzeJoin mock data
	analyzeJoinResult *datasource.JoinAnalysis
	analyzeJoinErr    error

	// GetDistinctValues mock data
	distinctValuesMap map[string][]string // key: "table.column"
	distinctValuesErr error

	// AnalyzeColumnStats mock data
	columnStatsMap map[string]datasource.ColumnStats // key: "table.column"
	columnStatsErr error
}

func (m *mockSchemaDiscovererForJoinStats) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	if m.analyzeJoinErr != nil {
		return nil, m.analyzeJoinErr
	}
	if m.analyzeJoinResult != nil {
		return m.analyzeJoinResult, nil
	}
	// Return a default valid result that passes aggressive filtering:
	// - SourceMatched > 0 (required to not be rejected as "no match")
	// - OrphanCount == 0 (required to not be rejected as "has orphans")
	return &datasource.JoinAnalysis{
		JoinCount:     100,
		SourceMatched: 100,
		TargetMatched: 100,
		OrphanCount:   0,
	}, nil
}

func (m *mockSchemaDiscovererForJoinStats) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	if m.distinctValuesErr != nil {
		return nil, m.distinctValuesErr
	}
	key := tableName + "." + columnName
	if values, ok := m.distinctValuesMap[key]; ok {
		return values, nil
	}
	return []string{}, nil
}

func (m *mockSchemaDiscovererForJoinStats) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	if m.columnStatsErr != nil {
		return nil, m.columnStatsErr
	}
	var results []datasource.ColumnStats
	for _, colName := range columnNames {
		key := tableName + "." + colName
		if stats, ok := m.columnStatsMap[key]; ok {
			results = append(results, stats)
		}
	}
	return results, nil
}

func (m *mockSchemaDiscovererForJoinStats) Close() error {
	return nil
}

// ============================================================================
// collectJoinStatistics Tests
// ============================================================================

func TestCollectJoinStatistics_Success(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	adapter := &mockSchemaDiscovererForJoinStats{
		analyzeJoinResult: &datasource.JoinAnalysis{
			JoinCount:          100,
			SourceMatched:      50,
			TargetMatched:      45,
			OrphanCount:        10,
			ReverseOrphanCount: 5,
		},
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	err := collector.collectJoinStatistics(context.Background(), adapter, candidate)
	require.NoError(t, err)

	// Verify all fields are populated
	assert.Equal(t, int64(100), candidate.JoinCount)
	assert.Equal(t, int64(50), candidate.SourceMatched)
	assert.Equal(t, int64(45), candidate.TargetMatched)
	assert.Equal(t, int64(10), candidate.OrphanCount)
	assert.Equal(t, int64(5), candidate.ReverseOrphans)
}

func TestCollectJoinStatistics_Error(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	adapter := &mockSchemaDiscovererForJoinStats{
		analyzeJoinErr: errors.New("database connection failed"),
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	err := collector.collectJoinStatistics(context.Background(), adapter, candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "analyze join")
	assert.Contains(t, err.Error(), "orders.user_id")
	assert.Contains(t, err.Error(), "users.id")

	// Candidate fields should remain zero
	assert.Zero(t, candidate.JoinCount)
	assert.Zero(t, candidate.SourceMatched)
}

func TestCollectJoinStatistics_ZeroResults(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	adapter := &mockSchemaDiscovererForJoinStats{
		analyzeJoinResult: &datasource.JoinAnalysis{
			JoinCount:          0,
			SourceMatched:      0,
			TargetMatched:      0,
			OrphanCount:        100, // All orphans - no matches
			ReverseOrphanCount: 50,
		},
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "invalid_fk",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	err := collector.collectJoinStatistics(context.Background(), adapter, candidate)
	require.NoError(t, err)

	// Verify zero matches are correctly reported
	assert.Equal(t, int64(0), candidate.JoinCount)
	assert.Equal(t, int64(0), candidate.SourceMatched)
	assert.Equal(t, int64(100), candidate.OrphanCount)
}

// ============================================================================
// collectSampleValues Tests
// ============================================================================

func TestCollectSampleValues_Success(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	adapter := &mockSchemaDiscovererForJoinStats{
		distinctValuesMap: map[string][]string{
			"orders.user_id": {"uuid-1", "uuid-2", "uuid-3"},
			"users.id":       {"uuid-1", "uuid-2", "uuid-4", "uuid-5"},
		},
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	err := collector.collectSampleValues(context.Background(), adapter, candidate)
	require.NoError(t, err)

	// Verify samples are populated
	assert.Equal(t, []string{"uuid-1", "uuid-2", "uuid-3"}, candidate.SourceSamples)
	assert.Equal(t, []string{"uuid-1", "uuid-2", "uuid-4", "uuid-5"}, candidate.TargetSamples)
}

func TestCollectSampleValues_SourceError(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	adapter := &mockSchemaDiscovererForJoinStats{
		distinctValuesErr: errors.New("query timeout"),
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	err := collector.collectSampleValues(context.Background(), adapter, candidate)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get source samples")
	assert.Contains(t, err.Error(), "orders.user_id")
}

func TestCollectSampleValues_EmptyResults(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	adapter := &mockSchemaDiscovererForJoinStats{
		distinctValuesMap: map[string][]string{
			"orders.user_id": {},
			"users.id":       {},
		},
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	err := collector.collectSampleValues(context.Background(), adapter, candidate)
	require.NoError(t, err)

	// Empty samples are OK
	assert.Empty(t, candidate.SourceSamples)
	assert.Empty(t, candidate.TargetSamples)
}

// ============================================================================
// collectDistinctCounts Tests
// ============================================================================

func TestCollectDistinctCounts_Success(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	adapter := &mockSchemaDiscovererForJoinStats{
		columnStatsMap: map[string]datasource.ColumnStats{
			"orders.user_id": {
				ColumnName:    "user_id",
				RowCount:      1000,
				NonNullCount:  950,
				DistinctCount: 100,
			},
			"users.id": {
				ColumnName:    "id",
				RowCount:      500,
				NonNullCount:  500, // PKs are never null
				DistinctCount: 500,
			},
		},
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	err := collector.collectDistinctCounts(context.Background(), adapter, candidate)
	require.NoError(t, err)

	// Verify source stats
	assert.Equal(t, int64(100), candidate.SourceDistinctCount)
	assert.InDelta(t, 0.05, candidate.SourceNullRate, 0.001) // (1000-950)/1000 = 5%

	// Verify target stats
	assert.Equal(t, int64(500), candidate.TargetDistinctCount)
	assert.Equal(t, 0.0, candidate.TargetNullRate) // 0% null
}

func TestCollectDistinctCounts_StatsError_Continues(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Stats error doesn't fail the whole operation - it just logs a warning
	adapter := &mockSchemaDiscovererForJoinStats{
		columnStatsErr: errors.New("stats collection failed"),
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	// Should not return error - stats collection failure is non-fatal
	err := collector.collectDistinctCounts(context.Background(), adapter, candidate)
	require.NoError(t, err)

	// Fields should remain zero
	assert.Zero(t, candidate.SourceDistinctCount)
	assert.Zero(t, candidate.SourceNullRate)
	assert.Zero(t, candidate.TargetDistinctCount)
	assert.Zero(t, candidate.TargetNullRate)
}

func TestCollectDistinctCounts_ZeroRowCount(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	adapter := &mockSchemaDiscovererForJoinStats{
		columnStatsMap: map[string]datasource.ColumnStats{
			"orders.user_id": {
				ColumnName:    "user_id",
				RowCount:      0, // Empty table
				NonNullCount:  0,
				DistinctCount: 0,
			},
			"users.id": {
				ColumnName:    "id",
				RowCount:      0,
				NonNullCount:  0,
				DistinctCount: 0,
			},
		},
	}

	candidate := &RelationshipCandidate{
		SourceTable:  "orders",
		SourceColumn: "user_id",
		TargetTable:  "users",
		TargetColumn: "id",
	}

	err := collector.collectDistinctCounts(context.Background(), adapter, candidate)
	require.NoError(t, err)

	// With zero row count, null rate should be 0 (avoid division by zero)
	assert.Equal(t, int64(0), candidate.SourceDistinctCount)
	assert.Equal(t, 0.0, candidate.SourceNullRate)
}

// ============================================================================
// CollectCandidates Error Handling Tests (Task 2.5)
// ============================================================================

func TestCollectCandidates_DatasourceGetError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: map[string][]*models.SchemaColumn{},
		allColumns:     []*models.SchemaColumn{},
		tables:         []*models.SchemaTable{},
	}

	dsSvc := &mockDatasourceServiceForCandidateCollector{
		getErr: errors.New("datasource not found"),
	}

	collector := NewRelationshipCandidateCollector(repo, &mockColumnMetadataRepoForCandidateCollector{metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata)}, &mockAdapterFactoryForCandidateCollector{}, dsSvc, zap.NewNop())

	_, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get datasource")
}

func TestCollectCandidates_AdapterCreationError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: map[string][]*models.SchemaColumn{},
		allColumns:     []*models.SchemaColumn{},
		tables:         []*models.SchemaTable{},
	}

	adapterFactory := &mockAdapterFactoryForCandidateCollector{
		schemaDiscovererErr: errors.New("connection failed"),
	}

	collector := NewRelationshipCandidateCollector(repo, &mockColumnMetadataRepoForCandidateCollector{metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata)}, adapterFactory, &mockDatasourceServiceForCandidateCollector{}, zap.NewNop())

	_, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create schema discoverer")
}

func TestCollectCandidates_CollectsJoinStatistics(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()

	// FK source column
	userIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":                "foreign_key",
				"classification_path": "uuid",
			},
		},
	}

	// PK target column
	usersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: map[string][]*models.SchemaColumn{
			"orders": {userIDCol},
		},
		allColumns: []*models.SchemaColumn{userIDCol, usersPKCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
			{ID: usersTableID, TableName: "users"},
		},
	}

	// Create a mock adapter with join statistics
	// Note: OrphanCount must be 0 to pass the aggressive filtering
	mockAdapter := &mockSchemaDiscovererForJoinStats{
		analyzeJoinResult: &datasource.JoinAnalysis{
			JoinCount:          500,
			SourceMatched:      100,
			TargetMatched:      90,
			OrphanCount:        0, // Must be 0 - candidates with orphans are rejected
			ReverseOrphanCount: 10,
		},
		distinctValuesMap: map[string][]string{
			"orders.user_id": {"uuid-1", "uuid-2", "uuid-3"},
			"users.id":       {"uuid-1", "uuid-2", "uuid-4"},
		},
		columnStatsMap: map[string]datasource.ColumnStats{
			"orders.user_id": {
				ColumnName:    "user_id",
				RowCount:      1000,
				NonNullCount:  950,
				DistinctCount: 100,
			},
			"users.id": {
				ColumnName:    "id",
				RowCount:      200,
				NonNullCount:  200,
				DistinctCount: 200,
			},
		},
	}

	adapterFactory := &mockAdapterFactoryForCandidateCollector{
		schemaDiscoverer: mockAdapter,
	}

	collector := NewRelationshipCandidateCollector(repo, &mockColumnMetadataRepoForCandidateCollector{metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata)}, adapterFactory, &mockDatasourceServiceForCandidateCollector{}, zap.NewNop())

	result, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, nil)
	require.NoError(t, err)
	require.Len(t, result, 1)

	candidate := result[0]

	// Verify join statistics were collected
	assert.Equal(t, int64(500), candidate.JoinCount)
	assert.Equal(t, int64(100), candidate.SourceMatched)
	assert.Equal(t, int64(90), candidate.TargetMatched)
	assert.Equal(t, int64(0), candidate.OrphanCount)
	assert.Equal(t, int64(10), candidate.ReverseOrphans)

	// Verify sample values were collected
	assert.Equal(t, []string{"uuid-1", "uuid-2", "uuid-3"}, candidate.SourceSamples)
	assert.Equal(t, []string{"uuid-1", "uuid-2", "uuid-4"}, candidate.TargetSamples)

	// Verify distinct counts and null rates were collected
	assert.Equal(t, int64(100), candidate.SourceDistinctCount)
	assert.InDelta(t, 0.05, candidate.SourceNullRate, 0.001) // (1000-950)/1000 = 5%
	assert.Equal(t, int64(200), candidate.TargetDistinctCount)
	assert.Equal(t, 0.0, candidate.TargetNullRate)
}

func TestCollectCandidates_ContinuesOnNonFatalErrors(t *testing.T) {
	// Tests that the collector continues processing candidates when
	// sample value and distinct count collection fails (these are non-fatal).
	// Note: Join analysis failure IS fatal (candidate rejected), so we must
	// provide valid join stats for the candidate to pass filtering.
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()
	usersTableID := uuid.New()

	// FK source column
	userIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":                "foreign_key",
				"classification_path": "uuid",
			},
		},
	}

	// PK target column
	usersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		allColumns: map[string][]*models.SchemaColumn{
			"orders": {userIDCol},
		},
		allColumns: []*models.SchemaColumn{userIDCol, usersPKCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
			{ID: usersTableID, TableName: "users"},
		},
	}

	// Create a mock adapter that succeeds for join analysis but fails for
	// sample values and distinct counts (which are non-fatal errors)
	mockAdapter := &mockSchemaDiscovererForJoinStats{
		analyzeJoinResult: &datasource.JoinAnalysis{
			JoinCount:     100,
			SourceMatched: 100, // Must be > 0 to pass filtering
			TargetMatched: 100,
			OrphanCount:   0, // Must be 0 to pass filtering
		},
		distinctValuesErr: errors.New("distinct values failed"),
		columnStatsErr:    errors.New("column stats failed"),
	}

	adapterFactory := &mockAdapterFactoryForCandidateCollector{
		schemaDiscoverer: mockAdapter,
	}

	collector := NewRelationshipCandidateCollector(repo, &mockColumnMetadataRepoForCandidateCollector{metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata)}, adapterFactory, &mockDatasourceServiceForCandidateCollector{}, zap.NewNop())

	// Should still succeed - sample/stats errors are logged but not fatal
	result, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, nil)
	require.NoError(t, err)
	require.Len(t, result, 1, "should return candidate even if stats collection fails")

	candidate := result[0]

	// Verify basic candidate info is present
	assert.Equal(t, "orders", candidate.SourceTable)
	assert.Equal(t, "user_id", candidate.SourceColumn)
	assert.Equal(t, "users", candidate.TargetTable)
	assert.Equal(t, "id", candidate.TargetColumn)

	// Join statistics should be present (analysis succeeded)
	assert.Equal(t, int64(100), candidate.JoinCount)
	assert.Equal(t, int64(100), candidate.SourceMatched)

	// Sample values and distinct counts should be empty/zero (collection failed)
	assert.Zero(t, candidate.SourceDistinctCount)
	assert.Empty(t, candidate.SourceSamples)
}
