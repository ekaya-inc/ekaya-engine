// Package services contains business logic for ekaya-engine.
package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"go.uber.org/zap"
)

// Common OAuth errors.
var (
	ErrInvalidAuthURL      = errors.New("invalid auth URL: not in allowed list")
	ErrTokenExchangeFailed = errors.New("token exchange failed")
)

// OAuthConfig contains configuration for the OAuth service.
type OAuthConfig struct {
	// BaseURL is the base URL of this service (for redirect URI).
	BaseURL string
	// ClientID is the OAuth client ID.
	ClientID string
	// AuthServerURL is the default OAuth authorization server URL.
	AuthServerURL string
	// JWKSEndpoints maps issuer URLs to JWKS endpoints (used as whitelist).
	JWKSEndpoints map[string]string
}

// TokenExchangeRequest contains the parameters for a token exchange.
type TokenExchangeRequest struct {
	// Code is the authorization code from the OAuth callback.
	Code string
	// CodeVerifier is the PKCE code verifier.
	CodeVerifier string
	// AuthURL is the auth server URL to exchange with (must be in whitelist).
	AuthURL string
	// RedirectURI is the callback URI. Required for MCP, optional for browser flow
	// (falls back to config.BaseURL/oauth/callback if empty).
	RedirectURI string
	// ClientID is the OAuth client ID. Required for MCP, optional for browser flow
	// (falls back to config.ClientID if empty).
	ClientID string
}

// TokenResponse contains the response from a token exchange.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// OAuthService defines the interface for OAuth operations.
type OAuthService interface {
	// ExchangeCodeForToken exchanges an authorization code for a JWT access token.
	ExchangeCodeForToken(ctx context.Context, req *TokenExchangeRequest) (string, error)
	// ValidateAuthURL validates an auth URL against the whitelist.
	// Returns the validated URL or error if not in whitelist.
	ValidateAuthURL(authURL string) (string, error)
}

// HTTPClient interface for testing.
type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// oauthService implements OAuthService.
type oauthService struct {
	config     *OAuthConfig
	httpClient HTTPClient
	logger     *zap.Logger
}

// NewOAuthService creates a new OAuth service.
func NewOAuthService(config *OAuthConfig, logger *zap.Logger) OAuthService {
	return &oauthService{
		config:     config,
		httpClient: &http.Client{},
		logger:     logger,
	}
}

// NewOAuthServiceWithClient creates a new OAuth service with a custom HTTP client (for testing).
func NewOAuthServiceWithClient(config *OAuthConfig, httpClient HTTPClient, logger *zap.Logger) OAuthService {
	return &oauthService{
		config:     config,
		httpClient: httpClient,
		logger:     logger,
	}
}

// ValidateAuthURL validates an auth URL against the JWKS endpoints whitelist.
func (s *oauthService) ValidateAuthURL(authURL string) (string, error) {
	// If empty, use default auth server URL
	if authURL == "" {
		return s.config.AuthServerURL, nil
	}

	// Check if auth URL is in the JWKS endpoints whitelist
	if _, ok := s.config.JWKSEndpoints[authURL]; ok {
		return authURL, nil
	}

	s.logger.Warn("Auth URL not in whitelist",
		zap.String("auth_url", authURL),
		zap.Any("allowed", s.config.JWKSEndpoints))

	return "", ErrInvalidAuthURL
}

// ExchangeCodeForToken exchanges an authorization code for a JWT access token.
func (s *oauthService) ExchangeCodeForToken(ctx context.Context, req *TokenExchangeRequest) (string, error) {
	// Validate auth URL
	authServerURL, err := s.ValidateAuthURL(req.AuthURL)
	if err != nil {
		return "", err
	}

	// Determine redirect_uri: use request value if provided, else construct from config
	redirectURI := req.RedirectURI
	if redirectURI == "" {
		baseURL, err := url.Parse(s.config.BaseURL)
		if err != nil {
			return "", fmt.Errorf("invalid base URL: %w", err)
		}
		parsedURI, err := baseURL.Parse("/oauth/callback")
		if err != nil {
			return "", fmt.Errorf("failed to construct redirect URI: %w", err)
		}
		redirectURI = parsedURI.String()
	}

	// Determine client_id: use request value if provided, else use config
	clientID := req.ClientID
	if clientID == "" {
		clientID = s.config.ClientID
	}

	// Build request body
	reqBody := map[string]string{
		"grant_type":   "authorization_code",
		"code":         req.Code,
		"redirect_uri": redirectURI,
		"client_id":    clientID,
	}

	// Include code_verifier if provided (PKCE)
	if req.CodeVerifier != "" {
		reqBody["code_verifier"] = req.CodeVerifier
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Build token URL
	authURL, err := url.Parse(authServerURL)
	if err != nil {
		return "", fmt.Errorf("invalid auth server URL: %w", err)
	}
	tokenURL, err := authURL.Parse("/token")
	if err != nil {
		return "", fmt.Errorf("failed to construct token URL: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL.String(), bytes.NewBuffer(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Send request
	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		s.logger.Error("Token request failed",
			zap.String("token_url", tokenURL.String()),
			zap.Error(err))
		return "", fmt.Errorf("%w: %v", ErrTokenExchangeFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		s.logger.Error("Token endpoint error",
			zap.Int("status", resp.StatusCode),
			zap.String("body", string(body)))
		return "", fmt.Errorf("%w: status %d", ErrTokenExchangeFailed, resp.StatusCode)
	}

	// Parse response
	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode token response: %w", err)
	}

	return tokenResp.AccessToken, nil
}

// Ensure oauthService implements OAuthService at compile time.
var _ OAuthService = (*oauthService)(nil)
