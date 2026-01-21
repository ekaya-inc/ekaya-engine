package models

import (
	"time"

	"github.com/google/uuid"
)

// ToolGroupConfig represents the configuration for a single tool group.
type ToolGroupConfig struct {
	Enabled bool `json:"enabled"`

	// Business Tools options
	// AllowOntologyMaintenance: When enabled, MCP clients can update ontology (entities, relationships, etc.)
	AllowOntologyMaintenance bool `json:"allowOntologyMaintenance,omitempty"`

	// Developer Tools options
	// AddQueryTools: When enabled, adds Query loadout (schema exploration and querying)
	AddQueryTools bool `json:"addQueryTools,omitempty"`
	// AddOntologyMaintenance: When enabled, adds Ontology Maintenance + Ontology Questions tools
	AddOntologyMaintenance bool `json:"addOntologyMaintenance,omitempty"`

	// Custom Tools options
	// CustomTools: List of individually selected tool names (only used when custom group is enabled)
	CustomTools []string `json:"customTools,omitempty"`

	// Legacy fields (kept for backward compatibility during migration)
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
func DefaultMCPConfig(projectID uuid.UUID) *MCPConfig {
	return &MCPConfig{
		ProjectID: projectID,
		ToolGroups: map[string]*ToolGroupConfig{
			"approved_queries": {Enabled: false},
			"agent_tools":      {Enabled: false},
			"developer":        {Enabled: false},
			"custom":           {Enabled: false},
		},
	}
}
