package services

import (
	"context"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Test Helpers
// ============================================================================

// testContextWithAuth creates a context with JWT claims for testing
func testContextWithAuthRD(projectID, userID string) context.Context {
	claims := &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject: userID,
		},
		ProjectID: projectID,
	}
	return context.WithValue(context.Background(), auth.ClaimsKey, claims)
}

// ============================================================================
// Mock Implementations for Relationship Discovery Tests
// ============================================================================

// rdMockSchemaRepository is a mock for SchemaRepository used in relationship discovery tests.
type rdMockSchemaRepository struct {
	mockSchemaRepository // embed existing mock

	// Additional fields for discovery
	columnsByTable map[uuid.UUID][]*models.SchemaColumn
}

func (m *rdMockSchemaRepository) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	if m.listColumnsErr != nil {
		return nil, m.listColumnsErr
	}
	if cols, ok := m.columnsByTable[tableID]; ok {
		return cols, nil
	}
	return []*models.SchemaColumn{}, nil
}

func (m *rdMockSchemaRepository) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}

func (m *rdMockSchemaRepository) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	if m.upsertRelationshipErr != nil {
		return m.upsertRelationshipErr
	}
	m.upsertedRelationships = append(m.upsertedRelationships, rel)
	return nil
}

// rdMockSchemaDiscoverer is a mock for SchemaDiscoverer used in relationship discovery tests.
type rdMockSchemaDiscoverer struct {
	columnStats     map[string][]datasource.ColumnStats // key: schema.table
	valueOverlap    *datasource.ValueOverlapResult
	joinAnalysis    *datasource.JoinAnalysis
	statsErr        error
	valueOverlapErr error
	joinAnalysisErr error
}

func (m *rdMockSchemaDiscoverer) DiscoverTables(ctx context.Context) ([]datasource.TableMetadata, error) {
	return nil, nil
}

func (m *rdMockSchemaDiscoverer) DiscoverColumns(ctx context.Context, schemaName, tableName string) ([]datasource.ColumnMetadata, error) {
	return nil, nil
}

func (m *rdMockSchemaDiscoverer) DiscoverForeignKeys(ctx context.Context) ([]datasource.ForeignKeyMetadata, error) {
	return nil, nil
}

func (m *rdMockSchemaDiscoverer) SupportsForeignKeys() bool {
	return true
}

func (m *rdMockSchemaDiscoverer) AnalyzeColumnStats(ctx context.Context, schemaName, tableName string, columnNames []string) ([]datasource.ColumnStats, error) {
	if m.statsErr != nil {
		return nil, m.statsErr
	}
	key := schemaName + "." + tableName
	if stats, ok := m.columnStats[key]; ok {
		return stats, nil
	}
	// Return stats for each column
	result := make([]datasource.ColumnStats, len(columnNames))
	for i, name := range columnNames {
		result[i] = datasource.ColumnStats{
			ColumnName:    name,
			RowCount:      100,
			NonNullCount:  100,
			DistinctCount: 100,
		}
	}
	return result, nil
}

func (m *rdMockSchemaDiscoverer) CheckValueOverlap(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string, sampleLimit int) (*datasource.ValueOverlapResult, error) {
	if m.valueOverlapErr != nil {
		return nil, m.valueOverlapErr
	}
	if m.valueOverlap != nil {
		return m.valueOverlap, nil
	}
	return &datasource.ValueOverlapResult{
		MatchRate:      0.95,
		SourceDistinct: 100,
		TargetDistinct: 100,
		MatchedCount:   95,
	}, nil
}

func (m *rdMockSchemaDiscoverer) AnalyzeJoin(ctx context.Context, sourceSchema, sourceTable, sourceColumn, targetSchema, targetTable, targetColumn string) (*datasource.JoinAnalysis, error) {
	if m.joinAnalysisErr != nil {
		return nil, m.joinAnalysisErr
	}
	if m.joinAnalysis != nil {
		return m.joinAnalysis, nil
	}
	return &datasource.JoinAnalysis{
		SourceMatched: 100,
		TargetMatched: 100,
		JoinCount:     100,
		OrphanCount:   0,
	}, nil
}

func (m *rdMockSchemaDiscoverer) GetDistinctValues(ctx context.Context, schemaName, tableName, columnName string, limit int) ([]string, error) {
	return nil, nil
}

func (m *rdMockSchemaDiscoverer) Close() error {
	return nil
}

// rdMockAdapterFactory is a mock for DatasourceAdapterFactory used in relationship discovery tests.
type rdMockAdapterFactory struct {
	discoverer    *rdMockSchemaDiscoverer
	discovererErr error
}

func (m *rdMockAdapterFactory) NewConnectionTester(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.ConnectionTester, error) {
	return nil, errors.New("not implemented")
}

func (m *rdMockAdapterFactory) NewSchemaDiscoverer(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.SchemaDiscoverer, error) {
	if m.discovererErr != nil {
		return nil, m.discovererErr
	}
	return m.discoverer, nil
}

func (m *rdMockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	return nil
}

func (m *rdMockAdapterFactory) NewQueryExecutor(ctx context.Context, dsType string, config map[string]any, projectID, datasourceID uuid.UUID, userID string) (datasource.QueryExecutor, error) {
	return nil, errors.New("not implemented")
}

// ============================================================================
// Helper Functions
// ============================================================================

func newTestRelationshipDiscoveryService(repo *rdMockSchemaRepository, dsSvc *mockDatasourceService, factory *rdMockAdapterFactory) RelationshipDiscoveryService {
	return NewRelationshipDiscoveryService(repo, dsSvc, factory, zap.NewNop())
}

// ============================================================================
// Tests
// ============================================================================

func TestRelationshipDiscoveryService_DiscoverRelationships_DatasourceError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &rdMockSchemaRepository{}
	dsSvc := &mockDatasourceService{
		getErr: errors.New("datasource not found"),
	}
	factory := &rdMockAdapterFactory{}

	service := newTestRelationshipDiscoveryService(repo, dsSvc, factory)

	ctx := testContextWithAuthRD(projectID.String(), "test-user-id")
	_, err := service.DiscoverRelationships(ctx, projectID, datasourceID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "failed to get datasource: datasource not found" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRelationshipDiscoveryService_DiscoverRelationships_AdapterError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	repo := &rdMockSchemaRepository{}
	dsSvc := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost"},
		},
	}
	factory := &rdMockAdapterFactory{
		discovererErr: errors.New("connection failed"),
	}

	service := newTestRelationshipDiscoveryService(repo, dsSvc, factory)

	ctx := testContextWithAuthRD(projectID.String(), "test-user-id")
	_, err := service.DiscoverRelationships(ctx, projectID, datasourceID)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "failed to create schema discoverer: connection failed" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRelationshipDiscoveryService_DiscoverRelationships_EmptyTables(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	tableID := uuid.New()

	zeroRowCount := int64(0)
	repo := &rdMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{ID: tableID, TableName: "empty_table", SchemaName: "public", RowCount: &zeroRowCount},
			},
		},
	}
	dsSvc := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost"},
		},
	}
	discoverer := &rdMockSchemaDiscoverer{}
	factory := &rdMockAdapterFactory{discoverer: discoverer}

	service := newTestRelationshipDiscoveryService(repo, dsSvc, factory)

	ctx := testContextWithAuthRD(projectID.String(), "test-user-id")
	result, err := service.DiscoverRelationships(ctx, projectID, datasourceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TablesAnalyzed != 1 {
		t.Errorf("expected 1 table analyzed, got %d", result.TablesAnalyzed)
	}
	if result.EmptyTables != 1 {
		t.Errorf("expected 1 empty table, got %d", result.EmptyTables)
	}
	if len(result.EmptyTableNames) != 1 || result.EmptyTableNames[0] != "empty_table" {
		t.Errorf("expected empty_table in EmptyTableNames, got %v", result.EmptyTableNames)
	}
}

func TestRelationshipDiscoveryService_DiscoverRelationships_Success(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	usersIDColID := uuid.New()
	ordersUserIDColID := uuid.New()

	rowCount := int64(100)
	repo := &rdMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{ID: usersTableID, TableName: "users", SchemaName: "public", RowCount: &rowCount, DatasourceID: datasourceID},
				{ID: ordersTableID, TableName: "orders", SchemaName: "public", RowCount: &rowCount, DatasourceID: datasourceID},
			},
			relationships: []*models.SchemaRelationship{}, // no existing relationships
		},
		columnsByTable: map[uuid.UUID][]*models.SchemaColumn{
			usersTableID: {
				{ID: usersIDColID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, SchemaTableID: usersTableID},
			},
			ordersTableID: {
				{ID: ordersUserIDColID, ColumnName: "user_id", DataType: "uuid", IsPrimaryKey: false, SchemaTableID: ordersTableID},
			},
		},
	}
	dsSvc := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost"},
		},
	}
	discoverer := &rdMockSchemaDiscoverer{
		columnStats: map[string][]datasource.ColumnStats{
			"public.users": {
				{ColumnName: "id", RowCount: 100, NonNullCount: 100, DistinctCount: 100},
			},
			"public.orders": {
				{ColumnName: "user_id", RowCount: 100, NonNullCount: 100, DistinctCount: 50},
			},
		},
		valueOverlap: &datasource.ValueOverlapResult{
			MatchRate:      0.95,
			SourceDistinct: 50,
			TargetDistinct: 100,
			MatchedCount:   48,
		},
		joinAnalysis: &datasource.JoinAnalysis{
			SourceMatched: 100,
			TargetMatched: 50,
			JoinCount:     100,
			OrphanCount:   5,
		},
	}
	factory := &rdMockAdapterFactory{discoverer: discoverer}

	service := newTestRelationshipDiscoveryService(repo, dsSvc, factory)

	ctx := testContextWithAuthRD(projectID.String(), "test-user-id")
	result, err := service.DiscoverRelationships(ctx, projectID, datasourceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TablesAnalyzed != 2 {
		t.Errorf("expected 2 tables analyzed, got %d", result.TablesAnalyzed)
	}
	if result.ColumnsAnalyzed != 2 {
		t.Errorf("expected 2 columns analyzed, got %d", result.ColumnsAnalyzed)
	}
	if result.EmptyTables != 0 {
		t.Errorf("expected 0 empty tables, got %d", result.EmptyTables)
	}
	// One relationship should be created (orders.user_id -> users.id)
	if result.RelationshipsCreated != 1 {
		t.Errorf("expected 1 relationship created, got %d", result.RelationshipsCreated)
	}
}

func TestRelationshipDiscoveryService_DiscoverRelationships_LowMatchRate(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()
	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	usersIDColID := uuid.New()
	ordersUserIDColID := uuid.New()

	rowCount := int64(100)
	repo := &rdMockSchemaRepository{
		mockSchemaRepository: mockSchemaRepository{
			tables: []*models.SchemaTable{
				{ID: usersTableID, TableName: "users", SchemaName: "public", RowCount: &rowCount, DatasourceID: datasourceID},
				{ID: ordersTableID, TableName: "orders", SchemaName: "public", RowCount: &rowCount, DatasourceID: datasourceID},
			},
			relationships: []*models.SchemaRelationship{},
		},
		columnsByTable: map[uuid.UUID][]*models.SchemaColumn{
			usersTableID: {
				{ID: usersIDColID, ColumnName: "id", DataType: "uuid", IsPrimaryKey: true, SchemaTableID: usersTableID},
			},
			ordersTableID: {
				{ID: ordersUserIDColID, ColumnName: "user_id", DataType: "uuid", IsPrimaryKey: false, SchemaTableID: ordersTableID},
			},
		},
	}
	dsSvc := &mockDatasourceService{
		datasource: &models.Datasource{
			ID:             datasourceID,
			ProjectID:      projectID,
			DatasourceType: "postgres",
			Config:         map[string]any{"host": "localhost"},
		},
	}
	discoverer := &rdMockSchemaDiscoverer{
		valueOverlap: &datasource.ValueOverlapResult{
			MatchRate:      0.30, // Below threshold
			SourceDistinct: 50,
			TargetDistinct: 100,
			MatchedCount:   15,
		},
	}
	factory := &rdMockAdapterFactory{discoverer: discoverer}

	service := newTestRelationshipDiscoveryService(repo, dsSvc, factory)

	ctx := testContextWithAuthRD(projectID.String(), "test-user-id")
	result, err := service.DiscoverRelationships(ctx, projectID, datasourceID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No relationships should be created due to low match rate
	if result.RelationshipsCreated != 0 {
		t.Errorf("expected 0 relationships created due to low match rate, got %d", result.RelationshipsCreated)
	}

	// Verify rejected candidate was recorded with rejection reason
	if len(repo.upsertedRelationships) != 1 {
		t.Fatalf("expected 1 rejected relationship recorded, got %d", len(repo.upsertedRelationships))
	}
	recorded := repo.upsertedRelationships[0]
	if recorded.RejectionReason == nil {
		t.Error("expected rejection reason to be set")
	} else if *recorded.RejectionReason != models.RejectionLowMatchRate {
		t.Errorf("expected rejection reason '%s', got '%s'", models.RejectionLowMatchRate, *recorded.RejectionReason)
	}
	if recorded.IsValidated {
		t.Error("expected IsValidated to be false for rejected candidate")
	}
}

// ============================================================================
// classifyJoinability Tests
// ============================================================================

func TestClassifyJoinability_PrimaryKey(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	col := &models.SchemaColumn{
		ColumnName:   "id",
		DataType:     "uuid",
		IsPrimaryKey: true,
	}

	isJoinable, reason := service.classifyJoinability(col, nil, 100)
	if !isJoinable {
		t.Error("expected PK column to be joinable")
	}
	if reason != models.JoinabilityPK {
		t.Errorf("expected reason 'pk', got '%s'", reason)
	}
}

func TestClassifyJoinability_ExcludedType_Timestamp(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	col := &models.SchemaColumn{
		ColumnName: "created_at",
		DataType:   "timestamp",
	}

	isJoinable, reason := service.classifyJoinability(col, nil, 100)
	if isJoinable {
		t.Error("expected timestamp column to not be joinable")
	}
	if reason != models.JoinabilityTypeExcluded {
		t.Errorf("expected reason 'type_excluded', got '%s'", reason)
	}
}

func TestClassifyJoinability_ExcludedType_Boolean(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	col := &models.SchemaColumn{
		ColumnName: "is_active",
		DataType:   "boolean",
	}

	isJoinable, reason := service.classifyJoinability(col, nil, 100)
	if isJoinable {
		t.Error("expected boolean column to not be joinable")
	}
	if reason != models.JoinabilityTypeExcluded {
		t.Errorf("expected reason 'type_excluded', got '%s'", reason)
	}
}

func TestClassifyJoinability_ExcludedType_Varchar(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	col := &models.SchemaColumn{
		ColumnName: "description",
		DataType:   "character varying(255)",
	}

	isJoinable, reason := service.classifyJoinability(col, nil, 100)
	if isJoinable {
		t.Error("expected varchar column to not be joinable")
	}
	if reason != models.JoinabilityTypeExcluded {
		t.Errorf("expected reason 'type_excluded', got '%s'", reason)
	}
}

func TestClassifyJoinability_NoStats(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	col := &models.SchemaColumn{
		ColumnName: "user_id",
		DataType:   "uuid",
	}

	isJoinable, reason := service.classifyJoinability(col, nil, 100)
	if isJoinable {
		t.Error("expected column without stats to not be joinable")
	}
	if reason != models.JoinabilityNoStats {
		t.Errorf("expected reason 'no_stats', got '%s'", reason)
	}
}

func TestClassifyJoinability_UniqueValues(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	col := &models.SchemaColumn{
		ColumnName: "email",
		DataType:   "text",
	}
	stats := &datasource.ColumnStats{
		RowCount:      100,
		NonNullCount:  100,
		DistinctCount: 100, // All unique
	}

	isJoinable, reason := service.classifyJoinability(col, stats, 100)
	if !isJoinable {
		t.Error("expected column with unique values to be joinable")
	}
	if reason != models.JoinabilityUniqueValues {
		t.Errorf("expected reason 'unique_values', got '%s'", reason)
	}
}

func TestClassifyJoinability_LowCardinality(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	col := &models.SchemaColumn{
		ColumnName: "status",
		DataType:   "text",
	}
	stats := &datasource.ColumnStats{
		RowCount:      10000,
		NonNullCount:  10000,
		DistinctCount: 5, // Very low cardinality
	}

	isJoinable, reason := service.classifyJoinability(col, stats, 10000)
	if isJoinable {
		t.Error("expected low cardinality column to not be joinable")
	}
	if reason != models.JoinabilityLowCardinality {
		t.Errorf("expected reason 'low_cardinality', got '%s'", reason)
	}
}

func TestClassifyJoinability_CardinalityOK(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	col := &models.SchemaColumn{
		ColumnName: "user_id",
		DataType:   "uuid",
	}
	stats := &datasource.ColumnStats{
		RowCount:      1000,
		NonNullCount:  1000,
		DistinctCount: 200, // 20% distinct - reasonable cardinality
	}

	isJoinable, reason := service.classifyJoinability(col, stats, 1000)
	if !isJoinable {
		t.Error("expected column with reasonable cardinality to be joinable")
	}
	if reason != models.JoinabilityCardinalityOK {
		t.Errorf("expected reason 'cardinality_ok', got '%s'", reason)
	}
}

// ============================================================================
// areTypesCompatible Tests
// ============================================================================

func TestAreTypesCompatible_SameType(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}

	if !service.areTypesCompatible("uuid", "uuid") {
		t.Error("expected uuid types to be compatible")
	}
	if !service.areTypesCompatible("text", "text") {
		t.Error("expected text types to be compatible")
	}
}

func TestAreTypesCompatible_DifferentTypes(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}

	if service.areTypesCompatible("uuid", "text") {
		t.Error("expected uuid and text to be incompatible")
	}
	if service.areTypesCompatible("integer", "text") {
		t.Error("expected integer and text to be incompatible")
	}
}

func TestAreTypesCompatible_NormalizedTypes(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}

	// Types with length specifiers should be normalized
	if !service.areTypesCompatible("varchar(255)", "varchar(100)") {
		t.Error("expected varchar types with different lengths to be compatible")
	}
}

// ============================================================================
// inferCardinality Tests
// ============================================================================

func TestInferCardinality_OneToOne(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	join := &datasource.JoinAnalysis{
		SourceMatched: 100,
		TargetMatched: 100,
		JoinCount:     100,
	}

	cardinality := service.inferCardinality(join)
	if cardinality != models.Cardinality1To1 {
		t.Errorf("expected 1:1, got %s", cardinality)
	}
}

func TestInferCardinality_NToOne(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	join := &datasource.JoinAnalysis{
		SourceMatched: 100,
		TargetMatched: 10,
		JoinCount:     100,
	}

	cardinality := service.inferCardinality(join)
	if cardinality != models.CardinalityNTo1 {
		t.Errorf("expected N:1, got %s", cardinality)
	}
}

func TestInferCardinality_OneToN(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	join := &datasource.JoinAnalysis{
		SourceMatched: 10,
		TargetMatched: 100,
		JoinCount:     100,
	}

	cardinality := service.inferCardinality(join)
	if cardinality != models.Cardinality1ToN {
		t.Errorf("expected 1:N, got %s", cardinality)
	}
}

func TestInferCardinality_NToM(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	join := &datasource.JoinAnalysis{
		SourceMatched: 50,
		TargetMatched: 50,
		JoinCount:     200, // Many-to-many
	}

	cardinality := service.inferCardinality(join)
	if cardinality != models.CardinalityNToM {
		t.Errorf("expected N:M, got %s", cardinality)
	}
}

func TestInferCardinality_Unknown(t *testing.T) {
	service := &relationshipDiscoveryService{logger: zap.NewNop()}
	join := &datasource.JoinAnalysis{
		SourceMatched: 0,
		TargetMatched: 0,
		JoinCount:     0,
	}

	cardinality := service.inferCardinality(join)
	if cardinality != models.CardinalityUnknown {
		t.Errorf("expected unknown, got %s", cardinality)
	}
}

// ============================================================================
// isNumericType Tests
// ============================================================================

func TestIsNumericType_IntegerTypes(t *testing.T) {
	intTypes := []string{"integer", "int", "int4", "bigint", "int8", "smallint", "int2"}
	for _, typ := range intTypes {
		if !isNumericType(typ) {
			t.Errorf("expected %s to be numeric", typ)
		}
	}
}

func TestIsNumericType_SerialTypes(t *testing.T) {
	serialTypes := []string{"serial", "bigserial", "smallserial"}
	for _, typ := range serialTypes {
		if !isNumericType(typ) {
			t.Errorf("expected %s to be numeric", typ)
		}
	}
}

func TestIsNumericType_DecimalTypes(t *testing.T) {
	decimalTypes := []string{"numeric", "decimal", "real", "double precision", "float", "float4", "float8"}
	for _, typ := range decimalTypes {
		if !isNumericType(typ) {
			t.Errorf("expected %s to be numeric", typ)
		}
	}
}

func TestIsNumericType_NonNumericTypes(t *testing.T) {
	nonNumericTypes := []string{"uuid", "text", "varchar", "boolean", "timestamp", "date"}
	for _, typ := range nonNumericTypes {
		if isNumericType(typ) {
			t.Errorf("expected %s to not be numeric", typ)
		}
	}
}

// ============================================================================
// shouldCreateCandidate Tests
// ============================================================================

func TestShouldCreateCandidate_AttributeColumns(t *testing.T) {
	// Rule 2: Attribute columns (email, password, name, description, status, type)
	// should NOT be FK sources
	tests := []struct {
		sourceColumn string
		targetTable  string
		wantResult   bool
		description  string
	}{
		// These should be rejected (attribute columns)
		{"email", "users", false, "email column should not be FK source"},
		{"user_email", "emails", false, "column containing 'email' should not be FK source"},
		{"password", "users", false, "password column should not be FK source"},
		{"hashed_password", "passwords", false, "column containing 'password' should not be FK source"},
		{"name", "users", false, "name column should not be FK source"},
		{"first_name", "names", false, "column containing 'name' should not be FK source"},
		{"description", "items", false, "description column should not be FK source"},
		{"short_description", "descriptions", false, "column containing 'description' should not be FK source"},
		{"status", "statuses", false, "status column should not be FK source"},
		{"order_status", "statuses", false, "column containing 'status' should not be FK source"},
		{"type", "types", false, "type column should not be FK source"},
		{"account_type", "types", false, "column containing 'type' should not be FK source"},

		// Case insensitivity tests
		{"EMAIL", "users", false, "EMAIL (uppercase) should not be FK source"},
		{"Status", "statuses", false, "Status (mixed case) should not be FK source"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := shouldCreateCandidate(tt.sourceColumn, tt.targetTable)
			if result != tt.wantResult {
				t.Errorf("shouldCreateCandidate(%q, %q) = %v, want %v",
					tt.sourceColumn, tt.targetTable, result, tt.wantResult)
			}
		})
	}
}

func TestShouldCreateCandidate_IDColumnNaming(t *testing.T) {
	// Rule 1: *_id columns should match their expected table
	tests := []struct {
		sourceColumn string
		targetTable  string
		wantResult   bool
		description  string
	}{
		// Valid: user_id should point to users/user table
		{"user_id", "users", true, "user_id → users is valid"},
		{"user_id", "user", true, "user_id → user is valid"},

		// Valid: account_id should point to accounts/account table
		{"account_id", "accounts", true, "account_id → accounts is valid"},
		{"account_id", "account", true, "account_id → account is valid"},

		// Valid: category_id with -ies plural
		{"category_id", "categories", true, "category_id → categories is valid"},
		{"category_id", "category", true, "category_id → category is valid"},

		// Invalid: user_id should NOT point to other tables
		{"user_id", "channels", false, "user_id → channels is invalid"},
		{"user_id", "accounts", false, "user_id → accounts is invalid"},
		{"account_id", "users", false, "account_id → users is invalid"},
		{"account_id", "channels", false, "account_id → channels is invalid"},

		// Case insensitivity tests
		{"User_ID", "Users", true, "User_ID → Users (mixed case) is valid"},
		{"USER_ID", "USERS", true, "USER_ID → USERS (uppercase) is valid"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := shouldCreateCandidate(tt.sourceColumn, tt.targetTable)
			if result != tt.wantResult {
				t.Errorf("shouldCreateCandidate(%q, %q) = %v, want %v",
					tt.sourceColumn, tt.targetTable, result, tt.wantResult)
			}
		})
	}
}

func TestShouldCreateCandidate_NonIDColumns(t *testing.T) {
	// Non-*_id columns that aren't attribute columns should be allowed
	tests := []struct {
		sourceColumn string
		targetTable  string
		wantResult   bool
		description  string
	}{
		// These should be allowed (not _id columns, not attribute columns)
		{"uuid", "users", true, "uuid column should be allowed"},
		{"code", "orders", true, "code column should be allowed"},
		{"reference", "items", true, "reference column should be allowed"},
		{"external_ref", "partners", true, "external_ref column should be allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := shouldCreateCandidate(tt.sourceColumn, tt.targetTable)
			if result != tt.wantResult {
				t.Errorf("shouldCreateCandidate(%q, %q) = %v, want %v",
					tt.sourceColumn, tt.targetTable, result, tt.wantResult)
			}
		})
	}
}

func TestShouldCreateCandidate_BugScenarios(t *testing.T) {
	// Test the specific bug scenarios from FIX-ontology-extraction-bug4.md
	tests := []struct {
		sourceColumn string
		targetTable  string
		wantResult   bool
		description  string
	}{
		// BUG-4a: accounts.email → account_authentications.email should be prevented
		{"email", "account_authentications", false, "email should not be FK source (BUG-4a)"},

		// BUG-4b: account_id → channels should be prevented (different ID types)
		{"account_id", "channels", false, "account_id should not point to channels (BUG-4b)"},

		// Valid case: account_authentications.account_id → accounts is valid
		{"account_id", "accounts", true, "account_id → accounts is valid FK"},
	}

	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			result := shouldCreateCandidate(tt.sourceColumn, tt.targetTable)
			if result != tt.wantResult {
				t.Errorf("shouldCreateCandidate(%q, %q) = %v, want %v",
					tt.sourceColumn, tt.targetTable, result, tt.wantResult)
			}
		})
	}
}
