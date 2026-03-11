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
		services.ToolGroupAgentTools: {Enabled: true},
		services.ToolGroupDeveloper:  {Enabled: true, AddQueryTools: true, AddOntologyMaintenance: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Agent should only get Limited Query tools
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
	// Admin role gets full developer tools based on config
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

	assert.True(t, toolNames["health"], "admin should have health")
	assert.True(t, toolNames["echo"], "admin should have echo")
	assert.True(t, toolNames["execute"], "admin should have execute")
	assert.True(t, toolNames["query"], "admin should have query")
	assert.True(t, toolNames["get_schema"], "admin should have get_schema")
	assert.True(t, toolNames["list_ontology_questions"], "admin should have list_ontology_questions")
}

func TestComputeToolsForRole_DataRole(t *testing.T) {
	// Data role gets full developer tools based on config (same as admin for MCP)
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

	assert.True(t, toolNames["health"], "data role should have health")
	assert.True(t, toolNames["echo"], "data role should have echo")
	assert.True(t, toolNames["execute"], "data role should have execute")
	assert.True(t, toolNames["query"], "data role should have query")

	// Ontology maintenance NOT included without AddOntologyMaintenance
	assert.False(t, toolNames["refresh_schema"], "data role should NOT have refresh_schema without option")
}

func TestComputeToolsForRole_UserRole(t *testing.T) {
	// User role gets tools based on user-role toggles only.
	// Developer toggles do NOT affect user tools.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-regular"

	// Only developer toggles set — user-role toggles are not set
	state := map[string]*models.ToolGroupConfig{
		"tools": {AddDirectDatabaseAccess: true, AddOntologyMaintenanceTools: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// User gets only health when no user-role toggles are enabled
	assert.True(t, toolNames["health"], "user should have health")

	// User should NOT get developer tools regardless of developer toggles
	assert.False(t, toolNames["echo"], "user should NOT have echo")
	assert.False(t, toolNames["execute"], "user should NOT have execute")
	assert.False(t, toolNames["query"], "user should NOT have query")
	assert.False(t, toolNames["sample"], "user should NOT have sample")
	assert.False(t, toolNames["get_schema"], "user should NOT have get_schema")
	assert.False(t, toolNames["refresh_schema"], "user should NOT have refresh_schema")
	assert.False(t, toolNames["update_column"], "user should NOT have update_column")
	assert.False(t, toolNames["list_approved_queries"], "user should NOT have list_approved_queries without AddRequestTools")
	assert.False(t, toolNames["execute_approved_query"], "user should NOT have execute_approved_query without AddRequestTools")
}

func TestComputeToolsForRole_UserRole_IgnoresConfig(t *testing.T) {
	// User role ignores developer config; only user-role toggles matter.
	// Legacy ToolGroupUser.AllowOntologyMaintenance maps to AddOntologySuggestions + AddRequestTools.
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-regular"

	state := map[string]*models.ToolGroupConfig{
		"tools": {
			AddDirectDatabaseAccess:     true, // developer toggle — ignored for user
			AddOntologyMaintenanceTools: true, // developer toggle — ignored for user
			AddOntologySuggestions:      true, // user toggle
			AddRequestTools:             true, // user toggle
		},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// User gets tools from user-role toggles
	assert.True(t, toolNames["health"], "user should have health")
	assert.True(t, toolNames["list_approved_queries"], "user should have list_approved_queries")
	assert.True(t, toolNames["execute_approved_query"], "user should have execute_approved_query")
	assert.True(t, toolNames["get_context"], "user should have get_context from AddOntologySuggestions")
	assert.True(t, toolNames["get_ontology"], "user should have get_ontology from AddOntologySuggestions")

	// Developer tools NOT accessible to user regardless of developer toggles
	assert.False(t, toolNames["echo"], "user should NOT have echo")
	assert.False(t, toolNames["execute"], "user should NOT have execute")
	assert.False(t, toolNames["get_schema"], "user should NOT have get_schema")
}

func TestComputeToolsForRole_NoRoles(t *testing.T) {
	// No roles defaults to user (least privilege) — only health without user-role toggles
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{},
	}
	claims.Subject = "user-no-roles"

	state := map[string]*models.ToolGroupConfig{
		"tools": {AddDirectDatabaseAccess: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Defaults to user-level tools (least privilege) — only health without user toggles
	assert.True(t, toolNames["health"], "should have health")

	// Should NOT get developer tools
	assert.False(t, toolNames["echo"], "should NOT have echo")
	assert.False(t, toolNames["execute"], "should NOT have execute")
	assert.False(t, toolNames["query"], "should NOT have query")
	assert.False(t, toolNames["list_approved_queries"], "should NOT have list_approved_queries without AddRequestTools")
}

func TestComputeToolsForRole_MultipleRolesUserAndAdmin(t *testing.T) {
	// effectiveRole uses highest privilege — admin > data > user
	// Even though "user" is first, "admin" is highest privilege
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser, models.RoleAdmin},
	}
	claims.Subject = "user-multi-role"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupDeveloper: {AddQueryTools: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Highest role is "admin", so full developer access
	assert.True(t, toolNames["health"], "should have health")
	assert.True(t, toolNames["echo"], "should have echo (admin is highest)")
	assert.True(t, toolNames["execute"], "should have execute (admin is highest)")
	assert.True(t, toolNames["query"], "should have query (admin is highest)")
}

func TestComputeToolsForRole_MultipleRolesDataAndUser(t *testing.T) {
	// effectiveRole uses highest privilege — data > user
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser, models.RoleData},
	}
	claims.Subject = "user-multi-role"

	state := map[string]*models.ToolGroupConfig{
		services.ToolGroupDeveloper: {AddQueryTools: true},
	}

	tools := computeToolsForRole(claims, state)
	toolNames := toolSpecNamesToMap(tools)

	// Highest role is "data", so full developer access
	assert.True(t, toolNames["health"], "should have health")
	assert.True(t, toolNames["echo"], "should have echo (data is highest)")
	assert.True(t, toolNames["execute"], "should have execute (data is highest)")
	assert.True(t, toolNames["query"], "should have query (data is highest)")
}

func TestEffectiveRole(t *testing.T) {
	tests := []struct {
		name     string
		roles    []string
		expected string
	}{
		{"admin role", []string{models.RoleAdmin}, models.RoleAdmin},
		{"data role", []string{models.RoleData}, models.RoleData},
		{"user role", []string{models.RoleUser}, models.RoleUser},
		{"empty roles defaults to user", []string{}, models.RoleUser},
		{"nil roles defaults to user", nil, models.RoleUser},
		{"highest privilege wins: data+admin", []string{models.RoleData, models.RoleAdmin}, models.RoleAdmin},
		{"highest privilege wins: user+admin", []string{models.RoleUser, models.RoleAdmin}, models.RoleAdmin},
		{"highest privilege wins: user+data", []string{models.RoleUser, models.RoleData}, models.RoleData},
		{"unknown role ignored, falls back to user", []string{"viewer"}, models.RoleUser},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &auth.Claims{Roles: tt.roles}
			result := effectiveRole(claims)
			assert.Equal(t, tt.expected, result)
		})
	}
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

// =============================================================================
// Security Boundary: User Role Allow-List
// =============================================================================

// TestSecurityBoundary_UserToolsAllowList is a security gate that verifies:
// 1. User role NEVER gets developer-only tools regardless of developer toggles
// 2. User role tools are determined solely by user-role toggles
// 3. With no user-role toggles, user gets only health
func TestSecurityBoundary_UserToolsAllowList(t *testing.T) {
	// Developer-only tools that must NEVER appear for role:user
	developerOnlyTools := []string{
		"echo", "execute", "get_schema", "search_schema", "probe_column",
		"update_column", "update_columns", "update_table",
		"refresh_schema", "list_ontology_questions", "resolve_ontology_question",
		"list_query_suggestions", "approve_query_suggestion", "create_approved_query",
		"list_pending_changes", "approve_change", "reject_change",
	}

	// Config permutations that enable developer toggles. User must NEVER get developer tools.
	configs := []struct {
		name  string
		state map[string]*models.ToolGroupConfig
	}{
		{"nil state", nil},
		{"empty state", map[string]*models.ToolGroupConfig{}},
		{"all developer flags on", map[string]*models.ToolGroupConfig{
			"tools": {AddDirectDatabaseAccess: true, AddOntologyMaintenanceTools: true, AddApprovalTools: true},
		}},
		{"agent tools on", map[string]*models.ToolGroupConfig{
			services.ToolGroupAgentTools: {Enabled: true},
		}},
		{"developer only toggles", map[string]*models.ToolGroupConfig{
			"tools":                      {AddDirectDatabaseAccess: true, AddOntologyMaintenanceTools: true, AddApprovalTools: true},
			services.ToolGroupAgentTools: {Enabled: true},
		}},
	}

	// Role variants that should all resolve to user-level access
	roleVariants := []struct {
		name  string
		roles []string
	}{
		{"explicit user role", []string{models.RoleUser}},
		{"empty roles", []string{}},
		{"nil roles", nil},
		{"unknown role", []string{"viewer"}},
	}

	for _, cfg := range configs {
		for _, rv := range roleVariants {
			name := cfg.name + "/" + rv.name
			t.Run(name, func(t *testing.T) {
				claims := &auth.Claims{
					ProjectID: "test-project",
					Roles:     rv.roles,
				}
				claims.Subject = "user-boundary-test"

				tools := computeToolsForRole(claims, cfg.state)
				actual := toolSpecNamesToMap(tools)

				// Health is always present
				assert.True(t, actual["health"],
					"SECURITY: health should always be available to user role")

				// Developer-only tools must NEVER be available
				for _, devTool := range developerOnlyTools {
					assert.False(t, actual[devTool],
						"SECURITY: developer tool %q must NOT be available to user role", devTool)
				}
			})
		}
	}

	// Verify that with no user-role toggles, user gets ONLY health
	t.Run("no user toggles gives only health", func(t *testing.T) {
		claims := &auth.Claims{
			ProjectID: "test-project",
			Roles:     []string{models.RoleUser},
		}
		claims.Subject = "user-boundary-test"

		state := map[string]*models.ToolGroupConfig{
			"tools": {AddDirectDatabaseAccess: true, AddOntologyMaintenanceTools: true, AddApprovalTools: true},
		}

		tools := computeToolsForRole(claims, state)
		assert.Len(t, tools, 1, "user with no user-role toggles should get only health")
		assert.Equal(t, "health", tools[0].Name)
	})

	// Verify that user-role toggles correctly add user tools
	t.Run("user toggles add user tools", func(t *testing.T) {
		claims := &auth.Claims{
			ProjectID: "test-project",
			Roles:     []string{models.RoleUser},
		}
		claims.Subject = "user-boundary-test"

		state := map[string]*models.ToolGroupConfig{
			"tools": {AddOntologySuggestions: true, AddRequestTools: true},
		}

		tools := computeToolsForRole(claims, state)
		actual := toolSpecNamesToMap(tools)

		assert.True(t, actual["health"], "should have health")
		assert.True(t, actual["get_context"], "should have get_context from AddOntologySuggestions")
		assert.True(t, actual["list_approved_queries"], "should have list_approved_queries from AddRequestTools")
		assert.True(t, actual["execute_approved_query"], "should have execute_approved_query from AddRequestTools")

		// Still no developer tools
		for _, devTool := range developerOnlyTools {
			assert.False(t, actual[devTool],
				"SECURITY: developer tool %q must NOT be available to user role", devTool)
		}
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
