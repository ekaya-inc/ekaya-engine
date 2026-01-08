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
		assert.True(t, checker.IsToolAccessible("health", nil, false))
		assert.False(t, checker.IsToolAccessible("echo", nil, false))
		assert.False(t, checker.IsToolAccessible("query", nil, false))
		assert.False(t, checker.IsToolAccessible("list_approved_queries", nil, false))
	})
}

func TestToolAccessChecker_DeveloperToolsEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: true, EnableExecute: true},
		ToolGroupApprovedQueries: {Enabled: false},
		ToolGroupAgentTools:      {Enabled: false},
	}

	t.Run("user gets ALL tools when developer is enabled", func(t *testing.T) {
		// Developer tools
		assert.True(t, checker.IsToolAccessible("echo", state, false), "echo should be accessible")
		assert.True(t, checker.IsToolAccessible("execute", state, false), "execute should be accessible")
		assert.True(t, checker.IsToolAccessible("get_schema", state, false), "get_schema should be accessible")

		// Approved queries tools (also accessible in developer mode)
		assert.True(t, checker.IsToolAccessible("query", state, false), "query should be accessible")
		assert.True(t, checker.IsToolAccessible("sample", state, false), "sample should be accessible")
		assert.True(t, checker.IsToolAccessible("validate", state, false), "validate should be accessible")
		assert.True(t, checker.IsToolAccessible("get_ontology", state, false), "get_ontology should be accessible")
		assert.True(t, checker.IsToolAccessible("list_glossary", state, false), "list_glossary should be accessible")
		assert.True(t, checker.IsToolAccessible("get_glossary_sql", state, false), "get_glossary_sql should be accessible")
		assert.True(t, checker.IsToolAccessible("list_approved_queries", state, false), "list_approved_queries should be accessible")
		assert.True(t, checker.IsToolAccessible("execute_approved_query", state, false), "execute_approved_query should be accessible")

		// Always tools
		assert.True(t, checker.IsToolAccessible("health", state, false), "health should be accessible")
	})

	t.Run("agent does not get developer tools", func(t *testing.T) {
		// Agents should NOT have access when only developer is enabled
		assert.False(t, checker.IsToolAccessible("echo", state, true), "agent should not access echo")
		assert.False(t, checker.IsToolAccessible("query", state, true), "agent should not access query")
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, true), "agent should not access list_approved_queries")

		// Health is always accessible
		assert.True(t, checker.IsToolAccessible("health", state, true), "agent should access health")
	})

	t.Run("execute requires EnableExecute sub-option", func(t *testing.T) {
		stateWithoutExecute := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {Enabled: true, EnableExecute: false},
		}
		assert.False(t, checker.IsToolAccessible("execute", stateWithoutExecute, false), "execute should not be accessible without EnableExecute")
		assert.True(t, checker.IsToolAccessible("echo", stateWithoutExecute, false), "echo should still be accessible")
	})
}

func TestToolAccessChecker_ApprovedQueriesEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: false},
		ToolGroupApprovedQueries: {Enabled: true},
		ToolGroupAgentTools:      {Enabled: false},
	}

	t.Run("user gets approved_queries tools only", func(t *testing.T) {
		// Approved queries tools
		assert.True(t, checker.IsToolAccessible("query", state, false), "query should be accessible")
		assert.True(t, checker.IsToolAccessible("sample", state, false), "sample should be accessible")
		assert.True(t, checker.IsToolAccessible("validate", state, false), "validate should be accessible")
		assert.True(t, checker.IsToolAccessible("get_ontology", state, false), "get_ontology should be accessible")
		assert.True(t, checker.IsToolAccessible("list_glossary", state, false), "list_glossary should be accessible")
		assert.True(t, checker.IsToolAccessible("get_glossary_sql", state, false), "get_glossary_sql should be accessible")
		assert.True(t, checker.IsToolAccessible("list_approved_queries", state, false), "list_approved_queries should be accessible")
		assert.True(t, checker.IsToolAccessible("execute_approved_query", state, false), "execute_approved_query should be accessible")

		// Developer tools should NOT be accessible
		assert.False(t, checker.IsToolAccessible("echo", state, false), "echo should NOT be accessible")
		assert.False(t, checker.IsToolAccessible("execute", state, false), "execute should NOT be accessible")
		assert.False(t, checker.IsToolAccessible("get_schema", state, false), "get_schema should NOT be accessible")

		// Always tools
		assert.True(t, checker.IsToolAccessible("health", state, false), "health should be accessible")
	})

	t.Run("agent does not get approved_queries tools without agent_tools", func(t *testing.T) {
		// Agents should NOT have access when only approved_queries is enabled
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, true), "agent should not access list_approved_queries")
		assert.False(t, checker.IsToolAccessible("query", state, true), "agent should not access query")

		// Health is always accessible
		assert.True(t, checker.IsToolAccessible("health", state, true), "agent should access health")
	})
}

func TestToolAccessChecker_AgentToolsEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: false},
		ToolGroupApprovedQueries: {Enabled: false},
		ToolGroupAgentTools:      {Enabled: true},
	}

	t.Run("agent gets agent-allowed tools only", func(t *testing.T) {
		// Agent-allowed tools
		assert.True(t, checker.IsToolAccessible("echo", state, true), "agent should access echo")
		assert.True(t, checker.IsToolAccessible("list_approved_queries", state, true), "agent should access list_approved_queries")
		assert.True(t, checker.IsToolAccessible("execute_approved_query", state, true), "agent should access execute_approved_query")
		assert.True(t, checker.IsToolAccessible("health", state, true), "agent should access health")

		// Non-agent tools
		assert.False(t, checker.IsToolAccessible("query", state, true), "agent should NOT access query")
		assert.False(t, checker.IsToolAccessible("sample", state, true), "agent should NOT access sample")
		assert.False(t, checker.IsToolAccessible("execute", state, true), "agent should NOT access execute")
		assert.False(t, checker.IsToolAccessible("get_schema", state, true), "agent should NOT access get_schema")
	})

	t.Run("user does not get tools from agent_tools mode", func(t *testing.T) {
		// Users should NOT have access when only agent_tools is enabled
		assert.False(t, checker.IsToolAccessible("echo", state, false), "user should not access echo")
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, false), "user should not access list_approved_queries")
		assert.False(t, checker.IsToolAccessible("query", state, false), "user should not access query")

		// Health is always accessible
		assert.True(t, checker.IsToolAccessible("health", state, false), "user should access health")
	})
}

func TestToolAccessChecker_NoToolGroupEnabled(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		ToolGroupDeveloper:       {Enabled: false},
		ToolGroupApprovedQueries: {Enabled: false},
		ToolGroupAgentTools:      {Enabled: false},
	}

	t.Run("user only gets health", func(t *testing.T) {
		assert.True(t, checker.IsToolAccessible("health", state, false), "health should be accessible")
		assert.False(t, checker.IsToolAccessible("echo", state, false), "echo should NOT be accessible")
		assert.False(t, checker.IsToolAccessible("query", state, false), "query should NOT be accessible")
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, false), "list_approved_queries should NOT be accessible")
	})

	t.Run("agent only gets health", func(t *testing.T) {
		assert.True(t, checker.IsToolAccessible("health", state, true), "health should be accessible")
		assert.False(t, checker.IsToolAccessible("echo", state, true), "echo should NOT be accessible")
		assert.False(t, checker.IsToolAccessible("list_approved_queries", state, true), "list_approved_queries should NOT be accessible")
	})
}

func TestToolAccessChecker_GetAccessibleTools(t *testing.T) {
	checker := NewToolAccessChecker()

	t.Run("developer mode returns all tools for user", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupDeveloper: {Enabled: true, EnableExecute: true},
		}

		tools := checker.GetAccessibleTools(state, false)

		// Should have all tools from registry
		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		// Verify key tools are present
		assert.True(t, toolNames["health"], "should have health")
		assert.True(t, toolNames["echo"], "should have echo")
		assert.True(t, toolNames["execute"], "should have execute")
		assert.True(t, toolNames["get_schema"], "should have get_schema")
		assert.True(t, toolNames["query"], "should have query")
		assert.True(t, toolNames["list_approved_queries"], "should have list_approved_queries")
	})

	t.Run("approved_queries mode returns limited tools for user", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupApprovedQueries: {Enabled: true},
		}

		tools := checker.GetAccessibleTools(state, false)

		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		// Should have approved_queries tools
		assert.True(t, toolNames["health"], "should have health")
		assert.True(t, toolNames["query"], "should have query")
		assert.True(t, toolNames["list_approved_queries"], "should have list_approved_queries")

		// Should NOT have developer tools
		assert.False(t, toolNames["echo"], "should NOT have echo")
		assert.False(t, toolNames["execute"], "should NOT have execute")
		assert.False(t, toolNames["get_schema"], "should NOT have get_schema")
	})

	t.Run("agent_tools mode returns agent tools for agent", func(t *testing.T) {
		state := map[string]*models.ToolGroupConfig{
			ToolGroupAgentTools: {Enabled: true},
		}

		tools := checker.GetAccessibleTools(state, true)

		toolNames := make(map[string]bool)
		for _, tool := range tools {
			toolNames[tool.Name] = true
		}

		// Should have agent-allowed tools
		assert.True(t, toolNames["health"], "should have health")
		assert.True(t, toolNames["echo"], "should have echo")
		assert.True(t, toolNames["list_approved_queries"], "should have list_approved_queries")
		assert.True(t, toolNames["execute_approved_query"], "should have execute_approved_query")

		// Should NOT have non-agent tools
		assert.False(t, toolNames["query"], "should NOT have query")
		assert.False(t, toolNames["execute"], "should NOT have execute")
		assert.False(t, toolNames["get_schema"], "should NOT have get_schema")
	})

	t.Run("nil state returns only health", func(t *testing.T) {
		tools := checker.GetAccessibleTools(nil, false)

		require.Len(t, tools, 1)
		assert.Equal(t, "health", tools[0].Name)
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
			name: "developer mode user",
			state: map[string]*models.ToolGroupConfig{
				ToolGroupDeveloper: {Enabled: true, EnableExecute: true},
			},
			isAgent: false,
		},
		{
			name: "approved_queries mode user",
			state: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
			isAgent: false,
		},
		{
			name: "agent_tools mode agent",
			state: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: true},
			},
			isAgent: true,
		},
		{
			name: "no tools enabled user",
			state: map[string]*models.ToolGroupConfig{
				ToolGroupDeveloper:       {Enabled: false},
				ToolGroupApprovedQueries: {Enabled: false},
				ToolGroupAgentTools:      {Enabled: false},
			},
			isAgent: false,
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

			// Verify tools NOT in the list are NOT accessible
			for _, regTool := range ToolRegistry {
				inList := false
				for _, tool := range tools {
					if tool.Name == regTool.Name {
						inList = true
						break
					}
				}

				if !inList {
					accessible := checker.IsToolAccessible(regTool.Name, tc.state, tc.isAgent)
					assert.False(t, accessible, "tool %q is not listed but IsToolAccessible returns true", regTool.Name)
				}
			}
		})
	}
}
