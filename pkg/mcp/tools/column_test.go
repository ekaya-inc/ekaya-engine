package tools

import (
	"context"
	"encoding/json"
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

	// Verify both tools are registered
	expectedTools := []string{"update_column", "delete_column_metadata"}
	foundTools := make(map[string]bool)

	for _, tool := range response.Result.Tools {
		foundTools[tool.Name] = true
	}

	for _, expected := range expectedTools {
		assert.True(t, foundTools[expected], "tool %s should be registered", expected)
	}
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
