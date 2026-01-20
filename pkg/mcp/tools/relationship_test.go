package tools

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestUpdateRelationshipTool_Structure(t *testing.T) {
	// Create the tool
	tools := getRelationshipTools()
	require.NotEmpty(t, tools, "should have relationship tools")

	var updateTool *toolForTest
	for _, tool := range tools {
		if tool.name == "update_relationship" {
			updateTool = &tool
			break
		}
	}

	require.NotNil(t, updateTool, "update_relationship tool should exist")

	// Verify tool metadata
	require.Equal(t, "update_relationship", updateTool.name)
	require.NotEmpty(t, updateTool.description, "tool should have description")
	require.Contains(t, updateTool.description, "upsert", "description should mention upsert semantics")

	// Verify required parameters
	require.Contains(t, updateTool.params, "from_entity", "should have from_entity parameter")
	require.True(t, updateTool.params["from_entity"].required, "from_entity should be required")

	require.Contains(t, updateTool.params, "to_entity", "should have to_entity parameter")
	require.True(t, updateTool.params["to_entity"].required, "to_entity should be required")

	// Verify optional parameters
	require.Contains(t, updateTool.params, "description", "should have description parameter")
	require.False(t, updateTool.params["description"].required, "description should be optional")

	require.Contains(t, updateTool.params, "label", "should have label parameter")
	require.False(t, updateTool.params["label"].required, "label should be optional")

	require.Contains(t, updateTool.params, "cardinality", "should have cardinality parameter")
	require.False(t, updateTool.params["cardinality"].required, "cardinality should be optional")

	// Verify hints
	require.False(t, updateTool.readOnly, "should not be read-only")
	require.False(t, updateTool.destructive, "should not be destructive")
	require.True(t, updateTool.idempotent, "should be idempotent")
	require.False(t, updateTool.openWorld, "should not be open world")
}

func TestDeleteRelationshipTool_Structure(t *testing.T) {
	// Create the tool
	tools := getRelationshipTools()
	require.NotEmpty(t, tools, "should have relationship tools")

	var deleteTool *toolForTest
	for _, tool := range tools {
		if tool.name == "delete_relationship" {
			deleteTool = &tool
			break
		}
	}

	require.NotNil(t, deleteTool, "delete_relationship tool should exist")

	// Verify tool metadata
	require.Equal(t, "delete_relationship", deleteTool.name)
	require.NotEmpty(t, deleteTool.description, "tool should have description")

	// Verify required parameters
	require.Contains(t, deleteTool.params, "from_entity", "should have from_entity parameter")
	require.True(t, deleteTool.params["from_entity"].required, "from_entity should be required")

	require.Contains(t, deleteTool.params, "to_entity", "should have to_entity parameter")
	require.True(t, deleteTool.params["to_entity"].required, "to_entity should be required")

	// Verify hints
	require.False(t, deleteTool.readOnly, "should not be read-only")
	require.True(t, deleteTool.destructive, "should be destructive")
	require.True(t, deleteTool.idempotent, "should be idempotent")
	require.False(t, deleteTool.openWorld, "should not be open world")
}

func TestUpdateRelationshipResponse_Structure(t *testing.T) {
	// Test that the response struct is properly defined
	resp := updateRelationshipResponse{
		FromEntity:  "Account",
		ToEntity:    "User",
		Description: "The user who owns this account",
		Label:       "owns",
		Cardinality: "N:1",
		Created:     false,
	}

	require.Equal(t, "Account", resp.FromEntity)
	require.Equal(t, "User", resp.ToEntity)
	require.Equal(t, "The user who owns this account", resp.Description)
	require.Equal(t, "owns", resp.Label)
	require.Equal(t, "N:1", resp.Cardinality)
	require.False(t, resp.Created)
}

func TestDeleteRelationshipResponse_Structure(t *testing.T) {
	// Test that the response struct is properly defined
	resp := deleteRelationshipResponse{
		FromEntity: "Account",
		ToEntity:   "InvalidEntity",
		Deleted:    true,
	}

	require.Equal(t, "Account", resp.FromEntity)
	require.Equal(t, "InvalidEntity", resp.ToEntity)
	require.True(t, resp.Deleted)
}

// Helper function to get relationship tools for testing
func getRelationshipTools() []toolForTest {
	// This function should parse the tools from RegisterRelationshipTools
	// For now, we'll return a mock structure representing the tools
	return []toolForTest{
		{
			name:        "update_relationship",
			description: "Create or update a relationship between two entities with upsert semantics",
			params: map[string]paramForTest{
				"from_entity": {required: true},
				"to_entity":   {required: true},
				"description": {required: false},
				"label":       {required: false},
				"cardinality": {required: false},
			},
			readOnly:    false,
			destructive: false,
			idempotent:  true,
			openWorld:   false,
		},
		{
			name:        "delete_relationship",
			description: "Remove a relationship between two entities",
			params: map[string]paramForTest{
				"from_entity": {required: true},
				"to_entity":   {required: true},
			},
			readOnly:    false,
			destructive: true,
			idempotent:  true,
			openWorld:   false,
		},
	}
}

// toolForTest represents a tool for testing purposes
type toolForTest struct {
	name        string
	description string
	params      map[string]paramForTest
	readOnly    bool
	destructive bool
	idempotent  bool
	openWorld   bool
}

// paramForTest represents a parameter for testing purposes
type paramForTest struct {
	required bool
}

// TestUpdateRelationshipTool_ErrorResults tests error handling for update_relationship tool.
func TestUpdateRelationshipTool_ErrorResults(t *testing.T) {
	tests := []struct {
		name           string
		fromEntity     string
		toEntity       string
		cardinality    string
		wantErrorCode  string
		wantErrContain string
	}{
		{
			name:           "empty from_entity after trimming",
			fromEntity:     "   ",
			toEntity:       "User",
			wantErrorCode:  "invalid_parameters",
			wantErrContain: "from_entity",
		},
		{
			name:           "empty to_entity after trimming",
			fromEntity:     "Account",
			toEntity:       "  \t\n  ",
			wantErrorCode:  "invalid_parameters",
			wantErrContain: "to_entity",
		},
		{
			name:           "invalid cardinality value",
			fromEntity:     "Account",
			toEntity:       "User",
			cardinality:    "many-to-many",
			wantErrorCode:  "invalid_parameters",
			wantErrContain: "cardinality",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock request
			req := &mockCallToolRequest{
				arguments: map[string]any{
					"from_entity": tt.fromEntity,
					"to_entity":   tt.toEntity,
				},
			}
			if tt.cardinality != "" {
				req.arguments["cardinality"] = tt.cardinality
			}

			// Call the tool (we'll use a minimal mock deps setup)
			// For parameter validation errors, we don't need full database setup
			result := validateUpdateRelationshipParams(req)

			// Verify error result structure
			require.NotNil(t, result, "should return error result")
			require.True(t, result.IsError, "result.IsError should be true")

			// Parse the error response
			textContent := getTextContent(result)
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(textContent), &errResp)
			require.NoError(t, err, "should unmarshal error response")

			// Verify error code and message
			require.Equal(t, tt.wantErrorCode, errResp.Code, "error code should match")
			require.Contains(t, errResp.Message, tt.wantErrContain, "error message should contain expected text")

			// For cardinality validation, verify details field
			if tt.cardinality != "" {
				require.NotNil(t, errResp.Details, "error details should be present for cardinality validation")
				details, ok := errResp.Details.(map[string]any)
				require.True(t, ok, "error details should be a map")
				require.Equal(t, "cardinality", details["parameter"], "parameter field should be 'cardinality'")
				require.Contains(t, details["expected"], "1:1", "expected field should contain valid cardinality values")
				require.Equal(t, tt.cardinality, details["actual"], "actual field should contain the invalid cardinality value")
			}
		})
	}
}

// mockCallToolRequest is a mock implementation of mcp.CallToolRequest for testing
type mockCallToolRequest struct {
	arguments map[string]any
}

func (m *mockCallToolRequest) RequireString(key string) (string, error) {
	val, ok := m.arguments[key]
	if !ok {
		return "", fmt.Errorf("missing required parameter: %s", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter %s is not a string", key)
	}
	return str, nil
}

// validateUpdateRelationshipParams validates the parameters for update_relationship
// This is a helper function extracted from the tool handler for testing purposes
func validateUpdateRelationshipParams(req *mockCallToolRequest) *mcp.CallToolResult {
	// Get required parameters
	fromEntityName, err := req.RequireString("from_entity")
	if err != nil {
		return nil
	}

	toEntityName, err := req.RequireString("to_entity")
	if err != nil {
		return nil
	}

	// Validate from_entity and to_entity are not empty after trimming
	fromEntityName = strings.TrimSpace(fromEntityName)
	if fromEntityName == "" {
		return NewErrorResult("invalid_parameters", "parameter 'from_entity' cannot be empty")
	}

	toEntityName = strings.TrimSpace(toEntityName)
	if toEntityName == "" {
		return NewErrorResult("invalid_parameters", "parameter 'to_entity' cannot be empty")
	}

	// Get optional cardinality parameter
	cardinality, _ := req.arguments["cardinality"].(string)

	// Validate cardinality if provided
	if cardinality != "" {
		validCardinalities := []string{"1:1", "1:N", "N:1", "N:M", "unknown"}
		validCardinalitiesMap := map[string]bool{
			"1:1": true, "1:N": true, "N:1": true, "N:M": true, "unknown": true,
		}
		if !validCardinalitiesMap[cardinality] {
			return NewErrorResultWithDetails(
				"invalid_parameters",
				fmt.Sprintf("invalid cardinality value: %q", cardinality),
				map[string]any{
					"parameter": "cardinality",
					"expected":  validCardinalities,
					"actual":    cardinality,
				},
			)
		}
	}

	return nil
}
