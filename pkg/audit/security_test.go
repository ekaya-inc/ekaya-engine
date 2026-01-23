package audit

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// setupTestLogger creates a test logger with an observer to capture log entries.
func setupTestLogger(t *testing.T) (*zap.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, recorded := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)
	return logger, recorded
}

func TestNewSecurityAuditor(t *testing.T) {
	logger, _ := setupTestLogger(t)
	auditor := NewSecurityAuditor(logger)

	assert.NotNil(t, auditor)
	assert.NotNil(t, auditor.logger)
}

func TestLogInjectionAttempt(t *testing.T) {
	logger, recorded := setupTestLogger(t)
	auditor := NewSecurityAuditor(logger)

	projectID := uuid.New()
	queryID := uuid.New()
	clientIP := "192.168.1.100"

	details := SQLInjectionDetails{
		ParamName:   "search",
		ParamValue:  "'; DROP TABLE users--",
		Fingerprint: "s&1c",
		QueryName:   "Search customers",
	}

	tests := []struct {
		name     string
		ctx      context.Context
		wantUser string
	}{
		{
			name: "with user context",
			ctx: func() context.Context {
				claims := &auth.Claims{
					ProjectID: projectID.String(),
				}
				claims.Subject = "user-123"
				return context.WithValue(context.Background(), auth.ClaimsKey, claims)
			}(),
			wantUser: "user-123",
		},
		{
			name:     "without user context",
			ctx:      context.Background(),
			wantUser: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorded.TakeAll() // Clear previous logs

			auditor.LogInjectionAttempt(tt.ctx, projectID, queryID, details, clientIP)

			// Verify log entry was created
			logs := recorded.All()
			require.Len(t, logs, 1, "Expected exactly one log entry")

			entry := logs[0]
			assert.Equal(t, zapcore.ErrorLevel, entry.Level, "Should log at ERROR level")
			assert.Equal(t, "SQL injection attempt detected", entry.Message)

			// Verify structured fields
			fields := entry.ContextMap()
			assert.Equal(t, projectID.String(), fields["project_id"])
			assert.Equal(t, queryID.String(), fields["query_id"])
			assert.Equal(t, "search", fields["param_name"])
			assert.Equal(t, "s&1c", fields["fingerprint"])
			assert.Equal(t, clientIP, fields["client_ip"])
			assert.Equal(t, tt.wantUser, fields["user_id"])
			assert.Equal(t, "critical", fields["severity"])

			// Verify JSON event structure
			eventJSON, ok := fields["event_json"].(string)
			require.True(t, ok, "event_json should be a string")

			var event SecurityEvent
			err := json.Unmarshal([]byte(eventJSON), &event)
			require.NoError(t, err, "event_json should be valid JSON")

			assert.Equal(t, EventSQLInjectionAttempt, event.EventType)
			assert.Equal(t, projectID, event.ProjectID)
			assert.Equal(t, queryID, event.QueryID)
			assert.Equal(t, tt.wantUser, event.UserID)
			assert.Equal(t, clientIP, event.ClientIP)
			assert.Equal(t, "critical", event.Severity)

			// Verify details
			detailsMap, ok := event.Details.(map[string]any)
			require.True(t, ok, "Details should be a map")
			assert.Equal(t, "search", detailsMap["param_name"])
			assert.Equal(t, "'; DROP TABLE users--", detailsMap["param_value"])
			assert.Equal(t, "s&1c", detailsMap["fingerprint"])
			assert.Equal(t, "Search customers", detailsMap["query_name"])
		})
	}
}

func TestLogParameterValidation(t *testing.T) {
	logger, recorded := setupTestLogger(t)
	auditor := NewSecurityAuditor(logger)

	projectID := uuid.New()
	queryID := uuid.New()
	clientIP := "10.0.0.50"
	errorMsg := "customer_id is required but not provided"

	claims := &auth.Claims{
		ProjectID: projectID.String(),
	}
	claims.Subject = "user-456"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	auditor.LogParameterValidation(ctx, projectID, queryID, errorMsg, clientIP)

	// Verify log entry
	logs := recorded.All()
	require.Len(t, logs, 1)

	entry := logs[0]
	assert.Equal(t, zapcore.WarnLevel, entry.Level, "Should log at WARN level")
	assert.Equal(t, "Parameter validation failed", entry.Message)

	// Verify structured fields
	fields := entry.ContextMap()
	assert.Equal(t, projectID.String(), fields["project_id"])
	assert.Equal(t, queryID.String(), fields["query_id"])
	assert.Equal(t, errorMsg, fields["error"])
	assert.Equal(t, clientIP, fields["client_ip"])
	assert.Equal(t, "user-456", fields["user_id"])
	assert.Equal(t, "warning", fields["severity"])

	// Verify JSON event structure
	eventJSON, ok := fields["event_json"].(string)
	require.True(t, ok)

	var event SecurityEvent
	err := json.Unmarshal([]byte(eventJSON), &event)
	require.NoError(t, err)

	assert.Equal(t, EventParameterValidation, event.EventType)
	assert.Equal(t, "warning", event.Severity)

	detailsMap, ok := event.Details.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, errorMsg, detailsMap["error"])
}

func TestLogQueryExecution(t *testing.T) {
	logger, recorded := setupTestLogger(t)
	auditor := NewSecurityAuditor(logger)

	projectID := uuid.New()
	queryID := uuid.New()
	clientIP := "172.16.0.1"
	queryName := "Get customer orders"

	claims := &auth.Claims{
		ProjectID: projectID.String(),
	}
	claims.Subject = "user-789"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	auditor.LogQueryExecution(ctx, projectID, queryID, queryName, clientIP)

	// Verify log entry
	logs := recorded.All()
	require.Len(t, logs, 1)

	entry := logs[0]
	assert.Equal(t, zapcore.InfoLevel, entry.Level, "Should log at INFO level")
	assert.Equal(t, "Query executed", entry.Message)

	// Verify structured fields
	fields := entry.ContextMap()
	assert.Equal(t, projectID.String(), fields["project_id"])
	assert.Equal(t, queryID.String(), fields["query_id"])
	assert.Equal(t, queryName, fields["query_name"])
	assert.Equal(t, clientIP, fields["client_ip"])
	assert.Equal(t, "user-789", fields["user_id"])
	assert.Equal(t, "info", fields["severity"])

	// Verify JSON event structure
	eventJSON, ok := fields["event_json"].(string)
	require.True(t, ok)

	var event SecurityEvent
	err := json.Unmarshal([]byte(eventJSON), &event)
	require.NoError(t, err)

	assert.Equal(t, EventQueryExecution, event.EventType)
	assert.Equal(t, "info", event.Severity)

	detailsMap, ok := event.Details.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, queryName, detailsMap["query_name"])
}

func TestMultipleInjectionAttempts(t *testing.T) {
	logger, recorded := setupTestLogger(t)
	auditor := NewSecurityAuditor(logger)

	projectID := uuid.New()
	ctx := context.Background()

	// Log multiple injection attempts
	attempts := []struct {
		queryID  uuid.UUID
		param    string
		value    string
		fp       string
		clientIP string
	}{
		{uuid.New(), "search", "' OR '1'='1", "o1o", "192.168.1.1"},
		{uuid.New(), "filter", "1; DELETE FROM users", "s&1c", "192.168.1.2"},
		{uuid.New(), "id", "1 UNION SELECT * FROM passwords", "s&1UE", "192.168.1.3"},
	}

	for _, att := range attempts {
		details := SQLInjectionDetails{
			ParamName:   att.param,
			ParamValue:  att.value,
			Fingerprint: att.fp,
			QueryName:   "Test Query",
		}
		auditor.LogInjectionAttempt(ctx, projectID, att.queryID, details, att.clientIP)
	}

	// Verify all were logged
	logs := recorded.All()
	require.Len(t, logs, 3, "Should have logged all three attempts")

	for i, entry := range logs {
		assert.Equal(t, zapcore.ErrorLevel, entry.Level)
		fields := entry.ContextMap()
		assert.Equal(t, attempts[i].clientIP, fields["client_ip"])
		assert.Equal(t, attempts[i].param, fields["param_name"])
	}
}

func TestSecurityEventSerialization(t *testing.T) {
	// Test that all event types serialize correctly
	tests := []struct {
		name      string
		eventType SecurityEventType
		severity  string
		details   any
	}{
		{
			name:      "injection attempt",
			eventType: EventSQLInjectionAttempt,
			severity:  "critical",
			details: SQLInjectionDetails{
				ParamName:   "test",
				ParamValue:  "test value",
				Fingerprint: "abc",
				QueryName:   "Test",
			},
		},
		{
			name:      "validation failure",
			eventType: EventParameterValidation,
			severity:  "warning",
			details: map[string]string{
				"error": "validation failed",
			},
		},
		{
			name:      "query execution",
			eventType: EventQueryExecution,
			severity:  "info",
			details: map[string]string{
				"query_name": "Test Query",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := SecurityEvent{
				EventType: tt.eventType,
				ProjectID: uuid.New(),
				QueryID:   uuid.New(),
				UserID:    "test-user",
				ClientIP:  "127.0.0.1",
				Details:   tt.details,
				Severity:  tt.severity,
			}

			// Verify it serializes to valid JSON
			jsonBytes, err := json.Marshal(event)
			require.NoError(t, err)

			// Verify it deserializes correctly
			var decoded SecurityEvent
			err = json.Unmarshal(jsonBytes, &decoded)
			require.NoError(t, err)

			assert.Equal(t, event.EventType, decoded.EventType)
			assert.Equal(t, event.ProjectID, decoded.ProjectID)
			assert.Equal(t, event.QueryID, decoded.QueryID)
			assert.Equal(t, event.UserID, decoded.UserID)
			assert.Equal(t, event.ClientIP, decoded.ClientIP)
			assert.Equal(t, event.Severity, decoded.Severity)
		})
	}
}

func TestLoggerNamespace(t *testing.T) {
	// Verify that the security auditor creates a proper logger namespace
	logger, recorded := setupTestLogger(t)
	auditor := NewSecurityAuditor(logger)

	projectID := uuid.New()
	queryID := uuid.New()
	details := SQLInjectionDetails{
		ParamName:   "test",
		ParamValue:  "test",
		Fingerprint: "abc",
		QueryName:   "Test",
	}

	auditor.LogInjectionAttempt(context.Background(), projectID, queryID, details, "127.0.0.1")

	logs := recorded.All()
	require.Len(t, logs, 1)

	// Verify logger name includes security_audit namespace
	assert.Equal(t, "security_audit", logs[0].LoggerName)
}

func TestLogModifyingQueryExecution_Success(t *testing.T) {
	logger, recorded := setupTestLogger(t)
	auditor := NewSecurityAuditor(logger)

	projectID := uuid.New()
	queryID := uuid.New()
	clientIP := "10.0.0.1"

	claims := &auth.Claims{
		ProjectID: projectID.String(),
	}
	claims.Subject = "user-modify-123"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	details := ModifyingQueryDetails{
		QueryName:       "Delete inactive users",
		SQLType:         "DELETE",
		SQL:             "DELETE FROM users WHERE last_login < '2024-01-01' RETURNING id",
		Parameters:      map[string]any{"cutoff": "2024-01-01"},
		RowsAffected:    42,
		RowCount:        42,
		Success:         true,
		ExecutionTimeMs: 150,
	}

	auditor.LogModifyingQueryExecution(ctx, projectID, queryID, details, clientIP)

	// Verify log entry
	logs := recorded.All()
	require.Len(t, logs, 1)

	entry := logs[0]
	assert.Equal(t, zapcore.InfoLevel, entry.Level, "Should log at INFO level for successful execution")
	assert.Equal(t, "Modifying query executed", entry.Message)

	// Verify structured fields
	fields := entry.ContextMap()
	assert.Equal(t, projectID.String(), fields["project_id"])
	assert.Equal(t, queryID.String(), fields["query_id"])
	assert.Equal(t, "Delete inactive users", fields["query_name"])
	assert.Equal(t, "DELETE", fields["sql_type"])
	assert.Equal(t, int64(42), fields["rows_affected"])
	assert.Equal(t, int64(150), fields["execution_time_ms"])
	assert.Equal(t, clientIP, fields["client_ip"])
	assert.Equal(t, "user-modify-123", fields["user_id"])
	assert.Equal(t, "info", fields["severity"])

	// Verify JSON event structure
	eventJSON, ok := fields["event_json"].(string)
	require.True(t, ok, "event_json should be a string")

	var event SecurityEvent
	err := json.Unmarshal([]byte(eventJSON), &event)
	require.NoError(t, err, "event_json should be valid JSON")

	assert.Equal(t, EventModifyingQueryExecution, event.EventType)
	assert.Equal(t, projectID, event.ProjectID)
	assert.Equal(t, queryID, event.QueryID)
	assert.Equal(t, "user-modify-123", event.UserID)
	assert.Equal(t, clientIP, event.ClientIP)
	assert.Equal(t, "info", event.Severity)

	// Verify details
	detailsMap, ok := event.Details.(map[string]any)
	require.True(t, ok, "Details should be a map")
	assert.Equal(t, "Delete inactive users", detailsMap["query_name"])
	assert.Equal(t, "DELETE", detailsMap["sql_type"])
	assert.Equal(t, float64(42), detailsMap["rows_affected"]) // JSON numbers are float64
	assert.Equal(t, true, detailsMap["success"])
}

func TestLogModifyingQueryExecution_Failure(t *testing.T) {
	logger, recorded := setupTestLogger(t)
	auditor := NewSecurityAuditor(logger)

	projectID := uuid.New()
	queryID := uuid.New()
	clientIP := "10.0.0.2"

	claims := &auth.Claims{
		ProjectID: projectID.String(),
	}
	claims.Subject = "user-fail-456"
	ctx := context.WithValue(context.Background(), auth.ClaimsKey, claims)

	details := ModifyingQueryDetails{
		QueryName:       "Update user status",
		SQLType:         "UPDATE",
		SQL:             "UPDATE users SET status = 'active' WHERE id = $1",
		Parameters:      map[string]any{"id": "invalid-uuid"},
		RowsAffected:    0,
		RowCount:        0,
		Success:         false,
		ErrorMessage:    "invalid input syntax for type uuid: \"invalid-uuid\"",
		ExecutionTimeMs: 50,
	}

	auditor.LogModifyingQueryExecution(ctx, projectID, queryID, details, clientIP)

	// Verify log entry
	logs := recorded.All()
	require.Len(t, logs, 1)

	entry := logs[0]
	assert.Equal(t, zapcore.WarnLevel, entry.Level, "Should log at WARN level for failed execution")
	assert.Equal(t, "Modifying query failed", entry.Message)

	// Verify structured fields
	fields := entry.ContextMap()
	assert.Equal(t, projectID.String(), fields["project_id"])
	assert.Equal(t, queryID.String(), fields["query_id"])
	assert.Equal(t, "Update user status", fields["query_name"])
	assert.Equal(t, "UPDATE", fields["sql_type"])
	assert.Contains(t, fields["error"], "invalid input syntax")
	assert.Equal(t, int64(50), fields["execution_time_ms"])
	assert.Equal(t, clientIP, fields["client_ip"])
	assert.Equal(t, "user-fail-456", fields["user_id"])
	assert.Equal(t, "warning", fields["severity"])

	// Verify JSON event structure
	eventJSON, ok := fields["event_json"].(string)
	require.True(t, ok, "event_json should be a string")

	var event SecurityEvent
	err := json.Unmarshal([]byte(eventJSON), &event)
	require.NoError(t, err, "event_json should be valid JSON")

	assert.Equal(t, EventModifyingQueryExecution, event.EventType)
	assert.Equal(t, "warning", event.Severity)

	// Verify error message in details
	detailsMap, ok := event.Details.(map[string]any)
	require.True(t, ok, "Details should be a map")
	assert.Equal(t, false, detailsMap["success"])
	assert.Contains(t, detailsMap["error_message"], "invalid input syntax")
}

func TestModifyingQueryEventSerialization(t *testing.T) {
	// Test that ModifyingQueryDetails serializes correctly
	details := ModifyingQueryDetails{
		QueryName:       "Insert new order",
		SQLType:         "INSERT",
		SQL:             "INSERT INTO orders (customer_id, total) VALUES ($1, $2) RETURNING id",
		Parameters:      map[string]any{"customer_id": "cust-123", "total": 99.99},
		RowsAffected:    1,
		RowCount:        1,
		Success:         true,
		ExecutionTimeMs: 25,
	}

	event := SecurityEvent{
		EventType: EventModifyingQueryExecution,
		ProjectID: uuid.New(),
		QueryID:   uuid.New(),
		UserID:    "test-user",
		ClientIP:  "127.0.0.1",
		Details:   details,
		Severity:  "info",
	}

	// Verify it serializes to valid JSON
	jsonBytes, err := json.Marshal(event)
	require.NoError(t, err)

	// Verify it deserializes correctly
	var decoded SecurityEvent
	err = json.Unmarshal(jsonBytes, &decoded)
	require.NoError(t, err)

	assert.Equal(t, event.EventType, decoded.EventType)
	assert.Equal(t, event.ProjectID, decoded.ProjectID)
	assert.Equal(t, event.QueryID, decoded.QueryID)
	assert.Equal(t, event.UserID, decoded.UserID)
	assert.Equal(t, event.ClientIP, decoded.ClientIP)
	assert.Equal(t, event.Severity, decoded.Severity)

	// Verify details
	detailsMap, ok := decoded.Details.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Insert new order", detailsMap["query_name"])
	assert.Equal(t, "INSERT", detailsMap["sql_type"])
	assert.Equal(t, true, detailsMap["success"])
	assert.Equal(t, float64(1), detailsMap["rows_affected"])
}
