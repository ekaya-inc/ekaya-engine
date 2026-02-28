package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/repositories"
)

// Mock repositories for column enrichment testing
type testColEnrichmentOntologyRepo struct {
	columnDetails map[string][]models.ColumnDetail
}

func (r *testColEnrichmentOntologyRepo) UpdateColumnDetails(ctx context.Context, projectID uuid.UUID, tableName string, details []models.ColumnDetail) error {
	if r.columnDetails == nil {
		r.columnDetails = make(map[string][]models.ColumnDetail)
	}
	r.columnDetails[tableName] = details
	return nil
}

func (r *testColEnrichmentOntologyRepo) Create(ctx context.Context, ontology *models.TieredOntology) error {
	return nil
}

func (r *testColEnrichmentOntologyRepo) GetActive(ctx context.Context, projectID uuid.UUID) (*models.TieredOntology, error) {
	return nil, nil
}

func (r *testColEnrichmentOntologyRepo) UpdateDomainSummary(ctx context.Context, projectID uuid.UUID, summary *models.DomainSummary) error {
	return nil
}

func (r *testColEnrichmentOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

type testColEnrichmentSchemaRepo struct {
	columnsByTable map[string][]*models.SchemaColumn
}

func (r *testColEnrichmentSchemaRepo) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string][]*models.SchemaColumn, error) {
	result := make(map[string][]*models.SchemaColumn)
	for _, tableName := range tableNames {
		if cols, ok := r.columnsByTable[tableName]; ok {
			result[tableName] = cols
		}
	}
	return result, nil
}

func (r *testColEnrichmentSchemaRepo) GetTablesByNames(ctx context.Context, projectID uuid.UUID, tableNames []string) (map[string]*models.SchemaTable, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	// Return a SchemaTable for any table that has columns in the mock
	if _, ok := r.columnsByTable[tableName]; ok {
		return &models.SchemaTable{
			ID:           uuid.New(),
			ProjectID:    projectID,
			DatasourceID: datasourceID,
			TableName:    tableName,
		}, nil
	}
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) ListAllTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, nil
}

func (r *testColEnrichmentSchemaRepo) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (r *testColEnrichmentSchemaRepo) ListAllColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, nil
}

func (r *testColEnrichmentSchemaRepo) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}

func (r *testColEnrichmentSchemaRepo) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, nil
}

func (r *testColEnrichmentSchemaRepo) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) GetRelationshipsByMethod(ctx context.Context, projectID, datasourceID uuid.UUID, method string) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (r *testColEnrichmentSchemaRepo) DeleteInferredRelationshipsByProject(ctx context.Context, projectID uuid.UUID) (int64, error) {
	return 0, nil
}

// Mock column metadata repository for column enrichment testing
type testColEnrichmentColumnMetadataRepo struct {
	metadataByColumnID map[uuid.UUID]*models.ColumnMetadata
}

func (r *testColEnrichmentColumnMetadataRepo) Upsert(ctx context.Context, meta *models.ColumnMetadata) error {
	return nil
}

func (r *testColEnrichmentColumnMetadataRepo) UpsertFromExtraction(ctx context.Context, meta *models.ColumnMetadata) error {
	return nil
}

func (r *testColEnrichmentColumnMetadataRepo) GetBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) (*models.ColumnMetadata, error) {
	if r.metadataByColumnID != nil {
		return r.metadataByColumnID[schemaColumnID], nil
	}
	return nil, nil
}

func (r *testColEnrichmentColumnMetadataRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

func (r *testColEnrichmentColumnMetadataRepo) GetBySchemaColumnIDs(ctx context.Context, schemaColumnIDs []uuid.UUID) ([]*models.ColumnMetadata, error) {
	var result []*models.ColumnMetadata
	if r.metadataByColumnID != nil {
		for _, id := range schemaColumnIDs {
			if meta, ok := r.metadataByColumnID[id]; ok {
				result = append(result, meta)
			}
		}
	}
	return result, nil
}

func (r *testColEnrichmentColumnMetadataRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentColumnMetadataRepo) DeleteBySchemaColumnID(ctx context.Context, schemaColumnID uuid.UUID) error {
	return nil
}

// Mock conversation repository for column enrichment testing
type testColEnrichmentConversationRepo struct{}

func (r *testColEnrichmentConversationRepo) Save(ctx context.Context, conv *models.LLMConversation) error {
	return nil
}

func (r *testColEnrichmentConversationRepo) Update(ctx context.Context, conv *models.LLMConversation) error {
	return nil
}

func (r *testColEnrichmentConversationRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status, errorMessage string) error {
	return nil
}

func (r *testColEnrichmentConversationRepo) GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (r *testColEnrichmentConversationRepo) GetByContext(ctx context.Context, projectID uuid.UUID, key, value string) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (r *testColEnrichmentConversationRepo) GetByConversationID(ctx context.Context, conversationID uuid.UUID) ([]*models.LLMConversation, error) {
	return nil, nil
}

func (r *testColEnrichmentConversationRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// Mock question service for column enrichment testing
type testColEnrichmentQuestionService struct{}

func (s *testColEnrichmentQuestionService) GetNextQuestion(ctx context.Context, projectID uuid.UUID, includeSkipped bool) (*models.OntologyQuestion, error) {
	return nil, nil
}

func (s *testColEnrichmentQuestionService) GetPendingQuestions(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyQuestion, error) {
	return nil, nil
}

func (s *testColEnrichmentQuestionService) GetPendingCount(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}

func (s *testColEnrichmentQuestionService) GetPendingCounts(ctx context.Context, projectID uuid.UUID) (*repositories.QuestionCounts, error) {
	return nil, nil
}

func (s *testColEnrichmentQuestionService) AnswerQuestion(ctx context.Context, questionID uuid.UUID, answer string, userID string) (*models.AnswerResult, error) {
	return nil, nil
}

func (s *testColEnrichmentQuestionService) SkipQuestion(ctx context.Context, questionID uuid.UUID) error {
	return nil
}

func (s *testColEnrichmentQuestionService) DeleteQuestion(ctx context.Context, questionID uuid.UUID) error {
	return nil
}

func (s *testColEnrichmentQuestionService) CreateQuestions(ctx context.Context, questions []*models.OntologyQuestion) error {
	return nil
}

// Mock datasource service for testing
type testColEnrichmentDatasourceService struct {
	datasourceID uuid.UUID
}

func (s *testColEnrichmentDatasourceService) Create(ctx context.Context, projectID uuid.UUID, name, dsType, provider string, config map[string]any) (*models.Datasource, error) {
	return nil, nil
}

func (s *testColEnrichmentDatasourceService) Get(ctx context.Context, projectID, id uuid.UUID) (*models.Datasource, error) {
	return nil, nil
}

func (s *testColEnrichmentDatasourceService) GetByName(ctx context.Context, projectID uuid.UUID, name string) (*models.Datasource, error) {
	return nil, nil
}

func (s *testColEnrichmentDatasourceService) List(ctx context.Context, projectID uuid.UUID) ([]*models.DatasourceWithStatus, error) {
	// Return a mock datasource for tests
	dsID := s.datasourceID
	if dsID == uuid.Nil {
		dsID = uuid.New()
	}
	return []*models.DatasourceWithStatus{
		{
			Datasource: &models.Datasource{
				ID:        dsID,
				ProjectID: projectID,
				Name:      "test-datasource",
			},
		},
	}, nil
}

func (s *testColEnrichmentDatasourceService) Rename(ctx context.Context, id uuid.UUID, name string) error {
	return nil
}

func (s *testColEnrichmentDatasourceService) Update(ctx context.Context, id uuid.UUID, name, dsType, provider string, config map[string]any) error {
	return nil
}

func (s *testColEnrichmentDatasourceService) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (s *testColEnrichmentDatasourceService) TestConnection(ctx context.Context, dsType string, config map[string]any, datasourceID uuid.UUID) error {
	return nil
}

// Mock LLM factory and client for testing
type testColEnrichmentLLMFactory struct {
	client llm.LLMClient
}

func (f *testColEnrichmentLLMFactory) CreateForProject(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	return f.client, nil
}

func (f *testColEnrichmentLLMFactory) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	return nil, nil
}

func (f *testColEnrichmentLLMFactory) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

type testColEnrichmentLLMClient struct {
	response     string
	callCount    int
	failUntil    int // Fail until call count reaches this value
	errorType    llm.ErrorType
	errorMessage string
	generateFunc func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error)
}

func (c *testColEnrichmentLLMClient) GenerateResponse(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
	// Use custom function if provided
	if c.generateFunc != nil {
		return c.generateFunc(ctx, prompt, systemMsg, temperature, thinking)
	}

	c.callCount++

	// Simulate transient failures
	if c.callCount <= c.failUntil {
		return nil, llm.NewError(c.errorType, c.errorMessage, true, errors.New("transient error"))
	}

	return &llm.GenerateResponseResult{
		Content: c.response,
	}, nil
}

func (c *testColEnrichmentLLMClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (c *testColEnrichmentLLMClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (c *testColEnrichmentLLMClient) GetModel() string {
	return "test-model"
}

func (c *testColEnrichmentLLMClient) GetEndpoint() string {
	return "http://test"
}

func (c *testColEnrichmentLLMClient) Close() error {
	return nil
}

// Tests for column enrichment improvements

func TestColumnEnrichmentService_EnrichProject_Success(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint", IsPrimaryKey: true},
		{ColumnName: "email", DataType: "varchar"},
	}

	// Mock LLM response
	llmResponse := `{
		"columns": [
			{
				"name": "id",
				"description": "Unique identifier for the user",
				"semantic_type": "identifier",
				"role": "identifier",
				"fk_association": null
			},
			{
				"name": "email",
				"description": "User's email address",
				"semantic_type": "email",
				"role": "attribute",
				"fk_association": null
			}
		]
	}`

	// Setup service
	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": columns,
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{
			response: llmResponse,
		},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"users"}, nil)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.TablesEnriched))
	assert.Equal(t, "users", result.TablesEnriched[0])
	assert.Equal(t, 0, len(result.TablesFailed))

	// Verify LLM was called
	client := llmFactory.client.(*testColEnrichmentLLMClient)
	assert.Equal(t, 1, client.callCount)

	// Verify column details were saved
	details := ontologyRepo.columnDetails["users"]
	require.Equal(t, 2, len(details))
	assert.Equal(t, "id", details[0].Name)
	assert.Equal(t, "identifier", details[0].SemanticType)
	assert.Equal(t, "email", details[1].Name)
	assert.Equal(t, "email", details[1].SemanticType)
}

func TestColumnEnrichmentService_EnrichProject_WithRetryOnTransientError(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint", IsPrimaryKey: true},
	}

	llmResponse := `{
		"columns": [
			{
				"name": "id",
				"description": "User ID",
				"semantic_type": "identifier",
				"role": "identifier",
				"fk_association": null
			}
		]
	}`

	// Setup service with LLM that fails twice then succeeds
	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": columns,
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{
			response:     llmResponse,
			failUntil:    2,
			errorType:    llm.ErrorTypeEndpoint,
			errorMessage: "rate limited",
		},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"users"}, nil)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.TablesEnriched))
	assert.Equal(t, 0, len(result.TablesFailed))

	// Verify LLM was called with retries
	client := llmFactory.client.(*testColEnrichmentLLMClient)
	assert.Equal(t, 3, client.callCount, "Should have retried twice before success")
}

func TestColumnEnrichmentService_EnrichProject_NonRetryableError(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
	}

	// Setup service with non-retryable error
	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": columns,
		},
	}

	// Create a client that always fails with auth error
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{
			failUntil:    100, // Always fail
			errorType:    llm.ErrorTypeAuth,
			errorMessage: "authentication failed",
		},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"users"}, nil)

	// Verify - should fail after retrying (retry.Do will still attempt retries)
	// Note: retry.Do attempts all retries even for non-retryable errors,
	// but logs them differently as "non-retryable"
	require.Error(t, err) // EnrichProject now fails fast on table failures
	assert.Contains(t, err.Error(), "1 of 1 tables failed enrichment")
	assert.Equal(t, 0, len(result.TablesEnriched))
	assert.Equal(t, 1, len(result.TablesFailed))

	// Verify LLM was called with retries
	client := llmFactory.client.(*testColEnrichmentLLMClient)
	assert.Equal(t, 4, client.callCount, "Will retry even on auth error (1 initial + 3 retries)")
}

func TestColumnEnrichmentService_EnrichProject_LargeTable(t *testing.T) {
	projectID := uuid.New()

	// Create 60 columns (exceeds maxColumnsPerChunk of 50)
	columns := make([]*models.SchemaColumn, 60)
	enrichmentResponse := make([]string, 60)
	for i := 0; i < 60; i++ {
		colName := fmt.Sprintf("col_%d", i+1)
		columns[i] = &models.SchemaColumn{
			ColumnName: colName,
			DataType:   "varchar",
		}
		enrichmentResponse[i] = fmt.Sprintf(`{
			"name": "%s",
			"description": "Column %d",
			"semantic_type": "text",
			"role": "attribute",
			"fk_association": null
		}`, colName, i+1)
	}

	// Create a combined LLM response that works for both chunks
	// Since our mock always returns the same response, we'll create one with all 60 columns
	allEnrichmentsJSON := `{"columns": [` + joinStrings(enrichmentResponse, ",") + `]}`

	// Setup service
	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"large_table": columns,
		},
	}

	client := &testColEnrichmentLLMClient{
		response: allEnrichmentsJSON,
	}
	llmFactory := &testColEnrichmentLLMFactory{client: client}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"large_table"}, nil)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.TablesEnriched))
	assert.Equal(t, 0, len(result.TablesFailed))

	// Verify LLM was called twice for chunked processing
	assert.Equal(t, 2, client.callCount, "Should have made 2 LLM calls for chunked processing")

	// Verify all columns were enriched
	details := ontologyRepo.columnDetails["large_table"]
	assert.Equal(t, 60, len(details))
}

func TestColumnEnrichmentService_EnrichProject_ProgressCallback(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
	}

	llmResponse := `{
		"columns": [
			{"name": "id", "description": "ID", "semantic_type": "identifier", "role": "identifier", "fk_association": null}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"table1": columns,
			"table2": columns,
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{response: llmResponse},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Track progress callbacks
	var progressCalls []struct {
		current int
		total   int
		message string
	}
	progressCallback := func(current, total int, message string) {
		progressCalls = append(progressCalls, struct {
			current int
			total   int
			message string
		}{current, total, message})
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"table1", "table2"}, progressCallback)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 2, len(result.TablesEnriched))
	assert.Equal(t, 2, len(progressCalls), "Should have called progress callback twice")
	assert.Equal(t, 1, progressCalls[0].current)
	assert.Equal(t, 2, progressCalls[0].total)
	assert.Equal(t, 2, progressCalls[1].current)
	assert.Equal(t, 2, progressCalls[1].total)
}

func TestColumnEnrichmentService_EnrichProject_EmptyProject(t *testing.T) {
	projectID := uuid.New()

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{columnsByTable: make(map[string][]*models.SchemaColumn)}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{}, nil)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 0, len(result.TablesEnriched))
	assert.Equal(t, 0, len(result.TablesFailed))

	// Verify LLM was not called
	client := llmFactory.client.(*testColEnrichmentLLMClient)
	assert.Equal(t, 0, client.callCount, "Should not call LLM for empty project")
}

func TestColumnEnrichmentService_EnrichProject_PartialFailure(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
	}

	llmResponse := `{
		"columns": [
			{"name": "id", "description": "ID", "semantic_type": "identifier", "role": "identifier", "fk_association": null}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"table1": columns,
			"table2": columns,
			"table3": columns,
		},
	}

	// Create a custom client that fails for table2 specifically
	client := &testColEnrichmentPartialFailureClient{
		response:      llmResponse,
		failTableName: "table2",
	}

	llmFactory := &testColEnrichmentLLMFactory{client: client}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"table1", "table2", "table3"}, nil)

	// Verify - should fail fast on partial failures
	require.Error(t, err, "EnrichProject should return error for partial failures")
	assert.Contains(t, err.Error(), "1 of 3 tables failed enrichment")
	assert.Equal(t, 2, len(result.TablesEnriched), "Should enrich table1 and table3")
	assert.Equal(t, 1, len(result.TablesFailed), "Should have 1 failed table")
	assert.Contains(t, result.TablesFailed, "table2")
}

// testColEnrichmentPartialFailureClient simulates a client that fails for a specific table
type testColEnrichmentPartialFailureClient struct {
	response       string
	failTableName  string
	failCallsCount int // track how many times we've failed for the target table
}

func (c *testColEnrichmentPartialFailureClient) GenerateResponse(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
	// Fail for a specific table by checking if the table name appears in the prompt
	// This makes the test deterministic regardless of processing order
	if strings.Contains(prompt, c.failTableName) {
		c.failCallsCount++
		// Fail all attempts (including retries) for this table
		if c.failCallsCount <= 4 { // 1 initial + 3 retries
			return nil, llm.NewError(llm.ErrorTypeAuth, "auth failed", false, errors.New("auth failed"))
		}
	}
	return &llm.GenerateResponseResult{Content: c.response}, nil
}

func (c *testColEnrichmentPartialFailureClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (c *testColEnrichmentPartialFailureClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (c *testColEnrichmentPartialFailureClient) GetModel() string {
	return "test-model"
}

func (c *testColEnrichmentPartialFailureClient) GetEndpoint() string {
	return "http://test"
}

func (c *testColEnrichmentPartialFailureClient) Close() error {
	return nil
}

func TestColumnEnrichmentService_EnrichTable_WithForeignKeys(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint", IsPrimaryKey: true},
		{ColumnName: "user_id", DataType: "bigint"},
	}

	llmResponse := `{
		"columns": [
			{
				"name": "id",
				"description": "Order identifier",
				"semantic_type": "identifier",
				"role": "identifier",
				"fk_association": null
			},
			{
				"name": "user_id",
				"description": "User who placed the order",
				"semantic_type": "identifier",
				"role": "dimension",
				"fk_association": "customer"
			}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": columns,
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{response: llmResponse},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	err := service.EnrichTable(context.Background(), projectID, "orders")

	// Verify
	require.NoError(t, err)
	details := ontologyRepo.columnDetails["orders"]
	require.Equal(t, 2, len(details))

	// Check FK role was captured
	userIDCol := details[1]
	assert.Equal(t, "user_id", userIDCol.Name)
	assert.Equal(t, "customer", userIDCol.FKAssociation)
}

func TestColumnEnrichmentService_EnrichTable_WithEnumValues(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint", IsPrimaryKey: true},
		{ColumnName: "status", DataType: "varchar"},
	}

	llmResponse := `{
		"columns": [
			{
				"name": "id",
				"description": "Order identifier",
				"semantic_type": "identifier",
				"role": "identifier",
				"fk_association": null
			},
			{
				"name": "status",
				"description": "Current order status",
				"semantic_type": "status",
				"role": "dimension",
				"synonyms": ["state", "order_state"],
				"enum_values": [
					{"value": "pending", "label": "Pending", "description": "Order is pending processing"},
					{"value": "shipped", "label": "Shipped", "description": "Order has been shipped"},
					{"value": "delivered", "label": "Delivered", "description": "Order has been delivered"}
				],
				"fk_association": null
			}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": columns,
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{response: llmResponse},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	err := service.EnrichTable(context.Background(), projectID, "orders")

	// Verify
	require.NoError(t, err)
	details := ontologyRepo.columnDetails["orders"]
	require.Equal(t, 2, len(details))

	// Check enum values were captured
	statusCol := details[1]
	assert.Equal(t, "status", statusCol.Name)
	assert.Equal(t, "status", statusCol.SemanticType)
	require.Equal(t, 3, len(statusCol.EnumValues))
	assert.Equal(t, "pending", statusCol.EnumValues[0].Value)
	assert.Equal(t, "Pending", statusCol.EnumValues[0].Label)
	assert.Equal(t, 2, len(statusCol.Synonyms))
	assert.Contains(t, statusCol.Synonyms, "state")
}

func TestColumnEnrichmentService_identifyEnumCandidates(t *testing.T) {
	service := &columnEnrichmentService{
		circuitBreaker: llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:         zap.NewNop(),
	}

	distinctCount5 := int64(5)
	distinctCount100 := int64(100)

	orderStatusID := uuid.New()
	userTypeID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
		{ID: orderStatusID, ColumnName: "order_status", DataType: "varchar"},
		{ID: userTypeID, ColumnName: "user_type", DataType: "varchar"},
		{ColumnName: "email", DataType: "varchar", DistinctCount: &distinctCount100},
		{ColumnName: "priority_level", DataType: "varchar", DistinctCount: &distinctCount5},
	}

	enumPath := string(models.ClassificationPathEnum)
	metadataByColumnID := map[uuid.UUID]*models.ColumnMetadata{
		orderStatusID: {
			SchemaColumnID:     orderStatusID,
			ClassificationPath: &enumPath,
		},
		userTypeID: {
			SchemaColumnID:    userTypeID,
			NeedsEnumAnalysis: true,
		},
	}

	candidates := service.identifyEnumCandidates(columns, metadataByColumnID)

	// Should identify columns with enum metadata or low cardinality
	assert.GreaterOrEqual(t, len(candidates), 3, "Should find at least 3 enum candidates")

	candidateNames := make(map[string]bool)
	for _, c := range candidates {
		candidateNames[c.ColumnName] = true
	}

	assert.True(t, candidateNames["order_status"], "Should identify 'order_status' as enum (via metadata classification)")
	assert.True(t, candidateNames["user_type"], "Should identify 'user_type' as enum (via metadata NeedsEnumAnalysis)")
	assert.True(t, candidateNames["priority_level"], "Should identify 'priority_level' as enum (low cardinality)")
	assert.False(t, candidateNames["id"], "Should not identify 'id' as enum")
}

// TestColumnEnrichmentService_EnrichTable_NoTable tests that enrichment succeeds but does nothing
// when the table is not found in the schema (creates minimal context, finds no columns).
func TestColumnEnrichmentService_EnrichTable_NoTable(t *testing.T) {
	projectID := uuid.New()

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{columnsByTable: make(map[string][]*models.SchemaColumn)}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	err := service.EnrichTable(context.Background(), projectID, "nonexistent_table")

	// Verify - should succeed but do nothing (no columns found)
	require.NoError(t, err)

	// Verify LLM was not called
	client := llmFactory.client.(*testColEnrichmentLLMClient)
	assert.Equal(t, 0, client.callCount, "Should not call LLM for nonexistent table")
}

func TestColumnEnrichmentService_EnrichTable_NoColumns(t *testing.T) {
	projectID := uuid.New()

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"empty_table": {}, // No columns
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{},
	}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	err := service.EnrichTable(context.Background(), projectID, "empty_table")

	// Verify - should succeed but do nothing
	require.NoError(t, err)

	// Verify LLM was not called
	client := llmFactory.client.(*testColEnrichmentLLMClient)
	assert.Equal(t, 0, client.callCount, "Should not call LLM for empty table")
}

func TestColumnEnrichmentService_EnrichProject_ContinuesOnFailure(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
	}

	llmResponse := `{
		"columns": [
			{"name": "id", "description": "ID", "semantic_type": "identifier", "role": "identifier", "fk_association": null}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"table1": columns,
			"table2": columns,
			"table3": columns,
		},
	}

	// Create client that permanently fails on second call with retryable error
	client := &testColEnrichmentRetryableFailureClient{
		response: llmResponse,
	}

	llmFactory := &testColEnrichmentLLMFactory{client: client}

	// Use a circuit breaker with high threshold so it doesn't trip during this test
	// The test expects table2 to fail but table1 and table3 to succeed, so we need
	// the circuit to stay closed throughout
	highThresholdCircuit := llm.NewCircuitBreaker(llm.CircuitBreakerConfig{
		Threshold:  10, // High threshold to prevent tripping
		ResetAfter: 30 * time.Second,
	})

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     highThresholdCircuit,
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"table1", "table2", "table3"}, nil)

	// Verify - should fail fast and track failures
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 of 3 tables failed enrichment")
	assert.Equal(t, 2, len(result.TablesEnriched), "Should enrich table1 and table3")
	assert.Equal(t, 1, len(result.TablesFailed), "Should track table2 failure")
	assert.Contains(t, result.TablesFailed, "table2")
}

// testColEnrichmentRetryableFailureClient simulates a client that fails on second call with retryable error
type testColEnrichmentRetryableFailureClient struct {
	response  string
	callCount int
	mu        sync.Mutex
}

func (c *testColEnrichmentRetryableFailureClient) GenerateResponse(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
	c.mu.Lock()
	c.callCount++
	c.mu.Unlock()

	// Fail on all calls for table2 (detect by checking if prompt contains "Table2")
	// This makes the test deterministic regardless of execution order
	// Note: Worker pool goroutines race for semaphore, so execution order is non-deterministic
	// even with MaxConcurrent=1. Using prompt content ensures correct behavior.
	if strings.Contains(prompt, "Table2") || strings.Contains(prompt, "table2") {
		return nil, llm.NewError(llm.ErrorTypeEndpoint, "endpoint error", true, errors.New("endpoint error"))
	}
	return &llm.GenerateResponseResult{Content: c.response}, nil
}

func (c *testColEnrichmentRetryableFailureClient) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (c *testColEnrichmentRetryableFailureClient) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (c *testColEnrichmentRetryableFailureClient) GetModel() string {
	return "test-model"
}

func (c *testColEnrichmentRetryableFailureClient) GetEndpoint() string {
	return "http://test"
}

func (c *testColEnrichmentRetryableFailureClient) Close() error {
	return nil
}

func TestColumnEnrichmentService_EnrichColumnsInChunks_ParallelProcessing(t *testing.T) {
	projectID := uuid.New()

	tableCtx := &TableContext{
		TableName: "large_table",
	}

	// Create 120 columns (will be split into 3 chunks of 50, 50, 20)
	columns := make([]*models.SchemaColumn, 120)
	for i := 0; i < 120; i++ {
		columns[i] = &models.SchemaColumn{
			ColumnName: fmt.Sprintf("col_%d", i+1),
			DataType:   "varchar",
		}
	}

	// Track concurrent execution
	type callInfo struct {
		startTime int64
		endTime   int64
	}
	var callsMu sync.Mutex
	calls := []callInfo{}

	// Create LLM client that tracks concurrent calls and returns all columns
	client := &testColEnrichmentLLMClient{
		generateFunc: func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
			callsMu.Lock()
			callIdx := len(calls)
			calls = append(calls, callInfo{startTime: time.Now().UnixNano()})
			callsMu.Unlock()

			// Simulate LLM processing time to ensure overlap
			time.Sleep(50 * time.Millisecond)

			callsMu.Lock()
			calls[callIdx].endTime = time.Now().UnixNano()
			callsMu.Unlock()

			// Parse which columns are in this chunk by looking at the prompt
			// The prompt format includes "| col_N |" in a markdown table
			enrichments := []string{}
			for i := 1; i <= 120; i++ {
				// Be very specific with the pattern to avoid substring matches
				// Look for column names between pipe delimiters with spaces
				pattern := fmt.Sprintf(" col_%d ", i)
				if strings.Contains(prompt, pattern) {
					enrichments = append(enrichments, fmt.Sprintf(`{
						"name": "col_%d",
						"description": "Column %d",
						"semantic_type": "text",
						"role": "attribute",
						"fk_association": null
					}`, i, i))
				}
			}

			if len(enrichments) == 0 {
				return nil, fmt.Errorf("no columns found in prompt")
			}

			response := `{"columns": [` + strings.Join(enrichments, ",") + `]}`
			return &llm.GenerateResponseResult{Content: response}, nil
		},
	}

	llmFactory := &testColEnrichmentLLMFactory{client: client}

	service := &columnEnrichmentService{
		llmFactory:     llmFactory,
		workerPool:     llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 3}, zap.NewNop()),
		circuitBreaker: llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:         zap.NewNop(),
	}

	// Execute chunked enrichment
	ctx := context.Background()
	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	require.NoError(t, err)

	result, err := service.enrichColumnsInChunks(
		ctx,
		projectID,
		llmClient,
		tableCtx,
		columns,
		make(map[string]string),
		make(map[string][]string),
		50,
	)

	// Verify success
	require.NoError(t, err)
	assert.Equal(t, 120, len(result), "Should enrich all 120 columns")

	// Verify all columns were enriched in order
	for i := 0; i < 120; i++ {
		assert.Equal(t, fmt.Sprintf("col_%d", i+1), result[i].Name, "Column at index %d should be col_%d", i, i+1)
	}

	// Verify parallel execution
	callsMu.Lock()
	assert.Equal(t, 3, len(calls), "Should have made 3 LLM calls for 3 chunks")

	// Check if any calls overlapped in time (indicating parallelism)
	if len(calls) >= 2 {
		// Sort by start time
		sortedCalls := make([]callInfo, len(calls))
		copy(sortedCalls, calls)
		sort.Slice(sortedCalls, func(i, j int) bool {
			return sortedCalls[i].startTime < sortedCalls[j].startTime
		})

		// Check if second call started before first call ended
		overlapDetected := sortedCalls[1].startTime < sortedCalls[0].endTime
		assert.True(t, overlapDetected, "Chunks should be processed in parallel")
	}
	callsMu.Unlock()
}

func TestColumnEnrichmentService_EnrichColumnsInChunks_ChunkFailure(t *testing.T) {
	projectID := uuid.New()

	tableCtx := &TableContext{
		TableName: "large_table",
	}

	// Create 100 columns (will be split into 2 chunks of 50)
	columns := make([]*models.SchemaColumn, 100)
	for i := 0; i < 100; i++ {
		columns[i] = &models.SchemaColumn{
			ColumnName: fmt.Sprintf("col_%d", i+1),
			DataType:   "varchar",
		}
	}

	// Create LLM client that fails on second chunk (consistently, even with retries)
	callCount := 0
	var mu sync.Mutex
	client := &testColEnrichmentLLMClient{
		generateFunc: func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
			mu.Lock()
			callCount++
			mu.Unlock()

			// Detect second chunk by looking for col_51 (which is in the second chunk)
			// This makes it fail consistently regardless of retry attempts
			if strings.Contains(prompt, " col_51 ") {
				return nil, llm.NewError(llm.ErrorTypeAuth, "auth error", false, errors.New("unauthorized"))
			}

			// Return valid response for first chunk
			enrichments := []string{}
			for i := 1; i <= 100; i++ {
				pattern := fmt.Sprintf(" col_%d ", i)
				if strings.Contains(prompt, pattern) {
					enrichments = append(enrichments, fmt.Sprintf(`{
						"name": "col_%d",
						"description": "Column %d",
						"semantic_type": "text",
						"role": "attribute",
						"fk_association": null
					}`, i, i))
				}
			}

			response := `{"columns": [` + strings.Join(enrichments, ",") + `]}`
			return &llm.GenerateResponseResult{Content: response}, nil
		},
	}

	llmFactory := &testColEnrichmentLLMFactory{client: client}

	service := &columnEnrichmentService{
		llmFactory:     llmFactory,
		workerPool:     llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 2}, zap.NewNop()),
		circuitBreaker: llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:         zap.NewNop(),
	}

	// Execute chunked enrichment
	ctx := context.Background()
	llmClient, err := llmFactory.CreateForProject(ctx, projectID)
	require.NoError(t, err)

	result, err := service.enrichColumnsInChunks(
		ctx,
		projectID,
		llmClient,
		tableCtx,
		columns,
		make(map[string]string),
		make(map[string][]string),
		50,
	)

	// Verify failure
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "chunk")
	assert.Contains(t, err.Error(), "failed")
}

// Helper to find a column detail by name
func findColumnDetail(details []models.ColumnDetail, name string) *models.ColumnDetail {
	for i := range details {
		if details[i].Name == name {
			return &details[i]
		}
	}
	return nil
}

// Helper function to join strings
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// TestColumnEnrichmentService_EnrichTable_IntegerEnumInference tests that integer enum
// columns like transaction_state get meaningful labels inferred from column context.
func TestColumnEnrichmentService_EnrichTable_IntegerEnumInference(t *testing.T) {
	projectID := uuid.New()

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint", IsPrimaryKey: true},
		{ColumnName: "transaction_state", DataType: "integer"},
	}

	// LLM response with inferred labels for integer enum values
	// The LLM should infer state progression from the column name context
	llmResponse := `{
		"columns": [
			{
				"name": "id",
				"description": "Unique identifier for the billing transaction",
				"semantic_type": "identifier",
				"role": "identifier",
				"fk_association": null
			},
			{
				"name": "transaction_state",
				"description": "Current state of the billing transaction in its lifecycle",
				"semantic_type": "status",
				"role": "dimension",
				"enum_values": [
					{"value": "1", "label": "Started", "description": "Transaction has been initiated"},
					{"value": "2", "label": "Ended", "description": "Transaction has completed"},
					{"value": "3", "label": "Waiting", "description": "Transaction is waiting for processing"}
				],
				"fk_association": null
			}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"billing_transactions": columns,
		},
	}

	// Track what prompt was sent to the LLM
	var capturedPrompt string
	llmClient := &testColEnrichmentLLMClient{
		generateFunc: func(ctx context.Context, prompt, systemMsg string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
			capturedPrompt = prompt
			return &llm.GenerateResponseResult{Content: llmResponse}, nil
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{client: llmClient}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: &testColEnrichmentColumnMetadataRepo{},
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	err := service.EnrichTable(context.Background(), projectID, "billing_transactions")

	// Verify
	require.NoError(t, err)

	// Verify the prompt includes enhanced enum instructions
	assert.Contains(t, capturedPrompt, "Return as objects")
	assert.Contains(t, capturedPrompt, "Infer labels from column context")
	assert.Contains(t, capturedPrompt, "For integer enums")
	assert.Contains(t, capturedPrompt, "For string enums")

	// Verify column details were saved with proper enum values
	details := ontologyRepo.columnDetails["billing_transactions"]
	require.Equal(t, 2, len(details))

	// Find transaction_state column and verify enum values with inferred labels
	stateCol := findColumnDetail(details, "transaction_state")
	require.NotNil(t, stateCol, "Should find transaction_state column")
	assert.Equal(t, "status", stateCol.SemanticType)
	require.Equal(t, 3, len(stateCol.EnumValues), "Should have 3 enum values")

	// Verify enum values have both value and inferred label
	assert.Equal(t, "1", stateCol.EnumValues[0].Value)
	assert.Equal(t, "Started", stateCol.EnumValues[0].Label)
	assert.Equal(t, "Transaction has been initiated", stateCol.EnumValues[0].Description)

	assert.Equal(t, "2", stateCol.EnumValues[1].Value)
	assert.Equal(t, "Ended", stateCol.EnumValues[1].Label)

	assert.Equal(t, "3", stateCol.EnumValues[2].Value)
	assert.Equal(t, "Waiting", stateCol.EnumValues[2].Label)
}

// TestColumnEnrichmentService_buildColumnEnrichmentPrompt_EnumInstructions verifies
// that the prompt includes proper instructions for inferring enum labels.
func TestColumnEnrichmentService_buildColumnEnrichmentPrompt_EnumInstructions(t *testing.T) {
	service := &columnEnrichmentService{
		circuitBreaker: llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:         zap.NewNop(),
	}

	tableCtx := &TableContext{
		TableName: "test_table",
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "status", DataType: "integer"},
	}

	enumSamples := map[string][]string{
		"status": {"1", "2", "3"},
	}

	prompt := service.buildColumnEnrichmentPrompt(tableCtx, columns, nil, enumSamples)

	// Verify enhanced enum instructions are present
	assert.Contains(t, prompt, "enum_values")
	assert.Contains(t, prompt, "Return as objects")
	assert.Contains(t, prompt, "Infer labels from column context")
	assert.Contains(t, prompt, "For integer enums")
	assert.Contains(t, prompt, "For string enums")
	assert.Contains(t, prompt, "Include description if you can infer the business meaning")

	// Verify response format example shows object structure
	assert.Contains(t, prompt, `"value":`)
	assert.Contains(t, prompt, `"label":`)
}

// TestColumnEnrichmentService_mergeEnumDefinitions tests that project-level enum
// definitions are correctly merged with sampled values.
func TestColumnEnrichmentService_mergeEnumDefinitions(t *testing.T) {
	service := &columnEnrichmentService{
		logger: zap.NewNop(),
	}

	tests := []struct {
		name           string
		tableName      string
		columnName     string
		sampledValues  []string
		enumDefs       []models.EnumDefinition
		expectedEnums  []models.EnumValue
		expectOverride bool // If true, all values should have descriptions
	}{
		{
			name:          "matching_exact_table_definition",
			tableName:     "billing_transactions",
			columnName:    "transaction_state",
			sampledValues: []string{"1", "2", "3"},
			enumDefs: []models.EnumDefinition{
				{
					Table:  "billing_transactions",
					Column: "transaction_state",
					Values: map[string]string{
						"1": "STARTED - Transaction started",
						"2": "ENDED - Transaction ended",
						"3": "WAITING - Awaiting chargeback period",
					},
				},
			},
			expectedEnums: []models.EnumValue{
				{Value: "1", Label: "STARTED", Description: "Transaction started"},
				{Value: "2", Label: "ENDED", Description: "Transaction ended"},
				{Value: "3", Label: "WAITING", Description: "Awaiting chargeback period"},
			},
			expectOverride: true,
		},
		{
			name:          "matching_wildcard_table_definition",
			tableName:     "offers",
			columnName:    "offer_type",
			sampledValues: []string{"1", "2"},
			enumDefs: []models.EnumDefinition{
				{
					Table:  "*",
					Column: "offer_type",
					Values: map[string]string{
						"1": "FREE - Free Engagement",
						"2": "PAID - Preauthorized per-minute",
					},
				},
			},
			expectedEnums: []models.EnumValue{
				{Value: "1", Label: "FREE", Description: "Free Engagement"},
				{Value: "2", Label: "PAID", Description: "Preauthorized per-minute"},
			},
			expectOverride: true,
		},
		{
			name:          "no_matching_definition_falls_back",
			tableName:     "users",
			columnName:    "status",
			sampledValues: []string{"active", "inactive"},
			enumDefs: []models.EnumDefinition{
				{
					Table:  "billing_transactions",
					Column: "transaction_state",
					Values: map[string]string{"1": "Started"},
				},
			},
			expectedEnums: []models.EnumValue{
				{Value: "active"},
				{Value: "inactive"},
			},
			expectOverride: false,
		},
		{
			name:          "partial_match_still_applies_definition",
			tableName:     "billing_transactions",
			columnName:    "transaction_state",
			sampledValues: []string{"1", "2", "99"}, // 99 not in definition
			enumDefs: []models.EnumDefinition{
				{
					Table:  "billing_transactions",
					Column: "transaction_state",
					Values: map[string]string{
						"1": "STARTED - Transaction started",
						"2": "ENDED - Transaction ended",
					},
				},
			},
			expectedEnums: []models.EnumValue{
				{Value: "1", Label: "STARTED", Description: "Transaction started"},
				{Value: "2", Label: "ENDED", Description: "Transaction ended"},
				{Value: "99"}, // No description for unknown value
			},
			expectOverride: true,
		},
		{
			name:          "exact_table_takes_precedence_over_wildcard",
			tableName:     "billing_transactions",
			columnName:    "offer_type",
			sampledValues: []string{"1"},
			enumDefs: []models.EnumDefinition{
				{
					Table:  "billing_transactions",
					Column: "offer_type",
					Values: map[string]string{
						"1": "EXACT - From exact table match",
					},
				},
				{
					Table:  "*",
					Column: "offer_type",
					Values: map[string]string{
						"1": "WILDCARD - From wildcard",
					},
				},
			},
			expectedEnums: []models.EnumValue{
				{Value: "1", Label: "EXACT", Description: "From exact table match"},
			},
			expectOverride: true,
		},
		{
			name:          "description_without_label_separator",
			tableName:     "orders",
			columnName:    "status",
			sampledValues: []string{"1"},
			enumDefs: []models.EnumDefinition{
				{
					Table:  "orders",
					Column: "status",
					Values: map[string]string{
						"1": "Just a plain description without separator",
					},
				},
			},
			expectedEnums: []models.EnumValue{
				{Value: "1", Description: "Just a plain description without separator"},
			},
			expectOverride: true,
		},
		{
			name:          "empty_sampled_values",
			tableName:     "billing_transactions",
			columnName:    "transaction_state",
			sampledValues: []string{},
			enumDefs: []models.EnumDefinition{
				{
					Table:  "billing_transactions",
					Column: "transaction_state",
					Values: map[string]string{"1": "STARTED - Transaction started"},
				},
			},
			expectedEnums:  nil,
			expectOverride: false,
		},
		{
			name:          "empty_enum_definitions",
			tableName:     "billing_transactions",
			columnName:    "transaction_state",
			sampledValues: []string{"1", "2"},
			enumDefs:      []models.EnumDefinition{},
			expectedEnums: []models.EnumValue{
				{Value: "1"},
				{Value: "2"},
			},
			expectOverride: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.mergeEnumDefinitions(tt.tableName, tt.columnName, tt.sampledValues, tt.enumDefs)

			if tt.expectedEnums == nil {
				assert.Nil(t, result)
				return
			}

			require.Equal(t, len(tt.expectedEnums), len(result), "enum value count mismatch")

			for i, expected := range tt.expectedEnums {
				assert.Equal(t, expected.Value, result[i].Value, "value mismatch at index %d", i)
				assert.Equal(t, expected.Label, result[i].Label, "label mismatch at index %d", i)
				assert.Equal(t, expected.Description, result[i].Description, "description mismatch at index %d", i)
			}
		})
	}
}

// TestColumnEnrichmentService_splitEnumDescription tests the helper function
// that parses "LABEL - Description" format.
func TestColumnEnrichmentService_splitEnumDescription(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"STARTED - Transaction started", []string{"STARTED", "Transaction started"}},
		{"FREE - Free Engagement", []string{"FREE", "Free Engagement"}},
		{"Just a description", nil},
		{" - Empty label", nil},
		{"Empty description - ", nil},
		{"", nil},
		{"MULTI - Part - Description", []string{"MULTI", "Part - Description"}},
		{"A-B - Hyphenated label", []string{"A-B", "Hyphenated label"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitEnumDescription(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected[0], result[0], "label mismatch")
				assert.Equal(t, tt.expected[1], result[1], "description mismatch")
			}
		})
	}
}

// TestColumnEnrichmentService_toEnumValues tests the helper function
// that converts string slices to EnumValue slices.
func TestColumnEnrichmentService_toEnumValues(t *testing.T) {
	tests := []struct {
		name   string
		input  []string
		expect []models.EnumValue
	}{
		{
			name:   "normal_values",
			input:  []string{"active", "inactive", "pending"},
			expect: []models.EnumValue{{Value: "active"}, {Value: "inactive"}, {Value: "pending"}},
		},
		{
			name:   "empty_slice",
			input:  []string{},
			expect: nil,
		},
		{
			name:   "nil_slice",
			input:  nil,
			expect: nil,
		},
		{
			name:   "single_value",
			input:  []string{"1"},
			expect: []models.EnumValue{{Value: "1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toEnumValues(tt.input)
			if tt.expect == nil {
				assert.Nil(t, result)
			} else {
				require.Equal(t, len(tt.expect), len(result))
				for i := range tt.expect {
					assert.Equal(t, tt.expect[i].Value, result[i].Value)
				}
			}
		})
	}
}

// TestColumnEnrichmentService_convertToColumnDetails_WithEnumDefinitions tests that
// enum definitions are properly merged during column detail conversion.
func TestColumnEnrichmentService_convertToColumnDetails_WithEnumDefinitions(t *testing.T) {
	service := &columnEnrichmentService{
		logger: zap.NewNop(),
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint", IsPrimaryKey: true},
		{ColumnName: "transaction_state", DataType: "integer"},
		{ColumnName: "status", DataType: "varchar"},
	}

	// LLM enrichments - transaction_state has inferred but incorrect enum values
	enrichments := []columnEnrichment{
		{
			Name:         "id",
			Description:  "Unique identifier",
			SemanticType: "identifier",
			Role:         "identifier",
		},
		{
			Name:         "transaction_state",
			Description:  "State of the transaction",
			SemanticType: "status",
			Role:         "dimension",
			EnumValues: []models.EnumValue{
				{Value: "1", Label: "State 1"}, // Wrong label - LLM guessed
				{Value: "2", Label: "State 2"},
			},
		},
		{
			Name:         "status",
			Description:  "User status",
			SemanticType: "status",
			Role:         "dimension",
			EnumValues: []models.EnumValue{
				{Value: "active", Label: "Active"},
			},
		},
	}

	enumSamples := map[string][]string{
		"transaction_state": {"1", "2"},
		"status":            {"active"},
	}

	// Project enum definitions with correct values
	enumDefs := []models.EnumDefinition{
		{
			Table:  "billing_transactions",
			Column: "transaction_state",
			Values: map[string]string{
				"1": "STARTED - Transaction started",
				"2": "ENDED - Transaction ended",
			},
		},
	}

	fkInfo := map[string]string{}
	enumDistributions := map[string]*datasource.EnumDistributionResult{}

	details := service.convertToColumnDetails("billing_transactions", enrichments, columns, fkInfo, enumSamples, enumDefs, enumDistributions, make(map[uuid.UUID]*models.ColumnMetadata))

	require.Equal(t, 3, len(details))

	// Check transaction_state - should have enum definitions merged (overriding LLM inference)
	stateCol := findColumnDetail(details, "transaction_state")
	require.NotNil(t, stateCol)
	require.Equal(t, 2, len(stateCol.EnumValues))
	assert.Equal(t, "1", stateCol.EnumValues[0].Value)
	assert.Equal(t, "STARTED", stateCol.EnumValues[0].Label)
	assert.Equal(t, "Transaction started", stateCol.EnumValues[0].Description)
	assert.Equal(t, "2", stateCol.EnumValues[1].Value)
	assert.Equal(t, "ENDED", stateCol.EnumValues[1].Label)

	// Check status - should keep LLM values (no enum definition for this column)
	statusCol := findColumnDetail(details, "status")
	require.NotNil(t, statusCol)
	require.Equal(t, 1, len(statusCol.EnumValues))
	assert.Equal(t, "active", statusCol.EnumValues[0].Value)
	assert.Equal(t, "Active", statusCol.EnumValues[0].Label)
}

func TestColumnEnrichmentResponse_QuestionsDeserialization(t *testing.T) {
	// Direct test of columnEnrichmentResponse JSON parsing

	responseJSON := `{
		"columns": [
			{
				"name": "email",
				"description": "User email address",
				"semantic_type": "email",
				"role": "identifier"
			}
		],
		"questions": [
			{
				"category": "terminology",
				"priority": 2,
				"question": "What does 'tik' mean in tiks_count?",
				"context": "Column name is unclear."
			}
		]
	}`

	response, err := llm.ParseJSONResponse[columnEnrichmentResponse](responseJSON)

	require.NoError(t, err)
	require.Len(t, response.Columns, 1)
	require.Len(t, response.Questions, 1)

	// Verify column
	assert.Equal(t, "email", response.Columns[0].Name)
	assert.Equal(t, "email", response.Columns[0].SemanticType)

	// Verify question
	assert.Equal(t, "terminology", response.Questions[0].Category)
	assert.Equal(t, 2, response.Questions[0].Priority)
	assert.Equal(t, "What does 'tik' mean in tiks_count?", response.Questions[0].Question)
	assert.Equal(t, "Column name is unclear.", response.Questions[0].Context)
}

func TestColumnEnrichmentResponse_NoQuestions(t *testing.T) {
	// Test that parsing succeeds when no questions are present

	responseJSON := `{
		"columns": [
			{
				"name": "id",
				"description": "Primary key",
				"semantic_type": "identifier",
				"role": "identifier"
			}
		]
	}`

	response, err := llm.ParseJSONResponse[columnEnrichmentResponse](responseJSON)

	require.NoError(t, err)
	require.Len(t, response.Columns, 1)
	assert.Empty(t, response.Questions, "Questions should be empty when not present")
}

func TestColumnEnrichmentResponse_MultipleQuestions(t *testing.T) {
	// Test that multiple questions of different categories are parsed correctly

	responseJSON := `{
		"columns": [
			{
				"name": "status",
				"description": "Status field",
				"semantic_type": "status",
				"role": "dimension"
			}
		],
		"questions": [
			{
				"category": "enumeration",
				"priority": 1,
				"question": "What do status values 'A', 'P', 'C' represent?",
				"context": "Column status has cryptic single-letter values."
			},
			{
				"category": "data_quality",
				"priority": 3,
				"question": "Is 85% NULL in phone column expected?",
				"context": "Column phone has very high null rate."
			},
			{
				"category": "temporal",
				"priority": 2,
				"question": "Does deleted_at=NULL mean active records?",
				"context": "Soft delete pattern detected."
			}
		]
	}`

	response, err := llm.ParseJSONResponse[columnEnrichmentResponse](responseJSON)

	require.NoError(t, err)
	require.Len(t, response.Columns, 1)
	require.Len(t, response.Questions, 3)

	// Verify question categories
	assert.Equal(t, "enumeration", response.Questions[0].Category)
	assert.Equal(t, 1, response.Questions[0].Priority)

	assert.Equal(t, "data_quality", response.Questions[1].Category)
	assert.Equal(t, 3, response.Questions[1].Priority)

	assert.Equal(t, "temporal", response.Questions[2].Category)
	assert.Equal(t, 2, response.Questions[2].Priority)
}

// TestApplyEnumDistributions verifies that distribution metadata is properly applied to enum values.
func TestApplyEnumDistributions(t *testing.T) {
	// Create enum values from LLM enrichment (without distribution data)
	enumValues := []models.EnumValue{
		{Value: "pending", Label: "Pending"},
		{Value: "processing", Label: "Processing"},
		{Value: "completed", Label: "Completed"},
		{Value: "failed", Label: "Failed"},
	}

	// Create distribution data from database analysis
	dist := &datasource.EnumDistributionResult{
		ColumnName:    "status",
		TotalRows:     1000,
		DistinctCount: 4,
		NullCount:     10,
		Distributions: []datasource.EnumValueDistribution{
			{
				Value:                 "completed",
				Count:                 500,
				Percentage:            50.0,
				CompletionRate:        100.0,
				IsLikelyTerminalState: true,
			},
			{
				Value:                "pending",
				Count:                300,
				Percentage:           30.0,
				CompletionRate:       0.0,
				IsLikelyInitialState: true,
			},
			{
				Value:              "processing",
				Count:              180,
				Percentage:         18.0,
				CompletionRate:     0.0,
				IsLikelyErrorState: false,
			},
			{
				Value:              "failed",
				Count:              10,
				Percentage:         1.0,
				CompletionRate:     0.0,
				IsLikelyErrorState: true,
			},
		},
		HasStateSemantics: true,
	}

	// Apply distributions
	result := applyEnumDistributions(enumValues, dist)

	// Verify results
	require.Len(t, result, 4)

	// Find each value and verify its distribution data
	pendingVal := findEnumValue(result, "pending")
	require.NotNil(t, pendingVal)
	assert.Equal(t, int64(300), *pendingVal.Count)
	assert.Equal(t, 30.0, *pendingVal.Percentage)
	assert.True(t, *pendingVal.IsLikelyInitialState)
	assert.Nil(t, pendingVal.IsLikelyTerminalState)

	completedVal := findEnumValue(result, "completed")
	require.NotNil(t, completedVal)
	assert.Equal(t, int64(500), *completedVal.Count)
	assert.Equal(t, 50.0, *completedVal.Percentage)
	assert.True(t, *completedVal.IsLikelyTerminalState)
	assert.Nil(t, completedVal.IsLikelyInitialState)

	failedVal := findEnumValue(result, "failed")
	require.NotNil(t, failedVal)
	assert.Equal(t, int64(10), *failedVal.Count)
	assert.True(t, *failedVal.IsLikelyErrorState)
}

// TestApplyEnumDistributions_NoDistributionData verifies graceful handling when no distribution exists.
func TestApplyEnumDistributions_NoDistributionData(t *testing.T) {
	enumValues := []models.EnumValue{
		{Value: "active", Label: "Active"},
		{Value: "inactive", Label: "Inactive"},
	}

	// No distribution data
	result := applyEnumDistributions(enumValues, nil)

	// Should return the original values unchanged
	require.Len(t, result, 2)
	assert.Nil(t, result[0].Count)
	assert.Nil(t, result[0].Percentage)
}

// TestApplyEnumDistributions_PartialMatch verifies handling when not all values have distribution data.
func TestApplyEnumDistributions_PartialMatch(t *testing.T) {
	enumValues := []models.EnumValue{
		{Value: "active", Label: "Active"},
		{Value: "inactive", Label: "Inactive"},
		{Value: "unknown", Label: "Unknown"}, // This won't have distribution data
	}

	dist := &datasource.EnumDistributionResult{
		Distributions: []datasource.EnumValueDistribution{
			{Value: "active", Count: 800, Percentage: 80.0},
			{Value: "inactive", Count: 200, Percentage: 20.0},
			// "unknown" is not in distribution (maybe it was added after analysis)
		},
	}

	result := applyEnumDistributions(enumValues, dist)

	require.Len(t, result, 3)

	activeVal := findEnumValue(result, "active")
	assert.Equal(t, int64(800), *activeVal.Count)

	unknownVal := findEnumValue(result, "unknown")
	assert.Nil(t, unknownVal.Count, "Values not in distribution should not have count set")
}

// TestFindCompletionTimestampColumn verifies completion timestamp column detection via ColumnMetadata.
func TestFindCompletionTimestampColumn(t *testing.T) {
	completedAtID := uuid.New()
	finishedAtID := uuid.New()
	createdAtID := uuid.New()
	updatedAtID := uuid.New()

	tests := []struct {
		name               string
		columns            []*models.SchemaColumn
		metadataByColumnID map[uuid.UUID]*models.ColumnMetadata
		expected           string
	}{
		{
			name: "finds column with completion purpose in metadata",
			columns: []*models.SchemaColumn{
				{ColumnName: "id", DataType: "integer"},
				{ColumnName: "status", DataType: "varchar"},
				{ID: completedAtID, ColumnName: "completed_at", DataType: "timestamp"},
				{ID: createdAtID, ColumnName: "created_at", DataType: "timestamp"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				completedAtID: {
					SchemaColumnID: completedAtID,
					Features: models.ColumnMetadataFeatures{
						TimestampFeatures: &models.TimestampFeatures{
							TimestampPurpose: models.TimestampPurposeCompletion,
						},
					},
				},
				createdAtID: {
					SchemaColumnID: createdAtID,
					Features: models.ColumnMetadataFeatures{
						TimestampFeatures: &models.TimestampFeatures{
							TimestampPurpose: models.TimestampPurposeAuditCreated,
						},
					},
				},
			},
			expected: "completed_at",
		},
		{
			name: "finds finished_at with completion purpose",
			columns: []*models.SchemaColumn{
				{ColumnName: "id", DataType: "integer"},
				{ID: finishedAtID, ColumnName: "finished_at", DataType: "timestamp with time zone"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				finishedAtID: {
					SchemaColumnID: finishedAtID,
					Features: models.ColumnMetadataFeatures{
						TimestampFeatures: &models.TimestampFeatures{
							TimestampPurpose: models.TimestampPurposeCompletion,
						},
					},
				},
			},
			expected: "finished_at",
		},
		{
			name: "returns empty when no completion metadata",
			columns: []*models.SchemaColumn{
				{ColumnName: "id", DataType: "integer"},
				{ID: createdAtID, ColumnName: "created_at", DataType: "timestamp"},
				{ID: updatedAtID, ColumnName: "updated_at", DataType: "timestamp"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				createdAtID: {
					SchemaColumnID: createdAtID,
					Features: models.ColumnMetadataFeatures{
						TimestampFeatures: &models.TimestampFeatures{
							TimestampPurpose: models.TimestampPurposeAuditCreated,
						},
					},
				},
				updatedAtID: {
					SchemaColumnID: updatedAtID,
					Features: models.ColumnMetadataFeatures{
						TimestampFeatures: &models.TimestampFeatures{
							TimestampPurpose: models.TimestampPurposeAuditUpdated,
						},
					},
				},
			},
			expected: "",
		},
		{
			name: "returns empty when no metadata available",
			columns: []*models.SchemaColumn{
				{ColumnName: "completed_at", DataType: "timestamp"},
			},
			metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata),
			expected:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findCompletionTimestampColumn(tt.columns, tt.metadataByColumnID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsTimestampType verifies timestamp type detection.
func TestIsTimestampType(t *testing.T) {
	tests := []struct {
		dataType string
		expected bool
	}{
		{"timestamp", true},
		{"timestamp with time zone", true},
		{"timestamp without time zone", true},
		{"TIMESTAMP", true},
		{"datetime", true},
		{"datetime2", true},
		{"date", true},
		{"varchar", false},
		{"integer", false},
		{"bigint", false},
		{"text", false},
	}

	for _, tt := range tests {
		t.Run(tt.dataType, func(t *testing.T) {
			assert.Equal(t, tt.expected, isTimestampType(tt.dataType))
		})
	}
}

// Helper function for tests
func findEnumValue(values []models.EnumValue, target string) *models.EnumValue {
	for i := range values {
		if values[i].Value == target {
			return &values[i]
		}
	}
	return nil
}

// TestFilterColumnsForLLM tests the logic for separating columns that need LLM enrichment
// from those with high-confidence ColumnMetadata.
func TestFilterColumnsForLLM(t *testing.T) {
	service := &columnEnrichmentService{
		logger: zap.NewNop(),
	}

	ptrFloat := func(f float64) *float64 { return &f }
	ptrStr := func(s string) *string { return &s }

	type testCase struct {
		name                     string
		columns                  []*models.SchemaColumn
		metadataByColumnID       map[uuid.UUID]*models.ColumnMetadata
		expectedNeedLLM          int
		expectedSynthetic        int
		expectedSyntheticColumns []string
	}

	// Pre-generate IDs for columns that need metadata
	idColID := uuid.New()
	statusColID := uuid.New()
	valueColID := uuid.New()
	statusEnumColID := uuid.New()
	hostIDColID := uuid.New()

	tests := []testCase{
		{
			name: "all columns need LLM - no features",
			columns: []*models.SchemaColumn{
				{ColumnName: "id", DataType: "bigint"},
				{ColumnName: "name", DataType: "varchar"},
			},
			metadataByColumnID: make(map[uuid.UUID]*models.ColumnMetadata),
			expectedNeedLLM:    2,
			expectedSynthetic:  0,
		},
		{
			name: "high confidence column skips LLM",
			columns: []*models.SchemaColumn{
				{ID: idColID, ColumnName: "id", DataType: "bigint"},
				{ColumnName: "name", DataType: "varchar"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				idColID: {
					SchemaColumnID: idColID,
					Confidence:     ptrFloat(0.95),
					Description:    ptrStr("Unique identifier for the user"),
					SemanticType:   ptrStr("identifier"),
					Role:           ptrStr("primary_key"),
				},
			},
			expectedNeedLLM:          1,
			expectedSynthetic:        1,
			expectedSyntheticColumns: []string{"id"},
		},
		{
			name: "low confidence column needs LLM",
			columns: []*models.SchemaColumn{
				{ID: statusColID, ColumnName: "status", DataType: "varchar"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				statusColID: {
					SchemaColumnID: statusColID,
					Confidence:     ptrFloat(0.6), // Below threshold
					Description:    ptrStr("Status value"),
					SemanticType:   ptrStr("enum"),
					Role:           ptrStr("dimension"),
				},
			},
			expectedNeedLLM:   1,
			expectedSynthetic: 0,
		},
		{
			name: "high confidence but missing description needs LLM",
			columns: []*models.SchemaColumn{
				{ID: valueColID, ColumnName: "value", DataType: "numeric"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				valueColID: {
					SchemaColumnID: valueColID,
					Confidence:     ptrFloat(0.95),
					Description:    ptrStr(""), // Empty description
					SemanticType:   ptrStr("measure"),
					Role:           ptrStr("measure"),
				},
			},
			expectedNeedLLM:   1,
			expectedSynthetic: 0,
		},
		{
			name: "column with enum features copies enum values",
			columns: []*models.SchemaColumn{
				{ID: statusEnumColID, ColumnName: "status", DataType: "varchar"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				statusEnumColID: {
					SchemaColumnID: statusEnumColID,
					Confidence:     ptrFloat(0.95),
					Description:    ptrStr("Order status"),
					SemanticType:   ptrStr("status"),
					Role:           ptrStr("dimension"),
					Features: models.ColumnMetadataFeatures{
						EnumFeatures: &models.EnumFeatures{
							IsStateMachine: true,
							Values: []models.ColumnEnumValue{
								{Value: "pending", Label: "Pending", Category: "initial", Count: 100, Percentage: 25.0},
								{Value: "completed", Label: "Completed", Category: "terminal_success", Count: 300, Percentage: 75.0},
							},
						},
					},
				},
			},
			expectedNeedLLM:          0,
			expectedSynthetic:        1,
			expectedSyntheticColumns: []string{"status"},
		},
		{
			name: "column with identifier features copies FK association",
			columns: []*models.SchemaColumn{
				{ID: hostIDColID, ColumnName: "host_id", DataType: "uuid"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				hostIDColID: {
					SchemaColumnID: hostIDColID,
					Confidence:     ptrFloat(0.95),
					Description:    ptrStr("Reference to the host user"),
					SemanticType:   ptrStr("foreign_key"),
					Role:           ptrStr("foreign_key"),
					Features: models.ColumnMetadataFeatures{
						IdentifierFeatures: &models.IdentifierFeatures{
							IdentifierType:   "foreign_key",
							EntityReferenced: "host",
							FKTargetTable:    "users",
							FKTargetColumn:   "id",
						},
					},
				},
			},
			expectedNeedLLM:          0,
			expectedSynthetic:        1,
			expectedSyntheticColumns: []string{"host_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			needLLM, synthetic := service.filterColumnsForLLM(tt.columns, tt.metadataByColumnID)

			assert.Equal(t, tt.expectedNeedLLM, len(needLLM), "columns needing LLM count")
			assert.Equal(t, tt.expectedSynthetic, len(synthetic), "synthetic enrichment count")

			// Verify synthetic column names
			for i, colName := range tt.expectedSyntheticColumns {
				assert.Equal(t, colName, synthetic[i].Name)
			}

			// Verify enum values are copied for enum column test
			if tt.name == "column with enum features copies enum values" && len(synthetic) > 0 {
				assert.Equal(t, 2, len(synthetic[0].EnumValues))
				assert.Equal(t, "pending", synthetic[0].EnumValues[0].Value)
				assert.Equal(t, "Pending", synthetic[0].EnumValues[0].Label)
			}

			// Verify FK association is copied
			if tt.name == "column with identifier features copies FK association" && len(synthetic) > 0 {
				assert.NotNil(t, synthetic[0].FKAssociation)
				assert.Equal(t, "host", *synthetic[0].FKAssociation)
			}
		})
	}
}

// TestConvertToColumnDetails_ColumnFeaturesMerge tests that ColumnMetadata features are properly
// merged into ColumnDetail output.
func TestConvertToColumnDetails_ColumnFeaturesMerge(t *testing.T) {
	service := &columnEnrichmentService{
		logger: zap.NewNop(),
	}

	ptrFloat := func(f float64) *float64 { return &f }
	ptrStr := func(s string) *string { return &s }

	deletedAtColID := uuid.New()
	statusColID := uuid.New()
	visitorIDColID := uuid.New()

	tests := []struct {
		name                 string
		columns              []*models.SchemaColumn
		metadataByColumnID   map[uuid.UUID]*models.ColumnMetadata
		enrichments          []columnEnrichment
		fkInfo               map[string]string
		expectedDescription  string
		expectedSemanticType string
		expectedRole         string
		expectedFKAssoc      string
		expectedEnumCount    int
	}{
		{
			name: "ColumnMetadata SemanticType takes precedence over LLM",
			columns: []*models.SchemaColumn{
				{ID: deletedAtColID, ColumnName: "deleted_at", DataType: "timestamp"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				deletedAtColID: {
					SchemaColumnID: deletedAtColID,
					Confidence:     ptrFloat(0.95),
					Description:    ptrStr("Soft delete timestamp"),
					SemanticType:   ptrStr("soft_delete"),
					Role:           ptrStr("attribute"),
					Features: models.ColumnMetadataFeatures{
						TimestampFeatures: &models.TimestampFeatures{
							IsSoftDelete:     true,
							TimestampPurpose: "soft_delete",
						},
					},
				},
			},
			enrichments: []columnEnrichment{
				{
					Name:         "deleted_at",
					Description:  "When the record was deleted",
					SemanticType: "timestamp_utc", // LLM says timestamp_utc
					Role:         "attribute",
				},
			},
			fkInfo:               make(map[string]string),
			expectedDescription:  "When the record was deleted", // LLM desc used when Features desc empty
			expectedSemanticType: "soft_delete",                 // ColumnMetadata takes precedence
			expectedRole:         "attribute",
		},
		{
			name: "EnumFeatures values are copied to EnumValues",
			columns: []*models.SchemaColumn{
				{ID: statusColID, ColumnName: "status", DataType: "varchar"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				statusColID: {
					SchemaColumnID: statusColID,
					Confidence:     ptrFloat(0.95),
					Description:    ptrStr("Order status"),
					SemanticType:   ptrStr("status"),
					Role:           ptrStr("dimension"),
					Features: models.ColumnMetadataFeatures{
						EnumFeatures: &models.EnumFeatures{
							Values: []models.ColumnEnumValue{
								{Value: "pending", Label: "Pending", Category: "initial", Count: 10, Percentage: 20.0},
								{Value: "active", Label: "Active", Category: "in_progress", Count: 30, Percentage: 60.0},
								{Value: "done", Label: "Done", Category: "terminal_success", Count: 10, Percentage: 20.0},
							},
						},
					},
				},
			},
			enrichments: []columnEnrichment{
				{
					Name:         "status",
					Description:  "LLM description",
					SemanticType: "enum",
					Role:         "dimension",
				},
			},
			fkInfo:               make(map[string]string),
			expectedDescription:  "LLM description",
			expectedSemanticType: "status",
			expectedRole:         "dimension",
			expectedEnumCount:    3,
		},
		{
			name: "IdentifierFeatures EntityReferenced becomes FKAssociation",
			columns: []*models.SchemaColumn{
				{ID: visitorIDColID, ColumnName: "visitor_id", DataType: "uuid"},
			},
			metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
				visitorIDColID: {
					SchemaColumnID: visitorIDColID,
					Confidence:     ptrFloat(0.95),
					Description:    ptrStr("Reference to visiting user"),
					SemanticType:   ptrStr("foreign_key"),
					Role:           ptrStr("foreign_key"),
					Features: models.ColumnMetadataFeatures{
						IdentifierFeatures: &models.IdentifierFeatures{
							EntityReferenced: "visitor",
							FKTargetTable:    "users",
						},
					},
				},
			},
			enrichments: []columnEnrichment{
				{
					Name:         "visitor_id",
					Description:  "LLM FK description",
					SemanticType: "identifier",
					Role:         "identifier",
					// LLM didn't provide FKAssociation
				},
			},
			fkInfo:               make(map[string]string),
			expectedDescription:  "LLM FK description",
			expectedSemanticType: "foreign_key", // ColumnMetadata takes precedence
			expectedRole:         "foreign_key",
			expectedFKAssoc:      "visitor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			details := service.convertToColumnDetails(
				"test_table",
				tt.enrichments,
				tt.columns,
				tt.fkInfo,
				nil, // enumSamples
				nil, // enumDefs
				nil, // enumDistributions
				tt.metadataByColumnID,
			)

			require.Equal(t, 1, len(details))
			detail := details[0]

			assert.Equal(t, tt.expectedDescription, detail.Description, "description")
			assert.Equal(t, tt.expectedSemanticType, detail.SemanticType, "semantic_type")
			assert.Equal(t, tt.expectedRole, detail.Role, "role")

			if tt.expectedFKAssoc != "" {
				assert.Equal(t, tt.expectedFKAssoc, detail.FKAssociation, "fk_association")
			}

			if tt.expectedEnumCount > 0 {
				assert.Equal(t, tt.expectedEnumCount, len(detail.EnumValues), "enum values count")
			}
		})
	}
}

// TestEnrichProject_SkipsLLMForHighConfidenceColumns verifies that the service
// skips LLM calls for columns with high-confidence ColumnMetadata.
func TestEnrichProject_SkipsLLMForHighConfidenceColumns(t *testing.T) {
	projectID := uuid.New()

	ptrFloat := func(f float64) *float64 { return &f }
	ptrStr := func(s string) *string { return &s }

	idColID := uuid.New()

	// Two columns: one with high-confidence metadata, one without
	columns := []*models.SchemaColumn{
		{
			ID:           idColID,
			ColumnName:   "id",
			DataType:     "bigint",
			IsPrimaryKey: true,
		},
		{
			// This column has no metadata, so needs LLM
			ColumnName: "email",
			DataType:   "varchar",
		},
	}

	// Build column metadata for the high-confidence column
	colMetadataRepo := &testColEnrichmentColumnMetadataRepo{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			idColID: {
				SchemaColumnID: idColID,
				Confidence:     ptrFloat(0.95),
				Description:    ptrStr("Unique user identifier"),
				SemanticType:   ptrStr("identifier"),
				Role:           ptrStr("primary_key"),
			},
		},
	}

	// LLM response only contains the email column (id is skipped)
	llmResponse := `{
		"columns": [
			{
				"name": "email",
				"description": "User's email address",
				"semantic_type": "email",
				"role": "attribute",
				"fk_association": null
			}
		]
	}`

	// Setup service
	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": columns,
		},
	}

	llmClient := &testColEnrichmentLLMClient{
		response: llmResponse,
	}
	llmFactory := &testColEnrichmentLLMFactory{client: llmClient}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: colMetadataRepo,
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"users"}, nil)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.TablesEnriched))
	assert.Equal(t, 0, len(result.TablesFailed))

	// Verify LLM was called once (for email column only)
	assert.Equal(t, 1, llmClient.callCount)

	// Verify column details were saved for both columns
	details := ontologyRepo.columnDetails["users"]
	require.Equal(t, 2, len(details))

	// Find each column in details
	var idDetail, emailDetail *models.ColumnDetail
	for i := range details {
		if details[i].Name == "id" {
			idDetail = &details[i]
		} else if details[i].Name == "email" {
			emailDetail = &details[i]
		}
	}

	require.NotNil(t, idDetail, "id column should be in details")
	require.NotNil(t, emailDetail, "email column should be in details")

	// id column should have ColumnFeatures data (from synthetic enrichment)
	assert.Equal(t, "Unique user identifier", idDetail.Description)
	assert.Equal(t, "identifier", idDetail.SemanticType)
	assert.Equal(t, "primary_key", idDetail.Role)

	// email column should have LLM-provided data
	assert.Equal(t, "User's email address", emailDetail.Description)
	assert.Equal(t, "email", emailDetail.SemanticType)
	assert.Equal(t, "attribute", emailDetail.Role)
}

// TestEnrichProject_AllColumnsHighConfidence verifies that no LLM calls are made
// when all columns have high-confidence ColumnMetadata.
func TestEnrichProject_AllColumnsHighConfidence(t *testing.T) {
	projectID := uuid.New()

	ptrFloat := func(f float64) *float64 { return &f }
	ptrStr := func(s string) *string { return &s }

	idColID := uuid.New()
	emailColID := uuid.New()

	// All columns have high-confidence metadata
	columns := []*models.SchemaColumn{
		{
			ID:           idColID,
			ColumnName:   "id",
			DataType:     "bigint",
			IsPrimaryKey: true,
		},
		{
			ID:         emailColID,
			ColumnName: "email",
			DataType:   "varchar",
		},
	}

	colMetadataRepo := &testColEnrichmentColumnMetadataRepo{
		metadataByColumnID: map[uuid.UUID]*models.ColumnMetadata{
			idColID: {
				SchemaColumnID: idColID,
				Confidence:     ptrFloat(0.95),
				Description:    ptrStr("Unique user identifier"),
				SemanticType:   ptrStr("identifier"),
				Role:           ptrStr("primary_key"),
			},
			emailColID: {
				SchemaColumnID: emailColID,
				Confidence:     ptrFloat(0.92),
				Description:    ptrStr("User's email address"),
				SemanticType:   ptrStr("email"),
				Role:           ptrStr("attribute"),
			},
		},
	}

	// Setup service
	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"users": columns,
		},
	}

	llmClient := &testColEnrichmentLLMClient{
		response: `{"columns": []}`, // Won't be used
	}
	llmFactory := &testColEnrichmentLLMFactory{client: llmClient}

	service := &columnEnrichmentService{
		ontologyRepo:       ontologyRepo,
		schemaRepo:         schemaRepo,
		columnMetadataRepo: colMetadataRepo,
		dsSvc:              &testColEnrichmentDatasourceService{},
		llmFactory:         llmFactory,
		workerPool:         llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:     llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:             zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"users"}, nil)

	// Verify
	require.NoError(t, err)
	assert.Equal(t, 1, len(result.TablesEnriched))
	assert.Equal(t, 0, len(result.TablesFailed))

	// Verify LLM was NOT called (all columns skipped)
	assert.Equal(t, 0, llmClient.callCount, "LLM should not be called when all columns have high-confidence features")

	// Verify column details were saved
	details := ontologyRepo.columnDetails["users"]
	require.Equal(t, 2, len(details))
}
