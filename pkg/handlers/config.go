package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// ConfigResponse contains public configuration for the frontend.
// Note: auth_server_url is not included here - frontend gets it from
// /.well-known/oauth-authorization-server (RFC 8414) issuer field.
type ConfigResponse struct {
	OAuthClientID string `json:"oauth_client_id"`
	BaseURL       string `json:"base_url"`
}

// ConfigHandler handles configuration requests.
type ConfigHandler struct {
	config         *config.Config
	adapterFactory datasource.DatasourceAdapterFactory
	logger         *zap.Logger
}

// NewConfigHandler creates a new config handler.
func NewConfigHandler(cfg *config.Config, adapterFactory datasource.DatasourceAdapterFactory, logger *zap.Logger) *ConfigHandler {
	return &ConfigHandler{
		config:         cfg,
		adapterFactory: adapterFactory,
		logger:         logger,
	}
}

// RegisterRoutes registers the config handler's routes on the given mux.
func (h *ConfigHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config/auth", h.Get)
	mux.HandleFunc("GET /api/config/datasource-types", h.GetDatasourceTypes)
	mux.HandleFunc("GET /api/config/server-status", h.GetServerStatus)
}

// Get returns public configuration for the frontend.
// GET /api/config/auth
// This endpoint is public (no authentication required) as it only exposes
// non-sensitive configuration needed for OAuth flow initialization.
func (h *ConfigHandler) Get(w http.ResponseWriter, r *http.Request) {
	response := ConfigResponse{
		OAuthClientID: h.config.OAuth.ClientID,
		BaseURL:       h.config.BaseURL,
	}

	w.Header().Set("Cache-Control", "public, max-age=300") // Cache for 5 minutes

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode config response", zap.Error(err))
	}
}

// ServerStatusResponse contains server accessibility information for the frontend.
type ServerStatusResponse struct {
	BaseURL                    string `json:"base_url"`
	IsLocalhost                bool   `json:"is_localhost"`
	IsHTTPS                    bool   `json:"is_https"`
	AccessibleForBusinessUsers bool   `json:"accessible_for_business_users"`
}

// GetServerStatus returns server accessibility status.
// GET /api/config/server-status
// This endpoint is public (derived from the already-public base_url).
func (h *ConfigHandler) GetServerStatus(w http.ResponseWriter, r *http.Request) {
	baseURL := h.config.BaseURL
	isHTTPS := strings.HasPrefix(baseURL, "https://")

	isLocalhost := false
	if parsed, err := url.Parse(baseURL); err == nil {
		hostname := parsed.Hostname()
		isLocalhost = hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1"
	}

	response := ServerStatusResponse{
		BaseURL:                    baseURL,
		IsLocalhost:                isLocalhost,
		IsHTTPS:                    isHTTPS,
		AccessibleForBusinessUsers: !isLocalhost && isHTTPS,
	}

	w.Header().Set("Cache-Control", "public, max-age=60")

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode server status response", zap.Error(err))
	}
}

// GetDatasourceTypes returns available datasource adapter types.
// GET /api/config/datasource-types
// This endpoint is public (no authentication required) as it only exposes
// which datasource types are compiled into the binary.
func (h *ConfigHandler) GetDatasourceTypes(w http.ResponseWriter, r *http.Request) {
	types := h.adapterFactory.ListTypes()

	w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour

	if err := WriteJSON(w, http.StatusOK, types); err != nil {
		h.logger.Error("Failed to encode datasource types response", zap.Error(err))
	}
}
