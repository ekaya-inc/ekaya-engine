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
	ErrCodeUnknownToolGroup = "unknown_tool_group"
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
	_ = ctx

	newState := v.deepCopy(transition.Current)

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

		v.applyUpdate(groupName, currentConfig, updateConfig)
	}

	enabledTools := GetEnabledTools(newState)

	return MCPStateResult{
		State:        newState,
		EnabledTools: enabledTools,
		Error:        nil,
	}
}

// applyUpdate applies the update config to the current config.
func (v *mcpStateValidator) applyUpdate(groupName string, current, update *models.ToolGroupConfig) {
	switch groupName {
	case ToolGroupTools:
		current.AddDirectDatabaseAccess = update.AddDirectDatabaseAccess
		current.AddOntologyMaintenanceTools = update.AddOntologyMaintenanceTools
		current.AddOntologySuggestions = update.AddOntologySuggestions
		current.AddApprovalTools = update.AddApprovalTools
		current.AddRequestTools = update.AddRequestTools
	case ToolGroupAgentTools:
		current.Enabled = update.Enabled
	}
}

// deepCopy creates a deep copy of the state map.
func (v *mcpStateValidator) deepCopy(state map[string]*models.ToolGroupConfig) map[string]*models.ToolGroupConfig {
	if state == nil {
		return make(map[string]*models.ToolGroupConfig)
	}

	copy := make(map[string]*models.ToolGroupConfig, len(state))
	for groupName, config := range state {
		if !validToolGroups[groupName] || config == nil {
			continue
		}

		copy[groupName] = &models.ToolGroupConfig{
			Enabled:                     config.Enabled,
			AddDirectDatabaseAccess:     config.AddDirectDatabaseAccess,
			AddOntologyMaintenanceTools: config.AddOntologyMaintenanceTools,
			AddOntologySuggestions:      config.AddOntologySuggestions,
			AddApprovalTools:            config.AddApprovalTools,
			AddRequestTools:             config.AddRequestTools,
		}
	}
	return copy
}

// Ensure mcpStateValidator implements MCPStateValidator at compile time.
var _ MCPStateValidator = (*mcpStateValidator)(nil)
