package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// GetAgentAPIKeyResponse is the response for GET /api/projects/{pid}/mcp/agent-key.
type GetAgentAPIKeyResponse struct {
	Key    string `json:"key"`    // Masked or full key depending on ?reveal=true
	Masked bool   `json:"masked"` // Whether key is masked
}

// RegenerateAgentAPIKeyResponse is the response for POST /api/projects/{pid}/mcp/agent-key/regenerate.
type RegenerateAgentAPIKeyResponse struct {
	Key string `json:"key"` // New unmasked key
}

// AgentAPIKeyHandler handles agent API key HTTP requests.
type AgentAPIKeyHandler struct {
	agentKeyService services.AgentAPIKeyService
	logger          *zap.Logger
}

// NewAgentAPIKeyHandler creates a new agent API key handler.
func NewAgentAPIKeyHandler(agentKeyService services.AgentAPIKeyService, logger *zap.Logger) *AgentAPIKeyHandler {
	return &AgentAPIKeyHandler{
		agentKeyService: agentKeyService,
		logger:          logger,
	}
}

// RegisterRoutes registers the agent API key handler's routes on the given mux.
func (h *AgentAPIKeyHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	keyBase := "/api/projects/{pid}/mcp/agent-key"

	mux.HandleFunc("GET "+keyBase,
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("POST "+keyBase+"/regenerate",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Regenerate)))
}

// Get handles GET /api/projects/{pid}/mcp/agent-key
// Returns the agent API key (masked by default, or full key with ?reveal=true).
// Auto-generates a new key if one doesn't exist.
func (h *AgentAPIKeyHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	key, err := h.agentKeyService.GetKey(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get agent API key",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get agent API key"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Generate key if it doesn't exist
	if key == "" {
		key, err = h.agentKeyService.GenerateKey(r.Context(), projectID)
		if err != nil {
			h.logger.Error("Failed to generate agent API key",
				zap.String("project_id", projectID.String()),
				zap.Error(err))
			if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to generate agent API key"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
	}

	// Check if ?reveal=true query parameter is present
	reveal := r.URL.Query().Get("reveal") == "true"

	responseKey := key
	masked := false
	if !reveal {
		responseKey = "****"
		masked = true
	} else {
		// Audit log: key was revealed
		h.logger.Info("Agent API key revealed",
			zap.String("project_id", projectID.String()))
	}

	response := ApiResponse{
		Success: true,
		Data: GetAgentAPIKeyResponse{
			Key:    responseKey,
			Masked: masked,
		},
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Regenerate handles POST /api/projects/{pid}/mcp/agent-key/regenerate
// Generates a new key, invalidating any previous key.
func (h *AgentAPIKeyHandler) Regenerate(w http.ResponseWriter, r *http.Request) {
	projectID, ok := ParseProjectID(w, r, h.logger)
	if !ok {
		return
	}

	newKey, err := h.agentKeyService.RegenerateKey(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to regenerate agent API key",
			zap.String("project_id", projectID.String()),
			zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to regenerate agent API key"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ApiResponse{
		Success: true,
		Data: RegenerateAgentAPIKeyResponse{
			Key: newKey,
		},
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}
