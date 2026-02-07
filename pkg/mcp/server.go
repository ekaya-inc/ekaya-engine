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

// ToolFilterFunc is a function that filters tools based on context.
type ToolFilterFunc = server.ToolFilterFunc

// ServerOption is a functional option for configuring the MCP server.
type ServerOption func(*serverOptions)

type serverOptions struct {
	toolFilter ToolFilterFunc
	hooks      *server.Hooks
}

// WithToolFilter sets a function to filter tools based on context.
func WithToolFilter(filter ToolFilterFunc) ServerOption {
	return func(opts *serverOptions) {
		opts.toolFilter = filter
	}
}

// WithHooks sets hooks for MCP server events (tool calls, errors, etc.).
func WithHooks(hooks *server.Hooks) ServerOption {
	return func(opts *serverOptions) {
		opts.hooks = hooks
	}
}

// NewServer creates a new MCP server instance.
func NewServer(name, version string, logger *zap.Logger, opts ...ServerOption) *Server {
	// Process options
	options := &serverOptions{}
	for _, opt := range opts {
		opt(options)
	}

	// Build server options
	serverOpts := []server.ServerOption{
		server.WithToolCapabilities(true),
	}
	if options.toolFilter != nil {
		serverOpts = append(serverOpts, server.WithToolFilter(options.toolFilter))
	}
	if options.hooks != nil {
		serverOpts = append(serverOpts, server.WithHooks(options.hooks))
	}

	mcpServer := server.NewMCPServer(
		name,
		version,
		serverOpts...,
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
