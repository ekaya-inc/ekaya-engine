package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// AIOptionConfig contains configuration for a single AI option (community or embedded).
type AIOptionConfig struct {
	Available      bool   `json:"available"`
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
	cfg    *config.Config
	logger *zap.Logger
}

// NewProjectConfigHandler creates a new project config handler.
func NewProjectConfigHandler(cfg *config.Config, logger *zap.Logger) *ProjectConfigHandler {
	return &ProjectConfigHandler{
		cfg:    cfg,
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
			Community: h.buildAIOption(&h.cfg.CommunityAI),
			Embedded:  h.buildAIOption(&h.cfg.EmbeddedAI),
		},
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode project config response", zap.Error(err))
		return
	}

	h.logger.Debug("Project config request served", zap.String("remote_addr", r.RemoteAddr))
}

// buildAIOption converts server config to API response format.
// Returns an AIOptionConfig with Available=false if not configured.
func (h *ProjectConfigHandler) buildAIOption(communityOrEmbedded interface{}) *AIOptionConfig {
	switch c := communityOrEmbedded.(type) {
	case *config.CommunityAIConfig:
		return &AIOptionConfig{
			Available:      c.IsAvailable(),
			LLMBaseURL:     c.LLMBaseURL,
			LLMModel:       c.LLMModel,
			EmbeddingURL:   c.EmbeddingURL,
			EmbeddingModel: c.EmbeddingModel,
		}
	case *config.EmbeddedAIConfig:
		return &AIOptionConfig{
			Available:      c.IsAvailable(),
			LLMBaseURL:     c.LLMBaseURL,
			LLMModel:       c.LLMModel,
			EmbeddingURL:   c.EmbeddingURL,
			EmbeddingModel: c.EmbeddingModel,
		}
	}
	return &AIOptionConfig{Available: false}
}
