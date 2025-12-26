// Package mcpauth provides MCP-specific authentication middleware.
// It wraps the core auth service with RFC 6750 Bearer token error responses.
package mcpauth

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// Middleware provides MCP-specific authentication middleware.
// Unlike the general auth middleware, this returns RFC 6750 WWW-Authenticate
// headers for OAuth 2.0 Bearer token authentication errors.
type Middleware struct {
	authService auth.AuthService
	logger      *zap.Logger
}

// NewMiddleware creates a new MCP auth middleware.
func NewMiddleware(authService auth.AuthService, logger *zap.Logger) *Middleware {
	return &Middleware{
		authService: authService,
		logger:      logger,
	}
}

// RequireAuth validates JWT and requires project ID to match URL path.
// The pathParamName is the name used in r.PathValue() (e.g., "pid").
// Returns RFC 6750 WWW-Authenticate headers on authentication failures.
func (m *Middleware) RequireAuth(pathParamName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				m.logger.Warn("MCP auth failed: project ID mismatch",
					zap.String("url_project_id", urlProjectID),
					zap.String("token_project_id", claims.ProjectID))
				m.writeWWWAuthenticate(w, http.StatusForbidden, "insufficient_scope", "The access token does not have access to this project")
				return
			}

			// Inject claims and token into context
			ctx := context.WithValue(r.Context(), auth.ClaimsKey, claims)
			ctx = context.WithValue(ctx, auth.TokenKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeWWWAuthenticate writes an RFC 6750 Bearer token error response.
// See: https://datatracker.ietf.org/doc/html/rfc6750#section-3
func (m *Middleware) writeWWWAuthenticate(w http.ResponseWriter, status int, errorCode, description string) {
	// RFC 6750 Section 3: WWW-Authenticate header format
	headerValue := `Bearer error="` + errorCode + `", error_description="` + description + `"`
	w.Header().Set("WWW-Authenticate", headerValue)
	w.WriteHeader(status)
}
