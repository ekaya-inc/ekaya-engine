//go:build ignore

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// ============================================================================
// Mock Implementations for Finalization Tests
// ============================================================================

type mockOntologyRepoForFinalization struct {
	activeOntology       *models.TieredOntology
	updatedDomainSummary *models.DomainSummary
	getActiveErr         error
	updateSummaryErr     error
}

func (m *mockOntologyRepoForFinalization) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	if m.getActiveErr != nil {
		return nil, m.getActiveErr
	}
	return m.activeOntology, nil
}

func (m *mockOntologyRepoForFinalization) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	if m.updateSummaryErr != nil {
		return m.updateSummaryErr
	}
	m.updatedDomainSummary = summary
	return nil
}

func (m *mockOntologyRepoForFinalization) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, columns []models.ColumnDetail) error {
	return nil
}

func (m *mockOntologyRepoForFinalization) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

func (m *mockOntologyRepoForFinalization) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

type mockSchemaRepoForFinalization struct {
	tables          []*models.SchemaTable
	columnsByTable  map[string][]*models.SchemaColumn
	listTablesErr   error
	getColumnsByErr error
}

func (m *mockSchemaRepoForFinalization) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	if m.listTablesErr != nil {
		return nil, m.listTablesErr
	}
	return m.tables, nil
}

func (m *mockSchemaRepoForFinalization) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
	if m.getColumnsByErr != nil {
		return nil, m.getColumnsByErr
	}
	return m.columnsByTable, nil
}

// Stub implementations for SchemaRepository interface
func (m *mockSchemaRepoForFinalization) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFinalization) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFinalization) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64, sampleValues []string) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnFeatures(ctx context.Context, projectID, columnID uuid.UUID, features *models.ColumnFeatures) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForFinalization) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepoForFinalization) ClearColumnFeaturesByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (m *mockSchemaRepoForFinalization) GetRelationshipsByMethod(ctx context.Context, projectID, datasourceID uuid.UUID, method string) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForFinalization) DeleteInferredRelationshipsByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	return 0, nil
}

type mockLLMClient struct {
	responseContent string
	generateErr     error
}

func (m *mockLLMClient) GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
	if m.generateErr != nil {
		return nil, m.generateErr
	}
	return &llm.GenerateResponseResult{
		Content:          m.responseContent,
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}, nil
}

func (m *mockLLMClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (m *mockLLMClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (m *mockLLMClient) GetModel() string {
	return "test-model"
}

func (m *mockLLMClient) GetEndpoint() string {
	return "https://test.endpoint"
}

type mockLLMFactoryForFinalization struct {
	client    llm.LLMClient
	createErr error
}

func (m *mockLLMFactoryForFinalization) CreateForProject(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.client, nil
}

func (m *mockLLMFactoryForFinalization) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.client, nil
}

func (m *mockLLMFactoryForFinalization) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

// ============================================================================
// Tests
// ============================================================================

func TestOntologyFinalization_GeneratesDomainDescription(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	desc1 := "Users of the platform"
	desc2 := "Customer orders"
	tables := []*models.SchemaTable{
		{TableName: "users", Description: &desc1},
		{TableName: "orders", Description: &desc2},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}

	schemaRepo := &mockSchemaRepoForFinalization{
		tables:         tables,
		columnsByTable: map[string][]*models.SchemaColumn{},
	}

	expectedDescription := "This is an e-commerce platform that tracks users and their orders."
	llmClient := &mockLLMClient{
		responseContent: `{"description": "This is an e-commerce platform that tracks users and their orders."}`,
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, schemaRepo, nil,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Verify domain summary was updated with LLM-generated description
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	assert.Equal(t, expectedDescription, ontologyRepo.updatedDomainSummary.Description)
}

func TestOntologyFinalization_SkipsIfNoTables(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	schemaRepo := &mockSchemaRepoForFinalization{
		tables:         []*models.SchemaTable{},
		columnsByTable: map[string][]*models.SchemaColumn{},
	}
	llmFactory := &mockLLMFactoryForFinalization{}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, schemaRepo, nil,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Verify no domain summary was updated (skipped)
	assert.Nil(t, ontologyRepo.updatedDomainSummary)
}

func TestOntologyFinalization_SkipsIfNoActiveOntology(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: nil, // No active ontology
	}
	schemaRepo := &mockSchemaRepoForFinalization{}
	llmFactory := &mockLLMFactoryForFinalization{}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, schemaRepo, nil,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Verify no domain summary was updated (skipped)
	assert.Nil(t, ontologyRepo.updatedDomainSummary)
}

func TestOntologyFinalization_LLMFailure(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "users"},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{
			ID:        ontologyID,
			ProjectID: projectID,
			IsActive:  true,
		},
	}
	schemaRepo := &mockSchemaRepoForFinalization{
		tables:         tables,
		columnsByTable: map[string][]*models.SchemaColumn{},
	}

	llmClient := &mockLLMClient{
		generateErr: errors.New("LLM unavailable"),
	}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}

	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(
		ontologyRepo, schemaRepo, nil,
		llmFactory, nil, logger,
	)

	err := svc.Finalize(ctx, projectID)

	// Verify error is propagated
	require.Error(t, err)
	assert.Contains(t, err.Error(), "LLM unavailable")

	// Verify domain summary was NOT updated
	assert.Nil(t, ontologyRepo.updatedDomainSummary)
}

// ============================================================================
// Convention Discovery Tests
// ============================================================================

func TestOntologyFinalization_DiscoversSoftDelete_Timestamp(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	// Two tables
	tables := []*models.SchemaTable{
		{TableName: "users"},
		{TableName: "orders"},
	}

	// Both tables have deleted_at column
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid", IsNullable: false},
			{ColumnName: "deleted_at", DataType: "timestamp with time zone", IsNullable: true},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid", IsNullable: false},
			{ColumnName: "deleted_at", DataType: "timestamptz", IsNullable: true},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)

	sd := ontologyRepo.updatedDomainSummary.Conventions.SoftDelete
	assert.True(t, sd.Enabled)
	assert.Equal(t, "deleted_at", sd.Column)
	assert.Equal(t, "timestamp", sd.ColumnType)
	assert.Equal(t, "deleted_at IS NULL", sd.Filter)
	assert.Equal(t, 1.0, sd.Coverage) // 100% of tables
}

func TestOntologyFinalization_DiscoversSoftDelete_Boolean(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "users"},
		{TableName: "orders"},
	}

	// Both tables have is_deleted boolean column
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid", IsNullable: false},
			{ColumnName: "is_deleted", DataType: "boolean", IsNullable: false},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid", IsNullable: false},
			{ColumnName: "is_deleted", DataType: "bool", IsNullable: false},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)

	sd := ontologyRepo.updatedDomainSummary.Conventions.SoftDelete
	assert.True(t, sd.Enabled)
	assert.Equal(t, "is_deleted", sd.Column)
	assert.Equal(t, "boolean", sd.ColumnType)
	assert.Equal(t, "is_deleted = false", sd.Filter)
}

func TestOntologyFinalization_DiscoversSoftDelete_Coverage(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	// 4 tables, only 1 has deleted_at (25% coverage - below threshold)
	tables := []*models.SchemaTable{
		{TableName: "users"},
		{TableName: "orders"},
		{TableName: "products"},
		{TableName: "categories"},
	}

	columnsByTable := map[string][]*models.SchemaColumn{
		"users":      {{ColumnName: "id", DataType: "uuid"}, {ColumnName: "deleted_at", DataType: "timestamptz", IsNullable: true}},
		"orders":     {{ColumnName: "id", DataType: "uuid"}},
		"products":   {{ColumnName: "id", DataType: "uuid"}},
		"categories": {{ColumnName: "id", DataType: "uuid"}},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Should NOT report soft delete convention (below 50% threshold)
	if ontologyRepo.updatedDomainSummary.Conventions != nil {
		assert.Nil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)
	}
}

func TestOntologyFinalization_DiscoversCurrency_Cents(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "transactions"},
	}

	// Multiple integer amount columns
	columnsByTable := map[string][]*models.SchemaColumn{
		"transactions": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "total_amount", DataType: "bigint"},
			{ColumnName: "fee_amount", DataType: "integer"},
			{ColumnName: "net_amount", DataType: "int8"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.Currency)

	cur := ontologyRepo.updatedDomainSummary.Conventions.Currency
	assert.Equal(t, "USD", cur.DefaultCurrency)
	assert.Equal(t, "cents", cur.Format)
	assert.Equal(t, "divide_by_100", cur.Transform)
	assert.Contains(t, cur.ColumnPatterns, "*_amount")
}

func TestOntologyFinalization_DiscoversCurrency_Dollars(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "transactions"},
	}

	// Decimal amount columns suggest dollars
	columnsByTable := map[string][]*models.SchemaColumn{
		"transactions": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "total_amount", DataType: "decimal(10,2)"},
			{ColumnName: "unit_price", DataType: "numeric(12,2)"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.Currency)

	cur := ontologyRepo.updatedDomainSummary.Conventions.Currency
	assert.Equal(t, "dollars", cur.Format)
	assert.Equal(t, "none", cur.Transform)
}

func TestOntologyFinalization_DiscoversAuditColumns_WithCoverage(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "users"},
		{TableName: "orders"},
	}

	// Both tables have audit columns
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "created_at", DataType: "timestamptz"},
			{ColumnName: "updated_at", DataType: "timestamptz"},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "created_at", DataType: "timestamptz"},
			{ColumnName: "updated_at", DataType: "timestamptz"},
			{ColumnName: "created_by", DataType: "uuid"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)

	audit := ontologyRepo.updatedDomainSummary.Conventions.AuditColumns
	require.Len(t, audit, 3) // created_at and updated_at (100% coverage), created_by is 50% (meets >= 0.5 threshold)

	// Verify created_at and updated_at are included with 100% coverage
	var createdAt, updatedAt, createdBy *models.AuditColumnInfo
	for i := range audit {
		switch audit[i].Column {
		case "created_at":
			createdAt = &audit[i]
		case "updated_at":
			updatedAt = &audit[i]
		case "created_by":
			createdBy = &audit[i]
		}
	}
	require.NotNil(t, createdAt)
	require.NotNil(t, updatedAt)
	require.NotNil(t, createdBy)
	assert.Equal(t, 1.0, createdAt.Coverage)
	assert.Equal(t, 1.0, updatedAt.Coverage)
	assert.Equal(t, 0.5, createdBy.Coverage) // 1 of 2 tables
}

func TestOntologyFinalization_NoConventions(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "users"},
	}

	// No soft delete, no currency, no audit columns
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "name", DataType: "text"},
			{ColumnName: "email", DataType: "text"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	// Conventions should be nil when nothing detected
	assert.Nil(t, ontologyRepo.updatedDomainSummary.Conventions)
}

// ============================================================================
// ColumnFeatures-based Convention Discovery Tests
// ============================================================================

func TestOntologyFinalization_ExtractsColumnFeatureInsights_SoftDelete(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "users"},
		{TableName: "orders"},
	}

	// Columns with ColumnFeatures indicating soft-delete
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "deleted_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_soft_delete":    true,
							"timestamp_purpose": "soft_delete",
						},
					},
				},
			},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "deleted_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_soft_delete":    true,
							"timestamp_purpose": "soft_delete",
						},
					},
				},
			},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)

	sd := ontologyRepo.updatedDomainSummary.Conventions.SoftDelete
	assert.True(t, sd.Enabled)
	assert.Equal(t, "deleted_at", sd.Column)
	assert.Equal(t, "timestamp", sd.ColumnType)
	assert.Equal(t, "deleted_at IS NULL", sd.Filter)
	assert.Equal(t, 1.0, sd.Coverage) // 100% of tables
}

func TestOntologyFinalization_ExtractsColumnFeatureInsights_AuditColumns(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "users"},
		{TableName: "orders"},
	}

	// Columns with ColumnFeatures indicating audit fields
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "created_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_audit_field":    true,
							"timestamp_purpose": "audit_created",
						},
					},
				},
			},
			{
				ColumnName: "updated_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_audit_field":    true,
							"timestamp_purpose": "audit_updated",
						},
					},
				},
			},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid"},
			{
				ColumnName: "created_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_audit_field":    true,
							"timestamp_purpose": "audit_created",
						},
					},
				},
			},
			{
				ColumnName: "updated_at",
				DataType:   "timestamptz",
				Metadata: map[string]any{
					"column_features": map[string]any{
						"timestamp_features": map[string]any{
							"is_audit_field":    true,
							"timestamp_purpose": "audit_updated",
						},
					},
				},
			},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)
	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)

	// Audit columns should be discovered from ColumnFeatures
	audit := ontologyRepo.updatedDomainSummary.Conventions.AuditColumns
	require.Len(t, audit, 2) // created_at and updated_at

	var createdAt, updatedAt *models.AuditColumnInfo
	for i := range audit {
		switch audit[i].Column {
		case "created_at":
			createdAt = &audit[i]
		case "updated_at":
			updatedAt = &audit[i]
		}
	}
	require.NotNil(t, createdAt)
	require.NotNil(t, updatedAt)
	assert.Equal(t, 1.0, createdAt.Coverage)
	assert.Equal(t, 1.0, updatedAt.Coverage)
}

func TestOntologyFinalization_FallsBackToPatternDetection_WhenNoColumnFeatures(t *testing.T) {
	ctx := context.Background()
	projectID := uuid.New()
	ontologyID := uuid.New()

	tables := []*models.SchemaTable{
		{TableName: "users"},
		{TableName: "orders"},
	}

	// Columns WITHOUT ColumnFeatures - should fallback to pattern-based detection
	columnsByTable := map[string][]*models.SchemaColumn{
		"users": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "deleted_at", DataType: "timestamptz", IsNullable: true}, // No Metadata
			{ColumnName: "created_at", DataType: "timestamptz"},
		},
		"orders": {
			{ColumnName: "id", DataType: "uuid"},
			{ColumnName: "deleted_at", DataType: "timestamptz", IsNullable: true}, // No Metadata
			{ColumnName: "created_at", DataType: "timestamptz"},
		},
	}

	ontologyRepo := &mockOntologyRepoForFinalization{
		activeOntology: &models.TieredOntology{ID: ontologyID, ProjectID: projectID, IsActive: true},
	}
	schemaRepo := &mockSchemaRepoForFinalization{tables: tables, columnsByTable: columnsByTable}
	llmClient := &mockLLMClient{responseContent: `{"description": "Test system."}`}
	llmFactory := &mockLLMFactoryForFinalization{client: llmClient}
	logger := zap.NewNop()

	svc := NewOntologyFinalizationService(ontologyRepo, schemaRepo, nil, llmFactory, nil, logger)

	err := svc.Finalize(ctx, projectID)
	require.NoError(t, err)

	require.NotNil(t, ontologyRepo.updatedDomainSummary)
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions)

	// Should still detect soft-delete via pattern matching fallback
	require.NotNil(t, ontologyRepo.updatedDomainSummary.Conventions.SoftDelete)
	assert.Equal(t, "deleted_at", ontologyRepo.updatedDomainSummary.Conventions.SoftDelete.Column)

	// Should still detect audit columns via pattern matching fallback
	// Both created_at and deleted_at are in the auditColumnNames list
	require.Len(t, ontologyRepo.updatedDomainSummary.Conventions.AuditColumns, 2)
}
