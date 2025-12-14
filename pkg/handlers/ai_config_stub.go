package handlers

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// AIConfigStubHandler provides stub responses for AI config endpoints
// that the frontend expects but ekaya-engine doesn't implement yet.
type AIConfigStubHandler struct {
	logger *zap.Logger
}

// NewAIConfigStubHandler creates a new AI config stub handler.
func NewAIConfigStubHandler(logger *zap.Logger) *AIConfigStubHandler {
	return &AIConfigStubHandler{logger: logger}
}

// RegisterRoutes registers the AI config stub routes.
// These must be registered BEFORE the projects handler to prevent
// /api/projects/ai-config from matching GET /api/projects/{pid}
func (h *AIConfigStubHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware) {
	// AI config endpoints - return "not configured" state
	mux.HandleFunc("GET /api/projects/ai-config", authMiddleware.RequireAuth(h.GetConfig))
	mux.HandleFunc("PUT /api/projects/ai-config", authMiddleware.RequireAuth(h.UpdateConfig))
	mux.HandleFunc("DELETE /api/projects/ai-config", authMiddleware.RequireAuth(h.DeleteConfig))
	mux.HandleFunc("POST /api/projects/ai-config/test", authMiddleware.RequireAuth(h.TestConnection))

	// AI options endpoint - return empty options
	mux.HandleFunc("GET /api/ai-options", authMiddleware.RequireAuth(h.GetOptions))
}

// GetConfig returns empty AI config (not configured state)
func (h *AIConfigStubHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"config_type": "none",
	})
}

// UpdateConfig is a stub that accepts but doesn't persist AI config
func (h *AIConfigStubHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	h.logger.Warn("AI config update attempted but not implemented")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "not_implemented",
		"message": "AI configuration is not yet available in ekaya-engine",
	})
}

// DeleteConfig is a stub for deleting AI config
func (h *AIConfigStubHandler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// TestConnection is a stub for testing AI connection
func (h *AIConfigStubHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	h.logger.Warn("AI connection test attempted but not implemented")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotImplemented)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   "not_implemented",
		"message": "AI configuration is not yet available in ekaya-engine",
	})
}

// GetOptions returns available AI options (empty for now)
func (h *AIConfigStubHandler) GetOptions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"community": nil,
		"embedded":  nil,
	})
}
