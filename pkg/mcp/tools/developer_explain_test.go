package tools

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterExplainQueryTool_Metadata(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	registerExplainQueryTool(mcpServer, &MCPToolDeps{})

	tool := mcpServer.GetTool("explain_query")
	require.NotNil(t, tool, "explain_query should be registered")

	assert.Contains(t, tool.Tool.Description, "without executing")
	assert.Contains(t, tool.Tool.Description, "SELECT")
	assert.Contains(t, tool.Tool.Description, "read-only WITH")
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint)
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint)
	require.NotNil(t, tool.Tool.Annotations.IdempotentHint)
	require.NotNil(t, tool.Tool.Annotations.OpenWorldHint)
	assert.True(t, *tool.Tool.Annotations.ReadOnlyHint)
	assert.False(t, *tool.Tool.Annotations.DestructiveHint)
	assert.True(t, *tool.Tool.Annotations.IdempotentHint)
	assert.False(t, *tool.Tool.Annotations.OpenWorldHint)
}

func TestExplainQueryResponse_Structure(t *testing.T) {
	response := explainQueryResponse{
		StatementType:    "SELECT",
		Plan:             "Seq Scan on users",
		PerformanceHints: []string{"Sequential scan detected"},
	}

	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &parsed))

	assert.Equal(t, "SELECT", parsed["statement_type"])
	assert.Equal(t, "Seq Scan on users", parsed["plan"])
	assert.NotNil(t, parsed["performance_hints"])
	assert.NotContains(t, parsed, "execution_time_ms")
	assert.NotContains(t, parsed, "planning_time_ms")
}
