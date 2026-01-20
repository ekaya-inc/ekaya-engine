package tools

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetSchemaToolErrorResults tests error result patterns for get_schema tool.
// These tests validate the error result structure without requiring database setup.
func TestGetSchemaToolErrorResults(t *testing.T) {
	t.Run("invalid boolean parameter - selected_only", func(t *testing.T) {
		// Simulate what happens when selected_only is not a boolean
		invalidValue := "not_a_boolean"

		// Simulate the parameter validation logic from get_schema handler
		result := NewErrorResultWithDetails(
			"invalid_parameters",
			fmt.Sprintf("parameter 'selected_only' must be a boolean, got %T", invalidValue),
			map[string]any{
				"parameter":     "selected_only",
				"expected_type": "boolean",
				"actual_type":   fmt.Sprintf("%T", invalidValue),
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
		assert.Contains(t, errResp.Message, "selected_only")
		assert.Contains(t, errResp.Message, "must be a boolean")

		// Verify details
		details, ok := errResp.Details.(map[string]any)
		assert.True(t, ok, "details should be a map")
		assert.Equal(t, "selected_only", details["parameter"])
		assert.Equal(t, "boolean", details["expected_type"])
	})

	t.Run("invalid boolean parameter - include_entities as number", func(t *testing.T) {
		// Simulate what happens when include_entities is a number instead of boolean
		invalidValue := 123

		// Simulate the parameter validation logic from get_schema handler
		result := NewErrorResultWithDetails(
			"invalid_parameters",
			fmt.Sprintf("parameter 'include_entities' must be a boolean, got %T", invalidValue),
			map[string]any{
				"parameter":     "include_entities",
				"expected_type": "boolean",
				"actual_type":   fmt.Sprintf("%T", invalidValue),
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
		assert.Contains(t, errResp.Message, "include_entities")
		assert.Contains(t, errResp.Message, "must be a boolean")

		// Verify details
		details, ok := errResp.Details.(map[string]any)
		assert.True(t, ok, "details should be a map")
		assert.Equal(t, "include_entities", details["parameter"])
		assert.Equal(t, "boolean", details["expected_type"])
		assert.Contains(t, details["actual_type"].(string), "int")
	})

	t.Run("invalid boolean parameter - include_entities as array", func(t *testing.T) {
		// Simulate what happens when include_entities is an array
		invalidValue := []string{"true"}

		// Simulate the parameter validation logic from get_schema handler
		result := NewErrorResultWithDetails(
			"invalid_parameters",
			fmt.Sprintf("parameter 'include_entities' must be a boolean, got %T", invalidValue),
			map[string]any{
				"parameter":     "include_entities",
				"expected_type": "boolean",
				"actual_type":   fmt.Sprintf("%T", invalidValue),
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
		assert.Contains(t, errResp.Message, "include_entities")
		assert.Contains(t, errResp.Message, "must be a boolean")
	})

	t.Run("ontology not found when semantic annotations requested", func(t *testing.T) {
		// Simulate the error handling when GetDatasourceSchemaWithEntities returns
		// a "no active ontology" error
		schemaServiceErr := fmt.Errorf("failed to get entities: no active ontology found")

		// Check if this matches our ontology error pattern
		if isOntologyNotFoundError(schemaServiceErr) {
			result := NewErrorResult(
				"ontology_not_found",
				"no active ontology found for project - cannot provide semantic annotations. Use include_entities=false for raw schema, or extract ontology first",
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse error response
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
			require.NoError(t, err)

			assert.True(t, errResp.Error)
			assert.Equal(t, "ontology_not_found", errResp.Code)
			assert.Contains(t, errResp.Message, "no active ontology")
			assert.Contains(t, errResp.Message, "include_entities=false", "should suggest workaround")
		}
	})

	t.Run("ontology not found - alternative error message", func(t *testing.T) {
		// Test with alternative error message format
		schemaServiceErr := fmt.Errorf("ontology not found for project abc-123")

		// Check if this matches our ontology error pattern
		if isOntologyNotFoundError(schemaServiceErr) {
			result := NewErrorResult(
				"ontology_not_found",
				"no active ontology found for project - cannot provide semantic annotations. Use include_entities=false for raw schema, or extract ontology first",
			)

			// Verify it's an error result
			assert.NotNil(t, result)
			assert.True(t, result.IsError)

			// Parse error response
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(getTextContent(result)), &errResp)
			require.NoError(t, err)

			assert.True(t, errResp.Error)
			assert.Equal(t, "ontology_not_found", errResp.Code)
		}
	})

	t.Run("system error remains as Go error", func(t *testing.T) {
		// Simulate a system error that should NOT be converted to error result
		schemaServiceErr := fmt.Errorf("database connection failed: timeout")

		// Verify this is NOT detected as an ontology error
		assert.False(t, isOntologyNotFoundError(schemaServiceErr), "database connection errors should remain as Go errors")
	})
}

// TestIsOntologyNotFoundError tests the helper function.
func TestIsOntologyNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "no active ontology error",
			err:      fmt.Errorf("failed to get entities: no active ontology found"),
			expected: true,
		},
		{
			name:     "ontology not found error",
			err:      fmt.Errorf("ontology not found for project"),
			expected: true,
		},
		{
			name:     "no active ontology - uppercase",
			err:      fmt.Errorf("NO ACTIVE ONTOLOGY FOUND"),
			expected: false, // Contains check is case-sensitive
		},
		{
			name:     "unrelated error - database",
			err:      fmt.Errorf("database connection failed"),
			expected: false,
		},
		{
			name:     "unrelated error - timeout",
			err:      fmt.Errorf("context deadline exceeded"),
			expected: false,
		},
		{
			name:     "partial match - active only",
			err:      fmt.Errorf("user is active"),
			expected: false,
		},
		{
			name:     "partial match - ontology in unrelated context",
			err:      fmt.Errorf("failed to parse ontology file"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isOntologyNotFoundError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestSchemaToolDeps_Structure verifies the SchemaToolDeps struct has all required fields.
func TestSchemaToolDeps_Structure(t *testing.T) {
	// Create a zero-value instance to verify struct is properly defined
	deps := &SchemaToolDeps{}

	// Verify all fields exist and have correct types
	assert.Nil(t, deps.DB, "DB field should be nil by default")
	assert.Nil(t, deps.MCPConfigService, "MCPConfigService field should be nil by default")
	assert.Nil(t, deps.ProjectService, "ProjectService field should be nil by default")
	assert.Nil(t, deps.SchemaService, "SchemaService field should be nil by default")
	assert.Nil(t, deps.Logger, "Logger field should be nil by default")
}
