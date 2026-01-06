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

	// Only health should be available with empty state
	require.Len(t, tools, 1)
	assert.Equal(t, "health", tools[0].Name)
}

func TestGetEnabledTools_DeveloperEnabled(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	// Should include developer tools (minus execute since EnableExecute=false) plus health
	// Note: query, sample, validate are now in approved_queries group
	toolNames := extractToolNames(tools)

	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "health")

	// execute requires EnableExecute sub-option
	assert.NotContains(t, toolNames, "execute")

	// Business user tools (query, sample, validate) are now in approved_queries group
	assert.NotContains(t, toolNames, "query")
	assert.NotContains(t, toolNames, "sample")
	assert.NotContains(t, toolNames, "validate")

	// approved_queries tools should not be included
	assert.NotContains(t, toolNames, "list_approved_queries")
	assert.NotContains(t, toolNames, "execute_approved_query")
	assert.NotContains(t, toolNames, "get_ontology")
	assert.NotContains(t, toolNames, "get_glossary")
}

func TestGetEnabledTools_DeveloperWithExecute(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {Enabled: true, EnableExecute: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// execute should now be included
	assert.Contains(t, toolNames, "execute")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "health")

	// query is now in approved_queries group, should not be present
	assert.NotContains(t, toolNames, "query")
}

func TestGetEnabledTools_ApprovedQueriesEnabled(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupApprovedQueries: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Should include approved_queries tools plus health
	// Business user tools (query, sample, validate) are now in approved_queries group
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "sample")
	assert.Contains(t, toolNames, "validate")
	assert.Contains(t, toolNames, "list_approved_queries")
	assert.Contains(t, toolNames, "execute_approved_query")
	assert.Contains(t, toolNames, "get_ontology")
	assert.Contains(t, toolNames, "get_glossary")
	assert.Contains(t, toolNames, "health")

	// developer tools should not be included
	assert.NotContains(t, toolNames, "echo")
	assert.NotContains(t, toolNames, "execute")
	assert.NotContains(t, toolNames, "get_schema")
}

func TestGetEnabledTools_BothGroupsEnabled(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: true, EnableExecute: true},
		ToolGroupApprovedQueries: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Should include both groups
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "execute")
	assert.Contains(t, toolNames, "list_approved_queries")
	assert.Contains(t, toolNames, "get_ontology")
	assert.Contains(t, toolNames, "health")
}

func TestGetEnabledTools_ForceModeHidesDeveloper(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: true, EnableExecute: true},
		ToolGroupApprovedQueries: {Enabled: true, ForceMode: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Force mode should hide developer tools even when enabled
	assert.NotContains(t, toolNames, "echo")
	assert.NotContains(t, toolNames, "execute")
	assert.NotContains(t, toolNames, "get_schema")

	// approved_queries tools (including business user tools) should still be visible
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "sample")
	assert.Contains(t, toolNames, "validate")
	assert.Contains(t, toolNames, "list_approved_queries")
	assert.Contains(t, toolNames, "get_ontology")
	assert.Contains(t, toolNames, "health")
}

func TestGetEnabledTools_AgentToolsEnabled(t *testing.T) {
	state := map[string]*models.ToolGroupConfig{
		ToolGroupAgentTools: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Agent tools mode: only specific tools available
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "list_approved_queries")
	assert.Contains(t, toolNames, "execute_approved_query")
	assert.Contains(t, toolNames, "health")

	// Developer-only tools should NOT be available
	assert.NotContains(t, toolNames, "query")
	assert.NotContains(t, toolNames, "sample")
	assert.NotContains(t, toolNames, "validate")
	assert.NotContains(t, toolNames, "execute")
	assert.NotContains(t, toolNames, "get_schema")

	// Ontology tools should NOT be available in agent mode
	assert.NotContains(t, toolNames, "get_ontology")
	assert.NotContains(t, toolNames, "get_glossary")
}

func TestGetEnabledTools_AgentToolsOverridesOthers(t *testing.T) {
	// Even with developer and approved_queries enabled, agent_tools takes precedence
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: true, EnableExecute: true},
		ToolGroupApprovedQueries: {Enabled: true},
		ToolGroupAgentTools:      {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Only agent-allowed tools should be visible
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "list_approved_queries")
	assert.Contains(t, toolNames, "execute_approved_query")
	assert.Contains(t, toolNames, "health")

	// Other tools should be hidden
	assert.NotContains(t, toolNames, "query")
	assert.NotContains(t, toolNames, "get_ontology")

	// Should have exactly 4 tools
	assert.Len(t, tools, 4)
}

func TestToolRegistry_ContainsAllExpectedTools(t *testing.T) {
	expectedTools := []string{
		"echo", "query", "sample", "validate", "execute", "get_schema",
		"get_ontology", "get_glossary", "list_approved_queries", "execute_approved_query",
		"health",
	}

	toolNames := make(map[string]bool)
	for _, tool := range ToolRegistry {
		toolNames[tool.Name] = true
	}

	for _, expected := range expectedTools {
		assert.True(t, toolNames[expected], "ToolRegistry missing expected tool: %s", expected)
	}
}

func TestToolRegistry_AllToolsHaveDescriptions(t *testing.T) {
	for _, tool := range ToolRegistry {
		assert.NotEmpty(t, tool.Description, "Tool %s has empty description", tool.Name)
	}
}

func TestToolRegistry_AllToolsHaveValidToolGroup(t *testing.T) {
	validGroups := map[string]bool{
		ToolGroupDeveloper:       true,
		ToolGroupApprovedQueries: true,
		ToolGroupAgentTools:      true,
		"always":                 true,
	}

	for _, tool := range ToolRegistry {
		assert.True(t, validGroups[tool.ToolGroup],
			"Tool %s has invalid tool group: %s", tool.Name, tool.ToolGroup)
	}
}

func TestToolRegistry_ExecuteHasSubOption(t *testing.T) {
	for _, tool := range ToolRegistry {
		if tool.Name == "execute" {
			assert.Equal(t, "enableExecute", tool.SubOption,
				"execute tool should have enableExecute sub-option")
			return
		}
	}
	t.Fatal("execute tool not found in registry")
}

func extractToolNames(tools []ToolDefinition) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}
