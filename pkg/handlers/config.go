package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// ConfigResponse contains public configuration for the frontend.
type ConfigResponse struct {
	AuthServerURL string `json:"auth_server_url"`
	OAuthClientID string `json:"oauth_client_id"`
	BaseURL       string `json:"base_url"`
}

// ConfigHandler handles configuration requests.
type ConfigHandler struct {
	config *config.Config
	logger *zap.Logger
}

// NewConfigHandler creates a new config handler.
func NewConfigHandler(cfg *config.Config, logger *zap.Logger) *ConfigHandler {
	return &ConfigHandler{
		config: cfg,
		logger: logger,
	}
}

// RegisterRoutes registers the config handler's routes on the given mux.
func (h *ConfigHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config", h.Get)
}

// Get returns public configuration for the frontend.
// GET /api/config
// This endpoint is public (no authentication required) as it only exposes
// non-sensitive configuration needed for OAuth flow initialization.
//
// Query parameters:
//   - auth_url: Optional auth server URL. Must be in JWKS endpoints whitelist.
//     If provided and valid, overrides the default auth server URL.
//     If provided but invalid, returns 400 error.
//     If not provided, uses default config.AuthServerURL.
func (h *ConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	// Extract and validate auth_url query parameter
	authURL := r.URL.Query().Get("auth_url")
	validatedAuthURL, errMsg := h.config.ValidateAuthURL(authURL)

	if errMsg != "" {
		h.logger.Warn("Invalid auth_url rejected",
			zap.String("auth_url", authURL),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("error", errMsg))
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_auth_url", "Invalid auth_url: not in allowed list"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := ConfigResponse{
		AuthServerURL: validatedAuthURL,
		OAuthClientID: h.config.OAuth.ClientID,
		BaseURL:       h.config.BaseURL,
	}

	// Don't cache if auth_url was provided (dynamic response)
	if authURL == "" {
		w.Header().Set("Cache-Control", "public, max-age=300") // Cache for 5 minutes
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode config response", zap.Error(err))
		return
	}

	h.logger.Debug("Config request served",
		zap.String("remote_addr", r.RemoteAddr),
		zap.String("auth_server_url", response.AuthServerURL),
		zap.Bool("custom_auth_url", authURL != ""))
}
