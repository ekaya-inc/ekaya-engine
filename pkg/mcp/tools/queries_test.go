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
