package models

import (
	"time"

	"github.com/google/uuid"
)

// AppID constants for known applications.
const (
	AppIDMCPServer     = "mcp-server"
	AppIDOntologyForge = "ontology-forge"
	AppIDAIDataLiaison = "ai-data-liaison"
	AppIDAIAgents      = "ai-agents"
	AppIDFileLoader    = "file-loader"
	AppIDMCPTunnel     = "mcp-tunnel"
)

// KnownAppIDs is the set of valid app identifiers.
var KnownAppIDs = map[string]bool{
	AppIDMCPServer:     true,
	AppIDOntologyForge: true,
	AppIDAIDataLiaison: true,
	AppIDAIAgents:      true,
	AppIDFileLoader:    true,
	AppIDMCPTunnel:     true,
}

// InstalledApp represents an application installed for a project.
type InstalledApp struct {
	ID          uuid.UUID      `json:"id"`
	ProjectID   uuid.UUID      `json:"project_id"`
	AppID       string         `json:"app_id"`
	InstalledAt time.Time      `json:"installed_at"`
	InstalledBy string         `json:"installed_by,omitempty"`
	ActivatedAt *time.Time     `json:"activated_at,omitempty"`
	Settings    map[string]any `json:"settings"`
}
