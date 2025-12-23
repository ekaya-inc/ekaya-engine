package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

func TestWellKnownHandler_OAuthDiscovery(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
			},
		},
	}
	handler := NewWellKnownHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()

	handler.OAuthDiscovery(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var metadata OAuthServerMetadata
	if err := json.NewDecoder(rec.Body).Decode(&metadata); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if metadata.Issuer != "https://auth.example.com" {
		t.Errorf("expected issuer 'https://auth.example.com', got '%s'", metadata.Issuer)
	}
	if metadata.AuthorizationEndpoint != "https://auth.example.com/authorize" {
		t.Errorf("expected authorization endpoint 'https://auth.example.com/authorize', got '%s'", metadata.AuthorizationEndpoint)
	}
	if metadata.TokenEndpoint != "https://auth.example.com/token" {
		t.Errorf("expected token endpoint 'https://auth.example.com/token', got '%s'", metadata.TokenEndpoint)
	}

	// Verify cache header is set for default response
	if rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Errorf("expected Cache-Control 'public, max-age=3600', got '%s'", rec.Header().Get("Cache-Control"))
	}
}

func TestWellKnownHandler_OAuthDiscovery_WithAuthURL(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com":  "https://auth.example.com/.well-known/jwks.json",
				"https://other.example.com": "https://other.example.com/.well-known/jwks.json",
			},
		},
	}
	handler := NewWellKnownHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server?auth_url=https://other.example.com", nil)
	rec := httptest.NewRecorder()

	handler.OAuthDiscovery(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var metadata OAuthServerMetadata
	if err := json.NewDecoder(rec.Body).Decode(&metadata); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if metadata.Issuer != "https://other.example.com" {
		t.Errorf("expected issuer 'https://other.example.com', got '%s'", metadata.Issuer)
	}

	// Verify cache header is private for dynamic response
	if rec.Header().Get("Cache-Control") != "private, no-cache" {
		t.Errorf("expected Cache-Control 'private, no-cache', got '%s'", rec.Header().Get("Cache-Control"))
	}
}

func TestWellKnownHandler_OAuthDiscovery_InvalidAuthURL(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
			},
		},
	}
	handler := NewWellKnownHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server?auth_url=https://malicious.example.com", nil)
	rec := httptest.NewRecorder()

	handler.OAuthDiscovery(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestWellKnownHandler_ProtectedResource(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
	}
	handler := NewWellKnownHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()

	handler.ProtectedResource(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var metadata ProtectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&metadata); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if metadata.Resource != "http://localhost:3443" {
		t.Errorf("expected resource 'http://localhost:3443', got '%s'", metadata.Resource)
	}
	if len(metadata.AuthorizationServers) != 1 || metadata.AuthorizationServers[0] != "https://auth.example.com" {
		t.Errorf("expected authorization_servers ['https://auth.example.com'], got %v", metadata.AuthorizationServers)
	}
	if len(metadata.BearerMethodsSupported) != 1 || metadata.BearerMethodsSupported[0] != "header" {
		t.Errorf("expected bearer_methods_supported ['header'], got %v", metadata.BearerMethodsSupported)
	}

	// Verify cache header
	if rec.Header().Get("Cache-Control") != "public, max-age=3600" {
		t.Errorf("expected Cache-Control 'public, max-age=3600', got '%s'", rec.Header().Get("Cache-Control"))
	}
}

func TestWellKnownHandler_ProtectedResource_MissingAuthServerURL(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "", // Empty - not configured
	}
	handler := NewWellKnownHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()

	handler.ProtectedResource(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status %d, got %d", http.StatusInternalServerError, rec.Code)
	}
}

func TestWellKnownHandler_RegisterRoutes(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
			},
		},
	}
	handler := NewWellKnownHandler(cfg, zap.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test oauth-authorization-server is registered
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/.well-known/oauth-authorization-server: expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Test oauth-protected-resource is registered
	req = httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/.well-known/oauth-protected-resource: expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
