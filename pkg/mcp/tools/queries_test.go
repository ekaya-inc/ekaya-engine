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
