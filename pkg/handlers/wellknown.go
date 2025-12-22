package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// OAuthServerMetadata represents the OAuth 2.0 Authorization Server Metadata (RFC 8414).
type OAuthServerMetadata struct {
	Issuer                            string   `json:"issuer"`
	AuthorizationEndpoint             string   `json:"authorization_endpoint"`
	TokenEndpoint                     string   `json:"token_endpoint"`
	JWKSUri                           string   `json:"jwks_uri,omitempty"`
	ScopesSupported                   []string `json:"scopes_supported,omitempty"`
	ResponseTypesSupported            []string `json:"response_types_supported"`
	GrantTypesSupported               []string `json:"grant_types_supported"`
	CodeChallengeMethodsSupported     []string `json:"code_challenge_methods_supported"`
	TokenEndpointAuthMethodsSupported []string `json:"token_endpoint_auth_methods_supported,omitempty"`
}

// WellKnownHandler handles /.well-known/* endpoints.
type WellKnownHandler struct {
	cfg    *config.Config
	logger *zap.Logger
}

// NewWellKnownHandler creates a new WellKnownHandler.
func NewWellKnownHandler(cfg *config.Config, logger *zap.Logger) *WellKnownHandler {
	return &WellKnownHandler{
		cfg:    cfg,
		logger: logger,
	}
}

// RegisterRoutes registers well-known endpoints.
func (h *WellKnownHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", h.OAuthDiscovery)
}

// OAuthDiscovery serves the OAuth 2.0 Authorization Server Metadata (RFC 8414).
//
// Query parameters:
//   - auth_url: Optional auth server URL. Must be in JWKS endpoints whitelist.
//     If provided and valid, returns discovery metadata for that auth server.
//     If provided but invalid, returns 400 error.
//     If not provided, uses default config.AuthServerURL.
func (h *WellKnownHandler) OAuthDiscovery(w http.ResponseWriter, r *http.Request) {
	authURL := r.URL.Query().Get("auth_url")
	validatedAuthURL, errMsg := h.cfg.ValidateAuthURL(authURL)

	if errMsg != "" {
		h.logger.Warn("Invalid auth_url rejected in OAuth discovery",
			zap.String("auth_url", authURL),
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("error", errMsg))
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_auth_url", "Invalid auth_url: not in allowed list"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	metadata := OAuthServerMetadata{
		Issuer:                validatedAuthURL,
		AuthorizationEndpoint: validatedAuthURL + "/authorize",
		TokenEndpoint:         validatedAuthURL + "/token",
		JWKSUri:               validatedAuthURL + "/.well-known/jwks.json",
		ResponseTypesSupported: []string{
			"code",
		},
		GrantTypesSupported: []string{
			"authorization_code",
		},
		CodeChallengeMethodsSupported: []string{
			"S256",
		},
		ScopesSupported: []string{
			"project:access",
		},
		TokenEndpointAuthMethodsSupported: []string{
			"none",
		},
	}

	// Don't cache if auth_url was provided (dynamic response)
	if authURL == "" {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	} else {
		w.Header().Set("Cache-Control", "private, no-cache")
	}

	if err := WriteJSON(w, http.StatusOK, metadata); err != nil {
		h.logger.Error("Failed to encode OAuth metadata", zap.Error(err))
	}
}
