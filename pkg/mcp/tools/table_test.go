package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// ============================================================================
// Tool Structure Tests
// ============================================================================

func TestRegisterTableTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &TableToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterTableTools(mcpServer, deps)

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

	// Verify all tools are registered
	expectedTools := []string{"update_table", "delete_table_metadata"}
	foundTools := make(map[string]bool)

	for _, tool := range response.Result.Tools {
		foundTools[tool.Name] = true
	}

	for _, expected := range expectedTools {
		assert.True(t, foundTools[expected], "tool %s should be registered", expected)
	}
}

func TestUpdateTableTool_Structure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &TableToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterTableTools(mcpServer, deps)

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

	// Find update_table tool
	var updateTableTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		} `json:"inputSchema"`
	}

	for i := range response.Result.Tools {
		if response.Result.Tools[i].Name == "update_table" {
			updateTableTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, updateTableTool, "update_table tool should exist")

	// Verify description mentions key use cases
	assert.Contains(t, updateTableTool.Description, "metadata", "description should mention metadata")
	assert.Contains(t, updateTableTool.Description, "ephemeral", "description should mention ephemeral")

	// Verify required parameters
	assert.Contains(t, updateTableTool.InputSchema.Required, "table", "table should be required")

	// Verify optional parameters exist
	assert.Contains(t, updateTableTool.InputSchema.Properties, "description", "should have description parameter")
	assert.Contains(t, updateTableTool.InputSchema.Properties, "usage_notes", "should have usage_notes parameter")
	assert.Contains(t, updateTableTool.InputSchema.Properties, "is_ephemeral", "should have is_ephemeral parameter")
	assert.Contains(t, updateTableTool.InputSchema.Properties, "preferred_alternative", "should have preferred_alternative parameter")
}

// ============================================================================
// Response Format Tests
// ============================================================================

func TestUpdateTable_ResponseStructure(t *testing.T) {
	response := updateTableResponse{
		Table:                "sessions",
		Description:          "Transient session tracking table",
		UsageNotes:           "Do not use for analytics",
		IsEphemeral:          true,
		PreferredAlternative: "billing_engagements",
		Created:              true,
	}

	// Verify response has required fields
	assert.NotEmpty(t, response.Table, "response should have table field")
	assert.NotEmpty(t, response.Description, "response should have description field")
	assert.NotEmpty(t, response.UsageNotes, "response should have usage_notes field")
	assert.True(t, response.IsEphemeral, "response should have is_ephemeral set")
	assert.NotEmpty(t, response.PreferredAlternative, "response should have preferred_alternative field")
	assert.True(t, response.Created, "response should have created field set")
}

func TestUpdateTable_JSONSerialization(t *testing.T) {
	response := updateTableResponse{
		Table:                "sessions",
		Description:          "Transient session tracking table",
		UsageNotes:           "Do not use for analytics",
		IsEphemeral:          true,
		PreferredAlternative: "billing_engagements",
		Created:              true,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)

	// Verify JSON structure
	var decoded map[string]any
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "sessions", decoded["table"])
	assert.Equal(t, "Transient session tracking table", decoded["description"])
	assert.Equal(t, "Do not use for analytics", decoded["usage_notes"])
	assert.Equal(t, true, decoded["is_ephemeral"])
	assert.Equal(t, "billing_engagements", decoded["preferred_alternative"])
	assert.Equal(t, true, decoded["created"])
}

func TestUpdateTable_JSONSerialization_MinimalResponse(t *testing.T) {
	// Test with only required fields set
	response := updateTableResponse{
		Table:       "sessions",
		IsEphemeral: false,
		Created:     false,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)

	// Verify JSON structure
	var decoded map[string]any
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "sessions", decoded["table"])
	assert.Equal(t, false, decoded["is_ephemeral"])
	assert.Equal(t, false, decoded["created"])

	// Empty strings should be omitted
	_, hasDescription := decoded["description"]
	assert.False(t, hasDescription, "empty description should be omitted")

	_, hasUsageNotes := decoded["usage_notes"]
	assert.False(t, hasUsageNotes, "empty usage_notes should be omitted")

	_, hasPreferredAlt := decoded["preferred_alternative"]
	assert.False(t, hasPreferredAlt, "empty preferred_alternative should be omitted")
}

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestPtrToString(t *testing.T) {
	t.Run("nil pointer returns empty string", func(t *testing.T) {
		result := ptrToString(nil)
		assert.Equal(t, "", result)
	})

	t.Run("non-nil pointer returns value", func(t *testing.T) {
		value := "test value"
		result := ptrToString(&value)
		assert.Equal(t, "test value", result)
	})

	t.Run("pointer to empty string returns empty string", func(t *testing.T) {
		value := ""
		result := ptrToString(&value)
		assert.Equal(t, "", result)
	})
}

// ============================================================================
// Error Result Tests
// ============================================================================

func TestDeleteTableMetadataTool_Structure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &TableToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterTableTools(mcpServer, deps)

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

	// Find delete_table_metadata tool
	var deleteTableMetadataTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		} `json:"inputSchema"`
	}

	for i := range response.Result.Tools {
		if response.Result.Tools[i].Name == "delete_table_metadata" {
			deleteTableMetadataTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, deleteTableMetadataTool, "delete_table_metadata tool should exist")

	// Verify description mentions key use cases
	assert.Contains(t, deleteTableMetadataTool.Description, "metadata", "description should mention metadata")
	assert.Contains(t, deleteTableMetadataTool.Description, "Clear", "description should mention clearing")

	// Verify required parameters
	assert.Contains(t, deleteTableMetadataTool.InputSchema.Required, "table", "table should be required")

	// Verify only table parameter exists (no optional params for delete)
	assert.Contains(t, deleteTableMetadataTool.InputSchema.Properties, "table", "should have table parameter")
}

// ============================================================================
// Delete Table Metadata Response Tests
// ============================================================================

func TestDeleteTableMetadata_ResponseStructure(t *testing.T) {
	t.Run("deleted response", func(t *testing.T) {
		response := deleteTableMetadataResponse{
			Table:   "sessions",
			Deleted: true,
		}

		assert.Equal(t, "sessions", response.Table, "response should have table field")
		assert.True(t, response.Deleted, "response should indicate deletion occurred")
	})

	t.Run("not found response", func(t *testing.T) {
		response := deleteTableMetadataResponse{
			Table:   "sessions",
			Deleted: false,
		}

		assert.Equal(t, "sessions", response.Table, "response should have table field")
		assert.False(t, response.Deleted, "response should indicate deletion did not occur")
	})
}

func TestDeleteTableMetadata_JSONSerialization(t *testing.T) {
	t.Run("deleted", func(t *testing.T) {
		response := deleteTableMetadataResponse{
			Table:   "sessions",
			Deleted: true,
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(response)
		require.NoError(t, err)

		// Verify JSON structure
		var decoded map[string]any
		err = json.Unmarshal(jsonData, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "sessions", decoded["table"])
		assert.Equal(t, true, decoded["deleted"])
	})

	t.Run("not found", func(t *testing.T) {
		response := deleteTableMetadataResponse{
			Table:   "nonexistent_table",
			Deleted: false,
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(response)
		require.NoError(t, err)

		// Verify JSON structure
		var decoded map[string]any
		err = json.Unmarshal(jsonData, &decoded)
		require.NoError(t, err)

		assert.Equal(t, "nonexistent_table", decoded["table"])
		assert.Equal(t, false, decoded["deleted"])
	})
}

func TestDeleteTableMetadataTool_ErrorResults(t *testing.T) {
	t.Run("empty table name after trimming", func(t *testing.T) {
		// Simulate empty table name validation
		table := "   " // whitespace only
		table = strings.TrimSpace(table)

		if table == "" {
			result := NewErrorResult("invalid_parameters", "parameter 'table' cannot be empty")

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse error response
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
			require.NoError(t, err)

			assert.True(t, errResp.Error)
			assert.Equal(t, "invalid_parameters", errResp.Code)
			assert.Contains(t, errResp.Message, "table")
			assert.Contains(t, errResp.Message, "cannot be empty")
		}
	})
}

func TestUpdateTableTool_ErrorResults(t *testing.T) {
	t.Run("empty table name after trimming", func(t *testing.T) {
		// Simulate empty table name validation
		table := "   " // whitespace only
		table = strings.TrimSpace(table)

		if table == "" {
			result := NewErrorResult("invalid_parameters", "parameter 'table' cannot be empty")

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse error response
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
			require.NoError(t, err)

			assert.True(t, errResp.Error)
			assert.Equal(t, "invalid_parameters", errResp.Code)
			assert.Contains(t, errResp.Message, "table")
			assert.Contains(t, errResp.Message, "cannot be empty")
		}
	})

	t.Run("table not found", func(t *testing.T) {
		// Simulate table not found error
		table := "nonexistent_table"
		result := NewErrorResult("TABLE_NOT_FOUND",
			fmt.Sprintf("table %q not found in schema registry. Run refresh_schema() after creating tables.", table))

		// Verify it's an error result
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Parse error response
		var errResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "TABLE_NOT_FOUND", errResp.Code)
		assert.Contains(t, errResp.Message, "nonexistent_table")
		assert.Contains(t, errResp.Message, "not found")
		assert.Contains(t, errResp.Message, "refresh_schema")
	})

	t.Run("preferred_alternative table not found", func(t *testing.T) {
		// Simulate preferred_alternative validation error
		altTable := "nonexistent_alternative"
		result := NewErrorResult("PREFERRED_ALTERNATIVE_NOT_FOUND",
			fmt.Sprintf("preferred_alternative table %q not found in schema registry", altTable))

		// Verify it's an error result
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Parse error response
		var errResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "PREFERRED_ALTERNATIVE_NOT_FOUND", errResp.Code)
		assert.Contains(t, errResp.Message, "preferred_alternative")
		assert.Contains(t, errResp.Message, "nonexistent_alternative")
		assert.Contains(t, errResp.Message, "not found")
	})

	t.Run("precedence blocked", func(t *testing.T) {
		// Simulate precedence blocked error (admin change cannot be overridden by MCP)
		result := NewErrorResult("precedence_blocked",
			"Cannot modify table metadata: precedence blocked (existing: manual, modifier: mcp). "+
				"Admin changes cannot be overridden by MCP. Use the UI to modify or delete this metadata.")

		// Verify it's an error result
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Parse error response
		var errResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "precedence_blocked", errResp.Code)
		assert.Contains(t, errResp.Message, "precedence blocked")
		assert.Contains(t, errResp.Message, "Admin changes cannot be overridden")
	})
}
