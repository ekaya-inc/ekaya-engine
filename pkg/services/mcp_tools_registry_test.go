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

	// Developer mode enables ALL tools including execute
	toolNames := extractToolNames(tools)

	// Developer-specific tools - execute is now available by default
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "execute") // execute is now available when developer mode is on

	// Business user tools are also available in developer mode
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "sample")
	assert.Contains(t, toolNames, "validate")
	assert.Contains(t, toolNames, "list_approved_queries")
	assert.Contains(t, toolNames, "execute_approved_query")
	assert.Contains(t, toolNames, "get_ontology")
	assert.Contains(t, toolNames, "list_glossary")
	assert.Contains(t, toolNames, "get_glossary_sql")

	// Health is always included
	assert.Contains(t, toolNames, "health")

	// Should have all tools from the registry
	assert.Len(t, tools, len(ToolRegistry))
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
	assert.Contains(t, toolNames, "list_glossary")
	assert.Contains(t, toolNames, "get_glossary_sql")
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

func TestGetEnabledTools_ForceModeNoLongerHidesDeveloper(t *testing.T) {
	// Note: ForceMode is no longer used with radio button behavior.
	// With radio buttons, only one group can be enabled at a time.
	// This test verifies that ForceMode flag is ignored if present.
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: true},
		ToolGroupApprovedQueries: {Enabled: true, ForceMode: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Both groups' tools should be visible (GetEnabledTools doesn't enforce radio button,
	// that's done by the state machine). Execute is now available by default.
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute") // execute is now available when developer mode is on
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "sample")
	assert.Contains(t, toolNames, "validate")
	assert.Contains(t, toolNames, "list_approved_queries")
	assert.Contains(t, toolNames, "get_ontology")
	assert.Contains(t, toolNames, "health")
}

func TestGetEnabledTools_AgentToolsEnabled(t *testing.T) {
	// GetEnabledTools returns tools for USER perspective (not agent).
	// When only agent_tools is enabled, from user perspective only health is available
	// since neither developer nor approved_queries is enabled.
	state := map[string]*models.ToolGroupConfig{
		ToolGroupAgentTools: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// From user perspective with only agent_tools enabled, only health is available
	assert.Contains(t, toolNames, "health")
	assert.Len(t, tools, 1)
}

func TestGetEnabledTools_AgentToolsDoesNotOverrideForUsers(t *testing.T) {
	// GetEnabledTools returns tools for USER perspective.
	// When developer is enabled, users get developer tools regardless of agent_tools.
	// agent_tools only affects actual agent callers (via GetEnabledToolsForAgent).
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: true, EnableExecute: true},
		ToolGroupApprovedQueries: {Enabled: true},
		ToolGroupAgentTools:      {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// From user perspective, developer mode gives ALL tools
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "get_ontology")
	assert.Contains(t, toolNames, "health")

	// Should have all tools from the registry
	assert.Len(t, tools, len(ToolRegistry))
}

func TestToolRegistry_ContainsAllExpectedTools(t *testing.T) {
	expectedTools := []string{
		"echo", "query", "sample", "validate", "execute", "get_schema",
		"get_ontology", "list_glossary", "get_glossary_sql", "list_approved_queries", "execute_approved_query",
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

func TestToolRegistry_ExecuteToolExists(t *testing.T) {
	// Execute is a developer tool that allows DDL/DML operations.
	// It's now available whenever developer mode is enabled (no separate opt-in).
	found := false
	for _, tool := range ToolRegistry {
		if tool.Name == "execute" {
			assert.Equal(t, ToolGroupDeveloper, tool.ToolGroup,
				"execute tool should be in developer group")
			assert.Empty(t, tool.SubOption,
				"execute tool should not require a sub-option")
			found = true
			break
		}
	}
	assert.True(t, found, "execute tool should exist in registry")
}

func extractToolNames(tools []ToolDefinition) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}
