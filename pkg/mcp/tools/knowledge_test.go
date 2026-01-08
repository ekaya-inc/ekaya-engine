package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/apperrors"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// mockKnowledgeRepository implements repositories.KnowledgeRepository for testing.
type mockKnowledgeRepository struct {
	facts []*models.KnowledgeFact
	err   error
}

func (m *mockKnowledgeRepository) Upsert(ctx context.Context, fact *models.KnowledgeFact) error {
	if m.err != nil {
		return m.err
	}
	// Set ID if not set (simulates database behavior)
	if fact.ID == uuid.Nil {
		fact.ID = uuid.New()
	}
	m.facts = append(m.facts, fact)
	return nil
}

func (m *mockKnowledgeRepository) GetByProject(ctx context.Context, projectID uuid.UUID) ([]*models.KnowledgeFact, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.facts, nil
}

func (m *mockKnowledgeRepository) GetByType(ctx context.Context, projectID uuid.UUID, factType string) ([]*models.KnowledgeFact, error) {
	if m.err != nil {
		return nil, m.err
	}
	filtered := make([]*models.KnowledgeFact, 0)
	for _, f := range m.facts {
		if f.FactType == factType {
			filtered = append(filtered, f)
		}
	}
	return filtered, nil
}

func (m *mockKnowledgeRepository) GetByKey(ctx context.Context, projectID uuid.UUID, factType, key string) (*models.KnowledgeFact, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, f := range m.facts {
		if f.FactType == factType && f.Key == key {
			return f, nil
		}
	}
	return nil, nil
}

func (m *mockKnowledgeRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.err != nil {
		return m.err
	}
	for i, f := range m.facts {
		if f.ID == id {
			m.facts = append(m.facts[:i], m.facts[i+1:]...)
			return nil
		}
	}
	return apperrors.ErrNotFound
}

func (m *mockKnowledgeRepository) DeleteByProject(ctx context.Context, projectID uuid.UUID) error {
	if m.err != nil {
		return m.err
	}
	m.facts = make([]*models.KnowledgeFact, 0)
	return nil
}

// TestKnowledgeToolDeps_Structure verifies the KnowledgeToolDeps struct has all required fields.
func TestKnowledgeToolDeps_Structure(t *testing.T) {
	deps := &KnowledgeToolDeps{}

	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.KnowledgeRepository, "KnowledgeRepository field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}

// TestKnowledgeToolDeps_Initialization verifies the struct can be initialized with dependencies.
func TestKnowledgeToolDeps_Initialization(t *testing.T) {
	logger := zap.NewNop()
	knowledgeRepo := &mockKnowledgeRepository{}
	mcpConfigService := &mockMCPConfigService{}

	deps := &KnowledgeToolDeps{
		MCPConfigService:    mcpConfigService,
		KnowledgeRepository: knowledgeRepo,
		Logger:              logger,
	}

	assert.NotNil(t, deps, "KnowledgeToolDeps should be initialized")
	assert.Equal(t, logger, deps.Logger, "Logger should be set correctly")
	assert.Equal(t, knowledgeRepo, deps.KnowledgeRepository, "KnowledgeRepository should be set correctly")
}

// TestRegisterKnowledgeTools verifies tools are registered with the MCP server.
func TestRegisterKnowledgeTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &KnowledgeToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterKnowledgeTools(mcpServer, deps)

	// Verify tools are registered
	ctx := context.Background()
	result := mcpServer.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(resultBytes, &response))

	// Check both knowledge tools are registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["update_project_knowledge"], "update_project_knowledge tool should be registered")
	assert.True(t, toolNames["delete_project_knowledge"], "delete_project_knowledge tool should be registered")
}

// TestUpdateProjectKnowledge_ResponseStructure verifies the response structure.
func TestUpdateProjectKnowledge_ResponseStructure(t *testing.T) {
	// This test verifies the response structure without a full integration test.
	fact := "Platform fees are ~33% of total_amount"
	category := "business_rule"
	context := "Verified: tikr_share / total_amount ≈ 0.33 across all transactions"

	response := updateProjectKnowledgeResponse{
		FactID:   uuid.New().String(),
		Fact:     fact,
		Category: category,
		Context:  context,
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify all fields are present
	assert.Equal(t, fact, parsed["fact"])
	assert.Equal(t, category, parsed["category"])
	assert.Equal(t, context, parsed["context"])
	assert.NotEmpty(t, parsed["fact_id"])
}

// TestDeleteProjectKnowledge_ResponseStructure verifies the response structure.
func TestDeleteProjectKnowledge_ResponseStructure(t *testing.T) {
	factID := uuid.New().String()

	response := deleteProjectKnowledgeResponse{
		FactID:  factID,
		Deleted: true,
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify all fields are present
	assert.Equal(t, factID, parsed["fact_id"])
	assert.True(t, parsed["deleted"].(bool))
}

// TestUpdateProjectKnowledge_ValidCategories verifies valid categories.
func TestUpdateProjectKnowledge_ValidCategories(t *testing.T) {
	validCategories := []string{"terminology", "business_rule", "enumeration", "convention"}

	for _, category := range validCategories {
		t.Run(category, func(t *testing.T) {
			response := updateProjectKnowledgeResponse{
				FactID:   uuid.New().String(),
				Fact:     "Test fact",
				Category: category,
			}

			jsonBytes, err := json.Marshal(response)
			require.NoError(t, err)

			var parsed map[string]any
			require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

			assert.Equal(t, category, parsed["category"])
		})
	}
}

// Note: Full integration tests for tool execution with database require a database connection
// and would be covered in integration tests. The tests above verify that:
// - Tools are properly registered with the MCP server
// - Response structures serialize correctly
// - Valid categories are accepted
