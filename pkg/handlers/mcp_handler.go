package handlers

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/mcp"
)

// MCPHandler handles MCP protocol requests over HTTP.
type MCPHandler struct {
	httpServer *server.StreamableHTTPServer
	logger     *zap.Logger
}

// NewMCPHandler creates a new MCP handler from an MCP server.
func NewMCPHandler(mcpServer *mcp.Server, logger *zap.Logger) *MCPHandler {
	return &MCPHandler{
		httpServer: mcpServer.NewStreamableHTTPServer(),
		logger:     logger,
	}
}

// RegisterRoutes registers the MCP endpoint.
func (h *MCPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/mcp", h.httpServer)
}
