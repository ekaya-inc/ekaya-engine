package mcp

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/database"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

// AuditLogger writes MCP audit events to the database asynchronously.
type AuditLogger struct {
	db     *database.DB
	logger *zap.Logger

	// startTimes tracks when tool calls begin, keyed by request ID.
	startTimes sync.Map
}

// NewAuditLogger creates an AuditLogger that records MCP events.
func NewAuditLogger(db *database.DB, logger *zap.Logger) *AuditLogger {
	return &AuditLogger{
		db:     db,
		logger: logger.Named("mcp-audit"),
	}
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
	event.EventType = "tool_call"
	event.WasSuccessful = true
	event.DurationMs = &durationMs
	event.ResultSummary = summarizeResult(result)

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
	event.EventType = "tool_error"
	event.WasSuccessful = false
	event.DurationMs = &durationMs

	errMsg := err.Error()
	event.ErrorMessage = &errMsg

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
		SecurityLevel: "normal",
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
	}
}

// sanitizeParams sanitizes request parameters before storing.
// Truncates large string values and redacts potentially sensitive data.
func sanitizeParams(args any) map[string]any {
	params, ok := args.(map[string]any)
	if !ok || len(params) == 0 {
		return nil
	}

	sanitized := make(map[string]any, len(params))
	for k, v := range params {
		switch val := v.(type) {
		case string:
			// Truncate SQL > 10KB
			if len(val) > 10240 {
				sanitized[k] = val[:10240] + "...[truncated]"
			} else {
				sanitized[k] = val
			}
		default:
			sanitized[k] = v
		}
	}
	return sanitized
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
