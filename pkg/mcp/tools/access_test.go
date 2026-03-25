package tools

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

func toolSpecNamesToMap(tools []services.ToolSpec) map[string]bool {
	result := make(map[string]bool, len(tools))
	for _, tool := range tools {
		result[tool.Name] = true
	}
	return result
}

func TestComputeToolsForRole_AgentUsesLimitedQueryLoadout(t *testing.T) {
	claims := &auth.Claims{ProjectID: "test-project"}
	claims.Subject = "agent:" + uuid.New().String()

	tools := computeToolsForRole(claims, map[string]*models.ToolGroupConfig{
		services.ToolGroupTools: {
			AddDirectDatabaseAccess:     true,
			AddOntologyMaintenanceTools: true,
			AddRequestTools:             true,
		},
		services.ToolGroupAgentTools: {Enabled: true},
	})

	toolNames := toolSpecNamesToMap(tools)
	assert.True(t, toolNames["health"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.True(t, toolNames["execute_approved_query"])
	assert.False(t, toolNames["query"])
	assert.False(t, toolNames["echo"])
}

func TestComputeToolsForRole_UserUsesOnlyUserToggles(t *testing.T) {
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleUser},
	}
	claims.Subject = "user-123"

	tools := computeToolsForRole(claims, map[string]*models.ToolGroupConfig{
		services.ToolGroupTools: {
			AddDirectDatabaseAccess: true,
			AddApprovalTools:        true,
			AddOntologySuggestions:  true,
			AddRequestTools:         true,
		},
	})

	toolNames := toolSpecNamesToMap(tools)
	assert.True(t, toolNames["health"])
	assert.True(t, toolNames["query"])
	assert.True(t, toolNames["validate"])
	assert.True(t, toolNames["sample"])
	assert.True(t, toolNames["get_context"])
	assert.True(t, toolNames["get_schema"])
	assert.True(t, toolNames["probe_column"])
	assert.True(t, toolNames["list_project_knowledge"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.False(t, toolNames["echo"])
	assert.False(t, toolNames["list_query_suggestions"])
}

func TestComputeToolsForRole_AdminGetsUnionOfEnabledToggles(t *testing.T) {
	claims := &auth.Claims{
		ProjectID: "test-project",
		Roles:     []string{models.RoleAdmin},
	}
	claims.Subject = "user-456"

	tools := computeToolsForRole(claims, map[string]*models.ToolGroupConfig{
		services.ToolGroupTools: {
			AddDirectDatabaseAccess: true,
			AddOntologySuggestions:  true,
		},
	})

	toolNames := toolSpecNamesToMap(tools)
	assert.True(t, toolNames["echo"])
	assert.True(t, toolNames["query"])
	assert.True(t, toolNames["validate"])
	assert.True(t, toolNames["sample"])
	assert.True(t, toolNames["list_approved_queries"])
	assert.True(t, toolNames["get_schema"])
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
		{"highest privilege wins", []string{models.RoleUser, models.RoleAdmin}, models.RoleAdmin},
		{"unknown role falls back to user", []string{"viewer"}, models.RoleUser},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, effectiveRole(&auth.Claims{Roles: tt.roles}))
		})
	}
}

func TestToolAccessError(t *testing.T) {
	t.Run("newToolAccessError creates correct structure", func(t *testing.T) {
		err := newToolAccessError("authentication_required", "authentication required")

		assert.Equal(t, "authentication_required", err.Code)
		assert.Equal(t, "authentication required", err.Message)
		assert.NotNil(t, err.MCPResult)
		assert.Equal(t, "authentication required", err.Error())
	})

	t.Run("AsToolAccessResult extracts MCPResult from ToolAccessError", func(t *testing.T) {
		result := AsToolAccessResult(newToolAccessError("tool_not_enabled", "query tool is not enabled"))
		assert.NotNil(t, result)
		assert.True(t, result.IsError)
	})

	t.Run("AsToolAccessResult returns nil for regular errors", func(t *testing.T) {
		assert.Nil(t, AsToolAccessResult(assert.AnError))
		assert.Nil(t, AsToolAccessResult(nil))
	})
}
