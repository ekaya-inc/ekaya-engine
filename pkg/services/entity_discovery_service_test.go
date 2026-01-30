package services

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
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
func (m *mockEntityDiscoveryEntityRepo) DeleteBySource(ctx context.Context, projectID uuid.UUID, source models.ProvenanceSource) error {
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

func (m *mockEntityDiscoveryEntityRepo) MarkInferenceEntitiesStale(ctx context.Context, ontologyID uuid.UUID) error {
	return nil
}

func (m *mockEntityDiscoveryEntityRepo) ClearStaleFlag(ctx context.Context, entityID uuid.UUID) error {
	return nil
}

func (m *mockEntityDiscoveryEntityRepo) GetStaleEntities(ctx context.Context, ontologyID uuid.UUID) ([]*models.OntologyEntity, error) {
	return nil, nil
}

var _ repositories.OntologyEntityRepository = (*mockEntityDiscoveryEntityRepo)(nil)

type mockEntityDiscoveryConversationRepo struct {
	updateStatusCalled bool
	lastStatus         string
	lastErrorMessage   string
	lastConversationID uuid.UUID
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
		nil, // questionService not needed for this test
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
		nil, // questionService not needed for this test
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
		nil, // questionService not needed for this test
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
		nil, // questionService not needed for this test
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
		nil, // questionService not needed for this test
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
		nil, // questionService not needed for this test
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
		nil, // questionService not needed for this test
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
	var llmCallCount atomic.Int64
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		llmCallCount.Add(1)

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
		nil, // questionService not needed for this test
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
	assert.Equal(t, int64(2), llmCallCount.Load(), "Expected 2 LLM calls for 25 entities with batch size 20")

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
	var llmCallCount atomic.Int64
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		count := llmCallCount.Add(1)
		// Fail on second batch
		if count > 1 {
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
		nil, // questionService not needed for this test
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

// ============================================================================
// Table Grouping Tests
// ============================================================================

func TestExtractCoreConcept(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Sample table prefixes (s1_, s2_, s10_, etc.)
		{"sample prefix s1_", "s1_users", "users"},
		{"sample prefix s2_", "s2_orders", "orders"},
		{"sample prefix s10_", "s10_products", "products"},
		{"sample prefix s123_", "s123_events", "events"},

		// Common test/staging prefixes
		{"test prefix", "test_users", "users"},
		{"tmp prefix", "tmp_cache", "cache"},
		{"temp prefix", "temp_data", "data"},
		{"staging prefix", "staging_orders", "orders"},
		{"dev prefix", "dev_accounts", "accounts"},
		{"sample prefix", "sample_transactions", "transactions"},
		{"demo prefix", "demo_users", "users"},
		{"backup prefix", "backup_logs", "logs"},
		{"old prefix", "old_schema", "schema"},
		{"underscore prefix", "_temp_table", "temp_table"},
		{"copy_of prefix", "copy_of_users", "users"},
		{"archive prefix", "archive_events", "events"},

		// No prefix (unchanged, but lowercased)
		{"no prefix lowercase", "users", "users"},
		{"no prefix uppercase", "Users", "users"},
		{"no prefix mixed case", "BillingActivities", "billingactivities"},

		// Edge cases
		{"empty string", "", ""},
		{"only prefix s1_", "s1_", ""},
		{"multiple underscores", "s1_user_accounts", "user_accounts"},
		{"number in name", "user2_logs", "user2_logs"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCoreConcept(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasTestPrefix(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Tables WITH test prefixes
		{"s1_ prefix", "s1_users", true},
		{"s2_ prefix", "s2_orders", true},
		{"s10_ prefix", "s10_products", true},
		{"test_ prefix", "test_users", true},
		{"tmp_ prefix", "tmp_data", true},
		{"temp_ prefix", "temp_cache", true},
		{"staging_ prefix", "staging_orders", true},
		{"dev_ prefix", "dev_accounts", true},
		{"sample_ prefix", "sample_data", true},
		{"demo_ prefix", "demo_users", true},
		{"backup_ prefix", "backup_logs", true},
		{"old_ prefix", "old_schema", true},
		{"underscore prefix", "_temp", true},
		{"copy_of_ prefix", "copy_of_users", true},
		{"archive_ prefix", "archive_events", true},

		// Tables WITHOUT test prefixes
		{"normal table", "users", false},
		{"normal table with underscore", "user_accounts", false},
		{"normal table with number", "order2", false},
		{"contains test but no prefix", "users_test", false},
		{"contains s1 but no prefix", "users_s1", false},
		{"just numbers", "123_table", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasTestPrefix(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGroupSimilarTables(t *testing.T) {
	t.Run("groups tables by core concept", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{TableName: "users"},
			{TableName: "s1_users"},
			{TableName: "s2_users"},
			{TableName: "test_users"},
			{TableName: "orders"},
			{TableName: "s1_orders"},
		}

		groups := groupSimilarTables(tables)

		// Should have 2 groups: "users" and "orders"
		assert.Len(t, groups, 2)

		// "users" group should have 4 tables
		usersGroup := groups["users"]
		assert.Len(t, usersGroup, 4)

		// "orders" group should have 2 tables
		ordersGroup := groups["orders"]
		assert.Len(t, ordersGroup, 2)
	})

	t.Run("handles tables with no duplicates", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{TableName: "users"},
			{TableName: "orders"},
			{TableName: "products"},
		}

		groups := groupSimilarTables(tables)

		// Should have 3 groups, each with 1 table
		assert.Len(t, groups, 3)
		assert.Len(t, groups["users"], 1)
		assert.Len(t, groups["orders"], 1)
		assert.Len(t, groups["products"], 1)
	})

	t.Run("handles empty input", func(t *testing.T) {
		groups := groupSimilarTables([]*models.SchemaTable{})
		assert.Len(t, groups, 0)
	})

	t.Run("handles nil input", func(t *testing.T) {
		groups := groupSimilarTables(nil)
		assert.Len(t, groups, 0)
	})

	t.Run("groups all sample tables together", func(t *testing.T) {
		// Scenario: only sample tables exist, no "real" table
		tables := []*models.SchemaTable{
			{TableName: "s1_users"},
			{TableName: "s2_users"},
			{TableName: "s5_users"},
		}

		groups := groupSimilarTables(tables)

		assert.Len(t, groups, 1)
		assert.Len(t, groups["users"], 3)
	})
}

func TestSelectPrimaryTable(t *testing.T) {
	t.Run("prefers table without test prefix", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{TableName: "s1_users"},
			{TableName: "users"},
			{TableName: "s2_users"},
		}

		primary := selectPrimaryTable(tables)
		assert.Equal(t, "users", primary.TableName)
	})

	t.Run("prefers first non-test table when multiple exist", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{TableName: "s1_users"},
			{TableName: "users"},
			{TableName: "test_users"},
			{TableName: "accounts"}, // also no prefix
		}

		primary := selectPrimaryTable(tables)
		assert.Equal(t, "users", primary.TableName)
	})

	t.Run("falls back to first table if all have test prefixes", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{TableName: "s1_users"},
			{TableName: "s2_users"},
			{TableName: "test_users"},
		}

		primary := selectPrimaryTable(tables)
		assert.Equal(t, "s1_users", primary.TableName)
	})

	t.Run("handles single table", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{TableName: "users"},
		}

		primary := selectPrimaryTable(tables)
		assert.Equal(t, "users", primary.TableName)
	})

	t.Run("handles single test table", func(t *testing.T) {
		tables := []*models.SchemaTable{
			{TableName: "s1_users"},
		}

		primary := selectPrimaryTable(tables)
		assert.Equal(t, "s1_users", primary.TableName)
	})
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
		nil, // questionService not needed for this test
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

// ============================================================================
// IdentifyEntitiesFromDDL Table Grouping Tests
// ============================================================================

// trackingEntityRepo is a mock that tracks entity creates and alias creates
type trackingEntityRepo struct {
	mockEntityDiscoveryEntityRepo
	createdEntities []*models.OntologyEntity
	createdAliases  []*models.OntologyEntityAlias
}

func (m *trackingEntityRepo) Create(ctx context.Context, entity *models.OntologyEntity) error {
	if entity.ID == uuid.Nil {
		entity.ID = uuid.New()
	}
	m.createdEntities = append(m.createdEntities, entity)
	return nil
}

func (m *trackingEntityRepo) CreateAlias(ctx context.Context, alias *models.OntologyEntityAlias) error {
	if alias.ID == uuid.Nil {
		alias.ID = uuid.New()
	}
	m.createdAliases = append(m.createdAliases, alias)
	return nil
}

// mockSchemaRepoForGrouping is a mock schema repository for table grouping tests
type mockSchemaRepoForGrouping struct {
	tables  []*models.SchemaTable
	columns []*models.SchemaColumn
}

func (m *mockSchemaRepoForGrouping) ListTablesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID, selectedOnly bool) ([]*models.SchemaTable, error) {
	return m.tables, nil
}

func (m *mockSchemaRepoForGrouping) ListColumnsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return m.columns, nil
}

func (m *mockSchemaRepoForGrouping) GetColumnsWithFeaturesByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}

// Stub implementations for interface
func (m *mockSchemaRepoForGrouping) GetTableByID(ctx context.Context, projectID, tableID uuid.UUID) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, schemaName, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) FindTableByName(ctx context.Context, projectID, datasourceID uuid.UUID, tableName string) (*models.SchemaTable, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) UpsertTable(ctx context.Context, table *models.SchemaTable) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) SoftDeleteRemovedTables(ctx context.Context, projectID, datasourceID uuid.UUID, activeTableKeys []repositories.TableKey) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForGrouping) UpdateTableSelection(ctx context.Context, projectID, tableID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) UpdateTableMetadata(ctx context.Context, projectID, tableID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) ListColumnsByTable(ctx context.Context, projectID, tableID uuid.UUID, selectedOnly bool) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetColumnsByTables(ctx context.Context, projectID uuid.UUID, tableNames []string, selectedOnly bool) (map[string][]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetColumnCountByProject(ctx context.Context, projectID uuid.UUID) (int, error) {
	return 0, nil
}
func (m *mockSchemaRepoForGrouping) GetColumnByID(ctx context.Context, projectID, columnID uuid.UUID) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetColumnByName(ctx context.Context, tableID uuid.UUID, columnName string) (*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) UpsertColumn(ctx context.Context, column *models.SchemaColumn) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) SoftDeleteRemovedColumns(ctx context.Context, tableID uuid.UUID, activeColumnNames []string) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForGrouping) UpdateColumnSelection(ctx context.Context, projectID, columnID uuid.UUID, isSelected bool) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) UpdateColumnStats(ctx context.Context, columnID uuid.UUID, distinctCount, nullCount, minLength, maxLength *int64, sampleValues []string) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) UpdateColumnMetadata(ctx context.Context, projectID, columnID uuid.UUID, businessName, description *string) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) UpdateColumnFeatures(ctx context.Context, projectID, columnID uuid.UUID, features *models.ColumnFeatures) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) ListRelationshipsByDatasource(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetRelationshipByID(ctx context.Context, projectID, relationshipID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetRelationshipByColumns(ctx context.Context, sourceColumnID, targetColumnID uuid.UUID) (*models.SchemaRelationship, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) UpsertRelationship(ctx context.Context, rel *models.SchemaRelationship) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) UpdateRelationshipApproval(ctx context.Context, projectID, relationshipID uuid.UUID, isApproved bool) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) SoftDeleteRelationship(ctx context.Context, projectID, relationshipID uuid.UUID) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) SoftDeleteOrphanedRelationships(ctx context.Context, projectID, datasourceID uuid.UUID) (int64, error) {
	return 0, nil
}
func (m *mockSchemaRepoForGrouping) GetRelationshipDetails(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.RelationshipDetail, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetEmptyTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetOrphanTables(ctx context.Context, projectID, datasourceID uuid.UUID) ([]string, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) UpsertRelationshipWithMetrics(ctx context.Context, rel *models.SchemaRelationship, metrics *models.DiscoveryMetrics) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) GetJoinableColumns(ctx context.Context, projectID, tableID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) UpdateColumnJoinability(ctx context.Context, columnID uuid.UUID, rowCount, nonNullCount, distinctCount *int64, isJoinable *bool, joinabilityReason *string) error {
	return nil
}
func (m *mockSchemaRepoForGrouping) GetPrimaryKeyColumns(ctx context.Context, projectID, datasourceID uuid.UUID) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) GetNonPKColumnsByExactType(ctx context.Context, projectID, datasourceID uuid.UUID, dataType string) ([]*models.SchemaColumn, error) {
	return nil, nil
}
func (m *mockSchemaRepoForGrouping) SelectAllTablesAndColumns(ctx context.Context, projectID, datasourceID uuid.UUID) error {
	return nil
}

var _ repositories.SchemaRepository = (*mockSchemaRepoForGrouping)(nil)

func TestIdentifyEntitiesFromDDL_GroupsSimilarTables(t *testing.T) {
	// Test that tables with similar names (e.g., "users", "s1_users", "test_users")
	// result in ONE entity, not multiple.

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()

	// Create tables with PKs: "users", "s1_users", "s2_users", "test_users"
	usersTableID := uuid.New()
	s1UsersTableID := uuid.New()
	s2UsersTableID := uuid.New()
	testUsersTableID := uuid.New()
	ordersTableID := uuid.New()

	tables := []*models.SchemaTable{
		{ID: usersTableID, SchemaName: "public", TableName: "users"},
		{ID: s1UsersTableID, SchemaName: "public", TableName: "s1_users"},
		{ID: s2UsersTableID, SchemaName: "public", TableName: "s2_users"},
		{ID: testUsersTableID, SchemaName: "public", TableName: "test_users"},
		{ID: ordersTableID, SchemaName: "public", TableName: "orders"},
	}

	columns := []*models.SchemaColumn{
		{SchemaTableID: usersTableID, ColumnName: "id", IsPrimaryKey: true},
		{SchemaTableID: s1UsersTableID, ColumnName: "id", IsPrimaryKey: true},
		{SchemaTableID: s2UsersTableID, ColumnName: "id", IsPrimaryKey: true},
		{SchemaTableID: testUsersTableID, ColumnName: "id", IsPrimaryKey: true},
		{SchemaTableID: ordersTableID, ColumnName: "id", IsPrimaryKey: true},
	}

	entityRepo := &trackingEntityRepo{}
	schemaRepo := &mockSchemaRepoForGrouping{
		tables:  tables,
		columns: columns,
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		schemaRepo,
		nil,
		nil,
		nil, // questionService not needed for this test
		llm.NewMockClientFactory(),
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	entityCount, _, _, err := svc.IdentifyEntitiesFromDDL(context.Background(), projectID, ontologyID, datasourceID)

	// Verify: no error
	require.NoError(t, err)

	// Verify: should create only 2 entities ("users" and "orders"), not 5
	assert.Equal(t, 2, entityCount, "Expected 2 entities (users grouped, orders separate)")
	assert.Len(t, entityRepo.createdEntities, 2, "Expected 2 entity creates")

	// Verify: one entity should be for "users" concept, one for "orders"
	var usersEntity, ordersEntity *models.OntologyEntity
	for _, e := range entityRepo.createdEntities {
		if e.PrimaryTable == "users" {
			usersEntity = e
		} else if e.PrimaryTable == "orders" {
			ordersEntity = e
		}
	}

	require.NotNil(t, usersEntity, "Expected users entity to be created")
	require.NotNil(t, ordersEntity, "Expected orders entity to be created")

	// Verify: "users" entity should use the non-prefixed table as primary
	assert.Equal(t, "users", usersEntity.PrimaryTable)

	// Verify: aliases created for grouped tables (s1_users, s2_users, test_users)
	assert.Len(t, entityRepo.createdAliases, 3, "Expected 3 aliases for grouped user tables")

	// Verify aliases are for the users entity
	aliasNames := make([]string, len(entityRepo.createdAliases))
	for i, alias := range entityRepo.createdAliases {
		aliasNames[i] = alias.Alias
		assert.Equal(t, usersEntity.ID, alias.EntityID, "Alias should belong to users entity")
	}
	assert.ElementsMatch(t, []string{"s1_users", "s2_users", "test_users"}, aliasNames)
}

func TestIdentifyEntitiesFromDDL_AllTestTables_UsesFirstAsPrimary(t *testing.T) {
	// When all tables in a group have test prefixes, use the first one as primary

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()

	// Only test tables, no "real" users table
	s1UsersTableID := uuid.New()
	s2UsersTableID := uuid.New()
	s5UsersTableID := uuid.New()

	tables := []*models.SchemaTable{
		{ID: s1UsersTableID, SchemaName: "public", TableName: "s1_users"},
		{ID: s2UsersTableID, SchemaName: "public", TableName: "s2_users"},
		{ID: s5UsersTableID, SchemaName: "public", TableName: "s5_users"},
	}

	columns := []*models.SchemaColumn{
		{SchemaTableID: s1UsersTableID, ColumnName: "id", IsPrimaryKey: true},
		{SchemaTableID: s2UsersTableID, ColumnName: "id", IsPrimaryKey: true},
		{SchemaTableID: s5UsersTableID, ColumnName: "id", IsPrimaryKey: true},
	}

	entityRepo := &trackingEntityRepo{}
	schemaRepo := &mockSchemaRepoForGrouping{
		tables:  tables,
		columns: columns,
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		schemaRepo,
		nil,
		nil,
		nil, // questionService not needed for this test
		llm.NewMockClientFactory(),
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	entityCount, _, _, err := svc.IdentifyEntitiesFromDDL(context.Background(), projectID, ontologyID, datasourceID)

	// Verify: no error
	require.NoError(t, err)

	// Verify: should create only 1 entity
	assert.Equal(t, 1, entityCount, "Expected 1 entity for all test tables")
	assert.Len(t, entityRepo.createdEntities, 1, "Expected 1 entity create")

	// Verify: the first table (s1_users) should be used as primary
	assert.Equal(t, "s1_users", entityRepo.createdEntities[0].PrimaryTable)

	// Verify: other tables are aliases
	assert.Len(t, entityRepo.createdAliases, 2, "Expected 2 aliases")
	aliasNames := []string{entityRepo.createdAliases[0].Alias, entityRepo.createdAliases[1].Alias}
	assert.ElementsMatch(t, []string{"s2_users", "s5_users"}, aliasNames)
}

func TestIdentifyEntitiesFromDDL_NoGrouping_UniqueTables(t *testing.T) {
	// When tables have no naming overlap, each should create its own entity

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()

	usersTableID := uuid.New()
	ordersTableID := uuid.New()
	productsTableID := uuid.New()

	tables := []*models.SchemaTable{
		{ID: usersTableID, SchemaName: "public", TableName: "users"},
		{ID: ordersTableID, SchemaName: "public", TableName: "orders"},
		{ID: productsTableID, SchemaName: "public", TableName: "products"},
	}

	columns := []*models.SchemaColumn{
		{SchemaTableID: usersTableID, ColumnName: "id", IsPrimaryKey: true},
		{SchemaTableID: ordersTableID, ColumnName: "id", IsPrimaryKey: true},
		{SchemaTableID: productsTableID, ColumnName: "id", IsPrimaryKey: true},
	}

	entityRepo := &trackingEntityRepo{}
	schemaRepo := &mockSchemaRepoForGrouping{
		tables:  tables,
		columns: columns,
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		schemaRepo,
		nil,
		nil,
		nil, // questionService not needed for this test
		llm.NewMockClientFactory(),
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	entityCount, _, _, err := svc.IdentifyEntitiesFromDDL(context.Background(), projectID, ontologyID, datasourceID)

	// Verify: no error
	require.NoError(t, err)

	// Verify: 3 entities, no aliases
	assert.Equal(t, 3, entityCount, "Expected 3 entities")
	assert.Len(t, entityRepo.createdEntities, 3, "Expected 3 entity creates")
	assert.Len(t, entityRepo.createdAliases, 0, "Expected no aliases when tables are unique")
}

// ============================================================================
// Tests for Existing Entity Names in Prompt
// ============================================================================

func TestEnrichEntitiesWithLLM_IncludesExistingNamesInPrompt(t *testing.T) {
	// This test verifies that when some entities have already been enriched,
	// their names are included in the prompt to prevent duplicate naming.

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()
	tableID := uuid.New()

	// Create one entity that has been enriched (has description and name differs from table)
	// and one entity that hasn't been enriched yet
	enrichedEntity := &models.OntologyEntity{
		ID:            uuid.New(),
		ProjectID:     projectID,
		OntologyID:    ontologyID,
		Name:          "Account", // LLM-generated name (different from table name)
		Description:   "A user account in the system",
		PrimarySchema: "public",
		PrimaryTable:  "accounts",
		PrimaryColumn: "id",
	}

	unenrichedEntity := &models.OntologyEntity{
		ID:            uuid.New(),
		ProjectID:     projectID,
		OntologyID:    ontologyID,
		Name:          "users", // Same as table name (not yet enriched)
		Description:   "",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{enrichedEntity, unenrichedEntity},
	}

	var capturedPrompt string
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		capturedPrompt = prompt

		// Return valid response for the unenriched entity
		return &llm.GenerateResponseResult{
			Content: `{
				"entities": [
					{
						"table_name": "accounts",
						"entity_name": "Account",
						"description": "A user account",
						"domain": "customer",
						"key_columns": [],
						"alternative_names": []
					},
					{
						"table_name": "users",
						"entity_name": "User",
						"description": "A platform user",
						"domain": "customer",
						"key_columns": [],
						"alternative_names": []
					}
				]
			}`,
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
		&mockEntityDiscoveryConversationRepo{},
		nil, // questionService not needed for this test
		mockFactory,
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	tables := []*models.SchemaTable{
		{ID: uuid.New(), SchemaName: "public", TableName: "accounts"},
		{ID: tableID, SchemaName: "public", TableName: "users"},
	}
	columns := []*models.SchemaColumn{
		{SchemaTableID: tables[0].ID, ColumnName: "id"},
		{SchemaTableID: tableID, ColumnName: "id"},
	}

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: no error
	require.NoError(t, err)

	// Verify: the prompt includes existing entity names section
	assert.Contains(t, capturedPrompt, "EXISTING ENTITY NAMES (DO NOT REUSE)")
	assert.Contains(t, capturedPrompt, "Account") // The enriched entity name should be listed

	// Verify: the prompt includes instructions about naming
	assert.Contains(t, capturedPrompt, "Check if a similar name already exists")
	assert.Contains(t, capturedPrompt, "Choose a distinct name if the concept is different")
	assert.Contains(t, capturedPrompt, "Merge tables representing the same concept")
}

func TestEnrichEntitiesWithLLM_NoExistingNames_NoExistingNamesSection(t *testing.T) {
	// This test verifies that when no entities have been enriched yet,
	// the "EXISTING ENTITY NAMES" section is not included in the prompt.

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()
	tableID := uuid.New()

	// Create only unenriched entities (name == table name, no description)
	unenrichedEntity := &models.OntologyEntity{
		ID:            uuid.New(),
		ProjectID:     projectID,
		OntologyID:    ontologyID,
		Name:          "users", // Same as table name
		Description:   "",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
	}

	entityRepo := &mockEntityDiscoveryEntityRepo{
		entities: []*models.OntologyEntity{unenrichedEntity},
	}

	var capturedPrompt string
	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		capturedPrompt = prompt
		return &llm.GenerateResponseResult{
			Content: `{
				"entities": [
					{
						"table_name": "users",
						"entity_name": "User",
						"description": "A platform user",
						"domain": "customer",
						"key_columns": [],
						"alternative_names": []
					}
				]
			}`,
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
		&mockEntityDiscoveryConversationRepo{},
		nil, // questionService not needed for this test
		mockFactory,
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	tables := []*models.SchemaTable{
		{ID: tableID, SchemaName: "public", TableName: "users"},
	}
	columns := []*models.SchemaColumn{
		{SchemaTableID: tableID, ColumnName: "id"},
	}

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: no error
	require.NoError(t, err)

	// Verify: the prompt does NOT include existing entity names section
	assert.NotContains(t, capturedPrompt, "EXISTING ENTITY NAMES (DO NOT REUSE)")
}

// ============================================================================
// Tests for Provenance Fields
// ============================================================================

func TestIdentifyEntitiesFromDDL_SetsConfidence(t *testing.T) {
	// Test that DDL-based entity discovery sets confidence=0.5
	// (DDL-derived entities have lower confidence until LLM enrichment)

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()

	tableID := uuid.New()
	tables := []*models.SchemaTable{
		{ID: tableID, SchemaName: "public", TableName: "users"},
	}

	columns := []*models.SchemaColumn{
		{SchemaTableID: tableID, ColumnName: "id", IsPrimaryKey: true},
	}

	entityRepo := &trackingEntityRepo{}
	schemaRepo := &mockSchemaRepoForGrouping{
		tables:  tables,
		columns: columns,
	}

	mockTenantCtx := func(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error) {
		return ctx, func() {}, nil
	}

	svc := NewEntityDiscoveryService(
		entityRepo,
		schemaRepo,
		nil,
		nil,
		nil, // questionService not needed for this test
		llm.NewMockClientFactory(),
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	// Execute
	entityCount, _, _, err := svc.IdentifyEntitiesFromDDL(context.Background(), projectID, ontologyID, datasourceID)

	// Verify: no error
	require.NoError(t, err)
	assert.Equal(t, 1, entityCount)
	require.Len(t, entityRepo.createdEntities, 1)

	// Verify: DDL-based entities should have confidence=0.5
	entity := entityRepo.createdEntities[0]
	assert.Equal(t, 0.5, entity.Confidence, "DDL-based entities should have confidence=0.5")
}

// trackingUpdateEntityRepo extends trackingEntityRepo to also track Update calls
type trackingUpdateEntityRepo struct {
	trackingEntityRepo
	updatedEntities []*models.OntologyEntity
}

func (m *trackingUpdateEntityRepo) Update(ctx context.Context, entity *models.OntologyEntity) error {
	m.updatedEntities = append(m.updatedEntities, entity)
	return nil
}

func TestEnrichEntitiesWithLLM_SetsConfidence(t *testing.T) {
	// Test that LLM enrichment increases confidence to 0.8

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()
	tableID := uuid.New()

	// Create an entity with initial DDL-based confidence of 0.5
	entity := &models.OntologyEntity{
		ID:            uuid.New(),
		ProjectID:     projectID,
		OntologyID:    ontologyID,
		Name:          "users",
		Description:   "",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
		Confidence:    0.5, // DDL-based confidence
	}

	entityRepo := &trackingUpdateEntityRepo{
		trackingEntityRepo: trackingEntityRepo{
			mockEntityDiscoveryEntityRepo: mockEntityDiscoveryEntityRepo{
				entities: []*models.OntologyEntity{entity},
			},
		},
	}

	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content: `{
				"entities": [
					{
						"table_name": "users",
						"entity_name": "User",
						"description": "A platform user account",
						"domain": "customer",
						"key_columns": [],
						"alternative_names": []
					}
				]
			}`,
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
		&mockSchemaRepoForGrouping{},
		nil,
		nil,
		nil, // questionService not needed for this test
		mockFactory,
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	tables := []*models.SchemaTable{
		{ID: tableID, SchemaName: "public", TableName: "users"},
	}
	columns := []*models.SchemaColumn{
		{SchemaTableID: tableID, ColumnName: "id"},
	}

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: no error
	require.NoError(t, err)
	require.Len(t, entityRepo.updatedEntities, 1)

	// Verify: LLM-enriched entities should have confidence=0.8
	updatedEntity := entityRepo.updatedEntities[0]
	assert.Equal(t, 0.8, updatedEntity.Confidence, "LLM-enriched entities should have confidence=0.8")
	assert.Equal(t, "User", updatedEntity.Name, "Entity name should be updated by LLM")
	assert.Equal(t, "A platform user account", updatedEntity.Description, "Entity description should be updated by LLM")
}

func TestEnrichEntitiesWithLLM_QuestionsInResponse_Parsed(t *testing.T) {
	// Test that questions in the LLM response are parsed correctly.
	// Questions are logged but not yet stored (wiring happens in a later task).

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()
	tableID := uuid.New()

	entity := &models.OntologyEntity{
		ID:            uuid.New(),
		ProjectID:     projectID,
		OntologyID:    ontologyID,
		Name:          "users",
		Description:   "",
		PrimarySchema: "public",
		PrimaryTable:  "users",
		PrimaryColumn: "id",
		Confidence:    0.5,
	}

	entityRepo := &trackingUpdateEntityRepo{
		trackingEntityRepo: trackingEntityRepo{
			mockEntityDiscoveryEntityRepo: mockEntityDiscoveryEntityRepo{
				entities: []*models.OntologyEntity{entity},
			},
		},
	}

	// LLM response includes both entities and questions
	responseWithQuestions := `{
		"entities": [
			{
				"table_name": "users",
				"entity_name": "User",
				"description": "A platform user account",
				"domain": "customer",
				"key_columns": [{"name": "email", "synonyms": ["e-mail"]}],
				"alternative_names": ["member"]
			}
		],
		"questions": [
			{
				"category": "terminology",
				"priority": 2,
				"question": "What does 'tik' mean in tiks_count?",
				"context": "Column users.tiks_count appears to track some kind of count but 'tik' is not a standard term."
			},
			{
				"category": "business_rules",
				"priority": 1,
				"question": "Can a user have multiple email addresses?",
				"context": "The email column has a unique constraint."
			}
		]
	}`

	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content:        responseWithQuestions,
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
		&mockSchemaRepoForGrouping{},
		nil,
		nil,
		nil, // questionService not needed for this test
		mockFactory,
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	tables := []*models.SchemaTable{
		{ID: tableID, SchemaName: "public", TableName: "users"},
	}
	columns := []*models.SchemaColumn{
		{SchemaTableID: tableID, ColumnName: "id"},
		{SchemaTableID: tableID, ColumnName: "email"},
		{SchemaTableID: tableID, ColumnName: "tiks_count"},
	}

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: no error - questions in response should not cause failure
	require.NoError(t, err)

	// Verify: entity was still enriched correctly
	require.Len(t, entityRepo.updatedEntities, 1)
	updatedEntity := entityRepo.updatedEntities[0]
	assert.Equal(t, "User", updatedEntity.Name)
	assert.Equal(t, "A platform user account", updatedEntity.Description)
}

func TestParseEntityEnrichmentResponse_QuestionsExtracted(t *testing.T) {
	// Direct test of parseEntityEnrichmentResponse to verify question extraction

	svc := &entityDiscoveryService{
		logger: zap.NewNop(),
	}

	responseJSON := `{
		"entities": [
			{
				"table_name": "accounts",
				"entity_name": "Account",
				"description": "User account",
				"domain": "auth",
				"key_columns": [],
				"alternative_names": []
			}
		],
		"questions": [
			{
				"category": "enumeration",
				"priority": 1,
				"question": "What do status values 'A', 'P' mean?",
				"context": "Status column has cryptic values."
			},
			{
				"category": "relationship",
				"priority": 2,
				"question": "Is referrer_id a self-reference?",
				"context": "Column appears to reference the same table."
			}
		]
	}`

	entities, questions, err := svc.parseEntityEnrichmentResponse(responseJSON)

	require.NoError(t, err)
	require.Len(t, entities, 1)
	require.Len(t, questions, 2)

	// Verify entity
	assert.Equal(t, "accounts", entities[0].TableName)
	assert.Equal(t, "Account", entities[0].EntityName)

	// Verify questions
	assert.Equal(t, "enumeration", questions[0].Category)
	assert.Equal(t, 1, questions[0].Priority)
	assert.Equal(t, "What do status values 'A', 'P' mean?", questions[0].Question)
	assert.Equal(t, "Status column has cryptic values.", questions[0].Context)

	assert.Equal(t, "relationship", questions[1].Category)
	assert.Equal(t, 2, questions[1].Priority)
}

func TestParseEntityEnrichmentResponse_NoQuestions(t *testing.T) {
	// Test that parsing succeeds when no questions are present

	svc := &entityDiscoveryService{
		logger: zap.NewNop(),
	}

	responseJSON := `{
		"entities": [
			{
				"table_name": "users",
				"entity_name": "User",
				"description": "Platform user",
				"domain": "customer",
				"key_columns": [],
				"alternative_names": []
			}
		]
	}`

	entities, questions, err := svc.parseEntityEnrichmentResponse(responseJSON)

	require.NoError(t, err)
	require.Len(t, entities, 1)
	assert.Empty(t, questions, "Questions should be empty when not present")
}

func TestEnrichEntitiesWithLLM_DeduplicatesAliases(t *testing.T) {
	// Test that when LLM returns duplicate aliases, CreateAlias is only called once per unique alias.
	// This prevents duplicate key constraint violations in the database.

	projectID := uuid.New()
	ontologyID := uuid.New()
	datasourceID := uuid.New()
	conversationID := uuid.New()
	tableID := uuid.New()

	entity := &models.OntologyEntity{
		ID:            uuid.New(),
		ProjectID:     projectID,
		OntologyID:    ontologyID,
		Name:          "billing_engagements",
		Description:   "",
		PrimarySchema: "public",
		PrimaryTable:  "billing_engagements",
		PrimaryColumn: "id",
		Confidence:    0.5,
	}

	entityRepo := &trackingUpdateEntityRepo{
		trackingEntityRepo: trackingEntityRepo{
			mockEntityDiscoveryEntityRepo: mockEntityDiscoveryEntityRepo{
				entities: []*models.OntologyEntity{entity},
			},
		},
	}

	// LLM response with duplicate aliases: "Payment Intent" appears twice
	responseWithDuplicates := `{
		"entities": [
			{
				"table_name": "billing_engagements",
				"entity_name": "Engagement Payment",
				"description": "A payment for an engagement session",
				"domain": "billing",
				"key_columns": [{"name": "amount", "synonyms": ["total"]}],
				"alternative_names": ["Payment Intent", "Engagement Payment", "Payment Intent"]
			}
		]
	}`

	mockClient := llm.NewMockLLMClient()
	mockClient.GenerateResponseFunc = func(ctx context.Context, prompt string, systemMessage string, temperature float64, thinking bool) (*llm.GenerateResponseResult, error) {
		return &llm.GenerateResponseResult{
			Content:        responseWithDuplicates,
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
		&mockSchemaRepoForGrouping{},
		nil,
		nil,
		nil, // questionService not needed for this test
		mockFactory,
		nil,
		mockTenantCtx,
		zap.NewNop(),
	)

	tables := []*models.SchemaTable{
		{ID: tableID, SchemaName: "public", TableName: "billing_engagements"},
	}
	columns := []*models.SchemaColumn{
		{SchemaTableID: tableID, ColumnName: "id"},
		{SchemaTableID: tableID, ColumnName: "amount"},
	}

	// Execute
	err := svc.EnrichEntitiesWithLLM(context.Background(), projectID, ontologyID, datasourceID, tables, columns)

	// Verify: no error
	require.NoError(t, err)

	// Verify: CreateAlias should be called exactly 2 times (not 3), once for each unique alias
	// "Payment Intent" and "Engagement Payment" are the 2 unique aliases
	assert.Len(t, entityRepo.createdAliases, 2, "Expected 2 CreateAlias calls for 2 unique aliases, not 3")

	// Verify the unique aliases were created
	aliasNames := make([]string, len(entityRepo.createdAliases))
	for i, alias := range entityRepo.createdAliases {
		aliasNames[i] = alias.Alias
	}
	assert.ElementsMatch(t, []string{"Payment Intent", "Engagement Payment"}, aliasNames)
}
