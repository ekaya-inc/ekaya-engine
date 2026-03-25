package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolAccessChecker_IsToolAccessible_NonAgentUsesUnionOfEnabledToggles(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddDirectDatabaseAccess: true,
			AddRequestTools:         true,
		},
	}

	assert.True(t, checker.IsToolAccessible("health", state, false))
	assert.True(t, checker.IsToolAccessible("echo", state, false))
	assert.True(t, checker.IsToolAccessible("query", state, false))
	assert.True(t, checker.IsToolAccessible("list_approved_queries", state, false))
	assert.False(t, checker.IsToolAccessible("get_schema", state, false))
}

func TestToolAccessChecker_IsToolAccessible_AgentUsesLimitedLoadout(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddRequestTools:             true,
		},
		ToolGroupAgentTools: {Enabled: true},
	}

	assert.True(t, checker.IsToolAccessible("health", state, true))
	assert.True(t, checker.IsToolAccessible("list_approved_queries", state, true))
	assert.True(t, checker.IsToolAccessible("execute_approved_query", state, true))
	assert.False(t, checker.IsToolAccessible("query", state, true))
	assert.False(t, checker.IsToolAccessible("echo", state, true))
}

func TestToolAccessChecker_IgnoresUnsupportedGroups(t *testing.T) {
	checker := NewToolAccessChecker()

	state := map[string]*models.ToolGroupConfig{
		"developer": {Enabled: true},
		"user":      {Enabled: true},
	}

	assert.True(t, checker.IsToolAccessible("health", state, false))
	assert.False(t, checker.IsToolAccessible("echo", state, false))
	assert.False(t, checker.IsToolAccessible("list_approved_queries", state, false))
}

func TestToolAccessChecker_ListAndCallConsistency(t *testing.T) {
	checker := NewToolAccessChecker()

	testCases := []struct {
		name    string
		state   map[string]*models.ToolGroupConfig
		isAgent bool
	}{
		{
			name: "developer and user toggles",
			state: map[string]*models.ToolGroupConfig{
				ToolGroupTools: {
					AddDirectDatabaseAccess:     true,
					AddOntologyMaintenanceTools: true,
					AddRequestTools:             true,
				},
			},
		},
		{
			name: "agent tools",
			state: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: true},
			},
			isAgent: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tools := checker.GetAccessibleTools(tc.state, tc.isAgent)
			require.NotEmpty(t, tools)

			for _, tool := range tools {
				assert.True(t, checker.IsToolAccessible(tool.Name, tc.state, tc.isAgent))
			}
		})
	}
}
