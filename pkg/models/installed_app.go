package models

import (
	"time"

	"github.com/google/uuid"
)

// AppID constants for known applications.
const (
	AppIDMCPServer     = "mcp-server"
	AppIDAIDataLiaison = "ai-data-liaison"
)

// KnownAppIDs is the set of valid app identifiers.
var KnownAppIDs = map[string]bool{
	AppIDMCPServer:     true,
	AppIDAIDataLiaison: true,
}

// InstalledApp represents an application installed for a project.
type InstalledApp struct {
	ID          uuid.UUID              `json:"id"`
	ProjectID   uuid.UUID              `json:"project_id"`
	AppID       string                 `json:"app_id"`
	InstalledAt time.Time              `json:"installed_at"`
	InstalledBy string                 `json:"installed_by,omitempty"`
	Settings    map[string]any `json:"settings"`
}
