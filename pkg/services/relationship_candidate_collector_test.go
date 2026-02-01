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
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {userIDCol},
		},
		allColumns: []*models.SchemaColumn{userIDCol, usersPKCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
			{ID: usersTableID, TableName: "users"},
		},
	}

	collector := NewRelationshipCandidateCollector(repo, &mockAdapterFactoryForCandidateCollector{}, zap.NewNop())

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

	// Progress callback should be called at least 4 times (steps 0-3)
	assert.GreaterOrEqual(t, progressCalls, 4, "progress callback should be called for each step")
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
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": {userIDCol},
		},
		allColumns: []*models.SchemaColumn{userIDCol},
		tables: []*models.SchemaTable{
			{ID: ordersTableID, TableName: "orders"},
		},
	}

	collector := NewRelationshipCandidateCollector(repo, &mockAdapterFactoryForCandidateCollector{}, zap.NewNop())

	result, err := collector.CollectCandidates(context.Background(), projectID, datasourceID, nil)
	require.NoError(t, err)

	// No targets = no candidates
	assert.Empty(t, result, "no FK targets means no candidates")
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
