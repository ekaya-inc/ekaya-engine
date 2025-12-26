package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/mcp"
	mcpauth "github.com/ekaya-inc/ekaya-engine/pkg/mcp/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/mcp/tools"
)

// TestMCPIntegration_FullFlow tests the complete MCP request/response cycle
// with project-scoped authentication (Phase 4).
func TestMCPIntegration_FullFlow(t *testing.T) {
	logger := zap.NewNop()

	// Set up MCP server with health tool
	mcpServer := mcp.NewServer("ekaya-engine", "1.0.0-test", logger)
	tools.RegisterHealthTool(mcpServer.MCP(), "1.0.0-test", nil)
	mcpHandler := NewMCPHandler(mcpServer, logger)

	mux := http.NewServeMux()
	mcpHandler.RegisterRoutes(mux, newTestMCPAuthMiddleware())

	const projectID = "test-project"
	mcpEndpoint := "/mcp/" + projectID

	t.Run("initialize", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}},"id":1}`
		req := httptest.NewRequest(http.MethodPost, mcpEndpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var response struct {
			Result struct {
				ProtocolVersion string `json:"protocolVersion"`
				ServerInfo      struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				} `json:"serverInfo"`
			} `json:"result"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Result.ServerInfo.Name != "ekaya-engine" {
			t.Errorf("expected server name 'ekaya-engine', got '%s'", response.Result.ServerInfo.Name)
		}
	})

	t.Run("tools/list", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"tools/list","id":2}`
		req := httptest.NewRequest(http.MethodPost, mcpEndpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var response struct {
			Result struct {
				Tools []struct {
					Name        string `json:"name"`
					Description string `json:"description"`
				} `json:"tools"`
			} `json:"result"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(response.Result.Tools) != 1 {
			t.Fatalf("expected 1 tool, got %d", len(response.Result.Tools))
		}
		if response.Result.Tools[0].Name != "health" {
			t.Errorf("expected tool 'health', got '%s'", response.Result.Tools[0].Name)
		}
	})

	t.Run("tools/call health", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"health"},"id":3}`
		req := httptest.NewRequest(http.MethodPost, mcpEndpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
		}

		var response struct {
			Result struct {
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

		var healthResult struct {
			Engine  string `json:"engine"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(response.Result.Content[0].Text), &healthResult); err != nil {
			t.Fatalf("failed to parse health result: %v", err)
		}

		if healthResult.Engine != "healthy" {
			t.Errorf("expected engine 'healthy', got '%s'", healthResult.Engine)
		}
		if healthResult.Version != "1.0.0-test" {
			t.Errorf("expected version '1.0.0-test', got '%s'", healthResult.Version)
		}
	})

	t.Run("tools/call unknown tool", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"unknown"},"id":4}`
		req := httptest.NewRequest(http.MethodPost, mcpEndpoint, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status %d, got %d (JSON-RPC errors are still 200)", http.StatusOK, rec.Code)
		}

		var response struct {
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Error == nil {
			t.Fatal("expected error for unknown tool")
		}
	})
}

// TestMCPIntegration_AuthFailures tests authentication error cases.
func TestMCPIntegration_AuthFailures(t *testing.T) {
	logger := zap.NewNop()

	mcpServer := mcp.NewServer("ekaya-engine", "1.0.0-test", logger)
	tools.RegisterHealthTool(mcpServer.MCP(), "1.0.0-test", nil)
	mcpHandler := NewMCPHandler(mcpServer, logger)

	t.Run("missing token returns 401 with WWW-Authenticate", func(t *testing.T) {
		// Use auth service that rejects all requests
		failingAuthService := &mcpFailingAuthService{err: auth.ErrMissingAuthorization}
		middleware := mcpauth.NewMiddleware(failingAuthService, logger)

		mux := http.NewServeMux()
		mcpHandler.RegisterRoutes(mux, middleware)

		body := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", rec.Code)
		}

		wwwAuth := rec.Header().Get("WWW-Authenticate")
		if !strings.Contains(wwwAuth, "Bearer") {
			t.Errorf("expected WWW-Authenticate header with Bearer, got %q", wwwAuth)
		}
	})

	t.Run("project mismatch returns 403 with WWW-Authenticate", func(t *testing.T) {
		// Use auth service that returns claims with different project
		mismatchAuthService := &mcpMismatchAuthService{
			claims: &auth.Claims{ProjectID: "other-project"},
			token:  "test-token",
		}
		middleware := mcpauth.NewMiddleware(mismatchAuthService, logger)

		mux := http.NewServeMux()
		mcpHandler.RegisterRoutes(mux, middleware)

		body := `{"jsonrpc":"2.0","method":"tools/list","id":1}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/test-project", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		mux.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected status 403, got %d", rec.Code)
		}

		wwwAuth := rec.Header().Get("WWW-Authenticate")
		if !strings.Contains(wwwAuth, "insufficient_scope") {
			t.Errorf("expected insufficient_scope error, got %q", wwwAuth)
		}
	})
}

// mcpFailingAuthService always returns an error on ValidateRequest.
type mcpFailingAuthService struct {
	err error
}

func (m *mcpFailingAuthService) ValidateRequest(r *http.Request) (*auth.Claims, string, error) {
	return nil, "", m.err
}

func (m *mcpFailingAuthService) RequireProjectID(claims *auth.Claims) error {
	return nil
}

func (m *mcpFailingAuthService) ValidateProjectIDMatch(claims *auth.Claims, urlProjectID string) error {
	return nil
}

// mcpMismatchAuthService returns claims but fails on project ID match.
type mcpMismatchAuthService struct {
	claims *auth.Claims
	token  string
}

func (m *mcpMismatchAuthService) ValidateRequest(r *http.Request) (*auth.Claims, string, error) {
	return m.claims, m.token, nil
}

func (m *mcpMismatchAuthService) RequireProjectID(claims *auth.Claims) error {
	return nil
}

func (m *mcpMismatchAuthService) ValidateProjectIDMatch(claims *auth.Claims, urlProjectID string) error {
	return auth.ErrProjectIDMismatch
}
