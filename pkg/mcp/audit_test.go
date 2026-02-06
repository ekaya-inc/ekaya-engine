package mcp

import (
	"testing"

	mcplib "github.com/mark3labs/mcp-go/mcp"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
)

func TestClassifyToolCallSecurity_NilResult(t *testing.T) {
	event := &models.MCPAuditEvent{
		SecurityLevel: models.MCPSecurityNormal,
	}
	classifyToolCallSecurity(event, nil)

	if event.SecurityLevel != models.MCPSecurityNormal {
		t.Errorf("expected security level %q, got %q", models.MCPSecurityNormal, event.SecurityLevel)
	}
}

func TestClassifyToolCallSecurity_NonErrorResult(t *testing.T) {
	event := &models.MCPAuditEvent{
		SecurityLevel: models.MCPSecurityNormal,
	}
	result := &mcplib.CallToolResult{IsError: false}
	classifyToolCallSecurity(event, result)

	if event.SecurityLevel != models.MCPSecurityNormal {
		t.Errorf("expected security level %q, got %q", models.MCPSecurityNormal, event.SecurityLevel)
	}
}

func TestClassifyToolCallSecurity_InjectionDetected(t *testing.T) {
	event := &models.MCPAuditEvent{
		EventType:     models.MCPEventToolCall,
		SecurityLevel: models.MCPSecurityNormal,
	}
	result := &mcplib.CallToolResult{
		IsError: true,
		Content: []mcplib.Content{
			mcplib.TextContent{Text: `{"error_code":"security_violation","message":"SQL injection detected"}`},
		},
	}
	classifyToolCallSecurity(event, result)

	if event.EventType != models.MCPEventSQLInjectionAttempt {
		t.Errorf("expected event type %q, got %q", models.MCPEventSQLInjectionAttempt, event.EventType)
	}
	if event.SecurityLevel != models.MCPSecurityCritical {
		t.Errorf("expected security level %q, got %q", models.MCPSecurityCritical, event.SecurityLevel)
	}
	if len(event.SecurityFlags) != 1 || event.SecurityFlags[0] != "sql_injection_attempt" {
		t.Errorf("expected security flags [sql_injection_attempt], got %v", event.SecurityFlags)
	}
}

func TestClassifyToolCallSecurity_ToolNotEnabled(t *testing.T) {
	event := &models.MCPAuditEvent{
		EventType:     models.MCPEventToolCall,
		SecurityLevel: models.MCPSecurityNormal,
	}
	result := &mcplib.CallToolResult{
		IsError: true,
		Content: []mcplib.Content{
			mcplib.TextContent{Text: `{"error_code":"tool_not_enabled","message":"query tool is not enabled"}`},
		},
	}
	classifyToolCallSecurity(event, result)

	if event.SecurityLevel != models.MCPSecurityWarning {
		t.Errorf("expected security level %q, got %q", models.MCPSecurityWarning, event.SecurityLevel)
	}
	if len(event.SecurityFlags) != 1 || event.SecurityFlags[0] != "unauthorized_access" {
		t.Errorf("expected security flags [unauthorized_access], got %v", event.SecurityFlags)
	}
}

func TestClassifyErrorSecurity_InjectionError(t *testing.T) {
	event := &models.MCPAuditEvent{
		EventType:     models.MCPEventToolError,
		SecurityLevel: models.MCPSecurityNormal,
	}
	classifyErrorSecurity(event, "SQL injection attempt detected in parameter 'search'")

	if event.EventType != models.MCPEventSQLInjectionAttempt {
		t.Errorf("expected event type %q, got %q", models.MCPEventSQLInjectionAttempt, event.EventType)
	}
	if event.SecurityLevel != models.MCPSecurityCritical {
		t.Errorf("expected security level %q, got %q", models.MCPSecurityCritical, event.SecurityLevel)
	}
}

func TestClassifyErrorSecurity_AuthError(t *testing.T) {
	event := &models.MCPAuditEvent{
		EventType:     models.MCPEventToolError,
		SecurityLevel: models.MCPSecurityNormal,
	}
	classifyErrorSecurity(event, "authentication required")

	if event.SecurityLevel != models.MCPSecurityWarning {
		t.Errorf("expected security level %q, got %q", models.MCPSecurityWarning, event.SecurityLevel)
	}
	if len(event.SecurityFlags) != 1 || event.SecurityFlags[0] != "auth_failure" {
		t.Errorf("expected security flags [auth_failure], got %v", event.SecurityFlags)
	}
}

func TestClassifyErrorSecurity_RateLimitError(t *testing.T) {
	event := &models.MCPAuditEvent{
		EventType:     models.MCPEventToolError,
		SecurityLevel: models.MCPSecurityNormal,
	}
	classifyErrorSecurity(event, "rate limit exceeded for user")

	if event.EventType != models.MCPEventRateLimitHit {
		t.Errorf("expected event type %q, got %q", models.MCPEventRateLimitHit, event.EventType)
	}
	if event.SecurityLevel != models.MCPSecurityWarning {
		t.Errorf("expected security level %q, got %q", models.MCPSecurityWarning, event.SecurityLevel)
	}
}

func TestClassifyErrorSecurity_NormalError(t *testing.T) {
	event := &models.MCPAuditEvent{
		EventType:     models.MCPEventToolError,
		SecurityLevel: models.MCPSecurityNormal,
	}
	classifyErrorSecurity(event, "failed to connect to database")

	// Should remain unchanged for normal errors
	if event.EventType != models.MCPEventToolError {
		t.Errorf("expected event type %q, got %q", models.MCPEventToolError, event.EventType)
	}
	if event.SecurityLevel != models.MCPSecurityNormal {
		t.Errorf("expected security level %q, got %q", models.MCPSecurityNormal, event.SecurityLevel)
	}
}

func TestSanitizeParams_TruncatesLargeSQL(t *testing.T) {
	largeSQL := make([]byte, 20000)
	for i := range largeSQL {
		largeSQL[i] = 'a'
	}

	params := map[string]any{
		"sql": string(largeSQL),
	}

	result := sanitizeParams(params)
	sqlVal, ok := result["sql"].(string)
	if !ok {
		t.Fatal("expected sql to be a string")
	}

	// 10240 + len("...[truncated]") = 10254
	expectedLen := 10240 + len("...[truncated]")
	if len(sqlVal) != expectedLen {
		t.Errorf("expected truncated length %d, got %d", expectedLen, len(sqlVal))
	}
}

func TestSanitizeParams_PreservesSmallValues(t *testing.T) {
	params := map[string]any{
		"sql":   "SELECT 1",
		"limit": 100,
	}

	result := sanitizeParams(params)

	if result["sql"] != "SELECT 1" {
		t.Errorf("expected sql to be preserved, got %v", result["sql"])
	}
	if result["limit"] != 100 {
		t.Errorf("expected limit to be preserved, got %v", result["limit"])
	}
}

func TestSanitizeParams_NilInput(t *testing.T) {
	result := sanitizeParams(nil)
	if result != nil {
		t.Errorf("expected nil for nil input, got %v", result)
	}
}

func TestSummarizeResult_NilResult(t *testing.T) {
	result := summarizeResult(nil)
	if result != nil {
		t.Errorf("expected nil for nil result, got %v", result)
	}
}

func TestSummarizeResult_WithContent(t *testing.T) {
	result := summarizeResult(&mcplib.CallToolResult{
		IsError: false,
		Content: []mcplib.Content{
			mcplib.TextContent{Text: "hello world"},
		},
	})

	if result["is_error"] != false {
		t.Errorf("expected is_error=false, got %v", result["is_error"])
	}
	if result["content_count"] != 1 {
		t.Errorf("expected content_count=1, got %v", result["content_count"])
	}
	if result["preview"] != "hello world" {
		t.Errorf("expected preview='hello world', got %v", result["preview"])
	}
}

func TestSummarizeResult_TruncatesLongPreview(t *testing.T) {
	longText := make([]byte, 500)
	for i := range longText {
		longText[i] = 'x'
	}

	result := summarizeResult(&mcplib.CallToolResult{
		Content: []mcplib.Content{
			mcplib.TextContent{Text: string(longText)},
		},
	})

	preview, ok := result["preview"].(string)
	if !ok {
		t.Fatal("expected preview to be a string")
	}

	expectedLen := 200 + len("...[truncated]")
	if len(preview) != expectedLen {
		t.Errorf("expected preview length %d, got %d", expectedLen, len(preview))
	}
}

func TestEventTypeConstants(t *testing.T) {
	// Verify all event type constants have expected string values
	testCases := []struct {
		name     string
		constant string
		expected string
	}{
		{"MCPEventToolCall", models.MCPEventToolCall, "tool_call"},
		{"MCPEventToolError", models.MCPEventToolError, "tool_error"},
		{"MCPEventToolSuccess", models.MCPEventToolSuccess, "tool_success"},
		{"MCPEventAuthFailure", models.MCPEventAuthFailure, "mcp_auth_failure"},
		{"MCPEventQueryExecuted", models.MCPEventQueryExecuted, "query_executed"},
		{"MCPEventQueryBlocked", models.MCPEventQueryBlocked, "query_blocked"},
		{"MCPEventApprovedQueryExecuted", models.MCPEventApprovedQueryExecuted, "approved_query_executed"},
		{"MCPEventSQLInjectionAttempt", models.MCPEventSQLInjectionAttempt, "sql_injection_attempt"},
		{"MCPEventRateLimitHit", models.MCPEventRateLimitHit, "rate_limit_hit"},
		{"MCPEventUnauthorizedTableAccess", models.MCPEventUnauthorizedTableAccess, "unauthorized_table_access"},
		{"MCPEventSensitiveDataAccess", models.MCPEventSensitiveDataAccess, "sensitive_data_access"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.constant != tc.expected {
				t.Errorf("expected %q = %q, got %q", tc.name, tc.expected, tc.constant)
			}
		})
	}
}

func TestSecurityLevelConstants(t *testing.T) {
	if models.MCPSecurityNormal != "normal" {
		t.Errorf("expected MCPSecurityNormal = 'normal', got %q", models.MCPSecurityNormal)
	}
	if models.MCPSecurityWarning != "warning" {
		t.Errorf("expected MCPSecurityWarning = 'warning', got %q", models.MCPSecurityWarning)
	}
	if models.MCPSecurityCritical != "critical" {
		t.Errorf("expected MCPSecurityCritical = 'critical', got %q", models.MCPSecurityCritical)
	}
}
