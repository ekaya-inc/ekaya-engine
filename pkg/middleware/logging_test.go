package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestRequestLogger_LogsRequests(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if logs.Len() != 1 {
		t.Errorf("expected 1 log entry, got %d", logs.Len())
	}

	entry := logs.All()[0]
	if entry.Message != "HTTP request" {
		t.Errorf("expected message 'HTTP request', got '%s'", entry.Message)
	}
}

func TestRequestLogger_NilLogger_PassesThrough(t *testing.T) {
	called := false
	handler := RequestLogger(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("expected handler to be called")
	}
}

func TestRequestLogger_CapturesStatusCode(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	entry := logs.All()[0]
	statusField := entry.ContextMap()["status"]
	if statusField != int64(http.StatusNotFound) {
		t.Errorf("expected status %d, got %v", http.StatusNotFound, statusField)
	}
}

func TestResponseWriter_PreventsDuplicateWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	// First call should work
	rw.WriteHeader(http.StatusCreated)
	if rw.statusCode != http.StatusCreated {
		t.Errorf("expected status %d, got %d", http.StatusCreated, rw.statusCode)
	}

	// Second call should be ignored (no panic, no warning)
	rw.WriteHeader(http.StatusInternalServerError)
	if rw.statusCode != http.StatusCreated {
		t.Errorf("expected status to remain %d, got %d", http.StatusCreated, rw.statusCode)
	}

	// The recorded response should have the first status code
	if rec.Code != http.StatusCreated {
		t.Errorf("expected recorded status %d, got %d", http.StatusCreated, rec.Code)
	}
}

func TestResponseWriter_WriteTriggersWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	// Writing without explicit WriteHeader should set status 200
	_, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rw.statusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rw.statusCode)
	}
	if !rw.headerWritten {
		t.Error("expected headerWritten to be true")
	}
}

func TestResponseWriter_ExplicitWriteHeaderBeforeWrite(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	// Explicit WriteHeader then Write
	rw.WriteHeader(http.StatusAccepted)
	_, err := rw.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rw.statusCode != http.StatusAccepted {
		t.Errorf("expected status %d, got %d", http.StatusAccepted, rw.statusCode)
	}
}

func TestRequestLogger_HandlerWritesMultipleHeaders(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	// Simulate a handler that tries to write headers twice (common bug)
	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.WriteHeader(http.StatusInternalServerError) // This should be ignored
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Should log the first status code
	entry := logs.All()[0]
	statusField := entry.ContextMap()["status"]
	if statusField != int64(http.StatusBadRequest) {
		t.Errorf("expected status %d, got %v", http.StatusBadRequest, statusField)
	}
}
