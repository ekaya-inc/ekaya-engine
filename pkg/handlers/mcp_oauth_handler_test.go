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

// =============================================================================
// Dynamic Client Registration (RFC 7591) Tests
// =============================================================================

func TestMCPOAuthHandler_DCR_Success(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	reqBody := `{"redirect_uris":["http://localhost:8080/callback"],"client_name":"Claude Code"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.DynamicClientRegistration(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	// Verify headers
	if rec.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", rec.Header().Get("Content-Type"))
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("expected Cache-Control 'no-store', got '%s'", rec.Header().Get("Cache-Control"))
	}

	var response DCRResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify client_id is the well-known MCP client
	if response.ClientID != "ekaya-mcp" {
		t.Errorf("expected client_id 'ekaya-mcp', got '%s'", response.ClientID)
	}

	// Verify client_id_issued_at is set
	if response.ClientIDIssuedAt <= 0 {
		t.Errorf("expected client_id_issued_at > 0, got %d", response.ClientIDIssuedAt)
	}

	// Verify defaults are applied
	if response.TokenEndpointAuthMethod != "none" {
		t.Errorf("expected token_endpoint_auth_method 'none', got '%s'", response.TokenEndpointAuthMethod)
	}
	if len(response.GrantTypes) != 1 || response.GrantTypes[0] != "authorization_code" {
		t.Errorf("expected grant_types ['authorization_code'], got %v", response.GrantTypes)
	}
	if len(response.ResponseTypes) != 1 || response.ResponseTypes[0] != "code" {
		t.Errorf("expected response_types ['code'], got %v", response.ResponseTypes)
	}
	if response.Scope != "project:access" {
		t.Errorf("expected scope 'project:access', got '%s'", response.Scope)
	}

	// Verify request values are echoed back
	if len(response.RedirectURIs) != 1 || response.RedirectURIs[0] != "http://localhost:8080/callback" {
		t.Errorf("expected redirect_uris ['http://localhost:8080/callback'], got %v", response.RedirectURIs)
	}
	if response.ClientName != "Claude Code" {
		t.Errorf("expected client_name 'Claude Code', got '%s'", response.ClientName)
	}
}

func TestMCPOAuthHandler_DCR_WithCustomValues(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	reqBody := `{
		"redirect_uris":["http://localhost:8080/callback","http://localhost:9090/callback"],
		"client_name":"Test Client",
		"grant_types":["authorization_code","refresh_token"],
		"response_types":["code"],
		"scope":"project:access admin:read"
	}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.DynamicClientRegistration(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}

	var response DCRResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Verify custom values are preserved
	if len(response.RedirectURIs) != 2 {
		t.Errorf("expected 2 redirect_uris, got %d", len(response.RedirectURIs))
	}
	if len(response.GrantTypes) != 2 || response.GrantTypes[0] != "authorization_code" || response.GrantTypes[1] != "refresh_token" {
		t.Errorf("expected grant_types ['authorization_code', 'refresh_token'], got %v", response.GrantTypes)
	}
	if response.Scope != "project:access admin:read" {
		t.Errorf("expected scope 'project:access admin:read', got '%s'", response.Scope)
	}
}

func TestMCPOAuthHandler_DCR_MissingRedirectURIs(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	reqBody := `{"client_name":"Test Client"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.DynamicClientRegistration(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_redirect_uri" {
		t.Errorf("expected error 'invalid_redirect_uri', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_DCR_EmptyRedirectURIs(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	reqBody := `{"redirect_uris":[],"client_name":"Test Client"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.DynamicClientRegistration(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_redirect_uri" {
		t.Errorf("expected error 'invalid_redirect_uri', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_DCR_InvalidJSON(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.DynamicClientRegistration(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_client_metadata" {
		t.Errorf("expected error 'invalid_client_metadata', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_DCR_AllRegistrationsReturnSameClientID(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	reqBody := `{"redirect_uris":["http://localhost:8080/callback"]}`

	// First registration
	req1 := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req1.Header.Set("Content-Type", "application/json")
	rec1 := httptest.NewRecorder()
	handler.DynamicClientRegistration(rec1, req1)

	var response1 DCRResponse
	if err := json.NewDecoder(rec1.Body).Decode(&response1); err != nil {
		t.Fatalf("failed to decode response1: %v", err)
	}

	// Second registration
	req2 := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	handler.DynamicClientRegistration(rec2, req2)

	var response2 DCRResponse
	if err := json.NewDecoder(rec2.Body).Decode(&response2); err != nil {
		t.Fatalf("failed to decode response2: %v", err)
	}

	// Both should have the same well-known client_id
	if response1.ClientID != "ekaya-mcp" {
		t.Errorf("expected response1 client_id 'ekaya-mcp', got '%s'", response1.ClientID)
	}
	if response2.ClientID != "ekaya-mcp" {
		t.Errorf("expected response2 client_id 'ekaya-mcp', got '%s'", response2.ClientID)
	}
}

func TestMCPOAuthHandler_RegisterRoutes_IncludesDCR(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	reqBody := `{"redirect_uris":["http://localhost:8080/callback"]}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("/mcp/oauth/register: expected status %d, got %d", http.StatusCreated, rec.Code)
	}
}

func TestMCPOAuthHandler_DCR_ValidHTTPSRedirectURI(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	// HTTPS is valid for non-localhost
	reqBody := `{"redirect_uris":["https://example.com/callback"],"client_name":"Test Client"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.DynamicClientRegistration(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rec.Code)
	}
}

func TestMCPOAuthHandler_DCR_InvalidHTTPRedirectURI(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	// HTTP is NOT valid for non-localhost
	reqBody := `{"redirect_uris":["http://example.com/callback"],"client_name":"Test Client"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.DynamicClientRegistration(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for non-HTTPS non-localhost, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_redirect_uri" {
		t.Errorf("expected error 'invalid_redirect_uri', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_DCR_MalformedRedirectURI(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	// Missing scheme and host
	reqBody := `{"redirect_uris":["/callback"],"client_name":"Test Client"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.DynamicClientRegistration(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for malformed URI, got %d", http.StatusBadRequest, rec.Code)
	}

	var response map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["error"] != "invalid_redirect_uri" {
		t.Errorf("expected error 'invalid_redirect_uri', got '%s'", response["error"])
	}
}

func TestMCPOAuthHandler_DCR_LocalhostVariants(t *testing.T) {
	oauthService := &mockOAuthService{token: "test-token"}
	handler := NewMCPOAuthHandler(oauthService, zap.NewNop())

	// All localhost variants should be accepted with HTTP
	testCases := []string{
		"http://localhost:8080/callback",
		"http://127.0.0.1:8080/callback",
		"http://[::1]:8080/callback",
		"https://localhost:8080/callback", // HTTPS localhost is also fine
	}

	for _, uri := range testCases {
		reqBody := `{"redirect_uris":["` + uri + `"],"client_name":"Test Client"}`
		req := httptest.NewRequest(http.MethodPost, "/mcp/oauth/register", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		handler.DynamicClientRegistration(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status %d for localhost variant '%s', got %d", http.StatusCreated, uri, rec.Code)
		}
	}
}
