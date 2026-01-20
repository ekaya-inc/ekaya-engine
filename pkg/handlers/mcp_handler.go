package handlers

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/mcp"
	mcpauth "github.com/ekaya-inc/ekaya-engine/pkg/mcp/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/middleware"
)

// MCPHandler handles MCP protocol requests over HTTP.
type MCPHandler struct {
	httpServer *server.StreamableHTTPServer
	logger     *zap.Logger
	mcpConfig  config.MCPConfig
}

// NewMCPHandler creates a new MCP handler from an MCP server.
func NewMCPHandler(mcpServer *mcp.Server, logger *zap.Logger, mcpConfig config.MCPConfig) *MCPHandler {
	return &MCPHandler{
		httpServer: mcpServer.NewStreamableHTTPServer(),
		logger:     logger,
		mcpConfig:  mcpConfig,
	}
}

// RegisterRoutes registers the MCP endpoint with project-scoped authentication.
// Route: /mcp/{pid} where {pid} must match the project ID in the JWT token.
func (h *MCPHandler) RegisterRoutes(mux *http.ServeMux, mcpAuthMiddleware *mcpauth.Middleware) {
	// Wrap the MCP HTTP server with middleware layers:
	// 1. MCP request/response logging (innermost - logs JSON-RPC details)
	// 2. Authentication (middle - validates JWT token)
	// 3. Method check (outermost - rejects non-POST before auth)
	loggedHandler := middleware.MCPRequestLogger(h.logger, h.mcpConfig)(h.httpServer)
	authHandler := mcpAuthMiddleware.RequireAuth("pid")(loggedHandler)
	methodCheckedHandler := h.requirePOST(authHandler)
	mux.Handle("/mcp/{pid}", methodCheckedHandler)
}

// requirePOST returns 405 Method Not Allowed for non-POST requests.
// MCP over HTTP Streaming requires POST for JSON-RPC requests.
func (h *MCPHandler) requirePOST(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}
