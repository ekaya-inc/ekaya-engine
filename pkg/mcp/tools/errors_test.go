package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
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

// Tests for SQL error detection and conversion

func TestIsSQLUserError(t *testing.T) {
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
			name:     "generic error",
			err:      errors.New("something went wrong"),
			expected: false,
		},
		{
			name:     "connection error",
			err:      errors.New("connection refused"),
			expected: false,
		},
		{
			name:     "syntax error with SQLSTATE",
			err:      errors.New("ERROR: syntax error at or near \"SELEKT\" (SQLSTATE 42601)"),
			expected: true,
		},
		{
			name:     "undefined table with SQLSTATE",
			err:      errors.New("ERROR: relation \"nonexistent_table\" does not exist (SQLSTATE 42P01)"),
			expected: true,
		},
		{
			name:     "unique violation with SQLSTATE",
			err:      errors.New("ERROR: duplicate key value violates unique constraint (SQLSTATE 23505)"),
			expected: true,
		},
		{
			name:     "undefined column with SQLSTATE",
			err:      errors.New("ERROR: column \"foo\" does not exist (SQLSTATE 42703)"),
			expected: true,
		},
		{
			name:     "foreign key violation with SQLSTATE",
			err:      errors.New("ERROR: insert or update violates foreign key constraint (SQLSTATE 23503)"),
			expected: true,
		},
		{
			name:     "not null violation with SQLSTATE",
			err:      errors.New("ERROR: null value in column \"name\" violates not-null constraint (SQLSTATE 23502)"),
			expected: true,
		},
		{
			name:     "wrapped error with SQLSTATE",
			err:      fmt.Errorf("execution failed: %w", errors.New("ERROR: syntax error (SQLSTATE 42601)")),
			expected: true,
		},
		{
			name:     "data exception - division by zero",
			err:      errors.New("ERROR: division by zero (SQLSTATE 22012)"),
			expected: true,
		},
		{
			name:     "timeout error (not a SQL user error)",
			err:      errors.New("context deadline exceeded"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsSQLUserError(tt.err)
			assert.Equal(t, tt.expected, result, "IsSQLUserError() returned unexpected result")
		})
	}
}

func TestIsSQLUserError_PgError(t *testing.T) {
	// Test with actual pgconn.PgError struct
	tests := []struct {
		name     string
		code     string
		expected bool
	}{
		{"syntax error", "42601", true},
		{"undefined table", "42P01", true},
		{"undefined column", "42703", true},
		{"unique violation", "23505", true},
		{"foreign key violation", "23503", true},
		{"not null violation", "23502", true},
		{"check violation", "23514", true},
		{"division by zero", "22012", true},
		{"invalid datetime", "22007", true},
		{"successful completion (not an error)", "00000", false},
		{"connection exception", "08000", false},
		{"transaction rollback", "40000", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgErr := &pgconn.PgError{
				Code:    tt.code,
				Message: "test error message",
			}
			result := IsSQLUserError(pgErr)
			assert.Equal(t, tt.expected, result, "IsSQLUserError() returned unexpected result for SQLSTATE %s", tt.code)
		})
	}
}

func TestSQLUserErrorCode(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectedCode string
	}{
		{
			name:         "nil error",
			err:          nil,
			expectedCode: "",
		},
		{
			name:         "generic error (no SQLSTATE)",
			err:          errors.New("something went wrong"),
			expectedCode: "",
		},
		{
			name:         "syntax error",
			err:          errors.New("ERROR: syntax error (SQLSTATE 42601)"),
			expectedCode: "syntax_error",
		},
		{
			name:         "undefined table",
			err:          errors.New("ERROR: table not found (SQLSTATE 42P01)"),
			expectedCode: "undefined_table",
		},
		{
			name:         "undefined column",
			err:          errors.New("ERROR: column not found (SQLSTATE 42703)"),
			expectedCode: "undefined_column",
		},
		{
			name:         "unique violation",
			err:          errors.New("ERROR: duplicate key (SQLSTATE 23505)"),
			expectedCode: "unique_violation",
		},
		{
			name:         "foreign key violation",
			err:          errors.New("ERROR: FK violation (SQLSTATE 23503)"),
			expectedCode: "foreign_key_violation",
		},
		{
			name:         "not null violation",
			err:          errors.New("ERROR: null not allowed (SQLSTATE 23502)"),
			expectedCode: "not_null_violation",
		},
		{
			name:         "check violation",
			err:          errors.New("ERROR: check constraint failed (SQLSTATE 23514)"),
			expectedCode: "check_violation",
		},
		{
			name:         "division by zero",
			err:          errors.New("ERROR: division by zero (SQLSTATE 22012)"),
			expectedCode: "division_by_zero",
		},
		{
			name:         "invalid datetime",
			err:          errors.New("ERROR: invalid date format (SQLSTATE 22007)"),
			expectedCode: "invalid_datetime",
		},
		{
			name:         "generic SQL error (42xxx)",
			err:          errors.New("ERROR: some SQL error (SQLSTATE 42000)"),
			expectedCode: "sql_error",
		},
		{
			name:         "generic constraint error (23xxx)",
			err:          errors.New("ERROR: some constraint error (SQLSTATE 23000)"),
			expectedCode: "constraint_violation",
		},
		{
			name:         "generic data exception (22xxx)",
			err:          errors.New("ERROR: some data error (SQLSTATE 22000)"),
			expectedCode: "data_exception",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SQLUserErrorCode(tt.err)
			assert.Equal(t, tt.expectedCode, result, "SQLUserErrorCode() returned unexpected code")
		})
	}
}

func TestExtractSQLErrorMessage(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "simple error",
			err:      errors.New("something went wrong"),
			expected: "something went wrong",
		},
		{
			name:     "error with SQLSTATE suffix",
			err:      errors.New("syntax error at or near \"SELEKT\" (SQLSTATE 42601)"),
			expected: "syntax error at or near \"SELEKT\"",
		},
		{
			name:     "error with ERROR prefix",
			err:      errors.New("ERROR: relation \"users\" does not exist"),
			expected: "relation \"users\" does not exist",
		},
		{
			name:     "error with both prefix and suffix",
			err:      errors.New("ERROR: column \"foo\" does not exist (SQLSTATE 42703)"),
			expected: "column \"foo\" does not exist",
		},
		{
			name:     "wrapped error with multiple prefixes",
			err:      errors.New("execution failed: failed to execute statement: ERROR: syntax error (SQLSTATE 42601)"),
			expected: "syntax error",
		},
		{
			name:     "query execution failed prefix",
			err:      errors.New("query execution failed: ERROR: invalid input (SQLSTATE 22P02)"),
			expected: "invalid input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractSQLErrorMessage(tt.err)
			assert.Equal(t, tt.expected, result, "ExtractSQLErrorMessage() returned unexpected message")
		})
	}
}

func TestExtractSQLErrorMessage_PgError(t *testing.T) {
	// Test with actual pgconn.PgError struct - should return the Message field directly
	pgErr := &pgconn.PgError{
		Code:    "42601",
		Message: "syntax error at or near \"SELEKT\"",
	}
	result := ExtractSQLErrorMessage(pgErr)
	assert.Equal(t, "syntax error at or near \"SELEKT\"", result)
}

func TestNewSQLErrorResult(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		expectResult bool
		expectedCode string
	}{
		{
			name:         "nil error",
			err:          nil,
			expectResult: false,
		},
		{
			name:         "generic error (not SQL user error)",
			err:          errors.New("connection refused"),
			expectResult: false,
		},
		{
			name:         "syntax error",
			err:          errors.New("ERROR: syntax error at or near \"SELEKT\" (SQLSTATE 42601)"),
			expectResult: true,
			expectedCode: "syntax_error",
		},
		{
			name:         "undefined table",
			err:          errors.New("ERROR: relation \"nonexistent\" does not exist (SQLSTATE 42P01)"),
			expectResult: true,
			expectedCode: "undefined_table",
		},
		{
			name:         "unique violation",
			err:          errors.New("ERROR: duplicate key value violates unique constraint (SQLSTATE 23505)"),
			expectResult: true,
			expectedCode: "unique_violation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NewSQLErrorResult(tt.err)

			if !tt.expectResult {
				assert.Nil(t, result, "NewSQLErrorResult() should return nil for non-SQL user errors")
				return
			}

			require.NotNil(t, result, "NewSQLErrorResult() should return a result for SQL user errors")
			assert.True(t, result.IsError, "result.IsError should be true")

			// Parse the JSON content
			text := getTextContent(result)
			var errResp ErrorResponse
			err := json.Unmarshal([]byte(text), &errResp)
			require.NoError(t, err)

			assert.True(t, errResp.Error, "error field should be true")
			assert.Equal(t, tt.expectedCode, errResp.Code, "error code should match expected")
			assert.NotEmpty(t, errResp.Message, "error message should not be empty")
		})
	}
}

func TestNewSQLErrorResult_RealWorldExamples(t *testing.T) {
	// Test with error messages from the actual issue file

	t.Run("syntax error SELEKT", func(t *testing.T) {
		err := errors.New(`failed to execute statement: ERROR: syntax error at or near "SELEKT" (SQLSTATE 42601)`)
		result := NewSQLErrorResult(err)

		require.NotNil(t, result)
		text := getTextContent(result)
		var errResp ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(text), &errResp))

		assert.True(t, errResp.Error)
		assert.Equal(t, "syntax_error", errResp.Code)
		assert.Contains(t, errResp.Message, "SELEKT")
	})

	t.Run("table not found", func(t *testing.T) {
		err := errors.New(`failed to execute statement: ERROR: relation "nonexistent_table_xyz" does not exist (SQLSTATE 42P01)`)
		result := NewSQLErrorResult(err)

		require.NotNil(t, result)
		text := getTextContent(result)
		var errResp ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(text), &errResp))

		assert.True(t, errResp.Error)
		assert.Equal(t, "undefined_table", errResp.Code)
		assert.Contains(t, errResp.Message, "nonexistent_table_xyz")
	})

	t.Run("unique constraint violation", func(t *testing.T) {
		err := errors.New(`error during execution: ERROR: duplicate key value violates unique constraint "mcp_test_users_email_key" (SQLSTATE 23505)`)
		result := NewSQLErrorResult(err)

		require.NotNil(t, result)
		text := getTextContent(result)
		var errResp ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(text), &errResp))

		assert.True(t, errResp.Error)
		assert.Equal(t, "unique_violation", errResp.Code)
		assert.Contains(t, errResp.Message, "duplicate key")
	})

	t.Run("multi-statement rejection", func(t *testing.T) {
		err := errors.New(`failed to execute statement: ERROR: cannot insert multiple commands into a prepared statement (SQLSTATE 42601)`)
		result := NewSQLErrorResult(err)

		require.NotNil(t, result)
		text := getTextContent(result)
		var errResp ErrorResponse
		require.NoError(t, json.Unmarshal([]byte(text), &errResp))

		assert.True(t, errResp.Error)
		assert.Equal(t, "syntax_error", errResp.Code)
		assert.Contains(t, errResp.Message, "multiple commands")
	})
}
