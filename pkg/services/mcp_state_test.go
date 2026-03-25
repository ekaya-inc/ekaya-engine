package services

import (
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMCPStateValidator_RejectsUnknownToolGroups(t *testing.T) {
	validator := NewMCPStateValidator()

	result := validator.Apply(
		MCPStateTransition{
			Current: map[string]*models.ToolGroupConfig{},
			Update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
		},
		MCPStateContext{},
	)

	require.NotNil(t, result.Error)
	assert.Equal(t, ErrCodeUnknownToolGroup, result.Error.Code)
}

func TestMCPStateValidator_DropsLegacyGroupsFromState(t *testing.T) {
	validator := NewMCPStateValidator()

	result := validator.Apply(
		MCPStateTransition{
			Current: map[string]*models.ToolGroupConfig{
				ToolGroupTools:      {AddRequestTools: true},
				ToolGroupAgentTools: {Enabled: true},
				"developer":         {Enabled: true},
				"user":              {Enabled: true},
				"approved_queries":  {Enabled: true},
			},
			Update: map[string]*models.ToolGroupConfig{},
		},
		MCPStateContext{},
	)

	require.Nil(t, result.Error)
	assert.Len(t, result.State, 2)
	assert.Contains(t, result.State, ToolGroupTools)
	assert.Contains(t, result.State, ToolGroupAgentTools)
	assert.NotContains(t, result.State, "developer")
	assert.NotContains(t, result.State, "user")
	assert.NotContains(t, result.State, "approved_queries")
}

func TestMCPStateValidator_UpdatesToolsGroup(t *testing.T) {
	validator := NewMCPStateValidator()

	result := validator.Apply(
		MCPStateTransition{
			Current: map[string]*models.ToolGroupConfig{
				ToolGroupTools: {
					AddRequestTools: true,
				},
				ToolGroupAgentTools: {Enabled: true},
			},
			Update: map[string]*models.ToolGroupConfig{
				ToolGroupTools: {
					AddDirectDatabaseAccess: true,
					AddRequestTools:         false,
				},
			},
		},
		MCPStateContext{},
	)

	require.Nil(t, result.Error)
	assert.True(t, result.State[ToolGroupTools].AddDirectDatabaseAccess)
	assert.False(t, result.State[ToolGroupTools].AddRequestTools)
	assert.True(t, result.State[ToolGroupAgentTools].Enabled)

	enabledToolNames := extractToolNames(result.EnabledTools)
	assert.Contains(t, enabledToolNames, "echo")
	assert.NotContains(t, enabledToolNames, "list_approved_queries")
}

func TestMCPStateValidator_UpdatesAgentToolsEnabled(t *testing.T) {
	validator := NewMCPStateValidator()

	result := validator.Apply(
		MCPStateTransition{
			Current: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: true},
			},
			Update: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: false},
			},
		},
		MCPStateContext{},
	)

	require.Nil(t, result.Error)
	assert.False(t, result.State[ToolGroupAgentTools].Enabled)
}
