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
			ToolGroupDeveloper: {Enabled: false},
		}
		assert.True(t, checker.IsToolAccessible("health", state, false))
		assert.True(t, checker.IsToolAccessible("health", state, true))
	})

	t.Run("unknown tool is never accessible", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {Enabled: true},
		}
		assert.False(t, checker.IsToolAccessible("unknown_tool", state, false))
		assert.False(t, checker.IsToolAccessible("unknown_tool", state, true))
	})

	t.Run("nil state only allows always-tools", func(t *testing.T) {
		// Nil state = no config = only default loadout (health)
		assert.True(t, checker.IsToolAccessible("health", nil, false))
		assert.False(t, checker.IsToolAccessible("echo", nil, false))
		assert.False(t, checker.IsToolAccessible("query", nil, false))
		assert.False(t, checker.IsToolAccessible("list_approved_queries", nil, false))
	})
}

func TestToolAccessChecker_DeveloperToolsEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("user gets Developer Core tools when developer is enabled", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper:       {Enabled: true},
			ToolGroupApprovedQueries: {Enabled: false},
			ToolGroupAgentTools:      {Enabled: false},
		}

		// Developer Core tools (echo, execute only)
		assert.True(t, checker.IsToolAccessible("echo", state, false), "echo should be accessible")
		assert.True(t, checker.IsToolAccessible("execute", state, false), "execute should be accessible")

		// Query loadout tools NOT accessible without AddQueryTools option
		assert.False(t, checker.IsToolAccessible("validate", state, false), "validate NOT accessible without AddQueryTools")
		assert.False(t, checker.IsToolAccessible("query", state, false), "query NOT accessible without AddQueryTools")
		assert.False(t, checker.IsToolAccessible("explain_query", state, false), "explain_query NOT accessible without AddQueryTools")
		assert.False(t, checker.IsToolAccessible("get_schema", state, false), "get_schema NOT accessible without AddQueryTools")
		assert.False(t, checker.IsToolAccessible("sample", state, false), "sample NOT accessible without AddQueryTools")
		assert.False(t, checker.IsToolAccessible("get_ontology", state, false), "get_ontology NOT accessible without AddQueryTools")
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, false), "list_approved_queries NOT accessible without AddQueryTools")
	})

	t.Run("user gets more tools with AddQueryTools option", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {Enabled: true, AddQueryTools: true},
		}

		// Developer Core + Query (NO Ontology Maintenance - that requires AddOntologyMaintenance)
		assert.True(t, checker.IsToolAccessible("echo", state, false))
		assert.True(t, checker.IsToolAccessible("get_schema", state, false))
		assert.True(t, checker.IsToolAccessible("sample", state, false))
		assert.True(t, checker.IsToolAccessible("get_ontology", state, false))
		assert.True(t, checker.IsToolAccessible("list_approved_queries", state, false))
		// Ontology Maintenance NOT accessible without AddOntologyMaintenance
		assert.False(t, checker.IsToolAccessible("update_entity", state, false))
		assert.False(t, checker.IsToolAccessible("update_column", state, false))
	})

	t.Run("user gets Ontology Maintenance tools with AddOntologyMaintenance option", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {Enabled: true, AddOntologyMaintenance: true},
		}

		// Developer Core + Ontology Questions + Ontology Maintenance
		assert.True(t, checker.IsToolAccessible("echo", state, false))
		assert.True(t, checker.IsToolAccessible("list_ontology_questions", state, false))
		assert.True(t, checker.IsToolAccessible("resolve_ontology_question", state, false))
		assert.True(t, checker.IsToolAccessible("update_entity", state, false))
		assert.True(t, checker.IsToolAccessible("refresh_schema", state, false))

		// Query tools NOT accessible
		assert.False(t, checker.IsToolAccessible("get_schema", state, false))
		assert.False(t, checker.IsToolAccessible("sample", state, false))
	})

	t.Run("agent does not get developer tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {Enabled: true, AddQueryTools: true},
		}

		// Agent doesn't get tools just from developer mode
		assert.False(t, checker.IsToolAccessible("execute", state, true))
		assert.False(t, checker.IsToolAccessible("echo", state, true))
	})
}

func TestToolAccessChecker_ApprovedQueriesEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	// NOTE: For user auth, the Enabled flag is now IGNORED. Tools are determined by sub-options.
	// approved_queries.Enabled no longer controls user tools - only developer sub-options do.

	t.Run("user gets Developer Core tools regardless of approved_queries setting", func(t *testing.T) {
		// With new architecture, user auth always gets Developer Core.
		// approved_queries.Enabled is ignored for user auth.
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper:       {Enabled: false}, // Enabled is ignored
			ToolGroupApprovedQueries: {Enabled: true},  // Enabled is ignored for user auth
			ToolGroupAgentTools:      {Enabled: false},
		}

		// Developer Core tools are always included for user auth
		assert.True(t, checker.IsToolAccessible("echo", state, false), "echo should be accessible (Developer Core)")
		assert.True(t, checker.IsToolAccessible("execute", state, false), "execute should be accessible (Developer Core)")
		assert.True(t, checker.IsToolAccessible("health", state, false), "health should be accessible")

		// Query loadout NOT included without developer.AddQueryTools
		assert.False(t, checker.IsToolAccessible("query", state, false))
		assert.False(t, checker.IsToolAccessible("sample", state, false))
		assert.False(t, checker.IsToolAccessible("validate", state, false))
	})

	t.Run("user gets Query tools with developer.AddQueryTools option", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {AddQueryTools: true}, // Enabled is ignored, sub-options matter
		}

		// Developer Core + Query loadout
		assert.True(t, checker.IsToolAccessible("echo", state, false))
		assert.True(t, checker.IsToolAccessible("query", state, false))
		assert.True(t, checker.IsToolAccessible("sample", state, false))
		assert.True(t, checker.IsToolAccessible("validate", state, false))
		assert.True(t, checker.IsToolAccessible("get_ontology", state, false))
		assert.True(t, checker.IsToolAccessible("list_approved_queries", state, false))
		assert.True(t, checker.IsToolAccessible("get_schema", state, false))
	})

	t.Run("user gets Ontology Maintenance tools with developer.AddOntologyMaintenance option", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {AddOntologyMaintenance: true},
		}

		// Developer Core + Ontology Maintenance
		assert.True(t, checker.IsToolAccessible("echo", state, false))
		assert.True(t, checker.IsToolAccessible("update_entity", state, false))
		assert.True(t, checker.IsToolAccessible("update_column", state, false))
		assert.True(t, checker.IsToolAccessible("update_relationship", state, false))
		assert.True(t, checker.IsToolAccessible("delete_entity", state, false))
	})

	t.Run("agent does not get approved_queries tools without agent_tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupApprovedQueries: {Enabled: true},
			ToolGroupAgentTools:      {Enabled: false},
		}

		// Agents should NOT have access when only approved_queries is enabled
		// Agent auth still checks agent_tools.Enabled
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, true))
		assert.False(t, checker.IsToolAccessible("query", state, true))

		// Health is always accessible
		assert.True(t, checker.IsToolAccessible("health", state, true))
	})
}

func TestToolAccessChecker_AgentToolsEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("agent gets Limited Query tools when agent_tools is enabled", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper:       {Enabled: false},
			ToolGroupApprovedQueries: {Enabled: false},
			ToolGroupAgentTools:      {Enabled: true},
		}

		// Limited Query loadout for agents
		assert.True(t, checker.IsToolAccessible("health", state, true))
		assert.True(t, checker.IsToolAccessible("list_approved_queries", state, true))
		assert.True(t, checker.IsToolAccessible("execute_approved_query", state, true))

		// Other tools NOT accessible to agents
		assert.False(t, checker.IsToolAccessible("echo", state, true), "echo NOT in Limited Query")
		assert.False(t, checker.IsToolAccessible("query", state, true))
		assert.False(t, checker.IsToolAccessible("execute", state, true))
		assert.False(t, checker.IsToolAccessible("get_schema", state, true))
	})

	t.Run("user does not get tools from agent_tools mode", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper:       {Enabled: false}, // Enabled is ignored for user auth
			ToolGroupApprovedQueries: {Enabled: false}, // Enabled is ignored for user auth
			ToolGroupAgentTools:      {Enabled: true},
		}

		// For user auth, Developer Core is always included (Enabled is ignored)
		// agent_tools setting only affects agent authentication
		assert.True(t, checker.IsToolAccessible("echo", state, false), "echo should be accessible (Developer Core)")
		assert.True(t, checker.IsToolAccessible("health", state, false))

		// Query tools NOT included without AddQueryTools
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, false))
		assert.False(t, checker.IsToolAccessible("query", state, false))
	})
}

func TestToolAccessChecker_NoToolGroupEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: false}, // Enabled is ignored for user auth
		ToolGroupApprovedQueries: {Enabled: false}, // Enabled is ignored for user auth
		ToolGroupAgentTools:      {Enabled: false}, // Enabled IS checked for agent auth
	}

	t.Run("user gets Developer Core tools even without Enabled flag", func(t *testing.T) {
		// For user auth, Enabled is ignored - Developer Core is always included
		assert.True(t, checker.IsToolAccessible("health", state, false), "health should be accessible")
		assert.True(t, checker.IsToolAccessible("echo", state, false), "echo should be accessible (Developer Core)")
		assert.True(t, checker.IsToolAccessible("execute", state, false), "execute should be accessible (Developer Core)")

		// Query tools NOT accessible without AddQueryTools
		assert.False(t, checker.IsToolAccessible("query", state, false), "query should NOT be accessible without AddQueryTools")
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, false), "list_approved_queries should NOT be accessible without AddQueryTools")
	})

	t.Run("agent only gets health when agent_tools.Enabled=false", func(t *testing.T) {
		// Agent auth still checks agent_tools.Enabled
		assert.True(t, checker.IsToolAccessible("health", state, true), "health should be accessible")
		assert.False(t, checker.IsToolAccessible("echo", state, true), "echo should NOT be accessible")
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, true), "list_approved_queries should NOT be accessible")
	})
}

func TestToolAccessChecker_GetAccessibleTools(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("developer mode returns Developer Core tools for user", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {Enabled: true},
		}

		tools := checker.GetAccessibleTools(state, false)
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		// Developer Core tools (health + echo + execute only)
		assert.True(t, toolNames["health"])
		assert.True(t, toolNames["echo"])
		assert.True(t, toolNames["execute"])

		// Query tools NOT included without AddQueryTools
		assert.False(t, toolNames["validate"])
		assert.False(t, toolNames["query"])
		assert.False(t, toolNames["explain_query"])
		assert.False(t, toolNames["get_schema"])
		assert.False(t, toolNames["sample"])
	})

	t.Run("developer with AddQueryTools returns more tools", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {Enabled: true, AddQueryTools: true},
		}

		tools := checker.GetAccessibleTools(state, false)
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		// Developer Core + Query (NO Ontology Maintenance - that requires AddOntologyMaintenance)
		assert.True(t, toolNames["echo"])
		assert.True(t, toolNames["get_schema"])
		assert.True(t, toolNames["sample"])
		assert.True(t, toolNames["list_approved_queries"])
		// Ontology Maintenance NOT included without AddOntologyMaintenance
		assert.False(t, toolNames["update_entity"])
	})

	t.Run("approved_queries mode returns Developer Core tools for user", func(t *testing.T) {
		// For user auth, approved_queries.Enabled is ignored
		// User gets Developer Core (no sub-options set)
		state := map[string]*models.ToolGroupConfig{
			ToolGroupApprovedQueries: {Enabled: true}, // Enabled is ignored
		}

		tools := checker.GetAccessibleTools(state, false)
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		// Developer Core tools always included for user auth
		assert.True(t, toolNames["health"])
		assert.True(t, toolNames["echo"])
		assert.True(t, toolNames["execute"])

		// Query tools NOT included without AddQueryTools
		assert.False(t, toolNames["query"])
		assert.False(t, toolNames["get_schema"])
		assert.False(t, toolNames["list_approved_queries"])
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
		// Nil state = no config = only default loadout (health)
		// This is different from empty state or state with empty configs
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
// tool execution checks use the same logic. This is the key test that prevents
// the bug where tools are listed but cannot be called.
func TestToolAccessChecker_ListAndCallConsistency(t *testing.T) {
	checker := NewToolAccessChecker()

	testCases := []struct {
		name    string
		state   map[string]*models.ToolGroupConfig
		isAgent bool
	}{
		{
			name:    "developer mode user with AddQueryTools",
			state:   map[string]*models.ToolGroupConfig{ToolGroupDeveloper: {AddQueryTools: true}},
			isAgent: false,
		},
		{
			name:    "developer mode user with AddOntologyMaintenance",
			state:   map[string]*models.ToolGroupConfig{ToolGroupDeveloper: {AddOntologyMaintenance: true}},
			isAgent: false,
		},
		{
			name:    "developer mode user with no sub-options",
			state:   map[string]*models.ToolGroupConfig{ToolGroupDeveloper: {}},
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
