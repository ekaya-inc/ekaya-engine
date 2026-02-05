package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/stretchr/testify/assert"
)

func TestComputeUserTools(t *testing.T) {
	t.Run("returns Query loadout tools by default", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{}

		tools := ComputeUserTools(state)
		toolNames := toolNamesToMap(tools)

		// Default + Query loadout
		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["query"], "query should be included")
		assert.True(t, toolNames["sample"], "sample should be included")
		assert.True(t, toolNames["validate"], "validate should be included")
		assert.True(t, toolNames["get_ontology"], "get_ontology should be included")
		assert.True(t, toolNames["get_schema"], "get_schema should be included")
		assert.True(t, toolNames["list_approved_queries"], "list_approved_queries should be included")
		assert.True(t, toolNames["execute_approved_query"], "execute_approved_query should be included")

		// Developer-only tools NOT included
		assert.False(t, toolNames["echo"], "echo should NOT be included")
		assert.False(t, toolNames["execute"], "execute should NOT be included")

		// Ontology Maintenance NOT included without option
		assert.False(t, toolNames["update_table"], "update_table should NOT be included")
		assert.False(t, toolNames["update_column"], "update_column should NOT be included")
	})

	t.Run("includes Ontology Maintenance tools when AllowOntologyMaintenance is true", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupUser: {AllowOntologyMaintenance: true},
		}

		tools := ComputeUserTools(state)
		toolNames := toolNamesToMap(tools)

		// Query loadout tools
		assert.True(t, toolNames["query"], "query should be included")
		assert.True(t, toolNames["get_schema"], "get_schema should be included")

		// Ontology Maintenance tools
		assert.True(t, toolNames["update_table"], "update_table should be included")
		assert.True(t, toolNames["update_column"], "update_column should be included")
		assert.True(t, toolNames["update_glossary_term"], "update_glossary_term should be included")
		assert.True(t, toolNames["refresh_schema"], "refresh_schema should be included")

		// Developer-only tools still NOT included
		assert.False(t, toolNames["echo"], "echo should NOT be included")
		assert.False(t, toolNames["execute"], "execute should NOT be included")
	})

	t.Run("nil state returns Query loadout", func(t *testing.T) {
		tools := ComputeUserTools(nil)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["query"], "query should be included")
		assert.False(t, toolNames["update_table"], "update_table should NOT be included")
	})
}

func TestComputeDeveloperTools(t *testing.T) {
	t.Run("returns Developer Core tools by default", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		// Default + Developer Core
		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["echo"], "echo should be included")
		assert.True(t, toolNames["execute"], "execute should be included")

		// Query tools NOT included without AddQueryTools
		assert.False(t, toolNames["query"], "query should NOT be included")
		assert.False(t, toolNames["get_schema"], "get_schema should NOT be included")

		// Ontology Maintenance NOT included without AddOntologyMaintenance
		assert.False(t, toolNames["update_table"], "update_table should NOT be included")
	})

	t.Run("includes Query tools when AddQueryTools is true", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {AddQueryTools: true},
		}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		// Developer Core + Query
		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["echo"], "echo should be included")
		assert.True(t, toolNames["execute"], "execute should be included")
		assert.True(t, toolNames["query"], "query should be included")
		assert.True(t, toolNames["get_schema"], "get_schema should be included")
		assert.True(t, toolNames["sample"], "sample should be included")

		// Ontology Maintenance NOT included
		assert.False(t, toolNames["update_table"], "update_table should NOT be included")
	})

	t.Run("includes Ontology Maintenance and Questions when AddOntologyMaintenance is true", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {AddOntologyMaintenance: true},
		}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		// Developer Core + Ontology Maintenance + Ontology Questions
		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["echo"], "echo should be included")
		assert.True(t, toolNames["execute"], "execute should be included")
		assert.True(t, toolNames["update_table"], "update_table should be included")
		assert.True(t, toolNames["update_column"], "update_column should be included")
		assert.True(t, toolNames["list_ontology_questions"], "list_ontology_questions should be included")
		assert.True(t, toolNames["resolve_ontology_question"], "resolve_ontology_question should be included")

		// Query tools NOT included
		assert.False(t, toolNames["query"], "query should NOT be included")
		assert.False(t, toolNames["get_schema"], "get_schema should NOT be included")
	})

	t.Run("includes all tools when both options are true", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {AddQueryTools: true, AddOntologyMaintenance: true},
		}

		tools := ComputeDeveloperTools(state)
		toolNames := toolNamesToMap(tools)

		// Developer Core + Query + Ontology Maintenance + Ontology Questions
		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["echo"], "echo should be included")
		assert.True(t, toolNames["execute"], "execute should be included")
		assert.True(t, toolNames["query"], "query should be included")
		assert.True(t, toolNames["get_schema"], "get_schema should be included")
		assert.True(t, toolNames["update_table"], "update_table should be included")
		assert.True(t, toolNames["list_ontology_questions"], "list_ontology_questions should be included")
	})

	t.Run("nil state returns Developer Core only", func(t *testing.T) {
		tools := ComputeDeveloperTools(nil)
		toolNames := toolNamesToMap(tools)

		assert.True(t, toolNames["health"], "health should be included")
		assert.True(t, toolNames["echo"], "echo should be included")
		assert.True(t, toolNames["execute"], "execute should be included")
		assert.False(t, toolNames["query"], "query should NOT be included")
	})
}

func TestComputeAgentTools(t *testing.T) {
	t.Run("returns limited set of tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper:       {Enabled: true, AddQueryTools: true},
			ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: true},
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

		// No query tools beyond approved queries
		assert.False(t, toolNames["query"], "query should NOT be included")
		assert.False(t, toolNames["get_schema"], "get_schema should NOT be included")

		// No ontology maintenance tools
		assert.False(t, toolNames["update_table"], "update_table should NOT be included")
	})

	t.Run("ignores all configuration options", func(t *testing.T) {
		// Even with all options enabled, agent gets the same limited set
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper:       {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
			ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: true},
			ToolGroupAgentTools:      {Enabled: true},
		}

		tools := ComputeAgentTools(state)

		// Should only have 3 tools
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
