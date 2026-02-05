package tools

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/stretchr/testify/assert"
)

func TestComputeToolsForRole_Agent(t *testing.T) {
	// computeToolsForRole uses ComputeEnabledToolsFromConfig - for agents,
	// only agent_tools.Enabled controls access to Limited Query tools.
	claims := &auth.Claims{
		ProjectID: "test-project",
	}
	claims.Subject = "agent"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupAgentTools: {Enabled: true}, // Agent tools enabled
		services.ToolGroupDeveloper:  {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Agent should only get Limited Query tools (health, list_approved_queries, execute_approved_query)
	assert.True(t, toolNames["health"], "agent should have health")
	assert.True(t, toolNames["list_approved_queries"], "agent should have list_approved_queries")
	assert.True(t, toolNames["execute_approved_query"], "agent should have execute_approved_query")

	// Agent should NOT get developer or user tools
	assert.False(t, toolNames["echo"], "agent should NOT have echo")
	assert.False(t, toolNames["execute"], "agent should NOT have execute")
	assert.False(t, toolNames["query"], "agent should NOT have query")
	assert.False(t, toolNames["get_schema"], "agent should NOT have get_schema")
}

func TestComputeToolsForRole_AdminRole(t *testing.T) {
	// computeToolsForRole uses ComputeEnabledToolsFromConfig - roles don't matter,
	// only config state determines available tools for non-agent users.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleAdmin},
	}
	claims.Subject = "user-123"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupDeveloper: {AddQueryTools: true, AddOntologyMaintenance: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Non-agent users get Default + DeveloperCore + sub-options from config
	assert.True(t, toolNames["health"], "admin should have health")
	assert.True(t, toolNames["echo"], "admin should have echo")
	assert.True(t, toolNames["execute"], "admin should have execute")
	assert.True(t, toolNames["query"], "admin should have query")
	assert.True(t, toolNames["get_schema"], "admin should have get_schema")
	assert.True(t, toolNames["list_ontology_questions"], "admin should have list_ontology_questions")
}

func TestComputeToolsForRole_DataRole(t *testing.T) {
	// computeToolsForRole uses ComputeEnabledToolsFromConfig - roles don't matter,
	// only config state determines available tools for non-agent users.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleData},
	}
	claims.Subject = "user-456"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupDeveloper: {AddQueryTools: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Non-agent users get Default + DeveloperCore + sub-options from config
	assert.True(t, toolNames["health"], "data role should have health")
	assert.True(t, toolNames["echo"], "data role should have echo")
	assert.True(t, toolNames["execute"], "data role should have execute")
	assert.True(t, toolNames["query"], "data role should have query")

	// Ontology maintenance NOT included without AddOntologyMaintenance
	assert.False(t, toolNames["refresh_schema"], "data role should NOT have refresh_schema without option")
}

func TestComputeToolsForRole_DeveloperRole(t *testing.T) {
	// computeToolsForRole uses ComputeEnabledToolsFromConfig - roles don't matter,
	// only config state determines available tools for non-agent users.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{"developer"}, // developer role string
	}
	claims.Subject = "user-789"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupDeveloper: {AddQueryTools: true, AddOntologyMaintenance: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Non-agent users get Default + DeveloperCore + sub-options from config
	assert.True(t, toolNames["health"], "developer role should have health")
	assert.True(t, toolNames["echo"], "developer role should have echo")
	assert.True(t, toolNames["execute"], "developer role should have execute")
	assert.True(t, toolNames["query"], "developer role should have query")
	assert.True(t, toolNames["refresh_schema"], "developer role should have refresh_schema")
}

func TestComputeToolsForRole_UserRole(t *testing.T) {
	// computeToolsForRole now uses ComputeEnabledToolsFromConfig for consistency with
	// NewToolFilter's listing behavior. This means it ignores the role and uses config state.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-regular"

	// No AddQueryTools, so no query tools
	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupUser: {AllowOntologyMaintenance: false},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// For non-agent users, computeToolsForRole now delegates to ComputeEnabledToolsFromConfig
	// which provides Default + DeveloperCore loadouts, plus sub-options from developer config.
	assert.True(t, toolNames["health"], "user should have health")
	assert.True(t, toolNames["echo"], "user should have echo (DeveloperCore)")
	assert.True(t, toolNames["execute"], "user should have execute (DeveloperCore)")

	// Query tools are NOT included without AddQueryTools
	assert.False(t, toolNames["query"], "user should NOT have query without AddQueryTools")
	assert.False(t, toolNames["sample"], "user should NOT have sample without AddQueryTools")
	assert.False(t, toolNames["list_approved_queries"], "user should NOT have list_approved_queries without AddQueryTools")
}

func TestComputeToolsForRole_UserWithOntologyMaintenance(t *testing.T) {
	// computeToolsForRole uses ComputeEnabledToolsFromConfig which checks developer config,
	// not user config. The ToolGroupUser.AllowOntologyMaintenance is for ComputeUserTools
	// which is no longer used by computeToolsForRole.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-regular"

	// To get ontology maintenance for non-agent users, we need developer config
	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupDeveloper: {AddQueryTools: true, AddOntologyMaintenance: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Non-agent users get Default + DeveloperCore + sub-options from developer config
	assert.True(t, toolNames["health"], "user should have health")
	assert.True(t, toolNames["echo"], "user should have echo (DeveloperCore)")
	assert.True(t, toolNames["execute"], "user should have execute (DeveloperCore)")
	assert.True(t, toolNames["query"], "user should have query with AddQueryTools")
	assert.True(t, toolNames["refresh_schema"], "user should have refresh_schema with AddOntologyMaintenance")
	assert.True(t, toolNames["update_column"], "user should have update_column with AddOntologyMaintenance")
}

func TestComputeToolsForRole_NoRoles(t *testing.T) {
	// computeToolsForRole uses ComputeEnabledToolsFromConfig which provides
	// Default + DeveloperCore for all non-agent users regardless of roles.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{}, // No roles
	}
	claims.Subject = "user-no-roles"

	state := map[string]*models.ToolGroupConfig{}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Non-agent users get Default + DeveloperCore regardless of roles
	assert.True(t, toolNames["health"], "should have health")
	assert.True(t, toolNames["echo"], "should have echo (DeveloperCore)")
	assert.True(t, toolNames["execute"], "should have execute (DeveloperCore)")

	// No query tools without AddQueryTools
	assert.False(t, toolNames["query"], "should NOT have query without AddQueryTools")
}

func TestComputeToolsForRole_MultipleRolesIncludingAdmin(t *testing.T) {
	// computeToolsForRole uses ComputeEnabledToolsFromConfig which no longer
	// checks roles - all non-agent users get the same tool set based on config.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser, models.RoleAdmin}, // Both user and admin
	}
	claims.Subject = "user-multi-role"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupDeveloper: {AddQueryTools: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// All non-agent users get DeveloperCore (echo, execute)
	assert.True(t, toolNames["echo"], "should have echo (DeveloperCore)")
	assert.True(t, toolNames["execute"], "should have execute (DeveloperCore)")
	assert.True(t, toolNames["query"], "should have query with AddQueryTools")
}

func TestIsToolInList(t *testing.T) {
	tools := []services.ToolSpec{
		{Name: "health", Description: "Health check"},
		{Name: "query", Description: "Execute query"},
		{Name: "sample", Description: "Sample data"},
	}

	assert.True(t, isToolInList("health", tools), "health should be in list")
	assert.True(t, isToolInList("query", tools), "query should be in list")
	assert.True(t, isToolInList("sample", tools), "sample should be in list")
	assert.False(t, isToolInList("echo", tools), "echo should NOT be in list")
	assert.False(t, isToolInList("execute", tools), "execute should NOT be in list")
}

// TestToolAccessError verifies that ToolAccessError is created correctly
// and can be converted to MCP results using AsToolAccessResult.
func TestToolAccessError(t *testing.T) {
	t.Run("newToolAccessError creates correct structure", func(t *testing.T) {
		err := newToolAccessError("authentication_required", "authentication required")

		assert.Equal(t, "authentication_required", err.Code)
		assert.Equal(t, "authentication required", err.Message)
		assert.NotNil(t, err.MCPResult)
		assert.Equal(t, "authentication required", err.Error())
	})

	t.Run("AsToolAccessResult extracts MCPResult from ToolAccessError", func(t *testing.T) {
		err := newToolAccessError("tool_not_enabled", "query tool is not enabled")

		result := AsToolAccessResult(err)

		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("AsToolAccessResult returns nil for regular errors", func(t *testing.T) {
		err := assert.AnError

		result := AsToolAccessResult(err)

		assert.Nil(t, result, "regular errors should not convert to tool access results")
	})

	t.Run("AsToolAccessResult returns nil for nil", func(t *testing.T) {
		result := AsToolAccessResult(nil)

		assert.Nil(t, result)
	})
}

// toolSpecNamesToMap converts a slice of ToolSpec to a map for easy lookup.
func toolSpecNamesToMap(tools []services.ToolSpec) map[string]bool {
	result := make(map[string]bool)
	for _, tool := range tools {
		result[tool.Name] = true
	}
	return result
}
