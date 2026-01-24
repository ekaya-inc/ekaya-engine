package tools

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestRegisterDevQueryTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &DevQueryToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true},
		},
		ProjectService: &mockProjectService{},
		QueryService:   &mockQueryService{},
		Logger:         zap.NewNop(),
	}

	RegisterDevQueryTools(mcpServer, deps)

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

	// Check list_query_suggestions tool is registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["list_query_suggestions"], "list_query_suggestions tool should be registered")
}

func TestListQuerySuggestions_ResponseStructure(t *testing.T) {
	// Test that the response structure is correct
	suggestionID := uuid.New()
	parentQueryID := uuid.New()

	response := listQuerySuggestionsResponse{
		Suggestions: []querySuggestionInfo{
			{
				ID:           suggestionID.String(),
				Type:         "new",
				Name:         "Get user orders",
				SQL:          "SELECT * FROM orders WHERE user_id = {{user_id}}",
				SuggestedBy:  "agent",
				CreatedAt:    "2024-12-15T10:00:00Z",
				Context:      "User requested this query for reporting",
				DatasourceID: uuid.New().String(),
				Status:       "pending",
			},
			{
				ID:              uuid.New().String(),
				Type:            "update",
				Name:            "Subscribe to list",
				SQL:             "SELECT * FROM subscribe_to_list(...)",
				SuggestedBy:     "agent",
				CreatedAt:       "2024-12-15T10:30:00Z",
				Context:         "Added duplicate prevention",
				ParentQueryID:   parentQueryID.String(),
				ParentQueryName: "Subscribe to list (original)",
				Changes:         []string{"sql", "parameters"},
				DatasourceID:    uuid.New().String(),
				Status:          "pending",
			},
		},
		Count: 2,
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify top-level fields
	assert.Equal(t, float64(2), parsed["count"])
	assert.NotNil(t, parsed["suggestions"])

	// Verify suggestions array structure
	suggestions, ok := parsed["suggestions"].([]any)
	require.True(t, ok)
	require.Len(t, suggestions, 2)

	// Verify first suggestion (new query)
	firstSuggestion, ok := suggestions[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "new", firstSuggestion["type"])
	assert.NotEmpty(t, firstSuggestion["id"])
	assert.NotEmpty(t, firstSuggestion["name"])
	assert.NotEmpty(t, firstSuggestion["sql"])
	assert.NotEmpty(t, firstSuggestion["suggested_by"])
	assert.NotEmpty(t, firstSuggestion["created_at"])
	assert.NotEmpty(t, firstSuggestion["context"])
	assert.Equal(t, "pending", firstSuggestion["status"])
	// New suggestions should NOT have parent fields
	_, hasParentID := firstSuggestion["parent_query_id"]
	assert.False(t, hasParentID || firstSuggestion["parent_query_id"] == "", "new suggestions should not have parent_query_id")

	// Verify second suggestion (update)
	secondSuggestion, ok := suggestions[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "update", secondSuggestion["type"])
	assert.NotEmpty(t, secondSuggestion["parent_query_id"])
	assert.NotEmpty(t, secondSuggestion["parent_query_name"])
	assert.NotNil(t, secondSuggestion["changes"])

	// Verify changes array
	changes, ok := secondSuggestion["changes"].([]any)
	require.True(t, ok)
	assert.Len(t, changes, 2)
	assert.Equal(t, "sql", changes[0])
	assert.Equal(t, "parameters", changes[1])
}

func TestListQuerySuggestions_EmptyResults(t *testing.T) {
	response := listQuerySuggestionsResponse{
		Suggestions: []querySuggestionInfo{},
		Count:       0,
	}

	// Verify JSON serialization handles empty array correctly
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	assert.Equal(t, float64(0), parsed["count"])

	suggestions, ok := parsed["suggestions"].([]any)
	require.True(t, ok)
	assert.Empty(t, suggestions, "suggestions should be empty array")
}

func TestCalculateChanges(t *testing.T) {
	parentQueryID := uuid.New()
	projectID := uuid.New()
	datasourceID := uuid.New()

	// Create parent query
	parent := &models.Query{
		ID:                    parentQueryID,
		ProjectID:             projectID,
		DatasourceID:          datasourceID,
		NaturalLanguagePrompt: "Original Name",
		SQLQuery:              "SELECT * FROM users",
		Parameters:            []models.QueryParameter{{Name: "id", Type: "uuid", Required: true}},
		OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid"}},
		Tags:                  []string{"tag1"},
		AllowsModification:    false,
	}

	tests := []struct {
		name           string
		suggestion     *models.Query
		expectedFields []string
	}{
		{
			name: "SQL changed",
			suggestion: &models.Query{
				NaturalLanguagePrompt: "Original Name",
				SQLQuery:              "SELECT * FROM users WHERE active = true",
				Parameters:            []models.QueryParameter{{Name: "id", Type: "uuid", Required: true}},
				OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid"}},
				Tags:                  []string{"tag1"},
				AllowsModification:    false,
			},
			expectedFields: []string{"sql"},
		},
		{
			name: "Name changed",
			suggestion: &models.Query{
				NaturalLanguagePrompt: "Updated Name",
				SQLQuery:              "SELECT * FROM users",
				Parameters:            []models.QueryParameter{{Name: "id", Type: "uuid", Required: true}},
				OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid"}},
				Tags:                  []string{"tag1"},
				AllowsModification:    false,
			},
			expectedFields: []string{"name"},
		},
		{
			name: "Parameters changed",
			suggestion: &models.Query{
				NaturalLanguagePrompt: "Original Name",
				SQLQuery:              "SELECT * FROM users",
				Parameters:            []models.QueryParameter{{Name: "id", Type: "uuid", Required: false}}, // Changed required
				OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid"}},
				Tags:                  []string{"tag1"},
				AllowsModification:    false,
			},
			expectedFields: []string{"parameters"},
		},
		{
			name: "Tags changed",
			suggestion: &models.Query{
				NaturalLanguagePrompt: "Original Name",
				SQLQuery:              "SELECT * FROM users",
				Parameters:            []models.QueryParameter{{Name: "id", Type: "uuid", Required: true}},
				OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid"}},
				Tags:                  []string{"tag1", "tag2"}, // Added tag
				AllowsModification:    false,
			},
			expectedFields: []string{"tags"},
		},
		{
			name: "Multiple changes",
			suggestion: &models.Query{
				NaturalLanguagePrompt: "Updated Name",
				SQLQuery:              "SELECT * FROM users WHERE active = true",
				Parameters:            []models.QueryParameter{{Name: "id", Type: "uuid", Required: true}},
				OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid"}},
				Tags:                  []string{"tag1", "tag2"},
				AllowsModification:    false,
			},
			expectedFields: []string{"sql", "name", "tags"},
		},
		{
			name: "No changes",
			suggestion: &models.Query{
				NaturalLanguagePrompt: "Original Name",
				SQLQuery:              "SELECT * FROM users",
				Parameters:            []models.QueryParameter{{Name: "id", Type: "uuid", Required: true}},
				OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid"}},
				Tags:                  []string{"tag1"},
				AllowsModification:    false,
			},
			expectedFields: []string{},
		},
		{
			name: "AllowsModification changed",
			suggestion: &models.Query{
				NaturalLanguagePrompt: "Original Name",
				SQLQuery:              "SELECT * FROM users",
				Parameters:            []models.QueryParameter{{Name: "id", Type: "uuid", Required: true}},
				OutputColumns:         []models.OutputColumn{{Name: "id", Type: "uuid"}},
				Tags:                  []string{"tag1"},
				AllowsModification:    true, // Changed from false
			},
			expectedFields: []string{"allows_modification"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changes := calculateChanges(parent, tt.suggestion)

			// Check that all expected fields are present
			for _, expected := range tt.expectedFields {
				assert.Contains(t, changes, expected, "changes should contain %q", expected)
			}

			// Check that no unexpected fields are present
			assert.Len(t, changes, len(tt.expectedFields), "changes should have exactly %d elements", len(tt.expectedFields))
		})
	}
}

func TestStringPtrEqual(t *testing.T) {
	str1 := "hello"
	str2 := "hello"
	str3 := "world"

	tests := []struct {
		name     string
		a        *string
		b        *string
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "a nil, b not nil",
			a:        nil,
			b:        &str1,
			expected: false,
		},
		{
			name:     "a not nil, b nil",
			a:        &str1,
			b:        nil,
			expected: false,
		},
		{
			name:     "both equal",
			a:        &str1,
			b:        &str2,
			expected: true,
		},
		{
			name:     "both different",
			a:        &str1,
			b:        &str3,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stringPtrEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParametersEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []models.QueryParameter
		b        []models.QueryParameter
		expected bool
	}{
		{
			name:     "both empty",
			a:        []models.QueryParameter{},
			b:        []models.QueryParameter{},
			expected: true,
		},
		{
			name: "different lengths",
			a:    []models.QueryParameter{{Name: "id", Type: "uuid"}},
			b:    []models.QueryParameter{},
			expected: false,
		},
		{
			name: "same parameters",
			a: []models.QueryParameter{
				{Name: "id", Type: "uuid", Description: "User ID", Required: true},
			},
			b: []models.QueryParameter{
				{Name: "id", Type: "uuid", Description: "User ID", Required: true},
			},
			expected: true,
		},
		{
			name: "different name",
			a: []models.QueryParameter{
				{Name: "id", Type: "uuid"},
			},
			b: []models.QueryParameter{
				{Name: "user_id", Type: "uuid"},
			},
			expected: false,
		},
		{
			name: "different type",
			a: []models.QueryParameter{
				{Name: "id", Type: "uuid"},
			},
			b: []models.QueryParameter{
				{Name: "id", Type: "string"},
			},
			expected: false,
		},
		{
			name: "different required",
			a: []models.QueryParameter{
				{Name: "id", Type: "uuid", Required: true},
			},
			b: []models.QueryParameter{
				{Name: "id", Type: "uuid", Required: false},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parametersEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestOutputColumnsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []models.OutputColumn
		b        []models.OutputColumn
		expected bool
	}{
		{
			name:     "both empty",
			a:        []models.OutputColumn{},
			b:        []models.OutputColumn{},
			expected: true,
		},
		{
			name: "same columns",
			a: []models.OutputColumn{
				{Name: "id", Type: "uuid", Description: "User ID"},
			},
			b: []models.OutputColumn{
				{Name: "id", Type: "uuid", Description: "User ID"},
			},
			expected: true,
		},
		{
			name: "different name",
			a: []models.OutputColumn{
				{Name: "id", Type: "uuid"},
			},
			b: []models.OutputColumn{
				{Name: "user_id", Type: "uuid"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := outputColumnsEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTagsEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        []string
		b        []string
		expected bool
	}{
		{
			name:     "both empty",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "same tags",
			a:        []string{"tag1", "tag2"},
			b:        []string{"tag1", "tag2"},
			expected: true,
		},
		{
			name:     "different order",
			a:        []string{"tag1", "tag2"},
			b:        []string{"tag2", "tag1"},
			expected: false, // Order matters
		},
		{
			name:     "different lengths",
			a:        []string{"tag1"},
			b:        []string{"tag1", "tag2"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tagsEqual(tt.a, tt.b)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestListQuerySuggestions_ToolDescription(t *testing.T) {
	// Verify the tool description and parameters
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &DevQueryToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true},
		},
		ProjectService: &mockProjectService{},
		QueryService:   &mockQueryService{},
		Logger:         zap.NewNop(),
	}

	RegisterDevQueryTools(mcpServer, deps)

	ctx := context.Background()
	result := mcpServer.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema struct {
					Required   []string       `json:"required"`
					Properties map[string]any `json:"properties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(resultBytes, &response))

	// Find list_query_suggestions tool
	var listTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Required   []string       `json:"required"`
			Properties map[string]any `json:"properties"`
		} `json:"inputSchema"`
	}
	for i, tool := range response.Result.Tools {
		if tool.Name == "list_query_suggestions" {
			listTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, listTool, "list_query_suggestions tool should be found")

	// Verify description mentions key points
	assert.Contains(t, listTool.Description, "pending")
	assert.Contains(t, listTool.Description, "suggestion")
	assert.Contains(t, listTool.Description, "review")

	// Verify optional parameters exist
	assert.Contains(t, listTool.InputSchema.Properties, "status")
	assert.Contains(t, listTool.InputSchema.Properties, "datasource_id")

	// No required parameters (all are optional)
	assert.Empty(t, listTool.InputSchema.Required, "all parameters should be optional")
}

func TestQuerySuggestionInfo_OmitsEmptyFields(t *testing.T) {
	// Test that empty optional fields are omitted in JSON
	info := querySuggestionInfo{
		ID:           uuid.New().String(),
		Type:         "new",
		Name:         "Test Query",
		SQL:          "SELECT 1",
		SuggestedBy:  "agent",
		CreatedAt:    time.Now().Format("2006-01-02T15:04:05Z"),
		DatasourceID: uuid.New().String(),
		Status:       "pending",
		// Context, ParentQueryID, ParentQueryName, Changes are empty
	}

	jsonBytes, err := json.Marshal(info)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify empty string fields are omitted (due to omitempty tag)
	if context, ok := parsed["context"].(string); ok {
		assert.Empty(t, context, "context should be empty string or omitted")
	}

	if parentID, ok := parsed["parent_query_id"].(string); ok {
		assert.Empty(t, parentID, "parent_query_id should be empty string or omitted")
	}

	if parentName, ok := parsed["parent_query_name"].(string); ok {
		assert.Empty(t, parentName, "parent_query_name should be empty string or omitted")
	}

	// Changes is a slice, so it either won't be present or will be null/empty array
	if changes, ok := parsed["changes"].([]any); ok {
		assert.Empty(t, changes, "changes should be empty when no changes")
	}
}
