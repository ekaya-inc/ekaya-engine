package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/models"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// AIConfigRequest for PUT body.
type AIConfigRequest struct {
	ConfigType       string `json:"config_type"`
	LLMBaseURL       string `json:"llm_base_url,omitempty"`
	LLMAPIKey        string `json:"llm_api_key,omitempty"`
	LLMModel         string `json:"llm_model,omitempty"`
	EmbeddingBaseURL string `json:"embedding_base_url,omitempty"`
	EmbeddingAPIKey  string `json:"embedding_api_key,omitempty"`
	EmbeddingModel   string `json:"embedding_model,omitempty"`
}

// AIConfigResponse for GET response.
type AIConfigResponse struct {
	ProjectID        string `json:"project_id"`
	ConfigType       string `json:"config_type"`
	LLMBaseURL       string `json:"llm_base_url,omitempty"`
	LLMAPIKey        string `json:"llm_api_key,omitempty"` // Masked
	LLMModel         string `json:"llm_model,omitempty"`
	EmbeddingBaseURL string `json:"embedding_base_url,omitempty"`
	EmbeddingAPIKey  string `json:"embedding_api_key,omitempty"` // Masked
	EmbeddingModel   string `json:"embedding_model,omitempty"`
	LastTestedAt     string `json:"last_tested_at,omitempty"`
	LastTestSuccess  *bool  `json:"last_test_success,omitempty"`
}

// AIConfigHandler handles AI configuration HTTP requests.
type AIConfigHandler struct {
	service          services.AIConfigService
	connectionTester llm.ConnectionTester
	cfg              *config.Config
	logger           *zap.Logger
}

// NewAIConfigHandler creates a new AI config handler.
func NewAIConfigHandler(
	service services.AIConfigService,
	connectionTester llm.ConnectionTester,
	cfg *config.Config,
	logger *zap.Logger,
) *AIConfigHandler {
	return &AIConfigHandler{
		service:          service,
		connectionTester: connectionTester,
		cfg:              cfg,
		logger:           logger,
	}
}

// RegisterRoutes registers the AI config routes.
func (h *AIConfigHandler) RegisterRoutes(mux *http.ServeMux, authMiddleware *auth.Middleware, tenantMiddleware TenantMiddleware) {
	mux.HandleFunc("GET /api/projects/{pid}/ai-config",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.Get)))
	mux.HandleFunc("PUT /api/projects/{pid}/ai-config",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Upsert))))
	mux.HandleFunc("DELETE /api/projects/{pid}/ai-config",
		authMiddleware.RequireAuthWithPathValidation("pid")(
			auth.RequireRole(models.RoleAdmin)(tenantMiddleware(h.Delete))))
	mux.HandleFunc("POST /api/projects/{pid}/ai-config/test",
		authMiddleware.RequireAuthWithPathValidation("pid")(tenantMiddleware(h.TestConnection)))
}

// Get returns AI config for project (masked keys).
func (h *AIConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	cfg, err := h.service.Get(r.Context(), projectID)
	if err != nil {
		h.logger.Error("Failed to get AI config", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to get AI config"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := AIConfigResponse{
		ProjectID:  projectID.String(),
		ConfigType: "none",
	}

	if cfg != nil {
		response.ConfigType = string(cfg.ConfigType)
		response.LLMBaseURL = cfg.LLMBaseURL
		response.LLMAPIKey = models.MaskedAPIKey(cfg.LLMAPIKey)
		response.LLMModel = cfg.LLMModel
		response.EmbeddingBaseURL = cfg.EmbeddingBaseURL
		response.EmbeddingAPIKey = models.MaskedAPIKey(cfg.EmbeddingAPIKey)
		response.EmbeddingModel = cfg.EmbeddingModel
		response.LastTestSuccess = cfg.LastTestSuccess
		if cfg.LastTestedAt != nil {
			response.LastTestedAt = cfg.LastTestedAt.Format(time.RFC3339)
		}
	}

	if err := WriteJSON(w, http.StatusOK, ApiResponse{Success: true, Data: response}); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

// Upsert creates or updates AI config.
func (h *AIConfigHandler) Upsert(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req AIConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate config_type
	configType := models.AIConfigType(req.ConfigType)
	switch configType {
	case models.AIConfigNone, models.AIConfigBYOK, models.AIConfigCommunity, models.AIConfigEmbedded:
		// Valid
	default:
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_config_type", "Must be none, byok, community, or embedded"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	cfg := &models.AIConfig{
		ConfigType:       configType,
		LLMBaseURL:       req.LLMBaseURL,
		LLMAPIKey:        req.LLMAPIKey,
		LLMModel:         req.LLMModel,
		EmbeddingBaseURL: req.EmbeddingBaseURL,
		EmbeddingAPIKey:  req.EmbeddingAPIKey,
		EmbeddingModel:   req.EmbeddingModel,
	}

	if err := h.service.Upsert(r.Context(), projectID, cfg); err != nil {
		h.logger.Error("Failed to save AI config", zap.Error(err))
		if err := ErrorResponse(w, http.StatusBadRequest, "save_failed", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	h.logger.Info("AI config saved",
		zap.String("project_id", projectID.String()),
		zap.String("config_type", string(configType)))

	// Return saved config (re-fetch to get masked keys)
	h.Get(w, r)
}

// Delete removes AI config.
func (h *AIConfigHandler) Delete(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	if err := h.service.Delete(r.Context(), projectID); err != nil {
		h.logger.Error("Failed to delete AI config", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "internal_error", "Failed to delete AI config"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	h.logger.Info("AI config deleted", zap.String("project_id", projectID.String()))
	w.WriteHeader(http.StatusNoContent)
}

// TestConnection tests AI connection.
func (h *AIConfigHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	pidStr := r.PathValue("pid")
	projectID, err := uuid.Parse(pidStr)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_project_id", "Invalid project ID format"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	var req AIConfigRequest
	if r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
	}

	testConfig, err := h.buildTestConfig(r, projectID, &req)
	if err != nil {
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_config", err.Error()); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	result := h.connectionTester.Test(r.Context(), testConfig)

	// Update test result if using saved config
	if req.LLMBaseURL == "" && req.ConfigType != "community" && req.ConfigType != "embedded" {
		_ = h.service.UpdateTestResult(r.Context(), projectID, result.Success)
	}

	resp := ApiResponse{Success: result.Success, Data: result}
	if result.Success {
		resp.Message = result.Message
	} else {
		resp.Error = result.Message
	}
	if err := WriteJSON(w, http.StatusOK, resp); err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

func (h *AIConfigHandler) buildTestConfig(r *http.Request, projectID uuid.UUID, req *AIConfigRequest) (*llm.TestConfig, error) {
	// Test server config (community)
	if req.ConfigType == "community" {
		if !h.cfg.CommunityAI.IsAvailable() {
			return nil, errCommunityNotConfigured
		}
		return &llm.TestConfig{
			LLMBaseURL:       h.cfg.CommunityAI.LLMBaseURL,
			LLMModel:         h.cfg.CommunityAI.LLMModel,
			EmbeddingBaseURL: h.cfg.CommunityAI.EmbeddingURL,
			EmbeddingModel:   h.cfg.CommunityAI.EmbeddingModel,
		}, nil
	}

	// Test server config (embedded)
	if req.ConfigType == "embedded" {
		if !h.cfg.EmbeddedAI.IsAvailable() {
			return nil, errEmbeddedNotConfigured
		}
		return &llm.TestConfig{
			LLMBaseURL:       h.cfg.EmbeddedAI.LLMBaseURL,
			LLMModel:         h.cfg.EmbeddedAI.LLMModel,
			EmbeddingBaseURL: h.cfg.EmbeddedAI.EmbeddingURL,
			EmbeddingModel:   h.cfg.EmbeddedAI.EmbeddingModel,
		}, nil
	}

	// Test provided credentials
	if req.LLMBaseURL != "" {
		return &llm.TestConfig{
			LLMBaseURL:       req.LLMBaseURL,
			LLMAPIKey:        req.LLMAPIKey,
			LLMModel:         req.LLMModel,
			EmbeddingBaseURL: req.EmbeddingBaseURL,
			EmbeddingAPIKey:  req.EmbeddingAPIKey,
			EmbeddingModel:   req.EmbeddingModel,
		}, nil
	}

	// Test saved config
	saved, err := h.service.Get(r.Context(), projectID)
	if err != nil {
		return nil, err
	}
	if saved == nil || saved.ConfigType == models.AIConfigNone {
		return nil, errNoSavedConfig
	}

	return &llm.TestConfig{
		LLMBaseURL:       saved.LLMBaseURL,
		LLMAPIKey:        saved.LLMAPIKey,
		LLMModel:         saved.LLMModel,
		EmbeddingBaseURL: saved.EmbeddingBaseURL,
		EmbeddingAPIKey:  saved.EmbeddingAPIKey,
		EmbeddingModel:   saved.EmbeddingModel,
	}, nil
}

// Error sentinels for buildTestConfig
var (
	errCommunityNotConfigured = &configError{"community AI not configured on server"}
	errEmbeddedNotConfigured  = &configError{"embedded AI not configured on server"}
	errNoSavedConfig          = &configError{"no saved config and no credentials provided"}
)

type configError struct {
	msg string
}

func (e *configError) Error() string {
	return e.msg
}
