package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// MCPTokenResponse represents the OAuth 2.0 token response for MCP clients.
type MCPTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
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
