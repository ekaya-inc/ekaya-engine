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
			AddDirectDatabaseAccess: true,
			AddOntologySuggestions:  true,
			AddRequestTools:         true,
		},
	})

	toolNames := toolNamesToMap(tools)
	assert.True(t, toolNames["health"])
	assert.True(t, toolNames["query"])
	assert.True(t, toolNames["validate"])
	assert.True(t, toolNames["sample"])
	assert.True(t, toolNames["get_context"])
	assert.True(t, toolNames["get_ontology"])
	assert.True(t, toolNames["get_schema"])
	assert.True(t, toolNames["search_schema"])
	assert.True(t, toolNames["probe_column"])
	assert.True(t, toolNames["probe_columns"])
	assert.True(t, toolNames["list_project_knowledge"])
	assert.True(t, toolNames["list_relationships"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.True(t, toolNames["execute_approved_query"])
	assert.True(t, toolNames["suggest_approved_query"])
	assert.False(t, toolNames["echo"])
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
	assert.True(t, toolNames["validate"])
	assert.True(t, toolNames["sample"])
	assert.True(t, toolNames["explain_query"])
	assert.True(t, toolNames["get_schema"])
	assert.True(t, toolNames["update_table"])
	assert.True(t, toolNames["create_approved_query"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.True(t, toolNames["execute_approved_query"])
	assert.True(t, toolNames["list_query_suggestions"])
	assert.True(t, toolNames["list_glossary"])
	assert.True(t, toolNames["get_glossary_sql"])
	assert.True(t, toolNames["get_query_history"])
	assert.False(t, toolNames["get_context"])
}

func TestComputeDeveloperTools_OntologyMaintenanceIncludesApprovedQueryCoreTools(t *testing.T) {
	tools := ComputeDeveloperTools(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddOntologyMaintenanceTools: true,
		},
	})

	toolNames := toolNamesToMap(tools)
	for _, toolName := range []string{
		"create_approved_query",
		"update_approved_query",
		"delete_approved_query",
		"list_approved_queries",
		"execute_approved_query",
	} {
		assert.True(t, toolNames[toolName], "expected %s in ontology maintenance tools", toolName)
	}
	assert.False(t, toolNames["list_query_suggestions"])
	assert.False(t, toolNames["suggest_approved_query"])
}

func TestComputeUserTools_OntologySuggestionsIncludeApprovedQueryExecutionTools(t *testing.T) {
	tools := ComputeUserTools(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddOntologySuggestions: true,
		},
	})

	toolNames := toolNamesToMap(tools)
	assert.True(t, toolNames["get_context"])
	assert.True(t, toolNames["get_ontology"])
	assert.True(t, toolNames["search_schema"])
	assert.True(t, toolNames["get_schema"])
	assert.True(t, toolNames["get_column_metadata"])
	assert.True(t, toolNames["probe_column"])
	assert.True(t, toolNames["probe_columns"])
	assert.True(t, toolNames["list_project_knowledge"])
	assert.True(t, toolNames["list_relationships"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.True(t, toolNames["execute_approved_query"])
	assert.False(t, toolNames["query"])
	assert.False(t, toolNames["suggest_approved_query"])
}

func TestComputeDeveloperTools_ApprovalToolsExcludeApprovedQueryCoreTools(t *testing.T) {
	tools := ComputeDeveloperTools(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddApprovalTools: true,
		},
	})

	toolNames := toolNamesToMap(tools)
	assert.True(t, toolNames["list_query_suggestions"])
	assert.True(t, toolNames["approve_query_suggestion"])
	assert.True(t, toolNames["reject_query_suggestion"])
	assert.True(t, toolNames["list_glossary"])
	assert.True(t, toolNames["get_glossary_sql"])
	assert.True(t, toolNames["get_query_history"])
	assert.False(t, toolNames["create_approved_query"])
	assert.False(t, toolNames["list_approved_queries"])
	assert.False(t, toolNames["execute_approved_query"])
	assert.False(t, toolNames["explain_query"])
}

func TestComputeUserTools_RequestToolsExcludeApprovedQueryCoreTools(t *testing.T) {
	tools := ComputeUserTools(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddRequestTools: true,
		},
	})

	toolNames := toolNamesToMap(tools)
	assert.False(t, toolNames["query"])
	assert.False(t, toolNames["sample"])
	assert.False(t, toolNames["validate"])
	assert.True(t, toolNames["list_glossary"])
	assert.True(t, toolNames["get_glossary_sql"])
	assert.True(t, toolNames["suggest_approved_query"])
	assert.True(t, toolNames["get_query_history"])
	assert.False(t, toolNames["list_approved_queries"])
	assert.False(t, toolNames["execute_approved_query"])
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
			AddOntologySuggestions:  true,
		},
	}, false)

	toolNames := toolNamesToMap(tools)
	assert.True(t, toolNames["health"])
	assert.True(t, toolNames["echo"])
	assert.True(t, toolNames["execute"])
	assert.True(t, toolNames["query"])
	assert.True(t, toolNames["validate"])
	assert.True(t, toolNames["sample"])
	assert.True(t, toolNames["explain_query"])
	assert.True(t, toolNames["get_schema"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.True(t, toolNames["list_project_knowledge"])
}
