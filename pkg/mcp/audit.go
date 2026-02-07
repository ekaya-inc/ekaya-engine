package mcp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/mcp/tools"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// AuditLogger writes MCP audit events to the database asynchronously.
type AuditLogger struct {
	db           *database.DB
	logger       *zap.Logger
	alertTrigger AlertEventEvaluator

	// startTimes tracks when tool calls begin, keyed by request ID.
	startTimes sync.Map
}

// AlertEventEvaluator evaluates audit events for alert conditions.
// Defined here to avoid a circular dependency between mcp and services packages.
type AlertEventEvaluator interface {
	EvaluateEvent(ctx context.Context, event *models.MCPAuditEvent) error
}

// NewAuditLogger creates an AuditLogger that records MCP events.
func NewAuditLogger(db *database.DB, logger *zap.Logger) *AuditLogger {
	return &AuditLogger{
		db:     db,
		logger: logger.Named("mcp-audit"),
	}
}

// SetAlertTrigger configures an alert trigger service to evaluate events after recording.
// This is set after construction to avoid circular dependency issues during DI wiring.
func (a *AuditLogger) SetAlertTrigger(trigger AlertEventEvaluator) {
	a.alertTrigger = trigger
}

// Hooks returns mcp-go Hooks configured to capture tool call events.
func (a *AuditLogger) Hooks() *server.Hooks {
	hooks := &server.Hooks{}
	hooks.AddBeforeCallTool(a.beforeCallTool)
	hooks.AddAfterCallTool(a.afterCallTool)
	hooks.AddOnError(a.onError)
	return hooks
}

func (a *AuditLogger) beforeCallTool(_ context.Context, id any, _ *mcplib.CallToolRequest) {
	a.startTimes.Store(id, time.Now())
}

func (a *AuditLogger) afterCallTool(ctx context.Context, id any, req *mcplib.CallToolRequest, result *mcplib.CallToolResult) {
	startTime, _ := a.loadAndDeleteStart(id)
	durationMs := int(time.Since(startTime).Milliseconds())

	event := a.buildEvent(ctx, req)
	event.EventType = models.MCPEventToolCall
	event.WasSuccessful = true
	event.DurationMs = &durationMs
	event.ResultSummary = summarizeResult(result)

	// Classify security level based on result content
	classifyToolCallSecurity(event, result)

	go a.record(event)
}

func (a *AuditLogger) onError(ctx context.Context, id any, method mcplib.MCPMethod, message any, err error) {
	if method != mcplib.MethodToolsCall {
		return
	}

	req, ok := message.(*mcplib.CallToolRequest)
	if !ok {
		return
	}

	startTime, _ := a.loadAndDeleteStart(id)
	durationMs := int(time.Since(startTime).Milliseconds())

	event := a.buildEvent(ctx, req)
	event.EventType = models.MCPEventToolError
	event.WasSuccessful = false
	event.DurationMs = &durationMs

	errMsg := err.Error()
	event.ErrorMessage = &errMsg

	// Classify security level based on error content
	classifyErrorSecurity(event, errMsg)

	go a.record(event)
}

func (a *AuditLogger) loadAndDeleteStart(id any) (time.Time, bool) {
	if v, ok := a.startTimes.LoadAndDelete(id); ok {
		return v.(time.Time), true
	}
	return time.Now(), false
}

func (a *AuditLogger) buildEvent(ctx context.Context, req *mcplib.CallToolRequest) *models.MCPAuditEvent {
	event := &models.MCPAuditEvent{
		SecurityLevel: models.MCPSecurityNormal,
	}

	// Extract tool name
	toolName := req.Params.Name
	event.ToolName = &toolName

	// Extract request params (sanitized)
	event.RequestParams = sanitizeParams(req.Params.Arguments)

	// Extract user info from auth claims
	if claims, ok := auth.GetClaims(ctx); ok {
		event.UserID = claims.Subject
		if claims.Email != "" {
			event.UserEmail = &claims.Email
		}
		if pid, err := uuid.Parse(claims.ProjectID); err == nil {
			event.ProjectID = pid
		}
	}

	return event
}

// record writes the audit event to the database asynchronously.
// Uses a fresh database connection to avoid races with the caller's connection.
func (a *AuditLogger) record(event *models.MCPAuditEvent) {
	if event.ProjectID == uuid.Nil {
		a.logger.Warn("Skipping MCP audit event: no project ID")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	scope, err := a.db.WithTenant(ctx, event.ProjectID)
	if err != nil {
		a.logger.Warn("Failed to record MCP audit event: could not acquire tenant scope",
			zap.Error(err),
			zap.String("project_id", event.ProjectID.String()))
		return
	}
	defer scope.Close()

	tenantCtx := database.SetTenantScope(ctx, scope)

	requestParamsJSON := marshalJSON(event.RequestParams)
	resultSummaryJSON := marshalJSON(event.ResultSummary)
	clientInfoJSON := marshalJSON(event.ClientInfo)

	query := `
		INSERT INTO engine_mcp_audit_log (
			project_id, user_id, user_email, session_id,
			event_type, tool_name,
			request_params, natural_language, sql_query,
			was_successful, error_message, result_summary,
			duration_ms, security_level, security_flags,
			client_info
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	_, err = scope.Conn.Exec(tenantCtx, query,
		event.ProjectID,
		event.UserID,
		event.UserEmail,
		event.SessionID,
		event.EventType,
		event.ToolName,
		requestParamsJSON,
		event.NaturalLanguage,
		event.SQLQuery,
		event.WasSuccessful,
		event.ErrorMessage,
		resultSummaryJSON,
		event.DurationMs,
		event.SecurityLevel,
		event.SecurityFlags,
		clientInfoJSON,
	)
	if err != nil {
		a.logger.Error("Failed to record MCP audit event",
			zap.Error(err),
			zap.String("project_id", event.ProjectID.String()),
			zap.String("event_type", event.EventType))
		return
	}

	// Evaluate alert triggers after the audit event is recorded.
	// Runs in the same goroutine (already async) — does not block MCP responses.
	if a.alertTrigger != nil {
		if triggerErr := a.alertTrigger.EvaluateEvent(tenantCtx, event); triggerErr != nil {
			a.logger.Error("Alert trigger evaluation failed",
				zap.Error(triggerErr),
				zap.String("project_id", event.ProjectID.String()),
				zap.String("event_type", event.EventType))
		}
	}
}

// maxSQLSize is the maximum size of SQL strings stored in audit logs.
const maxSQLSize = 10240 // 10KB

// sqlStringLiteralPattern matches SQL string literals: 'value', 'it”s escaped', etc.
// Handles escaped single quotes within strings.
var sqlStringLiteralPattern = regexp.MustCompile(`'(?:[^']*(?:'')?)*[^']*'`)

// sensitiveParamDetector is used to identify sensitive parameter keys in audit logs.
var sensitiveParamDetector = tools.DefaultSensitiveDetector

// sanitizeParams sanitizes request parameters before storing in the audit log.
// Applies: SQL truncation, string literal redaction, sensitive value hashing.
func sanitizeParams(args any) map[string]any {
	params, ok := args.(map[string]any)
	if !ok || len(params) == 0 {
		return nil
	}

	sanitized := make(map[string]any, len(params))
	for k, v := range params {
		sanitized[k] = sanitizeValue(k, v)
	}
	return sanitized
}

// sanitizeValue applies the appropriate sanitization based on key name and value type.
func sanitizeValue(key string, value any) any {
	// Hash values for sensitive parameter keys (password, token, api_key, etc.)
	if sensitiveParamDetector.IsSensitiveColumn(key) {
		return hashSensitiveValue(value)
	}

	switch val := value.(type) {
	case string:
		return sanitizeStringParam(key, val)
	case map[string]any:
		return sanitizeNestedParams(val)
	default:
		return value
	}
}

// sanitizeStringParam handles string values: truncates SQL and redacts string literals.
func sanitizeStringParam(key string, val string) string {
	// Truncate > 10KB
	if len(val) > maxSQLSize {
		val = val[:maxSQLSize] + "...[truncated]"
	}

	// Redact SQL string literals in SQL-like parameters
	if isSQLParam(key) {
		val = redactSQLStringLiterals(val)
	}

	return val
}

// sanitizeNestedParams recursively sanitizes nested map parameters,
// preserving structure but hiding sensitive values.
func sanitizeNestedParams(params map[string]any) map[string]any {
	sanitized := make(map[string]any, len(params))
	for k, v := range params {
		sanitized[k] = sanitizeValue(k, v)
	}
	return sanitized
}

// isSQLParam returns true if a parameter key likely contains SQL.
func isSQLParam(key string) bool {
	lower := strings.ToLower(key)
	return lower == "sql" || lower == "query" || strings.HasSuffix(lower, "_sql") || strings.HasSuffix(lower, "_query")
}

// redactSQLStringLiterals replaces string literal values in SQL with '***',
// preserving the query structure for debugging while hiding user-provided values.
func redactSQLStringLiterals(sql string) string {
	return sqlStringLiteralPattern.ReplaceAllString(sql, "'***'")
}

// hashSensitiveValue returns a SHA-256 hash prefix for sensitive values,
// allowing correlation across audit entries without storing the actual value.
func hashSensitiveValue(value any) string {
	var str string
	switch v := value.(type) {
	case string:
		str = v
	default:
		str = fmt.Sprintf("%v", v)
	}
	hash := sha256.Sum256([]byte(str))
	return "sha256:" + hex.EncodeToString(hash[:8]) // First 8 bytes = 16 hex chars
}

// summarizeResult creates a compact summary of the tool result.
func summarizeResult(result *mcplib.CallToolResult) map[string]any {
	if result == nil {
		return nil
	}

	summary := map[string]any{
		"is_error": result.IsError,
	}

	if len(result.Content) > 0 {
		summary["content_count"] = len(result.Content)
		// Include a truncated preview of the first text content
		for _, c := range result.Content {
			if tc, ok := c.(mcplib.TextContent); ok {
				text := tc.Text

				// Extract row_count from JSON responses for alert evaluation
				extractRowCount(text, summary)

				if len(text) > 200 {
					text = text[:200] + "...[truncated]"
				}
				summary["preview"] = text
				break
			}
		}
	}

	return summary
}

// extractRowCount attempts to extract the row_count field from a JSON text response
// and stores it in the summary map. Used by the alert trigger service to detect
// large data exports without parsing the full response.
func extractRowCount(text string, summary map[string]any) {
	var partial struct {
		RowCount *int `json:"row_count"`
	}
	if err := json.Unmarshal([]byte(text), &partial); err == nil && partial.RowCount != nil {
		summary["row_count"] = *partial.RowCount
	}
}

// RecordAuthFailure logs a failed MCP authentication attempt.
// Called from the MCP auth middleware when authentication fails.
func (a *AuditLogger) RecordAuthFailure(projectID uuid.UUID, userID, reason, clientIP string) {
	event := &models.MCPAuditEvent{
		ProjectID:     projectID,
		UserID:        userID,
		EventType:     models.MCPEventAuthFailure,
		WasSuccessful: false,
		ErrorMessage:  &reason,
		SecurityLevel: models.MCPSecurityWarning,
		SecurityFlags: []string{"auth_failure"},
		ClientInfo: map[string]any{
			"client_ip": clientIP,
		},
	}

	go a.record(event)
}

// classifyToolCallSecurity inspects a successful tool result to detect security-relevant
// patterns (e.g., injection detection reported as a successful MCP response with error JSON).
func classifyToolCallSecurity(event *models.MCPAuditEvent, result *mcplib.CallToolResult) {
	if result == nil || !result.IsError {
		return
	}

	// Check result content for security-relevant error codes
	for _, c := range result.Content {
		tc, ok := c.(mcplib.TextContent)
		if !ok {
			continue
		}
		text := strings.ToLower(tc.Text)

		if strings.Contains(text, "security_violation") || strings.Contains(text, "injection") {
			event.EventType = models.MCPEventSQLInjectionAttempt
			event.SecurityLevel = models.MCPSecurityCritical
			event.SecurityFlags = append(event.SecurityFlags, "sql_injection_attempt")
			return
		}
		if strings.Contains(text, "tool_not_enabled") || strings.Contains(text, "authentication_required") {
			event.SecurityLevel = models.MCPSecurityWarning
			event.SecurityFlags = append(event.SecurityFlags, "unauthorized_access")
			return
		}
	}
}

// classifyErrorSecurity inspects an error message to detect security-relevant patterns
// and upgrades the event's security classification accordingly.
func classifyErrorSecurity(event *models.MCPAuditEvent, errMsg string) {
	lower := strings.ToLower(errMsg)

	if strings.Contains(lower, "injection") || strings.Contains(lower, "sql injection") {
		event.EventType = models.MCPEventSQLInjectionAttempt
		event.SecurityLevel = models.MCPSecurityCritical
		event.SecurityFlags = append(event.SecurityFlags, "sql_injection_attempt")
	} else if strings.Contains(lower, "authentication") || strings.Contains(lower, "unauthorized") {
		event.SecurityLevel = models.MCPSecurityWarning
		event.SecurityFlags = append(event.SecurityFlags, "auth_failure")
	} else if strings.Contains(lower, "rate limit") {
		event.EventType = models.MCPEventRateLimitHit
		event.SecurityLevel = models.MCPSecurityWarning
		event.SecurityFlags = append(event.SecurityFlags, "rate_limit")
	}
}

// marshalJSON converts a map to JSON bytes, returning nil for empty/nil maps.
func marshalJSON(m map[string]any) []byte {
	if len(m) == 0 {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}
