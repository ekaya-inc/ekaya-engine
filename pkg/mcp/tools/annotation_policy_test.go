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

func assertToolReadOnlyHint(t *testing.T, mcpServer *server.MCPServer, toolName string, want bool) {
	t.Helper()

	tool := mcpServer.GetTool(toolName)
	require.NotNil(t, tool, "tool should be registered")
	require.NotNil(t, tool.Tool.Annotations.ReadOnlyHint, "tool should declare read-only hint")
	assert.Equal(t, want, *tool.Tool.Annotations.ReadOnlyHint)
}

func assertToolIdempotentHint(t *testing.T, mcpServer *server.MCPServer, toolName string, want bool) {
	t.Helper()

	tool := mcpServer.GetTool(toolName)
	require.NotNil(t, tool, "tool should be registered")
	require.NotNil(t, tool.Tool.Annotations.IdempotentHint, "tool should declare idempotent hint")
	assert.Equal(t, want, *tool.Tool.Annotations.IdempotentHint)
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
	t.Run("list_project_knowledge", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerListProjectKnowledgeTool(mcpServer, &KnowledgeToolDeps{
			BaseMCPToolDeps:     testEnabledBaseDeps(),
			KnowledgeRepository: &mockKnowledgeRepository{},
		})

		assertToolReadOnlyHint(t, mcpServer, "list_project_knowledge", true)
		assertToolDestructiveHint(t, mcpServer, "list_project_knowledge", false)
	})

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

func TestCallerVisibleIdempotencyHints(t *testing.T) {
	t.Run("query is non-idempotent when history can be recorded", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerQueryTool(mcpServer, &MCPToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})

		assertToolIdempotentHint(t, mcpServer, "query", false)
	})

	t.Run("knowledge upsert is non-idempotent when it creates a new fact_id", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerUpdateProjectKnowledgeTool(mcpServer, &KnowledgeToolDeps{
			BaseMCPToolDeps:     testEnabledBaseDeps(),
			KnowledgeRepository: &mockKnowledgeRepository{},
		})

		assertToolIdempotentHint(t, mcpServer, "update_project_knowledge", false)
	})

	t.Run("project knowledge listing is idempotent", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerListProjectKnowledgeTool(mcpServer, &KnowledgeToolDeps{
			BaseMCPToolDeps:     testEnabledBaseDeps(),
			KnowledgeRepository: &mockKnowledgeRepository{},
		})

		assertToolIdempotentHint(t, mcpServer, "list_project_knowledge", true)
	})

	t.Run("ontology question transitions are non-idempotent", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
		deps := &QuestionToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
			QuestionRepo:    &mockQuestionRepository{},
		}

		registerResolveOntologyQuestionTool(mcpServer, deps)
		registerSkipOntologyQuestionTool(mcpServer, deps)
		registerEscalateOntologyQuestionTool(mcpServer, deps)
		registerDismissOntologyQuestionTool(mcpServer, deps)

		assertToolIdempotentHint(t, mcpServer, "resolve_ontology_question", false)
		assertToolIdempotentHint(t, mcpServer, "skip_ontology_question", false)
		assertToolIdempotentHint(t, mcpServer, "escalate_ontology_question", false)
		assertToolIdempotentHint(t, mcpServer, "dismiss_ontology_question", false)
	})

	t.Run("pending change transitions are non-idempotent except bulk approval", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerApproveChangeTool(mcpServer, &MCPToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})
		registerRejectChangeTool(mcpServer, &MCPToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})
		registerApproveAllChangesTool(mcpServer, &MCPToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})

		assertToolIdempotentHint(t, mcpServer, "approve_change", false)
		assertToolIdempotentHint(t, mcpServer, "reject_change", false)
		assertToolIdempotentHint(t, mcpServer, "approve_all_changes", true)
	})

	t.Run("scan_data_changes is non-idempotent because it can create new pending change ids", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerScanDataChangesTool(mcpServer, &MCPToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})

		assertToolIdempotentHint(t, mcpServer, "scan_data_changes", false)
	})

	t.Run("dev query workflow exposes non-idempotent create and state transitions", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
		deps := &DevQueryToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
			ProjectService:  &mockProjectService{},
			QueryService:    &mockQueryService{},
		}

		registerApproveQuerySuggestionTool(mcpServer, deps)
		registerRejectQuerySuggestionTool(mcpServer, deps)
		registerCreateApprovedQueryTool(mcpServer, deps)
		registerUpdateApprovedQueryTool(mcpServer, deps)
		registerDeleteApprovedQueryTool(mcpServer, deps)

		assertToolIdempotentHint(t, mcpServer, "approve_query_suggestion", false)
		assertToolIdempotentHint(t, mcpServer, "reject_query_suggestion", false)
		assertToolIdempotentHint(t, mcpServer, "create_approved_query", false)
		assertToolIdempotentHint(t, mcpServer, "update_approved_query", true)
		assertToolIdempotentHint(t, mcpServer, "delete_approved_query", false)
	})

	t.Run("metadata upserts and metadata deletes that tolerate misses stay idempotent", func(t *testing.T) {
		mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

		registerUpdateGlossaryTermTool(mcpServer, &GlossaryToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
			GlossaryService: &mockGlossaryService{},
		})
		registerDeleteGlossaryTermTool(mcpServer, &GlossaryToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
			GlossaryService: &mockGlossaryService{},
		})
		registerUpdateColumnTool(mcpServer, &ColumnToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})
		registerDeleteColumnMetadataTool(mcpServer, &ColumnToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})
		registerUpdateTableTool(mcpServer, &TableToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})
		registerDeleteTableMetadataTool(mcpServer, &TableToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})
		registerUpdateColumnsTool(mcpServer, &ColumnToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})
		registerRefreshSchemaTool(mcpServer, &MCPToolDeps{
			BaseMCPToolDeps: testEnabledBaseDeps(),
		})
		registerRecordQueryFeedbackTool(mcpServer, &QueryToolDeps{
			BaseMCPToolDeps:     testEnabledBaseDeps(),
			QueryHistoryService: nil,
		})

		assertToolIdempotentHint(t, mcpServer, "update_glossary_term", true)
		assertToolIdempotentHint(t, mcpServer, "delete_glossary_term", true)
		assertToolIdempotentHint(t, mcpServer, "update_column", true)
		assertToolIdempotentHint(t, mcpServer, "delete_column_metadata", true)
		assertToolIdempotentHint(t, mcpServer, "update_table", true)
		assertToolIdempotentHint(t, mcpServer, "delete_table_metadata", true)
		assertToolIdempotentHint(t, mcpServer, "update_columns", true)
		assertToolIdempotentHint(t, mcpServer, "refresh_schema", true)
		assertToolIdempotentHint(t, mcpServer, "record_query_feedback", true)
	})
}
