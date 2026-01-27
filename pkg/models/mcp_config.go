package models

import (
	"time"

	"github.com/google/uuid"
)

// ToolGroupConfig represents the configuration for a single tool group.
// Tool groups no longer have enable/disable toggles - tools are filtered by JWT role.
// The Enabled field is kept for backward compatibility but is ignored.
type ToolGroupConfig struct {
	// User Tools options
	// AllowOntologyMaintenance: When enabled, MCP clients with user role can update ontology
	// (entities, relationships, etc.). Defaults to true.
	AllowOntologyMaintenance bool `json:"allowOntologyMaintenance"`

	// Developer Tools options
	// AddQueryTools: When enabled, adds Query loadout (schema exploration and querying)
	AddQueryTools bool `json:"addQueryTools"`
	// AddOntologyMaintenance: When enabled, adds Ontology Maintenance + Ontology Questions tools
	AddOntologyMaintenance bool `json:"addOntologyMaintenance"`

	// Custom Tools options
	// CustomTools: List of individually selected tool names (only used when custom group is enabled)
	CustomTools []string `json:"customTools,omitempty"`

	// Legacy fields (kept for backward compatibility during migration)
	// Enabled is no longer used for tool selection - tools are filtered by JWT role
	Enabled                bool `json:"enabled,omitempty"`
	ForceMode              bool `json:"forceMode,omitempty"`
	AllowClientSuggestions bool `json:"allowClientSuggestions,omitempty"`
}

// MCPConfig represents the MCP server configuration for a project.
type MCPConfig struct {
	ProjectID            uuid.UUID                   `json:"project_id"`
	ToolGroups           map[string]*ToolGroupConfig `json:"tool_groups"`
	AgentAPIKeyEncrypted string                      `json:"-"` // Never serialize - encrypted at rest
	CreatedAt            time.Time                   `json:"created_at"`
	UpdatedAt            time.Time                   `json:"updated_at"`
}

// DefaultMCPConfig returns the default MCP configuration for a new project.
// With role-based tool filtering, groups no longer have enable/disable toggles.
// Sub-options default to true (maximally permissive).
func DefaultMCPConfig(projectID uuid.UUID) *MCPConfig {
	return &MCPConfig{
		ProjectID: projectID,
		ToolGroups: map[string]*ToolGroupConfig{
			// User tools - allowOntologyMaintenance defaults to true
			"user": {AllowOntologyMaintenance: true},
			// Developer tools - both sub-options default to true
			"developer": {AddQueryTools: true, AddOntologyMaintenance: true},
			// Agent tools - enabled by default (Enabled is legacy but still checked for agents)
			"agent_tools": {Enabled: true},
		},
	}
}
