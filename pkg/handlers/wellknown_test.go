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
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server", nil)
	req.Host = "localhost:3443"
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
	// TokenEndpoint should point to local MCP endpoint
	if metadata.TokenEndpoint != "http://localhost:3443/mcp/oauth/token" {
		t.Errorf("expected token endpoint 'http://localhost:3443/mcp/oauth/token', got '%s'", metadata.TokenEndpoint)
	}
	// RegistrationEndpoint should be present for DCR (RFC 7591)
	if metadata.RegistrationEndpoint != "http://localhost:3443/mcp/oauth/register" {
		t.Errorf("expected registration endpoint 'http://localhost:3443/mcp/oauth/register', got '%s'", metadata.RegistrationEndpoint)
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
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server?auth_url=https://other.example.com", nil)
	req.Host = "localhost:3443"
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
	// TokenEndpoint still points to local MCP endpoint
	if metadata.TokenEndpoint != "http://localhost:3443/mcp/oauth/token" {
		t.Errorf("expected token endpoint 'http://localhost:3443/mcp/oauth/token', got '%s'", metadata.TokenEndpoint)
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
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

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
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	req.Host = "localhost:3443"
	rec := httptest.NewRecorder()

	handler.ProtectedResource(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var metadata ProtectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&metadata); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Resource should use the request host
	if metadata.Resource != "http://localhost:3443" {
		t.Errorf("expected resource 'http://localhost:3443', got '%s'", metadata.Resource)
	}
	// AuthorizationServers should be the base URL
	// (per RFC 9728, client appends /.well-known/oauth-authorization-server to discover metadata)
	expectedAuthServer := "http://localhost:3443"
	if len(metadata.AuthorizationServers) != 1 || metadata.AuthorizationServers[0] != expectedAuthServer {
		t.Errorf("expected authorization_servers ['%s'], got %v", expectedAuthServer, metadata.AuthorizationServers)
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
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

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
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

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

	// Test oauth-protected-resource wildcard route is registered (uses valid UUID)
	req = httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp/6089f231-1ccb-4ab8-bba1-7e7a03893939", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/.well-known/oauth-protected-resource/mcp/{id}: expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestWellKnownHandler_OAuthDiscovery_WithProjectID(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
			},
		},
	}
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

	projectID := "6089f231-1ccb-4ab8-bba1-7e7a03893939"
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server?project_id="+projectID, nil)
	req.Host = "localhost:3443"
	rec := httptest.NewRecorder()

	handler.OAuthDiscovery(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var metadata OAuthServerMetadata
	if err := json.NewDecoder(rec.Body).Decode(&metadata); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Authorization endpoint should include project_id
	expectedAuthEndpoint := "https://auth.example.com/authorize?project_id=" + projectID
	if metadata.AuthorizationEndpoint != expectedAuthEndpoint {
		t.Errorf("expected authorization_endpoint '%s', got '%s'", expectedAuthEndpoint, metadata.AuthorizationEndpoint)
	}

	// Verify cache header is private for dynamic response
	if rec.Header().Get("Cache-Control") != "private, no-cache" {
		t.Errorf("expected Cache-Control 'private, no-cache', got '%s'", rec.Header().Get("Cache-Control"))
	}
}

func TestWellKnownHandler_ProtectedResource_WithMCPProjectPath(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
	}
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	projectID := "6089f231-1ccb-4ab8-bba1-7e7a03893939"
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp/"+projectID, nil)
	req.Host = "localhost:3443"
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var metadata ProtectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&metadata); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Resource should include the full path
	expectedResource := "http://localhost:3443/mcp/" + projectID
	if metadata.Resource != expectedResource {
		t.Errorf("expected resource '%s', got '%s'", expectedResource, metadata.Resource)
	}

	// AuthorizationServers should be the base URL with the resource path
	// (per RFC 9728, client appends /.well-known/oauth-authorization-server to discover metadata)
	expectedAuthServer := "http://localhost:3443/mcp/" + projectID
	if len(metadata.AuthorizationServers) != 1 || metadata.AuthorizationServers[0] != expectedAuthServer {
		t.Errorf("expected authorization_servers ['%s'], got %v", expectedAuthServer, metadata.AuthorizationServers)
	}

	// Verify cache header is private for path-specific response
	if rec.Header().Get("Cache-Control") != "private, no-cache" {
		t.Errorf("expected Cache-Control 'private, no-cache', got '%s'", rec.Header().Get("Cache-Control"))
	}
}

func TestWellKnownHandler_OAuthDiscovery_WithPathBasedProjectID(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
			},
		},
	}
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	projectID := "6089f231-1ccb-4ab8-bba1-7e7a03893939"
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server/mcp/"+projectID, nil)
	req.Host = "localhost:3443"
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var metadata OAuthServerMetadata
	if err := json.NewDecoder(rec.Body).Decode(&metadata); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Authorization endpoint should include project_id extracted from path
	expectedAuthEndpoint := "https://auth.example.com/authorize?project_id=" + projectID
	if metadata.AuthorizationEndpoint != expectedAuthEndpoint {
		t.Errorf("expected authorization_endpoint '%s', got '%s'", expectedAuthEndpoint, metadata.AuthorizationEndpoint)
	}

	// Verify cache header is private for path-based response
	if rec.Header().Get("Cache-Control") != "private, no-cache" {
		t.Errorf("expected Cache-Control 'private, no-cache', got '%s'", rec.Header().Get("Cache-Control"))
	}
}

func TestWellKnownHandler_OAuthDiscovery_InvalidPathProjectID(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
		Auth: config.AuthConfig{
			JWKSEndpoints: map[string]string{
				"https://auth.example.com": "https://auth.example.com/.well-known/jwks.json",
			},
		},
	}
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test with invalid project_id (not a UUID)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-authorization-server/mcp/invalid-project-id", nil)
	req.Host = "localhost:3443"
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid project_id, got %d", http.StatusBadRequest, rec.Code)
	}
}

func TestWellKnownHandler_ProtectedResource_InvalidProjectID(t *testing.T) {
	cfg := &config.Config{
		BaseURL:       "http://localhost:3443",
		AuthServerURL: "https://auth.example.com",
	}
	handler := NewWellKnownHandler(cfg, nil, zap.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test with invalid project_id (not a UUID)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp/invalid-project-id", nil)
	req.Host = "localhost:3443"
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected status %d for invalid project_id, got %d", http.StatusBadRequest, rec.Code)
	}
}
