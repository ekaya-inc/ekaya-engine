package models

import (
	"time"

	"github.com/google/uuid"
)

// MCP audit event types define the vocabulary of events captured in engine_mcp_audit_log.
const (
	// Tool invocation events
	MCPEventToolCall    = "tool_call"    // Successful tool invocation (includes duration, result summary)
	MCPEventToolError   = "tool_error"   // Tool invocation that returned an error
	MCPEventToolSuccess = "tool_success" // Alias: successful tool call with detailed result

	// Connection events
	MCPEventAuthFailure = "mcp_auth_failure" // Authentication failure (JWT or API key)

	// Query events
	MCPEventQueryExecuted         = "query_executed"          // Query execution via MCP tool
	MCPEventQueryBlocked          = "query_blocked"           // Query blocked by policy
	MCPEventApprovedQueryExecuted = "approved_query_executed" // Pre-approved query execution

	// Security events
	MCPEventSQLInjectionAttempt     = "sql_injection_attempt"     // SQL injection pattern detected
	MCPEventRateLimitHit            = "rate_limit_hit"            // Rate limit exceeded
	MCPEventUnauthorizedTableAccess = "unauthorized_table_access" // Access to restricted table
	MCPEventSensitiveDataAccess     = "sensitive_data_access"     // Access to sensitive column
)

// MCP audit security levels classify the severity of audit events.
const (
	MCPSecurityNormal   = "normal"
	MCPSecurityWarning  = "warning"
	MCPSecurityCritical = "critical"
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
	WasSuccessful bool           `json:"was_successful"`
	ErrorMessage  *string        `json:"error_message,omitempty"`
	ResultSummary map[string]any `json:"result_summary,omitempty"`

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
