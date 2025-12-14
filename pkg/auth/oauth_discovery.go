package auth

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// OAuthServerMetadata represents the OAuth 2.0 Authorization Server Metadata (RFC 8414)
// This is used by clients to discover OAuth endpoints
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

// HandleOAuthDiscovery serves the OAuth 2.0 Authorization Server Metadata
// This endpoint is required per RFC 8414
// Endpoint: /.well-known/oauth-authorization-server
//
// Query parameters:
//   - auth_url: Optional auth server URL. Must be in JWKS endpoints whitelist.
//     If provided and valid, returns discovery metadata for that auth server.
//     If provided but invalid, returns 400 error.
//     If not provided, uses default config.AuthServerURL.
func HandleOAuthDiscovery(cfg *config.Config, logger *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract and validate auth_url query parameter
		authURL := r.URL.Query().Get("auth_url")
		validatedAuthURL, errMsg := cfg.ValidateAuthURL(authURL)

		if errMsg != "" {
			logger.Warn("Invalid auth_url rejected in OAuth discovery",
				zap.String("auth_url", authURL),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("error", errMsg))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "invalid_auth_url",
				"message": "Invalid auth_url: not in allowed list",
			})
			return
		}

		// Build metadata pointing to the validated auth server
		metadata := OAuthServerMetadata{
			Issuer:                validatedAuthURL,
			AuthorizationEndpoint: validatedAuthURL + "/authorize",
			TokenEndpoint:         validatedAuthURL + "/token",
			JWKSUri:               validatedAuthURL + "/.well-known/jwks.json",
			ResponseTypesSupported: []string{
				"code", // Authorization Code Flow (required for OAuth 2.1)
			},
			GrantTypesSupported: []string{
				"authorization_code", // User-delegated scenarios
			},
			CodeChallengeMethodsSupported: []string{
				"S256", // PKCE required for all clients per OAuth 2.1
			},
			ScopesSupported: []string{
				"project:access", // Project-scoped access
			},
			TokenEndpointAuthMethodsSupported: []string{
				"none", // Public clients (PKCE provides security)
			},
		}

		w.Header().Set("Content-Type", "application/json")
		// Don't cache if auth_url was provided (dynamic response)
		if authURL == "" {
			w.Header().Set("Cache-Control", "public, max-age=3600") // Cache for 1 hour
		} else {
			w.Header().Set("Cache-Control", "private, no-cache")
		}

		if err := json.NewEncoder(w).Encode(metadata); err != nil {
			logger.Error("Failed to encode OAuth metadata", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logger.Debug("Served OAuth discovery metadata",
			zap.String("remote_addr", r.RemoteAddr),
			zap.String("user_agent", r.UserAgent()),
			zap.String("auth_server_url", validatedAuthURL),
			zap.Bool("custom_auth_url", authURL != ""))
	}
}
