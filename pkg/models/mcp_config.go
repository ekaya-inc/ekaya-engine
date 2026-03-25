package models

import (
	"time"

	"github.com/google/uuid"
)

// ToolGroupConfig represents the configuration for a single tool group.
// The `tools` group uses per-app toggles, while `agent_tools` uses Enabled.
type ToolGroupConfig struct {
	// Per-app toggles under toolGroups["tools"].
	AddDirectDatabaseAccess     bool `json:"addDirectDatabaseAccess"`
	AddOntologyMaintenanceTools bool `json:"addOntologyMaintenanceTools"`
	AddOntologySuggestions      bool `json:"addOntologySuggestions"`
	AddApprovalTools            bool `json:"addApprovalTools"`
	AddRequestTools             bool `json:"addRequestTools"`

	// Enabled is only used by groups with a binary on/off switch, currently agent_tools.
	Enabled bool `json:"enabled,omitempty"`
}

// MCPConfig represents the MCP server configuration for a project.
type MCPConfig struct {
	ProjectID          uuid.UUID                   `json:"project_id"`
	ToolGroups         map[string]*ToolGroupConfig `json:"tool_groups"`
	AuditRetentionDays *int                        `json:"audit_retention_days"` // nil = use default (90 days)
	CreatedAt          time.Time                   `json:"created_at"`
	UpdatedAt          time.Time                   `json:"updated_at"`
}

// DefaultMCPConfig returns the default MCP configuration for a new project.
// With role-based tool filtering, groups no longer have enable/disable toggles.
// Sub-options default to true (maximally permissive).
func DefaultMCPConfig(projectID uuid.UUID) *MCPConfig {
	return &MCPConfig{
		ProjectID: projectID,
		ToolGroups: map[string]*ToolGroupConfig{
			// Per-app toggles — all default to enabled
			"tools": {
				AddDirectDatabaseAccess:     true,
				AddOntologyMaintenanceTools: true,
				AddOntologySuggestions:      true,
				AddApprovalTools:            true,
				AddRequestTools:             true,
			},
			// Agent tools are enabled by default.
			"agent_tools": {Enabled: true},
		},
	}
}
