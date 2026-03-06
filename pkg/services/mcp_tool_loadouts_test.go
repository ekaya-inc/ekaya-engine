package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestComputeUserTools(t *testing.T) {
	t.Run("returns only health with empty state", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{}

		tools := ComputeUserTools(state)
		toolNames := toolNamesToMap(tools)

		// Only health with no toggles enabled
		assert.True(t, toolNames["health"], "health should be included")

		// Should NOT get any other tools without toggles
		assert.False(t, toolNames["query"], "query should NOT be included")
		assert.False(t, toolNames["sample"], "sample should NOT be included")
		assert.False(t, toolNames["echo"], "echo should NOT be included")
		assert.False(t, toolNames["execute"], "execute should NOT be included")
	})

	t.Run("includes user tools when toggles enabled", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {
				AddOntologySuggestions: true,
				AddRequestTools:        true,
			},
		}

		tools := ComputeUserTools(state)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"])
		// Ontology Suggestions tools
		assert.True(t, toolNames["get_context"])
		assert.True(t, toolNames["get_ontology"])
		assert.True(t, toolNames["list_glossary"])
		assert.True(t, toolNames["get_glossary_sql"])
		// Request tools
		assert.True(t, toolNames["query"])
		assert.True(t, toolNames["sample"])
		assert.True(t, toolNames["list_approved_queries"])
		assert.True(t, toolNames["execute_approved_query"])

		// Should NOT get developer tools
		assert.False(t, toolNames["echo"])
		assert.False(t, toolNames["execute"])
	})

	t.Run("backward compat with legacy user key", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupUser: {AllowOntologyMaintenance: true},
		}

		tools := ComputeUserTools(state)
		toolNames := toolNamesToMap(tools)

		// Legacy AllowOntologyMaintenance maps to AddOntologySuggestions + AddRequestTools
		assert.True(t, toolNames["health"])
		assert.True(t, toolNames["get_context"])
		assert.True(t, toolNames["query"])
		assert.True(t, toolNames["list_approved_queries"])
	})

	t.Run("nil state returns only health", func(t *testing.T) {
		tools := ComputeUserTools(nil)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"], "health should be included")
		assert.Len(t, tools, 1, "nil state should only include health")
	})
}

func TestComputeDeveloperTools(t *testing.T) {
	t.Run("returns only health with empty state", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		// Only health with no toggles enabled
		assert.True(t, toolNames["health"], "health should be included")

		// No tools without toggles
		assert.False(t, toolNames["echo"], "echo should NOT be included")
		assert.False(t, toolNames["execute"], "execute should NOT be included")
		assert.False(t, toolNames["query"], "query should NOT be included")
	})

	t.Run("includes Direct Database Access tools when toggle enabled", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddDirectDatabaseAccess: true},
		}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["echo"], "echo should be included")
		assert.True(t, toolNames["execute"], "execute should be included")
		assert.True(t, toolNames["query"], "query should be included")

		// Ontology Maintenance NOT included
		assert.False(t, toolNames["update_table"], "update_table should NOT be included")
	})

	t.Run("includes Ontology Maintenance tools when toggle enabled", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddOntologyMaintenanceTools: true},
		}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["get_schema"], "get_schema should be included")
		assert.True(t, toolNames["update_table"], "update_table should be included")
		assert.True(t, toolNames["update_column"], "update_column should be included")
		assert.True(t, toolNames["list_ontology_questions"], "list_ontology_questions should be included")
		assert.True(t, toolNames["resolve_ontology_question"], "resolve_ontology_question should be included")

		// Direct Database Access NOT included
		assert.False(t, toolNames["echo"], "echo should NOT be included")
		assert.False(t, toolNames["execute"], "execute should NOT be included")
	})

	t.Run("includes Approval tools when toggle enabled", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddApprovalTools: true},
		}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["list_query_suggestions"], "list_query_suggestions should be included")
		assert.True(t, toolNames["approve_query_suggestion"], "approve_query_suggestion should be included")
		assert.True(t, toolNames["create_approved_query"], "create_approved_query should be included")
	})

	t.Run("includes all tools when all toggles enabled", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {
				AddDirectDatabaseAccess:     true,
				AddOntologyMaintenanceTools: true,
				AddApprovalTools:            true,
			},
		}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["echo"], "echo should be included")
		assert.True(t, toolNames["execute"], "execute should be included")
		assert.True(t, toolNames["query"], "query should be included")
		assert.True(t, toolNames["get_schema"], "get_schema should be included")
		assert.True(t, toolNames["update_table"], "update_table should be included")
		assert.True(t, toolNames["list_ontology_questions"], "list_ontology_questions should be included")
		assert.True(t, toolNames["list_query_suggestions"], "list_query_suggestions should be included")
	})

	t.Run("backward compat with legacy developer key", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {AddQueryTools: true, AddOntologyMaintenance: true},
		}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		// Legacy AddQueryTools maps to AddDirectDatabaseAccess + AddApprovalTools
		assert.True(t, toolNames["health"])
		assert.True(t, toolNames["echo"])
		assert.True(t, toolNames["execute"])
		assert.True(t, toolNames["query"])
		assert.True(t, toolNames["list_query_suggestions"])

		// Legacy AddOntologyMaintenance maps to AddOntologyMaintenanceTools
		assert.True(t, toolNames["update_table"])
		assert.True(t, toolNames["list_ontology_questions"])
	})

	t.Run("nil state returns only health", func(t *testing.T) {
		tools := ComputeDeveloperTools(nil)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"], "health should be included")
		assert.Len(t, tools, 1, "nil state should only include health")
	})
}

func TestComputeAgentTools(t *testing.T) {
	t.Run("returns limited set of tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {
				AddDirectDatabaseAccess:     true,
				AddOntologyMaintenanceTools: true,
				AddApprovalTools:            true,
			},
		}

		tools := ComputeAgentTools(state)
		toolNames := toolNamesToMap(tools)

		// Only Default + Limited Query
		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["list_approved_queries"], "list_approved_queries should be included")
		assert.True(t, toolNames["execute_approved_query"], "execute_approved_query should be included")

		// No developer tools
		assert.False(t, toolNames["echo"], "echo should NOT be included")
		assert.False(t, toolNames["execute"], "execute should NOT be included")
		assert.False(t, toolNames["query"], "query should NOT be included")
	})

	t.Run("ignores all configuration options", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {
				AddDirectDatabaseAccess:     true,
				AddOntologyMaintenanceTools: true,
				AddOntologySuggestions:      true,
				AddApprovalTools:            true,
				AddRequestTools:             true,
			},
			ToolGroupAgentTools: {Enabled: true},
		}

		tools := ComputeAgentTools(state)
		assert.Len(t, tools, 3, "agent should get exactly 3 tools")

		toolNames := toolNamesToMap(tools)
		assert.True(t, toolNames["health"])
		assert.True(t, toolNames["list_approved_queries"])
		assert.True(t, toolNames["execute_approved_query"])
	})

	t.Run("nil state returns same limited set", func(t *testing.T) {
		tools := ComputeAgentTools(nil)

		assert.Len(t, tools, 3, "agent should get exactly 3 tools")

		toolNames := toolNamesToMap(tools)
		assert.True(t, toolNames["health"])
		assert.True(t, toolNames["list_approved_queries"])
		assert.True(t, toolNames["execute_approved_query"])
	})
}

// toolNamesToMap converts a slice of ToolSpec to a map for easy lookup.
func toolNamesToMap(tools []ToolSpec) map[string]bool {
	result := make(map[string]bool)
	for _, tool := range tools {
		result[tool.Name] = true
	}
	return result
}
