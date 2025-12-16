package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

// mockAdapterFactory is a test mock for DatasourceAdapterFactory.
type mockAdapterFactory struct {
	types []datasource.DatasourceAdapterInfo
}

func (m *mockAdapterFactory) NewConnectionTester(_ context.Context, _ string, _ map[string]any) (datasource.ConnectionTester, error) {
	return nil, nil
}

func (m *mockAdapterFactory) NewSchemaDiscoverer(_ context.Context, _ string, _ map[string]any) (datasource.SchemaDiscoverer, error) {
	return nil, nil
}

func (m *mockAdapterFactory) ListTypes() []datasource.DatasourceAdapterInfo {
	if m.types != nil {
		return m.types
	}
	return []datasource.DatasourceAdapterInfo{}
}

func TestConfigHandler_Get_Success(t *testing.T) {
	cfg := &config.Config{
		BaseURL: "http://localhost:3443",
		OAuth: config.OAuthConfig{
			ClientID: "test-client-id",
		},
	}

	handler := NewConfigHandler(cfg, &mockAdapterFactory{}, zap.NewNop())

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

	handler := NewConfigHandler(cfg, &mockAdapterFactory{}, zap.NewNop())

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

func TestConfigHandler_GetDatasourceTypes_Success(t *testing.T) {
	cfg := &config.Config{}
	mockFactory := &mockAdapterFactory{
		types: []datasource.DatasourceAdapterInfo{
			{
				Type:        "postgres",
				DisplayName: "PostgreSQL",
				Description: "Connect to PostgreSQL 12+",
				Icon:        "postgres",
			},
			{
				Type:        "mysql",
				DisplayName: "MySQL",
				Description: "Connect to MySQL 8+",
				Icon:        "mysql",
			},
		},
	}

	handler := NewConfigHandler(cfg, mockFactory, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/config/datasource-types", nil)
	rec := httptest.NewRecorder()

	handler.GetDatasourceTypes(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp []datasource.DatasourceAdapterInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp) != 2 {
		t.Fatalf("expected 2 types, got %d", len(resp))
	}

	if resp[0].Type != "postgres" {
		t.Errorf("expected first type 'postgres', got %q", resp[0].Type)
	}

	if resp[1].DisplayName != "MySQL" {
		t.Errorf("expected second display_name 'MySQL', got %q", resp[1].DisplayName)
	}

	// Check caching header
	cacheControl := rec.Header().Get("Cache-Control")
	if cacheControl != "public, max-age=3600" {
		t.Errorf("expected Cache-Control 'public, max-age=3600', got %q", cacheControl)
	}
}

func TestConfigHandler_GetDatasourceTypes_Empty(t *testing.T) {
	cfg := &config.Config{}
	handler := NewConfigHandler(cfg, &mockAdapterFactory{}, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/api/config/datasource-types", nil)
	rec := httptest.NewRecorder()

	handler.GetDatasourceTypes(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp []datasource.DatasourceAdapterInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp) != 0 {
		t.Errorf("expected 0 types, got %d", len(resp))
	}
}
