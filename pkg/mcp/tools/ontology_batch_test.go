package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================================
// Tool Structure Tests
// ============================================================================

func TestRegisterBatchTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &ColumnToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			Logger: zap.NewNop(),
		},
	}

	RegisterBatchTools(mcpServer, deps)

	ctx := context.Background()
	result := mcpServer.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	resultBytes, err := json.Marshal(result)
	require.NoError(t, err)

	var response struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}

	err = json.Unmarshal(resultBytes, &response)
	require.NoError(t, err)

	// Verify update_columns tool is registered
	foundTool := false
	for _, tool := range response.Result.Tools {
		if tool.Name == "update_columns" {
			foundTool = true
			break
		}
	}

	assert.True(t, foundTool, "update_columns tool should be registered")
}

func TestUpdateColumnsTool_Structure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &ColumnToolDeps{
		BaseMCPToolDeps: BaseMCPToolDeps{
			Logger: zap.NewNop(),
		},
	}

	RegisterBatchTools(mcpServer, deps)

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
					Type       string                 `json:"type"`
					Properties map[string]interface{} `json:"properties"`
					Required   []string               `json:"required"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}

	err = json.Unmarshal(resultBytes, &response)
	require.NoError(t, err)

	// Find update_columns tool
	var updateColumnsTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		} `json:"inputSchema"`
	}

	for i := range response.Result.Tools {
		if response.Result.Tools[i].Name == "update_columns" {
			updateColumnsTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, updateColumnsTool, "update_columns tool should exist")

	// Verify description mentions batch and transaction
	assert.Contains(t, updateColumnsTool.Description, "multiple columns", "description should mention multiple columns")
	assert.Contains(t, updateColumnsTool.Description, "transaction", "description should mention transaction")
	assert.Contains(t, updateColumnsTool.Description, "50", "description should mention the max batch size")

	// Verify required parameters
	assert.Contains(t, updateColumnsTool.InputSchema.Required, "updates", "updates should be required")

	// Verify properties exist
	assert.Contains(t, updateColumnsTool.InputSchema.Properties, "updates", "should have updates parameter")
}

// ============================================================================
// Response Format Tests
// ============================================================================

func TestUpdateColumnsResponse_Structure(t *testing.T) {
	response := UpdateColumnsResponse{
		Updated: 3,
		Results: []ColumnUpdateResult{
			{Table: "users", Column: "deleted_at", Status: "success", Created: true},
			{Table: "accounts", Column: "deleted_at", Status: "success", Created: true},
			{Table: "sessions", Column: "deleted_at", Status: "success", Created: false},
		},
	}

	// Verify response has required fields
	assert.Equal(t, 3, response.Updated, "should report 3 updated columns")
	assert.Len(t, response.Results, 3, "should have 3 results")

	// Verify per-column results
	assert.Equal(t, "users", response.Results[0].Table)
	assert.Equal(t, "deleted_at", response.Results[0].Column)
	assert.Equal(t, "success", response.Results[0].Status)
	assert.True(t, response.Results[0].Created)
}

func TestUpdateColumnsResponse_WithErrors(t *testing.T) {
	response := UpdateColumnsResponse{
		Updated: 0,
		Results: []ColumnUpdateResult{
			{Table: "users", Column: "deleted_at", Status: "error", Error: "table not found"},
			{Table: "accounts", Column: "deleted_at", Status: "pending"},
		},
	}

	// Verify error handling
	assert.Equal(t, 0, response.Updated, "should report 0 updated when errors occur")
	assert.Equal(t, "error", response.Results[0].Status)
	assert.Equal(t, "table not found", response.Results[0].Error)
}

func TestUpdateColumnsResponse_JSONSerialization(t *testing.T) {
	response := UpdateColumnsResponse{
		Updated: 2,
		Results: []ColumnUpdateResult{
			{Table: "users", Column: "deleted_at", Status: "success", Created: true},
			{Table: "accounts", Column: "created_at", Status: "success", Created: false},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)

	// Verify JSON structure
	var decoded map[string]any
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, float64(2), decoded["updated"])

	results, ok := decoded["results"].([]any)
	require.True(t, ok, "results should be an array")
	assert.Len(t, results, 2)

	firstResult, ok := results[0].(map[string]any)
	require.True(t, ok, "first result should be a map")
	assert.Equal(t, "users", firstResult["table"])
	assert.Equal(t, "deleted_at", firstResult["column"])
	assert.Equal(t, "success", firstResult["status"])
	assert.Equal(t, true, firstResult["created"])
}

// ============================================================================
// Parsing Tests
// ============================================================================

func TestParseColumnUpdates_ValidInput(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			map[string]any{
				"table":       "users",
				"column":      "deleted_at",
				"description": "Soft delete timestamp",
			},
			map[string]any{
				"table":       "accounts",
				"column":      "deleted_at",
				"description": "Soft delete timestamp",
				"entity":      "Account",
				"role":        "attribute",
			},
		},
	}

	updates, err := parseColumnUpdates(req)
	require.NoError(t, err)

	assert.Len(t, updates, 2)

	// First update
	assert.Equal(t, "users", updates[0].Table)
	assert.Equal(t, "deleted_at", updates[0].Column)
	assert.NotNil(t, updates[0].Description)
	assert.Equal(t, "Soft delete timestamp", *updates[0].Description)
	assert.Nil(t, updates[0].Entity)
	assert.Nil(t, updates[0].Role)

	// Second update
	assert.Equal(t, "accounts", updates[1].Table)
	assert.Equal(t, "deleted_at", updates[1].Column)
	assert.NotNil(t, updates[1].Description)
	assert.NotNil(t, updates[1].Entity)
	assert.Equal(t, "Account", *updates[1].Entity)
	assert.NotNil(t, updates[1].Role)
	assert.Equal(t, "attribute", *updates[1].Role)
}

func TestParseColumnUpdates_WithEnumValues(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			map[string]any{
				"table":       "users",
				"column":      "status",
				"description": "User account status",
				"enum_values": []any{
					"ACTIVE - Normal active account",
					"SUSPENDED - Temporarily disabled",
				},
			},
		},
	}

	updates, err := parseColumnUpdates(req)
	require.NoError(t, err)

	assert.Len(t, updates, 1)
	assert.Len(t, updates[0].EnumValues, 2)
	assert.Equal(t, "ACTIVE - Normal active account", updates[0].EnumValues[0])
	assert.Equal(t, "SUSPENDED - Temporarily disabled", updates[0].EnumValues[1])
}

func TestParseColumnUpdates_MissingTable(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			map[string]any{
				"column":      "deleted_at",
				"description": "Soft delete timestamp",
			},
		},
	}

	_, err := parseColumnUpdates(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "table")
	assert.Contains(t, err.Error(), "required")
}

func TestParseColumnUpdates_MissingColumn(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			map[string]any{
				"table":       "users",
				"description": "Soft delete timestamp",
			},
		},
	}

	_, err := parseColumnUpdates(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "column")
	assert.Contains(t, err.Error(), "required")
}

func TestParseColumnUpdates_EmptyTable(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			map[string]any{
				"table":  "   ", // whitespace only
				"column": "deleted_at",
			},
		},
	}

	_, err := parseColumnUpdates(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "table")
	assert.Contains(t, err.Error(), "empty")
}

func TestParseColumnUpdates_EmptyColumn(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			map[string]any{
				"table":  "users",
				"column": "   ", // whitespace only
			},
		},
	}

	_, err := parseColumnUpdates(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "column")
	assert.Contains(t, err.Error(), "empty")
}

func TestParseColumnUpdates_InvalidRole(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			map[string]any{
				"table":  "users",
				"column": "deleted_at",
				"role":   "invalid_role",
			},
		},
	}

	_, err := parseColumnUpdates(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "role")
	assert.Contains(t, err.Error(), "must be one of")
	assert.Contains(t, err.Error(), "invalid_role")
}

func TestParseColumnUpdates_ValidRoles(t *testing.T) {
	validRoles := []string{"dimension", "measure", "identifier", "attribute"}

	for _, role := range validRoles {
		t.Run(role, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = map[string]any{
				"updates": []any{
					map[string]any{
						"table":  "users",
						"column": "deleted_at",
						"role":   role,
					},
				},
			}

			updates, err := parseColumnUpdates(req)
			require.NoError(t, err, "role %q should be valid", role)
			assert.NotNil(t, updates[0].Role)
			assert.Equal(t, role, *updates[0].Role)
		})
	}
}

func TestParseColumnUpdates_InvalidEnumValues(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			map[string]any{
				"table":  "users",
				"column": "status",
				"enum_values": []any{
					"ACTIVE",
					123, // int, not string
				},
			},
		},
	}

	_, err := parseColumnUpdates(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "enum_values")
	assert.Contains(t, err.Error(), "string")
}

func TestParseColumnUpdates_NotAnArray(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": "not an array",
	}

	_, err := parseColumnUpdates(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "array")
}

func TestParseColumnUpdates_UpdateNotAnObject(t *testing.T) {
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"updates": []any{
			"not an object",
		},
	}

	_, err := parseColumnUpdates(req)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "object")
}

// ============================================================================
// Batch Size Tests
// ============================================================================

func TestMaxBatchUpdateSize(t *testing.T) {
	assert.Equal(t, 50, MaxBatchUpdateSize, "Max batch size should be 50")
}

// ============================================================================
// Helper Tests
// ============================================================================

func TestFindUpdateIndex(t *testing.T) {
	updates := []ColumnUpdate{
		{Table: "users", Column: "deleted_at"},
		{Table: "accounts", Column: "deleted_at"},
		{Table: "sessions", Column: "deleted_at"},
	}

	// Find first
	assert.Equal(t, 0, findUpdateIndex(updates, "users", "deleted_at", 0))

	// Find second
	assert.Equal(t, 1, findUpdateIndex(updates, "accounts", "deleted_at", 0))

	// Find third
	assert.Equal(t, 2, findUpdateIndex(updates, "sessions", "deleted_at", 0))

	// Find with startFrom hint
	assert.Equal(t, 1, findUpdateIndex(updates, "accounts", "deleted_at", 1))

	// Fallback to beginning when not found after startFrom
	assert.Equal(t, 0, findUpdateIndex(updates, "users", "deleted_at", 2))
}

func TestMustMarshal(t *testing.T) {
	// Valid case
	result := mustMarshal(map[string]string{"key": "value"})
	assert.Equal(t, `{"key":"value"}`, result)

	// Test struct
	response := UpdateColumnsResponse{
		Updated: 1,
		Results: []ColumnUpdateResult{
			{Table: "users", Column: "status", Status: "success"},
		},
	}
	result = mustMarshal(response)
	assert.Contains(t, result, `"updated":1`)
	assert.Contains(t, result, `"table":"users"`)
}

// ============================================================================
// Error Result Tests
// ============================================================================

func TestUpdateColumnsTool_ErrorResults(t *testing.T) {
	t.Run("empty updates array", func(t *testing.T) {
		result := NewErrorResult("invalid_parameters", "updates array cannot be empty")

		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		var errResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "invalid_parameters", errResp.Code)
		assert.Contains(t, errResp.Message, "empty")
	})

	t.Run("too many updates", func(t *testing.T) {
		result := NewErrorResultWithDetails(
			"invalid_parameters",
			fmt.Sprintf("too many updates: maximum %d allowed per call, got %d", MaxBatchUpdateSize, 100),
			map[string]any{
				"max_allowed": MaxBatchUpdateSize,
				"received":    100,
			},
		)

		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		var errResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "invalid_parameters", errResp.Code)
		assert.Contains(t, errResp.Message, "too many updates")
		assert.Contains(t, errResp.Message, "50")
		assert.Contains(t, errResp.Message, "100")

		details, ok := errResp.Details.(map[string]any)
		require.True(t, ok, "details should be a map")
		assert.Equal(t, float64(MaxBatchUpdateSize), details["max_allowed"])
		assert.Equal(t, float64(100), details["received"])
	})
}

// ============================================================================
// ColumnUpdate Type Tests
// ============================================================================

func TestColumnUpdate_OptionalFields(t *testing.T) {
	// Test with all optional fields nil
	update := ColumnUpdate{
		Table:  "users",
		Column: "deleted_at",
	}

	assert.Equal(t, "users", update.Table)
	assert.Equal(t, "deleted_at", update.Column)
	assert.Nil(t, update.Description)
	assert.Nil(t, update.Entity)
	assert.Nil(t, update.Role)
	assert.Nil(t, update.EnumValues)
}

func TestColumnUpdate_JSONSerialization(t *testing.T) {
	desc := "Soft delete timestamp"
	entity := "User"
	role := "attribute"

	update := ColumnUpdate{
		Table:       "users",
		Column:      "deleted_at",
		Description: &desc,
		Entity:      &entity,
		Role:        &role,
		EnumValues:  []string{"value1", "value2"},
	}

	jsonData, err := json.Marshal(update)
	require.NoError(t, err)

	var decoded map[string]any
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "users", decoded["table"])
	assert.Equal(t, "deleted_at", decoded["column"])
	assert.Equal(t, "Soft delete timestamp", decoded["description"])
	assert.Equal(t, "User", decoded["entity"])
	assert.Equal(t, "attribute", decoded["role"])

	enumValues, ok := decoded["enum_values"].([]any)
	require.True(t, ok)
	assert.Len(t, enumValues, 2)
}

func TestColumnUpdate_JSONSerializationOmitsEmpty(t *testing.T) {
	update := ColumnUpdate{
		Table:  "users",
		Column: "deleted_at",
	}

	jsonData, err := json.Marshal(update)
	require.NoError(t, err)

	var decoded map[string]any
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "users", decoded["table"])
	assert.Equal(t, "deleted_at", decoded["column"])
	// Optional fields should be omitted
	_, hasDesc := decoded["description"]
	_, hasEntity := decoded["entity"]
	_, hasRole := decoded["role"]
	_, hasEnum := decoded["enum_values"]

	assert.False(t, hasDesc, "description should be omitted when nil")
	assert.False(t, hasEntity, "entity should be omitted when nil")
	assert.False(t, hasRole, "role should be omitted when nil")
	assert.False(t, hasEnum, "enum_values should be omitted when nil")
}
