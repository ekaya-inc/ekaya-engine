package models

import (
	"time"

	"github.com/google/uuid"
)

// ToolGroupConfig represents the configuration for a single tool group.
type ToolGroupConfig struct {
	Enabled       bool `json:"enabled"`
	EnableExecute bool `json:"enableExecute"`
}

// MCPConfig represents the MCP server configuration for a project.
type MCPConfig struct {
	ProjectID  uuid.UUID                   `json:"project_id"`
	ToolGroups map[string]*ToolGroupConfig `json:"tool_groups"`
	CreatedAt  time.Time                   `json:"created_at"`
	UpdatedAt  time.Time                   `json:"updated_at"`
}

// DefaultMCPConfig returns the default MCP configuration for a new project.
func DefaultMCPConfig(projectID uuid.UUID) *MCPConfig {
	return &MCPConfig{
		ProjectID: projectID,
		ToolGroups: map[string]*ToolGroupConfig{
			"developer": {Enabled: false},
		},
	}
}
