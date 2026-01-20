package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestMCPRequestLogger(t *testing.T) {
	t.Run("logs successful tool call", func(t *testing.T) {
		// Setup observer to capture logs
		core, logs := observer.New(zapcore.DebugLevel)
		logger := zap.New(core)

		// Create test handler that returns a successful JSON-RPC response
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"success"}]}}`))
		})

		// Wrap with MCP logging middleware
		wrapped := MCPRequestLogger(logger)(handler)

		// Create test request with JSON-RPC tool call
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_ontology","arguments":{"depth":"domain"}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", bytes.NewBufferString(reqBody))
		rec := httptest.NewRecorder()

		// Execute request
		wrapped.ServeHTTP(rec, req)

		// Verify logs
		assert.Equal(t, 2, logs.Len(), "Should log request and response")

		// Check request log
		requestLog := logs.All()[0]
		assert.Equal(t, "MCP request", requestLog.Message)
		assert.Equal(t, "tools/call", requestLog.ContextMap()["method"])
		assert.Equal(t, "get_ontology", requestLog.ContextMap()["tool"])
		assert.NotNil(t, requestLog.ContextMap()["arguments"])

		// Check response log
		responseLog := logs.All()[1]
		assert.Equal(t, "MCP response success", responseLog.Message)
		assert.Equal(t, "get_ontology", responseLog.ContextMap()["tool"])
		assert.NotNil(t, responseLog.ContextMap()["duration"])
	})

	t.Run("logs tool call with error response", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := zap.New(core)

		// Create test handler that returns a JSON-RPC error
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK) // JSON-RPC errors return HTTP 200
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32603,"message":"no active ontology found"}}`))
		})

		wrapped := MCPRequestLogger(logger)(handler)

		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_ontology","arguments":{"depth":"columns"}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", bytes.NewBufferString(reqBody))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.Equal(t, 2, logs.Len())

		// Check error response log
		responseLog := logs.All()[1]
		assert.Equal(t, "MCP response error", responseLog.Message)
		assert.Equal(t, "get_ontology", responseLog.ContextMap()["tool"])
		assert.Equal(t, int64(-32603), responseLog.ContextMap()["error_code"])
		assert.Equal(t, "no active ontology found", responseLog.ContextMap()["error_message"])
		assert.NotNil(t, responseLog.ContextMap()["duration"])
	})

	t.Run("sanitizes sensitive parameters", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := zap.New(core)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		})

		wrapped := MCPRequestLogger(logger)(handler)

		// Request with sensitive parameters
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","arguments":{"password":"secret123","api_key":"abc123","normal_param":"visible"}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", bytes.NewBufferString(reqBody))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Check that sensitive fields are redacted
		requestLog := logs.All()[0]
		args := requestLog.ContextMap()["arguments"].(map[string]interface{})
		assert.Equal(t, "[REDACTED]", args["password"])
		assert.Equal(t, "[REDACTED]", args["api_key"])
		assert.Equal(t, "visible", args["normal_param"])
	})

	t.Run("truncates long string values", func(t *testing.T) {
		core, logs := observer.New(zapcore.DebugLevel)
		logger := zap.New(core)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
		})

		wrapped := MCPRequestLogger(logger)(handler)

		// Create a string longer than 200 characters
		longString := string(make([]byte, 250))
		for i := range longString {
			longString = longString[:i] + "a" + longString[i+1:]
		}

		// Manually construct with long string
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","arguments":{"long_param":"` + longString + `"}}}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", bytes.NewBufferString(reqBody))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Check that long string is truncated
		requestLog := logs.All()[0]
		args := requestLog.ContextMap()["arguments"].(map[string]interface{})
		truncated := args["long_param"].(string)
		assert.True(t, len(truncated) <= 203, "Should truncate to 200 chars + '...'")
		assert.True(t, len(truncated) > 200, "Should have ellipsis")
		assert.Contains(t, truncated, "...")
	})

	t.Run("passes through with nil logger", func(t *testing.T) {
		called := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		wrapped := MCPRequestLogger(nil)(handler)

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", bytes.NewBufferString(`{}`))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		assert.True(t, called, "Should pass through to handler")
		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("handles malformed JSON request gracefully", func(t *testing.T) {
		core, _ := observer.New(zapcore.DebugLevel)
		logger := zap.New(core)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"error":"bad request"}`))
		})

		wrapped := MCPRequestLogger(logger)(handler)

		// Send malformed JSON
		req := httptest.NewRequest(http.MethodPost, "/mcp/test", bytes.NewBufferString(`{invalid json`))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Should handle gracefully without crashing
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("handles empty request body", func(t *testing.T) {
		core, _ := observer.New(zapcore.DebugLevel)
		logger := zap.New(core)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		wrapped := MCPRequestLogger(logger)(handler)

		req := httptest.NewRequest(http.MethodPost, "/mcp/test", bytes.NewBufferString(""))
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Should handle gracefully
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestSanitizeArguments(t *testing.T) {
	t.Run("redacts sensitive keywords", func(t *testing.T) {
		args := map[string]interface{}{
			"password":       "secret",
			"api_key":        "abc123",
			"access_token":   "xyz789",
			"client_secret":  "hidden",
			"credential":     "cred123",
			"normal_field":   "visible",
		}

		result := sanitizeArguments(args)

		assert.Equal(t, "[REDACTED]", result["password"])
		assert.Equal(t, "[REDACTED]", result["api_key"])
		assert.Equal(t, "[REDACTED]", result["access_token"])
		assert.Equal(t, "[REDACTED]", result["client_secret"])
		assert.Equal(t, "[REDACTED]", result["credential"])
		assert.Equal(t, "visible", result["normal_field"])
	})

	t.Run("truncates long strings", func(t *testing.T) {
		longString := string(make([]byte, 250))
		for i := range longString {
			longString = longString[:i] + "x" + longString[i+1:]
		}

		args := map[string]interface{}{
			"long_value": longString,
			"short":      "abc",
		}

		result := sanitizeArguments(args)

		truncated := result["long_value"].(string)
		assert.True(t, len(truncated) <= 203) // 200 + "..."
		assert.Contains(t, truncated, "...")
		assert.Equal(t, "abc", result["short"])
	})

	t.Run("handles nil arguments", func(t *testing.T) {
		result := sanitizeArguments(nil)
		assert.Nil(t, result)
	})

	t.Run("handles empty arguments", func(t *testing.T) {
		result := sanitizeArguments(map[string]interface{}{})
		assert.NotNil(t, result)
		assert.Equal(t, 0, len(result))
	})

	t.Run("preserves non-string values", func(t *testing.T) {
		args := map[string]interface{}{
			"number": 42,
			"bool":   true,
			"null":   nil,
			"array":  []string{"a", "b"},
			"object": map[string]string{"key": "value"},
		}

		result := sanitizeArguments(args)

		assert.Equal(t, 42, result["number"])
		assert.Equal(t, true, result["bool"])
		assert.Nil(t, result["null"])
		assert.Equal(t, args["array"], result["array"])
		assert.Equal(t, args["object"], result["object"])
	})

	t.Run("case insensitive keyword matching", func(t *testing.T) {
		args := map[string]interface{}{
			"PASSWORD":     "secret",
			"Api_Key":      "abc123",
			"AccessToken":  "xyz789",
		}

		result := sanitizeArguments(args)

		assert.Equal(t, "[REDACTED]", result["PASSWORD"])
		assert.Equal(t, "[REDACTED]", result["Api_Key"])
		assert.Equal(t, "[REDACTED]", result["AccessToken"])
	})
}
