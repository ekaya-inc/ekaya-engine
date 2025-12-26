package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// MCPTokenResponse represents the OAuth 2.0 token response for MCP clients.
type MCPTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// DCRRequest represents an OAuth 2.0 Dynamic Client Registration request (RFC 7591).
type DCRRequest struct {
	RedirectURIs            []string `json:"redirect_uris"`
	ClientName              string   `json:"client_name,omitempty"`
	ClientURI               string   `json:"client_uri,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
	SoftwareID              string   `json:"software_id,omitempty"`
	SoftwareVersion         string   `json:"software_version,omitempty"`
}

// DCRResponse represents an OAuth 2.0 Dynamic Client Registration response (RFC 7591).
type DCRResponse struct {
	ClientID                string   `json:"client_id"`
	ClientSecret            string   `json:"client_secret,omitempty"`
	ClientIDIssuedAt        int64    `json:"client_id_issued_at,omitempty"`
	ClientSecretExpiresAt   int64    `json:"client_secret_expires_at,omitempty"`
	RedirectURIs            []string `json:"redirect_uris,omitempty"`
	ClientName              string   `json:"client_name,omitempty"`
	TokenEndpointAuthMethod string   `json:"token_endpoint_auth_method,omitempty"`
	GrantTypes              []string `json:"grant_types,omitempty"`
	ResponseTypes           []string `json:"response_types,omitempty"`
	Scope                   string   `json:"scope,omitempty"`
}

// MCPOAuthHandler handles MCP OAuth token exchange.
type MCPOAuthHandler struct {
	oauthService services.OAuthService
	logger       *zap.Logger
}

// NewMCPOAuthHandler creates a new MCP OAuth handler.
func NewMCPOAuthHandler(oauthService services.OAuthService, logger *zap.Logger) *MCPOAuthHandler {
	return &MCPOAuthHandler{
		oauthService: oauthService,
		logger:       logger,
	}
}

// RegisterRoutes registers MCP OAuth endpoints.
func (h *MCPOAuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /mcp/oauth/token", h.TokenExchange)
	mux.HandleFunc("POST /mcp/oauth/register", h.DynamicClientRegistration)
}

// TokenExchange handles POST /mcp/oauth/token
// This endpoint exchanges an authorization code for a Bearer token.
// Unlike the browser OAuth flow, this returns the token as JSON (no cookies).
//
// Request (application/x-www-form-urlencoded):
//
//	grant_type=authorization_code
//	code=<authorization_code>
//	code_verifier=<pkce_verifier>
//	redirect_uri=<callback_uri>
//	client_id=<client_id>
//	auth_url=<optional_auth_server_url>
//
// Response:
//
//	{"access_token": "...", "token_type": "Bearer", "expires_in": 3600}
func (h *MCPOAuthHandler) TokenExchange(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.logger.Warn("Failed to parse form", zap.Error(err))
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Failed to parse request body")
		return
	}

	grantType := r.FormValue("grant_type")
	code := r.FormValue("code")
	codeVerifier := r.FormValue("code_verifier")
	redirectURI := r.FormValue("redirect_uri")
	clientID := r.FormValue("client_id")
	authURL := r.FormValue("auth_url")

	// Validate grant_type
	if grantType != "authorization_code" {
		h.logger.Warn("Unsupported grant_type", zap.String("grant_type", grantType))
		h.writeOAuthError(w, http.StatusBadRequest, "unsupported_grant_type", "Only authorization_code grant is supported")
		return
	}

	// Validate required fields
	if code == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameter: code")
		return
	}
	if codeVerifier == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameter: code_verifier")
		return
	}
	if redirectURI == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameter: redirect_uri")
		return
	}
	if clientID == "" {
		h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Missing required parameter: client_id")
		return
	}

	// Exchange code for token
	token, err := h.oauthService.ExchangeCodeForToken(r.Context(), &services.TokenExchangeRequest{
		Code:         code,
		CodeVerifier: codeVerifier,
		RedirectURI:  redirectURI,
		ClientID:     clientID,
		AuthURL:      authURL,
	})
	if err != nil {
		if err == services.ErrInvalidAuthURL {
			h.logger.Warn("Invalid auth_url", zap.String("auth_url", authURL))
			h.writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid auth_url: not in allowed list")
			return
		}
		h.logger.Error("Token exchange failed", zap.Error(err))
		h.writeOAuthError(w, http.StatusInternalServerError, "server_error", "Token exchange failed")
		return
	}

	// Return token response
	response := MCPTokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		ExpiresIn:   86400, // 24 hours (matches cookie expiry)
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode token response", zap.Error(err))
	}
}

// writeOAuthError writes an OAuth 2.0 error response (RFC 6749 Section 5.2).
func (h *MCPOAuthHandler) writeOAuthError(w http.ResponseWriter, status int, errorCode, description string) {
	response := map[string]string{
		"error":             errorCode,
		"error_description": description,
	}
	if err := WriteJSON(w, status, response); err != nil {
		h.logger.Error("Failed to write OAuth error response", zap.Error(err))
	}
}

// validateRedirectURI validates a single redirect URI per RFC 7591 recommendations.
// Returns an error message if invalid, empty string if valid.
func validateRedirectURI(uri string) string {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "malformed URI"
	}

	// Must have a scheme
	if parsed.Scheme == "" {
		return "missing scheme"
	}

	// Must have a host
	if parsed.Host == "" {
		return "missing host"
	}

	// Allow localhost for development (http or https)
	host := parsed.Hostname()
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "" // Valid for localhost
	}

	// For non-localhost, require HTTPS
	if parsed.Scheme != "https" {
		return "non-localhost URIs must use HTTPS"
	}

	return "" // Valid
}

// DynamicClientRegistration handles POST /mcp/oauth/register
// RFC 7591 compliant - allows MCP clients to register without pre-registration.
// Since we use PKCE for security (public clients), we return a well-known client_id
// but no client_secret.
func (h *MCPOAuthHandler) DynamicClientRegistration(w http.ResponseWriter, r *http.Request) {
	var req DCRRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("Invalid DCR request", zap.Error(err))
		h.writeDCRError(w, http.StatusBadRequest, "invalid_client_metadata", "Invalid JSON request body")
		return
	}

	// Validate redirect_uris is provided (required per RFC 7591)
	if len(req.RedirectURIs) == 0 {
		h.logger.Warn("DCR request missing redirect_uris")
		h.writeDCRError(w, http.StatusBadRequest, "invalid_redirect_uri", "redirect_uris is required")
		return
	}

	// Validate each redirect_uri format and security requirements
	for _, uri := range req.RedirectURIs {
		if errMsg := validateRedirectURI(uri); errMsg != "" {
			h.logger.Warn("Invalid redirect_uri in DCR request",
				zap.String("redirect_uri", uri),
				zap.String("error", errMsg))
			h.writeDCRError(w, http.StatusBadRequest, "invalid_redirect_uri", "Invalid redirect_uri: "+errMsg)
			return
		}
	}

	// Use the well-known MCP client_id that ekaya-central recognizes.
	// This is a public client (PKCE provides security, no client_secret needed).
	// The client_id is pre-registered in ekaya-central's OAUTH_CLIENTS registry.
	clientID := "ekaya-mcp"

	// Set defaults for optional fields
	tokenEndpointAuthMethod := req.TokenEndpointAuthMethod
	if tokenEndpointAuthMethod == "" {
		tokenEndpointAuthMethod = "none" // Public client (PKCE provides security)
	}

	grantTypes := req.GrantTypes
	if len(grantTypes) == 0 {
		grantTypes = []string{"authorization_code"}
	}

	responseTypes := req.ResponseTypes
	if len(responseTypes) == 0 {
		responseTypes = []string{"code"}
	}

	scope := req.Scope
	if scope == "" {
		scope = "project:access"
	}

	response := DCRResponse{
		ClientID:                clientID,
		ClientIDIssuedAt:        time.Now().Unix(),
		ClientSecretExpiresAt:   0, // No expiration for public clients
		RedirectURIs:            req.RedirectURIs,
		ClientName:              req.ClientName,
		TokenEndpointAuthMethod: tokenEndpointAuthMethod,
		GrantTypes:              grantTypes,
		ResponseTypes:           responseTypes,
		Scope:                   scope,
	}

	w.Header().Set("Cache-Control", "no-store")
	if err := WriteJSON(w, http.StatusCreated, response); err != nil {
		h.logger.Error("Failed to encode DCR response", zap.Error(err))
		return
	}

	h.logger.Info("Dynamic client registration successful",
		zap.String("client_id", clientID),
		zap.String("client_name", req.ClientName),
		zap.Strings("redirect_uris", req.RedirectURIs),
		zap.String("remote_addr", r.RemoteAddr))
}

// writeDCRError writes a DCR error response (RFC 7591 Section 3.2.2).
func (h *MCPOAuthHandler) writeDCRError(w http.ResponseWriter, status int, errorCode, description string) {
	response := map[string]string{
		"error":             errorCode,
		"error_description": description,
	}
	if err := WriteJSON(w, status, response); err != nil {
		h.logger.Error("Failed to write DCR error response", zap.Error(err))
	}
}
