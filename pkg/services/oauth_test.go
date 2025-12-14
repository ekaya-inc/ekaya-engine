package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"go.uber.org/zap"
)

// mockHTTPClient is a mock implementation of HTTPClient for testing.
type mockHTTPClient struct {
	response    *http.Response
	err         error
	capturedURL string // Captures the last request URL
}

func (m *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	m.capturedURL = req.URL.String()
	if m.err != nil {
		return nil, m.err
	}
	return m.response, nil
}

func TestOAuthService_ValidateAuthURL_Default(t *testing.T) {
	config := &OAuthConfig{
		AuthServerURL: "https://auth.example.com",
		JWKSEndpoints: map[string]string{
			"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
		},
	}

	service := NewOAuthService(config, zap.NewNop())

	// Empty auth URL should return default
	result, err := service.ValidateAuthURL("")
	if err != nil {
		t.Fatalf("ValidateAuthURL failed: %v", err)
	}

	if result != "https://auth.example.com" {
		t.Errorf("expected default auth server URL, got %q", result)
	}
}

func TestOAuthService_ValidateAuthURL_Whitelisted(t *testing.T) {
	config := &OAuthConfig{
		AuthServerURL: "https://auth.example.com",
		JWKSEndpoints: map[string]string{
			"https://auth.example.com":     "https://auth.example.com/.well-known/jwks.json",
			"https://auth.dev.example.com": "https://auth.dev.example.com/.well-known/jwks.json",
		},
	}

	service := NewOAuthService(config, zap.NewNop())

	result, err := service.ValidateAuthURL("https://auth.dev.example.com")
	if err != nil {
		t.Fatalf("ValidateAuthURL failed: %v", err)
	}

	if result != "https://auth.dev.example.com" {
		t.Errorf("expected whitelisted URL, got %q", result)
	}
}

func TestOAuthService_ValidateAuthURL_NotWhitelisted(t *testing.T) {
	config := &OAuthConfig{
		AuthServerURL: "https://auth.example.com",
		JWKSEndpoints: map[string]string{
			"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
		},
	}

	service := NewOAuthService(config, zap.NewNop())

	_, err := service.ValidateAuthURL("https://malicious.example.com")
	if err == nil {
		t.Fatal("expected error for non-whitelisted auth URL")
	}

	if !errors.Is(err, ErrInvalidAuthURL) {
		t.Errorf("expected ErrInvalidAuthURL, got %v", err)
	}
}

func TestOAuthService_ExchangeCodeForToken_Success(t *testing.T) {
	config := &OAuthConfig{
		BaseURL:       "https://app.example.com",
		ClientID:      "test-client",
		AuthServerURL: "https://auth.example.com",
		JWKSEndpoints: map[string]string{
			"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
		},
	}

	// Create mock response
	tokenResp := TokenResponse{
		AccessToken: "jwt-token-here",
		TokenType:   "Bearer",
		ExpiresIn:   86400,
	}
	respBody, _ := json.Marshal(tokenResp)

	mockClient := &mockHTTPClient{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		},
	}

	service := NewOAuthServiceWithClient(config, mockClient, zap.NewNop())

	token, err := service.ExchangeCodeForToken(context.Background(), &TokenExchangeRequest{
		Code:         "auth-code-123",
		CodeVerifier: "verifier-456",
		AuthURL:      "",
	})

	if err != nil {
		t.Fatalf("ExchangeCodeForToken failed: %v", err)
	}

	if token != "jwt-token-here" {
		t.Errorf("expected token 'jwt-token-here', got %q", token)
	}
}

func TestOAuthService_ExchangeCodeForToken_InvalidAuthURL(t *testing.T) {
	config := &OAuthConfig{
		BaseURL:       "https://app.example.com",
		ClientID:      "test-client",
		AuthServerURL: "https://auth.example.com",
		JWKSEndpoints: map[string]string{
			"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
		},
	}

	service := NewOAuthService(config, zap.NewNop())

	_, err := service.ExchangeCodeForToken(context.Background(), &TokenExchangeRequest{
		Code:         "auth-code-123",
		CodeVerifier: "verifier-456",
		AuthURL:      "https://malicious.example.com",
	})

	if err == nil {
		t.Fatal("expected error for invalid auth URL")
	}

	if !errors.Is(err, ErrInvalidAuthURL) {
		t.Errorf("expected ErrInvalidAuthURL, got %v", err)
	}
}

func TestOAuthService_ExchangeCodeForToken_HTTPError(t *testing.T) {
	config := &OAuthConfig{
		BaseURL:       "https://app.example.com",
		ClientID:      "test-client",
		AuthServerURL: "https://auth.example.com",
		JWKSEndpoints: map[string]string{
			"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
		},
	}

	mockClient := &mockHTTPClient{
		err: errors.New("connection refused"),
	}

	service := NewOAuthServiceWithClient(config, mockClient, zap.NewNop())

	_, err := service.ExchangeCodeForToken(context.Background(), &TokenExchangeRequest{
		Code:         "auth-code-123",
		CodeVerifier: "verifier-456",
		AuthURL:      "",
	})

	if err == nil {
		t.Fatal("expected error for HTTP failure")
	}

	if !errors.Is(err, ErrTokenExchangeFailed) {
		t.Errorf("expected ErrTokenExchangeFailed, got %v", err)
	}
}

func TestOAuthService_ExchangeCodeForToken_TokenEndpointError(t *testing.T) {
	config := &OAuthConfig{
		BaseURL:       "https://app.example.com",
		ClientID:      "test-client",
		AuthServerURL: "https://auth.example.com",
		JWKSEndpoints: map[string]string{
			"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
		},
	}

	// Create error response
	errorResp := map[string]string{
		"error":             "invalid_grant",
		"error_description": "Authorization code expired",
	}
	respBody, _ := json.Marshal(errorResp)

	mockClient := &mockHTTPClient{
		response: &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		},
	}

	service := NewOAuthServiceWithClient(config, mockClient, zap.NewNop())

	_, err := service.ExchangeCodeForToken(context.Background(), &TokenExchangeRequest{
		Code:         "expired-code",
		CodeVerifier: "verifier-456",
		AuthURL:      "",
	})

	if err == nil {
		t.Fatal("expected error for token endpoint error")
	}

	if !errors.Is(err, ErrTokenExchangeFailed) {
		t.Errorf("expected ErrTokenExchangeFailed, got %v", err)
	}
}

func TestOAuthService_ExchangeCodeForToken_WithCustomAuthURL(t *testing.T) {
	config := &OAuthConfig{
		BaseURL:       "https://app.example.com",
		ClientID:      "test-client",
		AuthServerURL: "https://auth.example.com",
		JWKSEndpoints: map[string]string{
			"https://auth.example.com":     "https://auth.example.com/.well-known/jwks.json",
			"https://auth.dev.example.com": "https://auth.dev.example.com/.well-known/jwks.json",
		},
	}

	// Create mock response
	tokenResp := TokenResponse{
		AccessToken: "dev-jwt-token",
		TokenType:   "Bearer",
		ExpiresIn:   86400,
	}
	respBody, _ := json.Marshal(tokenResp)

	mockClient := &mockHTTPClient{
		response: &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewReader(respBody)),
		},
	}

	service := NewOAuthServiceWithClient(config, mockClient, zap.NewNop())

	token, err := service.ExchangeCodeForToken(context.Background(), &TokenExchangeRequest{
		Code:         "auth-code-123",
		CodeVerifier: "verifier-456",
		AuthURL:      "https://auth.dev.example.com",
	})

	if err != nil {
		t.Fatalf("ExchangeCodeForToken failed: %v", err)
	}

	if token != "dev-jwt-token" {
		t.Errorf("expected token 'dev-jwt-token', got %q", token)
	}

	// Verify the correct auth server was used
	if mockClient.capturedURL != "https://auth.dev.example.com/token" {
		t.Errorf("expected URL 'https://auth.dev.example.com/token', got %q", mockClient.capturedURL)
	}
}

func TestOAuthService_Interface(t *testing.T) {
	config := &OAuthConfig{}
	service := NewOAuthService(config, zap.NewNop())
	var _ OAuthService = service
}
