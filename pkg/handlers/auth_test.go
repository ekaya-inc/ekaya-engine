package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
)

// mockOAuthService is a mock implementation of OAuthService for testing.
type mockOAuthService struct {
	token string
	err   error
}

func (m *mockOAuthService) ExchangeCodeForToken(ctx context.Context, req *services.TokenExchangeRequest) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.token, nil
}

func (m *mockOAuthService) ValidateAuthURL(authURL string) (string, error) {
	return authURL, nil
}

func TestAuthHandler_CompleteOAuth_Success(t *testing.T) {
	// Initialize session store for testing
	auth.InitSessionStore()

	cfg := &config.Config{
		BaseURL:      "http://localhost:3443",
		CookieDomain: "",
	}

	oauthService := &mockOAuthService{token: "test-jwt-token"}
	handler := NewAuthHandler(oauthService, cfg, zap.NewNop())

	reqBody := CompleteOAuthRequest{
		Code:         "auth-code-123",
		State:        "state-456",
		CodeVerifier: "verifier-789",
		AuthURL:      "",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CompleteOAuth(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	// Check response body
	var resp CompleteOAuthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if !resp.Success {
		t.Error("expected success to be true")
	}

	if resp.RedirectURL != "/" {
		t.Errorf("expected redirect_url '/', got %q", resp.RedirectURL)
	}

	// Check that cookie was set
	cookies := rec.Result().Cookies()
	var jwtCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "ekaya_jwt" {
			jwtCookie = c
			break
		}
	}

	if jwtCookie == nil {
		t.Fatal("expected ekaya_jwt cookie to be set")
	}

	if jwtCookie.Value != "test-jwt-token" {
		t.Errorf("expected cookie value 'test-jwt-token', got %q", jwtCookie.Value)
	}

	if !jwtCookie.HttpOnly {
		t.Error("expected cookie to be HttpOnly")
	}
}

func TestAuthHandler_CompleteOAuth_MissingCode(t *testing.T) {
	auth.InitSessionStore()

	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
	}

	handler := NewAuthHandler(&mockOAuthService{}, cfg, zap.NewNop())

	reqBody := CompleteOAuthRequest{
		Code:         "", // Missing
		State:        "state-456",
		CodeVerifier: "verifier-789",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CompleteOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "missing_parameters" {
		t.Errorf("expected error 'missing_parameters', got %q", resp["error"])
	}
}

func TestAuthHandler_CompleteOAuth_MissingState(t *testing.T) {
	auth.InitSessionStore()

	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
	}

	handler := NewAuthHandler(&mockOAuthService{}, cfg, zap.NewNop())

	reqBody := CompleteOAuthRequest{
		Code:         "auth-code-123",
		State:        "", // Missing
		CodeVerifier: "verifier-789",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CompleteOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestAuthHandler_CompleteOAuth_MissingCodeVerifier(t *testing.T) {
	auth.InitSessionStore()

	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
	}

	handler := NewAuthHandler(&mockOAuthService{}, cfg, zap.NewNop())

	reqBody := CompleteOAuthRequest{
		Code:         "auth-code-123",
		State:        "state-456",
		CodeVerifier: "", // Missing
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CompleteOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}
}

func TestAuthHandler_CompleteOAuth_InvalidAuthURL(t *testing.T) {
	auth.InitSessionStore()

	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
	}

	oauthService := &mockOAuthService{err: services.ErrInvalidAuthURL}
	handler := NewAuthHandler(oauthService, cfg, zap.NewNop())

	reqBody := CompleteOAuthRequest{
		Code:         "auth-code-123",
		State:        "state-456",
		CodeVerifier: "verifier-789",
		AuthURL:      "https://malicious.example.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CompleteOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_auth_url" {
		t.Errorf("expected error 'invalid_auth_url', got %q", resp["error"])
	}
}

func TestAuthHandler_CompleteOAuth_TokenExchangeFailed(t *testing.T) {
	auth.InitSessionStore()

	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
	}

	oauthService := &mockOAuthService{err: errors.New("connection refused")}
	handler := NewAuthHandler(oauthService, cfg, zap.NewNop())

	reqBody := CompleteOAuthRequest{
		Code:         "auth-code-123",
		State:        "state-456",
		CodeVerifier: "verifier-789",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CompleteOAuth(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "token_exchange_failed" {
		t.Errorf("expected error 'token_exchange_failed', got %q", resp["error"])
	}
}

func TestAuthHandler_CompleteOAuth_InvalidJSON(t *testing.T) {
	auth.InitSessionStore()

	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
	}

	handler := NewAuthHandler(&mockOAuthService{}, cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader([]byte("not json")))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CompleteOAuth(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["error"] != "invalid_request" {
		t.Errorf("expected error 'invalid_request', got %q", resp["error"])
	}
}

func TestAuthHandler_CookieSettingsForLocalhost(t *testing.T) {
	auth.InitSessionStore()

	cfg := &config.Config{
		BaseURL:      "http://localhost:3443",
		CookieDomain: "",
	}

	oauthService := &mockOAuthService{token: "test-jwt-token"}
	handler := NewAuthHandler(oauthService, cfg, zap.NewNop())

	reqBody := CompleteOAuthRequest{
		Code:         "auth-code-123",
		State:        "state-456",
		CodeVerifier: "verifier-789",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest(http.MethodPost, "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.CompleteOAuth(rec, req)

	cookies := rec.Result().Cookies()
	var jwtCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "ekaya_jwt" {
			jwtCookie = c
			break
		}
	}

	if jwtCookie == nil {
		t.Fatal("expected ekaya_jwt cookie to be set")
	}

	// Localhost should have Secure: false
	if jwtCookie.Secure {
		t.Error("expected Secure to be false for localhost")
	}

	// Localhost should have empty Domain
	if jwtCookie.Domain != "" {
		t.Errorf("expected empty Domain for localhost, got %q", jwtCookie.Domain)
	}
}
