package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type strictClientToolsListResponse struct {
	Result struct {
		Tools []strictClientTool `json:"tools"`
	} `json:"result"`
}

type strictClientTool struct {
	Name        string         `json:"name"`
	InputSchema map[string]any `json:"inputSchema"`
}

func TestNewTool_ZeroArgInputSchemaIncludesEmptyProperties(t *testing.T) {
	tool := mcp.NewTool("x")

	payload, err := json.Marshal(tool)
	require.NoError(t, err)

	var got struct {
		InputSchema map[string]any `json:"inputSchema"`
	}
	require.NoError(t, json.Unmarshal(payload, &got))

	require.Equal(t, "object", got.InputSchema["type"])

	properties, ok := got.InputSchema["properties"].(map[string]any)
	require.True(t, ok, "zero-arg tool schema must include an explicit properties object")
	assert.Empty(t, properties)
}

func TestRegisteredToolCatalog_StrictClientsRequirePropertiesOnObjectSchemas(t *testing.T) {
	mcpServer := newStrictClientCompatibilityTestServer()

	tools := listToolsForStrictClientSchemaTest(t, mcpServer)
	assertStrictClientCompatibleToolCatalog(t, tools)

	for _, name := range []string{
		"approve_all_changes",
		"get_schema",
		"health",
		"list_glossary",
	} {
		tool := findStrictClientTool(t, tools, name)
		require.Equal(t, "object", tool.InputSchema["type"], "expected %s to use an object input schema", name)

		properties, ok := tool.InputSchema["properties"].(map[string]any)
		require.True(t, ok, "expected %s to include explicit empty properties", name)
		assert.Empty(t, properties, "expected %s to remain a zero-arg tool", name)
	}
}

func newStrictClientCompatibilityTestServer() *server.MCPServer {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	baseDeps := BaseMCPToolDeps{
		Logger: zap.NewNop(),
	}

	RegisterHealthTool(mcpServer, "1.0.0-test", &HealthToolDeps{
		Logger: zap.NewNop(),
	})
	RegisterMCPTools(mcpServer, &MCPToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterApprovedQueriesTools(mcpServer, &QueryToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterDevQueryTools(mcpServer, &DevQueryToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterSchemaTools(mcpServer, &SchemaToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterOntologyTools(mcpServer, &OntologyToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterGlossaryTools(mcpServer, &GlossaryToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterKnowledgeTools(mcpServer, &KnowledgeToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterContextTools(mcpServer, &ContextToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterColumnTools(mcpServer, &ColumnToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterBatchTools(mcpServer, &ColumnToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterTableTools(mcpServer, &TableToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterProbeTools(mcpServer, &ProbeToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterSearchTools(mcpServer, &SearchToolDeps{
		BaseMCPToolDeps: baseDeps,
	})
	RegisterQuestionTools(mcpServer, &QuestionToolDeps{
		BaseMCPToolDeps: baseDeps,
	})

	return mcpServer
}

func listToolsForStrictClientSchemaTest(t *testing.T, mcpServer *server.MCPServer) []strictClientTool {
	t.Helper()

	result := mcpServer.HandleMessage(context.Background(), []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	payload, err := json.Marshal(result)
	require.NoError(t, err)

	var response strictClientToolsListResponse
	require.NoError(t, json.Unmarshal(payload, &response))
	require.NotEmpty(t, response.Result.Tools, "expected tools/list to return registered tools")

	return response.Result.Tools
}

func assertStrictClientCompatibleToolCatalog(t *testing.T, tools []strictClientTool) {
	t.Helper()

	for _, tool := range tools {
		require.NotNil(t, tool.InputSchema, "tool %q must include an input schema", tool.Name)

		if tool.InputSchema["type"] != "object" {
			continue
		}

		if _, ok := tool.InputSchema["properties"]; !ok {
			schemaJSON, err := json.Marshal(tool.InputSchema)
			require.NoError(t, err)
			t.Fatalf("tool %q has strict-client-incompatible object schema: %s", tool.Name, schemaJSON)
		}
	}
}

func findStrictClientTool(t *testing.T, tools []strictClientTool, name string) strictClientTool {
	t.Helper()

	for _, tool := range tools {
		if tool.Name == name {
			return tool
		}
	}

	t.Fatalf("tool %q not found in tools/list response", name)
	return strictClientTool{}
}
