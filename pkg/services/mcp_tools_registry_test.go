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

	// Empty state: no toggles enabled, only health
	toolNames := extractToolNames(tools)
	assert.Contains(t, toolNames, "health")
	assert.Len(t, tools, 1, "empty state should only return health")
}

func TestGetEnabledTools_DeveloperEnabled(t *testing.T) {
	// Legacy developer.Enabled is now ignored — toggles control tools
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// No toggles set, only health
	assert.Contains(t, toolNames, "health")
	assert.Len(t, tools, 1, "developer.Enabled alone should only return health")
}

func TestGetEnabledTools_DeveloperWithQueryTools(t *testing.T) {
	// Legacy AddQueryTools maps to AddDirectDatabaseAccess + AddApprovalTools
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {Enabled: true, AddQueryTools: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Mapped via backward compat: AddDirectDatabaseAccess + AddApprovalTools
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "list_query_suggestions")

	// Ontology Maintenance tools should NOT be included
	assert.NotContains(t, toolNames, "update_table")
	assert.NotContains(t, toolNames, "update_column")
	assert.NotContains(t, toolNames, "refresh_schema")
}

func TestGetEnabledTools_DeveloperWithOntologyMaintenance(t *testing.T) {
	// Legacy AddOntologyMaintenance maps to AddOntologyMaintenanceTools
	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper: {Enabled: true, AddOntologyMaintenance: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	assert.Contains(t, toolNames, "health")
	// Ontology Maintenance tools
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "update_table")
	assert.Contains(t, toolNames, "update_column")
	assert.Contains(t, toolNames, "list_ontology_questions")
	assert.Contains(t, toolNames, "resolve_ontology_question")
	assert.Contains(t, toolNames, "refresh_schema")
	assert.Contains(t, toolNames, "list_pending_changes")

	// Direct Database Access tools should NOT be included
	assert.NotContains(t, toolNames, "echo")
	assert.NotContains(t, toolNames, "execute")
}

func TestGetEnabledTools_WithToolsKey(t *testing.T) {
	// New "tools" config key takes precedence
	state := map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddApprovalTools:            true,
			AddOntologySuggestions:      true,
			AddRequestTools:             true,
		},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// All toggle-controlled tools should be present
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "echo")
	assert.Contains(t, toolNames, "execute")
	assert.Contains(t, toolNames, "query")
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "update_table")
	assert.Contains(t, toolNames, "list_ontology_questions")
	assert.Contains(t, toolNames, "list_query_suggestions")
	assert.Contains(t, toolNames, "get_context")
	assert.Contains(t, toolNames, "get_ontology")
}

func TestGetEnabledTools_ApprovedQueriesEnabled(t *testing.T) {
	// Legacy approved_queries.Enabled has no effect on tool computation
	state := map[string]*models.ToolGroupConfig{
		ToolGroupApprovedQueries: {Enabled: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Only health - no toggles set
	assert.Contains(t, toolNames, "health")
	assert.Len(t, tools, 1)
}

func TestGetEnabledTools_ApprovedQueriesWithOntologyMaintenance(t *testing.T) {
	// Legacy approved_queries flags don't affect new toggle system
	state := map[string]*models.ToolGroupConfig{
		ToolGroupApprovedQueries: {Enabled: true, AllowOntologyMaintenance: true},
	}
	tools := GetEnabledTools(state)

	toolNames := extractToolNames(tools)

	// Only health - approved_queries flags don't map to new toggles
	assert.Contains(t, toolNames, "health")
	assert.NotContains(t, toolNames, "update_table")
	assert.NotContains(t, toolNames, "update_column")
}

func TestGetEnabledTools_AgentToolsEnabled_UserPerspective(t *testing.T) {
	// From USER perspective, agent_tools doesn't add any tools
	state := map[string]*models.ToolGroupConfig{
		ToolGroupAgentTools: {Enabled: true},
	}
	tools := GetEnabledTools(state) // GetEnabledTools uses isAgent=false

	toolNames := extractToolNames(tools)

	// Only health - no toggles set for developer/user tools
	assert.Contains(t, toolNames, "health")
	assert.Len(t, tools, 1)
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
	state := map[string]*models.ToolGroupConfig{
		"tools": {AddDirectDatabaseAccess: true, AddOntologyMaintenanceTools: true},
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
