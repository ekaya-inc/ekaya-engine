package tools

import (
	"testing"

	"github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func testEnabledBaseDeps() BaseMCPToolDeps {
	return BaseMCPToolDeps{
		MCPConfigService: &mockMCPConfigService{
			config: &models.ToolGroupConfig{Enabled: true},
		},
		Logger: zap.NewNop(),
	}
}

func assertToolDestructiveHint(t *testing.T, mcpServer *server.MCPServer, toolName string, want bool) {
	t.Helper()

	tool := mcpServer.GetTool(toolName)
	require.NotNil(t, tool, "tool should be registered")
	require.NotNil(t, tool.Tool.Annotations.DestructiveHint, "tool should declare destructive hint")
	assert.Equal(t, want, *tool.Tool.Annotations.DestructiveHint)
}

func TestCustomerDatasourceWriteToolsAreDestructive(t *testing.T) {
	t.Run("execute", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerExecuteTool(mcpServer, &MCPToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})

		assertToolDestructiveHint(t, mcpServer, "execute", true)
	})

	t.Run("execute_approved_query", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerExecuteApprovedQueryTool(mcpServer, &QueryToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
			ProjectService:  &mockProjectService{},
			QueryService:    &mockQueryService{},
		})

		assertToolDestructiveHint(t, mcpServer, "execute_approved_query", true)
	})
}

func TestMetadataDeleteToolsAreNotDestructive(t *testing.T) {
	t.Run("delete_approved_query", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerDeleteApprovedQueryTool(mcpServer, &DevQueryToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
			ProjectService:  &mockProjectService{},
			QueryService:    &mockQueryService{},
		})

		assertToolDestructiveHint(t, mcpServer, "delete_approved_query", false)
	})

	t.Run("delete_glossary_term", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerDeleteGlossaryTermTool(mcpServer, &GlossaryToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
			GlossaryService: &mockGlossaryService{},
		})

		assertToolDestructiveHint(t, mcpServer, "delete_glossary_term", false)
	})

	t.Run("delete_column_metadata", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerDeleteColumnMetadataTool(mcpServer, &ColumnToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})

		assertToolDestructiveHint(t, mcpServer, "delete_column_metadata", false)
	})

	t.Run("delete_table_metadata", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerDeleteTableMetadataTool(mcpServer, &TableToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})

		assertToolDestructiveHint(t, mcpServer, "delete_table_metadata", false)
	})

	t.Run("delete_project_knowledge", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerDeleteProjectKnowledgeTool(mcpServer, &KnowledgeToolDeps{
			BaseMCPToolDeps:     testEnabledBaseDeps(),
			KnowledgeRepository: &mockKnowledgeRepository{},
		})

		assertToolDestructiveHint(t, mcpServer, "delete_project_knowledge", false)
	})
}
