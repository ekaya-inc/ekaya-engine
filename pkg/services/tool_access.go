package services

import (
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// ToolAccessChecker determines tool accessibility based on configuration state.
// This is the single source of truth for tool access decisions, used by both:
// - MCP tool filter (NewToolFilter) for listing available tools
// - Tool handlers (check*Enabled functions) for authorizing tool execution
//
// By centralizing this logic, we guarantee that if a tool is listed, it can be called.
type ToolAccessChecker interface {
	// IsToolAccessible checks if a specific tool is accessible given the current state.
	// The isAgent parameter indicates if the caller is using agent authentication.
	IsToolAccessible(toolName string, state map[string]*models.ToolGroupConfig, isAgent bool) bool

	// GetAccessibleTools returns all tools accessible given the current state.
	// The isAgent parameter indicates if the caller is using agent authentication.
	GetAccessibleTools(state map[string]*models.ToolGroupConfig, isAgent bool) []ToolDefinition
}

type toolAccessChecker struct{}

// NewToolAccessChecker creates a new tool access checker.
func NewToolAccessChecker() ToolAccessChecker {
	return &toolAccessChecker{}
}

// IsToolAccessible checks if a specific tool is accessible given the current state.
// Uses the loadout system to determine access.
func (c *toolAccessChecker) IsToolAccessible(toolName string, state map[string]*models.ToolGroupConfig, isAgent bool) bool {
	// Compute enabled tools using the loadout system
	enabledTools := ComputeEnabledToolsFromConfig(state, isAgent)

	// Check if the requested tool is in the enabled list
	for _, tool := range enabledTools {
		if tool.Name == toolName {
			return true
		}
	}
	return false
}

// GetAccessibleTools returns all tools accessible given the current state.
// Uses the loadout system to compute the tool list in canonical order.
func (c *toolAccessChecker) GetAccessibleTools(state map[string]*models.ToolGroupConfig, isAgent bool) []ToolDefinition {
	// Get tools from loadout system (returns ToolSpec)
	specs := ComputeEnabledToolsFromConfig(state, isAgent)

	// Convert ToolSpec to ToolDefinition for backward compatibility
	result := make([]ToolDefinition, len(specs))
	for i, spec := range specs {
		result[i] = ToolDefinition{
			Name:        spec.Name,
			Description: spec.Description,
		}
	}
	return result
}

// Ensure toolAccessChecker implements ToolAccessChecker at compile time.
var _ ToolAccessChecker = (*toolAccessChecker)(nil)
