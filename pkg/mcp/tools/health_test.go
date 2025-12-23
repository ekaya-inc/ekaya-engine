package tools

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestRegisterHealthTool(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))

	RegisterHealthTool(mcpServer, "test-version")

	// Verify tool is registered by calling tools/list
	ctx := context.Background()
	result := mcpServer.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","method":"tools/list","id":1}`))

	// Marshal the result back to JSON for parsing
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	// Parse the response to verify the health tool is present
	var response struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resultBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	found := false
	for _, tool := range response.Result.Tools {
		if tool.Name == "health" {
			found = true
			if tool.Description != "Returns server health status and version" {
				t.Errorf("unexpected description: %s", tool.Description)
			}
			break
		}
	}
	if !found {
		t.Error("health tool not found in tools/list response")
	}
}

func TestHealthTool_Execute(t *testing.T) {
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	RegisterHealthTool(mcpServer, "1.2.3")

	ctx := context.Background()
	request := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"health"},"id":1}`
	result := mcpServer.HandleMessage(ctx, []byte(request))

	// Marshal the result back to JSON for parsing
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	// Parse the response
	var response struct {
		Result struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resultBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(response.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	content := response.Result.Content[0]
	if content.Type != "text" {
		t.Errorf("expected content type 'text', got '%s'", content.Type)
	}

	// Parse the health result
	var health struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(content.Text), &health); err != nil {
		t.Fatalf("failed to unmarshal health result: %v", err)
	}

	if health.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", health.Status)
	}
	if health.Version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got '%s'", health.Version)
	}
}

func TestHealthTool_VersionWithSpecialChars(t *testing.T) {
	// Test that version with special characters is properly JSON-escaped
	mcpServer := server.NewMCPServer("test", "1.0.0", server.WithToolCapabilities(true))
	versionWithQuotes := `1.0.0-beta"test`
	RegisterHealthTool(mcpServer, versionWithQuotes)

	ctx := context.Background()
	request := `{"jsonrpc":"2.0","method":"tools/call","params":{"name":"health"},"id":1}`
	result := mcpServer.HandleMessage(ctx, []byte(request))

	// Marshal the result back to JSON for parsing
	resultBytes, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}

	// Parse the response
	var response struct {
		Result struct {
			Content []mcp.TextContent `json:"content"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resultBytes, &response); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if len(response.Result.Content) == 0 {
		t.Fatal("expected content in response")
	}

	// Parse the health result - this should work because we use json.Marshal
	var health healthResult
	if err := json.Unmarshal([]byte(response.Result.Content[0].Text), &health); err != nil {
		t.Fatalf("failed to unmarshal health result with special chars: %v", err)
	}

	if health.Version != versionWithQuotes {
		t.Errorf("expected version %q, got %q", versionWithQuotes, health.Version)
	}
}
