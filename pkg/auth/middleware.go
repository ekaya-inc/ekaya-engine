package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"go.uber.org/zap"
)

// Middleware provides HTTP authentication middleware.
// It is thin and delegates authentication logic to AuthService.
type Middleware struct {
	authService AuthService
	logger      *zap.Logger
}

// NewMiddleware creates a new auth middleware with the given AuthService.
func NewMiddleware(authService AuthService, logger *zap.Logger) *Middleware {
	return &Middleware{
		authService: authService,
		logger:      logger,
	}
}

// RequireAuth validates JWT and requires a valid project ID.
// Sets claims and token in context for downstream handlers.
// Use this for endpoints that need authentication but don't have a project ID in the URL.
func (m *Middleware) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, token, err := m.authService.ValidateRequest(r)
		if err != nil {
			m.unauthorized(w, "Authentication required")
			return
		}

		if err := m.authService.RequireProjectID(claims); err != nil {
			m.badRequest(w, "Missing project ID in token")
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		ctx = context.WithValue(ctx, TokenKey, token)

		// Inject Azure token reference ID into context if present
		if claims.AzureTokenRefID != "" {
			ctx = context.WithValue(ctx, AzureTokenRefIDKey, claims.AzureTokenRefID)
		}

		next(w, r.WithContext(ctx))
	}
}

// RequireAuthWithPathValidation validates JWT and matches URL path project ID to token.
// Use for endpoints like /api/projects/{pid} where URL contains project scope.
// pathParamName is the name used in r.PathValue() (e.g., "pid").
func (m *Middleware) RequireAuthWithPathValidation(pathParamName string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, token, err := m.authService.ValidateRequest(r)
			if err != nil {
				m.unauthorized(w, "Authentication required")
				return
			}

			if err := m.authService.RequireProjectID(claims); err != nil {
				m.badRequest(w, "Missing project ID in token")
				return
			}

			// Get path parameter using Go 1.22+ http.ServeMux
			urlProjectID := r.PathValue(pathParamName)

			if err := m.authService.ValidateProjectIDMatch(claims, urlProjectID); err != nil {
				m.forbidden(w, "Project ID mismatch between token and URL")
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			ctx = context.WithValue(ctx, TokenKey, token)

			// Inject Azure token reference ID into context if present
			if claims.AzureTokenRefID != "" {
				ctx = context.WithValue(ctx, AzureTokenRefIDKey, claims.AzureTokenRefID)
			}

			next(w, r.WithContext(ctx))
		}
	}
}

// unauthorized returns a 401 response with JSON error body.
func (m *Middleware) unauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "unauthorized",
		"message": message,
	})
}

// badRequest returns a 400 response with JSON error body.
func (m *Middleware) badRequest(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "bad_request",
		"message": message,
	})
}

// forbidden returns a 403 response with JSON error body.
func (m *Middleware) forbidden(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "forbidden",
		"message": message,
	})
}
