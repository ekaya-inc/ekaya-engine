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

func (r *testColEnrichmentOntologyRepo) UpdateEntitySummary(ctx context.Context, projectID uuid.UUID, tableName string, summary *models.EntitySummary) error {
	return nil
}

func (r *testColEnrichmentOntologyRepo) UpdateEntitySummaries(ctx context.Context, projectID uuid.UUID, summaries map[string]*models.EntitySummary) error {
	return nil
}

func (r *testColEnrichmentOntologyRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentOntologyRepo) GetNextVersion(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 1, nil
}

type testColEnrichmentEntityRepo struct {
	entities []*models.OntologyEntity
}

func (r *testColEnrichmentEntityRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return r.entities, nil
}

func (r *testColEnrichmentEntityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	for _, e := range r.entities {
		if e.ID == entityID {
			return e, nil
		}
	}
	return nil, fmt.Errorf("entity not found")
}

func (r *testColEnrichmentEntityRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return r.entities, nil
}

func (r *testColEnrichmentEntityRepo) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) Delete(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (r *testColEnrichmentEntityRepo) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentEntityRepo) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) DeleteKeyColumn(ctx context.Context, keyColumnID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}

func (r *testColEnrichmentEntityRepo) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}

func (r *testColEnrichmentEntityRepo) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}

func (r *testColEnrichmentEntityRepo) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	for _, e := range r.entities {
		if e.ProjectID == projectID && e.Name == name {
			return e, nil
		}
	}
	return nil, nil
}

func (r *testColEnrichmentEntityRepo) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	return 0, nil
}

func (r *testColEnrichmentEntityRepo) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
	return nil, nil
}

func (r *testColEnrichmentEntityRepo) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentEntityRepo) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

type testColEnrichmentRelRepo struct{}

func (r *testColEnrichmentRelRepo) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	// Return empty relationships for now
	return []*models.EntityRelationship{}, nil
}

func (r *testColEnrichmentRelRepo) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testColEnrichmentRelRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentRelRepo) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	return make(map[uuid.UUID][]*models.EntityRelationship), nil
}

func (r *testColEnrichmentRelRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentRelRepo) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	return nil
}

func (r *testColEnrichmentRelRepo) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	return nil
}

func (r *testColEnrichmentRelRepo) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentRelRepo) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentRelRepo) Upsert(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testColEnrichmentRelRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentRelRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentRelRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentRelRepo) Update(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testColEnrichmentRelRepo) MarkInferenceRelationshipsStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentRelRepo) ClearStaleFlag(ctx context.Context, relationshipID uuid.UUID) error {
	return nil
}

func (r *testColEnrichmentRelRepo) GetStaleRelationships(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testColEnrichmentRelRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

type testColEnrichmentSchemaRepo struct {
	columnsByTable map[string][]*models.SchemaColumn
}

func (r *testColEnrichmentSchemaRepo) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
	result := make(map[string][]*models.SchemaColumn)
	for _, tableName := range tableNames {
		if cols, ok := r.columnsByTable[tableName]; ok {
			result[tableName] = cols
		}
	}
	return result, nil
}

func (r *testColEnrichmentSchemaRepo) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}

func (r *testColEnrichmentSchemaRepo) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
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

func (r *testColEnrichmentSchemaRepo) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
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

func (r *testColEnrichmentSchemaRepo) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) GetColumnByName(ctx context.Context, projectID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
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

func (r *testColEnrichmentSchemaRepo) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64, sampleValues []string) error {
	return nil
}

func (r *testColEnrichmentSchemaRepo) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

// Mock datasource service for testing
type testColEnrichmentDatasourceService struct{}

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
	// Return empty list - enum sampling will be skipped
	return []*models.DatasourceWithStatus{}, nil
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

	// Setup: One entity with two columns
	entities := []*models.OntologyEntity{
		{
			ID:           uuid.New(),
			Name:         "User",
			Description:  "A user account",
			PrimaryTable: "users",
		},
	}

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
	entityRepo := &testColEnrichmentEntityRepo{entities: entities}
	relRepo := &testColEnrichmentRelRepo{}
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
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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

	entities := []*models.OntologyEntity{
		{
			ID:           uuid.New(),
			Name:         "User",
			PrimaryTable: "users",
		},
	}

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
	entityRepo := &testColEnrichmentEntityRepo{entities: entities}
	relRepo := &testColEnrichmentRelRepo{}
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
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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

	entities := []*models.OntologyEntity{
		{
			ID:           uuid.New(),
			Name:         "User",
			PrimaryTable: "users",
		},
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
	}

	// Setup service with non-retryable error
	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	entityRepo := &testColEnrichmentEntityRepo{entities: entities}
	relRepo := &testColEnrichmentRelRepo{}
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
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"users"}, nil)

	// Verify - should fail after retrying (retry.Do will still attempt retries)
	// Note: retry.Do attempts all retries even for non-retryable errors,
	// but logs them differently as "non-retryable"
	require.NoError(t, err) // EnrichProject continues on table failures
	assert.Equal(t, 0, len(result.TablesEnriched))
	assert.Equal(t, 1, len(result.TablesFailed))

	// Verify LLM was called with retries
	client := llmFactory.client.(*testColEnrichmentLLMClient)
	assert.Equal(t, 4, client.callCount, "Will retry even on auth error (1 initial + 3 retries)")
}

func TestColumnEnrichmentService_EnrichProject_LargeTable(t *testing.T) {
	projectID := uuid.New()

	entities := []*models.OntologyEntity{
		{
			ID:           uuid.New(),
			Name:         "LargeTable",
			PrimaryTable: "large_table",
		},
	}

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
	entityRepo := &testColEnrichmentEntityRepo{entities: entities}
	relRepo := &testColEnrichmentRelRepo{}
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
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), Name: "Table1", PrimaryTable: "table1"},
		{ID: uuid.New(), Name: "Table2", PrimaryTable: "table2"},
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
	}

	llmResponse := `{
		"columns": [
			{"name": "id", "description": "ID", "semantic_type": "identifier", "role": "identifier", "fk_association": null}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	entityRepo := &testColEnrichmentEntityRepo{entities: entities}
	relRepo := &testColEnrichmentRelRepo{}
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
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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
	entityRepo := &testColEnrichmentEntityRepo{entities: []*models.OntologyEntity{}}
	relRepo := &testColEnrichmentRelRepo{}
	schemaRepo := &testColEnrichmentSchemaRepo{columnsByTable: make(map[string][]*models.SchemaColumn)}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{},
	}

	service := &columnEnrichmentService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), Name: "Table1", PrimaryTable: "table1"},
		{ID: uuid.New(), Name: "Table2", PrimaryTable: "table2"},
		{ID: uuid.New(), Name: "Table3", PrimaryTable: "table3"},
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
	}

	llmResponse := `{
		"columns": [
			{"name": "id", "description": "ID", "semantic_type": "identifier", "role": "identifier", "fk_association": null}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	entityRepo := &testColEnrichmentEntityRepo{entities: entities}
	relRepo := &testColEnrichmentRelRepo{}
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
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"table1", "table2", "table3"}, nil)

	// Verify - should continue despite failure on table2
	require.NoError(t, err, "EnrichProject should not return error for partial failures")
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

	entity := &models.OntologyEntity{
		ID:           uuid.New(),
		Name:         "Order",
		PrimaryTable: "orders",
	}

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
	entityRepo := &testColEnrichmentEntityRepo{entities: []*models.OntologyEntity{entity}}
	relRepo := &testColEnrichmentRelRepo{}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": columns,
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{response: llmResponse},
	}

	service := &columnEnrichmentService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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

	entity := &models.OntologyEntity{
		ID:           uuid.New(),
		Name:         "Order",
		PrimaryTable: "orders",
	}

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
	entityRepo := &testColEnrichmentEntityRepo{entities: []*models.OntologyEntity{entity}}
	relRepo := &testColEnrichmentRelRepo{}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"orders": columns,
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{response: llmResponse},
	}

	service := &columnEnrichmentService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
		{ColumnName: "order_status", DataType: "varchar"},
		{ColumnName: "user_type", DataType: "varchar"},
		{ColumnName: "email", DataType: "varchar", DistinctCount: &distinctCount100},
		{ColumnName: "priority_level", DataType: "varchar", DistinctCount: &distinctCount5},
	}

	candidates := service.identifyEnumCandidates(columns)

	// Should identify columns with enum patterns or low cardinality
	assert.GreaterOrEqual(t, len(candidates), 3, "Should find at least 3 enum candidates")

	candidateNames := make(map[string]bool)
	for _, c := range candidates {
		candidateNames[c.ColumnName] = true
	}

	assert.True(t, candidateNames["order_status"], "Should identify 'order_status' as enum")
	assert.True(t, candidateNames["user_type"], "Should identify 'user_type' as enum")
	assert.True(t, candidateNames["priority_level"], "Should identify 'priority_level' as enum (low cardinality)")
	assert.False(t, candidateNames["id"], "Should not identify 'id' as enum")
}

func TestColumnEnrichmentService_EnrichTable_NoEntity(t *testing.T) {
	projectID := uuid.New()

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	entityRepo := &testColEnrichmentEntityRepo{entities: []*models.OntologyEntity{}} // Empty
	relRepo := &testColEnrichmentRelRepo{}
	schemaRepo := &testColEnrichmentSchemaRepo{columnsByTable: make(map[string][]*models.SchemaColumn)}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{},
	}

	service := &columnEnrichmentService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
	}

	// Execute
	err := service.EnrichTable(context.Background(), projectID, "nonexistent_table")

	// Verify
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no entity found")
}

func TestColumnEnrichmentService_EnrichTable_NoColumns(t *testing.T) {
	projectID := uuid.New()

	entity := &models.OntologyEntity{
		ID:           uuid.New(),
		Name:         "EmptyTable",
		PrimaryTable: "empty_table",
	}

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	entityRepo := &testColEnrichmentEntityRepo{entities: []*models.OntologyEntity{entity}}
	relRepo := &testColEnrichmentRelRepo{}
	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"empty_table": {}, // No columns
		},
	}
	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{},
	}

	service := &columnEnrichmentService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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

	entities := []*models.OntologyEntity{
		{ID: uuid.New(), Name: "Table1", PrimaryTable: "table1"},
		{ID: uuid.New(), Name: "Table2", PrimaryTable: "table2"},
		{ID: uuid.New(), Name: "Table3", PrimaryTable: "table3"},
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
	}

	llmResponse := `{
		"columns": [
			{"name": "id", "description": "ID", "semantic_type": "identifier", "role": "identifier", "fk_association": null}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	entityRepo := &testColEnrichmentEntityRepo{entities: entities}
	relRepo := &testColEnrichmentRelRepo{}
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
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   highThresholdCircuit,
		logger:           zap.NewNop(),
	}

	// Execute
	result, err := service.EnrichProject(context.Background(), projectID, []string{"table1", "table2", "table3"}, nil)

	// Verify - should not return error but track failures
	require.NoError(t, err)
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

	entity := &models.OntologyEntity{
		ID:           uuid.New(),
		Name:         "LargeTable",
		PrimaryTable: "large_table",
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
		entity,
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

	entity := &models.OntologyEntity{
		ID:           uuid.New(),
		Name:         "LargeTable",
		PrimaryTable: "large_table",
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
		entity,
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

// TestColumnEnrichmentService_EnrichTable_SelfReferentialFK tests that self-referential
// foreign keys (e.g., employees.manager_id -> employees.id) are properly handled
// with appropriate fk_association values like "manager" or "reports_to".
func TestColumnEnrichmentService_EnrichTable_SelfReferentialFK(t *testing.T) {
	projectID := uuid.New()
	entityID := uuid.New()

	entity := &models.OntologyEntity{
		ID:           entityID,
		Name:         "Employee",
		Description:  "An employee in the organization",
		PrimaryTable: "employees",
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint", IsPrimaryKey: true},
		{ColumnName: "name", DataType: "varchar"},
		{ColumnName: "manager_id", DataType: "bigint"}, // Self-referential FK
	}

	// LLM response with proper self-referential FK association
	llmResponse := `{
		"columns": [
			{
				"name": "id",
				"description": "Unique identifier for the employee",
				"semantic_type": "identifier",
				"role": "identifier",
				"fk_association": null
			},
			{
				"name": "name",
				"description": "Full name of the employee",
				"semantic_type": "text",
				"role": "attribute",
				"fk_association": null
			},
			{
				"name": "manager_id",
				"description": "Reference to the employee's manager in the organizational hierarchy",
				"semantic_type": "identifier",
				"role": "dimension",
				"fk_association": "manager"
			}
		]
	}`

	ontologyRepo := &testColEnrichmentOntologyRepo{columnDetails: make(map[string][]models.ColumnDetail)}
	entityRepo := &testColEnrichmentEntityRepo{entities: []*models.OntologyEntity{entity}}

	// Create a relationship repo that returns self-referential FK
	relRepo := &testSelfRefRelRepo{
		relationships: []*models.EntityRelationship{
			{
				ID:                uuid.New(),
				SourceEntityID:    entityID,
				TargetEntityID:    entityID, // Same entity - self-referential
				SourceColumnTable: "employees",
				SourceColumnName:  "manager_id",
				TargetColumnTable: "employees",
				TargetColumnName:  "id",
			},
		},
	}

	schemaRepo := &testColEnrichmentSchemaRepo{
		columnsByTable: map[string][]*models.SchemaColumn{
			"employees": columns,
		},
	}

	llmFactory := &testColEnrichmentLLMFactory{
		client: &testColEnrichmentLLMClient{response: llmResponse},
	}

	service := &columnEnrichmentService{
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
	}

	// Execute
	err := service.EnrichTable(context.Background(), projectID, "employees")

	// Verify
	require.NoError(t, err)
	details := ontologyRepo.columnDetails["employees"]
	require.Equal(t, 3, len(details))

	// Find the manager_id column and verify self-referential FK association
	managerCol := findColumnDetail(details, "manager_id")
	require.NotNil(t, managerCol, "Should find manager_id column")
	assert.Equal(t, "manager_id", managerCol.Name)
	assert.Equal(t, "manager", managerCol.FKAssociation, "Self-referential FK should have 'manager' association")
	assert.True(t, managerCol.IsForeignKey, "manager_id should be marked as FK")
	assert.Equal(t, "employees", managerCol.ForeignTable, "FK should point to same table (employees)")
}

// testSelfRefRelRepo is a mock relationship repo that returns self-referential relationships
type testSelfRefRelRepo struct {
	relationships []*models.EntityRelationship
}

func (r *testSelfRefRelRepo) GetByTables(ctx context.Context, projectID uuid.UUID, tableNames []string) ([]*models.EntityRelationship, error) {
	// Filter relationships to those involving the requested tables
	var result []*models.EntityRelationship
	for _, rel := range r.relationships {
		for _, tableName := range tableNames {
			if rel.SourceColumnTable == tableName {
				result = append(result, rel)
				break
			}
		}
	}
	return result, nil
}

func (r *testSelfRefRelRepo) Create(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testSelfRefRelRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return r.relationships, nil
}

func (r *testSelfRefRelRepo) GetByOntologyGroupedByTarget(ctx context.Context, ontologyID uuid.UUID) (map[uuid.UUID][]*models.EntityRelationship, error) {
	return make(map[uuid.UUID][]*models.EntityRelationship), nil
}

func (r *testSelfRefRelRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.EntityRelationship, error) {
	return r.relationships, nil
}

func (r *testSelfRefRelRepo) UpdateDescription(ctx context.Context, id uuid.UUID, description string) error {
	return nil
}

func (r *testSelfRefRelRepo) UpdateDescriptionAndAssociation(ctx context.Context, id uuid.UUID, description string, association string) error {
	return nil
}

func (r *testSelfRefRelRepo) GetByTargetEntity(ctx context.Context, entityID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testSelfRefRelRepo) GetByEntityPair(ctx context.Context, ontologyID uuid.UUID, fromEntityID uuid.UUID, toEntityID uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testSelfRefRelRepo) Upsert(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testSelfRefRelRepo) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *testSelfRefRelRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testSelfRefRelRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testSelfRefRelRepo) Update(ctx context.Context, rel *models.EntityRelationship) error {
	return nil
}

func (r *testSelfRefRelRepo) MarkInferenceRelationshipsStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (r *testSelfRefRelRepo) ClearStaleFlag(ctx context.Context, relationshipID uuid.UUID) error {
	return nil
}

func (r *testSelfRefRelRepo) GetStaleRelationships(ctx context.Context, ontologyID uuid.UUID) ([]*models.EntityRelationship, error) {
	return nil, nil
}

func (r *testSelfRefRelRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
	return nil
}

// TestColumnEnrichmentService_EnrichTable_IntegerEnumInference tests that integer enum
// columns like transaction_state get meaningful labels inferred from column context.
func TestColumnEnrichmentService_EnrichTable_IntegerEnumInference(t *testing.T) {
	projectID := uuid.New()

	entity := &models.OntologyEntity{
		ID:           uuid.New(),
		Name:         "BillingTransaction",
		Description:  "A billing transaction recording payment activity",
		PrimaryTable: "billing_transactions",
	}

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
	entityRepo := &testColEnrichmentEntityRepo{entities: []*models.OntologyEntity{entity}}
	relRepo := &testColEnrichmentRelRepo{}
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
		ontologyRepo:     ontologyRepo,
		entityRepo:       entityRepo,
		relationshipRepo: relRepo,
		schemaRepo:       schemaRepo,
		dsSvc:            &testColEnrichmentDatasourceService{},
		llmFactory:       llmFactory,
		workerPool:       llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 1}, zap.NewNop()),
		circuitBreaker:   llm.NewCircuitBreaker(llm.DefaultCircuitBreakerConfig()),
		logger:           zap.NewNop(),
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

	entity := &models.OntologyEntity{
		ID:           uuid.New(),
		Name:         "TestEntity",
		PrimaryTable: "test_table",
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "status", DataType: "integer"},
	}

	enumSamples := map[string][]string{
		"status": {"1", "2", "3"},
	}

	prompt := service.buildColumnEnrichmentPrompt(entity, columns, nil, enumSamples)

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
	fkDetailedInfo := map[string]*FKRelationshipInfo{}
	enumDistributions := map[string]*datasource.EnumDistributionResult{}

	details := service.convertToColumnDetails("billing_transactions", enrichments, columns, fkInfo, fkDetailedInfo, enumSamples, enumDefs, enumDistributions)

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

// TestFindCompletionTimestampColumn verifies completion timestamp column detection.
func TestFindCompletionTimestampColumn(t *testing.T) {
	tests := []struct {
		name     string
		columns  []*models.SchemaColumn
		expected string
	}{
		{
			name: "finds completed_at",
			columns: []*models.SchemaColumn{
				{ColumnName: "id", DataType: "integer"},
				{ColumnName: "status", DataType: "varchar"},
				{ColumnName: "completed_at", DataType: "timestamp"},
				{ColumnName: "created_at", DataType: "timestamp"},
			},
			expected: "completed_at",
		},
		{
			name: "finds finished_at",
			columns: []*models.SchemaColumn{
				{ColumnName: "id", DataType: "integer"},
				{ColumnName: "finished_at", DataType: "timestamp with time zone"},
			},
			expected: "finished_at",
		},
		{
			name: "finds ended_at",
			columns: []*models.SchemaColumn{
				{ColumnName: "ended_at", DataType: "datetime"},
			},
			expected: "ended_at",
		},
		{
			name: "fallback to completion-like column",
			columns: []*models.SchemaColumn{
				{ColumnName: "job_completion_time", DataType: "timestamp"},
			},
			expected: "job_completion_time",
		},
		{
			name: "ignores non-timestamp columns",
			columns: []*models.SchemaColumn{
				{ColumnName: "completed_at", DataType: "varchar"}, // Not a timestamp!
				{ColumnName: "status", DataType: "varchar"},
			},
			expected: "",
		},
		{
			name: "returns empty when no completion column",
			columns: []*models.SchemaColumn{
				{ColumnName: "id", DataType: "integer"},
				{ColumnName: "created_at", DataType: "timestamp"},
				{ColumnName: "updated_at", DataType: "timestamp"},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findCompletionTimestampColumn(tt.columns)
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

// ============================================================================
// Soft Delete Pattern Recognition Tests
// ============================================================================

func TestDetectSoftDeletePattern_TypicalSoftDelete(t *testing.T) {
	// Typical GORM soft delete: deleted_at, timestamp, nullable, 97% NULL
	rowCount := int64(1000)
	nullCount := int64(970) // 97% NULL = 97% active

	col := &models.SchemaColumn{
		ColumnName: "deleted_at",
		DataType:   "timestamp with time zone",
		IsNullable: true,
		RowCount:   &rowCount,
		NullCount:  &nullCount,
	}

	result := detectSoftDeletePattern(col)

	require.NotNil(t, result, "Should detect soft delete pattern")
	assert.Contains(t, result.Description, "Soft delete timestamp")
	assert.Contains(t, result.Description, "NULL = active record")
	assert.Contains(t, result.Description, "97.0% of records are active")
	assert.Equal(t, "soft_delete_timestamp", result.SemanticType)
	assert.Equal(t, models.ColumnRoleAttribute, result.Role)
	assert.InDelta(t, 97.0, result.ActiveRate, 0.1)
}

func TestDetectSoftDeletePattern_AllActiveRecords(t *testing.T) {
	// Edge case: all records are active (100% NULL)
	rowCount := int64(500)
	nullCount := int64(500) // 100% NULL

	col := &models.SchemaColumn{
		ColumnName: "deleted_at",
		DataType:   "timestamp",
		IsNullable: true,
		RowCount:   &rowCount,
		NullCount:  &nullCount,
	}

	result := detectSoftDeletePattern(col)

	require.NotNil(t, result)
	assert.Contains(t, result.Description, "100.0% of records are active")
	assert.InDelta(t, 100.0, result.ActiveRate, 0.1)
}

func TestDetectSoftDeletePattern_MinimumThreshold(t *testing.T) {
	// Boundary case: exactly 90% NULL (should match)
	rowCount := int64(1000)
	nullCount := int64(900) // 90% NULL

	col := &models.SchemaColumn{
		ColumnName: "deleted_at",
		DataType:   "timestamp",
		IsNullable: true,
		RowCount:   &rowCount,
		NullCount:  &nullCount,
	}

	result := detectSoftDeletePattern(col)

	require.NotNil(t, result, "Should detect at exactly 90% threshold")
	assert.InDelta(t, 90.0, result.ActiveRate, 0.1)
}

func TestDetectSoftDeletePattern_BelowThreshold(t *testing.T) {
	// Below threshold: 89% NULL (should NOT match)
	rowCount := int64(1000)
	nullCount := int64(890) // 89% NULL

	col := &models.SchemaColumn{
		ColumnName: "deleted_at",
		DataType:   "timestamp",
		IsNullable: true,
		RowCount:   &rowCount,
		NullCount:  &nullCount,
	}

	result := detectSoftDeletePattern(col)

	assert.Nil(t, result, "Should not detect when below 90% threshold")
}

func TestDetectSoftDeletePattern_WrongColumnName(t *testing.T) {
	tests := []struct {
		name       string
		columnName string
	}{
		{"created_at column", "created_at"},
		{"updated_at column", "updated_at"},
		{"is_deleted column", "is_deleted"},
		{"deleted column", "deleted"},
		{"deletion_timestamp column", "deletion_timestamp"},
		{"removed_at column", "removed_at"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rowCount := int64(1000)
			nullCount := int64(970)

			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "timestamp",
				IsNullable: true,
				RowCount:   &rowCount,
				NullCount:  &nullCount,
			}

			result := detectSoftDeletePattern(col)
			assert.Nil(t, result, "Should not detect for column name: %s", tt.columnName)
		})
	}
}

func TestDetectSoftDeletePattern_WrongDataType(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
	}{
		{"boolean type", "boolean"},
		{"integer type", "integer"},
		{"varchar type", "varchar(255)"},
		{"text type", "text"},
		{"bigint type", "bigint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rowCount := int64(1000)
			nullCount := int64(970)

			col := &models.SchemaColumn{
				ColumnName: "deleted_at",
				DataType:   tt.dataType,
				IsNullable: true,
				RowCount:   &rowCount,
				NullCount:  &nullCount,
			}

			result := detectSoftDeletePattern(col)
			assert.Nil(t, result, "Should not detect for data type: %s", tt.dataType)
		})
	}
}

func TestDetectSoftDeletePattern_NotNullable(t *testing.T) {
	// deleted_at that is NOT NULL is not a soft delete pattern
	rowCount := int64(1000)
	nullCount := int64(970)

	col := &models.SchemaColumn{
		ColumnName: "deleted_at",
		DataType:   "timestamp",
		IsNullable: false, // NOT NULL constraint
		RowCount:   &rowCount,
		NullCount:  &nullCount,
	}

	result := detectSoftDeletePattern(col)
	assert.Nil(t, result, "Should not detect when column is NOT NULL")
}

func TestDetectSoftDeletePattern_NoStats(t *testing.T) {
	// No statistics available - should assume high null rate
	col := &models.SchemaColumn{
		ColumnName: "deleted_at",
		DataType:   "timestamp with time zone",
		IsNullable: true,
		RowCount:   nil, // No stats
		NullCount:  nil,
	}

	result := detectSoftDeletePattern(col)

	require.NotNil(t, result, "Should detect with no stats (assumes 100% active)")
	assert.Contains(t, result.Description, "100.0% of records are active")
}

func TestDetectSoftDeletePattern_ZeroRows(t *testing.T) {
	// Edge case: empty table
	rowCount := int64(0)
	nullCount := int64(0)

	col := &models.SchemaColumn{
		ColumnName: "deleted_at",
		DataType:   "timestamp",
		IsNullable: true,
		RowCount:   &rowCount,
		NullCount:  &nullCount,
	}

	result := detectSoftDeletePattern(col)

	// Should still detect pattern based on schema, assume 100% active
	require.NotNil(t, result)
	assert.Contains(t, result.Description, "100.0% of records are active")
}

func TestDetectSoftDeletePattern_WithNonNullCount(t *testing.T) {
	// Some systems track NonNullCount instead of NullCount
	rowCount := int64(1000)
	nonNullCount := int64(30) // 30 deleted records = 97% active

	col := &models.SchemaColumn{
		ColumnName:   "deleted_at",
		DataType:     "timestamptz",
		IsNullable:   true,
		RowCount:     &rowCount,
		NonNullCount: &nonNullCount,
		NullCount:    nil, // Only NonNullCount available
	}

	result := detectSoftDeletePattern(col)

	require.NotNil(t, result)
	assert.InDelta(t, 97.0, result.ActiveRate, 0.1)
}

func TestDetectSoftDeletePattern_TimestampVariants(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
	}{
		{"plain timestamp", "timestamp"},
		{"timestamp with tz", "timestamp with time zone"},
		{"timestamptz", "timestamptz"},
		{"datetime", "datetime"},
		{"datetime2", "datetime2"},
		{"TIMESTAMP uppercase", "TIMESTAMP"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rowCount := int64(1000)
			nullCount := int64(950)

			col := &models.SchemaColumn{
				ColumnName: "deleted_at",
				DataType:   tt.dataType,
				IsNullable: true,
				RowCount:   &rowCount,
				NullCount:  &nullCount,
			}

			result := detectSoftDeletePattern(col)
			require.NotNil(t, result, "Should detect for timestamp variant: %s", tt.dataType)
		})
	}
}

func TestDetectSoftDeletePattern_CaseInsensitiveColumnName(t *testing.T) {
	tests := []struct {
		name       string
		columnName string
		shouldFind bool
	}{
		{"lowercase", "deleted_at", true},
		{"UPPERCASE", "DELETED_AT", true},
		{"MixedCase", "Deleted_At", true},
		{"partial match", "deleted_at_timestamp", false}, // Exact match required
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rowCount := int64(1000)
			nullCount := int64(950)

			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "timestamp",
				IsNullable: true,
				RowCount:   &rowCount,
				NullCount:  &nullCount,
			}

			result := detectSoftDeletePattern(col)
			if tt.shouldFind {
				assert.NotNil(t, result, "Should detect for column name: %s", tt.columnName)
			} else {
				assert.Nil(t, result, "Should not detect for column name: %s", tt.columnName)
			}
		})
	}
}

// =============================================================================
// Monetary Column Detection Tests
// =============================================================================

func TestDetectMonetaryColumnPattern_TypicalAmount(t *testing.T) {
	// Typical monetary column: amount, bigint, with currency column
	col := &models.SchemaColumn{
		ColumnName: "amount",
		DataType:   "bigint",
	}

	result := detectMonetaryColumnPattern(col, "currency")

	require.NotNil(t, result, "Should detect monetary pattern")
	assert.Contains(t, result.Description, "Monetary amount in minor units")
	assert.Contains(t, result.Description, "Pair with currency column")
	assert.Contains(t, result.Description, "ISO 4217")
	assert.Equal(t, "currency_cents", result.SemanticType)
	assert.Equal(t, models.ColumnRoleMeasure, result.Role)
	assert.Equal(t, "currency", result.CurrencyColumn)
}

func TestDetectMonetaryColumnPattern_AmountWithoutCurrency(t *testing.T) {
	// Amount column without a currency column - still detect but different description
	col := &models.SchemaColumn{
		ColumnName: "amount",
		DataType:   "bigint",
	}

	result := detectMonetaryColumnPattern(col, "")

	require.NotNil(t, result, "Should detect monetary pattern without currency column")
	assert.Contains(t, result.Description, "Monetary amount in minor units")
	assert.Contains(t, result.Description, "Integer values represent smallest currency unit")
	assert.NotContains(t, result.Description, "Pair with")
	assert.Equal(t, "currency_cents", result.SemanticType)
	assert.Empty(t, result.CurrencyColumn)
}

func TestDetectMonetaryColumnPattern_SuffixPatterns(t *testing.T) {
	tests := []struct {
		name       string
		columnName string
	}{
		{"total_amount", "total_amount"},
		{"base_amount", "base_amount"},
		{"net_amount", "net_amount"},
		{"tikr_share", "tikr_share"},
		{"creator_share", "creator_share"},
		{"grand_total", "grand_total"},
		{"sub_total", "sub_total"},
		{"unit_price", "unit_price"},
		{"list_price", "list_price"},
		{"total_cost", "total_cost"},
		{"service_fee", "service_fee"},
		{"platform_fee", "platform_fee"},
		{"order_value", "order_value"},
		{"total_value", "total_value"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "bigint",
			}

			result := detectMonetaryColumnPattern(col, "currency")

			require.NotNil(t, result, "Should detect monetary pattern for: %s", tt.columnName)
			assert.Equal(t, "currency_cents", result.SemanticType)
			assert.Equal(t, models.ColumnRoleMeasure, result.Role)
		})
	}
}

func TestDetectMonetaryColumnPattern_IntegerTypes(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
	}{
		{"bigint", "bigint"},
		{"integer", "integer"},
		{"int", "int"},
		{"int4", "int4"},
		{"int8", "int8"},
		{"smallint", "smallint"},
		{"numeric", "numeric"},
		{"BIGINT uppercase", "BIGINT"},
		{"INTEGER uppercase", "INTEGER"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "amount",
				DataType:   tt.dataType,
			}

			result := detectMonetaryColumnPattern(col, "currency")

			require.NotNil(t, result, "Should detect for integer type: %s", tt.dataType)
			assert.Equal(t, "currency_cents", result.SemanticType)
		})
	}
}

func TestDetectMonetaryColumnPattern_NonIntegerTypes(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
	}{
		{"varchar", "varchar(255)"},
		{"text", "text"},
		{"boolean", "boolean"},
		{"timestamp", "timestamp"},
		{"date", "date"},
		{"double", "double precision"},
		{"float", "float"},
		{"real", "real"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "amount",
				DataType:   tt.dataType,
			}

			result := detectMonetaryColumnPattern(col, "currency")

			assert.Nil(t, result, "Should NOT detect for non-integer type: %s", tt.dataType)
		})
	}
}

func TestDetectMonetaryColumnPattern_NonMonetaryColumnNames(t *testing.T) {
	tests := []struct {
		name       string
		columnName string
	}{
		{"created_at", "created_at"},
		{"user_id", "user_id"},
		{"status", "status"},
		{"count", "count"},
		{"quantity", "quantity"},
		{"position", "position"},
		{"version", "version"},
		{"amount_type", "amount_type"}, // Contains amount but as prefix
		{"preamount", "preamount"},     // Contains amount but not as suffix/standalone
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "bigint",
			}

			result := detectMonetaryColumnPattern(col, "currency")

			assert.Nil(t, result, "Should NOT detect monetary pattern for: %s", tt.columnName)
		})
	}
}

func TestDetectMonetaryColumnPattern_CaseInsensitiveColumnName(t *testing.T) {
	tests := []struct {
		name       string
		columnName string
		shouldFind bool
	}{
		{"lowercase amount", "amount", true},
		{"UPPERCASE AMOUNT", "AMOUNT", true},
		{"MixedCase Amount", "Amount", true},
		{"TOTAL_AMOUNT uppercase", "TOTAL_AMOUNT", true},
		{"Total_Price mixed", "Total_Price", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "bigint",
			}

			result := detectMonetaryColumnPattern(col, "currency")

			if tt.shouldFind {
				require.NotNil(t, result, "Should detect for column name: %s", tt.columnName)
			} else {
				assert.Nil(t, result, "Should not detect for column name: %s", tt.columnName)
			}
		})
	}
}

// =============================================================================
// findCurrencyColumn Tests
// =============================================================================

func TestFindCurrencyColumn_ExactMatch(t *testing.T) {
	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
		{ColumnName: "amount", DataType: "bigint"},
		{ColumnName: "currency", DataType: "varchar(3)", SampleValues: []string{"USD", "EUR", "GBP"}},
		{ColumnName: "created_at", DataType: "timestamp"},
	}

	result := findCurrencyColumn(columns)

	assert.Equal(t, "currency", result)
}

func TestFindCurrencyColumn_SuffixMatch(t *testing.T) {
	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
		{ColumnName: "amount", DataType: "bigint"},
		{ColumnName: "payment_currency", DataType: "varchar(3)", SampleValues: []string{"USD", "CAD", "AUD"}},
	}

	result := findCurrencyColumn(columns)

	assert.Equal(t, "payment_currency", result)
}

func TestFindCurrencyColumn_ISO4217Validation(t *testing.T) {
	columns := []*models.SchemaColumn{
		{ColumnName: "currency", DataType: "varchar(10)", SampleValues: []string{"USD", "EUR", "GBP", "CAD", "AUD"}},
	}

	result := findCurrencyColumn(columns)

	assert.Equal(t, "currency", result)
}

func TestFindCurrencyColumn_NotISO4217(t *testing.T) {
	// Currency column with non-ISO4217 values (lowercase, long strings)
	columns := []*models.SchemaColumn{
		{ColumnName: "currency", DataType: "varchar(50)", SampleValues: []string{"usd", "euro", "dollar"}},
	}

	result := findCurrencyColumn(columns)

	// Should not match because sample values don't look like ISO 4217
	assert.Empty(t, result)
}

func TestFindCurrencyColumn_MixedISO4217Values(t *testing.T) {
	// More than 50% are ISO 4217 codes, should match
	columns := []*models.SchemaColumn{
		{ColumnName: "currency", DataType: "varchar(10)", SampleValues: []string{"USD", "EUR", "GBP", "unknown"}},
	}

	result := findCurrencyColumn(columns)

	// 3/4 = 75% are ISO 4217, should match
	assert.Equal(t, "currency", result)
}

func TestFindCurrencyColumn_NoSampleValues(t *testing.T) {
	// No sample values, but name and type match - should assume it's currency
	columns := []*models.SchemaColumn{
		{ColumnName: "currency", DataType: "varchar(3)", SampleValues: nil},
	}

	result := findCurrencyColumn(columns)

	assert.Equal(t, "currency", result)
}

func TestFindCurrencyColumn_WrongType(t *testing.T) {
	// Column named currency but wrong type (integer)
	columns := []*models.SchemaColumn{
		{ColumnName: "currency", DataType: "integer", SampleValues: []string{"1", "2", "3"}},
	}

	result := findCurrencyColumn(columns)

	assert.Empty(t, result)
}

func TestFindCurrencyColumn_NoCurrencyColumn(t *testing.T) {
	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "bigint"},
		{ColumnName: "amount", DataType: "bigint"},
		{ColumnName: "status", DataType: "varchar(20)"},
	}

	result := findCurrencyColumn(columns)

	assert.Empty(t, result)
}

func TestFindCurrencyColumn_CaseInsensitive(t *testing.T) {
	columns := []*models.SchemaColumn{
		{ColumnName: "CURRENCY", DataType: "varchar(3)", SampleValues: []string{"USD", "EUR"}},
	}

	result := findCurrencyColumn(columns)

	assert.Equal(t, "CURRENCY", result)
}

// =============================================================================
// isISO4217CurrencyCode Tests
// =============================================================================

func TestIsISO4217CurrencyCode_ValidCodes(t *testing.T) {
	validCodes := []string{"USD", "EUR", "GBP", "CAD", "AUD", "JPY", "CNY", "INR", "BRL", "MXN"}

	for _, code := range validCodes {
		t.Run(code, func(t *testing.T) {
			assert.True(t, isISO4217CurrencyCode(code), "Should be valid ISO 4217: %s", code)
		})
	}
}

func TestIsISO4217CurrencyCode_InvalidCodes(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"lowercase", "usd"},
		{"mixed case", "Usd"},
		{"too short", "US"},
		{"too long", "USDC"},
		{"with number", "US1"},
		{"with space", "US "},
		{"empty", ""},
		{"numbers only", "123"},
		{"with special char", "US$"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.False(t, isISO4217CurrencyCode(tt.value), "Should NOT be valid ISO 4217: %s", tt.value)
		})
	}
}

// =============================================================================
// isMonetaryIntegerType Tests
// =============================================================================

func TestIsMonetaryIntegerType_ValidTypes(t *testing.T) {
	validTypes := []string{
		"bigint", "BIGINT", "integer", "INTEGER", "int", "INT",
		"int4", "INT4", "int8", "INT8", "smallint", "SMALLINT",
		"numeric", "NUMERIC", "numeric(10,0)",
	}

	for _, typ := range validTypes {
		t.Run(typ, func(t *testing.T) {
			assert.True(t, isMonetaryIntegerType(typ), "Should be valid monetary integer type: %s", typ)
		})
	}
}

func TestIsMonetaryIntegerType_InvalidTypes(t *testing.T) {
	invalidTypes := []string{
		"varchar", "text", "boolean", "timestamp", "date",
		"double precision", "float", "real", "uuid", "jsonb",
	}

	for _, typ := range invalidTypes {
		t.Run(typ, func(t *testing.T) {
			assert.False(t, isMonetaryIntegerType(typ), "Should NOT be valid monetary integer type: %s", typ)
		})
	}
}

// =============================================================================
// UUID Text Column Detection Tests
// =============================================================================

func TestDetectUUIDTextColumnPattern_TypicalUUIDColumn(t *testing.T) {
	// Typical UUID column: text type with all sample values matching UUID format
	col := &models.SchemaColumn{
		ColumnName: "channel_id",
		DataType:   "varchar(36)",
		SampleValues: []string{
			"550e8400-e29b-41d4-a716-446655440000",
			"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			"f47ac10b-58cc-4372-a567-0e02b2c3d479",
			"a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11",
			"3fa85f64-5717-4562-b3fc-2c963f66afa6",
		},
	}

	result := detectUUIDTextColumnPattern(col)

	require.NotNil(t, result, "Should detect UUID text pattern")
	assert.Contains(t, result.Description, "UUID stored as text")
	assert.Contains(t, result.Description, "36 characters")
	assert.Contains(t, result.Description, "Logical foreign key")
	assert.Equal(t, "uuid_text", result.SemanticType)
	assert.Equal(t, models.ColumnRoleIdentifier, result.Role)
	assert.InDelta(t, 100.0, result.MatchRate, 0.1)
}

func TestDetectUUIDTextColumnPattern_LowercaseUUIDs(t *testing.T) {
	// UUIDs can be stored in lowercase
	col := &models.SchemaColumn{
		ColumnName: "external_id",
		DataType:   "text",
		SampleValues: []string{
			"550e8400-e29b-41d4-a716-446655440000",
			"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			"f47ac10b-58cc-4372-a567-0e02b2c3d479",
		},
	}

	result := detectUUIDTextColumnPattern(col)

	require.NotNil(t, result, "Should detect lowercase UUIDs")
	assert.Equal(t, "uuid_text", result.SemanticType)
}

func TestDetectUUIDTextColumnPattern_UppercaseUUIDs(t *testing.T) {
	// UUIDs can be stored in uppercase
	col := &models.SchemaColumn{
		ColumnName: "session_id",
		DataType:   "char(36)",
		SampleValues: []string{
			"550E8400-E29B-41D4-A716-446655440000",
			"6BA7B810-9DAD-11D1-80B4-00C04FD430C8",
			"F47AC10B-58CC-4372-A567-0E02B2C3D479",
		},
	}

	result := detectUUIDTextColumnPattern(col)

	require.NotNil(t, result, "Should detect uppercase UUIDs")
	assert.Equal(t, "uuid_text", result.SemanticType)
}

func TestDetectUUIDTextColumnPattern_MixedCaseUUIDs(t *testing.T) {
	// UUIDs can have mixed case
	col := &models.SchemaColumn{
		ColumnName: "ref_id",
		DataType:   "varchar(40)",
		SampleValues: []string{
			"550e8400-E29B-41d4-A716-446655440000",
			"6BA7b810-9dad-11D1-80B4-00c04fd430c8",
		},
	}

	result := detectUUIDTextColumnPattern(col)

	require.NotNil(t, result, "Should detect mixed-case UUIDs")
	assert.Equal(t, "uuid_text", result.SemanticType)
}

func TestDetectUUIDTextColumnPattern_TextTypeVariants(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
	}{
		{"varchar", "varchar(36)"},
		{"VARCHAR uppercase", "VARCHAR(36)"},
		{"text", "text"},
		{"TEXT uppercase", "TEXT"},
		{"char", "char(36)"},
		{"CHAR uppercase", "CHAR(36)"},
		{"character varying", "character varying(36)"},
		{"nvarchar", "nvarchar(36)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "uuid_col",
				DataType:   tt.dataType,
				SampleValues: []string{
					"550e8400-e29b-41d4-a716-446655440000",
					"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				},
			}

			result := detectUUIDTextColumnPattern(col)
			require.NotNil(t, result, "Should detect for text type: %s", tt.dataType)
			assert.Equal(t, "uuid_text", result.SemanticType)
		})
	}
}

func TestDetectUUIDTextColumnPattern_NonTextTypes(t *testing.T) {
	tests := []struct {
		name     string
		dataType string
	}{
		{"uuid native", "uuid"},
		{"integer", "integer"},
		{"bigint", "bigint"},
		{"boolean", "boolean"},
		{"timestamp", "timestamp"},
		{"date", "date"},
		{"jsonb", "jsonb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "uuid_col",
				DataType:   tt.dataType,
				SampleValues: []string{
					"550e8400-e29b-41d4-a716-446655440000",
					"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
				},
			}

			result := detectUUIDTextColumnPattern(col)
			assert.Nil(t, result, "Should NOT detect for non-text type: %s", tt.dataType)
		})
	}
}

func TestDetectUUIDTextColumnPattern_NoSampleValues(t *testing.T) {
	// No sample values available
	col := &models.SchemaColumn{
		ColumnName:   "maybe_uuid",
		DataType:     "varchar(36)",
		SampleValues: nil,
	}

	result := detectUUIDTextColumnPattern(col)
	assert.Nil(t, result, "Should NOT detect when no sample values available")
}

func TestDetectUUIDTextColumnPattern_EmptySampleValues(t *testing.T) {
	// Empty sample values array
	col := &models.SchemaColumn{
		ColumnName:   "maybe_uuid",
		DataType:     "varchar(36)",
		SampleValues: []string{},
	}

	result := detectUUIDTextColumnPattern(col)
	assert.Nil(t, result, "Should NOT detect when sample values are empty")
}

func TestDetectUUIDTextColumnPattern_BelowThreshold(t *testing.T) {
	// Only 50% match (below 99% threshold)
	col := &models.SchemaColumn{
		ColumnName: "mixed_id",
		DataType:   "varchar(50)",
		SampleValues: []string{
			"550e8400-e29b-41d4-a716-446655440000", // UUID
			"not-a-uuid-at-all",                    // Not UUID
			"6ba7b810-9dad-11d1-80b4-00c04fd430c8", // UUID
			"another-random-string",                // Not UUID
		},
	}

	result := detectUUIDTextColumnPattern(col)
	assert.Nil(t, result, "Should NOT detect when below 99% threshold (50%)")
}

func TestDetectUUIDTextColumnPattern_SingleNonUUID(t *testing.T) {
	// 98% match (still below 99% threshold with 50 samples, 1 non-match)
	samples := make([]string, 100)
	for i := 0; i < 99; i++ {
		samples[i] = "550e8400-e29b-41d4-a716-446655440000"
	}
	samples[99] = "not-a-uuid" // 1% non-match -> 99% match (at threshold)

	col := &models.SchemaColumn{
		ColumnName:   "almost_all_uuids",
		DataType:     "varchar(50)",
		SampleValues: samples,
	}

	result := detectUUIDTextColumnPattern(col)
	// 99% is exactly at the threshold (>99% required), so should not match
	assert.Nil(t, result, "Should NOT detect at exactly 99% (need >99%)")
}

func TestDetectUUIDTextColumnPattern_AllMatch(t *testing.T) {
	// 100% match (above 99% threshold)
	samples := make([]string, 100)
	for i := 0; i < 100; i++ {
		samples[i] = "550e8400-e29b-41d4-a716-446655440000"
	}

	col := &models.SchemaColumn{
		ColumnName:   "all_uuids",
		DataType:     "varchar(36)",
		SampleValues: samples,
	}

	result := detectUUIDTextColumnPattern(col)
	require.NotNil(t, result, "Should detect at 100% match rate")
	assert.InDelta(t, 100.0, result.MatchRate, 0.1)
}

func TestDetectUUIDTextColumnPattern_InvalidUUIDFormats(t *testing.T) {
	tests := []struct {
		name   string
		values []string
	}{
		{"missing hyphens", []string{"550e8400e29b41d4a716446655440000"}},
		{"wrong hyphen positions", []string{"550e8-400e29b41d4-a716-446655440000"}},
		{"too short", []string{"550e8400-e29b-41d4-a716-446655440"}},
		{"too long", []string{"550e8400-e29b-41d4-a716-4466554400000"}},
		{"invalid characters", []string{"550e8400-e29b-41d4-a716-44665544GGGG"}},
		{"with braces", []string{"{550e8400-e29b-41d4-a716-446655440000}"}},
		{"urn format", []string{"urn:uuid:550e8400-e29b-41d4-a716-446655440000"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName:   "invalid_format",
				DataType:     "varchar(50)",
				SampleValues: tt.values,
			}

			result := detectUUIDTextColumnPattern(col)
			assert.Nil(t, result, "Should NOT detect invalid UUID format: %s", tt.name)
		})
	}
}

func TestDetectUUIDTextColumnPattern_SingleValue(t *testing.T) {
	// Single sample value that is a UUID (100% match)
	col := &models.SchemaColumn{
		ColumnName:   "single_uuid",
		DataType:     "text",
		SampleValues: []string{"550e8400-e29b-41d4-a716-446655440000"},
	}

	result := detectUUIDTextColumnPattern(col)
	require.NotNil(t, result, "Should detect single UUID value")
	assert.Equal(t, "uuid_text", result.SemanticType)
}

func TestDetectUUIDTextColumnPattern_NilUUIDs(t *testing.T) {
	// v1 UUID (time-based)
	col := &models.SchemaColumn{
		ColumnName: "v1_uuid",
		DataType:   "varchar(36)",
		SampleValues: []string{
			"6ba7b810-9dad-11d1-80b4-00c04fd430c8",
			"6ba7b811-9dad-11d1-80b4-00c04fd430c8",
		},
	}

	result := detectUUIDTextColumnPattern(col)
	require.NotNil(t, result, "Should detect v1 UUIDs")
	assert.Equal(t, "uuid_text", result.SemanticType)
}

// =============================================================================
// isTextType Tests
// =============================================================================

func TestIsTextType_ValidTypes(t *testing.T) {
	validTypes := []string{
		"varchar", "VARCHAR", "varchar(255)",
		"text", "TEXT",
		"char", "CHAR", "char(36)",
		"character varying", "CHARACTER VARYING",
		"nvarchar", "NVARCHAR",
		"character", "CHARACTER",
	}

	for _, typ := range validTypes {
		t.Run(typ, func(t *testing.T) {
			assert.True(t, isTextType(typ), "Should be valid text type: %s", typ)
		})
	}
}

func TestIsTextType_InvalidTypes(t *testing.T) {
	invalidTypes := []string{
		"uuid", "UUID",
		"integer", "INTEGER",
		"bigint", "BIGINT",
		"boolean", "BOOLEAN",
		"timestamp", "TIMESTAMP",
		"date", "DATE",
		"jsonb", "JSONB",
		"bytea", "BYTEA",
		"double precision",
		"numeric",
	}

	for _, typ := range invalidTypes {
		t.Run(typ, func(t *testing.T) {
			assert.False(t, isTextType(typ), "Should NOT be valid text type: %s", typ)
		})
	}
}

// =============================================================================
// detectTimestampScalePattern Tests
// =============================================================================

func TestDetectTimestampScalePattern_Seconds(t *testing.T) {
	// Typical Unix timestamp in seconds (10 digits)
	col := &models.SchemaColumn{
		ColumnName: "created_at",
		DataType:   "bigint",
		SampleValues: []string{
			"1704067200", // 2024-01-01 00:00:00 UTC
			"1704153600", // 2024-01-02 00:00:00 UTC
			"1704240000", // 2024-01-03 00:00:00 UTC
		},
	}

	result := detectTimestampScalePattern(col)

	require.NotNil(t, result, "Should detect timestamp pattern")
	assert.Contains(t, result.Description, "Unix timestamp in seconds")
	assert.Contains(t, result.Description, "since Unix epoch")
	assert.Equal(t, "unix_timestamp_seconds", result.SemanticType)
	assert.Equal(t, models.ColumnRoleAttribute, result.Role)
	assert.Equal(t, "seconds", result.Scale)
}

func TestDetectTimestampScalePattern_Milliseconds(t *testing.T) {
	// Unix timestamp in milliseconds (13 digits)
	col := &models.SchemaColumn{
		ColumnName: "updated_at",
		DataType:   "bigint",
		SampleValues: []string{
			"1704067200000", // 2024-01-01 00:00:00.000 UTC
			"1704153600000", // 2024-01-02 00:00:00.000 UTC
			"1704240000123", // 2024-01-03 00:00:00.123 UTC
		},
	}

	result := detectTimestampScalePattern(col)

	require.NotNil(t, result, "Should detect milliseconds timestamp pattern")
	assert.Contains(t, result.Description, "Unix timestamp in milliseconds")
	assert.Equal(t, "unix_timestamp_milliseconds", result.SemanticType)
	assert.Equal(t, "milliseconds", result.Scale)
}

func TestDetectTimestampScalePattern_Microseconds(t *testing.T) {
	// Unix timestamp in microseconds (16 digits)
	col := &models.SchemaColumn{
		ColumnName: "event_time",
		DataType:   "bigint",
		SampleValues: []string{
			"1704067200000000", // 2024-01-01 00:00:00.000000 UTC
			"1704153600123456", // 2024-01-02 00:00:00.123456 UTC
		},
	}

	result := detectTimestampScalePattern(col)

	require.NotNil(t, result, "Should detect microseconds timestamp pattern")
	assert.Contains(t, result.Description, "Unix timestamp in microseconds")
	assert.Equal(t, "unix_timestamp_microseconds", result.SemanticType)
	assert.Equal(t, "microseconds", result.Scale)
}

func TestDetectTimestampScalePattern_Nanoseconds(t *testing.T) {
	// Unix timestamp in nanoseconds (19 digits)
	col := &models.SchemaColumn{
		ColumnName: "marker_at",
		DataType:   "bigint",
		SampleValues: []string{
			"1704067200000000000", // 2024-01-01 00:00:00.000000000 UTC
			"1704153600123456789", // 2024-01-02 00:00:00.123456789 UTC
		},
	}

	result := detectTimestampScalePattern(col)

	require.NotNil(t, result, "Should detect nanoseconds timestamp pattern")
	assert.Contains(t, result.Description, "Unix timestamp in nanoseconds")
	assert.Equal(t, "unix_timestamp_nanoseconds", result.SemanticType)
	assert.Equal(t, "nanoseconds", result.Scale)
}

func TestDetectTimestampScalePattern_MarkerAtWithCursorHint(t *testing.T) {
	// marker_at columns should get cursor-based pagination hint
	col := &models.SchemaColumn{
		ColumnName: "marker_at",
		DataType:   "bigint",
		SampleValues: []string{
			"1704067200000000000",
			"1704153600000000000",
		},
	}

	result := detectTimestampScalePattern(col)

	require.NotNil(t, result, "Should detect timestamp pattern for marker_at")
	assert.Contains(t, result.Description, "cursor-based pagination", "marker_at should mention cursor-based pagination")
}

func TestDetectTimestampScalePattern_CreatedAtWithRecordHint(t *testing.T) {
	// created_at columns should get record timestamp hint
	col := &models.SchemaColumn{
		ColumnName: "created_at",
		DataType:   "bigint",
		SampleValues: []string{
			"1704067200",
			"1704153600",
		},
	}

	result := detectTimestampScalePattern(col)

	require.NotNil(t, result, "Should detect timestamp pattern for created_at")
	assert.Contains(t, result.Description, "Record timestamp", "created_at should mention record timestamp")
}

func TestDetectTimestampScalePattern_TimeColumnName(t *testing.T) {
	// Column names containing 'time' should be detected
	tests := []struct {
		name       string
		columnName string
	}{
		{"event_time", "event_time"},
		{"start_time", "start_time"},
		{"end_time", "end_time"},
		{"timestamp", "timestamp"},
		{"process_time", "process_time"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "bigint",
				SampleValues: []string{
					"1704067200",
					"1704153600",
				},
			}

			result := detectTimestampScalePattern(col)
			require.NotNil(t, result, "Should detect timestamp pattern for: %s", tt.columnName)
			assert.Equal(t, "unix_timestamp_seconds", result.SemanticType)
		})
	}
}

func TestDetectTimestampScalePattern_AtSuffixColumnNames(t *testing.T) {
	// Column names ending with '_at' should be detected
	tests := []struct {
		name       string
		columnName string
	}{
		{"created_at", "created_at"},
		{"updated_at", "updated_at"},
		{"deleted_at", "deleted_at"},
		{"started_at", "started_at"},
		{"finished_at", "finished_at"},
		{"expires_at", "expires_at"},
		{"marker_at", "marker_at"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "bigint",
				SampleValues: []string{
					"1704067200",
					"1704153600",
				},
			}

			result := detectTimestampScalePattern(col)
			require.NotNil(t, result, "Should detect timestamp pattern for: %s", tt.columnName)
		})
	}
}

func TestDetectTimestampScalePattern_NonTimestampColumnNames(t *testing.T) {
	// Column names that don't match timestamp patterns should not be detected
	tests := []struct {
		name       string
		columnName string
	}{
		{"user_id", "user_id"},
		{"amount", "amount"},
		{"count", "count"},
		{"status", "status"},
		{"version", "version"},
		{"flat", "flat"},     // ends with 'at' but not '_at'
		{"format", "format"}, // ends with 'at' but not '_at'
		{"combat", "combat"}, // ends with 'at' but not '_at'
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "bigint",
				SampleValues: []string{
					"1704067200",
					"1704153600",
				},
			}

			result := detectTimestampScalePattern(col)
			assert.Nil(t, result, "Should NOT detect timestamp pattern for: %s", tt.columnName)
		})
	}
}

func TestDetectTimestampScalePattern_NonBigintTypes(t *testing.T) {
	// Non-bigint types should not be detected
	tests := []struct {
		name     string
		dataType string
	}{
		{"integer", "integer"},
		{"int4", "int4"},
		{"smallint", "smallint"},
		{"varchar", "varchar(255)"},
		{"text", "text"},
		{"timestamp", "timestamp"},
		{"timestamptz", "timestamptz"},
		{"date", "date"},
		{"boolean", "boolean"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "created_at",
				DataType:   tt.dataType,
				SampleValues: []string{
					"1704067200",
					"1704153600",
				},
			}

			result := detectTimestampScalePattern(col)
			assert.Nil(t, result, "Should NOT detect timestamp pattern for type: %s", tt.dataType)
		})
	}
}

func TestDetectTimestampScalePattern_BigintTypeVariants(t *testing.T) {
	// Various bigint type representations should be detected
	tests := []struct {
		name     string
		dataType string
	}{
		{"bigint", "bigint"},
		{"BIGINT", "BIGINT"},
		{"int8", "int8"},
		{"INT8", "INT8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "created_at",
				DataType:   tt.dataType,
				SampleValues: []string{
					"1704067200",
					"1704153600",
				},
			}

			result := detectTimestampScalePattern(col)
			require.NotNil(t, result, "Should detect timestamp pattern for type: %s", tt.dataType)
		})
	}
}

func TestDetectTimestampScalePattern_NoSampleValues(t *testing.T) {
	// No sample values should not be detected
	col := &models.SchemaColumn{
		ColumnName:   "created_at",
		DataType:     "bigint",
		SampleValues: []string{},
	}

	result := detectTimestampScalePattern(col)
	assert.Nil(t, result, "Should NOT detect timestamp pattern without sample values")
}

func TestDetectTimestampScalePattern_NilSampleValues(t *testing.T) {
	// Nil sample values should not be detected
	col := &models.SchemaColumn{
		ColumnName:   "created_at",
		DataType:     "bigint",
		SampleValues: nil,
	}

	result := detectTimestampScalePattern(col)
	assert.Nil(t, result, "Should NOT detect timestamp pattern with nil sample values")
}

func TestDetectTimestampScalePattern_MixedDigitLengths(t *testing.T) {
	// If less than 80% of values have consistent digit length, should not detect
	col := &models.SchemaColumn{
		ColumnName: "created_at",
		DataType:   "bigint",
		SampleValues: []string{
			"1704067200",       // 10 digits (seconds)
			"1704067200000",    // 13 digits (milliseconds)
			"1704067200000000", // 16 digits (microseconds)
			"1704067200",       // 10 digits
			"1704067200000",    // 13 digits
		},
	}

	result := detectTimestampScalePattern(col)
	assert.Nil(t, result, "Should NOT detect timestamp pattern with mixed digit lengths")
}

func TestDetectTimestampScalePattern_InvalidDigitLengths(t *testing.T) {
	// Values with digit lengths outside known timestamp ranges
	tests := []struct {
		name   string
		values []string
	}{
		{"too_short", []string{"12345", "67890", "11111"}},                           // 5 digits
		{"too_long", []string{"12345678901234567890123", "98765432109876543210123"}}, // 23 digits
		{"8_digits", []string{"12345678", "87654321"}},                               // 8 digits - just below seconds range
		{"21_digits", []string{"123456789012345678901", "987654321098765432109"}},    // 21 digits - just above nanoseconds range
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName:   "created_at",
				DataType:     "bigint",
				SampleValues: tt.values,
			}

			result := detectTimestampScalePattern(col)
			assert.Nil(t, result, "Should NOT detect timestamp pattern for digit lengths: %v", tt.values)
		})
	}
}

func TestDetectTimestampScalePattern_NegativeValues(t *testing.T) {
	// Negative values should be ignored (but not cause failure)
	col := &models.SchemaColumn{
		ColumnName: "created_at",
		DataType:   "bigint",
		SampleValues: []string{
			"-1704067200", // negative
			"1704067200",  // positive, 10 digits
			"1704153600",  // positive, 10 digits
		},
	}

	result := detectTimestampScalePattern(col)
	// Should still detect based on the valid positive values (2 out of 3 = 66%, but negative is skipped)
	// With 2 valid values both being 10 digits = 100% of valid values
	require.NotNil(t, result, "Should detect timestamp pattern ignoring negative values")
	assert.Equal(t, "seconds", result.Scale)
}

func TestDetectTimestampScalePattern_EmptyStringValues(t *testing.T) {
	// Empty string values should be ignored
	col := &models.SchemaColumn{
		ColumnName: "created_at",
		DataType:   "bigint",
		SampleValues: []string{
			"",
			"1704067200",
			"1704153600",
		},
	}

	result := detectTimestampScalePattern(col)
	require.NotNil(t, result, "Should detect timestamp pattern ignoring empty values")
	assert.Equal(t, "seconds", result.Scale)
}

func TestDetectTimestampScalePattern_CaseInsensitiveColumnName(t *testing.T) {
	// Column name matching should be case-insensitive
	tests := []struct {
		name       string
		columnName string
	}{
		{"CREATED_AT", "CREATED_AT"},
		{"Created_At", "Created_At"},
		{"EVENT_TIME", "EVENT_TIME"},
		{"Event_Time", "Event_Time"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "bigint",
				SampleValues: []string{
					"1704067200",
					"1704153600",
				},
			}

			result := detectTimestampScalePattern(col)
			require.NotNil(t, result, "Should detect timestamp pattern for: %s (case-insensitive)", tt.columnName)
		})
	}
}

func TestDetectTimestampScalePattern_DigitLengthBoundaries(t *testing.T) {
	// Test boundary values for each scale range
	tests := []struct {
		name          string
		digits        int
		expectedScale string
	}{
		{"9_digits_seconds", 9, "seconds"},
		{"10_digits_seconds", 10, "seconds"},
		{"11_digits_seconds", 11, "seconds"},
		{"12_digits_milliseconds", 12, "milliseconds"},
		{"13_digits_milliseconds", 13, "milliseconds"},
		{"14_digits_milliseconds", 14, "milliseconds"},
		{"15_digits_microseconds", 15, "microseconds"},
		{"16_digits_microseconds", 16, "microseconds"},
		{"17_digits_microseconds", 17, "microseconds"},
		{"18_digits_nanoseconds", 18, "nanoseconds"},
		{"19_digits_nanoseconds", 19, "nanoseconds"},
		{"20_digits_nanoseconds", 20, "nanoseconds"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Generate a value with exactly the required number of digits
			value := "1"
			for i := 1; i < tt.digits; i++ {
				value += "0"
			}

			col := &models.SchemaColumn{
				ColumnName: "created_at",
				DataType:   "bigint",
				SampleValues: []string{
					value,
					value,
				},
			}

			result := detectTimestampScalePattern(col)
			require.NotNil(t, result, "Should detect timestamp pattern for %d digits", tt.digits)
			assert.Equal(t, tt.expectedScale, result.Scale, "Expected scale %s for %d digits", tt.expectedScale, tt.digits)
		})
	}
}

// =============================================================================
// isBigintType Tests
// =============================================================================

func TestIsBigintType_ValidTypes(t *testing.T) {
	validTypes := []string{
		"bigint", "BIGINT", "Bigint",
		"int8", "INT8", "Int8",
	}

	for _, typ := range validTypes {
		t.Run(typ, func(t *testing.T) {
			assert.True(t, isBigintType(typ), "Should be valid bigint type: %s", typ)
		})
	}
}

func TestIsBigintType_InvalidTypes(t *testing.T) {
	invalidTypes := []string{
		"integer", "INTEGER",
		"int", "INT",
		"int4", "INT4",
		"smallint", "SMALLINT",
		"numeric", "NUMERIC",
		"varchar", "text", "boolean",
		"timestamp", "date",
		"double precision", "real", "float",
	}

	for _, typ := range invalidTypes {
		t.Run(typ, func(t *testing.T) {
			assert.False(t, isBigintType(typ), "Should NOT be valid bigint type: %s", typ)
		})
	}
}

// =============================================================================
// countDigits Tests
// =============================================================================

func TestCountDigits(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"12345", 5},
		{"1704067200", 10},
		{"1704067200000", 13},
		{"1704067200000000", 16},
		{"1704067200000000000", 19},
		{"", 0},
		{"abc", 0},
		{"-123", 3},
		{"12.34", 4},
		{"1,234,567", 7},
		{"  123  ", 3},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := countDigits(tt.input)
			assert.Equal(t, tt.expected, result, "countDigits(%q) should be %d", tt.input, tt.expected)
		})
	}
}

// =============================================================================
// inferTimestampScale Tests
// =============================================================================

func TestInferTimestampScale_EmptyValues(t *testing.T) {
	result := inferTimestampScale([]string{})
	assert.Equal(t, "", result)
}

func TestInferTimestampScale_AllEmptyStrings(t *testing.T) {
	result := inferTimestampScale([]string{"", "", ""})
	assert.Equal(t, "", result)
}

func TestInferTimestampScale_AllNegative(t *testing.T) {
	result := inferTimestampScale([]string{"-1", "-2", "-3"})
	assert.Equal(t, "", result)
}

func TestInferTimestampScale_ConsistentLength(t *testing.T) {
	// All 10-digit values should return "seconds"
	result := inferTimestampScale([]string{"1704067200", "1704153600", "1704240000"})
	assert.Equal(t, "seconds", result)
}

func TestInferTimestampScale_80PercentThreshold(t *testing.T) {
	// 4 out of 5 = 80% with same length should pass
	values := []string{
		"1704067200", // 10 digits
		"1704153600", // 10 digits
		"1704240000", // 10 digits
		"1704326400", // 10 digits
		"12345",      // 5 digits
	}
	result := inferTimestampScale(values)
	assert.Equal(t, "seconds", result)
}

func TestInferTimestampScale_Below80PercentThreshold(t *testing.T) {
	// 3 out of 5 = 60% with same length should fail
	values := []string{
		"1704067200",     // 10 digits
		"1704153600",     // 10 digits
		"1704240000",     // 10 digits
		"12345678901234", // 14 digits
		"1234567890123",  // 13 digits
	}
	result := inferTimestampScale(values)
	assert.Equal(t, "", result)
}

// =============================================================================
// generateTimestampScaleDescription Tests
// =============================================================================

func TestGenerateTimestampScaleDescription_AllScales(t *testing.T) {
	tests := []struct {
		scale           string
		expectedContain string
	}{
		{"seconds", "seconds since Unix epoch (1970-01-01)"},
		{"milliseconds", "milliseconds since Unix epoch"},
		{"microseconds", "microseconds since Unix epoch"},
		{"nanoseconds", "nanoseconds since Unix epoch"},
	}

	for _, tt := range tests {
		t.Run(tt.scale, func(t *testing.T) {
			result := generateTimestampScaleDescription(tt.scale, "created_at")
			assert.Contains(t, result, tt.expectedContain)
		})
	}
}

func TestGenerateTimestampScaleDescription_MarkerColumn(t *testing.T) {
	result := generateTimestampScaleDescription("nanoseconds", "marker_at")
	assert.Contains(t, result, "cursor-based pagination")
}

func TestGenerateTimestampScaleDescription_CursorColumn(t *testing.T) {
	result := generateTimestampScaleDescription("nanoseconds", "cursor_time")
	assert.Contains(t, result, "cursor-based pagination")
}

func TestGenerateTimestampScaleDescription_CreatedColumn(t *testing.T) {
	result := generateTimestampScaleDescription("seconds", "created_at")
	assert.Contains(t, result, "Record timestamp")
}

func TestGenerateTimestampScaleDescription_UpdatedColumn(t *testing.T) {
	result := generateTimestampScaleDescription("seconds", "updated_at")
	assert.Contains(t, result, "Record timestamp")
}

func TestGenerateTimestampScaleDescription_UnknownScale(t *testing.T) {
	result := generateTimestampScaleDescription("unknown", "created_at")
	assert.Equal(t, "", result)
}

// =============================================================================
// FK Column Pattern Detection Tests
// =============================================================================

func TestDetectFKColumnPattern_DBConstraint(t *testing.T) {
	col := &models.SchemaColumn{
		ColumnName: "user_id",
		DataType:   "uuid",
	}

	fkInfo := &FKRelationshipInfo{
		TargetTable:     "users",
		TargetColumn:    "id",
		DetectionMethod: models.DetectionMethodForeignKey,
		Confidence:      1.0,
		IsDBConstraint:  true,
	}

	result := detectFKColumnPattern(col, fkInfo)

	require.NotNil(t, result, "Should detect FK column pattern for DB constraint")
	assert.Equal(t, "Foreign key to users.id.", result.Description)
	assert.Equal(t, "foreign_key", result.SemanticType)
	assert.Equal(t, models.ColumnRoleIdentifier, result.Role)
	assert.Equal(t, 1.0, result.Confidence)
}

func TestDetectFKColumnPattern_LogicalFK(t *testing.T) {
	col := &models.SchemaColumn{
		ColumnName: "host_id",
		DataType:   "uuid",
	}

	fkInfo := &FKRelationshipInfo{
		TargetTable:     "users",
		TargetColumn:    "user_id",
		DetectionMethod: models.DetectionMethodPKMatch,
		Confidence:      0.9,
		IsDBConstraint:  false,
	}

	result := detectFKColumnPattern(col, fkInfo)

	require.NotNil(t, result, "Should detect FK column pattern for logical FK")
	assert.Contains(t, result.Description, "Foreign key to users.user_id")
	assert.Contains(t, result.Description, "90% confidence")
	assert.Contains(t, result.Description, "No database constraint")
	assert.Contains(t, result.Description, "logical reference validated via data overlap")
	assert.Equal(t, "logical_foreign_key", result.SemanticType)
	assert.Equal(t, models.ColumnRoleIdentifier, result.Role)
	assert.Equal(t, 0.9, result.Confidence)
}

func TestDetectFKColumnPattern_NilFKInfo(t *testing.T) {
	col := &models.SchemaColumn{
		ColumnName: "user_id",
		DataType:   "uuid",
	}

	result := detectFKColumnPattern(col, nil)

	assert.Nil(t, result, "Should return nil when no FK info is provided")
}

func TestDetectFKColumnPattern_DifferentTargets(t *testing.T) {
	tests := []struct {
		name         string
		targetTable  string
		targetColumn string
		wantContains string
	}{
		{"accounts table", "accounts", "id", "Foreign key to accounts.id"},
		{"orders table", "orders", "order_id", "Foreign key to orders.order_id"},
		{"products table", "products", "product_uuid", "Foreign key to products.product_uuid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "ref_id",
				DataType:   "uuid",
			}

			fkInfo := &FKRelationshipInfo{
				TargetTable:     tt.targetTable,
				TargetColumn:    tt.targetColumn,
				DetectionMethod: models.DetectionMethodPKMatch,
				Confidence:      0.9,
				IsDBConstraint:  false,
			}

			result := detectFKColumnPattern(col, fkInfo)

			require.NotNil(t, result)
			assert.Contains(t, result.Description, tt.wantContains)
		})
	}
}

func TestFKRelationshipInfo_Fields(t *testing.T) {
	fkInfo := &FKRelationshipInfo{
		TargetTable:     "users",
		TargetColumn:    "id",
		DetectionMethod: models.DetectionMethodPKMatch,
		Confidence:      0.85,
		IsDBConstraint:  false,
	}

	assert.Equal(t, "users", fkInfo.TargetTable)
	assert.Equal(t, "id", fkInfo.TargetColumn)
	assert.Equal(t, models.DetectionMethodPKMatch, fkInfo.DetectionMethod)
	assert.Equal(t, 0.85, fkInfo.Confidence)
	assert.False(t, fkInfo.IsDBConstraint)
}

func TestColumnEnrichmentService_convertToColumnDetails_WithFKPatternDetection(t *testing.T) {
	logger := zap.NewNop()
	service := &columnEnrichmentService{
		logger: logger,
	}

	columns := []*models.SchemaColumn{
		{ColumnName: "id", DataType: "uuid", IsPrimaryKey: true},
		{ColumnName: "user_id", DataType: "uuid"},
		{ColumnName: "host_id", DataType: "uuid"},
		{ColumnName: "status", DataType: "varchar"},
	}

	enrichments := []columnEnrichment{
		{Name: "id", Description: "Primary key", Role: "identifier", SemanticType: "identifier"},
		{Name: "user_id", Description: "LLM description for user_id", Role: "identifier", SemanticType: "identifier"},
		{Name: "host_id", Description: "LLM description for host_id", Role: "identifier", SemanticType: "identifier"},
		{Name: "status", Description: "Status field", Role: "dimension", SemanticType: "status"},
	}

	// Simple FK info for LLM context
	fkInfo := map[string]string{
		"user_id": "users",
		"host_id": "users",
	}

	// Detailed FK info for pattern detection
	fkDetailedInfo := map[string]*FKRelationshipInfo{
		"user_id": {
			TargetTable:     "users",
			TargetColumn:    "id",
			DetectionMethod: models.DetectionMethodForeignKey, // DB constraint
			Confidence:      1.0,
			IsDBConstraint:  true,
		},
		"host_id": {
			TargetTable:     "users",
			TargetColumn:    "user_id",
			DetectionMethod: models.DetectionMethodPKMatch, // Inferred
			Confidence:      0.9,
			IsDBConstraint:  false,
		},
	}

	enumSamples := map[string][]string{}
	enumDefs := []models.EnumDefinition{}
	enumDistributions := map[string]*datasource.EnumDistributionResult{}

	details := service.convertToColumnDetails("engagements", enrichments, columns, fkInfo, fkDetailedInfo, enumSamples, enumDefs, enumDistributions)

	require.Equal(t, 4, len(details))

	// Check user_id - DB FK constraint
	userIDCol := findColumnDetail(details, "user_id")
	require.NotNil(t, userIDCol)
	assert.True(t, userIDCol.IsForeignKey)
	assert.Equal(t, "users", userIDCol.ForeignTable)
	assert.Equal(t, "Foreign key to users.id.", userIDCol.Description)
	assert.Equal(t, "foreign_key", userIDCol.SemanticType)
	assert.Equal(t, models.ColumnRoleIdentifier, userIDCol.Role)

	// Check host_id - Logical FK (inferred via pk_match)
	hostIDCol := findColumnDetail(details, "host_id")
	require.NotNil(t, hostIDCol)
	assert.True(t, hostIDCol.IsForeignKey)
	assert.Equal(t, "users", hostIDCol.ForeignTable)
	assert.Contains(t, hostIDCol.Description, "Foreign key to users.user_id")
	assert.Contains(t, hostIDCol.Description, "90% confidence")
	assert.Contains(t, hostIDCol.Description, "No database constraint")
	assert.Equal(t, "logical_foreign_key", hostIDCol.SemanticType)
	assert.Equal(t, models.ColumnRoleIdentifier, hostIDCol.Role)

	// Check status - should retain LLM enrichment (no FK pattern applies)
	statusCol := findColumnDetail(details, "status")
	require.NotNil(t, statusCol)
	assert.False(t, statusCol.IsForeignKey)
	assert.Equal(t, "Status field", statusCol.Description)
	assert.Equal(t, "status", statusCol.SemanticType)
}

// ============================================================================
// Role Detection Tests
// ============================================================================

func Test_detectRoleFromColumnName(t *testing.T) {
	tests := []struct {
		name         string
		columnName   string
		expectedRole string
		expectedDesc string
		expectNil    bool
	}{
		// User roles - content/marketplace platforms
		{
			name:         "host_id detects host role",
			columnName:   "host_id",
			expectedRole: "host",
			expectedDesc: "content provider",
		},
		{
			name:         "visitor_id detects visitor role",
			columnName:   "visitor_id",
			expectedRole: "visitor",
			expectedDesc: "content consumer",
		},
		{
			name:         "host_user_id detects host role",
			columnName:   "host_user_id",
			expectedRole: "host",
			expectedDesc: "content provider",
		},

		// User roles - ownership
		{
			name:         "creator_id detects creator role",
			columnName:   "creator_id",
			expectedRole: "creator",
			expectedDesc: "entity creator",
		},
		{
			name:         "owner_id detects owner role",
			columnName:   "owner_id",
			expectedRole: "owner",
			expectedDesc: "entity owner",
		},

		// User roles - messaging/transfers
		{
			name:         "sender_id detects sender role",
			columnName:   "sender_id",
			expectedRole: "sender",
			expectedDesc: "message sender",
		},
		{
			name:         "recipient_id detects recipient role",
			columnName:   "recipient_id",
			expectedRole: "recipient",
			expectedDesc: "message recipient",
		},

		// User roles - financial transactions
		{
			name:         "payer_id detects payer role",
			columnName:   "payer_id",
			expectedRole: "payer",
			expectedDesc: "payment source",
		},
		{
			name:         "payee_id detects payee role",
			columnName:   "payee_id",
			expectedRole: "payee",
			expectedDesc: "payment recipient",
		},
		{
			name:         "payer_user_id detects payer role",
			columnName:   "payer_user_id",
			expectedRole: "payer",
			expectedDesc: "payment source",
		},

		// User roles - e-commerce
		{
			name:         "buyer_id detects buyer role",
			columnName:   "buyer_id",
			expectedRole: "buyer",
			expectedDesc: "purchasing party",
		},
		{
			name:         "seller_id detects seller role",
			columnName:   "seller_id",
			expectedRole: "seller",
			expectedDesc: "selling party",
		},

		// Account roles
		{
			name:         "source_account_id detects source role",
			columnName:   "source_account_id",
			expectedRole: "source",
			expectedDesc: "source account",
		},
		{
			name:         "destination_account_id detects destination role",
			columnName:   "destination_account_id",
			expectedRole: "destination",
			expectedDesc: "destination account",
		},
		{
			name:         "from_account_id detects from role",
			columnName:   "from_account_id",
			expectedRole: "from",
			expectedDesc: "originating account",
		},
		{
			name:         "to_account_id detects to role",
			columnName:   "to_account_id",
			expectedRole: "to",
			expectedDesc: "receiving account",
		},

		// Generic references (no role)
		{
			name:       "user_id has no role",
			columnName: "user_id",
			expectNil:  true,
		},
		{
			name:       "account_id has no role",
			columnName: "account_id",
			expectNil:  true,
		},
		{
			name:       "order_id has no role",
			columnName: "order_id",
			expectNil:  true,
		},

		// Other columns
		{
			name:       "status has no role",
			columnName: "status",
			expectNil:  true,
		},
		{
			name:       "created_at has no role",
			columnName: "created_at",
			expectNil:  true,
		},
		{
			name:       "amount has no role",
			columnName: "amount",
			expectNil:  true,
		},

		// Edge cases - partial matches should NOT detect roles
		{
			name:       "ghost_id does not match host",
			columnName: "ghost_id",
			expectNil:  true,
		},

		// Case insensitivity
		{
			name:         "HOST_ID uppercase detects host role",
			columnName:   "HOST_ID",
			expectedRole: "host",
			expectedDesc: "content provider",
		},
		{
			name:         "Host_User_Id mixed case detects host role",
			columnName:   "Host_User_Id",
			expectedRole: "host",
			expectedDesc: "content provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectRoleFromColumnName(tt.columnName)

			if tt.expectNil {
				assert.Nil(t, result, "expected nil for column %s", tt.columnName)
				return
			}

			require.NotNil(t, result, "expected role detection for column %s", tt.columnName)
			assert.Equal(t, tt.expectedRole, result.Role, "role mismatch for column %s", tt.columnName)
			assert.Equal(t, tt.expectedDesc, result.Description, "description mismatch for column %s", tt.columnName)
		})
	}
}

func Test_detectRolesInTable(t *testing.T) {
	tests := []struct {
		name          string
		fkInfo        map[string]string
		expectedRoles map[string]string
	}{
		{
			name: "detects multiple roles to same table",
			fkInfo: map[string]string{
				"host_id":    "users",
				"visitor_id": "users",
				"order_id":   "orders",
			},
			expectedRoles: map[string]string{
				"host_id":    "host",
				"visitor_id": "visitor",
			},
		},
		{
			name: "detects account roles",
			fkInfo: map[string]string{
				"source_account_id": "accounts",
				"dest_account_id":   "accounts",
			},
			expectedRoles: map[string]string{
				"source_account_id": "source",
			},
		},
		{
			name: "no roles detected for generic FKs",
			fkInfo: map[string]string{
				"user_id":    "users",
				"account_id": "accounts",
				"order_id":   "orders",
			},
			expectedRoles: map[string]string{},
		},
		{
			name:          "empty FK info returns empty roles",
			fkInfo:        map[string]string{},
			expectedRoles: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectRolesInTable(tt.fkInfo)

			assert.Equal(t, len(tt.expectedRoles), len(result), "expected %d roles, got %d", len(tt.expectedRoles), len(result))

			for colName, expectedRole := range tt.expectedRoles {
				detectedRole, ok := result[colName]
				require.True(t, ok, "expected role for column %s", colName)
				assert.Equal(t, expectedRole, detectedRole.Role, "role mismatch for column %s", colName)
			}
		})
	}
}

func Test_detectFKColumnPattern_WithRoleDetection(t *testing.T) {
	tests := []struct {
		name               string
		columnName         string
		fkInfo             *FKRelationshipInfo
		expectRole         string
		expectRoleInDesc   bool
		expectDescContains []string
	}{
		{
			name:       "host_id with DB constraint includes role in description",
			columnName: "host_id",
			fkInfo: &FKRelationshipInfo{
				TargetTable:    "users",
				TargetColumn:   "id",
				IsDBConstraint: true,
				Confidence:     1.0,
			},
			expectRole:       "host",
			expectRoleInDesc: true,
			expectDescContains: []string{
				"Foreign key to users.id",
				"Role: host",
				"content provider",
			},
		},
		{
			name:       "visitor_id with pk_match includes role in description",
			columnName: "visitor_id",
			fkInfo: &FKRelationshipInfo{
				TargetTable:     "users",
				TargetColumn:    "user_id",
				IsDBConstraint:  false,
				DetectionMethod: "pk_match",
				Confidence:      0.95,
			},
			expectRole:       "visitor",
			expectRoleInDesc: true,
			expectDescContains: []string{
				"Foreign key to users.user_id",
				"95% confidence",
				"Role: visitor",
				"content consumer",
			},
		},
		{
			name:       "payer_id includes payer role",
			columnName: "payer_id",
			fkInfo: &FKRelationshipInfo{
				TargetTable:    "users",
				TargetColumn:   "id",
				IsDBConstraint: true,
				Confidence:     1.0,
			},
			expectRole:       "payer",
			expectRoleInDesc: true,
			expectDescContains: []string{
				"Role: payer",
				"payment source",
			},
		},
		{
			name:       "source_account_id includes source role",
			columnName: "source_account_id",
			fkInfo: &FKRelationshipInfo{
				TargetTable:    "accounts",
				TargetColumn:   "id",
				IsDBConstraint: true,
				Confidence:     1.0,
			},
			expectRole:       "source",
			expectRoleInDesc: true,
			expectDescContains: []string{
				"Role: source",
				"source account",
			},
		},
		{
			name:       "user_id (generic) has no role in description",
			columnName: "user_id",
			fkInfo: &FKRelationshipInfo{
				TargetTable:    "users",
				TargetColumn:   "id",
				IsDBConstraint: true,
				Confidence:     1.0,
			},
			expectRole:       "",
			expectRoleInDesc: false,
			expectDescContains: []string{
				"Foreign key to users.id",
			},
		},
		{
			name:       "nil FK info returns nil",
			columnName: "host_id",
			fkInfo:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
			}

			result := detectFKColumnPattern(col, tt.fkInfo)

			if tt.fkInfo == nil {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, tt.expectRole, result.DetectedRole, "detected role mismatch")

			for _, expected := range tt.expectDescContains {
				assert.Contains(t, result.Description, expected,
					"description should contain '%s', got: %s", expected, result.Description)
			}

			if !tt.expectRoleInDesc {
				assert.NotContains(t, result.Description, "Role:",
					"description should NOT contain role for generic FK")
			}
		})
	}
}

func Test_RolePatterns_Coverage(t *testing.T) {
	// Verify that all roles have descriptions
	for entityType, roles := range RolePatterns {
		for _, role := range roles {
			desc, ok := RoleDescriptions[role]
			assert.True(t, ok, "role %s (entity type: %s) should have a description", role, entityType)
			assert.NotEmpty(t, desc, "role %s should have a non-empty description", role)
		}
	}
}

func Test_FKAssociation_SetFromDetectedRole(t *testing.T) {
	// This tests that convertToColumnDetails properly sets FKAssociation
	// from the detected role when LLM doesn't provide one
	tableName := "engagements"
	columns := []*models.SchemaColumn{
		{ColumnName: "host_id", DataType: "uuid"},
		{ColumnName: "visitor_id", DataType: "uuid"},
		{ColumnName: "user_id", DataType: "uuid"},
	}

	enrichments := []columnEnrichment{
		{Name: "host_id", Description: "Host user", Role: "dimension"},
		{Name: "visitor_id", Description: "Visitor user", Role: "dimension", FKAssociation: nil},
		{Name: "user_id", Description: "Generic user", Role: "dimension"},
	}

	fkInfo := map[string]string{
		"host_id":    "users",
		"visitor_id": "users",
		"user_id":    "users",
	}

	fkDetailedInfo := map[string]*FKRelationshipInfo{
		"host_id": {
			TargetTable:    "users",
			TargetColumn:   "id",
			IsDBConstraint: true,
			Confidence:     1.0,
		},
		"visitor_id": {
			TargetTable:    "users",
			TargetColumn:   "id",
			IsDBConstraint: true,
			Confidence:     1.0,
		},
		"user_id": {
			TargetTable:    "users",
			TargetColumn:   "id",
			IsDBConstraint: true,
			Confidence:     1.0,
		},
	}

	svc := &columnEnrichmentService{}
	details := svc.convertToColumnDetails(
		tableName,
		enrichments,
		columns,
		fkInfo,
		fkDetailedInfo,
		nil, // enumSamples
		nil, // enumDefs
		nil, // enumDistributions
	)

	// Find columns by name
	var hostCol, visitorCol, userCol *models.ColumnDetail
	for i := range details {
		switch details[i].Name {
		case "host_id":
			hostCol = &details[i]
		case "visitor_id":
			visitorCol = &details[i]
		case "user_id":
			userCol = &details[i]
		}
	}

	// host_id should have FKAssociation set to "host"
	require.NotNil(t, hostCol)
	assert.Equal(t, "host", hostCol.FKAssociation, "host_id should have FKAssociation 'host'")
	assert.Contains(t, hostCol.Description, "Role: host", "host_id description should include role")

	// visitor_id should have FKAssociation set to "visitor"
	require.NotNil(t, visitorCol)
	assert.Equal(t, "visitor", visitorCol.FKAssociation, "visitor_id should have FKAssociation 'visitor'")
	assert.Contains(t, visitorCol.Description, "Role: visitor", "visitor_id description should include role")

	// user_id should NOT have FKAssociation (generic reference)
	require.NotNil(t, userCol)
	assert.Empty(t, userCol.FKAssociation, "user_id should NOT have FKAssociation (generic reference)")
	assert.NotContains(t, userCol.Description, "Role:", "user_id description should NOT include role")
}

// ============================================================================
// Boolean Naming Pattern Detection Tests
// ============================================================================

func TestDetectBooleanNamingPattern_TypicalIsPrefix(t *testing.T) {
	// Typical is_ prefixed boolean column
	col := &models.SchemaColumn{
		ColumnName: "is_active",
		DataType:   "boolean",
	}

	result := detectBooleanNamingPattern(col)

	require.NotNil(t, result, "Should detect boolean pattern for is_ prefix")
	assert.Contains(t, result.Description, "Boolean flag")
	assert.Contains(t, result.Description, "Indicates whether")
	assert.Contains(t, result.Description, "active")
	assert.Equal(t, "boolean_flag", result.SemanticType)
	assert.Equal(t, models.ColumnRoleDimension, result.Role)
	assert.Equal(t, "active", result.FeatureName)
	assert.Equal(t, "is_", result.NamingPattern)
}

func TestDetectBooleanNamingPattern_HasPrefix(t *testing.T) {
	// has_ prefixed boolean column
	col := &models.SchemaColumn{
		ColumnName: "has_premium_subscription",
		DataType:   "boolean",
	}

	result := detectBooleanNamingPattern(col)

	require.NotNil(t, result, "Should detect boolean pattern for has_ prefix")
	assert.Contains(t, result.Description, "Boolean flag")
	assert.Contains(t, result.Description, "Indicates whether this entity has")
	assert.Contains(t, result.Description, "premium subscription")
	assert.Equal(t, "boolean_flag", result.SemanticType)
	assert.Equal(t, "premium_subscription", result.FeatureName)
	assert.Equal(t, "has_", result.NamingPattern)
}

func TestDetectBooleanNamingPattern_CanPrefix(t *testing.T) {
	// can_ prefixed boolean column
	col := &models.SchemaColumn{
		ColumnName: "can_edit",
		DataType:   "boolean",
	}

	result := detectBooleanNamingPattern(col)

	require.NotNil(t, result, "Should detect boolean pattern for can_ prefix")
	assert.Contains(t, result.Description, "Boolean flag")
	assert.Contains(t, result.Description, "Indicates whether this entity can")
	assert.Contains(t, result.Description, "edit")
	assert.Equal(t, "can_", result.NamingPattern)
}

func TestDetectBooleanNamingPattern_AllPrefixes(t *testing.T) {
	// Test all supported prefixes
	tests := []struct {
		prefix     string
		columnName string
		expectedIn string
	}{
		{"is_", "is_verified", "Indicates whether"},
		{"has_", "has_access", "Indicates whether this entity has"},
		{"can_", "can_login", "Indicates whether this entity can"},
		{"should_", "should_notify", "Indicates whether this entity should"},
		{"allow_", "allow_marketing", "Indicates whether"},
		{"allows_", "allows_sharing", "Indicates whether"},
		{"needs_", "needs_review", "Indicates whether this entity needs"},
		{"was_", "was_deleted", "Indicates whether this entity was"},
		{"will_", "will_expire", "Indicates whether this entity will"},
	}

	for _, tt := range tests {
		t.Run(tt.prefix, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "boolean",
			}

			result := detectBooleanNamingPattern(col)

			require.NotNil(t, result, "Should detect boolean pattern for %s prefix", tt.prefix)
			assert.Contains(t, result.Description, tt.expectedIn, "Description should contain prefix description")
			assert.Equal(t, tt.prefix, result.NamingPattern)
		})
	}
}

func TestDetectBooleanNamingPattern_NonBooleanPrefix(t *testing.T) {
	// Column names that don't start with known prefixes
	tests := []struct {
		name       string
		columnName string
	}{
		{"regular column", "active"},
		{"status column", "status"},
		{"enabled suffix", "feature_enabled"},
		{"flag suffix", "email_verified_flag"},
		{"different prefix", "do_send_emails"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "boolean",
			}

			result := detectBooleanNamingPattern(col)
			assert.Nil(t, result, "Should not detect for column: %s", tt.columnName)
		})
	}
}

func TestDetectBooleanNamingPattern_BooleanDataTypes(t *testing.T) {
	// Test different boolean data type representations
	tests := []struct {
		name     string
		dataType string
	}{
		{"boolean", "boolean"},
		{"bool", "bool"},
		{"BOOLEAN", "BOOLEAN"},
		{"bit", "bit"},
		{"bit(1)", "bit(1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "is_active",
				DataType:   tt.dataType,
			}

			result := detectBooleanNamingPattern(col)
			require.NotNil(t, result, "Should detect for data type: %s", tt.dataType)
			assert.Equal(t, "boolean_flag", result.SemanticType)
		})
	}
}

func TestDetectBooleanNamingPattern_NonBooleanDataTypes(t *testing.T) {
	// Non-boolean data types should not be detected (unless boolean-like integer)
	tests := []struct {
		name     string
		dataType string
	}{
		{"text", "text"},
		{"varchar", "varchar(10)"},
		{"timestamp", "timestamp"},
		{"date", "date"},
		{"uuid", "uuid"},
		{"jsonb", "jsonb"},
		{"float", "float"},
		{"decimal", "decimal(10,2)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: "is_active",
				DataType:   tt.dataType,
			}

			result := detectBooleanNamingPattern(col)
			assert.Nil(t, result, "Should not detect for non-boolean type: %s", tt.dataType)
		})
	}
}

func TestDetectBooleanNamingPattern_BooleanLikeInteger(t *testing.T) {
	// Integer columns with exactly 2 distinct values (0/1) should be detected
	distinctCount := int64(2)
	col := &models.SchemaColumn{
		ColumnName:    "is_enabled",
		DataType:      "integer",
		DistinctCount: &distinctCount,
		SampleValues:  []string{"0", "1"},
	}

	result := detectBooleanNamingPattern(col)

	require.NotNil(t, result, "Should detect boolean-like integer column")
	assert.Equal(t, "boolean_flag", result.SemanticType)
	assert.Equal(t, "enabled", result.FeatureName)
}

func TestDetectBooleanNamingPattern_IntegerWithTrueFalse(t *testing.T) {
	// Integer columns with true/false sample values
	distinctCount := int64(2)
	col := &models.SchemaColumn{
		ColumnName:    "has_feature",
		DataType:      "smallint",
		DistinctCount: &distinctCount,
		SampleValues:  []string{"true", "false"},
	}

	result := detectBooleanNamingPattern(col)

	require.NotNil(t, result, "Should detect integer column with true/false values")
	assert.Equal(t, "boolean_flag", result.SemanticType)
}

func TestDetectBooleanNamingPattern_IntegerNotBooleanLike(t *testing.T) {
	// Integer columns with more than 2 distinct values should NOT be detected
	distinctCount := int64(5)
	col := &models.SchemaColumn{
		ColumnName:    "is_status",
		DataType:      "integer",
		DistinctCount: &distinctCount,
		SampleValues:  []string{"0", "1", "2", "3", "4"},
	}

	result := detectBooleanNamingPattern(col)

	assert.Nil(t, result, "Should not detect integer column with more than 2 distinct values")
}

func TestDetectBooleanNamingPattern_IntegerWithNonBinaryValues(t *testing.T) {
	// Integer columns with 2 distinct values but not 0/1
	distinctCount := int64(2)
	col := &models.SchemaColumn{
		ColumnName:    "is_type",
		DataType:      "integer",
		DistinctCount: &distinctCount,
		SampleValues:  []string{"5", "10"},
	}

	result := detectBooleanNamingPattern(col)

	assert.Nil(t, result, "Should not detect integer column with non-binary values")
}

func TestDetectBooleanNamingPattern_IntegerNoDistinctCount(t *testing.T) {
	// Integer columns without distinct count should not be detected
	col := &models.SchemaColumn{
		ColumnName: "is_enabled",
		DataType:   "integer",
		// No DistinctCount
	}

	result := detectBooleanNamingPattern(col)

	assert.Nil(t, result, "Should not detect integer column without distinct count")
}

func TestDetectBooleanNamingPattern_CaseInsensitive(t *testing.T) {
	// Column name matching should be case-insensitive
	tests := []struct {
		name       string
		columnName string
	}{
		{"uppercase", "IS_ACTIVE"},
		{"mixed case", "Is_Active"},
		{"camel case like", "IS_active"},
		{"all caps has", "HAS_FEATURE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &models.SchemaColumn{
				ColumnName: tt.columnName,
				DataType:   "boolean",
			}

			result := detectBooleanNamingPattern(col)
			require.NotNil(t, result, "Should detect case-insensitive: %s", tt.columnName)
		})
	}
}

func TestHumanizeFeatureName_BasicConversion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"active", "active"},
		{"email_verified", "email verified"},
		{"premium_subscription", "premium subscription"},
		{"two_factor_auth", "two factor auth"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := humanizeFeatureName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHumanizeFeatureName_Abbreviations(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pc_enabled", "PC enabled"},
		{"api_access", "API access"},
		{"ssl_verified", "SSL verified"},
		{"mfa_enabled", "MFA enabled"},
		{"sso_active", "SSO active"},
		{"id_verified", "ID verified"},
		{"uuid_generated", "UUID generated"},
		{"url_shortened", "URL shortened"},
		{"http_enabled", "HTTP enabled"},
		{"https_required", "HTTPS required"},
		{"tls_configured", "TLS configured"},
		{"2fa_enabled", "2FA enabled"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := humanizeFeatureName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsBooleanType(t *testing.T) {
	tests := []struct {
		dataType string
		expected bool
	}{
		{"boolean", true},
		{"bool", true},
		{"BOOLEAN", true},
		{"bit", true},
		{"bit(1)", true},
		{"integer", false},
		{"text", false},
		{"varchar", false},
		{"timestamp", false},
	}

	for _, tt := range tests {
		t.Run(tt.dataType, func(t *testing.T) {
			result := isBooleanType(tt.dataType)
			assert.Equal(t, tt.expected, result, "isBooleanType(%s)", tt.dataType)
		})
	}
}

func TestIsBooleanLikeIntegerColumn(t *testing.T) {
	distinctCount2 := int64(2)
	distinctCount5 := int64(5)

	tests := []struct {
		name     string
		col      *models.SchemaColumn
		expected bool
	}{
		{
			name: "integer with 2 values 0/1",
			col: &models.SchemaColumn{
				DataType:      "integer",
				DistinctCount: &distinctCount2,
				SampleValues:  []string{"0", "1"},
			},
			expected: true,
		},
		{
			name: "smallint with 2 values",
			col: &models.SchemaColumn{
				DataType:      "smallint",
				DistinctCount: &distinctCount2,
				SampleValues:  []string{"0", "1"},
			},
			expected: true,
		},
		{
			name: "bigint with 2 values",
			col: &models.SchemaColumn{
				DataType:      "bigint",
				DistinctCount: &distinctCount2,
				SampleValues:  []string{"0", "1"},
			},
			expected: true,
		},
		{
			name: "integer with true/false strings",
			col: &models.SchemaColumn{
				DataType:      "integer",
				DistinctCount: &distinctCount2,
				SampleValues:  []string{"true", "false"},
			},
			expected: true,
		},
		{
			name: "integer with more than 2 values",
			col: &models.SchemaColumn{
				DataType:      "integer",
				DistinctCount: &distinctCount5,
				SampleValues:  []string{"0", "1", "2", "3", "4"},
			},
			expected: false,
		},
		{
			name: "integer with non-binary sample values",
			col: &models.SchemaColumn{
				DataType:      "integer",
				DistinctCount: &distinctCount2,
				SampleValues:  []string{"5", "10"},
			},
			expected: false,
		},
		{
			name: "integer without distinct count",
			col: &models.SchemaColumn{
				DataType: "integer",
			},
			expected: false,
		},
		{
			name: "non-integer type",
			col: &models.SchemaColumn{
				DataType:      "text",
				DistinctCount: &distinctCount2,
				SampleValues:  []string{"0", "1"},
			},
			expected: false,
		},
		{
			name: "integer with no sample values (still valid if count is 2)",
			col: &models.SchemaColumn{
				DataType:      "integer",
				DistinctCount: &distinctCount2,
				SampleValues:  []string{},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBooleanLikeIntegerColumn(tt.col)
			assert.Equal(t, tt.expected, result, "isBooleanLikeIntegerColumn for %s", tt.name)
		})
	}
}

func TestGenerateBooleanDescription(t *testing.T) {
	tests := []struct {
		name             string
		prefixDesc       string
		humanizedFeature string
		expectedContains []string
	}{
		{
			name:             "is prefix",
			prefixDesc:       "Indicates whether",
			humanizedFeature: "active",
			expectedContains: []string{"Boolean flag", "Indicates whether", "active"},
		},
		{
			name:             "has prefix",
			prefixDesc:       "Indicates whether this entity has",
			humanizedFeature: "premium subscription",
			expectedContains: []string{"Boolean flag", "Indicates whether this entity has", "premium subscription"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := generateBooleanDescription(tt.prefixDesc, tt.humanizedFeature)
			for _, expected := range tt.expectedContains {
				assert.Contains(t, result, expected)
			}
		})
	}
}
