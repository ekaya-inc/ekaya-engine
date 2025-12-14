package handlers

import (
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
func (h *AIConfigStubHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware) {
	// AI config endpoints - return "not configured" state
	mux.HandleFunc("GET /api/projects/{pid}/ai-config", authMiddleware.RequireAuthWithPathValidation("pid")(h.GetConfig))
	mux.HandleFunc("PUT /api/projects/{pid}/ai-config", authMiddleware.RequireAuthWithPathValidation("pid")(h.UpdateConfig))
	mux.HandleFunc("DELETE /api/projects/{pid}/ai-config", authMiddleware.RequireAuthWithPathValidation("pid")(h.DeleteConfig))
	mux.HandleFunc("POST /api/projects/{pid}/ai-config/test", authMiddleware.RequireAuthWithPathValidation("pid")(h.TestConnection))

	// AI options endpoint - return empty options
	mux.HandleFunc("GET /api/ai-options", authMiddleware.RequireAuth(h.GetOptions))
}

// GetConfig returns empty AI config (not configured state)
func (h *AIConfigStubHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	if err := WriteJSON(w, http.StatusOK, map[string]interface{}{
		"config_type": "none",
	}); err != nil {
		h.logger.Error("Failed to encode AI config response", zap.Error(err))
	}
}

// UpdateConfig is a stub that accepts but doesn't persist AI config
func (h *AIConfigStubHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	h.logger.Warn("AI config update attempted but not implemented")
	if err := ErrorResponse(w, http.StatusNotImplemented, "not_implemented", "AI configuration is not yet available in ekaya-engine"); err != nil {
		h.logger.Error("Failed to write error response", zap.Error(err))
	}
}

// DeleteConfig is a stub for deleting AI config
func (h *AIConfigStubHandler) DeleteConfig(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// TestConnection is a stub for testing AI connection
func (h *AIConfigStubHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	h.logger.Warn("AI connection test attempted but not implemented")
	if err := ErrorResponse(w, http.StatusNotImplemented, "not_implemented", "AI configuration is not yet available in ekaya-engine"); err != nil {
		h.logger.Error("Failed to write error response", zap.Error(err))
	}
}

// GetOptions returns available AI options (empty for now)
func (h *AIConfigStubHandler) GetOptions(w http.ResponseWriter, r *http.Request) {
	if err := WriteJSON(w, http.StatusOK, map[string]interface{}{
		"community": nil,
		"embedded":  nil,
	}); err != nil {
		h.logger.Error("Failed to encode AI options response", zap.Error(err))
	}
}
