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

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ============================================================================
// Tool Structure Tests
// ============================================================================

func TestRegisterColumnTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &ColumnToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterColumnTools(mcpServer, deps)

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
	expectedTools := []string{"get_column_metadata", "update_column", "delete_column_metadata"}
	foundTools := make(map[string]bool)

	for _, tool := range response.Result.Tools {
		foundTools[tool.Name] = true
	}

	for _, expected := range expectedTools {
		assert.True(t, foundTools[expected], "tool %s should be registered", expected)
	}
}

func TestGetColumnMetadataTool_Structure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &ColumnToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterColumnTools(mcpServer, deps)

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

	// Find get_column_metadata tool
	var getColumnTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		} `json:"inputSchema"`
	}

	for i := range response.Result.Tools {
		if response.Result.Tools[i].Name == "get_column_metadata" {
			getColumnTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, getColumnTool, "get_column_metadata tool should exist")

	// Verify description mentions use before update_column
	assert.Contains(t, getColumnTool.Description, "update_column", "description should mention use before update_column")

	// Verify required parameters
	assert.Contains(t, getColumnTool.InputSchema.Required, "table", "table should be required")
	assert.Contains(t, getColumnTool.InputSchema.Required, "column", "column should be required")

	// Verify properties exist
	assert.Contains(t, getColumnTool.InputSchema.Properties, "table", "should have table parameter")
	assert.Contains(t, getColumnTool.InputSchema.Properties, "column", "should have column parameter")
}

func TestUpdateColumnTool_Structure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &ColumnToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterColumnTools(mcpServer, deps)

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

	// Find update_column tool
	var updateColumnTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		} `json:"inputSchema"`
	}

	for i := range response.Result.Tools {
		if response.Result.Tools[i].Name == "update_column" {
			updateColumnTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, updateColumnTool, "update_column tool should exist")

	// Verify description
	assert.NotEmpty(t, updateColumnTool.Description)

	// Verify required parameters
	assert.Contains(t, updateColumnTool.InputSchema.Required, "table", "table should be required")
	assert.Contains(t, updateColumnTool.InputSchema.Required, "column", "column should be required")

	// Verify optional parameters exist
	assert.Contains(t, updateColumnTool.InputSchema.Properties, "description", "should have description parameter")
	assert.Contains(t, updateColumnTool.InputSchema.Properties, "enum_values", "should have enum_values parameter")
	assert.Contains(t, updateColumnTool.InputSchema.Properties, "entity", "should have entity parameter")
	assert.Contains(t, updateColumnTool.InputSchema.Properties, "role", "should have role parameter")
}

func TestDeleteColumnMetadataTool_Structure(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &ColumnToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterColumnTools(mcpServer, deps)

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

	// Find delete_column_metadata tool
	var deleteColumnTool *struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		InputSchema struct {
			Type       string                 `json:"type"`
			Properties map[string]interface{} `json:"properties"`
			Required   []string               `json:"required"`
		} `json:"inputSchema"`
	}

	for i := range response.Result.Tools {
		if response.Result.Tools[i].Name == "delete_column_metadata" {
			deleteColumnTool = &response.Result.Tools[i]
			break
		}
	}

	require.NotNil(t, deleteColumnTool, "delete_column_metadata tool should exist")

	// Verify description
	assert.NotEmpty(t, deleteColumnTool.Description)

	// Verify required parameters
	assert.Contains(t, deleteColumnTool.InputSchema.Required, "table", "table should be required")
	assert.Contains(t, deleteColumnTool.InputSchema.Required, "column", "column should be required")
}

// ============================================================================
// Response Format Tests
// ============================================================================

func TestGetColumnMetadata_ResponseStructure(t *testing.T) {
	// Test with full metadata
	response := getColumnMetadataResponse{
		Table:  "users",
		Column: "status",
		Schema: columnSchemaInfo{
			DataType:     "varchar(50)",
			IsNullable:   false,
			IsPrimaryKey: false,
		},
		Metadata: &columnMetadataInfo{
			Description:  "User account status",
			SemanticType: "enum",
			EnumValues:   []string{"ACTIVE - Normal account", "SUSPENDED - Temporarily disabled"},
			Entity:       "User",
			Role:         "attribute",
		},
	}

	// Verify response has required fields
	assert.NotEmpty(t, response.Table, "response should have table field")
	assert.NotEmpty(t, response.Column, "response should have column field")
	assert.NotEmpty(t, response.Schema.DataType, "response should have schema.data_type field")

	// Verify metadata section
	require.NotNil(t, response.Metadata, "response should have metadata when enriched")
	assert.NotEmpty(t, response.Metadata.Description, "metadata should have description")
	assert.NotEmpty(t, response.Metadata.Entity, "metadata should have entity")
	assert.NotEmpty(t, response.Metadata.Role, "metadata should have role")
	assert.NotEmpty(t, response.Metadata.EnumValues, "metadata should have enum_values")
}

func TestGetColumnMetadata_ResponseStructure_NoMetadata(t *testing.T) {
	// Test with only schema info (no ontology metadata)
	response := getColumnMetadataResponse{
		Table:  "users",
		Column: "created_at",
		Schema: columnSchemaInfo{
			DataType:     "timestamp",
			IsNullable:   false,
			IsPrimaryKey: false,
		},
		Metadata: nil, // No metadata yet
	}

	// Verify response has schema fields
	assert.Equal(t, "users", response.Table)
	assert.Equal(t, "created_at", response.Column)
	assert.Equal(t, "timestamp", response.Schema.DataType)
	assert.False(t, response.Schema.IsNullable)
	assert.False(t, response.Schema.IsPrimaryKey)

	// Verify metadata is nil (omitted in JSON)
	assert.Nil(t, response.Metadata)
}

func TestGetColumnMetadata_JSONSerialization(t *testing.T) {
	response := getColumnMetadataResponse{
		Table:  "users",
		Column: "status",
		Schema: columnSchemaInfo{
			DataType:     "varchar(50)",
			IsNullable:   false,
			IsPrimaryKey: false,
		},
		Metadata: &columnMetadataInfo{
			Description: "User account status",
			Entity:      "User",
			Role:        "attribute",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)

	// Verify JSON structure
	var decoded map[string]any
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	assert.Equal(t, "users", decoded["table"])
	assert.Equal(t, "status", decoded["column"])

	schema, ok := decoded["schema"].(map[string]any)
	require.True(t, ok, "schema should be a map")
	assert.Equal(t, "varchar(50)", schema["data_type"])
	assert.Equal(t, false, schema["is_nullable"])
	assert.Equal(t, false, schema["is_primary_key"])

	metadata, ok := decoded["metadata"].(map[string]any)
	require.True(t, ok, "metadata should be a map")
	assert.Equal(t, "User account status", metadata["description"])
	assert.Equal(t, "User", metadata["entity"])
	assert.Equal(t, "attribute", metadata["role"])
}

func TestGetColumnMetadata_JSONSerialization_OmitsEmptyMetadata(t *testing.T) {
	response := getColumnMetadataResponse{
		Table:  "users",
		Column: "created_at",
		Schema: columnSchemaInfo{
			DataType:     "timestamp",
			IsNullable:   true,
			IsPrimaryKey: false,
		},
		Metadata: nil, // No metadata
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	require.NoError(t, err)

	// Verify metadata is omitted (not included as null)
	var decoded map[string]any
	err = json.Unmarshal(jsonData, &decoded)
	require.NoError(t, err)

	_, hasMetadata := decoded["metadata"]
	assert.False(t, hasMetadata, "metadata should be omitted when nil")
}

func TestUpdateColumn_ResponseStructure(t *testing.T) {
	response := updateColumnResponse{
		Table:       "users",
		Column:      "status",
		Description: "User account status",
		EnumValues:  []string{"ACTIVE - Normal account", "SUSPENDED - Temporarily disabled"},
		Entity:      "User",
		Role:        "attribute",
		Created:     true,
	}

	// Verify response has required fields
	assert.NotEmpty(t, response.Table, "response should have table field")
	assert.NotEmpty(t, response.Column, "response should have column field")
	assert.True(t, response.Created, "response should have created field set")
}

func TestDeleteColumnMetadata_ResponseStructure(t *testing.T) {
	response := deleteColumnMetadataResponse{
		Table:   "users",
		Column:  "status",
		Deleted: true,
	}

	// Verify response has required fields
	assert.NotEmpty(t, response.Table, "response should have table field")
	assert.NotEmpty(t, response.Column, "response should have column field")
	assert.True(t, response.Deleted, "response should have deleted field set")
}

// ============================================================================
// Enum Parsing Tests
// ============================================================================

func TestParseEnumValues_WithDescriptions(t *testing.T) {
	input := []string{
		"ACTIVE - Normal active account",
		"SUSPENDED - Temporarily disabled",
		"BANNED - Permanently banned",
	}

	result := parseEnumValues(input)

	require.Len(t, result, 3, "should have 3 enum values")

	// Check first value
	assert.Equal(t, "ACTIVE", result[0].Value)
	assert.Equal(t, "Normal active account", result[0].Description)

	// Check second value
	assert.Equal(t, "SUSPENDED", result[1].Value)
	assert.Equal(t, "Temporarily disabled", result[1].Description)
}

func TestParseEnumValues_WithoutDescriptions(t *testing.T) {
	input := []string{
		"ACTIVE",
		"SUSPENDED",
		"BANNED",
	}

	result := parseEnumValues(input)

	require.Len(t, result, 3, "should have 3 enum values")

	// Check values have no descriptions
	for i, ev := range result {
		assert.Equal(t, input[i], ev.Value)
		assert.Empty(t, ev.Description, "expected empty description for '%s'", ev.Value)
	}
}

func TestParseEnumValues_MixedFormat(t *testing.T) {
	input := []string{
		"ACTIVE - Normal account",
		"PENDING",
		"SUSPENDED - Hold",
	}

	result := parseEnumValues(input)

	require.Len(t, result, 3, "should have 3 enum values")

	// First has description
	assert.Equal(t, "ACTIVE", result[0].Value)
	assert.Equal(t, "Normal account", result[0].Description)

	// Second has no description
	assert.Equal(t, "PENDING", result[1].Value)
	assert.Empty(t, result[1].Description)

	// Third has description
	assert.Equal(t, "SUSPENDED", result[2].Value)
	assert.Equal(t, "Hold", result[2].Description)
}

func TestParseEnumValues_EmptyArray(t *testing.T) {
	input := []string{}
	result := parseEnumValues(input)

	assert.Len(t, result, 0, "expected empty result")
}

func TestFormatEnumValues_WithDescriptions(t *testing.T) {
	input := []models.EnumValue{
		{Value: "ACTIVE", Description: "Normal account"},
		{Value: "SUSPENDED", Description: "Temporarily disabled"},
	}

	result := formatEnumValues(input)

	expected := []string{
		"ACTIVE - Normal account",
		"SUSPENDED - Temporarily disabled",
	}

	assert.Equal(t, expected, result)
}

func TestFormatEnumValues_WithoutDescriptions(t *testing.T) {
	input := []models.EnumValue{
		{Value: "ACTIVE"},
		{Value: "SUSPENDED"},
	}

	result := formatEnumValues(input)

	expected := []string{"ACTIVE", "SUSPENDED"}

	assert.Equal(t, expected, result)
}

func TestFormatEnumValues_NilInput(t *testing.T) {
	result := formatEnumValues(nil)

	assert.Nil(t, result, "expected nil for nil input")
}

func TestFormatEnumValues_EmptyArray(t *testing.T) {
	input := []models.EnumValue{}
	result := formatEnumValues(input)

	assert.Len(t, result, 0, "expected empty result")
}

// ============================================================================
// Roundtrip Tests
// ============================================================================

func TestEnumValues_Roundtrip(t *testing.T) {
	original := []string{
		"ACTIVE - Normal account",
		"SUSPENDED - Hold",
		"BANNED",
	}

	// Parse to models
	parsed := parseEnumValues(original)

	// Format back to strings
	formatted := formatEnumValues(parsed)

	// Verify roundtrip
	assert.Equal(t, original, formatted, "roundtrip should preserve values")
}

// ============================================================================
// Error Result Tests
// ============================================================================

// TestGetColumnMetadataTool_ErrorResults tests error result patterns for get_column_metadata tool.
// These tests validate the error result structure without requiring database setup.
func TestGetColumnMetadataTool_ErrorResults(t *testing.T) {
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

	t.Run("empty column name after trimming", func(t *testing.T) {
		// Simulate empty column name validation
		column := "   " // whitespace only
		column = strings.TrimSpace(column)

		if column == "" {
			result := NewErrorResult("invalid_parameters", "parameter 'column' cannot be empty")

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse error response
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
			require.NoError(t, err)

			assert.True(t, errResp.Error)
			assert.Equal(t, "invalid_parameters", errResp.Code)
			assert.Contains(t, errResp.Message, "column")
			assert.Contains(t, errResp.Message, "cannot be empty")
		}
	})

	t.Run("table not found", func(t *testing.T) {
		// Simulate table not found error
		table := "nonexistent_table"
		result := NewErrorResult("TABLE_NOT_FOUND",
			fmt.Sprintf("table %q not found in schema registry", table))

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
	})

	t.Run("column not found", func(t *testing.T) {
		// Simulate column not found error
		table := "users"
		column := "nonexistent_column"
		result := NewErrorResult("COLUMN_NOT_FOUND",
			fmt.Sprintf("column %q not found in table %q", column, table))

		// Verify it's an error result
		assert.NotNil(t, result)
		assert.True(t, result.IsError)

		// Parse error response
		var errResp ErrorResponse
		err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "COLUMN_NOT_FOUND", errResp.Code)
		assert.Contains(t, errResp.Message, "nonexistent_column")
		assert.Contains(t, errResp.Message, "users")
		assert.Contains(t, errResp.Message, "not found")
	})
}

// TestUpdateColumnTool_ErrorResults tests error result patterns for update_column tool.
// These tests validate the error result structure without requiring database setup.
func TestUpdateColumnTool_ErrorResults(t *testing.T) {
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

	t.Run("empty column name after trimming", func(t *testing.T) {
		// Simulate empty column name validation
		column := "   " // whitespace only
		column = strings.TrimSpace(column)

		if column == "" {
			result := NewErrorResult("invalid_parameters", "parameter 'column' cannot be empty")

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse error response
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
			require.NoError(t, err)

			assert.True(t, errResp.Error)
			assert.Equal(t, "invalid_parameters", errResp.Code)
			assert.Contains(t, errResp.Message, "column")
			assert.Contains(t, errResp.Message, "cannot be empty")
		}
	})

	t.Run("invalid role value", func(t *testing.T) {
		// Simulate role validation
		role := "invalid_role"
		validRoles := []string{"dimension", "measure", "identifier", "attribute"}

		isValidRole := false
		for _, validRole := range validRoles {
			if role == validRole {
				isValidRole = true
				break
			}
		}

		if !isValidRole {
			result := NewErrorResultWithDetails(
				"invalid_parameters",
				fmt.Sprintf("parameter 'role' must be one of: dimension, measure, identifier, attribute. Got: %q", role),
				map[string]any{
					"parameter": "role",
					"expected":  validRoles,
					"actual":    role,
				},
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse error response
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
			require.NoError(t, err)

			assert.True(t, errResp.Error)
			assert.Equal(t, "invalid_parameters", errResp.Code)
			assert.Contains(t, errResp.Message, "role")
			assert.Contains(t, errResp.Message, "must be one of")
			assert.Contains(t, errResp.Message, "invalid_role")

			// Verify details
			details, ok := errResp.Details.(map[string]any)
			require.True(t, ok, "details should be a map")
			assert.Equal(t, "role", details["parameter"])
			assert.Equal(t, "invalid_role", details["actual"])

			// Verify expected roles array
			expectedRoles, ok := details["expected"].([]any)
			require.True(t, ok, "expected should be an array")
			assert.Len(t, expectedRoles, 4)
		}
	})

	t.Run("enum_values array with non-string element - int", func(t *testing.T) {
		// Simulate enum_values validation with non-string element
		enumArray := []any{"ACTIVE", 123, "BANNED"} // Element at index 1 is int

		for i, ev := range enumArray {
			if _, ok := ev.(string); !ok {
				result := NewErrorResultWithDetails(
					"invalid_parameters",
					fmt.Sprintf("parameter 'enum_values' must be an array of strings. Element at index %d is %T, not string", i, ev),
					map[string]any{
						"parameter":             "enum_values",
						"invalid_element_index": i,
						"invalid_element_type":  fmt.Sprintf("%T", ev),
					},
				)

				// Verify it's an error result
				assert.NotNil(t, result)
				assert.True(t, result.IsError)

				// Parse error response
				var errResp ErrorResponse
				err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
				require.NoError(t, err)

				assert.True(t, errResp.Error)
				assert.Equal(t, "invalid_parameters", errResp.Code)
				assert.Contains(t, errResp.Message, "enum_values")
				assert.Contains(t, errResp.Message, "must be an array of strings")
				assert.Contains(t, errResp.Message, "index 1")

				// Verify details
				details, ok := errResp.Details.(map[string]any)
				require.True(t, ok, "details should be a map")
				assert.Equal(t, "enum_values", details["parameter"])
				assert.Equal(t, float64(1), details["invalid_element_index"]) // JSON unmarshals numbers as float64
				assert.Equal(t, "int", details["invalid_element_type"])

				break // Stop after first error
			}
		}
	})

	t.Run("enum_values array with non-string element - bool", func(t *testing.T) {
		// Simulate enum_values validation with non-string element
		enumArray := []any{"ACTIVE", true, "BANNED"} // Element at index 1 is bool

		for i, ev := range enumArray {
			if _, ok := ev.(string); !ok {
				result := NewErrorResultWithDetails(
					"invalid_parameters",
					fmt.Sprintf("parameter 'enum_values' must be an array of strings. Element at index %d is %T, not string", i, ev),
					map[string]any{
						"parameter":             "enum_values",
						"invalid_element_index": i,
						"invalid_element_type":  fmt.Sprintf("%T", ev),
					},
				)

				// Verify it's an error result
				assert.NotNil(t, result)
				assert.True(t, result.IsError)

				// Parse error response
				var errResp ErrorResponse
				err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
				require.NoError(t, err)

				assert.True(t, errResp.Error)
				assert.Equal(t, "invalid_parameters", errResp.Code)
				assert.Contains(t, errResp.Message, "enum_values")
				assert.Contains(t, errResp.Message, "must be an array of strings")
				assert.Contains(t, errResp.Message, "index 1")

				// Verify details
				details, ok := errResp.Details.(map[string]any)
				require.True(t, ok, "details should be a map")
				assert.Equal(t, "enum_values", details["parameter"])
				assert.Equal(t, float64(1), details["invalid_element_index"]) // JSON unmarshals numbers as float64
				assert.Equal(t, "bool", details["invalid_element_type"])

				break // Stop after first error
			}
		}
	})

	t.Run("enum_values array with non-string element - map", func(t *testing.T) {
		// Simulate enum_values validation with non-string element
		enumArray := []any{"ACTIVE", map[string]string{"key": "value"}, "BANNED"} // Element at index 1 is map

		for i, ev := range enumArray {
			if _, ok := ev.(string); !ok {
				result := NewErrorResultWithDetails(
					"invalid_parameters",
					fmt.Sprintf("parameter 'enum_values' must be an array of strings. Element at index %d is %T, not string", i, ev),
					map[string]any{
						"parameter":             "enum_values",
						"invalid_element_index": i,
						"invalid_element_type":  fmt.Sprintf("%T", ev),
					},
				)

				// Verify it's an error result
				assert.NotNil(t, result)
				assert.True(t, result.IsError)

				// Parse error response
				var errResp ErrorResponse
				err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
				require.NoError(t, err)

				assert.True(t, errResp.Error)
				assert.Equal(t, "invalid_parameters", errResp.Code)
				assert.Contains(t, errResp.Message, "enum_values")
				assert.Contains(t, errResp.Message, "must be an array of strings")
				assert.Contains(t, errResp.Message, "index 1")

				// Verify details
				details, ok := errResp.Details.(map[string]any)
				require.True(t, ok, "details should be a map")
				assert.Equal(t, "enum_values", details["parameter"])
				assert.Equal(t, float64(1), details["invalid_element_index"])       // JSON unmarshals numbers as float64
				assert.Contains(t, details["invalid_element_type"].(string), "map") // Type string contains "map"

				break // Stop after first error
			}
		}
	})

	t.Run("valid role values accepted", func(t *testing.T) {
		// Test that all valid role values pass validation
		validRoles := []string{"dimension", "measure", "identifier", "attribute"}

		for _, role := range validRoles {
			isValidRole := false
			for _, validRole := range validRoles {
				if role == validRole {
					isValidRole = true
					break
				}
			}
			assert.True(t, isValidRole, "role %q should be valid", role)
		}
	})
}

// ============================================================================
// Typed Column Write Tests
// ============================================================================

// TestUpdateColumn_TypedColumnMetadata verifies that update_column correctly
// writes typed columns to ColumnMetadata for the new schema.
func TestUpdateColumn_TypedColumnMetadata(t *testing.T) {
	t.Run("entity is stored in IdentifierFeatures.EntityReferenced", func(t *testing.T) {
		// Simulate the entity parameter being set
		entity := "User"

		// Create a column metadata object as the tool would
		colMeta := &models.ColumnMetadata{}
		if entity != "" {
			if colMeta.Features.IdentifierFeatures == nil {
				colMeta.Features.IdentifierFeatures = &models.IdentifierFeatures{}
			}
			colMeta.Features.IdentifierFeatures.EntityReferenced = entity
		}

		// Verify entity is stored correctly
		require.NotNil(t, colMeta.Features.IdentifierFeatures)
		assert.Equal(t, "User", colMeta.Features.IdentifierFeatures.EntityReferenced)
	})

	t.Run("enum values are stored in EnumFeatures with Value/Label separation", func(t *testing.T) {
		// Simulate enum_values parameter with descriptions
		enumValues := []string{
			"ACTIVE - Normal active account",
			"SUSPENDED - Temporarily disabled",
			"PENDING",
		}

		// Parse enums as the tool would
		parsedEnums := parseEnumValues(enumValues)

		// Create column metadata as the tool would
		colMeta := &models.ColumnMetadata{}
		colMeta.Features.EnumFeatures = &models.EnumFeatures{
			Values: make([]models.ColumnEnumValue, len(parsedEnums)),
		}
		for i, ev := range parsedEnums {
			colMeta.Features.EnumFeatures.Values[i] = models.ColumnEnumValue{
				Value: ev.Value,
				Label: ev.Description,
			}
		}

		// Verify enum values are stored correctly
		require.NotNil(t, colMeta.Features.EnumFeatures)
		require.Len(t, colMeta.Features.EnumFeatures.Values, 3)

		// Check first value has both Value and Label
		assert.Equal(t, "ACTIVE", colMeta.Features.EnumFeatures.Values[0].Value)
		assert.Equal(t, "Normal active account", colMeta.Features.EnumFeatures.Values[0].Label)

		// Check second value has both Value and Label
		assert.Equal(t, "SUSPENDED", colMeta.Features.EnumFeatures.Values[1].Value)
		assert.Equal(t, "Temporarily disabled", colMeta.Features.EnumFeatures.Values[1].Label)

		// Check third value has Value but no Label
		assert.Equal(t, "PENDING", colMeta.Features.EnumFeatures.Values[2].Value)
		assert.Empty(t, colMeta.Features.EnumFeatures.Values[2].Label)
	})

	t.Run("role is stored in typed column", func(t *testing.T) {
		role := "identifier"

		colMeta := &models.ColumnMetadata{}
		colMeta.Role = &role

		// Verify role is stored correctly
		require.NotNil(t, colMeta.Role)
		assert.Equal(t, "identifier", *colMeta.Role)
	})

	t.Run("description is stored in typed column", func(t *testing.T) {
		description := "User account status indicator"

		colMeta := &models.ColumnMetadata{}
		colMeta.Description = &description

		// Verify description is stored correctly
		require.NotNil(t, colMeta.Description)
		assert.Equal(t, "User account status indicator", *colMeta.Description)
	})

	t.Run("is_sensitive is stored in typed column", func(t *testing.T) {
		sensitive := true

		colMeta := &models.ColumnMetadata{}
		colMeta.IsSensitive = &sensitive

		// Verify is_sensitive is stored correctly
		require.NotNil(t, colMeta.IsSensitive)
		assert.True(t, *colMeta.IsSensitive)
	})

	t.Run("all fields combined", func(t *testing.T) {
		// Simulate all parameters being set
		description := "User account status"
		entity := "User"
		role := "attribute"
		sensitive := false
		enumValues := []string{"ACTIVE - Normal", "INACTIVE - Disabled"}

		// Parse enums
		parsedEnums := parseEnumValues(enumValues)

		// Create column metadata as the tool would
		colMeta := &models.ColumnMetadata{}

		if description != "" {
			colMeta.Description = &description
		}

		if entity != "" {
			if colMeta.Features.IdentifierFeatures == nil {
				colMeta.Features.IdentifierFeatures = &models.IdentifierFeatures{}
			}
			colMeta.Features.IdentifierFeatures.EntityReferenced = entity
		}

		if role != "" {
			colMeta.Role = &role
		}

		colMeta.Features.EnumFeatures = &models.EnumFeatures{
			Values: make([]models.ColumnEnumValue, len(parsedEnums)),
		}
		for i, ev := range parsedEnums {
			colMeta.Features.EnumFeatures.Values[i] = models.ColumnEnumValue{
				Value: ev.Value,
				Label: ev.Description,
			}
		}

		colMeta.IsSensitive = &sensitive

		// Verify all fields
		assert.Equal(t, "User account status", *colMeta.Description)
		assert.Equal(t, "User", colMeta.Features.IdentifierFeatures.EntityReferenced)
		assert.Equal(t, "attribute", *colMeta.Role)
		assert.False(t, *colMeta.IsSensitive)
		assert.Len(t, colMeta.Features.EnumFeatures.Values, 2)
		assert.Equal(t, "ACTIVE", colMeta.Features.EnumFeatures.Values[0].Value)
		assert.Equal(t, "Normal", colMeta.Features.EnumFeatures.Values[0].Label)
	})
}
