package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/mcp"
	"github.com/ekaya-inc/ekaya-engine/pkg/mcp/tools"
)

// TestMCPIntegration_FullFlow tests the complete MCP request/response cycle.
// This serves as a foundation for more complex integration tests when
// authentication (Phase 4) and database-backed tools (Phase 5) are added.
func TestMCPIntegration_FullFlow(t *testing.T) {
	logger := zap.NewNop()

	// Set up MCP server with health tool
	mcpServer := mcp.NewServer("ekaya-engine", "1.0.0-test", logger)
	tools.RegisterHealthTool(mcpServer.MCP(), "1.0.0-test")
	mcpHandler := NewMCPHandler(mcpServer, logger)

	mux := http.NewServeMux()
	mcpHandler.RegisterRoutes(mux)

	t.Run("initialize", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test-client","version":"1.0.0"}},"id":1}`
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
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
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
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
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
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
			Status  string `json:"status"`
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(response.Result.Content[0].Text), &healthResult); err != nil {
			t.Fatalf("failed to parse health result: %v", err)
		}

		if healthResult.Status != "ok" {
			t.Errorf("expected status 'ok', got '%s'", healthResult.Status)
		}
		if healthResult.Version != "1.0.0-test" {
			t.Errorf("expected version '1.0.0-test', got '%s'", healthResult.Version)
		}
	})

	t.Run("tools/call unknown tool", func(t *testing.T) {
		body := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"unknown"},"id":4}`
		req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
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

// TODO: Add authenticated integration tests when Phase 4 is implemented
// func TestMCPIntegration_WithAuth(t *testing.T) { ... }

// TODO: Add database-backed tool tests when Phase 5 is implemented
// func TestMCPIntegration_QueryTool(t *testing.T) { ... }
