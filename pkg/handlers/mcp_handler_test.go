package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/mcp"
	mcpauth "github.com/ekaya-inc/ekaya-engine/pkg/mcp/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/mcp/tools"
)

// defaultMCPConfig returns the default MCP logging configuration for tests.
func defaultMCPConfig() config.MCPConfig {
	return config.MCPConfig{
		LogRequests:  true,
		LogResponses: false,
		LogErrors:    true,
	}
}

// mcpPassingAuthService is a mock that always allows requests through.
type mcpPassingAuthService struct {
	claims *auth.Claims
	token  string
}

func (m *mcpPassingAuthService) ValidateRequest(r *http.Request) (*auth.Claims, string, error) {
	return m.claims, m.token, nil
}

func (m *mcpPassingAuthService) RequireProjectID(claims *auth.Claims) error {
	return nil
}

func (m *mcpPassingAuthService) ValidateProjectIDMatch(claims *auth.Claims, urlProjectID string) error {
	return nil
}

func newTestMCPAuthMiddleware() *mcpauth.Middleware {
	authService := &mcpPassingAuthService{
		claims: &auth.Claims{ProjectID: "test-project"},
		token:  "test-token",
	}
	return mcpauth.NewMiddleware(authService, nil, nil, zap.NewNop())
}

func TestNewMCPHandler(t *testing.T) {
	logger := zap.NewNop()
	mcpServer := mcp.NewServer("test", "1.0.0", logger)

	handler := NewMCPHandler(mcpServer, logger, defaultMCPConfig())

	if handler == nil {
		t.Fatal("expected non-nil handler")
	}
	if handler.httpServer == nil {
		t.Fatal("expected non-nil http server")
	}
	if handler.logger != logger {
		t.Error("expected logger to be set")
	}
}

func TestMCPHandler_RegisterRoutes(t *testing.T) {
	logger := zap.NewNop()
	mcpServer := mcp.NewServer("test", "1.0.0", logger)
	tools.RegisterHealthTool(mcpServer.MCP(), "1.0.0", nil)
	handler := NewMCPHandler(mcpServer, logger, defaultMCPConfig())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, newTestMCPAuthMiddleware())

	// Test POST /mcp/{pid} is registered and responds
	body := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/mcp/{pid}: expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify it's a valid JSON-RPC response
	var response map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["jsonrpc"] != "2.0" {
		t.Errorf("expected jsonrpc 2.0, got %v", response["jsonrpc"])
	}
	if response["id"] != float64(1) {
		t.Errorf("expected id 1, got %v", response["id"])
	}
}

func TestMCPHandler_ToolsCall(t *testing.T) {
	logger := zap.NewNop()
	mcpServer := mcp.NewServer("test", "test-version", logger)
	tools.RegisterHealthTool(mcpServer.MCP(), "test-version", nil)
	handler := NewMCPHandler(mcpServer, logger, defaultMCPConfig())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, newTestMCPAuthMiddleware())

	body := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"health"},"id":1}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Result  struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	// Parse the health result
	var healthResult struct {
		Engine  string `json:"engine"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(response.Result.Content[0].Text), &healthResult); err != nil {
		t.Fatalf("failed to unmarshal health result: %v", err)
	}

	if healthResult.Engine != "healthy" {
		t.Errorf("expected engine 'healthy', got '%s'", healthResult.Engine)
	}
	if healthResult.Version != "test-version" {
		t.Errorf("expected version 'test-version', got '%s'", healthResult.Version)
	}
}

func TestMCPHandler_GETReturnsMethodNotAllowed(t *testing.T) {
	logger := zap.NewNop()
	mcpServer := mcp.NewServer("test", "1.0.0", logger)
	tools.RegisterHealthTool(mcpServer.MCP(), "1.0.0", nil)
	handler := NewMCPHandler(mcpServer, logger, defaultMCPConfig())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, newTestMCPAuthMiddleware())

	// GET requests should return 405 Method Not Allowed
	req := httptest.NewRequest(http.MethodGet, "/mcp/test-project", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /mcp/{pid}: expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}

	// Should have Allow header indicating POST is allowed
	if allow := rec.Header().Get("Allow"); allow != "POST" {
		t.Errorf("expected Allow header 'POST', got '%s'", allow)
	}
}

func TestMCPHandler_UnsupportedMethodsReturnMethodNotAllowed(t *testing.T) {
	logger := zap.NewNop()
	mcpServer := mcp.NewServer("test", "1.0.0", logger)
	handler := NewMCPHandler(mcpServer, logger, defaultMCPConfig())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, newTestMCPAuthMiddleware())

	unsupportedMethods := []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch}

	for _, method := range unsupportedMethods {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/mcp/test-project", nil)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s /mcp/{pid}: expected status %d, got %d", method, http.StatusMethodNotAllowed, rec.Code)
			}
		})
	}
}

func TestMCPHandler_LoggingMiddlewareIntegration(t *testing.T) {
	// Create an observer logger to capture log output
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	mcpServer := mcp.NewServer("test", "1.0.0", logger)
	tools.RegisterHealthTool(mcpServer.MCP(), "1.0.0", nil)
	handler := NewMCPHandler(mcpServer, logger, defaultMCPConfig())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux, newTestMCPAuthMiddleware())

	// Make a successful tools/call request
	body := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"health"},"id":1}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Verify MCP request logging
	requestLogs := logs.FilterMessage("MCP request").All()
	if len(requestLogs) != 1 {
		t.Errorf("expected 1 MCP request log entry, got %d", len(requestLogs))
	} else {
		log := requestLogs[0]
		if log.ContextMap()["method"] != "tools/call" {
			t.Errorf("expected method 'tools/call', got '%v'", log.ContextMap()["method"])
		}
		if log.ContextMap()["tool"] != "health" {
			t.Errorf("expected tool 'health', got '%v'", log.ContextMap()["tool"])
		}
	}

	// Verify MCP response logging
	responseLogs := logs.FilterMessage("MCP response success").All()
	if len(responseLogs) != 1 {
		t.Errorf("expected 1 MCP response success log entry, got %d", len(responseLogs))
	} else {
		log := responseLogs[0]
		if log.ContextMap()["tool"] != "health" {
			t.Errorf("expected tool 'health', got '%v'", log.ContextMap()["tool"])
		}
		if _, ok := log.ContextMap()["duration"]; !ok {
			t.Error("expected duration to be logged")
		}
	}
}
