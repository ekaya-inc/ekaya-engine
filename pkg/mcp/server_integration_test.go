package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// TestServer_HTTPContextPropagation verifies that JWT claims from HTTP request context
// are properly propagated to MCP tool handlers.
func TestServer_HTTPContextPropagation(t *testing.T) {
	projectID := uuid.New()
	var receivedClaims *auth.Claims

	// Create MCP server and register a test tool that captures claims
	s := NewServer("test-server", "1.0.0", zap.NewNop())

	tool := mcp.NewTool("test-claims", mcp.WithDescription("Test tool that reads claims from context"))
	s.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		claims, ok := auth.GetClaims(ctx)
		if ok {
			receivedClaims = claims
		}
		return mcp.NewToolResultText("ok"), nil
	})

	// Create HTTP server from MCP server
	httpServer := s.NewStreamableHTTPServer()

	// Create a request that simulates what happens after auth middleware runs
	toolCallRequest := map[string]any{
		"jsonrpc": "2.0",
		"method":  "tools/call",
		"params": map[string]any{
			"name": "test-claims",
		},
		"id": 1,
	}
	body, _ := json.Marshal(toolCallRequest)

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	// Inject claims into request context (simulating what auth middleware does)
	claims := &auth.Claims{ProjectID: projectID.String()}
	ctx := context.WithValue(req.Context(), auth.ClaimsKey, claims)
	req = req.WithContext(ctx)

	// Execute request
	rec := httptest.NewRecorder()
	httpServer.ServeHTTP(rec, req)

	// Verify the tool handler received the claims from context
	if receivedClaims == nil {
		t.Fatal("expected tool handler to receive claims from HTTP context, but got nil")
	}
	if receivedClaims.ProjectID != projectID.String() {
		t.Errorf("expected project ID %q, got %q", projectID.String(), receivedClaims.ProjectID)
	}
}
