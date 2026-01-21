package services

import (
	"fmt"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// MCPStateError represents a validation error during state transition.
// It includes both a technical message and a user-friendly message.
type MCPStateError struct {
	Code    string // Machine-readable error code
	Message string // User-friendly message for UI display
}

func (e *MCPStateError) Error() string {
	return e.Message
}

// Common error codes for state transition failures
const (
	ErrCodeAgentToolsConflict = "agent_tools_conflict"
	ErrCodeForceModeConflict  = "force_mode_conflict"
	ErrCodeInvalidSubOption   = "invalid_sub_option"
	ErrCodeNoEnabledQueries   = "no_enabled_queries"
	ErrCodeUnknownToolGroup   = "unknown_tool_group"
)

// MCPStateTransition represents a request to change MCP config state.
type MCPStateTransition struct {
	Current map[string]*models.ToolGroupConfig
	Update  map[string]*models.ToolGroupConfig
}

// MCPStateResult represents the result of a state transition.
type MCPStateResult struct {
	State        map[string]*models.ToolGroupConfig
	EnabledTools []ToolDefinition // Tools enabled based on the resulting state
	Error        *MCPStateError
}

// MCPStateContext provides context needed for validation decisions.
type MCPStateContext struct {
	HasEnabledQueries bool
}

// MCPStateValidator validates and applies state transitions.
// It encapsulates all business rules for MCP configuration changes.
type MCPStateValidator interface {
	// Apply validates the transition and returns the new state or an error.
	// If an error is returned, the State field contains the unchanged current state.
	Apply(transition MCPStateTransition, ctx MCPStateContext) MCPStateResult
}

// mcpStateValidator implements MCPStateValidator with all business rules.
type mcpStateValidator struct{}

// NewMCPStateValidator creates a new state validator.
func NewMCPStateValidator() MCPStateValidator {
	return &mcpStateValidator{}
}

// Apply validates and applies the state transition.
func (v *mcpStateValidator) Apply(transition MCPStateTransition, ctx MCPStateContext) MCPStateResult {
	// Start with a deep copy of current state
	newState := v.deepCopy(transition.Current)

	// Track which group is being enabled in this transition (for radio button behavior)
	var newlyEnabledGroup string

	// Validate each update
	for groupName, updateConfig := range transition.Update {
		if !validToolGroups[groupName] {
			originalState := v.deepCopy(transition.Current)
			return MCPStateResult{
				State:        originalState,
				EnabledTools: GetEnabledTools(originalState),
				Error: &MCPStateError{
					Code:    ErrCodeUnknownToolGroup,
					Message: fmt.Sprintf("Unknown tool group: %s", groupName),
				},
			}
		}

		// Get current config for this group (or create empty)
		currentConfig := newState[groupName]
		if currentConfig == nil {
			currentConfig = &models.ToolGroupConfig{}
			newState[groupName] = currentConfig
		}

		// Track if this group is being enabled
		if updateConfig.Enabled && !currentConfig.Enabled {
			newlyEnabledGroup = groupName
		}

		// Check if trying to enable this group
		if updateConfig.Enabled && !currentConfig.Enabled {
			if err := v.validateEnabling(groupName, newState, ctx); err != nil {
				originalState := v.deepCopy(transition.Current)
				return MCPStateResult{
					State:        originalState,
					EnabledTools: GetEnabledTools(originalState),
					Error:        err,
				}
			}
		}

		// Apply the update
		v.applyUpdate(groupName, currentConfig, updateConfig)
	}

	// Apply mutual exclusivity rules (radio button behavior)
	v.applyMutualExclusivity(newState, newlyEnabledGroup)

	// Normalize state (reset sub-options when disabled)
	v.normalizeState(newState)

	// Compute enabled tools based on the final state
	// When agent_tools is enabled, show what agents would see (limited query tools)
	var enabledTools []ToolDefinition
	agentConfig := newState[ToolGroupAgentTools]
	if agentConfig != nil && agentConfig.Enabled {
		enabledTools = GetEnabledToolsForAgent(newState)
	} else {
		enabledTools = GetEnabledTools(newState)
	}

	return MCPStateResult{
		State:        newState,
		EnabledTools: enabledTools,
		Error:        nil,
	}
}

// validateEnabling checks if a tool group can be enabled.
// With radio button behavior, any group can always be enabled (it will disable others).
func (v *mcpStateValidator) validateEnabling(groupName string, state map[string]*models.ToolGroupConfig, ctx MCPStateContext) *MCPStateError {
	// Radio button behavior: any tool group can be enabled at any time.
	// The mutual exclusivity is handled in applyMutualExclusivity.
	return nil
}

// applyUpdate applies the update config to the current config.
func (v *mcpStateValidator) applyUpdate(groupName string, current, update *models.ToolGroupConfig) {
	current.Enabled = update.Enabled

	// New sub-options
	current.AllowOntologyMaintenance = update.AllowOntologyMaintenance
	current.AddQueryTools = update.AddQueryTools
	current.AddOntologyMaintenance = update.AddOntologyMaintenance
	current.CustomTools = update.CustomTools

	// Legacy sub-options (backward compatibility)
	current.ForceMode = update.ForceMode
	current.AllowClientSuggestions = update.AllowClientSuggestions
}

// applyMutualExclusivity enforces radio button behavior - only one tool group can be enabled.
// When a group is being enabled, all others are disabled.
// newlyEnabledGroup is the group that was just enabled in this transition (may be empty if none).
func (v *mcpStateValidator) applyMutualExclusivity(state map[string]*models.ToolGroupConfig, newlyEnabledGroup string) {
	// If a group was just enabled, disable all others (radio button behavior)
	if newlyEnabledGroup != "" {
		for groupName, config := range state {
			if config != nil && groupName != newlyEnabledGroup {
				config.Enabled = false
			}
		}
	}
}

// normalizeState ensures sub-options are false when their parent is disabled.
func (v *mcpStateValidator) normalizeState(state map[string]*models.ToolGroupConfig) {
	for _, config := range state {
		if config != nil && !config.Enabled {
			// New sub-options
			config.AllowOntologyMaintenance = false
			config.AddQueryTools = false
			config.AddOntologyMaintenance = false
			config.CustomTools = nil

			// Legacy sub-options
			config.ForceMode = false
			config.AllowClientSuggestions = false
		}
	}
}

// deepCopy creates a deep copy of the state map.
func (v *mcpStateValidator) deepCopy(state map[string]*models.ToolGroupConfig) map[string]*models.ToolGroupConfig {
	if state == nil {
		return make(map[string]*models.ToolGroupConfig)
	}

	copy := make(map[string]*models.ToolGroupConfig, len(state))
	for k, v := range state {
		if v != nil {
			// Copy CustomTools slice
			var customToolsCopy []string
			if v.CustomTools != nil {
				customToolsCopy = make([]string, len(v.CustomTools))
				for i, t := range v.CustomTools {
					customToolsCopy[i] = t
				}
			}

			copy[k] = &models.ToolGroupConfig{
				Enabled: v.Enabled,

				// New sub-options
				AllowOntologyMaintenance: v.AllowOntologyMaintenance,
				AddQueryTools:            v.AddQueryTools,
				AddOntologyMaintenance:   v.AddOntologyMaintenance,
				CustomTools:              customToolsCopy,

				// Legacy sub-options
				ForceMode:              v.ForceMode,
				AllowClientSuggestions: v.AllowClientSuggestions,
			}
		}
	}
	return copy
}

// Ensure mcpStateValidator implements MCPStateValidator at compile time.
var _ MCPStateValidator = (*mcpStateValidator)(nil)
