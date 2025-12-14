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
		BaseURL: "http://localhost:3443",
		OAuth: config.OAuthConfig{
			ClientID: "test-client-id",
		},
	}

	handler := NewConfigHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/config/auth", nil)
	rec := httptest.NewRecorder()

	handler.Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ConfigResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.OAuthClientID != "test-client-id" {
		t.Errorf("expected oauth_client_id 'test-client-id', got %q", resp.OAuthClientID)
	}

	if resp.BaseURL != "http://localhost:3443" {
		t.Errorf("expected base_url 'http://localhost:3443', got %q", resp.BaseURL)
	}

	// Check caching header
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "public, max-age=300" {
		t.Errorf("expected Cache-Control 'public, max-age=300', got %q", cacheControl)
	}
}

func TestConfigHandler_DoesNotExposeSensitiveData(t *testing.T) {
	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
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

	req := httptest.NewRequest(http.MethodGet, "/api/config/auth", nil)
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
