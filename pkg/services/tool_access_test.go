package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolAccessChecker_IsToolAccessible(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("health is always accessible", func(t *testing.T) {
		// No state
		assert.True(t, checker.IsToolAccessible("health", nil, false))
		assert.True(t, checker.IsToolAccessible("health", nil, true))

		// Empty state
		assert.True(t, checker.IsToolAccessible("health", map[string]*models.ToolGroupConfig{}, false))
		assert.True(t, checker.IsToolAccessible("health", map[string]*models.ToolGroupConfig{}, true))

		// Any state
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddDirectDatabaseAccess: true},
		}
		assert.True(t, checker.IsToolAccessible("health", state, false))
		assert.True(t, checker.IsToolAccessible("health", state, true))
	})

	t.Run("unknown tool is never accessible", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {
				AddDirectDatabaseAccess:     true,
				AddOntologyMaintenanceTools: true,
				AddApprovalTools:            true,
				AddOntologySuggestions:      true,
				AddRequestTools:             true,
			},
		}
		assert.False(t, checker.IsToolAccessible("unknown_tool", state, false))
		assert.False(t, checker.IsToolAccessible("unknown_tool", state, true))
	})

	t.Run("nil state only allows health", func(t *testing.T) {
		// Nil state = no config = only health
		assert.True(t, checker.IsToolAccessible("health", nil, false))
		assert.False(t, checker.IsToolAccessible("echo", nil, false))
		assert.False(t, checker.IsToolAccessible("query", nil, false))
		assert.False(t, checker.IsToolAccessible("list_approved_queries", nil, false))
	})
}

func TestToolAccessChecker_DeveloperToolsEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("user gets Direct Database Access tools with toggle", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddDirectDatabaseAccess: true},
		}

		// Direct Database Access tools
		assert.True(t, checker.IsToolAccessible("echo", state, false), "echo should be accessible")
		assert.True(t, checker.IsToolAccessible("execute", state, false), "execute should be accessible")
		assert.True(t, checker.IsToolAccessible("query", state, false), "query should be accessible")

		// Ontology tools NOT accessible without toggle
		assert.False(t, checker.IsToolAccessible("update_table", state, false), "update_table NOT accessible")
		assert.False(t, checker.IsToolAccessible("update_column", state, false), "update_column NOT accessible")
	})

	t.Run("user gets more tools with Ontology Maintenance toggle", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddOntologyMaintenanceTools: true},
		}

		// Ontology Maintenance tools
		assert.True(t, checker.IsToolAccessible("get_schema", state, false))
		assert.True(t, checker.IsToolAccessible("list_project_knowledge", state, false))
		assert.True(t, checker.IsToolAccessible("update_table", state, false))
		assert.True(t, checker.IsToolAccessible("update_column", state, false))
		assert.True(t, checker.IsToolAccessible("list_ontology_questions", state, false))

		// Direct Database Access NOT accessible without toggle
		assert.False(t, checker.IsToolAccessible("echo", state, false))
		assert.False(t, checker.IsToolAccessible("execute", state, false))
	})

	t.Run("user gets Approval tools with toggle", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddApprovalTools: true},
		}

		assert.True(t, checker.IsToolAccessible("list_query_suggestions", state, false))
		assert.True(t, checker.IsToolAccessible("approve_query_suggestion", state, false))
		assert.True(t, checker.IsToolAccessible("create_approved_query", state, false))

		// No other tools
		assert.False(t, checker.IsToolAccessible("echo", state, false))
	})

	t.Run("agent does not get developer tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddDirectDatabaseAccess: true},
		}

		// Agent doesn't get tools just from toggles
		assert.False(t, checker.IsToolAccessible("execute", state, true))
		assert.False(t, checker.IsToolAccessible("echo", state, true))
	})
}

func TestToolAccessChecker_ApprovedQueriesEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("legacy approved_queries has no effect on new system", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupApprovedQueries: {Enabled: true},
		}

		// Only health - legacy Enabled flag doesn't map to new toggles
		assert.True(t, checker.IsToolAccessible("health", state, false))
		assert.False(t, checker.IsToolAccessible("echo", state, false))
		assert.False(t, checker.IsToolAccessible("query", state, false))
	})

	t.Run("user gets tools with proper toggles", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {
				AddDirectDatabaseAccess: true,
				AddApprovalTools:        true,
			},
		}

		assert.True(t, checker.IsToolAccessible("echo", state, false))
		assert.True(t, checker.IsToolAccessible("query", state, false))
		assert.True(t, checker.IsToolAccessible("list_query_suggestions", state, false))
	})

	t.Run("agent does not get tools without agent_tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupApprovedQueries: {Enabled: true},
			ToolGroupAgentTools:      {Enabled: false},
		}

		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, true))
		assert.False(t, checker.IsToolAccessible("query", state, true))
		assert.True(t, checker.IsToolAccessible("health", state, true))
	})
}

func TestToolAccessChecker_AgentToolsEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("agent gets Limited Query tools when agent_tools is enabled", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupAgentTools: {Enabled: true},
		}

		// Limited Query loadout for agents
		assert.True(t, checker.IsToolAccessible("health", state, true))
		assert.True(t, checker.IsToolAccessible("list_approved_queries", state, true))
		assert.True(t, checker.IsToolAccessible("execute_approved_query", state, true))

		// Other tools NOT accessible to agents
		assert.False(t, checker.IsToolAccessible("echo", state, true), "echo NOT in Limited Query")
		assert.False(t, checker.IsToolAccessible("query", state, true))
		assert.False(t, checker.IsToolAccessible("execute", state, true))
	})

	t.Run("user does not get tools from agent_tools mode", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupAgentTools: {Enabled: true},
		}

		// No toggles set for developer/user tools, so only health
		assert.True(t, checker.IsToolAccessible("health", state, false))
		assert.False(t, checker.IsToolAccessible("echo", state, false))
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, false))
	})
}

func TestToolAccessChecker_NoToolGroupEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		ToolGroupAgentTools: {Enabled: false},
	}

	t.Run("user gets only health without toggles", func(t *testing.T) {
		assert.True(t, checker.IsToolAccessible("health", state, false), "health should be accessible")
		assert.False(t, checker.IsToolAccessible("echo", state, false), "echo should NOT be accessible")
		assert.False(t, checker.IsToolAccessible("query", state, false), "query should NOT be accessible")
	})

	t.Run("agent only gets health when agent_tools.Enabled=false", func(t *testing.T) {
		assert.True(t, checker.IsToolAccessible("health", state, true), "health should be accessible")
		assert.False(t, checker.IsToolAccessible("echo", state, true), "echo should NOT be accessible")
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, true), "list_approved_queries should NOT be accessible")
	})
}

func TestToolAccessChecker_GetAccessibleTools(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("Direct Database Access returns developer tools for user", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {AddDirectDatabaseAccess: true},
		}

		tools := checker.GetAccessibleTools(state, false)
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		assert.True(t, toolNames["health"])
		assert.True(t, toolNames["echo"])
		assert.True(t, toolNames["execute"])
		assert.True(t, toolNames["query"])

		// No ontology tools
		assert.False(t, toolNames["update_table"])
		assert.False(t, toolNames["get_schema"])
	})

	t.Run("all toggles returns all tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			"tools": {
				AddDirectDatabaseAccess:     true,
				AddOntologyMaintenanceTools: true,
				AddApprovalTools:            true,
				AddOntologySuggestions:      true,
				AddRequestTools:             true,
			},
		}

		tools := checker.GetAccessibleTools(state, false)
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		assert.True(t, toolNames["echo"])
		assert.True(t, toolNames["get_schema"])
		assert.True(t, toolNames["list_project_knowledge"])
		assert.True(t, toolNames["update_table"])
		assert.True(t, toolNames["list_query_suggestions"])
		assert.True(t, toolNames["get_context"])
		assert.True(t, toolNames["query"])
	})

	t.Run("agent_tools mode returns Limited Query tools for agent", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupAgentTools: {Enabled: true},
		}

		tools := checker.GetAccessibleTools(state, true) // isAgent = true
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		// Limited Query loadout
		assert.True(t, toolNames["health"])
		assert.True(t, toolNames["list_approved_queries"])
		assert.True(t, toolNames["execute_approved_query"])

		// echo is NOT in Limited Query loadout
		assert.False(t, toolNames["echo"])
		assert.False(t, toolNames["query"])
		assert.False(t, toolNames["execute"])
	})

	t.Run("nil state returns only health", func(t *testing.T) {
		tools := checker.GetAccessibleTools(nil, false)

		require.Len(t, tools, 1)
		assert.Equal(t, "health", tools[0].Name)
	})
}

func TestToolAccessChecker_CustomToolsEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("user gets only selected custom tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupCustom: {
				Enabled:     true,
				CustomTools: []string{"query", "sample", "get_ontology"},
			},
		}

		// Selected tools
		assert.True(t, checker.IsToolAccessible("query", state, false))
		assert.True(t, checker.IsToolAccessible("sample", state, false))
		assert.True(t, checker.IsToolAccessible("get_ontology", state, false))

		// Health is always included
		assert.True(t, checker.IsToolAccessible("health", state, false))

		// Non-selected tools NOT accessible
		assert.False(t, checker.IsToolAccessible("execute", state, false))
		assert.False(t, checker.IsToolAccessible("echo", state, false))
		assert.False(t, checker.IsToolAccessible("get_schema", state, false))
	})
}

// TestToolAccessChecker_ListAndCallConsistency verifies that the tool list and
// tool execution checks use the same logic.
func TestToolAccessChecker_ListAndCallConsistency(t *testing.T) {
	checker := NewToolAccessChecker()

	testCases := []struct {
		name    string
		state   map[string]*models.ToolGroupConfig
		isAgent bool
	}{
		{
			name:    "Direct Database Access toggle",
			state:   map[string]*models.ToolGroupConfig{"tools": {AddDirectDatabaseAccess: true}},
			isAgent: false,
		},
		{
			name:    "Ontology Maintenance toggle",
			state:   map[string]*models.ToolGroupConfig{"tools": {AddOntologyMaintenanceTools: true}},
			isAgent: false,
		},
		{
			name:    "no toggles",
			state:   map[string]*models.ToolGroupConfig{},
			isAgent: false,
		},
		{
			name:    "agent_tools mode agent",
			state:   map[string]*models.ToolGroupConfig{ToolGroupAgentTools: {Enabled: true}},
			isAgent: true,
		},
		{
			name: "agent_tools disabled agent",
			state: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: false},
			},
			isAgent: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Get the list of accessible tools
			tools := checker.GetAccessibleTools(tc.state, tc.isAgent)

			// Verify each listed tool is also accessible via IsToolAccessible
			for _, tool := range tools {
				accessible := checker.IsToolAccessible(tool.Name, tc.state, tc.isAgent)
				assert.True(t, accessible, "tool %q is listed but IsToolAccessible returns false", tool.Name)
			}
		})
	}
}
