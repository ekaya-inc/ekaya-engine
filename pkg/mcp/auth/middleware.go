// Package mcpauth provides MCP-specific authentication middleware.
// It wraps the core auth service with RFC 6750 Bearer token error responses.
// Supports both JWT (Bearer) and Agent API key authentication methods.
package mcpauth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// Middleware provides MCP-specific authentication middleware.
// Unlike the general auth middleware, this returns RFC 6750 WWW-Authenticate
// headers for OAuth 2.0 Bearer token authentication errors.
// Supports both JWT (Bearer) and Agent API key authentication.
type Middleware struct {
	authService     auth.AuthService
	agentKeyService services.AgentAPIKeyService
	logger          *zap.Logger
}

// NewMiddleware creates a new MCP auth middleware.
// agentKeyService can be nil if agent API key authentication is not needed.
func NewMiddleware(authService auth.AuthService, agentKeyService services.AgentAPIKeyService, logger *zap.Logger) *Middleware {
	return &Middleware{
		authService:     authService,
		agentKeyService: agentKeyService,
		logger:          logger,
	}
}

// RequireAuth validates authentication and requires project ID to match URL path.
// It supports two authentication methods:
//  1. JWT (Bearer): Authorization: Bearer <jwt>
//  2. Agent API Key: Authorization: api-key:<key> OR X-API-Key: <key>
//
// The pathParamName is the name used in r.PathValue() (e.g., "pid").
// Returns RFC 6750 WWW-Authenticate headers on authentication failures.
func (m *Middleware) RequireAuth(pathParamName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try Agent API Key authentication first
			authHeader := r.Header.Get("Authorization")
			apiKey := ""

			if strings.HasPrefix(authHeader, "api-key:") {
				apiKey = strings.TrimPrefix(authHeader, "api-key:")
			} else if key := r.Header.Get("X-API-Key"); key != "" {
				apiKey = key
			}

			if apiKey != "" {
				m.handleAgentKeyAuth(w, r, next, pathParamName, apiKey)
				return
			}

			// Fall through to JWT authentication
			m.handleJWTAuth(w, r, next, pathParamName)
		})
	}
}

// handleAgentKeyAuth handles Agent API key authentication.
func (m *Middleware) handleAgentKeyAuth(w http.ResponseWriter, r *http.Request, next http.Handler, pathParamName string, apiKey string) {
	ctx := r.Context()

	if m.agentKeyService == nil {
		m.logger.Error("MCP auth failed: agent key service not configured")
		m.writeWWWAuthenticate(w, http.StatusInternalServerError, "server_error", "Agent authentication not configured")
		return
	}

	// Extract project ID from URL path
	urlProjectID := r.PathValue(pathParamName)
	if urlProjectID == "" {
		// Fall back to extracting from URL path directly
		var err error
		projectID, err := extractProjectIDFromPath(r.URL.Path)
		if err != nil {
			m.logger.Debug("MCP agent auth failed: missing project ID in URL path",
				zap.String("path", r.URL.Path),
				zap.Error(err))
			m.writeWWWAuthenticate(w, http.StatusBadRequest, "invalid_request", "Invalid project ID in URL")
			return
		}
		urlProjectID = projectID.String()
	}

	projectID, err := uuid.Parse(urlProjectID)
	if err != nil {
		m.logger.Debug("MCP agent auth failed: invalid project ID format",
			zap.String("path", r.URL.Path),
			zap.String("project_id", urlProjectID),
			zap.Error(err))
		m.writeWWWAuthenticate(w, http.StatusBadRequest, "invalid_request", "Invalid project ID format")
		return
	}

	// Validate API key
	valid, err := m.agentKeyService.ValidateKey(ctx, projectID, apiKey)
	if err != nil {
		m.logger.Error("MCP agent auth failed: key validation error",
			zap.String("path", r.URL.Path),
			zap.Error(err))
		m.writeWWWAuthenticate(w, http.StatusInternalServerError, "server_error", "Authentication failed")
		return
	}
	if !valid {
		m.logger.Debug("MCP agent auth failed: invalid API key",
			zap.String("path", r.URL.Path),
			zap.String("project_id", projectID.String()))
		m.writeWWWAuthenticate(w, http.StatusUnauthorized, "invalid_token", "Invalid API key")
		return
	}

	// Create synthetic claims for agent context
	claims := &auth.Claims{
		ProjectID: projectID.String(),
	}
	claims.Subject = "agent" // Special marker for agent authentication

	m.logger.Debug("MCP agent auth successful",
		zap.String("path", r.URL.Path),
		zap.String("project_id", projectID.String()))

	// Inject claims into context (no token for agent auth)
	ctx = context.WithValue(ctx, auth.ClaimsKey, claims)
	next.ServeHTTP(w, r.WithContext(ctx))
}

// handleJWTAuth handles JWT Bearer token authentication.
func (m *Middleware) handleJWTAuth(w http.ResponseWriter, r *http.Request, next http.Handler, pathParamName string) {
	claims, token, err := m.authService.ValidateRequest(r)
	if err != nil {
		m.logger.Debug("MCP auth failed: invalid or missing token",
			zap.String("path", r.URL.Path),
			zap.Error(err))
		m.writeWWWAuthenticate(w, http.StatusUnauthorized, "invalid_token", "The access token is invalid or expired")
		return
	}

	if err := m.authService.RequireProjectID(claims); err != nil {
		m.logger.Debug("MCP auth failed: missing project ID",
			zap.String("path", r.URL.Path))
		m.writeWWWAuthenticate(w, http.StatusUnauthorized, "invalid_token", "The access token is missing required project scope")
		return
	}

	// Extract project ID from URL path
	urlProjectID := r.PathValue(pathParamName)
	if urlProjectID == "" {
		m.logger.Error("MCP auth failed: missing project ID in URL path",
			zap.String("path", r.URL.Path),
			zap.String("path_param", pathParamName))
		m.writeWWWAuthenticate(w, http.StatusBadRequest, "invalid_request", "Missing project ID in URL")
		return
	}

	// Validate project ID match
	if err := m.authService.ValidateProjectIDMatch(claims, urlProjectID); err != nil {
		m.logger.Debug("MCP auth failed: project ID mismatch",
			zap.String("url_project_id", urlProjectID),
			zap.String("token_project_id", claims.ProjectID))
		m.writeWWWAuthenticate(w, http.StatusForbidden, "insufficient_scope", "The access token does not have access to this project")
		return
	}

	// Inject claims and token into context
	ctx := context.WithValue(r.Context(), auth.ClaimsKey, claims)
	ctx = context.WithValue(ctx, auth.TokenKey, token)
	next.ServeHTTP(w, r.WithContext(ctx))
}

// extractProjectIDFromPath extracts project UUID from /mcp/{project-id} path.
func extractProjectIDFromPath(path string) (uuid.UUID, error) {
	// Expected format: /mcp/{project-id} or /mcp/{project-id}/...
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) < 2 || parts[0] != "mcp" {
		return uuid.Nil, fmt.Errorf("invalid path format: expected /mcp/{project-id}")
	}

	return uuid.Parse(parts[1])
}

// writeWWWAuthenticate writes an RFC 6750 Bearer token error response.
// See: https://datatracker.ietf.org/doc/html/rfc6750#section-3
func (m *Middleware) writeWWWAuthenticate(w http.ResponseWriter, status int, errorCode, description string) {
	// RFC 6750 Section 3: WWW-Authenticate header format
	headerValue := `Bearer error="` + errorCode + `", error_description="` + description + `"`
	w.Header().Set("WWW-Authenticate", headerValue)
	w.WriteHeader(status)
}
