package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/config"
)

func TestHealthHandler_Health(t *testing.T) {
	cfg := &config.Config{
		Version: "test-version",
		Env:     "test",
	}
	handler := NewHealthHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got '%s'", rec.Body.String())
	}
}

func TestHealthHandler_Ping(t *testing.T) {
	cfg := &config.Config{
		Version: "1.2.3",
		Env:     "test",
	}
	handler := NewHealthHandler(cfg, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	handler.Ping(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response PingResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", response.Status)
	}
	if response.Version != "1.2.3" {
		t.Errorf("expected version '1.2.3', got '%s'", response.Version)
	}
	if response.Service != "ekaya-engine" {
		t.Errorf("expected service 'ekaya-engine', got '%s'", response.Service)
	}
	if response.Environment != "test" {
		t.Errorf("expected environment 'test', got '%s'", response.Environment)
	}
	if response.GoVersion == "" {
		t.Error("expected non-empty go_version")
	}
	if response.Hostname == "" {
		t.Error("expected non-empty hostname")
	}
}

func TestHealthHandler_RegisterRoutes(t *testing.T) {
	cfg := &config.Config{}
	handler := NewHealthHandler(cfg, zap.NewNop())

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test /health is registered
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/health: expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Test /ping is registered
	req = httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("/ping: expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
