package tools

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/stretchr/testify/assert"
)

func TestComputeToolsForRole_Agent(t *testing.T) {
	claims := &auth.Claims{
		ProjectID: "test-project",
	}
	claims.Subject = "agent"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupDeveloper: {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
		services.ToolGroupUser:      {Enabled: true, AllowOntologyMaintenance: true},
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

	// Admin gets Developer Tools
	assert.True(t, toolNames["health"], "admin should have health")
	assert.True(t, toolNames["echo"], "admin should have echo")
	assert.True(t, toolNames["execute"], "admin should have execute")
	assert.True(t, toolNames["query"], "admin should have query")
	assert.True(t, toolNames["get_schema"], "admin should have get_schema")
	assert.True(t, toolNames["list_ontology_questions"], "admin should have list_ontology_questions")
}

func TestComputeToolsForRole_DataRole(t *testing.T) {
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

	// Data role gets Developer Tools
	assert.True(t, toolNames["health"], "data role should have health")
	assert.True(t, toolNames["echo"], "data role should have echo")
	assert.True(t, toolNames["execute"], "data role should have execute")
	assert.True(t, toolNames["query"], "data role should have query")

	// Ontology maintenance NOT included without AddOntologyMaintenance
	assert.False(t, toolNames["refresh_schema"], "data role should NOT have refresh_schema without option")
}

func TestComputeToolsForRole_DeveloperRole(t *testing.T) {
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

	// Developer role gets Developer Tools
	assert.True(t, toolNames["health"], "developer role should have health")
	assert.True(t, toolNames["echo"], "developer role should have echo")
	assert.True(t, toolNames["execute"], "developer role should have execute")
	assert.True(t, toolNames["query"], "developer role should have query")
	assert.True(t, toolNames["refresh_schema"], "developer role should have refresh_schema")
}

func TestComputeToolsForRole_UserRole(t *testing.T) {
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-regular"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupUser: {AllowOntologyMaintenance: false},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// User gets User Tools (Query loadout by default)
	assert.True(t, toolNames["health"], "user should have health")
	assert.True(t, toolNames["query"], "user should have query")
	assert.True(t, toolNames["sample"], "user should have sample")
	assert.True(t, toolNames["get_schema"], "user should have get_schema")
	assert.True(t, toolNames["list_approved_queries"], "user should have list_approved_queries")

	// User should NOT get developer tools
	assert.False(t, toolNames["echo"], "user should NOT have echo")
	assert.False(t, toolNames["execute"], "user should NOT have execute")

	// User should NOT get ontology maintenance without option
	assert.False(t, toolNames["refresh_schema"], "user should NOT have refresh_schema without option")
}

func TestComputeToolsForRole_UserWithOntologyMaintenance(t *testing.T) {
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-regular"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupUser: {AllowOntologyMaintenance: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// User gets User Tools + Ontology Maintenance
	assert.True(t, toolNames["health"], "user should have health")
	assert.True(t, toolNames["query"], "user should have query")
	assert.True(t, toolNames["refresh_schema"], "user should have refresh_schema with option")
	assert.True(t, toolNames["update_column"], "user should have update_column with option")

	// User still should NOT get developer tools
	assert.False(t, toolNames["echo"], "user should NOT have echo")
	assert.False(t, toolNames["execute"], "user should NOT have execute")
}

func TestComputeToolsForRole_NoRoles(t *testing.T) {
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{}, // No roles
	}
	claims.Subject = "user-no-roles"

	state := map[string]*models.ToolGroupConfig{}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// No roles defaults to user behavior (Query loadout)
	assert.True(t, toolNames["health"], "should have health")
	assert.True(t, toolNames["query"], "should have query")
	assert.False(t, toolNames["echo"], "should NOT have echo")
}

func TestComputeToolsForRole_MultipleRolesIncludingAdmin(t *testing.T) {
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

	// Should get Developer Tools because admin role is present
	assert.True(t, toolNames["echo"], "should have echo due to admin role")
	assert.True(t, toolNames["execute"], "should have execute due to admin role")
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

// toolSpecNamesToMap converts a slice of ToolSpec to a map for easy lookup.
func toolSpecNamesToMap(tools []services.ToolSpec) map[string]bool {
	result := make(map[string]bool)
	for _, tool := range tools {
		result[tool.Name] = true
	}
	return result
}
