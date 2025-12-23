package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"
)

// Server wraps the mcp-go MCPServer with ekaya-engine patterns.
type Server struct {
	mcp    *server.MCPServer
	logger *zap.Logger
}

// NewServer creates a new MCP server instance.
func NewServer(name, version string, logger *zap.Logger) *Server {
	mcpServer := server.NewMCPServer(
		name,
		version,
		server.WithToolCapabilities(true),
	)

	return &Server{
		mcp:    mcpServer,
		logger: logger,
	}
}

// MCP returns the underlying MCPServer for tool registration.
func (s *Server) MCP() *server.MCPServer {
	return s.mcp
}

// NewStreamableHTTPServer creates an HTTP transport server wrapping this MCP server.
// The HTTP mux handles routing to /mcp, so no endpoint path is configured here.
func (s *Server) NewStreamableHTTPServer() *server.StreamableHTTPServer {
	return server.NewStreamableHTTPServer(
		s.mcp,
		server.WithStateLess(true),
	)
}

// RegisterTool is a convenience wrapper for registering a tool.
func (s *Server) RegisterTool(tool mcp.Tool, handler server.ToolHandlerFunc) {
	s.mcp.AddTool(tool, handler)
}
