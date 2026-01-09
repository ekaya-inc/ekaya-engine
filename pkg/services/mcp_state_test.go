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
				"developer": {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: false}, // trying to keep execute
			},
			expected: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: false}, // must be reset
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
				"developer": {Enabled: false},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
			expected: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
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
				assert.Equal(t, expectedConfig.ForceMode, actualConfig.ForceMode,
					"group %s: ForceMode mismatch", groupName)
				assert.Equal(t, expectedConfig.AllowClientSuggestions, actualConfig.AllowClientSuggestions,
					"group %s: AllowClientSuggestions mismatch", groupName)
			}
		})
	}
}

func TestMCPStateValidator_MutualExclusivity_AgentTools(t *testing.T) {
	// With radio button behavior, enabling any group disables the others.
	// No errors are returned - the system automatically switches.
	tests := []struct {
		name           string
		current        map[string]*models.ToolGroupConfig
		update         map[string]*models.ToolGroupConfig
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
			expectedStates: map[string]bool{
				ToolGroupApprovedQueries: false,
				ToolGroupAgentTools:      true,
			},
		},
		{
			name: "enabling developer while agent_tools is active - radio button switches",
			current: map[string]*models.ToolGroupConfig{
				"developer":         {Enabled: false},
				ToolGroupAgentTools: {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
			expectedStates: map[string]bool{
				"developer":         true,
				ToolGroupAgentTools: false, // Radio button disables agent_tools
			},
		},
		{
			name: "enabling approved_queries while agent_tools is active - radio button switches",
			current: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: false},
				ToolGroupAgentTools:      {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
			expectedStates: map[string]bool{
				ToolGroupApprovedQueries: true,
				ToolGroupAgentTools:      false, // Radio button disables agent_tools
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

			// Radio button behavior: no errors, just switches
			require.Nil(t, result.Error, "radio button transitions should not error")
			for groupName, expectedEnabled := range tt.expectedStates {
				config := result.State[groupName]
				require.NotNil(t, config, "group %s should exist", groupName)
				assert.Equal(t, expectedEnabled, config.Enabled,
					"group %s: expected enabled=%v", groupName, expectedEnabled)
			}
		})
	}
}

func TestMCPStateValidator_ForceMode_NoLongerUsed(t *testing.T) {
	// ForceMode is no longer used with radio button behavior.
	// This test verifies that ForceMode doesn't block enabling developer tools.
	validator := NewMCPStateValidator()

	t.Run("enabling developer works regardless of force mode", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{
					"developer":              {Enabled: false},
					ToolGroupApprovedQueries: {Enabled: true, ForceMode: true},
				},
				Update: map[string]*models.ToolGroupConfig{
					"developer": {Enabled: true},
				},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		// With radio button behavior, enabling developer should work
		// and disable approved_queries
		require.Nil(t, result.Error, "ForceMode should not block enabling developer with radio button behavior")
		assert.True(t, result.State["developer"].Enabled)
		assert.False(t, result.State[ToolGroupApprovedQueries].Enabled, "approved_queries should be disabled by radio button")
	})
}

func TestMCPStateValidator_ApprovedQueries_NoLongerRequiresEnabledQueries(t *testing.T) {
	// With radio button behavior, the requirement for enabled queries is removed.
	// Business users can enable the tool group regardless of query state.
	validator := NewMCPStateValidator()

	t.Run("can enable without enabled queries", func(t *testing.T) {
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

		// No error - can enable without queries now
		require.Nil(t, result.Error)
		assert.True(t, result.State[ToolGroupApprovedQueries].Enabled)
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

func TestMCPStateValidator_RadioButtonSwitchesState(t *testing.T) {
	// With radio button behavior, enabling developer while agent_tools is active
	// should succeed and switch to developer (disable agent_tools).
	validator := NewMCPStateValidator()

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

	// Radio button: no error, just switches
	require.Nil(t, result.Error)

	// Verify radio button switched the state
	assert.True(t, result.State["developer"].Enabled, "developer should now be enabled")
	assert.False(t, result.State[ToolGroupApprovedQueries].Enabled)
	assert.False(t, result.State[ToolGroupAgentTools].Enabled, "agent_tools should be disabled by radio button")
}

func TestMCPStateValidator_DeepCopyPreventsModification(t *testing.T) {
	// Ensure mutations don't affect the original state
	validator := NewMCPStateValidator()

	original := map[string]*models.ToolGroupConfig{
		"developer": {Enabled: false},
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

	require.Nil(t, result.Error)

	// New state should be updated
	assert.True(t, result.State["developer"].Enabled)

	// Original should be unchanged
	assert.False(t, original["developer"].Enabled)
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
			current: map[string]*models.ToolGroupConfig{"developer": {Enabled: true}},
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
			current: map[string]*models.ToolGroupConfig{"developer": {Enabled: true}},
			update:  map[string]*models.ToolGroupConfig{"developer": {Enabled: false}},
			ctx:     MCPStateContext{HasEnabledQueries: true},
		},
		{
			name: "complex: all groups with various states",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: true},
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

	t.Run("enabling developer shows Developer Core tools", func(t *testing.T) {
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

		// Should include Developer Core tools only (Default + DeveloperCore loadouts)
		assert.NotNil(t, findTool(result.EnabledTools, "health"), "health should be enabled (always available)")
		assert.NotNil(t, findTool(result.EnabledTools, "echo"), "echo should be enabled (Developer Core)")
		assert.NotNil(t, findTool(result.EnabledTools, "execute"), "execute should be enabled (Developer Core)")

		// Query loadout tools NOT included without AddQueryTools
		assert.Nil(t, findTool(result.EnabledTools, "validate"), "validate requires AddQueryTools")
		assert.Nil(t, findTool(result.EnabledTools, "query"), "query requires AddQueryTools")
		assert.Nil(t, findTool(result.EnabledTools, "explain_query"), "explain_query requires AddQueryTools")
		assert.Nil(t, findTool(result.EnabledTools, "get_schema"), "get_schema requires AddQueryTools")
		assert.Nil(t, findTool(result.EnabledTools, "sample"), "sample requires AddQueryTools")
	})

	t.Run("enabling approved_queries shows Query loadout tools", func(t *testing.T) {
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

		// Should include Query loadout tools (includes get_schema per spec)
		assert.NotNil(t, findTool(result.EnabledTools, "query"), "query should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "sample"), "sample should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "validate"), "validate should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "get_ontology"), "get_ontology should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "list_glossary"), "list_glossary should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "get_glossary_sql"), "get_glossary_sql should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "list_approved_queries"), "list_approved_queries should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "execute_approved_query"), "execute_approved_query should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "health"), "health should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "get_schema"), "get_schema IS in Query loadout per spec")
		assert.NotNil(t, findTool(result.EnabledTools, "get_context"), "get_context should be enabled")

		// Should NOT include developer-only tools
		assert.Nil(t, findTool(result.EnabledTools, "echo"), "echo should NOT be enabled")
		assert.Nil(t, findTool(result.EnabledTools, "execute"), "execute should NOT be enabled")

		// Should NOT include ontology maintenance without option
		assert.Nil(t, findTool(result.EnabledTools, "update_entity"), "update_entity requires option")
	})

	t.Run("enabling agent_tools shows limited query tools for agent perspective", func(t *testing.T) {
		// When agent_tools is enabled, the UI shows what agents would see (limited query tools).
		// This allows users to see what tools their agents will have access to.
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

		// When agent_tools is enabled, UI shows agent perspective (limited query tools)
		assert.Len(t, result.EnabledTools, 3)
		assert.NotNil(t, findTool(result.EnabledTools, "health"), "health should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "list_approved_queries"), "list_approved_queries should be enabled for agents")
		assert.NotNil(t, findTool(result.EnabledTools, "execute_approved_query"), "execute_approved_query should be enabled for agents")
	})

	t.Run("force mode no longer hides developer tools with radio button", func(t *testing.T) {
		// Note: With radio button behavior, both developer and approved_queries
		// cannot be enabled at the same time. This test verifies behavior when
		// state is manually set (e.g., from database).
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

		// With both enabled in the state (shouldn't happen with radio button,
		// but GetEnabledTools still shows both groups' tools)
		assert.NotNil(t, findTool(result.EnabledTools, "echo"), "echo should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "get_schema"), "get_schema should be enabled")
		assert.NotNil(t, findTool(result.EnabledTools, "query"), "query should be enabled")
	})

	t.Run("radio button switch shows new state enabled tools", func(t *testing.T) {
		// With radio button, enabling developer while agent_tools is active
		// should succeed and show Developer Core tools only
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{
					"developer":         {Enabled: false},
					ToolGroupAgentTools: {Enabled: true},
				},
				Update: map[string]*models.ToolGroupConfig{
					"developer": {Enabled: true}, // Radio button: switch to developer
				},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error, "radio button switch should not error")
		require.NotNil(t, result.EnabledTools)

		// Should reflect the new state (Developer Core tools only)
		assert.NotNil(t, findTool(result.EnabledTools, "health"), "health should be enabled (always available)")
		assert.NotNil(t, findTool(result.EnabledTools, "echo"), "echo should be enabled (Developer Core)")
		assert.NotNil(t, findTool(result.EnabledTools, "execute"), "execute should be enabled (Developer Core)")

		// Query loadout tools NOT included without AddQueryTools
		assert.Nil(t, findTool(result.EnabledTools, "query"), "query requires AddQueryTools")
		assert.Nil(t, findTool(result.EnabledTools, "validate"), "validate requires AddQueryTools")
		assert.Nil(t, findTool(result.EnabledTools, "explain_query"), "explain_query requires AddQueryTools")
		assert.Nil(t, findTool(result.EnabledTools, "get_schema"), "get_schema requires AddQueryTools")
		assert.Nil(t, findTool(result.EnabledTools, "sample"), "sample requires AddQueryTools")
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
				"developer":              {Enabled: true},
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
			// When agent_tools is enabled, we show agent perspective
			var expectedTools []ToolDefinition
			agentConfig := result.State[ToolGroupAgentTools]
			if agentConfig != nil && agentConfig.Enabled {
				expectedTools = GetEnabledToolsForAgent(result.State)
			} else {
				expectedTools = GetEnabledTools(result.State)
			}

			// Verify EnabledTools matches
			require.Equal(t, len(expectedTools), len(result.EnabledTools),
				"EnabledTools count should match expected tools")

			// Create a map for easier comparison
			expectedMap := make(map[string]bool)
			for _, tool := range expectedTools {
				expectedMap[tool.Name] = true
			}

			for _, tool := range result.EnabledTools {
				assert.True(t, expectedMap[tool.Name],
					"tool %s in EnabledTools should be in expected tools", tool.Name)
			}
		})
	}
}

// ============================================================================
// NEW TESTS: Radio Button Behavior for Tool Groups
// ============================================================================
// These tests verify the simplified radio-button behavior where only ONE
// top-level tool group can be enabled at a time.

func TestMCPStateValidator_RadioButton_OnlyOneGroupEnabled(t *testing.T) {
	// CRITICAL: Only one of business_user, agent_tools, or developer can be
	// enabled at a time. Enabling one should disable the others.
	validator := NewMCPStateValidator()

	tests := []struct {
		name           string
		current        map[string]*models.ToolGroupConfig
		update         map[string]*models.ToolGroupConfig
		expectedStates map[string]bool // groupName -> expectedEnabled
	}{
		{
			name: "enabling business_user disables developer",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: true},
				ToolGroupApprovedQueries: {Enabled: false},
				ToolGroupAgentTools:      {Enabled: false},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
			expectedStates: map[string]bool{
				"developer":              false,
				ToolGroupApprovedQueries: true,
				ToolGroupAgentTools:      false,
			},
		},
		{
			name: "enabling business_user disables agent_tools",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: false},
				ToolGroupApprovedQueries: {Enabled: false},
				ToolGroupAgentTools:      {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
			expectedStates: map[string]bool{
				"developer":              false,
				ToolGroupApprovedQueries: true,
				ToolGroupAgentTools:      false,
			},
		},
		{
			name: "enabling developer disables business_user",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: false},
				ToolGroupApprovedQueries: {Enabled: true},
				ToolGroupAgentTools:      {Enabled: false},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
			expectedStates: map[string]bool{
				"developer":              true,
				ToolGroupApprovedQueries: false,
				ToolGroupAgentTools:      false,
			},
		},
		{
			name: "enabling developer disables agent_tools",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: false},
				ToolGroupApprovedQueries: {Enabled: false},
				ToolGroupAgentTools:      {Enabled: true},
			},
			update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true},
			},
			expectedStates: map[string]bool{
				"developer":              true,
				ToolGroupApprovedQueries: false,
				ToolGroupAgentTools:      false,
			},
		},
		{
			name: "enabling agent_tools disables both business_user and developer",
			current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: true},
				ToolGroupApprovedQueries: {Enabled: true},
				ToolGroupAgentTools:      {Enabled: false},
			},
			update: map[string]*models.ToolGroupConfig{
				ToolGroupAgentTools: {Enabled: true},
			},
			expectedStates: map[string]bool{
				"developer":              false,
				ToolGroupApprovedQueries: false,
				ToolGroupAgentTools:      true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Apply(
				MCPStateTransition{Current: tt.current, Update: tt.update},
				MCPStateContext{HasEnabledQueries: true}, // Not relevant for radio button behavior
			)

			require.Nil(t, result.Error, "radio button transitions should never error")

			for groupName, expectedEnabled := range tt.expectedStates {
				config := result.State[groupName]
				require.NotNil(t, config, "group %s should exist", groupName)
				assert.Equal(t, expectedEnabled, config.Enabled,
					"group %s: expected enabled=%v", groupName, expectedEnabled)
			}
		})
	}
}

func TestMCPStateValidator_RadioButton_NoQueriesNotBlocking(t *testing.T) {
	// With radio button behavior, enabling business_user tools should work
	// even without enabled queries (the restriction is removed)
	validator := NewMCPStateValidator()

	result := validator.Apply(
		MCPStateTransition{
			Current: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: false},
			},
			Update: map[string]*models.ToolGroupConfig{
				ToolGroupApprovedQueries: {Enabled: true},
			},
		},
		MCPStateContext{HasEnabledQueries: false}, // No queries exist
	)

	// Should succeed - no longer blocked by missing queries
	require.Nil(t, result.Error, "enabling business_user should not require enabled queries")
	assert.True(t, result.State[ToolGroupApprovedQueries].Enabled)
}

func TestMCPStateValidator_DeveloperTools_ExecuteAvailableByDefault(t *testing.T) {
	// When developer tools is enabled, execute is always available as part of Developer Core loadout
	validator := NewMCPStateValidator()

	findTool := func(tools []ToolDefinition, name string) *ToolDefinition {
		for _, tool := range tools {
			if tool.Name == name {
				return &tool
			}
		}
		return nil
	}

	result := validator.Apply(
		MCPStateTransition{
			Current: map[string]*models.ToolGroupConfig{},
			Update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: true}, // Just enabled, no enableExecute flag needed
			},
		},
		MCPStateContext{HasEnabledQueries: true},
	)

	require.Nil(t, result.Error)
	require.NotNil(t, result.EnabledTools)

	// Execute should be included when developer mode is on
	assert.NotNil(t, findTool(result.EnabledTools, "execute"),
		"execute should be enabled when developer mode is on")
	assert.NotNil(t, findTool(result.EnabledTools, "echo"),
		"echo should be enabled")
	// Developer Core does NOT include get_schema - need AddQueryTools option for that
	assert.Nil(t, findTool(result.EnabledTools, "get_schema"),
		"get_schema requires AddQueryTools option")
}

func TestMCPStateValidator_RadioButton_DisablingOneDoesNotEnableAnother(t *testing.T) {
	// Disabling the active group should not automatically enable another
	validator := NewMCPStateValidator()

	result := validator.Apply(
		MCPStateTransition{
			Current: map[string]*models.ToolGroupConfig{
				"developer":              {Enabled: true},
				ToolGroupApprovedQueries: {Enabled: false},
				ToolGroupAgentTools:      {Enabled: false},
			},
			Update: map[string]*models.ToolGroupConfig{
				"developer": {Enabled: false},
			},
		},
		MCPStateContext{HasEnabledQueries: true},
	)

	require.Nil(t, result.Error)

	// All groups should be disabled now
	assert.False(t, result.State["developer"].Enabled)
	assert.False(t, result.State[ToolGroupApprovedQueries].Enabled)
	assert.False(t, result.State[ToolGroupAgentTools].Enabled)
}

func TestMCPStateValidator_RadioButton_EnabledToolsReflectSelection(t *testing.T) {
	// Verify that EnabledTools correctly reflects the radio button selection
	validator := NewMCPStateValidator()

	findTool := func(tools []ToolDefinition, name string) *ToolDefinition {
		for _, tool := range tools {
			if tool.Name == name {
				return &tool
			}
		}
		return nil
	}

	t.Run("business_user selected shows Query loadout tools", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{"developer": {Enabled: true}},
				Update:  map[string]*models.ToolGroupConfig{ToolGroupApprovedQueries: {Enabled: true}},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)

		// Query loadout tools should be present
		assert.NotNil(t, findTool(result.EnabledTools, "query"))
		assert.NotNil(t, findTool(result.EnabledTools, "sample"))
		assert.NotNil(t, findTool(result.EnabledTools, "list_approved_queries"))
		assert.NotNil(t, findTool(result.EnabledTools, "get_schema")) // get_schema IS in Query loadout

		// Developer Core tools should NOT be present (radio button disabled them)
		assert.Nil(t, findTool(result.EnabledTools, "echo"))
		assert.Nil(t, findTool(result.EnabledTools, "execute"))
	})

	t.Run("developer selected shows Developer Core tools", func(t *testing.T) {
		result := validator.Apply(
			MCPStateTransition{
				Current: map[string]*models.ToolGroupConfig{ToolGroupApprovedQueries: {Enabled: true}},
				Update:  map[string]*models.ToolGroupConfig{"developer": {Enabled: true}},
			},
			MCPStateContext{HasEnabledQueries: true},
		)

		require.Nil(t, result.Error)

		// Developer Core tools should be present (Default + DeveloperCore loadouts)
		assert.NotNil(t, findTool(result.EnabledTools, "health"))
		assert.NotNil(t, findTool(result.EnabledTools, "echo"))
		assert.NotNil(t, findTool(result.EnabledTools, "execute"))

		// Query loadout tools should NOT be present (need AddQueryTools option)
		assert.Nil(t, findTool(result.EnabledTools, "validate"))
		assert.Nil(t, findTool(result.EnabledTools, "query"))
		assert.Nil(t, findTool(result.EnabledTools, "explain_query"))
		assert.Nil(t, findTool(result.EnabledTools, "get_schema"))
		assert.Nil(t, findTool(result.EnabledTools, "sample"))

		// Developer Core = 2 tools (echo, execute) + Default (health) = 3 tools
		assert.Len(t, result.EnabledTools, 3)
	})
}
