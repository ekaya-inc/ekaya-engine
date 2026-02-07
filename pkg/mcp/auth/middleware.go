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

// TenantScopeProvider creates tenant-scoped contexts for database operations.
type TenantScopeProvider interface {
	// WithTenantScope returns a context with tenant scope set for the given project.
	// The cleanup function must be called when the scope is no longer needed.
	WithTenantScope(ctx context.Context, projectID uuid.UUID) (context.Context, func(), error)
}

// AuthFailureLogger records authentication failures to the MCP audit log.
type AuthFailureLogger interface {
	RecordAuthFailure(projectID uuid.UUID, userID, reason, clientIP string)
}

// Middleware provides MCP-specific authentication middleware.
// Unlike the general auth middleware, this returns RFC 6750 WWW-Authenticate
// headers for OAuth 2.0 Bearer token authentication errors.
// Supports both JWT (Bearer) and Agent API key authentication.
type Middleware struct {
	authService     auth.AuthService
	agentKeyService services.AgentAPIKeyService
	tenantProvider  TenantScopeProvider
	auditLogger     AuthFailureLogger
	logger          *zap.Logger
}

// NewMiddleware creates a new MCP auth middleware.
// agentKeyService and tenantProvider can be nil if agent API key authentication is not needed.
// auditLogger can be nil if auth failure auditing is not needed.
func NewMiddleware(authService auth.AuthService, agentKeyService services.AgentAPIKeyService, tenantProvider TenantScopeProvider, logger *zap.Logger, opts ...MiddlewareOption) *Middleware {
	m := &Middleware{
		authService:     authService,
		agentKeyService: agentKeyService,
		tenantProvider:  tenantProvider,
		logger:          logger,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// MiddlewareOption configures optional Middleware behavior.
type MiddlewareOption func(*Middleware)

// WithAuditLogger enables auth failure auditing via the given logger.
func WithAuditLogger(logger AuthFailureLogger) MiddlewareOption {
	return func(m *Middleware) {
		m.auditLogger = logger
	}
}

// RequireAuth validates authentication and requires project ID to match URL path.
// It supports two authentication methods:
//  1. JWT (Bearer): Authorization: Bearer <jwt> (3 dot-separated segments)
//  2. Agent API Key: Authorization: Bearer <key>, Authorization: api-key:<key>, or X-API-Key: <key>
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
				// Explicit api-key: prefix
				apiKey = strings.TrimPrefix(authHeader, "api-key:")
			} else if key := r.Header.Get("X-API-Key"); key != "" {
				// X-API-Key header
				apiKey = key
			} else if strings.HasPrefix(authHeader, "Bearer ") {
				// Bearer token - determine if it's a JWT or API key
				token := strings.TrimPrefix(authHeader, "Bearer ")
				if !isJWT(token) {
					// Not a JWT (no 3 dot-separated segments), treat as API key
					apiKey = token
				}
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

// isJWT checks if a token looks like a JWT (3 dot-separated segments).
// JWTs have the format: header.payload.signature
func isJWT(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 3
}

// handleAgentKeyAuth handles Agent API key authentication.
func (m *Middleware) handleAgentKeyAuth(w http.ResponseWriter, r *http.Request, next http.Handler, pathParamName string, apiKey string) {
	ctx := r.Context()

	if m.agentKeyService == nil {
		m.logger.Error("MCP auth failed: agent key service not configured")
		m.writeWWWAuthenticate(w, http.StatusInternalServerError, "server_error", "Agent authentication not configured")
		return
	}

	if m.tenantProvider == nil {
		m.logger.Error("MCP auth failed: tenant provider not configured")
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

	// Create tenant-scoped context for API key validation
	tenantCtx, cleanup, err := m.tenantProvider.WithTenantScope(ctx, projectID)
	if err != nil {
		m.logger.Error("MCP agent auth failed: could not create tenant scope",
			zap.String("path", r.URL.Path),
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		m.writeWWWAuthenticate(w, http.StatusInternalServerError, "server_error", "Authentication failed")
		return
	}
	defer cleanup()

	// Validate API key
	valid, err := m.agentKeyService.ValidateKey(tenantCtx, projectID, apiKey)
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
		m.recordAuthFailure(projectID, "agent", "Invalid API key", r.RemoteAddr)
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
		// Try to extract project ID for audit logging
		if pid, pidErr := extractProjectIDFromPath(r.URL.Path); pidErr == nil {
			m.recordAuthFailure(pid, "unknown", "Invalid or expired token", r.RemoteAddr)
		}
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
		if pid, pidErr := uuid.Parse(urlProjectID); pidErr == nil {
			m.recordAuthFailure(pid, claims.Subject, "Project ID mismatch", r.RemoteAddr)
		}
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

// recordAuthFailure delegates to the audit logger if configured.
// This is a no-op if no audit logger was provided.
func (m *Middleware) recordAuthFailure(projectID uuid.UUID, userID, reason, clientIP string) {
	if m.auditLogger != nil {
		m.auditLogger.RecordAuthFailure(projectID, userID, reason, clientIP)
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
