package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/stretchr/testify/assert"
)

func toolNamesToMap(tools []ToolSpec) map[string]bool {
	result := make(map[string]bool, len(tools))
	for _, tool := range tools {
		result[tool.Name] = true
	}
	return result
}

func TestComputeUserTools_UsesUserToggles(t *testing.T) {
	tools := ComputeUserTools(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddOntologySuggestions: true,
			AddRequestTools:        true,
		},
	})

	toolNames := toolNamesToMap(tools)
	assert.True(t, toolNames["health"])
	assert.True(t, toolNames["get_context"])
	assert.True(t, toolNames["get_ontology"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.True(t, toolNames["query"])
	assert.False(t, toolNames["echo"])
	assert.False(t, toolNames["get_schema"])
}

func TestComputeDeveloperTools_UsesDeveloperToggles(t *testing.T) {
	tools := ComputeDeveloperTools(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddApprovalTools:            true,
		},
	})

	toolNames := toolNamesToMap(tools)
	assert.True(t, toolNames["health"])
	assert.True(t, toolNames["echo"])
	assert.True(t, toolNames["execute"])
	assert.True(t, toolNames["query"])
	assert.True(t, toolNames["get_schema"])
	assert.True(t, toolNames["update_table"])
	assert.True(t, toolNames["list_query_suggestions"])
	assert.False(t, toolNames["get_context"])
	assert.False(t, toolNames["list_approved_queries"])
}

func TestComputeEnabledToolsFromConfig_AgentUsesAgentToolsOnly(t *testing.T) {
	tools := ComputeEnabledToolsFromConfig(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddApprovalTools:            true,
			AddOntologySuggestions:      true,
			AddRequestTools:             true,
		},
		ToolGroupAgentTools: {Enabled: true},
	}, true)

	toolNames := toolNamesToMap(tools)
	assert.Equal(t, 3, len(tools))
	assert.True(t, toolNames["health"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.True(t, toolNames["execute_approved_query"])
	assert.False(t, toolNames["query"])
	assert.False(t, toolNames["echo"])
}

func TestComputeEnabledToolsFromConfig_MergesUserAndDeveloperLoadouts(t *testing.T) {
	tools := ComputeEnabledToolsFromConfig(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddDirectDatabaseAccess: true,
			AddRequestTools:         true,
		},
	}, false)

	toolNames := toolNamesToMap(tools)
	assert.True(t, toolNames["health"])
	assert.True(t, toolNames["echo"])
	assert.True(t, toolNames["execute"])
	assert.True(t, toolNames["query"])
	assert.True(t, toolNames["list_approved_queries"])
}
