package handlers

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// CompleteOAuthRequest represents the request body for OAuth completion.
type CompleteOAuthRequest struct {
	Code         string `json:"code"`
	State        string `json:"state"`
	CodeVerifier string `json:"code_verifier"`
	AuthURL      string `json:"auth_url"`
}

// CompleteOAuthResponse represents the response for OAuth completion.
type CompleteOAuthResponse struct {
	Success     bool   `json:"success"`
	RedirectURL string `json:"redirect_url"`
}

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	oauthService services.OAuthService
	config       *config.Config
	logger       *zap.Logger
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(oauthService services.OAuthService, cfg *config.Config, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		oauthService: oauthService,
		config:       cfg,
		logger:       logger,
	}
}

// RegisterRoutes registers the auth handler's routes on the given mux.
func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/complete-oauth", h.CompleteOAuth)
}

// CompleteOAuth handles POST /api/auth/complete-oauth
// This endpoint is called by the React frontend after receiving the OAuth callback.
// It exchanges the authorization code for a JWT and sets it as an httpOnly cookie.
func (h *AuthHandler) CompleteOAuth(w http.ResponseWriter, r *http.Request) {
	var req CompleteOAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("Invalid request body", zap.Error(err))
		h.errorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body")
		return
	}

	// Validate required fields
	if req.Code == "" || req.State == "" || req.CodeVerifier == "" {
		h.logger.Warn("Missing required parameters")
		h.errorResponse(w, http.StatusBadRequest, "missing_parameters", "Missing code, state, or code_verifier")
		return
	}

	// Exchange code for token
	token, err := h.oauthService.ExchangeCodeForToken(r.Context(), &services.TokenExchangeRequest{
		Code:         req.Code,
		CodeVerifier: req.CodeVerifier,
		AuthURL:      req.AuthURL,
	})
	if err != nil {
		if err == services.ErrInvalidAuthURL {
			h.logger.Warn("Invalid auth_url", zap.String("auth_url", req.AuthURL))
			h.errorResponse(w, http.StatusBadRequest, "invalid_auth_url", "Invalid auth_url: not in allowed list")
			return
		}
		h.logger.Error("Token exchange failed", zap.Error(err))
		h.errorResponse(w, http.StatusInternalServerError, "token_exchange_failed", "Authentication failed")
		return
	}

	// Derive cookie settings from base URL
	cookieSettings := auth.DeriveCookieSettings(h.config.BaseURL, h.config.CookieDomain)

	// Set JWT in httpOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "ekaya_jwt",
		Value:    token,
		HttpOnly: true,
		Secure:   cookieSettings.Secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400, // 24 hours
		Path:     "/",
		Domain:   cookieSettings.Domain,
	})

	// Get original URL from session and clean up
	session, _ := auth.GetSession(r)
	originalURL, _ := session.Values[auth.SessionKeyOriginalURL].(string)
	auth.ClearSessionValues(session)
	if err := auth.SaveSession(r, w, session); err != nil {
		h.logger.Error("Failed to save session", zap.Error(err))
	}

	if originalURL == "" {
		originalURL = "/"
	}

	h.logger.Info("OAuth completion successful",
		zap.String("original_url", originalURL))

	// Return success with redirect URL
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(CompleteOAuthResponse{
		Success:     true,
		RedirectURL: originalURL,
	}); err != nil {
		h.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// errorResponse writes a JSON error response.
func (h *AuthHandler) errorResponse(w http.ResponseWriter, statusCode int, errorCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error":   errorCode,
		"message": message,
	})
}
