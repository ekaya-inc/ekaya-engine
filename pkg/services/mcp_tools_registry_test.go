package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEnabledTools_NilState(t *testing.T) {
	tools := GetEnabledTools(nil)

	// Only health should be available with nil state
	require.Len(t, tools, 1)
	assert.Equal(t, "health", tools[0].Name)
}

func TestGetEnabledTools_EmptyState(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{}
	tools := GetEnabledTools(state)

	// For user auth, Developer Core is always included
	toolNames := extractToolNames(tools)
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")
}

func TestGetEnabledTools_DeveloperEnabled(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	// Developer mode only enables Developer Core loadout (health, echo, execute)
	toolNames := extractToolNames(tools)

	// Developer Core tools only
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")

	// Query loadout tools should NOT be included without AddQueryTools option
	assert.NotContains(t, toolNames, "validate")
	assert.NotContains(t, toolNames, "query")
	assert.NotContains(t, toolNames, "explain_query")
	assert.NotContains(t, toolNames, "get_schema")
	assert.NotContains(t, toolNames, "sample")
	assert.NotContains(t, toolNames, "list_approved_queries")
	assert.NotContains(t, toolNames, "get_ontology")
}

func TestGetEnabledTools_DeveloperWithQueryTools(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {Enabled: true, AddQueryTools: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Developer Core + Query (but NOT Ontology Maintenance)
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "get_ontology")
	assert.Contains(t, toolNames, "sample")
	assert.Contains(t, toolNames, "list_approved_queries")

	// Ontology Maintenance tools should NOT be included with AddQueryTools alone
	assert.NotContains(t, toolNames, "update_entity")
	assert.NotContains(t, toolNames, "update_column")
	assert.NotContains(t, toolNames, "refresh_schema")
}

func TestGetEnabledTools_DeveloperWithOntologyMaintenance(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {Enabled: true, AddOntologyMaintenance: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Developer Core + Ontology Maintenance + Ontology Questions
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")
	// Ontology Questions tools
	assert.Contains(t, toolNames, "list_ontology_questions")
	assert.Contains(t, toolNames, "resolve_ontology_question")
	assert.Contains(t, toolNames, "skip_ontology_question")
	// Ontology Maintenance tools
	assert.Contains(t, toolNames, "update_entity")
	assert.Contains(t, toolNames, "update_column")
	assert.Contains(t, toolNames, "refresh_schema")
	assert.Contains(t, toolNames, "list_pending_changes")

	// Query tools should NOT be included
	assert.NotContains(t, toolNames, "get_schema")
	assert.NotContains(t, toolNames, "sample")
}

func TestGetEnabledTools_ApprovedQueriesEnabled(t *testing.T) {
	// NOTE: For user auth, approved_queries.Enabled is ignored.
	// Developer Core is always included. Use developer.AddQueryTools for Query loadout.
	state := map[string]*models.ToolGroupConfig{
		ToolGroupApprovedQueries: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Developer Core is always included for user auth
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")

	// Query tools require developer.AddQueryTools=true, not approved_queries.Enabled
	assert.NotContains(t, toolNames, "query")
	assert.NotContains(t, toolNames, "sample")
	assert.NotContains(t, toolNames, "validate")
}

func TestGetEnabledTools_ApprovedQueriesWithOntologyMaintenance(t *testing.T) {
	// NOTE: For user auth, approved_queries flags are ignored.
	// Use developer.AddOntologyMaintenance for Ontology Maintenance loadout.
	state := map[string]*models.ToolGroupConfig{
		ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Developer Core is always included for user auth
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")

	// Ontology Maintenance requires developer.AddOntologyMaintenance, not approved_queries flags
	assert.NotContains(t, toolNames, "update_entity")
	assert.NotContains(t, toolNames, "update_column")
}

func TestGetEnabledTools_AgentToolsEnabled_UserPerspective(t *testing.T) {
	// From USER perspective, enabling agent_tools doesn't add any tools beyond Developer Core
	// Developer Core is always included for user auth
	state := map[string]*models.ToolGroupConfig{
		ToolGroupAgentTools: {Enabled: true},
	}
	tools := GetEnabledTools(state) // GetEnabledTools uses isAgent=false

	toolNames := extractToolNames(tools)

	// Developer Core is always included for user auth
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")

	// Agent-specific tools should NOT be included in user perspective
	// (Limited Query loadout is only for agents)
}

func TestGetEnabledToolsForAgent_AgentToolsEnabled(t *testing.T) {
	// From AGENT perspective, enabling agent_tools gives Limited Query tools
	state := map[string]*models.ToolGroupConfig{
		ToolGroupAgentTools: {Enabled: true},
	}
	tools := GetEnabledToolsForAgent(state)

	toolNames := extractToolNames(tools)

	// Limited Query loadout
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "list_approved_queries")
	assert.Contains(t, toolNames, "execute_approved_query")

	// Should NOT include other tools
	assert.NotContains(t, toolNames, "query")
	assert.NotContains(t, toolNames, "execute")
	assert.NotContains(t, toolNames, "echo")
}

func TestGetEnabledTools_CustomToolsEnabled(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupCustom: {
			Enabled:     true,
			CustomTools: []string{"query", "sample", "get_ontology"},
		},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Selected tools + health (always included)
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "sample")
	assert.Contains(t, toolNames, "get_ontology")

	// Non-selected tools should NOT be included
	assert.NotContains(t, toolNames, "execute")
	assert.NotContains(t, toolNames, "echo")
	assert.NotContains(t, toolNames, "get_schema")
}

func TestGetEnabledTools_ToolsInCanonicalOrder(t *testing.T) {
	// Use developer.AddQueryTools to get Query loadout for more tools to verify order
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {AddQueryTools: true},
	}
	tools := GetEnabledTools(state)

	// Tools should be in canonical order (as defined in AllToolsOrdered)
	toolNames := extractToolNames(tools)

	// health should come first
	assert.Equal(t, "health", toolNames[0])

	// Verify order matches AllToolsOrdered for tools that are present
	var lastIndex int
	for _, name := range toolNames {
		idx := GetToolOrder(name)
		require.GreaterOrEqual(t, idx, lastIndex, "Tools should be in canonical order")
		lastIndex = idx
	}
}

func TestToolRegistry_ContainsAllExpectedTools(t *testing.T) {
	// Verify all tools in AllToolsOrdered are defined
	for _, tool := range AllToolsOrdered {
		spec := GetToolSpec(tool.Name)
		require.NotNil(t, spec, "Tool %s should be in AllToolsOrdered", tool.Name)
		assert.NotEmpty(t, spec.Description, "Tool %s should have a description", tool.Name)
	}
}

func TestMergeLoadouts_Deduplication(t *testing.T) {
	// Query and Developer Core both have validate, query, explain_query
	tools := MergeLoadouts(LoadoutDeveloperCore, LoadoutQuery)

	// Count occurrences of each tool
	counts := make(map[string]int)
	for _, tool := range tools {
		counts[tool.Name]++
	}

	// Each tool should appear exactly once
	for name, count := range counts {
		assert.Equal(t, 1, count, "Tool %s should appear exactly once", name)
	}
}

func extractToolNames(tools []ToolDefinition) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}
