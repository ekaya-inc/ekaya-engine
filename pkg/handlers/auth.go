package handlers

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
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

// LogoutResponse represents the response for logout.
type LogoutResponse struct {
	Success     bool   `json:"success"`
	RedirectURL string `json:"redirect_url"`
}

// AuthHandler handles authentication-related HTTP requests.
type AuthHandler struct {
	oauthService   services.OAuthService
	projectService services.ProjectService
	config         *config.Config
	logger         *zap.Logger
}

// NewAuthHandler creates a new auth handler.
func NewAuthHandler(oauthService services.OAuthService, projectService services.ProjectService, cfg *config.Config, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{
		oauthService:   oauthService,
		projectService: projectService,
		config:         cfg,
		logger:         logger,
	}
}

// RegisterRoutes registers the auth handler's routes on the given mux.
func (h *AuthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/auth/complete-oauth", h.CompleteOAuth)
	mux.HandleFunc("POST /api/projects/{pid}/auth/logout", h.Logout)
}

// CompleteOAuth handles POST /api/auth/complete-oauth
// This endpoint is called by the React frontend after receiving the OAuth callback.
// It exchanges the authorization code for a JWT and sets it as an httpOnly cookie.
func (h *AuthHandler) CompleteOAuth(w http.ResponseWriter, r *http.Request) {
	var req CompleteOAuthRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Warn("Invalid request body", zap.Error(err))
		if err := ErrorResponse(w, http.StatusBadRequest, "invalid_request", "Invalid request body"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	// Validate required fields
	if req.Code == "" || req.State == "" || req.CodeVerifier == "" {
		h.logger.Warn("Missing required parameters")
		if err := ErrorResponse(w, http.StatusBadRequest, "missing_parameters", "Missing code, state, or code_verifier"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
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
			if err := ErrorResponse(w, http.StatusBadRequest, "invalid_auth_url", "Invalid auth_url: not in allowed list"); err != nil {
				h.logger.Error("Failed to write error response", zap.Error(err))
			}
			return
		}
		h.logger.Error("Token exchange failed", zap.Error(err))
		if err := ErrorResponse(w, http.StatusInternalServerError, "token_exchange_failed", "Authentication failed"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
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
	if err := WriteJSON(w, http.StatusOK, CompleteOAuthResponse{
		Success:     true,
		RedirectURL: originalURL,
	}); err != nil {
		h.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// Logout handles POST /api/projects/{pid}/auth/logout
// Clears the ekaya_jwt cookie and returns a redirect URL to ekaya-central project page.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Derive cookie settings from base URL (same settings used when setting the cookie)
	cookieSettings := auth.DeriveCookieSettings(h.config.BaseURL, h.config.CookieDomain)

	// Clear the JWT cookie by setting MaxAge to -1
	http.SetCookie(w, &http.Cookie{
		Name:     "ekaya_jwt",
		Value:    "",
		HttpOnly: true,
		Secure:   cookieSettings.Secure,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1, // Delete immediately
		Path:     "/",
		Domain:   cookieSettings.Domain,
	})

	// Get redirect URL from project's stored project_page_url using pid from path
	pid := r.PathValue("pid")
	redirectURL := h.getProjectPageURL(r.Context(), pid)

	h.logger.Info("User logged out", zap.String("redirect_url", redirectURL))

	// Return success with redirect URL
	if err := WriteJSON(w, http.StatusOK, LogoutResponse{
		Success:     true,
		RedirectURL: redirectURL,
	}); err != nil {
		h.logger.Error("Failed to encode response", zap.Error(err))
	}
}

// getProjectPageURL retrieves the project page URL from the project's stored parameters.
// Falls back to "/" if project not found or URL not available.
func (h *AuthHandler) getProjectPageURL(ctx context.Context, pid string) string {
	if pid == "" {
		return "/"
	}

	projectID, err := uuid.Parse(pid)
	if err != nil {
		return "/"
	}

	project, err := h.projectService.GetByIDWithoutTenant(ctx, projectID)
	if err != nil || project.Parameters == nil {
		return "/"
	}

	if url, ok := project.Parameters["project_page_url"].(string); ok && url != "" {
		return url
	}

	return "/"
}

// GetMeResponse represents the response for the /api/auth/me endpoint.
type GetMeResponse struct {
	Email         string   `json:"email"`
	ProjectID     string   `json:"projectId"`
	Roles         []string `json:"roles"`
	HasAzureToken bool     `json:"hasAzureToken"`
	AzureTokenExp *int64   `json:"azureTokenExp,omitempty"`
}

// GetMe handles GET /api/auth/me
// Returns information about the currently authenticated user, including Azure token status.
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	// Get claims from context (set by auth middleware)
	claims, ok := auth.GetClaims(r.Context())
	if !ok || claims == nil {
		if err := ErrorResponse(w, http.StatusUnauthorized, "unauthorized", "Not authenticated"); err != nil {
			h.logger.Error("Failed to write error response", zap.Error(err))
		}
		return
	}

	response := GetMeResponse{
		Email:         claims.Email,
		ProjectID:     claims.ProjectID,
		Roles:         claims.Roles,
		HasAzureToken: claims.AzureAccessToken != "",
	}

	// Only include expiry if token is present
	if claims.AzureAccessToken != "" && claims.AzureTokenExpiry > 0 {
		response.AzureTokenExp = &claims.AzureTokenExpiry
	}

	if err := WriteJSON(w, http.StatusOK, response); err != nil {
		h.logger.Error("Failed to encode response", zap.Error(err))
	}
}
