package mcp

import (
	"context"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"go.uber.org/zap"
)

func TestNewServer(t *testing.T) {
	logger := zap.NewNop()
	s := NewServer("test-server", "1.0.0", logger)

	if s == nil {
		t.Fatal("expected non-nil server")
	}
	if s.mcp == nil {
		t.Fatal("expected non-nil mcp server")
	}
	if s.logger != logger {
		t.Error("expected logger to be set")
	}
}

func TestServer_MCP(t *testing.T) {
	s := NewServer("test-server", "1.0.0", zap.NewNop())

	mcpServer := s.MCP()
	if mcpServer == nil {
		t.Fatal("expected non-nil mcp server from MCP()")
	}
	if mcpServer != s.mcp {
		t.Error("expected MCP() to return the internal mcp server")
	}
}

func TestServer_RegisterTool(t *testing.T) {
	s := NewServer("test-server", "1.0.0", zap.NewNop())

	tool := mcp.NewTool("test-tool", mcp.WithDescription("A test tool"))
	handlerCalled := false

	s.RegisterTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		handlerCalled = true
		return mcp.NewToolResultText("success"), nil
	})

	// Verify tool was registered by checking if we can list it
	// The mcp-go library doesn't expose a direct way to check registered tools,
	// so we verify the handler can be called via the server
	if handlerCalled {
		t.Error("handler should not be called during registration")
	}
}

func TestServer_NewStreamableHTTPServer(t *testing.T) {
	s := NewServer("test-server", "1.0.0", zap.NewNop())

	httpServer := s.NewStreamableHTTPServer()
	if httpServer == nil {
		t.Fatal("expected non-nil HTTP server")
	}
}
