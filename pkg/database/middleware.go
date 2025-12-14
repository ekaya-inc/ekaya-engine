package database

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// WithTenantContext creates middleware that sets up a tenant-scoped DB connection.
// It runs AFTER auth middleware and uses the project ID from JWT claims.
// The connection is automatically cleaned up after the handler returns.
func WithTenantContext(db *DB, logger *zap.Logger) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := auth.GetClaims(r.Context())
			if !ok || claims.ProjectID == "" {
				logger.Error("Missing project context in claims")
				writeError(w, http.StatusInternalServerError, "internal_error", "Missing project context")
				return
			}

			projectID, err := uuid.Parse(claims.ProjectID)
			if err != nil {
				logger.Error("Invalid project ID format in claims",
					zap.String("project_id", claims.ProjectID),
					zap.Error(err))
				writeError(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format")
				return
			}

			scope, err := db.WithTenant(r.Context(), projectID)
			if err != nil {
				logger.Error("Failed to acquire tenant connection",
					zap.String("project_id", projectID.String()),
					zap.Error(err))
				writeError(w, http.StatusInternalServerError, "database_error", "Database connection error")
				return
			}
			defer scope.Close()

			ctx := SetTenantScope(r.Context(), scope)
			next(w, r.WithContext(ctx))
		}
	}
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, statusCode int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   errorCode,
		"message": message,
	})
}
