package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/ekaya-inc/ekaya-engine/pkg/adapters/datasource"
	"github.com/ekaya-inc/ekaya-engine/pkg/config"
	"github.com/ekaya-inc/ekaya-engine/pkg/testhelpers"
)

func TestHealthHandler_Health_WithoutConnManager(t *testing.T) {
	cfg := &config.Config{
		Version: "test-version",
		Env:     "test",
	}
	handler := NewHealthHandler(cfg, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", response.Status)
	}
	if response.Connections != nil {
		t.Error("expected nil connections when conn manager not provided")
	}
}

func TestHealthHandler_Health_WithConnManager(t *testing.T) {
	cfg := &config.Config{
		Version: "test-version",
		Env:     "test",
	}

	// Create connection manager
	connManagerCfg := datasource.ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          10,
		PoolMinConns:          1,
	}
	connManager := datasource.NewConnectionManager(connManagerCfg, zap.NewNop())
	defer connManager.Close()

	// Create a pool to get some stats
	db := testhelpers.GetTestDB(t)
	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"
	datasourceID := uuid.New()
	connString := db.ConnStr

	pool, err := connManager.GetOrCreatePool(ctx, projectID, userID, datasourceID, connString)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	handler := NewHealthHandler(cfg, connManager, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	handler.Health(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response HealthResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", response.Status)
	}
	if response.Connections == nil {
		t.Fatal("expected non-nil connections when conn manager provided")
	}
	if response.Connections.TotalConnections != 1 {
		t.Errorf("expected 1 total connection, got %d", response.Connections.TotalConnections)
	}
	if response.Connections.MaxConnectionsPerUser != 10 {
		t.Errorf("expected max 10 connections per user, got %d", response.Connections.MaxConnectionsPerUser)
	}
	if response.Connections.TTLMinutes != 5 {
		t.Errorf("expected TTL 5 minutes, got %d", response.Connections.TTLMinutes)
	}
}

func TestHealthHandler_Ping(t *testing.T) {
	cfg := &config.Config{
		Version: "1.2.3",
	}
	handler := NewHealthHandler(cfg, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()

	handler.Ping(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	assertPingCORSHeaders(t, rec)

	var response map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	expected := map[string]any{
		"status":  "ok",
		"version": "1.2.3",
		"service": "ekaya-engine",
	}

	if len(response) != len(expected) {
		t.Fatalf("expected %d ping fields, got %d: %#v", len(expected), len(response), response)
	}

	for key, want := range expected {
		if got := response[key]; got != want {
			t.Errorf("expected %s %q, got %#v", key, want, got)
		}
	}

	for _, field := range []string{"hostname", "go_version", "environment"} {
		if _, ok := response[field]; ok {
			t.Errorf("did not expect %q in ping response", field)
		}
	}
}

func TestHealthHandler_PingOptions(t *testing.T) {
	cfg := &config.Config{
		Version: "1.2.3",
	}
	handler := NewHealthHandler(cfg, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodOptions, "/ping", nil)
	rec := httptest.NewRecorder()

	handler.Ping(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected status %d, got %d", http.StatusNoContent, rec.Code)
	}

	assertPingCORSHeaders(t, rec)

	if rec.Body.Len() != 0 {
		t.Errorf("expected empty response body for OPTIONS /ping, got %q", rec.Body.String())
	}
}

func TestHealthHandler_Metrics_WithoutConnManager(t *testing.T) {
	cfg := &config.Config{
		Version: "test-version",
		Env:     "test",
	}
	handler := NewHealthHandler(cfg, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.Metrics(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestHealthHandler_Metrics_WithConnManager(t *testing.T) {
	cfg := &config.Config{
		Version: "test-version",
		Env:     "test",
	}

	// Create connection manager
	connManagerCfg := datasource.ConnectionManagerConfig{
		TTLMinutes:            5,
		MaxConnectionsPerUser: 10,
		PoolMaxConns:          10,
		PoolMinConns:          1,
	}
	connManager := datasource.NewConnectionManager(connManagerCfg, zap.NewNop())
	defer connManager.Close()

	// Create a pool to get some stats
	db := testhelpers.GetTestDB(t)
	ctx := context.Background()
	projectID := uuid.New()
	userID := "test-user"
	datasourceID := uuid.New()
	connString := db.ConnStr

	pool, err := connManager.GetOrCreatePool(ctx, projectID, userID, datasourceID, connString)
	if err != nil {
		t.Fatalf("failed to create pool: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool")
	}

	handler := NewHealthHandler(cfg, connManager, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.Metrics(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var stats datasource.ConnectionStats
	if err := json.NewDecoder(rec.Body).Decode(&stats); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if stats.TotalConnections != 1 {
		t.Errorf("expected 1 total connection, got %d", stats.TotalConnections)
	}
	if stats.MaxConnectionsPerUser != 10 {
		t.Errorf("expected max 10 connections per user, got %d", stats.MaxConnectionsPerUser)
	}
	if stats.TTLMinutes != 5 {
		t.Errorf("expected TTL 5 minutes, got %d", stats.TTLMinutes)
	}
	if len(stats.ConnectionsByProject) != 1 {
		t.Errorf("expected 1 project in stats, got %d", len(stats.ConnectionsByProject))
	}
	if len(stats.ConnectionsByUser) != 1 {
		t.Errorf("expected 1 user in stats, got %d", len(stats.ConnectionsByUser))
	}
}

func TestHealthHandler_RegisterRoutes(t *testing.T) {
	cfg := &config.Config{}
	handler := NewHealthHandler(cfg, nil, zap.NewNop())

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
	assertPingCORSHeaders(t, rec)

	req = httptest.NewRequest(http.MethodOptions, "/ping", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("/ping OPTIONS: expected status %d, got %d", http.StatusNoContent, rec.Code)
	}
	assertPingCORSHeaders(t, rec)

	// Test /metrics is registered
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	// Should return 503 when no connection manager is provided
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/metrics: expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func assertPingCORSHeaders(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected Access-Control-Allow-Origin '*', got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "GET, HEAD, OPTIONS" {
		t.Errorf("expected Access-Control-Allow-Methods 'GET, HEAD, OPTIONS', got %q", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type" {
		t.Errorf("expected Access-Control-Allow-Headers 'Content-Type', got %q", got)
	}
}
