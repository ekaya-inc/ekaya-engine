package models

import (
	"time"

	"github.com/google/uuid"
)

// Agent represents a named AI agent with scoped query access.
type Agent struct {
	ID              uuid.UUID  `json:"id"`
	ProjectID       uuid.UUID  `json:"project_id"`
	Name            string     `json:"name"`
	APIKeyEncrypted string     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	LastAccessAt    *time.Time `json:"last_access_at"`
	MCPCallCount    int64      `json:"mcp_call_count"`
}
