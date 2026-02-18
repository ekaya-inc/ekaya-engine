package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/models"
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

// RequireRole returns middleware that checks if the authenticated user has one of the allowed roles.
// Must be used AFTER RequireAuth or RequireAuthWithPathValidation (claims must be in context).
// Returns 403 Forbidden if the user's role is not in the allowed set.
func RequireRole(allowedRoles ...string) func(http.HandlerFunc) http.HandlerFunc {
	allowed := make(map[string]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowed[r] = true
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := GetClaims(r.Context())
			if !ok || claims == nil {
				writeJSONError(w, http.StatusUnauthorized, "unauthorized", "Authentication required")
				return
			}

			for _, role := range claims.Roles {
				if allowed[role] {
					next(w, r)
					return
				}
			}

			writeJSONError(w, http.StatusForbidden, "forbidden", "Insufficient permissions")
		}
	}
}

// writeJSONError writes a JSON error response with the given status code.
func writeJSONError(w http.ResponseWriter, status int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   errCode,
		"message": message,
	})
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

// RequireAuthWithProvenance combines authentication and provenance context injection.
// It validates JWT, requires a valid project ID, and sets manual provenance context.
// Use this for UI API endpoints that modify ontology objects.
func (m *Middleware) RequireAuthWithProvenance(next http.HandlerFunc) http.HandlerFunc {
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

		// Parse user ID from claims.Subject as UUID
		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			m.logger.Warn("Failed to parse user ID as UUID",
				zap.String("subject", claims.Subject),
				zap.Error(err))
			m.badRequest(w, "Invalid user ID format in token")
			return
		}

		ctx := context.WithValue(r.Context(), ClaimsKey, claims)
		ctx = context.WithValue(ctx, TokenKey, token)

		// Inject Azure token reference ID into context if present
		if claims.AzureTokenRefID != "" {
			ctx = context.WithValue(ctx, AzureTokenRefIDKey, claims.AzureTokenRefID)
		}

		// Inject manual provenance context for UI operations
		ctx = models.WithManualProvenance(ctx, userID)

		next(w, r.WithContext(ctx))
	}
}

// RequireAuthWithPathValidationAndProvenance combines path validation and provenance context.
// Use for endpoints like /api/projects/{pid}/entities where URL contains project scope
// and operations modify ontology objects.
func (m *Middleware) RequireAuthWithPathValidationAndProvenance(pathParamName string) func(http.HandlerFunc) http.HandlerFunc {
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

			// Parse user ID from claims.Subject as UUID
			userID, err := uuid.Parse(claims.Subject)
			if err != nil {
				m.logger.Warn("Failed to parse user ID as UUID",
					zap.String("subject", claims.Subject),
					zap.Error(err))
				m.badRequest(w, "Invalid user ID format in token")
				return
			}

			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			ctx = context.WithValue(ctx, TokenKey, token)

			// Inject Azure token reference ID into context if present
			if claims.AzureTokenRefID != "" {
				ctx = context.WithValue(ctx, AzureTokenRefIDKey, claims.AzureTokenRefID)
			}

			// Inject manual provenance context for UI operations
			ctx = models.WithManualProvenance(ctx, userID)

			next(w, r.WithContext(ctx))
		}
	}
}
