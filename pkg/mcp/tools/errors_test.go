package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getTextContent extracts the text string from the first text content item
func getTextContent(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	// The Content slice contains mcp.Content interface types
	// We need to marshal and unmarshal to extract the text
	jsonBytes, _ := json.Marshal(result.Content[0])
	var textContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	json.Unmarshal(jsonBytes, &textContent)
	return textContent.Text
}

func TestNewErrorResult(t *testing.T) {
	result := NewErrorResult("test_error", "this is a test error")

	require.NotNil(t, result)
	require.Len(t, result.Content, 1)

	// Extract and parse the JSON content
	text := getTextContent(result)
	var errResp ErrorResponse
	err := json.Unmarshal([]byte(text), &errResp)
	require.NoError(t, err)

	// Verify the error response structure
	assert.True(t, errResp.Error, "error field should be true")
	assert.Equal(t, "test_error", errResp.Code)
	assert.Equal(t, "this is a test error", errResp.Message)
	assert.Nil(t, errResp.Details, "details should be nil when not provided")
}

func TestNewErrorResultWithDetails(t *testing.T) {
	details := map[string]any{
		"invalid_columns": []string{"foo", "bar"},
		"valid_columns":   []string{"id", "name", "status"},
		"count":           2,
	}

	result := NewErrorResultWithDetails("validation_error", "invalid columns provided", details)

	require.NotNil(t, result)
	require.Len(t, result.Content, 1)

	// Extract and parse the JSON content
	text := getTextContent(result)
	var errResp ErrorResponse
	err := json.Unmarshal([]byte(text), &errResp)
	require.NoError(t, err)

	// Verify the error response structure
	assert.True(t, errResp.Error, "error field should be true")
	assert.Equal(t, "validation_error", errResp.Code)
	assert.Equal(t, "invalid columns provided", errResp.Message)
	assert.NotNil(t, errResp.Details, "details should not be nil")

	// Verify the details content
	detailsMap, ok := errResp.Details.(map[string]any)
	require.True(t, ok, "details should be a map")
	assert.Contains(t, detailsMap, "invalid_columns")
	assert.Contains(t, detailsMap, "valid_columns")
	assert.Contains(t, detailsMap, "count")
	assert.Equal(t, float64(2), detailsMap["count"]) // JSON numbers are float64
}

func TestErrorResponse_JSONStructure(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		message  string
		details  any
		wantJSON string
	}{
		{
			name:     "simple error without details",
			code:     "not_found",
			message:  "resource not found",
			details:  nil,
			wantJSON: `{"error":true,"code":"not_found","message":"resource not found"}`,
		},
		{
			name:     "error with string details",
			code:     "invalid_input",
			message:  "bad request",
			details:  "parameter 'depth' is required",
			wantJSON: `{"error":true,"code":"invalid_input","message":"bad request","details":"parameter 'depth' is required"}`,
		},
		{
			name:    "error with structured details",
			code:    "validation_error",
			message: "validation failed",
			details: map[string]any{
				"field": "email",
				"issue": "invalid format",
			},
			wantJSON: `{"error":true,"code":"validation_error","message":"validation failed","details":{"field":"email","issue":"invalid format"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result *mcp.CallToolResult
			if tt.details == nil {
				result = NewErrorResult(tt.code, tt.message)
			} else {
				result = NewErrorResultWithDetails(tt.code, tt.message, tt.details)
			}

			text := getTextContent(result)

			// Verify JSON can be unmarshaled
			var got, want map[string]any
			require.NoError(t, json.Unmarshal([]byte(text), &got))
			require.NoError(t, json.Unmarshal([]byte(tt.wantJSON), &want))

			// Compare structures
			assert.Equal(t, want, got)
		})
	}
}

func TestErrorResponse_RealWorldExamples(t *testing.T) {
	t.Run("ontology_not_found", func(t *testing.T) {
		result := NewErrorResult("ontology_not_found", "no active ontology found for project")

		text := getTextContent(result)
		var errResp ErrorResponse
		err := json.Unmarshal([]byte(text), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "ontology_not_found", errResp.Code)
		assert.Contains(t, errResp.Message, "no active ontology")
	})

	t.Run("entity_not_found", func(t *testing.T) {
		result := NewErrorResultWithDetails(
			"entity_not_found",
			"entity 'InvalidEntity' does not exist",
			map[string]any{
				"requested_entity":   "InvalidEntity",
				"available_entities": []string{"User", "Account", "Order"},
			},
		)

		text := getTextContent(result)
		var errResp ErrorResponse
		err := json.Unmarshal([]byte(text), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "entity_not_found", errResp.Code)
		assert.NotNil(t, errResp.Details)
	})

	t.Run("invalid_parameters", func(t *testing.T) {
		result := NewErrorResultWithDetails(
			"invalid_parameters",
			"table names required for columns depth",
			map[string]any{
				"parameter": "tables",
				"depth":     "columns",
				"hint":      "specify tables parameter when using depth='columns'",
			},
		)

		text := getTextContent(result)
		var errResp ErrorResponse
		err := json.Unmarshal([]byte(text), &errResp)
		require.NoError(t, err)

		assert.True(t, errResp.Error)
		assert.Equal(t, "invalid_parameters", errResp.Code)

		detailsMap := errResp.Details.(map[string]any)
		assert.Equal(t, "tables", detailsMap["parameter"])
		assert.Equal(t, "columns", detailsMap["depth"])
	})
}
