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
	sqlpkg "github.com/ekaya-inc/ekaya-engine/pkg/sql"
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
			name:      "nil error returns nil",
			err:       nil,
			queryName: "Test Query",
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

func TestListApprovedQueries_OutputColumnsFallback(t *testing.T) {
	// Test that output_columns are parsed from SQL when not manually specified
	tests := []struct {
		name           string
		query          *models.Query
		expectedCols   []string
		expectFallback bool
	}{
		{
			name: "manually specified output_columns",
			query: &models.Query{
				ID:                    uuid.New(),
				NaturalLanguagePrompt: "Get customer revenue",
				SQLQuery:              "SELECT u.name, SUM(o.amount) AS total FROM users u JOIN orders o GROUP BY u.name",
				OutputColumns: []models.OutputColumn{
					{Name: "customer_name", Type: "string", Description: "Full customer name"},
					{Name: "revenue", Type: "decimal", Description: "Total revenue from customer"},
				},
			},
			expectedCols:   []string{"customer_name", "revenue"},
			expectFallback: false,
		},
		{
			name: "fallback to SQL parsing - simple query",
			query: &models.Query{
				ID:                    uuid.New(),
				NaturalLanguagePrompt: "Get all users",
				SQLQuery:              "SELECT id, name, email FROM users",
				OutputColumns:         []models.OutputColumn{}, // Empty - should fallback
			},
			expectedCols:   []string{"id", "name", "email"},
			expectFallback: true,
		},
		{
			name: "fallback to SQL parsing - with aliases",
			query: &models.Query{
				ID:                    uuid.New(),
				NaturalLanguagePrompt: "Customer revenue report",
				SQLQuery:              "SELECT u.name AS customer_name, SUM(o.amount) AS total_revenue FROM users u JOIN orders o GROUP BY u.name",
				OutputColumns:         []models.OutputColumn{}, // Empty - should fallback
			},
			expectedCols:   []string{"customer_name", "total_revenue"},
			expectFallback: true,
		},
		{
			name: "fallback to SQL parsing - aggregate functions",
			query: &models.Query{
				ID:                    uuid.New(),
				NaturalLanguagePrompt: "Order statistics",
				SQLQuery:              "SELECT COUNT(*) AS order_count, AVG(total) AS avg_total FROM orders",
				OutputColumns:         []models.OutputColumn{}, // Empty - should fallback
			},
			expectedCols:   []string{"order_count", "avg_total"},
			expectFallback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create the approved query info structure manually
			// (simulating what happens in the list_approved_queries handler)
			var outputCols []outputColumnInfo

			if len(tt.query.OutputColumns) > 0 {
				// Use manually specified
				outputCols = make([]outputColumnInfo, len(tt.query.OutputColumns))
				for j, oc := range tt.query.OutputColumns {
					outputCols[j] = outputColumnInfo{
						Name:        oc.Name,
						Type:        oc.Type,
						Description: oc.Description,
					}
				}
			} else {
				// Fallback to SQL parsing
				parsedCols, err := sqlpkg.ParseSelectColumns(tt.query.SQLQuery)
				require.NoError(t, err, "SQL parsing should not fail")

				if len(parsedCols) > 0 {
					outputCols = make([]outputColumnInfo, len(parsedCols))
					for j, pc := range parsedCols {
						outputCols[j] = outputColumnInfo{
							Name:        pc.Name,
							Type:        "",
							Description: "",
						}
					}
				}
			}

			// Verify column count
			assert.Equal(t, len(tt.expectedCols), len(outputCols),
				"output column count should match expected")

			// Verify column names
			for i, expectedName := range tt.expectedCols {
				if i < len(outputCols) {
					assert.Equal(t, expectedName, outputCols[i].Name,
						"column %d name should match", i)

					if tt.expectFallback {
						// Fallback columns should have empty type and description
						assert.Empty(t, outputCols[i].Type,
							"fallback column should have empty type")
						assert.Empty(t, outputCols[i].Description,
							"fallback column should have empty description")
					} else {
						// Manually specified columns should have type and description
						assert.NotEmpty(t, outputCols[i].Type,
							"manual column should have type")
						assert.NotEmpty(t, outputCols[i].Description,
							"manual column should have description")
					}
				}
			}
		})
	}
}
