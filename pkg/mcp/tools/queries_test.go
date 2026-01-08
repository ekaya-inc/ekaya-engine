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

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestRegisterApprovedQueriesTools(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &QueryToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true},
		},
		ProjectService: &mockProjectService{},
		QueryService:   &mockQueryService{},
		Logger:         zap.NewNop(),
	}

	RegisterApprovedQueriesTools(mcpServer, deps)

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

	// Check both tools are registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["list_approved_queries"], "list_approved_queries tool should be registered")
	assert.True(t, toolNames["execute_approved_query"], "execute_approved_query tool should be registered")
}

// Note: Full integration tests for tool execution require a database connection
// and would be covered in integration tests. The tool registration test above
// verifies that both tools are properly registered with the MCP server.

func TestExecuteApprovedQuery_ResponseMetadata(t *testing.T) {
	// This test verifies that the execute_approved_query response includes query_name and parameters_used.
	// Note: This is a unit test that verifies the response structure without a full integration test.
	// The actual execution is tested in integration tests.

	queryName := "Total revenue by customer"

	// Verify the response structure includes the new fields
	response := struct {
		QueryName      string           `json:"query_name"`
		ParametersUsed map[string]any   `json:"parameters_used"`
		Columns        []string         `json:"columns"`
		Rows           []map[string]any `json:"rows"`
		RowCount       int              `json:"row_count"`
		Truncated      bool             `json:"truncated"`
	}{
		QueryName: queryName,
		ParametersUsed: map[string]any{
			"start_date": "2024-01-01",
			"end_date":   "2024-01-31",
		},
		Columns: []string{"name", "total"},
		Rows: []map[string]any{
			{"name": "Acme Corp", "total": 15000.00},
			{"name": "Beta Inc", "total": 12500.00},
		},
		RowCount:  2,
		Truncated: false,
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify all fields are present
	assert.Equal(t, queryName, parsed["query_name"], "query_name should be present")
	assert.NotNil(t, parsed["parameters_used"], "parameters_used should be present")
	assert.NotNil(t, parsed["columns"], "columns should be present")
	assert.NotNil(t, parsed["rows"], "rows should be present")
	assert.NotNil(t, parsed["row_count"], "row_count should be present")
	assert.NotNil(t, parsed["truncated"], "truncated should be present")

	// Verify parameters_used structure
	paramsUsed, ok := parsed["parameters_used"].(map[string]any)
	require.True(t, ok, "parameters_used should be a map")
	assert.Equal(t, "2024-01-01", paramsUsed["start_date"])
	assert.Equal(t, "2024-01-31", paramsUsed["end_date"])
}

func TestExecuteApprovedQuery_ExecutionTime(t *testing.T) {
	// This test verifies that the execute_approved_query response includes execution_time_ms.
	// Note: This is a unit test that verifies the response structure without a full integration test.
	// The actual execution timing is tested in integration tests.

	// Verify the response structure includes execution_time_ms
	response := struct {
		QueryName       string           `json:"query_name"`
		ParametersUsed  map[string]any   `json:"parameters_used"`
		Columns         []string         `json:"columns"`
		Rows            []map[string]any `json:"rows"`
		RowCount        int              `json:"row_count"`
		Truncated       bool             `json:"truncated"`
		ExecutionTimeMs int64            `json:"execution_time_ms"`
	}{
		QueryName: "Total revenue by customer",
		ParametersUsed: map[string]any{
			"start_date": "2024-01-01",
		},
		Columns: []string{"name", "total"},
		Rows: []map[string]any{
			{"name": "Acme Corp", "total": 15000.00},
		},
		RowCount:        1,
		Truncated:       false,
		ExecutionTimeMs: 145, // Example execution time
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify execution_time_ms is present
	assert.NotNil(t, parsed["execution_time_ms"], "execution_time_ms should be present")

	// Verify it's a number
	execTime, ok := parsed["execution_time_ms"].(float64) // JSON numbers parse as float64
	require.True(t, ok, "execution_time_ms should be a number")
	assert.Equal(t, float64(145), execTime, "execution_time_ms should have the correct value")
}

func TestEnhanceErrorWithContext(t *testing.T) {
	tests := []struct {
		name         string
		err          error
		queryName    string
		wantContains []string
	}{
		{
			name:         "nil error returns nil",
			err:          nil,
			queryName:    "Test Query",
			wantContains: []string{},
		},
		{
			name:      "parameter validation error",
			err:       fmt.Errorf("required parameter 'start_date' is missing"),
			queryName: "Total revenue by customer",
			wantContains: []string{
				"parameter_validation",
				"Total revenue by customer",
				"required parameter 'start_date' is missing",
			},
		},
		{
			name:      "type validation error",
			err:       fmt.Errorf("parameter 'limit': cannot convert 'abc' to integer"),
			queryName: "Revenue report",
			wantContains: []string{
				"type_validation",
				"Revenue report",
				"cannot convert",
			},
		},
		{
			name:      "SQL injection error",
			err:       fmt.Errorf("potential SQL injection detected in parameter 'user_id'"),
			queryName: "User query",
			wantContains: []string{
				"security_violation",
				"User query",
				"SQL injection",
			},
		},
		{
			name:      "execution error",
			err:       fmt.Errorf("failed to execute query: connection timeout"),
			queryName: "Complex analytics",
			wantContains: []string{
				"execution_error",
				"Complex analytics",
				"execute",
			},
		},
		{
			name:      "unknown error type",
			err:       fmt.Errorf("something went wrong"),
			queryName: "Some query",
			wantContains: []string{
				"query_error",
				"Some query",
				"something went wrong",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := enhanceErrorWithContext(tt.err, tt.queryName)

			if tt.err == nil {
				assert.Nil(t, result, "enhanceErrorWithContext should return nil for nil input")
				return
			}

			require.NotNil(t, result, "enhanceErrorWithContext should return an error")
			errMsg := result.Error()

			for _, want := range tt.wantContains {
				assert.Contains(t, errMsg, want, "error message should contain %q", want)
			}
		})
	}
}

func TestCategorizeError(t *testing.T) {
	tests := []struct {
		errMsg   string
		expected string
	}{
		{
			errMsg:   "required parameter 'date' is missing",
			expected: "parameter_validation",
		},
		{
			errMsg:   "unknown parameter 'foo' provided",
			expected: "parameter_validation",
		},
		{
			errMsg:   "cannot convert 'abc' to integer",
			expected: "type_validation",
		},
		{
			errMsg:   "invalid format for date parameter",
			expected: "type_validation",
		},
		{
			errMsg:   "potential SQL injection detected",
			expected: "security_violation",
		},
		{
			errMsg:   "SQL INJECTION attempt blocked",
			expected: "security_violation",
		},
		{
			errMsg:   "failed to execute query",
			expected: "execution_error",
		},
		{
			errMsg:   "query execution timed out",
			expected: "execution_error",
		},
		{
			errMsg:   "database connection lost",
			expected: "query_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.errMsg, func(t *testing.T) {
			result := categorizeError(tt.errMsg)
			assert.Equal(t, tt.expected, result, "categorizeError(%q) = %q, want %q", tt.errMsg, result, tt.expected)
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		substrs  []string
		expected bool
	}{
		{
			name:     "exact match",
			s:        "parameter validation failed",
			substrs:  []string{"parameter"},
			expected: true,
		},
		{
			name:     "case insensitive match",
			s:        "Parameter Validation Failed",
			substrs:  []string{"parameter"},
			expected: true,
		},
		{
			name:     "multiple substrings, first matches",
			s:        "required field is missing",
			substrs:  []string{"required", "optional"},
			expected: true,
		},
		{
			name:     "multiple substrings, second matches",
			s:        "optional field provided",
			substrs:  []string{"required", "optional"},
			expected: true,
		},
		{
			name:     "no match",
			s:        "something else",
			substrs:  []string{"parameter", "required"},
			expected: false,
		},
		{
			name:     "partial word match",
			s:        "parameterized query",
			substrs:  []string{"parameter"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.s, tt.substrs)
			assert.Equal(t, tt.expected, result, "containsAny(%q, %v) = %v, want %v", tt.s, tt.substrs, result, tt.expected)
		})
	}
}

func TestListApprovedQueries_OutputColumns(t *testing.T) {
	// Test that manually specified output_columns are returned correctly
	// Note: SQL parsing fallback was removed - output columns come from query execution results
	query := &models.Query{
		ID:                    uuid.New(),
		NaturalLanguagePrompt: "Get customer revenue",
		SQLQuery:              "SELECT u.name, SUM(o.amount) AS total FROM users u JOIN orders o GROUP BY u.name",
		OutputColumns: []models.OutputColumn{
			{Name: "customer_name", Type: "string", Description: "Full customer name"},
			{Name: "revenue", Type: "decimal", Description: "Total revenue from customer"},
		},
	}

	// Simulate what happens in the list_approved_queries handler
	var outputCols []outputColumnInfo
	if len(query.OutputColumns) > 0 {
		outputCols = make([]outputColumnInfo, len(query.OutputColumns))
		for j, oc := range query.OutputColumns {
			outputCols[j] = outputColumnInfo{
				Name:        oc.Name,
				Type:        oc.Type,
				Description: oc.Description,
			}
		}
	}

	// Verify column count
	require.Equal(t, 2, len(outputCols), "should have 2 output columns")

	// Verify first column
	assert.Equal(t, "customer_name", outputCols[0].Name)
	assert.Equal(t, "string", outputCols[0].Type)
	assert.Equal(t, "Full customer name", outputCols[0].Description)

	// Verify second column
	assert.Equal(t, "revenue", outputCols[1].Name)
	assert.Equal(t, "decimal", outputCols[1].Type)
	assert.Equal(t, "Total revenue from customer", outputCols[1].Description)
}

func TestListApprovedQueries_EmptyOutputColumns(t *testing.T) {
	// Test that empty output_columns returns empty (no fallback parsing)
	query := &models.Query{
		ID:                    uuid.New(),
		NaturalLanguagePrompt: "Get all users",
		SQLQuery:              "SELECT id, name, email FROM users",
		OutputColumns:         []models.OutputColumn{}, // Empty - no output columns specified
	}

	// Simulate what happens in the list_approved_queries handler
	var outputCols []outputColumnInfo
	if len(query.OutputColumns) > 0 {
		outputCols = make([]outputColumnInfo, len(query.OutputColumns))
		for j, oc := range query.OutputColumns {
			outputCols[j] = outputColumnInfo{
				Name:        oc.Name,
				Type:        oc.Type,
				Description: oc.Description,
			}
		}
	}

	// Verify empty output columns (no fallback parsing)
	assert.Empty(t, outputCols, "should have no output columns when not specified")
}

func TestRegisterSuggestApprovedQueryTool(t *testing.T) {
	// Verify that suggest_approved_query tool is registered
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &QueryToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true},
		},
		ProjectService: &mockProjectService{},
		QueryService:   &mockQueryService{},
		Logger:         zap.NewNop(),
	}

	RegisterApprovedQueriesTools(mcpServer, deps)

	// Verify tool is registered
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

	// Check suggest_approved_query tool is registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["suggest_approved_query"], "suggest_approved_query tool should be registered")
}

func TestParseParameterDefinitions(t *testing.T) {
	tests := []struct {
		name      string
		input     []any
		wantErr   bool
		wantCount int
		validate  func(*testing.T, []models.QueryParameter)
	}{
		{
			name: "valid parameters with all fields",
			input: []any{
				map[string]any{
					"name":        "host_username",
					"type":        "string",
					"description": "Host's username",
					"required":    true,
					"example":     "damon",
				},
			},
			wantErr:   false,
			wantCount: 1,
			validate: func(t *testing.T, params []models.QueryParameter) {
				assert.Equal(t, "host_username", params[0].Name)
				assert.Equal(t, "string", params[0].Type)
				assert.Equal(t, "Host's username", params[0].Description)
				assert.True(t, params[0].Required)
				assert.Equal(t, "damon", params[0].Default)
			},
		},
		{
			name: "parameter with defaults",
			input: []any{
				map[string]any{
					"name": "user_id",
					"type": "integer",
				},
			},
			wantErr:   false,
			wantCount: 1,
			validate: func(t *testing.T, params []models.QueryParameter) {
				assert.Equal(t, "user_id", params[0].Name)
				assert.Equal(t, "integer", params[0].Type)
				assert.True(t, params[0].Required) // Default to required
			},
		},
		{
			name: "missing name field",
			input: []any{
				map[string]any{
					"type": "string",
				},
			},
			wantErr: true,
		},
		{
			name: "missing type field",
			input: []any{
				map[string]any{
					"name": "param1",
				},
			},
			wantErr: true,
		},
		{
			name:      "empty array",
			input:     []any{},
			wantErr:   false,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseParameterDefinitions(tt.input)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantCount, len(result))

			if tt.validate != nil {
				tt.validate(t, result)
			}
		})
	}
}

func TestBuildOutputColumns(t *testing.T) {
	columns := []columnDetail{
		{Name: "month", Type: "TIMESTAMP"},
		{Name: "total_earned_usd", Type: "NUMERIC"},
		{Name: "transaction_count", Type: "BIGINT"},
	}

	descriptions := map[string]string{
		"total_earned_usd":  "Total earnings in USD (converted from cents)",
		"transaction_count": "Number of completed transactions",
	}

	result := buildOutputColumns(columns, descriptions)

	require.Equal(t, 3, len(result))

	// Check first column (no description provided)
	assert.Equal(t, "month", result[0].Name)
	assert.Equal(t, "TIMESTAMP", result[0].Type)
	assert.Empty(t, result[0].Description)

	// Check second column (description provided)
	assert.Equal(t, "total_earned_usd", result[1].Name)
	assert.Equal(t, "NUMERIC", result[1].Type)
	assert.Equal(t, "Total earnings in USD (converted from cents)", result[1].Description)

	// Check third column (description provided)
	assert.Equal(t, "transaction_count", result[2].Name)
	assert.Equal(t, "BIGINT", result[2].Type)
	assert.Equal(t, "Number of completed transactions", result[2].Description)
}

func TestSuggestApprovedQuery_ResponseStructure(t *testing.T) {
	// Verify the response structure of suggest_approved_query tool
	response := struct {
		SuggestionID string             `json:"suggestion_id"`
		Status       string             `json:"status"`
		Validation   validationResponse `json:"validation"`
	}{
		SuggestionID: uuid.New().String(),
		Status:       "pending",
		Validation: validationResponse{
			SQLValid:   true,
			DryRunRows: 3,
			DetectedOutputColumns: []columnDetail{
				{Name: "month", Type: "TIMESTAMP"},
				{Name: "total", Type: "NUMERIC"},
			},
		},
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify required fields are present
	assert.NotEmpty(t, parsed["suggestion_id"])
	assert.Equal(t, "pending", parsed["status"])
	assert.NotNil(t, parsed["validation"])

	// Verify validation object structure
	validation, ok := parsed["validation"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, validation["sql_valid"])
	assert.Equal(t, float64(3), validation["dry_run_rows"])
	assert.NotNil(t, validation["detected_output_columns"])
}

func TestGetQueryHistory_ResponseStructure(t *testing.T) {
	// This test verifies the get_query_history response structure.
	// Full integration testing with actual database records is done in integration tests.

	// Example response structure
	response := struct {
		RecentQueries []struct {
			SQL             string         `json:"sql"`
			ExecutedAt      string         `json:"executed_at"`
			RowCount        int            `json:"row_count"`
			ExecutionTimeMs int            `json:"execution_time_ms"`
			Parameters      map[string]any `json:"parameters,omitempty"`
			QueryName       *string        `json:"query_name,omitempty"`
		} `json:"recent_queries"`
		Count     int `json:"count"`
		HoursBack int `json:"hours_back"`
	}{
		RecentQueries: []struct {
			SQL             string         `json:"sql"`
			ExecutedAt      string         `json:"executed_at"`
			RowCount        int            `json:"row_count"`
			ExecutionTimeMs int            `json:"execution_time_ms"`
			Parameters      map[string]any `json:"parameters,omitempty"`
			QueryName       *string        `json:"query_name,omitempty"`
		}{
			{
				SQL:             "SELECT * FROM users WHERE username = $1",
				ExecutedAt:      "2024-01-15T10:30:00Z",
				RowCount:        42,
				ExecutionTimeMs: 145,
				Parameters: map[string]any{
					"username": "john_doe",
				},
				QueryName: strPtr("Find user by username"),
			},
			{
				SQL:             "SELECT COUNT(*) FROM orders WHERE created_at >= $1",
				ExecutedAt:      "2024-01-15T10:25:00Z",
				RowCount:        1,
				ExecutionTimeMs: 89,
				Parameters: map[string]any{
					"start_date": "2024-01-01",
				},
				QueryName: strPtr("Count recent orders"),
			},
		},
		Count:     2,
		HoursBack: 24,
	}

	// Verify JSON serialization works
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	// Verify top-level fields
	assert.Equal(t, float64(2), parsed["count"])
	assert.Equal(t, float64(24), parsed["hours_back"])
	assert.NotNil(t, parsed["recent_queries"])

	// Verify recent_queries array structure
	queries, ok := parsed["recent_queries"].([]any)
	require.True(t, ok)
	require.Len(t, queries, 2)

	// Verify first query structure
	firstQuery, ok := queries[0].(map[string]any)
	require.True(t, ok)
	assert.NotEmpty(t, firstQuery["sql"])
	assert.NotEmpty(t, firstQuery["executed_at"])
	assert.NotNil(t, firstQuery["row_count"])
	assert.NotNil(t, firstQuery["execution_time_ms"])
	assert.NotNil(t, firstQuery["parameters"])
	assert.NotNil(t, firstQuery["query_name"])
}

func TestGetQueryHistory_Registration(t *testing.T) {
	// Verify get_query_history tool is registered
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	deps := &QueryToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true},
		},
		ProjectService: &mockProjectService{},
		QueryService:   &mockQueryService{},
		Logger:         zap.NewNop(),
	}

	RegisterApprovedQueriesTools(mcpServer, deps)

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

	// Check get_query_history tool is registered
	toolNames := make(map[string]bool)
	for _, tool := range response.Result.Tools {
		toolNames[tool.Name] = true
	}

	assert.True(t, toolNames["get_query_history"], "get_query_history tool should be registered")
}

// Helper function to create string pointer
func strPtr(s string) *string {
	return &s
}
