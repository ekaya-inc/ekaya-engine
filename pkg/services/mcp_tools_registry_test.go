package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func extractToolNames(tools []ToolDefinition) []string {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	return names
}

func TestGetEnabledTools_IgnoresLegacyGroupKeys(t *testing.T) {
	tools := GetEnabledTools(map[string]*models.ToolGroupConfig{
		"developer": {Enabled: true},
		"user":      {Enabled: true},
	})

	require.Len(t, tools, 1)
	assert.Equal(t, "health", tools[0].Name)
}

func TestGetEnabledTools_UsesSupportedToolGroups(t *testing.T) {
	tools := GetEnabledTools(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddOntologyMaintenanceTools: true,
			AddRequestTools:             true,
		},
	})

	toolNames := extractToolNames(tools)
	assert.Contains(t, toolNames, "health")
	assert.Contains(t, toolNames, "get_schema")
	assert.Contains(t, toolNames, "update_table")
	assert.Contains(t, toolNames, "suggest_approved_query")
	assert.Contains(t, toolNames, "list_glossary")
	assert.NotContains(t, toolNames, "echo")
}

func TestGetEnabledToolsForAgent_UsesAgentToolsGroup(t *testing.T) {
	tools := GetEnabledToolsForAgent(map[string]*models.ToolGroupConfig{
		ToolGroupAgentTools: {Enabled: true},
	})

	toolNames := extractToolNames(tools)
	assert.Equal(t, []string{"health", "list_approved_queries", "execute_approved_query"}, toolNames)
}

func TestGetEnabledTools_CanonicalOrder(t *testing.T) {
	tools := GetEnabledTools(map[string]*models.ToolGroupConfig{
		ToolGroupTools: {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
		},
	})

	lastIndex := -1
	for _, name := range extractToolNames(tools) {
		index := GetToolOrder(name)
		require.Greater(t, index, lastIndex)
		lastIndex = index
	}
}

func TestListQuerySuggestionsDescriptionMentionsRejected(t *testing.T) {
	var registryDescription string
	for _, tool := range ToolRegistry {
		if tool.Name == "list_query_suggestions" {
			registryDescription = tool.Description
			break
		}
	}

	require.NotEmpty(t, registryDescription)
	assert.Contains(t, registryDescription, "rejected")

	spec := GetToolSpec("list_query_suggestions")
	require.NotNil(t, spec)
	assert.Contains(t, spec.Description, "rejected")
}

func TestGlossaryToolDescriptionsStayAlignedAcrossRegistryAndLoadouts(t *testing.T) {
	expected := map[string]string{
		"list_glossary":        "List business glossary terms with definitions and SQL availability",
		"get_glossary_sql":     "Get a business term's glossary entry, including SQL when available",
		"create_glossary_term": "Create a business glossary term with optional SQL",
		"update_glossary_term": "Create or update a business glossary term with optional SQL and aliases",
	}

	registryDescriptions := make(map[string]string, len(ToolRegistry))
	for _, tool := range ToolRegistry {
		registryDescriptions[tool.Name] = tool.Description
	}

	for toolName, description := range expected {
		assert.Equal(t, description, registryDescriptions[toolName], "registry description mismatch for %s", toolName)

		spec := GetToolSpec(toolName)
		require.NotNil(t, spec)
		assert.Equal(t, description, spec.Description, "loadout description mismatch for %s", toolName)
	}
}
