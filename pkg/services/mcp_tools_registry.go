package services

import "github.com/ekaya-inc/ekaya-engine/pkg/models"

// ToolDefinition describes an MCP tool for display purposes.
type ToolDefinition struct {
	Name        string // Tool name as exposed via MCP (e.g., "get_schema")
	Description string // One-line description for UI display
	ToolGroup   string // "developer", "approved_queries", "agent_tools", or "always"
	SubOption   string // Optional sub-option requirement (e.g., "enableExecute")
}

// ToolGroupDeveloper is the developer tools group identifier.
const ToolGroupDeveloper = "developer"

// ToolRegistry contains all MCP tool definitions.
// This is the single source of truth for tool metadata, used by both
// the UI display (GetEnabledTools) and the MCP tool filter (pkg/mcp/tools).
var ToolRegistry = []ToolDefinition{
	// Developer tools
	{Name: "echo", Description: "Echo back input message for testing", ToolGroup: ToolGroupDeveloper},
	{Name: "execute", Description: "Execute DDL/DML statements", ToolGroup: ToolGroupDeveloper, SubOption: "enableExecute"},
	{Name: "get_schema", Description: "Get database schema with entity semantics", ToolGroup: ToolGroupDeveloper},

	// Business user tools (approved_queries group)
	// These read-only query tools enable business users to answer ad-hoc questions
	// when pre-approved queries don't match their request.
	{Name: "query", Description: "Execute read-only SQL SELECT statements", ToolGroup: ToolGroupApprovedQueries},
	{Name: "sample", Description: "Quick data preview from a table", ToolGroup: ToolGroupApprovedQueries},
	{Name: "validate", Description: "Check SQL syntax without executing", ToolGroup: ToolGroupApprovedQueries},
	{Name: "get_ontology", Description: "Get business ontology for query generation", ToolGroup: ToolGroupApprovedQueries},
	{Name: "get_glossary", Description: "Get business glossary terms", ToolGroup: ToolGroupApprovedQueries},
	{Name: "list_approved_queries", Description: "List pre-approved SQL queries", ToolGroup: ToolGroupApprovedQueries},
	{Name: "execute_approved_query", Description: "Execute a pre-approved query by ID", ToolGroup: ToolGroupApprovedQueries},

	// Health is always available
	{Name: "health", Description: "Server health check", ToolGroup: "always"},
}

// agentAllowedTools defines which tools are available when agent_tools is enabled.
// When agent_tools mode is active, only these specific tools are exposed.
var agentAllowedTools = map[string]bool{
	"echo":                   true,
	"list_approved_queries":  true,
	"execute_approved_query": true,
	"health":                 true,
}

// GetEnabledTools returns the list of tools enabled based on the current state.
// This computes which tools would be visible to a user (not an agent) based on
// the tool group configurations.
func GetEnabledTools(state map[string]*models.ToolGroupConfig) []ToolDefinition {
	if state == nil {
		// Only health is available with no state
		return filterAlwaysTools()
	}

	var enabled []ToolDefinition

	// Check if agent_tools is enabled - this changes everything
	agentConfig := state[ToolGroupAgentTools]
	if agentConfig != nil && agentConfig.Enabled {
		// Agent tools mode: only specific tools are available
		for _, tool := range ToolRegistry {
			if agentAllowedTools[tool.Name] {
				enabled = append(enabled, tool)
			}
		}
		return enabled
	}

	// Normal mode: check each tool group

	// Check if force mode is active (hides developer tools)
	aqConfig := state[ToolGroupApprovedQueries]
	forceMode := aqConfig != nil && aqConfig.ForceMode

	// Developer tools
	devConfig := state[ToolGroupDeveloper]
	showDeveloper := devConfig != nil && devConfig.Enabled && !forceMode
	showExecute := showDeveloper && devConfig.EnableExecute

	// Approved queries tools
	showApprovedQueries := aqConfig != nil && aqConfig.Enabled

	for _, tool := range ToolRegistry {
		switch tool.ToolGroup {
		case ToolGroupDeveloper:
			if !showDeveloper {
				continue
			}
			// Check sub-option for execute tool
			if tool.SubOption == "enableExecute" && !showExecute {
				continue
			}
			enabled = append(enabled, tool)

		case ToolGroupApprovedQueries:
			if showApprovedQueries {
				enabled = append(enabled, tool)
			}

		case "always":
			enabled = append(enabled, tool)
		}
	}

	return enabled
}

// filterAlwaysTools returns only tools that are always available.
func filterAlwaysTools() []ToolDefinition {
	var result []ToolDefinition
	for _, tool := range ToolRegistry {
		if tool.ToolGroup == "always" {
			result = append(result, tool)
		}
	}
	return result
}
