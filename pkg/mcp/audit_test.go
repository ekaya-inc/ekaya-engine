package mcp

import (
	"strings"
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

// --- SQL string literal redaction tests ---

func TestSanitizeParams_RedactsSQLStringLiterals(t *testing.T) {
	params := map[string]any{
		"sql": "SELECT * FROM users WHERE name = 'John' AND city = 'New York'",
	}

	result := sanitizeParams(params)
	sqlVal := result["sql"].(string)

	expected := "SELECT * FROM users WHERE name = '***' AND city = '***'"
	if sqlVal != expected {
		t.Errorf("expected SQL %q, got %q", expected, sqlVal)
	}
}

func TestSanitizeParams_RedactsSQLEscapedQuotes(t *testing.T) {
	params := map[string]any{
		"sql": "SELECT * FROM users WHERE name = 'O''Brien'",
	}

	result := sanitizeParams(params)
	sqlVal := result["sql"].(string)

	expected := "SELECT * FROM users WHERE name = '***'"
	if sqlVal != expected {
		t.Errorf("expected SQL %q, got %q", expected, sqlVal)
	}
}

func TestSanitizeParams_PreservesSQLStructure(t *testing.T) {
	params := map[string]any{
		"sql": "SELECT id, name FROM users WHERE created_at > '2024-01-01' ORDER BY id LIMIT 10",
	}

	result := sanitizeParams(params)
	sqlVal := result["sql"].(string)

	// Structure should be preserved — keywords, columns, table names intact
	if !strings.Contains(sqlVal, "SELECT id, name FROM users WHERE created_at >") {
		t.Errorf("expected SQL structure to be preserved, got %q", sqlVal)
	}
	if !strings.Contains(sqlVal, "'***'") {
		t.Errorf("expected string literal to be redacted, got %q", sqlVal)
	}
	if strings.Contains(sqlVal, "2024-01-01") {
		t.Errorf("expected date literal to be redacted, got %q", sqlVal)
	}
}

func TestSanitizeParams_SQLWithNoLiterals(t *testing.T) {
	params := map[string]any{
		"sql": "SELECT COUNT(*) FROM users WHERE id = 42",
	}

	result := sanitizeParams(params)
	sqlVal := result["sql"].(string)

	// Numeric literals are NOT redacted — only string literals
	expected := "SELECT COUNT(*) FROM users WHERE id = 42"
	if sqlVal != expected {
		t.Errorf("expected SQL %q, got %q", expected, sqlVal)
	}
}

func TestSanitizeParams_QueryKeyAlsoRedacted(t *testing.T) {
	params := map[string]any{
		"query": "SELECT * FROM logs WHERE message = 'error'",
	}

	result := sanitizeParams(params)
	sqlVal := result["query"].(string)

	if !strings.Contains(sqlVal, "'***'") {
		t.Errorf("expected 'query' key to have SQL redaction, got %q", sqlVal)
	}
}

func TestSanitizeParams_NonSQLStringPreserved(t *testing.T) {
	params := map[string]any{
		"table_name": "users",
		"format":     "json",
	}

	result := sanitizeParams(params)

	if result["table_name"] != "users" {
		t.Errorf("expected table_name to be preserved, got %v", result["table_name"])
	}
	if result["format"] != "json" {
		t.Errorf("expected format to be preserved, got %v", result["format"])
	}
}

// --- Sensitive parameter hashing tests ---

func TestSanitizeParams_HashesSensitiveKeys(t *testing.T) {
	params := map[string]any{
		"password":   "my-secret-password",
		"api_key":    "sk-1234567890",
		"table_name": "users",
	}

	result := sanitizeParams(params)

	// Sensitive keys should be hashed
	passwordVal, ok := result["password"].(string)
	if !ok {
		t.Fatal("expected password to be a string")
	}
	if !strings.HasPrefix(passwordVal, "sha256:") {
		t.Errorf("expected password to be hashed, got %q", passwordVal)
	}
	if passwordVal == "my-secret-password" {
		t.Error("expected password to NOT be plaintext")
	}

	apiKeyVal, ok := result["api_key"].(string)
	if !ok {
		t.Fatal("expected api_key to be a string")
	}
	if !strings.HasPrefix(apiKeyVal, "sha256:") {
		t.Errorf("expected api_key to be hashed, got %q", apiKeyVal)
	}

	// Non-sensitive keys should be preserved
	if result["table_name"] != "users" {
		t.Errorf("expected table_name to be preserved, got %v", result["table_name"])
	}
}

func TestSanitizeParams_HashIsDeterministic(t *testing.T) {
	// Same value should produce the same hash for correlation
	hash1 := hashSensitiveValue("my-secret")
	hash2 := hashSensitiveValue("my-secret")

	if hash1 != hash2 {
		t.Errorf("expected deterministic hash, got %q and %q", hash1, hash2)
	}

	// Different values should produce different hashes
	hash3 := hashSensitiveValue("different-secret")
	if hash1 == hash3 {
		t.Error("expected different hashes for different values")
	}
}

func TestSanitizeParams_HashFormat(t *testing.T) {
	result := hashSensitiveValue("test-value")

	if !strings.HasPrefix(result, "sha256:") {
		t.Errorf("expected sha256: prefix, got %q", result)
	}
	// sha256: (7 chars) + 16 hex chars = 23 chars
	if len(result) != 23 {
		t.Errorf("expected hash length 23, got %d", len(result))
	}
}

// --- Nested parameter sanitization tests ---

func TestSanitizeParams_SanitizesNestedMaps(t *testing.T) {
	params := map[string]any{
		"config": map[string]any{
			"password":   "secret123",
			"table_name": "users",
		},
	}

	result := sanitizeParams(params)
	nested, ok := result["config"].(map[string]any)
	if !ok {
		t.Fatal("expected config to be a map")
	}

	passwordVal, ok := nested["password"].(string)
	if !ok {
		t.Fatal("expected nested password to be a string")
	}
	if !strings.HasPrefix(passwordVal, "sha256:") {
		t.Errorf("expected nested password to be hashed, got %q", passwordVal)
	}
	if nested["table_name"] != "users" {
		t.Errorf("expected nested table_name to be preserved, got %v", nested["table_name"])
	}
}

// --- Helper function tests ---

func TestIsSQLParam(t *testing.T) {
	tests := []struct {
		key    string
		expect bool
	}{
		{"sql", true},
		{"SQL", true},
		{"query", true},
		{"QUERY", true},
		{"raw_sql", true},
		{"generated_query", true},
		{"table_name", false},
		{"limit", false},
		{"format", false},
		{"sql_mode", false}, // not a suffix match
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			if got := isSQLParam(tc.key); got != tc.expect {
				t.Errorf("isSQLParam(%q) = %v, want %v", tc.key, got, tc.expect)
			}
		})
	}
}

func TestRedactSQLStringLiterals(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "simple literal",
			input:  "WHERE name = 'John'",
			expect: "WHERE name = '***'",
		},
		{
			name:   "multiple literals",
			input:  "WHERE name = 'John' AND city = 'NYC'",
			expect: "WHERE name = '***' AND city = '***'",
		},
		{
			name:   "empty literal",
			input:  "WHERE name = ''",
			expect: "WHERE name = '***'",
		},
		{
			name:   "no literals",
			input:  "SELECT COUNT(*) FROM users",
			expect: "SELECT COUNT(*) FROM users",
		},
		{
			name:   "numeric not affected",
			input:  "WHERE id = 42 AND name = 'test'",
			expect: "WHERE id = 42 AND name = '***'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := redactSQLStringLiterals(tc.input)
			if got != tc.expect {
				t.Errorf("redactSQLStringLiterals(%q) = %q, want %q", tc.input, got, tc.expect)
			}
		})
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

func TestSummarizeResult_ExtractsRowCount(t *testing.T) {
	jsonText := `{"columns":["id","name"],"rows":[{"id":1,"name":"test"}],"row_count":15000,"truncated":false}`
	result := summarizeResult(&mcplib.CallToolResult{
		Content: []mcplib.Content{
			mcplib.TextContent{Text: jsonText},
		},
	})

	rc, ok := result["row_count"]
	if !ok {
		t.Fatal("expected row_count in result summary")
	}
	if rc != 15000 {
		t.Errorf("expected row_count=15000, got %v", rc)
	}
}

func TestSummarizeResult_NoRowCountInNonJSON(t *testing.T) {
	result := summarizeResult(&mcplib.CallToolResult{
		Content: []mcplib.Content{
			mcplib.TextContent{Text: "plain text response"},
		},
	})

	_, ok := result["row_count"]
	if ok {
		t.Error("expected no row_count for non-JSON response")
	}
}

func TestExtractRowCount_ValidJSON(t *testing.T) {
	summary := map[string]any{}
	extractRowCount(`{"row_count":42}`, summary)
	if summary["row_count"] != 42 {
		t.Errorf("expected row_count=42, got %v", summary["row_count"])
	}
}

func TestExtractRowCount_NoRowCount(t *testing.T) {
	summary := map[string]any{}
	extractRowCount(`{"other":"value"}`, summary)
	if _, ok := summary["row_count"]; ok {
		t.Error("expected no row_count for JSON without it")
	}
}

func TestExtractRowCount_InvalidJSON(t *testing.T) {
	summary := map[string]any{}
	extractRowCount("not json", summary)
	if _, ok := summary["row_count"]; ok {
		t.Error("expected no row_count for invalid JSON")
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
