// Package audit provides security audit logging for SIEM consumption.
// It logs security-relevant events in structured JSON format for easy parsing
// and integration with security information and event management systems.
package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// SecurityEventType categorizes security-relevant events for filtering and alerting.
type SecurityEventType string

const (
	// EventSQLInjectionAttempt is logged when libinjection detects SQL injection patterns.
	EventSQLInjectionAttempt SecurityEventType = "sql_injection_attempt"
	// EventParameterValidation is logged when parameter validation fails.
	EventParameterValidation SecurityEventType = "parameter_validation_failure"
	// EventQueryExecution is logged for successful query execution (optional, can be high volume).
	EventQueryExecution SecurityEventType = "query_execution"
)

// SecurityEvent represents an auditable security event with all relevant context
// for SIEM ingestion and analysis.
type SecurityEvent struct {
	Timestamp time.Time         `json:"timestamp"`
	EventType SecurityEventType `json:"event_type"`
	ProjectID uuid.UUID         `json:"project_id"`
	QueryID   uuid.UUID         `json:"query_id,omitempty"`
	UserID    string            `json:"user_id,omitempty"`
	ClientIP  string            `json:"client_ip,omitempty"`
	Details   any               `json:"details"`
	Severity  string            `json:"severity"` // info, warning, critical
}

// SQLInjectionDetails contains specifics of a detected SQL injection attempt.
type SQLInjectionDetails struct {
	ParamName   string `json:"param_name"`
	ParamValue  string `json:"param_value"`
	Fingerprint string `json:"fingerprint"` // libinjection fingerprint for pattern analysis
	QueryName   string `json:"query_name"`
}

// SecurityAuditor logs security events for SIEM consumption.
// Events are logged in structured JSON format with appropriate severity levels.
type SecurityAuditor struct {
	logger *zap.Logger
}

// NewSecurityAuditor creates a new security auditor with a dedicated logger namespace.
// The logger is automatically configured with "security_audit" namespace for easy
// filtering in SIEM systems.
func NewSecurityAuditor(logger *zap.Logger) *SecurityAuditor {
	// Create a child logger with security-specific namespace for SIEM parsing
	securityLogger := logger.Named("security_audit")
	return &SecurityAuditor{logger: securityLogger}
}

// LogInjectionAttempt records a detected SQL injection attempt with full context.
// This is logged at ERROR level with "critical" severity for immediate alerting.
//
// The context is used to extract user ID from JWT claims if available.
// Client IP should be extracted from the HTTP request (typically r.RemoteAddr).
//
// Example usage:
//
//	auditor.LogInjectionAttempt(ctx, projectID, queryID,
//	    audit.SQLInjectionDetails{
//	        ParamName:   "search",
//	        ParamValue:  "'; DROP TABLE users--",
//	        Fingerprint: "s&1c",
//	        QueryName:   "Search customers",
//	    },
//	    r.RemoteAddr,
//	)
func (a *SecurityAuditor) LogInjectionAttempt(
	ctx context.Context,
	projectID, queryID uuid.UUID,
	details SQLInjectionDetails,
	clientIP string,
) {
	// Extract user ID from context if available
	userID := auth.GetUserIDFromContext(ctx)

	event := SecurityEvent{
		Timestamp: time.Now().UTC(),
		EventType: EventSQLInjectionAttempt,
		ProjectID: projectID,
		QueryID:   queryID,
		UserID:    userID,
		ClientIP:  clientIP,
		Details:   details,
		Severity:  "critical",
	}

	// Serialize event to JSON for SIEM ingestion
	// Ignoring error as marshaling known types should never fail
	eventJSON, _ := json.Marshal(event)

	// Structured logging for SIEM ingestion
	// Log at ERROR level to ensure visibility in monitoring systems
	a.logger.Error("SQL injection attempt detected",
		zap.String("event_json", string(eventJSON)),
		zap.String("project_id", projectID.String()),
		zap.String("query_id", queryID.String()),
		zap.String("param_name", details.ParamName),
		zap.String("fingerprint", details.Fingerprint),
		zap.String("client_ip", clientIP),
		zap.String("user_id", userID),
		zap.String("severity", "critical"),
	)
}

// LogParameterValidation records a parameter validation failure.
// This is logged at WARN level as these are typically user errors, not attacks.
//
// Example usage:
//
//	auditor.LogParameterValidation(ctx, projectID, queryID,
//	    "customer_id is required but not provided",
//	    r.RemoteAddr,
//	)
func (a *SecurityAuditor) LogParameterValidation(
	ctx context.Context,
	projectID, queryID uuid.UUID,
	errorMessage string,
	clientIP string,
) {
	userID := auth.GetUserIDFromContext(ctx)

	event := SecurityEvent{
		Timestamp: time.Now().UTC(),
		EventType: EventParameterValidation,
		ProjectID: projectID,
		QueryID:   queryID,
		UserID:    userID,
		ClientIP:  clientIP,
		Details: map[string]string{
			"error": errorMessage,
		},
		Severity: "warning",
	}

	eventJSON, _ := json.Marshal(event)

	a.logger.Warn("Parameter validation failed",
		zap.String("event_json", string(eventJSON)),
		zap.String("project_id", projectID.String()),
		zap.String("query_id", queryID.String()),
		zap.String("error", errorMessage),
		zap.String("client_ip", clientIP),
		zap.String("user_id", userID),
		zap.String("severity", "warning"),
	)
}

// LogQueryExecution records a successful query execution for audit trail.
// This is logged at INFO level and can be enabled/disabled based on audit requirements.
// Note: This can generate high log volume in production.
//
// Example usage:
//
//	auditor.LogQueryExecution(ctx, projectID, queryID, "Search customers", r.RemoteAddr)
func (a *SecurityAuditor) LogQueryExecution(
	ctx context.Context,
	projectID, queryID uuid.UUID,
	queryName string,
	clientIP string,
) {
	userID := auth.GetUserIDFromContext(ctx)

	event := SecurityEvent{
		Timestamp: time.Now().UTC(),
		EventType: EventQueryExecution,
		ProjectID: projectID,
		QueryID:   queryID,
		UserID:    userID,
		ClientIP:  clientIP,
		Details: map[string]string{
			"query_name": queryName,
		},
		Severity: "info",
	}

	eventJSON, _ := json.Marshal(event)

	a.logger.Info("Query executed",
		zap.String("event_json", string(eventJSON)),
		zap.String("project_id", projectID.String()),
		zap.String("query_id", queryID.String()),
		zap.String("query_name", queryName),
		zap.String("client_ip", clientIP),
		zap.String("user_id", userID),
		zap.String("severity", "info"),
	)
}
