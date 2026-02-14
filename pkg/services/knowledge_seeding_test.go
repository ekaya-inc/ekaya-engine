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
)

// mockKnowledgeServiceForSeeding implements KnowledgeService for testing.
type mockKnowledgeServiceForSeeding struct {
	getByTypeResult []*models.KnowledgeFact
	getByTypeErr    error
	getAllResult    []*models.KnowledgeFact
	getAllErr       error
	storedFacts     []storedFactRecord
	storeErr        error
}

type storedFactRecord struct {
	projectID uuid.UUID
	factType  string
	value     string
	context   string
	source    string
}

func (m *mockKnowledgeServiceForSeeding) Store(ctx context.Context, projectID uuid.UUID, factType, value, contextInfo string) (*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeServiceForSeeding) StoreWithSource(ctx context.Context, projectID uuid.UUID, factType, value, contextInfo, source string) (*models.KnowledgeFact, error) {
	if m.storeErr != nil {
		return nil, m.storeErr
	}
	m.storedFacts = append(m.storedFacts, storedFactRecord{
		projectID: projectID,
		factType:  factType,
		value:     value,
		context:   contextInfo,
		source:    source,
	})
	return &models.KnowledgeFact{
		ID:        uuid.New(),
		ProjectID: projectID,
		FactType:  factType,
		Value:     value,
		Context:   contextInfo,
		Source:    source,
	}, nil
}

func (m *mockKnowledgeServiceForSeeding) Update(ctx context.Context, projectID, id uuid.UUID, factType, value, contextInfo string) (*models.KnowledgeFact, error) {
	return nil, nil
}

func (m *mockKnowledgeServiceForSeeding) GetAll(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	if m.getAllErr != nil {
		return nil, m.getAllErr
	}
	return m.getAllResult, nil
}

func (m *mockKnowledgeServiceForSeeding) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	if m.getByTypeErr != nil {
		return nil, m.getByTypeErr
	}
	return m.getByTypeResult, nil
}

func (m *mockKnowledgeServiceForSeeding) Delete(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (m *mockKnowledgeServiceForSeeding) DeleteAll(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

// mockSchemaServiceForSeeding implements SchemaService for testing.
type mockSchemaServiceForSeeding struct {
	schemaForPrompt string
	schemaErr       error
}

func (m *mockSchemaServiceForSeeding) RefreshDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID, autoSelect bool) (*models.RefreshResult, error) {
	return nil, nil
}
func (m *mockSchemaServiceForSeeding) GetDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error) {
	return nil, nil
}
func (m *mockSchemaServiceForSeeding) GetDatasourceTable(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.DatasourceTable, error) {
	return nil, nil
}
func (m *mockSchemaServiceForSeeding) AddManualRelationship(ctx context.Context, projectID, datasourceID uuid.UUID, req *models.AddRelationshipRequest) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaServiceForSeeding) RemoveRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockSchemaServiceForSeeding) GetRelationshipsForDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaServiceForSeeding) GetRelationshipsResponse(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.RelationshipsResponse, error) {
	return nil, nil
}
func (m *mockSchemaServiceForSeeding) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaServiceForSeeding) SaveSelections(ctx context.Context, projectID, datasourceID uuid.UUID, tableSelections map[uuid.UUID]bool, columnSelections map[uuid.UUID][]uuid.UUID) error {
	return nil
}
func (m *mockSchemaServiceForSeeding) GetSelectedDatasourceSchema(ctx context.Context, projectID, datasourceID uuid.UUID) (*models.DatasourceSchema, error) {
	return nil, nil
}
func (m *mockSchemaServiceForSeeding) GetDatasourceSchemaForPrompt(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) (string, error) {
	if m.schemaErr != nil {
		return "", m.schemaErr
	}
	return m.schemaForPrompt, nil
}
func (m *mockSchemaServiceForSeeding) SelectAllTables(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

func (m *mockSchemaServiceForSeeding) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return nil, nil
}

func (m *mockSchemaServiceForSeeding) ListAllTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaTable, error) {
	return nil, nil
}

func (m *mockSchemaServiceForSeeding) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockSchemaServiceForSeeding) ListAllColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}

func (m *mockSchemaServiceForSeeding) GetColumnMetadataByProject(ctx context.Context, projectID uuid.UUID) ([]*models.ColumnMetadata, error) {
	return nil, nil
}

// mockLLMFactoryForSeeding implements llm.LLMClientFactory for testing.
type mockLLMFactoryForSeeding struct {
	client    llm.LLMClient
	createErr error
}

func (m *mockLLMFactoryForSeeding) CreateForProject(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.client, nil
}

func (m *mockLLMFactoryForSeeding) CreateEmbeddingClient(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
	return nil, nil
}

func (m *mockLLMFactoryForSeeding) CreateStreamingClient(ctx context.Context, projectID uuid.UUID) (*llm.StreamingClient, error) {
	return nil, nil
}

// mockLLMClientForSeeding implements llm.LLMClient for testing.
type mockLLMClientForSeeding struct {
	response       *llm.GenerateResponseResult
	err            error
	capturedPrompt string // Captures the prompt sent to GenerateResponse
}

func (m *mockLLMClientForSeeding) GenerateResponse(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
	m.capturedPrompt = prompt
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func (m *mockLLMClientForSeeding) CreateEmbedding(ctx context.Context, input string, model string) ([]float32, error) {
	return nil, nil
}

func (m *mockLLMClientForSeeding) CreateEmbeddings(ctx context.Context, inputs []string, model string) ([][]float32, error) {
	return nil, nil
}

func (m *mockLLMClientForSeeding) GetModel() string {
	return "test-model"
}

func (m *mockLLMClientForSeeding) GetEndpoint() string {
	return "http://localhost"
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_NoOverview(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{}, // No overview fact
	}
	schemaSvc := &mockSchemaServiceForSeeding{}
	llmFactory := &mockLLMFactoryForSeeding{}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	assert.NoError(t, err)
	assert.Equal(t, 0, count)
	assert.Empty(t, knowledgeSvc.storedFacts)
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_WithOverview(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "This is an e-commerce platform. Users are customers who purchase products.",
			},
		},
	}
	schemaSvc := &mockSchemaServiceForSeeding{
		schemaForPrompt: "Table: users\nColumns: id, name, email",
	}

	// LLM returns valid JSON with extracted facts
	llmClient := &mockLLMClientForSeeding{
		response: &llm.GenerateResponseResult{
			Content: `{
				"facts": [
					{
						"fact_type": "terminology",
						"key": "users_are_customers",
						"value": "Users refer to customers who purchase products",
						"context": "E-commerce domain"
					},
					{
						"fact_type": "business_rule",
						"key": "product_purchases",
						"value": "Products can be purchased by users",
						"context": ""
					}
				]
			}`,
			PromptTokens:     100,
			CompletionTokens: 50,
		},
	}
	llmFactory := &mockLLMFactoryForSeeding{client: llmClient}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	require.NoError(t, err)
	assert.Equal(t, 2, count)
	assert.Len(t, knowledgeSvc.storedFacts, 2)

	// Verify the stored facts
	assert.Equal(t, models.FactTypeTerminology, knowledgeSvc.storedFacts[0].factType)
	assert.Equal(t, "Users refer to customers who purchase products", knowledgeSvc.storedFacts[0].value)
	assert.Equal(t, "inferred", knowledgeSvc.storedFacts[0].source)

	assert.Equal(t, models.FactTypeBusinessRule, knowledgeSvc.storedFacts[1].factType)
	assert.Equal(t, "Products can be purchased by users", knowledgeSvc.storedFacts[1].value)
	assert.Equal(t, "inferred", knowledgeSvc.storedFacts[1].source)
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_LLMError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "Some overview text",
			},
		},
	}
	schemaSvc := &mockSchemaServiceForSeeding{}
	llmClient := &mockLLMClientForSeeding{
		err: errors.New("LLM service unavailable"),
	}
	llmFactory := &mockLLMFactoryForSeeding{client: llmClient}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "LLM generate response")
	assert.Equal(t, 0, count)
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_SchemaError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "Some overview text",
			},
		},
	}
	// Schema service returns error - should continue without schema context
	schemaSvc := &mockSchemaServiceForSeeding{
		schemaErr: errors.New("schema not available"),
	}
	llmClient := &mockLLMClientForSeeding{
		response: &llm.GenerateResponseResult{
			Content: `{"facts": []}`,
		},
	}
	llmFactory := &mockLLMFactoryForSeeding{client: llmClient}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	// Should not fail - schema is optional context
	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_EntityHintMapsToTerminology(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "Users are internal employees. Customers are external businesses.",
			},
		},
	}
	schemaSvc := &mockSchemaServiceForSeeding{}

	// LLM returns entity_hint which should map to terminology
	llmClient := &mockLLMClientForSeeding{
		response: &llm.GenerateResponseResult{
			Content: `{
				"facts": [
					{
						"fact_type": "entity_hint",
						"key": "user_vs_customer",
						"value": "Users are internal employees, Customers are external businesses",
						"context": "Entity distinction"
					}
				]
			}`,
		},
	}
	llmFactory := &mockLLMFactoryForSeeding{client: llmClient}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify entity_hint maps to terminology
	assert.Equal(t, models.FactTypeTerminology, knowledgeSvc.storedFacts[0].factType)
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_InvalidFactsFiltered(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "Some overview",
			},
		},
	}
	schemaSvc := &mockSchemaServiceForSeeding{}

	// LLM returns some invalid facts that should be filtered
	// Note: key field was removed from the system - filtering only checks fact_type and value
	llmClient := &mockLLMClientForSeeding{
		response: &llm.GenerateResponseResult{
			Content: `{
				"facts": [
					{
						"fact_type": "business_rule",
						"value": "This is valid"
					},
					{
						"fact_type": "unknown_type",
						"value": "Should be filtered due to unknown type"
					},
					{
						"fact_type": "convention",
						"value": "This is also valid"
					},
					{
						"fact_type": "terminology",
						"value": ""
					}
				]
			}`,
		},
	}
	llmFactory := &mockLLMFactoryForSeeding{client: llmClient}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	require.NoError(t, err)
	// Two valid facts should be stored: business_rule and convention
	// unknown_type is filtered (unknown fact type), empty value is filtered
	assert.Equal(t, 2, count)
	assert.Len(t, knowledgeSvc.storedFacts, 2)
	assert.Equal(t, "This is valid", knowledgeSvc.storedFacts[0].value)
	assert.Equal(t, "This is also valid", knowledgeSvc.storedFacts[1].value)
}

func TestMapFactTypeToModel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"business_rule", models.FactTypeBusinessRule},
		{"BUSINESS_RULE", models.FactTypeBusinessRule},
		{"convention", models.FactTypeConvention},
		{"terminology", models.FactTypeTerminology},
		{"domain_term", models.FactTypeTerminology},
		{"entity_hint", models.FactTypeTerminology},
		{"unknown", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapFactTypeToModel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTruncateForLog(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly10x", 10, "exactly10x"},
		{"this is a longer string", 10, "this is a ..."},
		{"", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := truncateForLog(tt.input, tt.maxLen)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_GetAllError(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "Some overview text",
			},
		},
		getAllErr: errors.New("database connection failed"),
	}
	schemaSvc := &mockSchemaServiceForSeeding{}
	llmClient := &mockLLMClientForSeeding{
		response: &llm.GenerateResponseResult{
			Content: `{"facts": []}`,
		},
	}
	llmFactory := &mockLLMFactoryForSeeding{client: llmClient}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	// Should not fail - GetAll error is logged and continues without existing facts
	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_SkipsWhenFactsExist(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Existing learned facts means seeding should be skipped (one-time initialization)
	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "Some overview text",
			},
		},
		getAllResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "Some overview text",
			},
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  models.FactTypeTerminology,
				Value:     "Some existing learned fact",
			},
		},
	}
	schemaSvc := &mockSchemaServiceForSeeding{}

	// LLM should NOT be called - we're skipping seeding because facts already exist
	llmClient := &mockLLMClientForSeeding{
		err: errors.New("LLM should not be called"),
	}
	llmFactory := &mockLLMFactoryForSeeding{client: llmClient}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	// Should skip seeding without error
	require.NoError(t, err)
	assert.Equal(t, 0, count)
	// LLM was not called (no capturedPrompt)
	assert.Empty(t, llmClient.capturedPrompt)
}

func TestKnowledgeSeedingService_ExtractKnowledgeFromOverview_NoExistingFacts(t *testing.T) {
	projectID := uuid.New()
	datasourceID := uuid.New()

	knowledgeSvc := &mockKnowledgeServiceForSeeding{
		getByTypeResult: []*models.KnowledgeFact{
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "New project overview",
			},
		},
		getAllResult: []*models.KnowledgeFact{
			// Only project_overview exists - no other facts
			{
				ID:        uuid.New(),
				ProjectID: projectID,
				FactType:  "project_overview",
				Value:     "New project overview",
			},
		},
	}
	schemaSvc := &mockSchemaServiceForSeeding{}

	llmClient := &mockLLMClientForSeeding{
		response: &llm.GenerateResponseResult{
			Content: `{"facts": [{"fact_type": "terminology", "key": "new_fact", "value": "A new fact"}]}`,
		},
	}

	llmFactory := &mockLLMFactoryForSeeding{client: llmClient}

	svc := NewKnowledgeSeedingService(knowledgeSvc, schemaSvc, llmFactory, zap.NewNop())

	count, err := svc.ExtractKnowledgeFromOverview(context.Background(), projectID, datasourceID)

	require.NoError(t, err)
	assert.Equal(t, 1, count)

	// Verify "Existing Project Knowledge" section is NOT included when only project_overview exists
	assert.NotContains(t, llmClient.capturedPrompt, "Existing Project Knowledge")
}
