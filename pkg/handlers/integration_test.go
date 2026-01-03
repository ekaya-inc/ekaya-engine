package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/auth"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/services"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

// mockTokenServer creates a mock OAuth token server for testing.
func mockTokenServer(t *testing.T, expectedCode string, shouldFail bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/token" {
			t.Errorf("expected /token, got %s", r.URL.Path)
		}

		var reqBody map[string]string
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Errorf("failed to decode request: %v", err)
		}

		if reqBody["code"] != expectedCode {
			t.Errorf("expected code %q, got %q", expectedCode, reqBody["code"])
		}

		if shouldFail {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": testhelpers.GenerateTestJWT("user-123", "test-project", "user@example.com"),
			"token_type":   "Bearer",
			"expires_in":   86400,
		})
	}))
}

func TestCompleteOAuthFlow_Success(t *testing.T) {
	auth.InitSessionStore()
	logger := zap.NewNop()

	mockServer := mockTokenServer(t, "test-code-123", false)
	defer mockServer.Close()

	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: mockServer.URL,
		CookieDomain:  "",
		OAuth:         config.OAuthConfig{ClientID: "test-client"},
		Auth: config.AuthConfig{
			EnableVerification: false,
			JWKSEndpoints:      map[string]string{mockServer.URL: mockServer.URL + "/.well-known/jwks.json"},
		},
	}

	oauthService := services.NewOAuthService(&services.OAuthConfig{
		BaseURL:       cfg.BaseURL,
		ClientID:      cfg.OAuth.ClientID,
		AuthServerURL: cfg.AuthServerURL,
		JWKSEndpoints: cfg.Auth.JWKSEndpoints,
	}, logger)

	authHandler := NewAuthHandler(oauthService, &mockProjectService{}, cfg, logger)

	// Step 1: Set up session with state (simulating OAuth initiation)
	req1 := httptest.NewRequest("POST", "/api/auth/complete-oauth", nil)
	w1 := httptest.NewRecorder()
	session, _ := auth.Store.Get(req1, auth.SessionName)
	session.Values[auth.SessionKeyState] = "test-state-abc"
	session.Values[auth.SessionKeyOriginalURL] = "/projects/my-project"
	_ = session.Save(req1, w1)

	// Step 2: Complete OAuth with code (simulating callback)
	reqBody := map[string]string{
		"code":          "test-code-123",
		"state":         "test-state-abc",
		"code_verifier": "test-verifier",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req2 := httptest.NewRequest("POST", "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req2.Header.Set("Content-Type", "application/json")
	// Forward session cookie from first request
	for _, cookie := range w1.Result().Cookies() {
		req2.AddCookie(cookie)
	}

	w2 := httptest.NewRecorder()
	authHandler.CompleteOAuth(w2, req2)

	// Verify success
	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}

	// Verify JWT cookie was set
	var jwtCookie *http.Cookie
	for _, c := range w2.Result().Cookies() {
		if c.Name == "ekaya_jwt" {
			jwtCookie = c
			break
		}
	}
	if jwtCookie == nil {
		t.Fatal("expected ekaya_jwt cookie to be set")
	}
	if !jwtCookie.HttpOnly {
		t.Error("expected HttpOnly cookie")
	}

	// Verify response body
	var resp CompleteOAuthResponse
	if err := json.NewDecoder(w2.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Error("expected success: true")
	}
	if resp.RedirectURL != "/projects/my-project" {
		t.Errorf("expected redirect to '/projects/my-project', got %q", resp.RedirectURL)
	}
}

func TestCompleteOAuthFlow_InvalidAuthURL(t *testing.T) {
	auth.InitSessionStore()
	logger := zap.NewNop()

	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		OAuth:         config.OAuthConfig{ClientID: "test-client"},
		Auth: config.AuthConfig{
			EnableVerification: false,
			JWKSEndpoints:      map[string]string{"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json"},
		},
	}

	oauthService := services.NewOAuthService(&services.OAuthConfig{
		BaseURL:       cfg.BaseURL,
		ClientID:      cfg.OAuth.ClientID,
		AuthServerURL: cfg.AuthServerURL,
		JWKSEndpoints: cfg.Auth.JWKSEndpoints,
	}, logger)

	authHandler := NewAuthHandler(oauthService, &mockProjectService{}, cfg, logger)

	// Try with malicious auth_url NOT in whitelist
	reqBody := map[string]string{
		"code":          "test-code",
		"state":         "test-state",
		"code_verifier": "test-verifier",
		"auth_url":      "https://malicious.example.com",
	}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/api/auth/complete-oauth", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	authHandler.CompleteOAuth(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "invalid_auth_url" {
		t.Errorf("expected error 'invalid_auth_url', got %q", resp["error"])
	}
}

func TestProtectedEndpoint_WithCookieAuth(t *testing.T) {
	auth.InitSessionStore()
	logger := zap.NewNop()

	// Create JWKS client with verification disabled
	jwksClient, _ := auth.NewJWKSClient(&auth.JWKSConfig{EnableVerification: false})
	authService := auth.NewAuthService(jwksClient, logger)
	authMiddleware := auth.NewMiddleware(authService, logger)

	// Create projects handler with mock service
	projectService := &mockProjectService{}
	projectsHandler := NewProjectsHandler(projectService, logger)

	// Set up mux with protected route - use a no-op tenant middleware for testing
	mux := http.NewServeMux()
	noopTenantMiddleware := func(next http.HandlerFunc) http.HandlerFunc { return next }
	mux.HandleFunc("GET /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(noopTenantMiddleware(projectsHandler.Get)))

	// Create request with JWT cookie
	projectID := "550e8400-e29b-41d4-a716-446655440000"
	token := testhelpers.GenerateTestJWT("user-123", projectID, "user@example.com")

	req := httptest.NewRequest("GET", "/api/projects/"+projectID, nil)
	req.AddCookie(&http.Cookie{
		Name:  "ekaya_jwt",
		Value: token,
	})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ProjectResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.PID != projectID {
		t.Errorf("expected pid %q, got %q", projectID, resp.PID)
	}
}

func TestProtectedEndpoint_WithBearerAuth(t *testing.T) {
	auth.InitSessionStore()
	logger := zap.NewNop()

	jwksClient, _ := auth.NewJWKSClient(&auth.JWKSConfig{EnableVerification: false})
	authService := auth.NewAuthService(jwksClient, logger)
	authMiddleware := auth.NewMiddleware(authService, logger)

	projectService := &mockProjectService{}
	projectsHandler := NewProjectsHandler(projectService, logger)

	mux := http.NewServeMux()
	noopTenantMiddleware := func(next http.HandlerFunc) http.HandlerFunc { return next }
	mux.HandleFunc("GET /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(noopTenantMiddleware(projectsHandler.Get)))

	projectID := "550e8400-e29b-41d4-a716-446655440000"
	token := testhelpers.GenerateTestJWTWithBearer("user-456", projectID, "api@example.com")

	req := httptest.NewRequest("GET", "/api/projects/"+projectID, nil)
	req.Header.Set("Authorization", token)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ProjectResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.PID != projectID {
		t.Errorf("expected pid %q, got %q", projectID, resp.PID)
	}
}

func TestProtectedEndpoint_CookiePreferredOverHeader(t *testing.T) {
	auth.InitSessionStore()
	logger := zap.NewNop()

	jwksClient, _ := auth.NewJWKSClient(&auth.JWKSConfig{EnableVerification: false})
	authService := auth.NewAuthService(jwksClient, logger)
	authMiddleware := auth.NewMiddleware(authService, logger)

	projectService := &mockProjectService{}
	projectsHandler := NewProjectsHandler(projectService, logger)

	mux := http.NewServeMux()
	noopTenantMiddleware := func(next http.HandlerFunc) http.HandlerFunc { return next }
	mux.HandleFunc("GET /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(noopTenantMiddleware(projectsHandler.Get)))

	projectID := "550e8400-e29b-41d4-a716-446655440000"
	cookieToken := testhelpers.GenerateTestJWT("cookie-user", projectID, "cookie@example.com")
	headerToken := testhelpers.GenerateTestJWTWithBearer("header-user", projectID, "header@example.com")

	req := httptest.NewRequest("GET", "/api/projects/"+projectID, nil)
	req.AddCookie(&http.Cookie{Name: "ekaya_jwt", Value: cookieToken})
	req.Header.Set("Authorization", headerToken)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Response doesn't contain user info anymore, just verify success
	var resp ProjectResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.Status != "success" {
		t.Errorf("expected success status, got %q", resp.Status)
	}
}

func TestProtectedEndpoint_ProjectIDMismatch(t *testing.T) {
	auth.InitSessionStore()
	logger := zap.NewNop()

	jwksClient, _ := auth.NewJWKSClient(&auth.JWKSConfig{EnableVerification: false})
	authService := auth.NewAuthService(jwksClient, logger)
	authMiddleware := auth.NewMiddleware(authService, logger)

	projectService := &mockProjectService{}
	projectsHandler := NewProjectsHandler(projectService, logger)

	mux := http.NewServeMux()
	noopTenantMiddleware := func(next http.HandlerFunc) http.HandlerFunc { return next }
	mux.HandleFunc("GET /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(noopTenantMiddleware(projectsHandler.Get)))

	// Token has project A, URL has project B
	tokenProjectID := "550e8400-e29b-41d4-a716-446655440000"
	urlProjectID := "660e8400-e29b-41d4-a716-446655440000"
	token := testhelpers.GenerateTestJWT("user-123", tokenProjectID, "user@example.com")

	req := httptest.NewRequest("GET", "/api/projects/"+urlProjectID, nil)
	req.AddCookie(&http.Cookie{Name: "ekaya_jwt", Value: token})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden, got %d", w.Code)
	}
}

func TestProtectedEndpoint_NoAuth(t *testing.T) {
	auth.InitSessionStore()
	logger := zap.NewNop()

	jwksClient, _ := auth.NewJWKSClient(&auth.JWKSConfig{EnableVerification: false})
	authService := auth.NewAuthService(jwksClient, logger)
	authMiddleware := auth.NewMiddleware(authService, logger)

	projectService := &mockProjectService{}
	projectsHandler := NewProjectsHandler(projectService, logger)

	mux := http.NewServeMux()
	noopTenantMiddleware := func(next http.HandlerFunc) http.HandlerFunc { return next }
	mux.HandleFunc("GET /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(noopTenantMiddleware(projectsHandler.Get)))

	req := httptest.NewRequest("GET", "/api/projects/550e8400-e29b-41d4-a716-446655440000", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 Unauthorized, got %d", w.Code)
	}
}

func TestProtectedEndpoint_MissingProjectID(t *testing.T) {
	auth.InitSessionStore()
	logger := zap.NewNop()

	jwksClient, _ := auth.NewJWKSClient(&auth.JWKSConfig{EnableVerification: false})
	authService := auth.NewAuthService(jwksClient, logger)
	authMiddleware := auth.NewMiddleware(authService, logger)

	projectService := &mockProjectService{}
	projectsHandler := NewProjectsHandler(projectService, logger)

	mux := http.NewServeMux()
	noopTenantMiddleware := func(next http.HandlerFunc) http.HandlerFunc { return next }
	mux.HandleFunc("GET /api/projects/{pid}",
		authMiddleware.RequireAuthWithPathValidation("pid")(noopTenantMiddleware(projectsHandler.Get)))

	// Token without project ID
	token := testhelpers.GenerateTestJWT("user-123", "", "user@example.com")

	req := httptest.NewRequest("GET", "/api/projects/550e8400-e29b-41d4-a716-446655440000", nil)
	req.AddCookie(&http.Cookie{Name: "ekaya_jwt", Value: token})

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for missing project ID in token, got %d", w.Code)
	}
}
