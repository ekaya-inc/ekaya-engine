package services

import (
	"context"
	"fmt"
	"strings"
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
// Mock Implementations for Entity Discovery Service Tests
// ============================================================================

type mockEntityDiscoveryEntityRepo struct {
	entities []*models.OntologyEntity
}

func (m *mockEntityDiscoveryEntityRepo) GetByOntology(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return m.entities, nil
}

// Stub implementations for interface
func (m *mockEntityDiscoveryEntityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) GetByID(ctx context.Context, entityID uuid.UUID) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryEntityRepo) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryEntityRepo) GetByName(ctx context.Context, ontologyID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryEntityRepo) GetByProjectAndName(ctx context.Context, projectID uuid.UUID, name string) (*models.OntologyEntity, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryEntityRepo) DeleteByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) DeleteInferenceEntitiesByOntology(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) Update(ctx context.Context, entity *models.OntologyEntity) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) SoftDelete(ctx context.Context, entityID uuid.UUID, reason string) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) Restore(ctx context.Context, entityID uuid.UUID) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) GetAliasesByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityAlias, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryEntityRepo) GetAllAliasesByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityAlias, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryEntityRepo) DeleteAlias(ctx context.Context, aliasID uuid.UUID) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) CreateKeyColumn(ctx context.Context, keyColumn *models.OntologyEntityKeyColumn) error {
	return nil
}
func (m *mockEntityDiscoveryEntityRepo) GetKeyColumnsByEntity(ctx context.Context, entityID uuid.UUID) ([]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryEntityRepo) GetAllKeyColumnsByProject(ctx context.Context, projectID uuid.UUID) (map[uuid.UUID][]*models.OntologyEntityKeyColumn, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryEntityRepo) CountOccurrencesByEntity(ctx context.Context, entityID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockEntityDiscoveryEntityRepo) GetOccurrenceTablesByEntity(ctx context.Context, entityID uuid.UUID, limit int) ([]string, error) {
	return nil, nil
}

var _ repositories.OntologyEntityRepository = (*mockEntityDiscoveryEntityRepo)(nil)

type mockEntityDiscoveryConversationRepo struct {
	updateStatusCalled  bool
	lastStatus          string
	lastErrorMessage    string
	lastConversationID  uuid.UUID
}

func (m *mockEntityDiscoveryConversationRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status, errorMessage string) error {
	m.updateStatusCalled = true
	m.lastConversationID = id
	m.lastStatus = status
	m.lastErrorMessage = errorMessage
	return nil
}

// Stub implementations for interface
func (m *mockEntityDiscoveryConversationRepo) Save(ctx context.Context, conv *models.LLMConversation) error {
	return nil
}
func (m *mockEntityDiscoveryConversationRepo) Update(ctx context.Context, conv *models.LLMConversation) error {
	return nil
}
func (m *mockEntityDiscoveryConversationRepo) GetByProject(ctx context.Context, projectID uuid.UUID, limit int) ([]*models.LLMConversation, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryConversationRepo) GetByContext(ctx context.Context, projectID uuid.UUID, key, value string) ([]*models.LLMConversation, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryConversationRepo) GetByConversationID(ctx context.Context, conversationID uuid.UUID) ([]*models.LLMConversation, error) {
	return nil, nil
}
func (m *mockEntityDiscoveryConversationRepo) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	return nil
}

var _ repositories.ConversationRepository = (*mockEntityDiscoveryConversationRepo)(nil)

// ============================================================================
// Tests
// ============================================================================

func TestEnrichEntitiesWithLLM_ParseFailure_ReturnsError(t *testing.T) {
	// Setup
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()
	tableID := uuid.New()

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				OntologyID:    ontologyID,
				Name:          "users",
				Description:   "",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
			},
		},
	}

	conversationRepo := &mockEntityDiscoveryConversationRepo{}

	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content:        "this is not valid JSON at all { broken",
			ConversationID: conversationID,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.CreateForProjectFunc = func(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
		return mockClient, nil
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil, // schemaRepo not needed for this test
		nil, // ontologyRepo not needed for this test
		conversationRepo,
		mockFactory,
		nil, // workerPool not needed for this test
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	tables := []*models.SchemaTable{
		{
			ID:         tableID,
			SchemaName: "public",
			TableName:  "users",
		},
	}
	columns := []*models.SchemaColumn{
		{
			SchemaTableID: tableID,
			ColumnName:    "id",
		},
		{
			SchemaTableID: tableID,
			ColumnName:    "email",
		},
	}

	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: error should be returned (fail fast)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity enrichment parse failure")

	// Verify: conversation status should be updated to error
	assert.True(t, conversationRepo.updateStatusCalled, "UpdateStatus should be called")
	assert.Equal(t, conversationID, conversationRepo.lastConversationID)
	assert.Equal(t, models.LLMConversationStatusError, conversationRepo.lastStatus)
	assert.Contains(t, conversationRepo.lastErrorMessage, "parse_failure")
}

func TestEnrichEntitiesWithLLM_ValidResponse_Success(t *testing.T) {
	// Setup
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()
	tableID := uuid.New()

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				OntologyID:    ontologyID,
				Name:          "users",
				Description:   "",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
			},
		},
	}

	conversationRepo := &mockEntityDiscoveryConversationRepo{}

	validResponse := `{
		"entities": [
			{
				"table_name": "users",
				"entity_name": "User",
				"description": "A platform user account",
				"domain": "customer",
				"key_columns": [{"name": "email", "synonyms": ["e-mail"]}],
				"alternative_names": ["member", "account"]
			}
		]
	}`

	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content:        validResponse,
			ConversationID: conversationID,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.CreateForProjectFunc = func(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
		return mockClient, nil
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil,
		nil,
		conversationRepo,
		mockFactory,
		nil, // workerPool not needed for this test
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	tables := []*models.SchemaTable{
		{
			ID:         tableID,
			SchemaName: "public",
			TableName:  "users",
		},
	}
	columns := []*models.SchemaColumn{
		{
			SchemaTableID: tableID,
			ColumnName:    "id",
		},
		{
			SchemaTableID: tableID,
			ColumnName:    "email",
		},
	}

	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: no error for valid response
	require.NoError(t, err)

	// Verify: conversation status should NOT be updated to error
	assert.False(t, conversationRepo.updateStatusCalled, "UpdateStatus should not be called for success")
}

func TestEnrichEntitiesWithLLM_EmptyEntities_ReturnsNil(t *testing.T) {
	// Setup
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{}, // empty
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil,
		nil,
		nil,
		llm.NewMockClientFactory(),
		nil, // workerPool not needed for this test
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, nil, nil)

	// Verify: no error when no entities exist (nothing to enrich)
	require.NoError(t, err)
}

func TestEnrichEntitiesWithLLM_IncompleteResponse_ReturnsError(t *testing.T) {
	// This test verifies that if the LLM response is missing some entities
	// (e.g., truncated response), an error is returned.

	// Setup
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()
	tableID1 := uuid.New()
	tableID2 := uuid.New()
	tableID3 := uuid.New()

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				OntologyID:    ontologyID,
				Name:          "users",
				Description:   "",
				PrimarySchema: "public",
				PrimaryTable:  "users",
				PrimaryColumn: "id",
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				OntologyID:    ontologyID,
				Name:          "orders",
				Description:   "",
				PrimarySchema: "public",
				PrimaryTable:  "orders",
				PrimaryColumn: "id",
			},
			{
				ID:            uuid.New(),
				ProjectID:     projectID,
				OntologyID:    ontologyID,
				Name:          "products",
				Description:   "",
				PrimarySchema: "public",
				PrimaryTable:  "products",
				PrimaryColumn: "id",
			},
		},
	}

	conversationRepo := &mockEntityDiscoveryConversationRepo{}

	// LLM response only includes 1 of 3 entities (simulating truncated response)
	incompleteResponse := `{
		"entities": [
			{
				"table_name": "users",
				"entity_name": "User",
				"description": "A platform user account",
				"domain": "customer",
				"key_columns": [{"name": "email", "synonyms": ["e-mail"]}],
				"alternative_names": ["member"]
			}
		]
	}`

	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content:        incompleteResponse,
			ConversationID: conversationID,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.CreateForProjectFunc = func(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
		return mockClient, nil
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil,
		nil,
		conversationRepo,
		mockFactory,
		nil, // workerPool not needed for this test
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	tables := []*models.SchemaTable{
		{ID: tableID1, SchemaName: "public", TableName: "users"},
		{ID: tableID2, SchemaName: "public", TableName: "orders"},
		{ID: tableID3, SchemaName: "public", TableName: "products"},
	}
	columns := []*models.SchemaColumn{
		{SchemaTableID: tableID1, ColumnName: "id"},
		{SchemaTableID: tableID2, ColumnName: "id"},
		{SchemaTableID: tableID3, ColumnName: "id"},
	}

	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: error should be returned for incomplete response
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity enrichment incomplete")
	assert.Contains(t, err.Error(), "2 entities not in LLM response")
	assert.Contains(t, err.Error(), "orders")
	assert.Contains(t, err.Error(), "products")
}

// ============================================================================
// ValidateEnrichment Tests
// ============================================================================

func TestValidateEnrichment_AllEntitiesHaveDescriptions_ReturnsNil(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{
			{
				ID:          uuid.New(),
				Name:        "User",
				Description: "A platform user account",
			},
			{
				ID:          uuid.New(),
				Name:        "Order",
				Description: "A customer order",
			},
		},
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil,
		nil,
		nil,
		llm.NewMockClientFactory(),
		nil, // workerPool not needed for this test
		mockTenantCtx,
		zap.NewNop(),
	)

	err := svc.ValidateEnrichment(context.Background(), projectID, ontologyID)

	require.NoError(t, err)
}

func TestValidateEnrichment_SomeEntitiesLackDescriptions_ReturnsError(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{
			{
				ID:          uuid.New(),
				Name:        "User",
				Description: "A platform user account",
			},
			{
				ID:          uuid.New(),
				Name:        "users", // raw table name retained
				Description: "",      // empty description
			},
			{
				ID:          uuid.New(),
				Name:        "s10_events", // raw table name retained
				Description: "",           // empty description
			},
		},
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil,
		nil,
		nil,
		llm.NewMockClientFactory(),
		nil, // workerPool not needed for this test
		mockTenantCtx,
		zap.NewNop(),
	)

	err := svc.ValidateEnrichment(context.Background(), projectID, ontologyID)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "2 entities lack descriptions")
	assert.Contains(t, err.Error(), "users")
	assert.Contains(t, err.Error(), "s10_events")
}

func TestValidateEnrichment_NoEntities_ReturnsNil(t *testing.T) {
	projectID := uuid.New()
	ontologyID := uuid.New()

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{}, // empty
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil,
		nil,
		nil,
		llm.NewMockClientFactory(),
		nil, // workerPool not needed for this test
		mockTenantCtx,
		zap.NewNop(),
	)

	err := svc.ValidateEnrichment(context.Background(), projectID, ontologyID)

	require.NoError(t, err)
}

// ============================================================================
// Batched Enrichment Tests
// ============================================================================

func TestEnrichEntitiesWithLLM_BatchedEnrichment_Success(t *testing.T) {
	// This test verifies that when there are more than 20 entities,
	// the service processes them in batches using the worker pool.

	// Setup
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()

	// Create 25 entities to trigger batching (batch size is 20)
	var entities []*models.OntologyEntity
	var tables []*models.SchemaTable
	var columns []*models.SchemaColumn

	for i := 0; i < 25; i++ {
		tableID := uuid.New()
		tableName := fmt.Sprintf("table_%d", i)
		entities = append(entities, &models.OntologyEntity{
			ID:            uuid.New(),
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          tableName,
			Description:   "",
			PrimarySchema: "public",
			PrimaryTable:  tableName,
			PrimaryColumn: "id",
		})
		tables = append(tables, &models.SchemaTable{
			ID:         tableID,
			SchemaName: "public",
			TableName:  tableName,
		})
		columns = append(columns, &models.SchemaColumn{
			SchemaTableID: tableID,
			ColumnName:    "id",
		})
	}

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: entities,
	}

	conversationRepo := &mockEntityDiscoveryConversationRepo{}

	// Track how many times LLM was called (should be 2 batches: 20 + 5)
	llmCallCount := 0
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		llmCallCount++

		// Generate response for all entities mentioned in the prompt
		var responseEntities []string
		for i := 0; i < 25; i++ {
			tableName := fmt.Sprintf("table_%d", i)
			if strings.Contains(prompt, tableName) {
				responseEntities = append(responseEntities, fmt.Sprintf(`{
					"table_name": "%s",
					"entity_name": "Entity%d",
					"description": "Description for entity %d",
					"domain": "test",
					"key_columns": [],
					"alternative_names": []
				}`, tableName, i, i))
			}
		}

		response := fmt.Sprintf(`{"entities": [%s]}`, strings.Join(responseEntities, ","))
		return &llm.GenerateResponseResult{
			Content:        response,
			ConversationID: conversationID,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.CreateForProjectFunc = func(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
		return mockClient, nil
	}

	// Create a real worker pool for this test
	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 4}, zap.NewNop())

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil, // schemaRepo not needed
		nil, // ontologyRepo not needed
		conversationRepo,
		mockFactory,
		workerPool,
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: no error
	require.NoError(t, err)

	// Verify: LLM was called twice (2 batches: 20 + 5)
	assert.Equal(t, 2, llmCallCount, "Expected 2 LLM calls for 25 entities with batch size 20")

	// Verify: conversation status should NOT be updated to error
	assert.False(t, conversationRepo.updateStatusCalled, "UpdateStatus should not be called for success")
}

func TestEnrichEntitiesWithLLM_BatchedEnrichment_BatchFailure(t *testing.T) {
	// This test verifies that if one batch fails, the entire operation fails fast.

	// Setup
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()

	// Create 25 entities to trigger batching
	var entities []*models.OntologyEntity
	var tables []*models.SchemaTable
	var columns []*models.SchemaColumn

	for i := 0; i < 25; i++ {
		tableID := uuid.New()
		tableName := fmt.Sprintf("table_%d", i)
		entities = append(entities, &models.OntologyEntity{
			ID:            uuid.New(),
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          tableName,
			Description:   "",
			PrimarySchema: "public",
			PrimaryTable:  tableName,
			PrimaryColumn: "id",
		})
		tables = append(tables, &models.SchemaTable{
			ID:         tableID,
			SchemaName: "public",
			TableName:  tableName,
		})
		columns = append(columns, &models.SchemaColumn{
			SchemaTableID: tableID,
			ColumnName:    "id",
		})
	}

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: entities,
	}

	conversationRepo := &mockEntityDiscoveryConversationRepo{}

	// Make LLM return error on second call
	llmCallCount := 0
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		llmCallCount++
		// Fail on second batch
		if llmCallCount > 1 {
			return nil, fmt.Errorf("simulated LLM error")
		}

		// Generate response for first batch
		var responseEntities []string
		for i := 0; i < 20; i++ {
			tableName := fmt.Sprintf("table_%d", i)
			if strings.Contains(prompt, tableName) {
				responseEntities = append(responseEntities, fmt.Sprintf(`{
					"table_name": "%s",
					"entity_name": "Entity%d",
					"description": "Description for entity %d",
					"domain": "test",
					"key_columns": [],
					"alternative_names": []
				}`, tableName, i, i))
			}
		}

		response := fmt.Sprintf(`{"entities": [%s]}`, strings.Join(responseEntities, ","))
		return &llm.GenerateResponseResult{
			Content:        response,
			ConversationID: conversationID,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.CreateForProjectFunc = func(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
		return mockClient, nil
	}

	// Create a real worker pool for this test
	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 4}, zap.NewNop())

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil,
		nil,
		conversationRepo,
		mockFactory,
		workerPool,
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: error should be returned (fail fast on batch failure)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity enrichment batch failed")
	assert.Contains(t, err.Error(), "simulated LLM error")
}

func TestEnrichEntitiesWithLLM_SmallEntitySet_NoBatching(t *testing.T) {
	// This test verifies that small entity sets (≤20) are processed in a single call.

	// Setup
	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()

	// Create exactly 20 entities (should NOT trigger batching)
	var entities []*models.OntologyEntity
	var tables []*models.SchemaTable
	var columns []*models.SchemaColumn

	for i := 0; i < 20; i++ {
		tableID := uuid.New()
		tableName := fmt.Sprintf("table_%d", i)
		entities = append(entities, &models.OntologyEntity{
			ID:            uuid.New(),
			ProjectID:     projectID,
			OntologyID:    ontologyID,
			Name:          tableName,
			Description:   "",
			PrimarySchema: "public",
			PrimaryTable:  tableName,
			PrimaryColumn: "id",
		})
		tables = append(tables, &models.SchemaTable{
			ID:         tableID,
			SchemaName: "public",
			TableName:  tableName,
		})
		columns = append(columns, &models.SchemaColumn{
			SchemaTableID: tableID,
			ColumnName:    "id",
		})
	}

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: entities,
	}

	conversationRepo := &mockEntityDiscoveryConversationRepo{}

	// Track how many times LLM was called (should be exactly 1)
	llmCallCount := 0
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		llmCallCount++

		// Generate response for all entities
		var responseEntities []string
		for i := 0; i < 20; i++ {
			tableName := fmt.Sprintf("table_%d", i)
			responseEntities = append(responseEntities, fmt.Sprintf(`{
				"table_name": "%s",
				"entity_name": "Entity%d",
				"description": "Description for entity %d",
				"domain": "test",
				"key_columns": [],
				"alternative_names": []
			}`, tableName, i, i))
		}

		response := fmt.Sprintf(`{"entities": [%s]}`, strings.Join(responseEntities, ","))
		return &llm.GenerateResponseResult{
			Content:        response,
			ConversationID: conversationID,
		}, nil
	}

	mockFactory := llm.NewMockClientFactory()
	mockFactory.CreateForProjectFunc = func(ctx context.Context, projectID uuid.UUID) (llm.LLMClient, error) {
		return mockClient, nil
	}

	// Create a real worker pool for this test
	workerPool := llm.NewWorkerPool(llm.WorkerPoolConfig{MaxConcurrent: 4}, zap.NewNop())

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		nil,
		nil,
		conversationRepo,
		mockFactory,
		workerPool,
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: no error
	require.NoError(t, err)

	// Verify: LLM was called exactly once (no batching for ≤20 entities)
	assert.Equal(t, 1, llmCallCount, "Expected 1 LLM call for 20 entities (no batching)")
}
