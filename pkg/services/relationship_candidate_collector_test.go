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

	// GetColumnsWithFeaturesByDatasource mock data
	columnsByTable        map[string][]*models.SchemaColumn
	columnsByTableErr     error

	// ListColumnsByDatasource mock data
	allColumns    []*models.SchemaColumn
	allColumnsErr error

	// ListTablesByDatasource mock data
	tables    []*models.SchemaTable
	tablesErr error
}

func (m *mockSchemaRepoForCandidateCollector) GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error) {
	if m.columnsByTableErr != nil {
		return nil, m.columnsByTableErr
	}
	return m.columnsByTable, nil
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

// mockAdapterFactoryForCandidateCollector is a mock for DatasourceAdapterFactory.
type mockAdapterFactoryForCandidateCollector struct{}

func (m *mockAdapterFactoryForCandidateCollector) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAdapterFactoryForCandidateCollector) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAdapterFactoryForCandidateCollector) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAdapterFactoryForCandidateCollector) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

// ============================================================================
// Helper Functions
// ============================================================================

func newTestCandidateCollector(repo *mockSchemaRepoForCandidateCollector) *relationshipCandidateCollector {
	return &relationshipCandidateCollector{
		schemaRepo:     repo,
		adapterFactory: &mockAdapterFactoryForCandidateCollector{},
		logger:         zap.NewNop(),
	}
}

// createColumnWithFeatures creates a test column with the specified features in metadata.
func createColumnWithFeatures(columnName, dataType string, isPK bool, features *models.ColumnFeatures) *models.SchemaColumn {
	col := &models.SchemaColumn{
		ID:           uuid.New(),
		ColumnName:   columnName,
		DataType:     dataType,
		IsPrimaryKey: isPK,
	}
	if features != nil {
		col.Metadata = map[string]any{
			"column_features": map[string]any{
				"purpose":             features.Purpose,
				"role":                features.Role,
				"classification_path": string(features.ClassificationPath),
			},
		}
	}
	return col
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

	assert.True(t, collector.shouldExcludeFromFKSources(col), "primary keys should be excluded - they are targets, not sources")
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
			assert.True(t, collector.shouldExcludeFromFKSources(col), "%s should be excluded", tt.dataType)
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
			assert.True(t, collector.shouldExcludeFromFKSources(col), "%s should be excluded", tt.dataType)
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
			assert.True(t, collector.shouldExcludeFromFKSources(col), "%s should be excluded", tt.dataType)
		})
	}
}

func TestShouldExcludeFromFKSources_FeatureClassificationPath(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Test columns with features that indicate exclusion via classification path
	excludedPaths := []models.ClassificationPath{
		models.ClassificationPathTimestamp,
		models.ClassificationPathBoolean,
		models.ClassificationPathJSON,
	}

	for _, path := range excludedPaths {
		t.Run(string(path), func(t *testing.T) {
			col := createColumnWithFeatures("some_column", "text", false, &models.ColumnFeatures{
				ClassificationPath: path,
			})
			assert.True(t, collector.shouldExcludeFromFKSources(col), "classification_path=%s should be excluded", path)
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
			assert.False(t, collector.shouldExcludeFromFKSources(col), tt.description)
		})
	}
}

// ============================================================================
// isQualifiedFKSource Tests
// ============================================================================

func TestIsQualifiedFKSource_RoleForeignKey(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ColumnName: "user_id",
		DataType:   "uuid",
	}
	features := &models.ColumnFeatures{
		Role: models.RoleForeignKey,
	}

	assert.True(t, collector.isQualifiedFKSource(col, features), "role=foreign_key should qualify")
}

func TestIsQualifiedFKSource_PurposeIdentifier(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ColumnName: "account_id",
		DataType:   "uuid",
	}
	features := &models.ColumnFeatures{
		Purpose: models.PurposeIdentifier,
	}

	assert.True(t, collector.isQualifiedFKSource(col, features), "purpose=identifier should qualify")
}

func TestIsQualifiedFKSource_ClassificationPathUUID(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ColumnName: "some_id",
		DataType:   "uuid",
	}
	features := &models.ColumnFeatures{
		ClassificationPath: models.ClassificationPathUUID,
	}

	assert.True(t, collector.isQualifiedFKSource(col, features), "classification_path=uuid should qualify")
}

func TestIsQualifiedFKSource_ClassificationPathExternalID(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	col := &models.SchemaColumn{
		ColumnName: "stripe_customer_id",
		DataType:   "text",
	}
	features := &models.ColumnFeatures{
		ClassificationPath: models.ClassificationPathExternalID,
	}

	assert.True(t, collector.isQualifiedFKSource(col, features), "classification_path=external_id should qualify")
}

func TestIsQualifiedFKSource_NotQualified(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// Test columns that should NOT qualify
	tests := []struct {
		name     string
		features *models.ColumnFeatures
	}{
		{
			name: "attribute role",
			features: &models.ColumnFeatures{
				Role:    models.RoleAttribute,
				Purpose: models.PurposeText,
			},
		},
		{
			name: "measure role",
			features: &models.ColumnFeatures{
				Role:    models.RoleMeasure,
				Purpose: models.PurposeMeasure,
			},
		},
		{
			name: "text classification",
			features: &models.ColumnFeatures{
				ClassificationPath: models.ClassificationPathText,
				Purpose:            models.PurposeText,
			},
		},
		{
			name: "numeric classification",
			features: &models.ColumnFeatures{
				ClassificationPath: models.ClassificationPathNumeric,
				Purpose:            models.PurposeMeasure,
			},
		},
		{
			name: "enum classification",
			features: &models.ColumnFeatures{
				ClassificationPath: models.ClassificationPathEnum,
				Purpose:            models.PurposeEnum,
			},
		},
	}

	col := &models.SchemaColumn{
		ColumnName: "some_column",
		DataType:   "text",
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, collector.isQualifiedFKSource(col, tt.features), "%s should not qualify", tt.name)
		})
	}
}

func TestIsQualifiedFKSource_MultipleQualifications(t *testing.T) {
	collector := newTestCandidateCollector(nil)

	// A column with multiple qualifying criteria should still qualify
	col := &models.SchemaColumn{
		ColumnName: "user_id",
		DataType:   "uuid",
	}
	features := &models.ColumnFeatures{
		Role:               models.RoleForeignKey,
		Purpose:            models.PurposeIdentifier,
		ClassificationPath: models.ClassificationPathUUID,
	}

	assert.True(t, collector.isQualifiedFKSource(col, features), "column with multiple qualifications should qualify")
}

// ============================================================================
// identifyFKSources Tests
// ============================================================================

func TestIdentifyFKSources_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()

	// Create columns with features
	userIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "user_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":                "foreign_key",
				"purpose":             "identifier",
				"classification_path": "uuid",
			},
		},
	}

	accountIDCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: ordersTableID,
		ColumnName:    "account_id",
		DataType:      "uuid",
		IsPrimaryKey:  false,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":                "foreign_key",
				"classification_path": "uuid",
			},
		},
	}

	// Primary key should be excluded
	usersPKCol := &models.SchemaColumn{
		ID:            uuid.New(),
		SchemaTableID: usersTableID,
		ColumnName:    "id",
		DataType:      "uuid",
		IsPrimaryKey:  true,
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":                "primary_key",
				"classification_path": "uuid",
			},
		},
	}

	repo := &mockSchemaRepoForCandidateCollector{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {userIDCol, accountIDCol},
			"users":  {usersPKCol},
		},
		allColumns: []*models.SchemaColumn{userIDCol, accountIDCol, usersPKCol},
		tables: []*models.SchemaTable{
			{ID: usersTableID, TableName: "users"},
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	collector := newTestCandidateCollector(repo)

	sources, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
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
	// Test that columns marked as joinable but without features are included
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()

	isJoinable := true
	joinabilityReason := "cardinality_ok"

	// Column with no features but marked as joinable
	joinableCol := &models.SchemaColumn{
		ID:                uuid.New(),
		SchemaTableID:     ordersTableID,
		ColumnName:        "legacy_id",
		DataType:          "integer",
		IsPrimaryKey:      false,
		IsJoinable:        &isJoinable,
		JoinabilityReason: &joinabilityReason,
	}

	repo := &mockSchemaRepoForCandidateCollector{
		columnsByTable: map[string][]*models.SchemaColumn{}, // No columns with features
		allColumns:     []*models.SchemaColumn{joinableCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	collector := newTestCandidateCollector(repo)

	sources, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
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

	repo := &mockSchemaRepoForCandidateCollector{
		columnsByTable: map[string][]*models.SchemaColumn{},
		allColumns:     []*models.SchemaColumn{timestampCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	collector := newTestCandidateCollector(repo)

	sources, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	assert.Len(t, sources, 0, "timestamp should be excluded even if joinable")
}

func TestIdentifyFKSources_NoDuplicates(t *testing.T) {
	// Test that a column appearing in both features and joinable lists is not duplicated
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
		Metadata: map[string]any{
			"column_features": map[string]any{
				"role":                "foreign_key",
				"classification_path": "uuid",
			},
		},
	}

	repo := &mockSchemaRepoForCandidateCollector{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {col},
		},
		allColumns: []*models.SchemaColumn{col},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	collector := newTestCandidateCollector(repo)

	sources, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
	require.NoError(t, err)

	assert.Len(t, sources, 1, "column should appear only once")
	assert.Equal(t, "user_id", sources[0].Column.ColumnName)
}

func TestIdentifyFKSources_ErrorHandling(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	t.Run("GetColumnsWithFeaturesByDatasource error", func(t *testing.T) {
		repo := &mockSchemaRepoForCandidateCollector{
			columnsByTableErr: errors.New("database error"),
		}

		collector := newTestCandidateCollector(repo)

		_, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "get columns with features")
	})

	t.Run("ListColumnsByDatasource error", func(t *testing.T) {
		repo := &mockSchemaRepoForCandidateCollector{
			columnsByTable: map[string][]*models.SchemaColumn{},
			allColumnsErr:  errors.New("database error"),
		}

		collector := newTestCandidateCollector(repo)

		_, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list all columns")
	})

	t.Run("ListTablesByDatasource error", func(t *testing.T) {
		repo := &mockSchemaRepoForCandidateCollector{
			columnsByTable: map[string][]*models.SchemaColumn{},
			allColumns:     []*models.SchemaColumn{},
			tablesErr:      errors.New("database error"),
		}

		collector := newTestCandidateCollector(repo)

		_, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list tables")
	})
}

func TestIdentifyFKSources_EmptyDataset(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepoForCandidateCollector{
		columnsByTable: map[string][]*models.SchemaColumn{},
		allColumns:     []*models.SchemaColumn{},
		tables:         []*models.SchemaTable{},
	}

	collector := newTestCandidateCollector(repo)

	sources, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
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
		columnsByTable: map[string][]*models.SchemaColumn{},
		allColumns:     []*models.SchemaColumn{col},
		tables:         []*models.SchemaTable{}, // Empty - no tables to resolve
	}

	collector := newTestCandidateCollector(repo)

	sources, err := collector.identifyFKSources(context.Background(), projectID, datasourceID)
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
	assert.Equal(t, features, fkSource.Features)
	assert.Equal(t, "orders", fkSource.TableName)
	assert.Equal(t, "user_id", fkSource.Column.ColumnName)
}

// ============================================================================
// CollectCandidates Tests (Stub behavior)
// ============================================================================

func TestCollectCandidates_CallsIdentifyFKSources(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	ordersTableID := uuid.New()

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
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {userIDCol},
		},
		allColumns: []*models.SchemaColumn{userIDCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	collector := NewRelationshipCandidateCollector(repo, &mockAdapterFactoryForCandidateCollector{}, zap.NewNop())

	// Track progress callbacks
	progressCalls := 0
	progressCallback := func(current, total int, message string) {
		progressCalls++
	}

	// Currently returns nil as it's a stub - test that it doesn't error
	result, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, progressCallback)
	require.NoError(t, err)

	// Stub returns nil
	assert.Nil(t, result, "stub should return nil")

	// Progress callback should be called at least twice (start and after FK source identification)
	assert.GreaterOrEqual(t, progressCalls, 2, "progress callback should be called")
}

func TestCollectCandidates_PropagatesError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &mockSchemaRepoForCandidateCollector{
		columnsByTableErr: errors.New("database error"),
	}

	collector := NewRelationshipCandidateCollector(repo, &mockAdapterFactoryForCandidateCollector{}, zap.NewNop())

	_, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "identify FK sources")
}
