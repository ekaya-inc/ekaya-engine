package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

func TestConfigHandler_Get_Success(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		OAuth: config.OAuthConfig{
			ClientID: "test-client-id",
		},
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
			},
		},
	}

	handler := NewConfigHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.AuthServerURL != "https://auth.example.com" {
		t.Errorf("expected auth_server_url 'https://auth.example.com', got %q", resp.AuthServerURL)
	}

	if resp.OAuthClientID != "test-client-id" {
		t.Errorf("expected oauth_client_id 'test-client-id', got %q", resp.OAuthClientID)
	}

	if resp.BaseURL != "http://localhost:3443" {
		t.Errorf("expected base_url 'http://localhost:3443', got %q", resp.BaseURL)
	}

	// Check caching header for default (no auth_url)
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "public, max-age=300" {
		t.Errorf("expected Cache-Control 'public, max-age=300', got %q", cacheControl)
	}
}

func TestConfigHandler_Get_WithValidAuthURL(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		OAuth: config.OAuthConfig{
			ClientID: "test-client-id",
		},
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com":     "https://auth.example.com/.well-known/jwks.json",
				"https://auth.dev.example.com": "https://auth.dev.example.com/.well-known/jwks.json",
			},
		},
	}

	handler := NewConfigHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/config?auth_url=https://auth.dev.example.com", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Should use the custom auth_url
	if resp.AuthServerURL != "https://auth.dev.example.com" {
		t.Errorf("expected auth_server_url 'https://auth.dev.example.com', got %q", resp.AuthServerURL)
	}

	// Check no-cache header for custom auth_url
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "private, no-cache" {
		t.Errorf("expected Cache-Control 'private, no-cache', got %q", cacheControl)
	}
}

func TestConfigHandler_Get_WithInvalidAuthURL(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		OAuth: config.OAuthConfig{
			ClientID: "test-client-id",
		},
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
			},
		},
	}

	handler := NewConfigHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/config?auth_url=https://malicious.example.com", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

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

func TestConfigHandler_Get_EmptyAuthServerURL(t *testing.T) {
	// Test when no auth server URL is configured
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "",
		OAuth: config.OAuthConfig{
			ClientID: "test-client-id",
		},
	}

	handler := NewConfigHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Should return empty auth_server_url
	if resp.AuthServerURL != "" {
		t.Errorf("expected empty auth_server_url, got %q", resp.AuthServerURL)
	}
}

func TestConfigHandler_DoesNotExposeSensitiveData(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		OAuth: config.OAuthConfig{
			ClientID: "test-client-id",
		},
		Database: config.DatabaseConfig{
			Password: "super-secret-password",
		},
		Redis: config.RedisConfig{
			Password: "redis-secret",
		},
		ProjectCredentialsKey: "encryption-key",
	}

	handler := NewConfigHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	body := rec.Body.String()

	// Verify sensitive data is not in response
	if containsString(body, "super-secret-password") {
		t.Error("response should not contain database password")
	}
	if containsString(body, "redis-secret") {
		t.Error("response should not contain redis password")
	}
	if containsString(body, "encryption-key") {
		t.Error("response should not contain project credentials key")
	}
}

func containsString(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) &&
		(haystack == needle ||
			(len(haystack) > len(needle) &&
				(haystack[:len(needle)] == needle ||
					containsString(haystack[1:], needle))))
}
