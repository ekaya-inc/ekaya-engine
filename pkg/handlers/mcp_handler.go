package handlers

import (
	"net/http"

	"github.com/mark3labs/mcp-go/server"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/mcp"
	mcpauth "github.com/ekaya-inc/ekaya-engine/pkg/mcp/auth"
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

// RegisterRoutes registers the MCP endpoint with project-scoped authentication.
// Route: /mcp/{pid} where {pid} must match the project ID in the JWT token.
func (h *MCPHandler) RegisterRoutes(mux *http.ServeMux, mcpAuthMiddleware *mcpauth.Middleware) {
	// Wrap the MCP HTTP server with method check and authentication middleware
	// Method check must come first to reject non-POST requests before auth
	methodCheckedHandler := h.requirePOST(mcpAuthMiddleware.RequireAuth("pid")(h.httpServer))
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
