package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestMCPStateValidator_NormalizeState_SubOptionsReset(t *testing.T) {
	// CRITICAL: Sub-options must be false when group is disabled
	// This prevents security issues like enableExecute=true while enabled=false
	tests := []struct {
		name     string
		current  map[string]*models.ToolGroupConfig
		update   map[string]*models.ToolGroupConfig
		expected map[string]*models.ToolGroupConfig
	}{
		{
			name: "disabling developer resets enableExecute",
			current: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true, EnableExecute: true},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: false, EnableExecute: true}, // trying to keep execute
			},
			expected: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: false, EnableExecute: false}, // must be reset
			},
		},
		{
			name: "disabling approved_queries resets all sub-options",
			current: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {
					Enabled:                true,
					ForceMode:              true,
					AllowClientSuggestions: true,
				},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: false},
			},
			expected: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {
					Enabled:                false,
					ForceMode:              false,
					AllowClientSuggestions: false,
				},
			},
		},
		{
			name: "enabling group preserves sub-options",
			current: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: false, EnableExecute: false},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true, EnableExecute: true},
			},
			expected: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true, EnableExecute: true},
			},
		},
	}

	validator := NewMCPStateValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Apply(
				MCPStateTransition{Current: tt.current, Update: tt.update},
				MCPStateContext{HasEnabledQueries: true},
			)

			require.Nil(t, result.Error)
			for groupName, expectedConfig := range tt.expected {
				actualConfig := result.State[groupName]
				require.NotNil(t, actualConfig, "group %s should exist", groupName)
				assert.Equal(t, expectedConfig.Enabled, actualConfig.Enabled,
					"group %s: Enabled mismatch", groupName)
				assert.Equal(t, expectedConfig.EnableExecute, actualConfig.EnableExecute,
					"group %s: EnableExecute mismatch", groupName)
				assert.Equal(t, expectedConfig.ForceMode, actualConfig.ForceMode,
					"group %s: ForceMode mismatch", groupName)
				assert.Equal(t, expectedConfig.AllowClientSuggestions, actualConfig.AllowClientSuggestions,
					"group %s: AllowClientSuggestions mismatch", groupName)
			}
		})
	}
}

func TestMCPStateValidator_MutualExclusivity_AgentTools(t *testing.T) {
	// Agent Tools is mutually exclusive with all other tools
	tests := []struct {
		name           string
		current        map[string]*models.ToolGroupConfig
		update         map[string]*models.ToolGroupConfig
		expectError    bool
		errorCode      string
		expectedStates map[string]bool // groupName -> expectedEnabled
	}{
		{
			name: "enabling agent_tools disables developer",
			current: map[string]*models.ToolGroupConfig{
				"developer":         {Enabled: true},
				ToolGroupAgentTools: {Enabled: false},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: true},
			},
			expectError: false,
			expectedStates: map[string]bool{
				"developer":         false,
				ToolGroupAgentTools: true,
			},
		},
		{
			name: "enabling agent_tools disables approved_queries",
			current: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
				ToolGroupAgentTools:      {Enabled: false},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: true},
			},
			expectError: false,
			expectedStates: map[string]bool{
				ToolGroupApprovedQueries: false,
				ToolGroupAgentTools:      true,
			},
		},
		{
			name: "cannot enable developer while agent_tools is active",
			current: map[string]*models.ToolGroupConfig{
				"developer":         {Enabled: false},
				ToolGroupAgentTools: {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
			expectError: true,
			errorCode:   ErrCodeAgentToolsConflict,
		},
		{
			name: "cannot enable approved_queries while agent_tools is active",
			current: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: false},
				ToolGroupAgentTools:      {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
			expectError: true,
			errorCode:   ErrCodeAgentToolsConflict,
		},
	}

	validator := NewMCPStateValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Apply(
				MCPStateTransition{Current: tt.current, Update: tt.update},
				MCPStateContext{HasEnabledQueries: true},
			)

			if tt.expectError {
				require.NotNil(t, result.Error, "expected error but got none")
				assert.Equal(t, tt.errorCode, result.Error.Code)
				// State should be unchanged on error
				for groupName, currentConfig := range tt.current {
					assert.Equal(t, currentConfig.Enabled, result.State[groupName].Enabled,
						"state should be unchanged on error for %s", groupName)
				}
			} else {
				require.Nil(t, result.Error)
				for groupName, expectedEnabled := range tt.expectedStates {
					config := result.State[groupName]
					require.NotNil(t, config, "group %s should exist", groupName)
					assert.Equal(t, expectedEnabled, config.Enabled,
						"group %s: expected enabled=%v", groupName, expectedEnabled)
				}
			}
		})
	}
}

func TestMCPStateValidator_ForceMode(t *testing.T) {
	// Force mode prevents enabling developer tools
	tests := []struct {
		name        string
		current     map[string]*models.ToolGroupConfig
		update      map[string]*models.ToolGroupConfig
		expectError bool
		errorCode   string
	}{
		{
			name: "cannot enable developer when force mode is on",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: false},
				ToolGroupApprovedQueries: {Enabled: true, ForceMode: true},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
			expectError: true,
			errorCode:   ErrCodeForceModeConflict,
		},
		{
			name: "can enable developer when force mode is off",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: false},
				ToolGroupApprovedQueries: {Enabled: true, ForceMode: false},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
			expectError: false,
		},
	}

	validator := NewMCPStateValidator()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Apply(
				MCPStateTransition{Current: tt.current, Update: tt.update},
				MCPStateContext{HasEnabledQueries: true},
			)

			if tt.expectError {
				require.NotNil(t, result.Error)
				assert.Equal(t, tt.errorCode, result.Error.Code)
			} else {
				require.Nil(t, result.Error)
			}
		})
	}
}

func TestMCPStateValidator_ApprovedQueries_RequiresEnabledQueries(t *testing.T) {
	validator := NewMCPStateValidator()

	t.Run("cannot enable without enabled queries", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{
					ToolGroupApprovedQueries: {Enabled: false},
				},
				Update: map[string]*models.ToolGroupConfig{
					ToolGroupApprovedQueries: {Enabled: true},
				},
			},
			MCPStateContext{HasEnabledQueries: false},
		)

		require.NotNil(t, result.Error)
		assert.Equal(t, ErrCodeNoEnabledQueries, result.Error.Code)
	})

	t.Run("can enable with enabled queries", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{
					ToolGroupApprovedQueries: {Enabled: false},
				},
				Update: map[string]*models.ToolGroupConfig{
					ToolGroupApprovedQueries: {Enabled: true},
				},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)
		assert.True(t, result.State[ToolGroupApprovedQueries].Enabled)
	})
}

func TestMCPStateValidator_UnknownToolGroup(t *testing.T) {
	validator := NewMCPStateValidator()

	result := validator.Apply(
		MCPStateTransition{
			Current: map[string]*models.ToolGroupConfig{},
			Update: map[string]*models.ToolGroupConfig{
				"unknown_group": {Enabled: true},
			},
		},
		MCPStateContext{HasEnabledQueries: true},
	)

	require.NotNil(t, result.Error)
	assert.Equal(t, ErrCodeUnknownToolGroup, result.Error.Code)
}

func TestMCPStateValidator_ErrorPreservesOriginalState(t *testing.T) {
	// CRITICAL: On error, state must be unchanged
	validator := NewMCPStateValidator()

	// Try to enable developer while agent_tools is active (should fail)
	original := map[string]*models.ToolGroupConfig{
		"developer":              {Enabled: false},
		ToolGroupApprovedQueries: {Enabled: false},
		ToolGroupAgentTools:      {Enabled: true},
	}

	result := validator.Apply(
		MCPStateTransition{
			Current: original,
			Update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
		},
		MCPStateContext{HasEnabledQueries: true},
	)

	require.NotNil(t, result.Error)

	// Verify original state is preserved
	assert.False(t, result.State["developer"].Enabled)
	assert.False(t, result.State[ToolGroupApprovedQueries].Enabled)
	assert.True(t, result.State[ToolGroupAgentTools].Enabled)
}

func TestMCPStateValidator_DeepCopyPreventsModification(t *testing.T) {
	// Ensure mutations don't affect the original state
	validator := NewMCPStateValidator()

	original := map[string]*models.ToolGroupConfig{
		"developer": {Enabled: false, EnableExecute: false},
	}

	result := validator.Apply(
		MCPStateTransition{
			Current: original,
			Update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true, EnableExecute: true},
			},
		},
		MCPStateContext{HasEnabledQueries: true},
	)

	require.Nil(t, result.Error)

	// New state should be updated
	assert.True(t, result.State["developer"].Enabled)
	assert.True(t, result.State["developer"].EnableExecute)

	// Original should be unchanged
	assert.False(t, original["developer"].Enabled)
	assert.False(t, original["developer"].EnableExecute)
}

func TestMCPStateValidator_AllPermutations_NoInvalidStates(t *testing.T) {
	// Comprehensive test: apply various transitions and verify
	// no invalid states are ever produced
	validator := NewMCPStateValidator()

	// Helper to check state validity
	assertValidState := func(t *testing.T, state map[string]*models.ToolGroupConfig, description string) {
		t.Helper()
		for groupName, config := range state {
			if config == nil {
				continue
			}
			// CRITICAL: If disabled, all sub-options must be false
			if !config.Enabled {
				assert.False(t, config.EnableExecute,
					"%s: %s disabled but EnableExecute=true", description, groupName)
				assert.False(t, config.ForceMode,
					"%s: %s disabled but ForceMode=true", description, groupName)
				assert.False(t, config.AllowClientSuggestions,
					"%s: %s disabled but AllowClientSuggestions=true", description, groupName)
			}
		}
		// CRITICAL: Agent tools and other tools are mutually exclusive
		if agentConfig := state[ToolGroupAgentTools]; agentConfig != nil && agentConfig.Enabled {
			if devConfig := state["developer"]; devConfig != nil {
				assert.False(t, devConfig.Enabled,
					"%s: agent_tools and developer both enabled", description)
			}
			if aqConfig := state[ToolGroupApprovedQueries]; aqConfig != nil {
				assert.False(t, aqConfig.Enabled,
					"%s: agent_tools and approved_queries both enabled", description)
			}
		}
	}

	// Test matrix of transitions
	transitions := []struct {
		name    string
		current map[string]*models.ToolGroupConfig
		update  map[string]*models.ToolGroupConfig
		ctx     MCPStateContext
	}{
		{
			name:    "enable developer from clean state",
			current: map[string]*models.ToolGroupConfig{},
			update:  map[string]*models.ToolGroupConfig{"developer": {Enabled: true}},
			ctx:     MCPStateContext{HasEnabledQueries: true},
		},
		{
			name:    "enable approved_queries",
			current: map[string]*models.ToolGroupConfig{},
			update:  map[string]*models.ToolGroupConfig{ToolGroupApprovedQueries: {Enabled: true}},
			ctx:     MCPStateContext{HasEnabledQueries: true},
		},
		{
			name:    "enable agent_tools over developer",
			current: map[string]*models.ToolGroupConfig{"developer": {Enabled: true, EnableExecute: true}},
			update:  map[string]*models.ToolGroupConfig{ToolGroupAgentTools: {Enabled: true}},
			ctx:     MCPStateContext{HasEnabledQueries: true},
		},
		{
			name:    "disable agent_tools",
			current: map[string]*models.ToolGroupConfig{ToolGroupAgentTools: {Enabled: true}},
			update:  map[string]*models.ToolGroupConfig{ToolGroupAgentTools: {Enabled: false}},
			ctx:     MCPStateContext{HasEnabledQueries: true},
		},
		{
			name:    "toggle developer with execute",
			current: map[string]*models.ToolGroupConfig{"developer": {Enabled: true, EnableExecute: true}},
			update:  map[string]*models.ToolGroupConfig{"developer": {Enabled: false, EnableExecute: true}},
			ctx:     MCPStateContext{HasEnabledQueries: true},
		},
		{
			name: "complex: all groups with various states",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: true, EnableExecute: true},
				ToolGroupApprovedQueries: {Enabled: true, ForceMode: false, AllowClientSuggestions: true},
				ToolGroupAgentTools:      {Enabled: false},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: true},
			},
			ctx: MCPStateContext{HasEnabledQueries: true},
		},
	}

	for _, tt := range transitions {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Apply(
				MCPStateTransition{Current: tt.current, Update: tt.update},
				tt.ctx,
			)

			// Whether error or success, resulting state must be valid
			assertValidState(t, result.State, tt.name)
		})
	}
}

// TestMCPStateValidator_EnabledTools verifies that EnabledTools is populated correctly
// based on the resulting state from Apply().
func TestMCPStateValidator_EnabledTools(t *testing.T) {
	validator := NewMCPStateValidator()

	// Helper to find tool by name
	findTool := func(tools []ToolDefinition, name string) *ToolDefinition {
		for _, tool := range tools {
			if tool.Name == name {
				return &tool
			}
		}
		return nil
	}

	t.Run("empty state returns only health tool", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{},
				Update:  map[string]*models.ToolGroupConfig{},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)
		require.NotNil(t, result.EnabledTools)
		assert.Len(t, result.EnabledTools, 1)
		assert.Equal(t, "health", result.EnabledTools[0].Name)
	})

	t.Run("enabling developer shows developer tools", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{},
				Update: map[string]*models.ToolGroupConfig{
					"developer": {Enabled: true},
				},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)
		require.NotNil(t, result.EnabledTools)

		// Should include echo, get_schema, health (but NOT execute without enableExecute)
		assert.NotNil(t, findTool(result.EnabledTools, "echo"), "echo should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "get_schema"), "get_schema should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "health"), "health should be enabled")
		assert.Nil(t, findTool(result.EnabledTools, "execute"), "execute should NOT be enabled without enableExecute")
	})

	t.Run("enabling developer with execute shows execute tool", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{},
				Update: map[string]*models.ToolGroupConfig{
					"developer": {Enabled: true, EnableExecute: true},
				},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)
		require.NotNil(t, result.EnabledTools)
		assert.NotNil(t, findTool(result.EnabledTools, "execute"), "execute should be enabled with enableExecute")
	})

	t.Run("enabling approved_queries shows business user tools", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{},
				Update: map[string]*models.ToolGroupConfig{
					ToolGroupApprovedQueries: {Enabled: true},
				},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)
		require.NotNil(t, result.EnabledTools)

		// Should include approved_queries tools
		assert.NotNil(t, findTool(result.EnabledTools, "query"), "query should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "sample"), "sample should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "validate"), "validate should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "get_ontology"), "get_ontology should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "get_glossary"), "get_glossary should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "list_approved_queries"), "list_approved_queries should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "execute_approved_query"), "execute_approved_query should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "health"), "health should be enabled")

		// Should NOT include developer tools
		assert.Nil(t, findTool(result.EnabledTools, "echo"), "echo should NOT be enabled")
		assert.Nil(t, findTool(result.EnabledTools, "get_schema"), "get_schema should NOT be enabled")
	})

	t.Run("enabling agent_tools shows only agent-allowed tools", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{
					"developer":              {Enabled: true},
					ToolGroupApprovedQueries: {Enabled: true},
				},
				Update: map[string]*models.ToolGroupConfig{
					ToolGroupAgentTools: {Enabled: true},
				},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)
		require.NotNil(t, result.EnabledTools)

		// Agent mode should only have: echo, list_approved_queries, execute_approved_query, health
		assert.Len(t, result.EnabledTools, 4)
		assert.NotNil(t, findTool(result.EnabledTools, "echo"), "echo should be enabled in agent mode")
		assert.NotNil(t, findTool(result.EnabledTools, "list_approved_queries"), "list_approved_queries should be enabled in agent mode")
		assert.NotNil(t, findTool(result.EnabledTools, "execute_approved_query"), "execute_approved_query should be enabled in agent mode")
		assert.NotNil(t, findTool(result.EnabledTools, "health"), "health should be enabled in agent mode")

		// Should NOT include these tools
		assert.Nil(t, findTool(result.EnabledTools, "get_schema"), "get_schema should NOT be enabled in agent mode")
		assert.Nil(t, findTool(result.EnabledTools, "query"), "query should NOT be enabled in agent mode")
	})

	t.Run("force mode hides developer tools", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{
					"developer":              {Enabled: true},
					ToolGroupApprovedQueries: {Enabled: true, ForceMode: true},
				},
				Update: map[string]*models.ToolGroupConfig{},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)
		require.NotNil(t, result.EnabledTools)

		// Force mode should hide developer tools even if developer is enabled
		assert.Nil(t, findTool(result.EnabledTools, "echo"), "echo should NOT be enabled with force mode")
		assert.Nil(t, findTool(result.EnabledTools, "get_schema"), "get_schema should NOT be enabled with force mode")

		// But approved_queries tools should still be visible
		assert.NotNil(t, findTool(result.EnabledTools, "query"), "query should be enabled with force mode")
		assert.NotNil(t, findTool(result.EnabledTools, "list_approved_queries"), "list_approved_queries should be enabled with force mode")
	})

	t.Run("error result includes original state enabled tools", func(t *testing.T) {
		// Start with agent_tools enabled (developer is disabled by mutual exclusivity)
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{
					"developer":         {Enabled: false},
					ToolGroupAgentTools: {Enabled: true},
				},
				Update: map[string]*models.ToolGroupConfig{
					"developer": {Enabled: true}, // Try to enable developer while agent_tools is active
				},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.NotNil(t, result.Error)
		require.NotNil(t, result.EnabledTools)

		// Should reflect the original state (agent_tools enabled)
		// Original state has agent_tools enabled, so should show agent-allowed tools
		assert.NotNil(t, findTool(result.EnabledTools, "echo"), "echo should be in original state tools")
		assert.NotNil(t, findTool(result.EnabledTools, "health"), "health should be in original state tools")
	})
}

// TestMCPStateValidator_EnabledToolsConsistency verifies that EnabledTools
// always matches what GetEnabledTools would return for the resulting state.
func TestMCPStateValidator_EnabledToolsConsistency(t *testing.T) {
	validator := NewMCPStateValidator()

	transitions := []struct {
		name    string
		current map[string]*models.ToolGroupConfig
		update  map[string]*models.ToolGroupConfig
	}{
		{
			name:    "empty to developer",
			current: map[string]*models.ToolGroupConfig{},
			update:  map[string]*models.ToolGroupConfig{"developer": {Enabled: true}},
		},
		{
			name:    "developer to agent_tools",
			current: map[string]*models.ToolGroupConfig{"developer": {Enabled: true}},
			update:  map[string]*models.ToolGroupConfig{ToolGroupAgentTools: {Enabled: true}},
		},
		{
			name: "both groups to agent_tools",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: true, EnableExecute: true},
				ToolGroupApprovedQueries: {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{ToolGroupAgentTools: {Enabled: true}},
		},
		{
			name:    "enable approved_queries",
			current: map[string]*models.ToolGroupConfig{},
			update:  map[string]*models.ToolGroupConfig{ToolGroupApprovedQueries: {Enabled: true}},
		},
		{
			name: "enable force mode",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: true},
				ToolGroupApprovedQueries: {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true, ForceMode: true},
			},
		},
	}

	for _, tt := range transitions {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Apply(
				MCPStateTransition{Current: tt.current, Update: tt.update},
				MCPStateContext{HasEnabledQueries: true},
			)

			// Compute what tools should be enabled based on the resulting state
			expectedTools := GetEnabledTools(result.State)

			// Verify EnabledTools matches
			require.Equal(t, len(expectedTools), len(result.EnabledTools),
				"EnabledTools count should match GetEnabledTools result")

			// Create a map for easier comparison
			expectedMap := make(map[string]bool)
			for _, tool := range expectedTools {
				expectedMap[tool.Name] = true
			}

			for _, tool := range result.EnabledTools {
				assert.True(t, expectedMap[tool.Name],
					"tool %s in EnabledTools should be in GetEnabledTools result", tool.Name)
			}
		})
	}
}
