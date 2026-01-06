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

	// Apply mutual exclusivity rules
	v.applyMutualExclusivity(newState)

	// Normalize state (reset sub-options when disabled)
	v.normalizeState(newState)

	// Compute enabled tools based on the final state
	enabledTools := GetEnabledTools(newState)

	return MCPStateResult{
		State:        newState,
		EnabledTools: enabledTools,
		Error:        nil,
	}
}

// validateEnabling checks if a tool group can be enabled.
func (v *mcpStateValidator) validateEnabling(groupName string, state map[string]*models.ToolGroupConfig, ctx MCPStateContext) *MCPStateError {
	switch groupName {
	case ToolGroupAgentTools:
		// Agent tools can always be enabled (they'll disable others)
		return nil

	case ToolGroupApprovedQueries:
		// Check if agent tools is enabled
		if agentConfig := state[ToolGroupAgentTools]; agentConfig != nil && agentConfig.Enabled {
			return &MCPStateError{
				Code:    ErrCodeAgentToolsConflict,
				Message: "Business User Tools cannot be enabled while Agent Tools is active.",
			}
		}
		// Check if there are enabled queries
		if !ctx.HasEnabledQueries {
			return &MCPStateError{
				Code:    ErrCodeNoEnabledQueries,
				Message: "Create and enable queries first.",
			}
		}
		return nil

	case "developer":
		// Check if agent tools is enabled
		if agentConfig := state[ToolGroupAgentTools]; agentConfig != nil && agentConfig.Enabled {
			return &MCPStateError{
				Code:    ErrCodeAgentToolsConflict,
				Message: "Developer Tools cannot be enabled while Agent Tools is active.",
			}
		}
		// Check if force mode is enabled
		if aqConfig := state[ToolGroupApprovedQueries]; aqConfig != nil && aqConfig.ForceMode {
			return &MCPStateError{
				Code:    ErrCodeForceModeConflict,
				Message: "Only Business User Tools are allowed. Disable FORCE mode first.",
			}
		}
		return nil
	}

	return nil
}

// applyUpdate applies the update config to the current config.
func (v *mcpStateValidator) applyUpdate(groupName string, current, update *models.ToolGroupConfig) {
	current.Enabled = update.Enabled
	current.EnableExecute = update.EnableExecute
	current.ForceMode = update.ForceMode
	current.AllowClientSuggestions = update.AllowClientSuggestions
}

// applyMutualExclusivity enforces mutual exclusivity rules.
func (v *mcpStateValidator) applyMutualExclusivity(state map[string]*models.ToolGroupConfig) {
	// If agent_tools is enabled, disable everything else
	if agentConfig := state[ToolGroupAgentTools]; agentConfig != nil && agentConfig.Enabled {
		if devConfig := state["developer"]; devConfig != nil {
			devConfig.Enabled = false
		}
		if aqConfig := state[ToolGroupApprovedQueries]; aqConfig != nil {
			aqConfig.Enabled = false
		}
	}
}

// normalizeState ensures sub-options are false when their parent is disabled.
func (v *mcpStateValidator) normalizeState(state map[string]*models.ToolGroupConfig) {
	for _, config := range state {
		if config != nil && !config.Enabled {
			config.EnableExecute = false
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
			copy[k] = &models.ToolGroupConfig{
				Enabled:                v.Enabled,
				EnableExecute:          v.EnableExecute,
				ForceMode:              v.ForceMode,
				AllowClientSuggestions: v.AllowClientSuggestions,
			}
		}
	}
	return copy
}

// Ensure mcpStateValidator implements MCPStateValidator at compile time.
var _ MCPStateValidator = (*mcpStateValidator)(nil)
