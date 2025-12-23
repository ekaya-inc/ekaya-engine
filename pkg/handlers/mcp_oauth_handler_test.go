package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

func TestMCPOAuthHandler_TokenExchange_Success(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-access-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "auth-code-123")
	form.Set("code_verifier", "verifier-456")
	form.Set("redirect_uri", "http://127.0.0.1:3000/callback")
	form.Set("client_id", "ekaya-mcp")

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.TokenExchange(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response MCPTokenResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.AccessToken != "test-access-token" {
		t.Errorf("expected access_token 'test-access-token', got '%s'", response.AccessToken)
	}
	if response.TokenType != "Bearer" {
		t.Errorf("expected token_type 'Bearer', got '%s'", response.TokenType)
	}
	if response.ExpiresIn != 86400 {
		t.Errorf("expected expires_in 86400, got %d", response.ExpiresIn)
	}
}

func TestMCPOAuthHandler_TokenExchange_MissingCode(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-access-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", "verifier-456")
	form.Set("redirect_uri", "http://127.0.0.1:3000/callback")
	form.Set("client_id", "ekaya-mcp")

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.TokenExchange(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_request" {
		t.Errorf("expected error 'invalid_request', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_TokenExchange_MissingCodeVerifier(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-access-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "auth-code-123")
	form.Set("redirect_uri", "http://127.0.0.1:3000/callback")
	form.Set("client_id", "ekaya-mcp")

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.TokenExchange(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_request" {
		t.Errorf("expected error 'invalid_request', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_TokenExchange_MissingRedirectURI(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-access-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "auth-code-123")
	form.Set("code_verifier", "verifier-456")
	form.Set("client_id", "ekaya-mcp")
	// redirect_uri intentionally omitted

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.TokenExchange(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_request" {
		t.Errorf("expected error 'invalid_request', got '%s'", response["error"])
	}
	if !strings.Contains(response["error_description"], "redirect_uri") {
		t.Errorf("expected error_description to mention redirect_uri, got '%s'", response["error_description"])
	}
}

func TestMCPOAuthHandler_TokenExchange_MissingClientID(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-access-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "auth-code-123")
	form.Set("code_verifier", "verifier-456")
	form.Set("redirect_uri", "http://127.0.0.1:3000/callback")
	// client_id intentionally omitted

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.TokenExchange(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_request" {
		t.Errorf("expected error 'invalid_request', got '%s'", response["error"])
	}
	if !strings.Contains(response["error_description"], "client_id") {
		t.Errorf("expected error_description to mention client_id, got '%s'", response["error_description"])
	}
}

func TestMCPOAuthHandler_TokenExchange_UnsupportedGrantType(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-access-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("code", "auth-code-123")
	form.Set("code_verifier", "verifier-456")
	form.Set("redirect_uri", "http://127.0.0.1:3000/callback")
	form.Set("client_id", "ekaya-mcp")

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.TokenExchange(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "unsupported_grant_type" {
		t.Errorf("expected error 'unsupported_grant_type', got '%s'", response["error"])
	}
}

// mockOAuthServiceWithAuthURLError returns ErrInvalidAuthURL for specific URLs.
type mockOAuthServiceWithAuthURLError struct {
	invalidURL string
}

func (m *mockOAuthServiceWithAuthURLError) ExchangeCodeForToken(ctx context.Context, req *services.TokenExchangeRequest) (string, error) {
	if req.AuthURL == m.invalidURL {
		return "", services.ErrInvalidAuthURL
	}
	return "test-token", nil
}

func (m *mockOAuthServiceWithAuthURLError) ValidateAuthURL(authURL string) (string, error) {
	if authURL == m.invalidURL {
		return "", services.ErrInvalidAuthURL
	}
	return authURL, nil
}

func TestMCPOAuthHandler_TokenExchange_InvalidAuthURL(t *testing.T) {
	oauthService := &mockOAuthServiceWithAuthURLError{invalidURL: "https://malicious.example.com"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "auth-code-123")
	form.Set("code_verifier", "verifier-456")
	form.Set("redirect_uri", "http://127.0.0.1:3000/callback")
	form.Set("client_id", "ekaya-mcp")
	form.Set("auth_url", "https://malicious.example.com")

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.TokenExchange(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_request" {
		t.Errorf("expected error 'invalid_request', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_TokenExchange_ServiceError(t *testing.T) {
	oauthService := &mockOAuthService{err: services.ErrTokenExchangeFailed}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "auth-code-123")
	form.Set("code_verifier", "verifier-456")
	form.Set("redirect_uri", "http://127.0.0.1:3000/callback")
	form.Set("client_id", "ekaya-mcp")

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	handler.TokenExchange(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "server_error" {
		t.Errorf("expected error 'server_error', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_RegisterRoutes(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", "test-code")
	form.Set("code_verifier", "test-verifier")
	form.Set("redirect_uri", "http://127.0.0.1:3000/callback")
	form.Set("client_id", "ekaya-mcp")

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("/mcp/oauth/token: expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
