package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
)

// AIOptionConfig contains configuration for a single AI option (community or embedded).
type AIOptionConfig struct {
	LLMBaseURL     string `json:"llm_base_url"`
	LLMModel       string `json:"llm_model"`
	EmbeddingURL   string `json:"embedding_url"`
	EmbeddingModel string `json:"embedding_model"`
}

// AIOptionsResponse contains available AI options.
type AIOptionsResponse struct {
	Community *AIOptionConfig `json:"community"`
	Embedded  *AIOptionConfig `json:"embedded"`
}

// ProjectConfigResponse contains server-level configuration for authenticated projects.
type ProjectConfigResponse struct {
	AIOptions AIOptionsResponse `json:"ai_options"`
}

// ProjectConfigHandler handles project-scoped configuration requests.
type ProjectConfigHandler struct {
	logger *zap.Logger
}

// NewProjectConfigHandler creates a new project config handler.
func NewProjectConfigHandler(logger *zap.Logger) *ProjectConfigHandler {
	return &ProjectConfigHandler{
		logger: logger,
	}
}

// RegisterRoutes registers the project config handler's routes on the given mux.
func (h *ProjectConfigHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware) {
	mux.HandleFunc("GET /api/config/project", authMiddleware.RequireAuth(h.Get))
}

// Get returns server-level configuration for authenticated projects.
// GET /api/config/project
// This endpoint requires authentication and returns configuration relevant
// to project-level features (currently AI options).
func (h *ProjectConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	response := ProjectConfigResponse{
		AIOptions: AIOptionsResponse{
			Community: nil, // Not configured in ekaya-engine yet
			Embedded:  nil, // Not configured in ekaya-engine yet
		},
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode project config response", zap.Error(err))
		return
	}

	h.logger.Debug("Project config request served", zap.String("remote_addr", r.RemoteAddr))
}
