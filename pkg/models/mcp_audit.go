package models

import (
	"time"

	"github.com/google/uuid"
)

// MCPAuditEvent represents a single entry in the MCP audit log.
type MCPAuditEvent struct {
	ID        uuid.UUID `json:"id"`
	ProjectID uuid.UUID `json:"project_id"`

	// Who
	UserID    string  `json:"user_id"`
	UserEmail *string `json:"user_email,omitempty"`
	SessionID *string `json:"session_id,omitempty"`

	// What
	EventType string  `json:"event_type"`
	ToolName  *string `json:"tool_name,omitempty"`

	// Request details
	RequestParams   map[string]any `json:"request_params,omitempty"`
	NaturalLanguage *string        `json:"natural_language,omitempty"`
	SQLQuery        *string        `json:"sql_query,omitempty"`

	// Response details
	WasSuccessful bool            `json:"was_successful"`
	ErrorMessage  *string         `json:"error_message,omitempty"`
	ResultSummary map[string]any  `json:"result_summary,omitempty"`

	// Performance
	DurationMs *int `json:"duration_ms,omitempty"`

	// Security classification
	SecurityLevel string   `json:"security_level"`
	SecurityFlags []string `json:"security_flags,omitempty"`

	// Context
	ClientInfo map[string]any `json:"client_info,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// MCPAuditEventFilters contains filters for querying MCP audit events.
type MCPAuditEventFilters struct {
	AuditPageFilters
	UserID        string
	EventType     string
	ToolName      string
	SecurityLevel string
}
