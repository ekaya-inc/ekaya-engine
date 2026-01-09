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
func (c *toolAccessChecker) IsToolAccessible(toolName string, state map[string]*models.ToolGroupConfig, isAgent bool) bool {
	// Health is always accessible
	if toolName == "health" {
		return true
	}

	// Find the tool in the registry
	var toolDef *ToolDefinition
	for i := range ToolRegistry {
		if ToolRegistry[i].Name == toolName {
			toolDef = &ToolRegistry[i]
			break
		}
	}

	// Unknown tool - not accessible
	if toolDef == nil {
		return false
	}

	// No state means only "always" tools are available
	if state == nil {
		return toolDef.ToolGroup == "always"
	}

	// Agent authentication has special rules
	if isAgent {
		return c.isToolAccessibleForAgent(toolDef, state)
	}

	// User authentication
	return c.isToolAccessibleForUser(toolDef, state)
}

// isToolAccessibleForAgent checks tool access for agent authentication.
// Agents can only access tools when agent_tools is enabled, and only specific tools.
func (c *toolAccessChecker) isToolAccessibleForAgent(tool *ToolDefinition, state map[string]*models.ToolGroupConfig) bool {
	// Check if agent_tools is enabled
	agentConfig := state[ToolGroupAgentTools]
	if agentConfig == nil || !agentConfig.Enabled {
		return false
	}

	// Only specific tools are available to agents
	return agentAllowedTools[tool.Name]
}

// isToolAccessibleForUser checks tool access for user authentication.
func (c *toolAccessChecker) isToolAccessibleForUser(tool *ToolDefinition, state map[string]*models.ToolGroupConfig) bool {
	// "always" tools are always accessible
	if tool.ToolGroup == "always" {
		return true
	}

	// Developer mode grants access to ALL tools
	devConfig := state[ToolGroupDeveloper]
	if devConfig != nil && devConfig.Enabled {
		return true
	}

	// Check if the tool's specific group is enabled
	switch tool.ToolGroup {
	case ToolGroupApprovedQueries:
		aqConfig := state[ToolGroupApprovedQueries]
		return aqConfig != nil && aqConfig.Enabled

	case ToolGroupDeveloper:
		// Already checked above, but if we get here, developer is not enabled
		return false

	case ToolGroupAgentTools:
		// Agent tools group is only for agent auth, not user auth
		return false
	}

	return false
}

// GetAccessibleTools returns all tools accessible given the current state.
func (c *toolAccessChecker) GetAccessibleTools(state map[string]*models.ToolGroupConfig, isAgent bool) []ToolDefinition {
	var result []ToolDefinition

	for _, tool := range ToolRegistry {
		if c.IsToolAccessible(tool.Name, state, isAgent) {
			result = append(result, tool)
		}
	}

	return result
}

// Ensure toolAccessChecker implements ToolAccessChecker at compile time.
var _ ToolAccessChecker = (*toolAccessChecker)(nil)
