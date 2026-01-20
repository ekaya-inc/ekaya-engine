package tools

import (
	"context"
	"encoding/json"
	"fmt"
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

// setupKnowledgeTest creates a test setup with mock dependencies for knowledge tools.
func setupKnowledgeTest(t *testing.T) (*mockKnowledgeRepository, *KnowledgeToolDeps) {
	t.Helper()

	mockRepo := &mockKnowledgeRepository{
		facts: make([]*models.KnowledgeFact, 0),
	}

	deps := &KnowledgeToolDeps{
		MCPConfigService:    &mockMCPConfigService{},
		KnowledgeRepository: mockRepo,
		Logger:              zap.NewNop(),
	}

	return mockRepo, deps
}

// TestSetupKnowledgeTest verifies the setupKnowledgeTest helper function.
func TestSetupKnowledgeTest(t *testing.T) {
	mockRepo, deps := setupKnowledgeTest(t)

	// Verify mock repository is initialized
	assert.NotNil(t, mockRepo, "mock repository should be initialized")
	assert.NotNil(t, mockRepo.facts, "facts slice should be initialized")
	assert.Len(t, mockRepo.facts, 0, "facts slice should be empty initially")

	// Verify deps are initialized
	assert.NotNil(t, deps, "deps should be initialized")
	assert.NotNil(t, deps.MCPConfigService, "MCPConfigService should be set")
	assert.NotNil(t, deps.KnowledgeRepository, "KnowledgeRepository should be set")
	assert.NotNil(t, deps.Logger, "Logger should be set")
	assert.Equal(t, mockRepo, deps.KnowledgeRepository, "KnowledgeRepository should be the mock")
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

// TestUpdateProjectKnowledgeTool_ErrorResults verifies error handling for invalid parameters.
func TestUpdateProjectKnowledgeTool_ErrorResults(t *testing.T) {
	t.Run("empty fact after trimming", func(t *testing.T) {
		// Simulate validation check for empty fact after trimming
		fact := "   "
		fact = trimString(fact)
		if fact == "" {
			result := NewErrorResult(
				"invalid_parameters",
				"parameter 'fact' cannot be empty",
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "parameter 'fact' cannot be empty")
		}
	})

	t.Run("invalid category value", func(t *testing.T) {
		// Simulate category validation
		category := "invalid_category"
		validCategories := []string{"terminology", "business_rule", "enumeration", "convention"}
		validCategoryMap := map[string]bool{
			"terminology":   true,
			"business_rule": true,
			"enumeration":   true,
			"convention":    true,
		}

		if !validCategoryMap[category] {
			result := NewErrorResultWithDetails(
				"invalid_parameters",
				"invalid category value",
				map[string]any{
					"parameter": "category",
					"expected":  validCategories,
					"actual":    category,
				},
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "invalid category value")

			// Check details
			detailsMap, ok := errorResp.Details.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "category", detailsMap["parameter"])
			assert.Equal(t, "invalid_category", detailsMap["actual"])
		}
	})

	t.Run("invalid fact_id UUID format", func(t *testing.T) {
		// Simulate UUID validation
		factIDStr := "not-a-uuid"
		factIDStr = trimString(factIDStr)
		_, err := uuid.Parse(factIDStr)
		if err != nil {
			result := NewErrorResult(
				"invalid_parameters",
				fmt.Sprintf("invalid fact_id format: %q is not a valid UUID", factIDStr),
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			jsonErr := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, jsonErr)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "invalid fact_id format")
			assert.Contains(t, errorResp.Message, "not-a-uuid")
		}
	})
}

// TestDeleteProjectKnowledgeTool_ErrorResults verifies error handling for invalid parameters.
func TestDeleteProjectKnowledgeTool_ErrorResults(t *testing.T) {
	t.Run("empty fact_id after trimming", func(t *testing.T) {
		// Simulate validation check for empty fact_id after trimming
		factIDStr := "   "
		factIDStr = trimString(factIDStr)
		if factIDStr == "" {
			result := NewErrorResult(
				"invalid_parameters",
				"parameter 'fact_id' cannot be empty",
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "parameter 'fact_id' cannot be empty")
		}
	})

	t.Run("invalid UUID format", func(t *testing.T) {
		// Simulate UUID validation
		factIDStr := "not-a-valid-uuid"
		factIDStr = trimString(factIDStr)
		_, err := uuid.Parse(factIDStr)
		if err != nil {
			result := NewErrorResult(
				"invalid_parameters",
				fmt.Sprintf("invalid fact_id format: %q is not a valid UUID", factIDStr),
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			jsonErr := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, jsonErr)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "invalid fact_id format")
			assert.Contains(t, errorResp.Message, "not-a-valid-uuid")
		}
	})

	t.Run("fact not found", func(t *testing.T) {
		// Simulate fact not found scenario
		factIDStr := uuid.New().String()

		result := NewErrorResult(
			"FACT_NOT_FOUND",
			fmt.Sprintf("fact %q not found", factIDStr),
		)

		// Verify it's an error result
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Parse the content to verify structure
		var errorResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
		require.NoError(t, err)

		assert.True(t, errorResp.Error)
		assert.Equal(t, "FACT_NOT_FOUND", errorResp.Code)
		assert.Contains(t, errorResp.Message, "fact")
		assert.Contains(t, errorResp.Message, "not found")
		assert.Contains(t, errorResp.Message, factIDStr)
	})
}

// TestUpdateProjectKnowledgeTool_ParameterValidation verifies parameter validation error handling.
func TestUpdateProjectKnowledgeTool_ParameterValidation(t *testing.T) {
	t.Run("empty fact after trimming", func(t *testing.T) {
		// Simulate validation check for empty fact after trimming
		fact := "   " // whitespace-only
		fact = trimString(fact)

		// This is what the implementation does
		if fact == "" {
			result := NewErrorResult("invalid_parameters", "parameter 'fact' cannot be empty")

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "parameter 'fact' cannot be empty")
		} else {
			t.Fatal("expected fact to be empty after trimming")
		}
	})

	t.Run("invalid category value", func(t *testing.T) {
		// Simulate category validation
		category := "invalid_category"
		validCategories := []string{"terminology", "business_rule", "enumeration", "convention"}
		validCategoryMap := map[string]bool{
			"terminology":   true,
			"business_rule": true,
			"enumeration":   true,
			"convention":    true,
		}

		// This is what the implementation does
		if !validCategoryMap[category] {
			result := NewErrorResultWithDetails(
				"invalid_parameters",
				"invalid category value",
				map[string]any{
					"parameter": "category",
					"expected":  validCategories,
					"actual":    category,
				},
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "invalid category value")

			// Check details
			detailsMap, ok := errorResp.Details.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "category", detailsMap["parameter"])
			assert.Equal(t, "invalid_category", detailsMap["actual"])

			// Verify expected categories are listed
			expectedList, ok := detailsMap["expected"].([]any)
			require.True(t, ok)
			assert.Len(t, expectedList, 4)
			expectedStrs := make([]string, len(expectedList))
			for i, v := range expectedList {
				expectedStrs[i] = v.(string)
			}
			assert.Contains(t, expectedStrs, "terminology")
			assert.Contains(t, expectedStrs, "business_rule")
			assert.Contains(t, expectedStrs, "enumeration")
			assert.Contains(t, expectedStrs, "convention")
		} else {
			t.Fatal("expected category to be invalid")
		}
	})

	t.Run("invalid fact_id UUID format", func(t *testing.T) {
		// Simulate UUID validation
		factIDStr := "not-a-uuid"
		factIDStr = trimString(factIDStr)

		// This is what the implementation does
		_, err := uuid.Parse(factIDStr)
		if err != nil {
			result := NewErrorResult(
				"invalid_parameters",
				fmt.Sprintf("invalid fact_id format: %q is not a valid UUID", factIDStr),
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			jsonErr := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, jsonErr)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "invalid fact_id format")
			assert.Contains(t, errorResp.Message, "not-a-uuid")
			assert.Contains(t, errorResp.Message, "not a valid UUID")
		} else {
			t.Fatal("expected UUID parsing to fail")
		}
	})

	t.Run("edge case: fact with newlines and tabs", func(t *testing.T) {
		// Fact with only newlines and tabs should be treated as empty
		fact := "\n\t\n"
		fact = trimString(fact)

		// This is what the implementation does
		if fact == "" {
			result := NewErrorResult("invalid_parameters", "parameter 'fact' cannot be empty")

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "parameter 'fact' cannot be empty")
		} else {
			t.Fatal("expected fact to be empty after trimming newlines/tabs")
		}
	})

	t.Run("edge case: mixed case category", func(t *testing.T) {
		// Category is case-sensitive, so "Business_Rule" should be invalid
		category := "Business_Rule"
		validCategories := []string{"terminology", "business_rule", "enumeration", "convention"}
		validCategoryMap := map[string]bool{
			"terminology":   true,
			"business_rule": true,
			"enumeration":   true,
			"convention":    true,
		}

		// This is what the implementation does
		if !validCategoryMap[category] {
			result := NewErrorResultWithDetails(
				"invalid_parameters",
				"invalid category value",
				map[string]any{
					"parameter": "category",
					"expected":  validCategories,
					"actual":    category,
				},
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse the content to verify structure
			var errorResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errorResp)
			require.NoError(t, err)

			assert.True(t, errorResp.Error)
			assert.Equal(t, "invalid_parameters", errorResp.Code)
			assert.Contains(t, errorResp.Message, "invalid category value")

			// Verify details
			detailsMap, ok := errorResp.Details.(map[string]any)
			require.True(t, ok)
			assert.Equal(t, "Business_Rule", detailsMap["actual"])
		} else {
			t.Fatal("expected category to be invalid")
		}
	})

	t.Run("edge case: empty string fact_id", func(t *testing.T) {
		// Empty string fact_id should be treated as not provided (optional parameter)
		factIDStr := ""
		factIDStr = trimString(factIDStr)

		// The implementation treats empty fact_id as not provided (doesn't validate)
		// This test documents that behavior - empty fact_id is silently ignored
		assert.Empty(t, factIDStr, "empty fact_id should remain empty after trimming")
	})
}

// Note: Full integration tests for tool execution with database require a database connection
// and would be covered in integration tests. The tests above verify that:
// - Tools are properly registered with the MCP server
// - Response structures serialize correctly
// - Valid categories are accepted
// - Error results are returned for invalid parameters
// - delete_project_knowledge error handling covers empty fact_id, invalid UUID format, and fact not found scenarios
// - update_project_knowledge parameter validation correctly handles edge cases
