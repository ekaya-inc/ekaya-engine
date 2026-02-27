package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestErrorResponse(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		errorCode  string
		message    string
	}{
		{"bad request", http.StatusBadRequest, "bad_request", "invalid input"},
		{"not found", http.StatusNotFound, "not_found", "resource not found"},
		{"internal error", http.StatusInternalServerError, "internal_error", "something went wrong"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()

			err := ErrorResponse(w, tt.statusCode, tt.errorCode, tt.message)
			if err != nil {
				t.Fatalf("ErrorResponse returned error: %v", err)
			}

			resp := w.Result()
			defer resp.Body.Close()

			if resp.StatusCode != tt.statusCode {
				t.Errorf("status code = %d, want %d", resp.StatusCode, tt.statusCode)
			}

			ct := resp.Header.Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}

			var body map[string]string
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response body: %v", err)
			}

			if body["error"] != tt.errorCode {
				t.Errorf("body[error] = %q, want %q", body["error"], tt.errorCode)
			}
			if body["message"] != tt.message {
				t.Errorf("body[message] = %q, want %q", body["message"], tt.message)
			}
		})
	}
}

func TestWriteJSON_Status200(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	err := WriteJSON(w, http.StatusOK, data)
	if err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	resp := w.Result()
	defer resp.Body.Close()

	// Status 200 is the default for ResponseRecorder, WriteJSON should not call WriteHeader
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want %q", ct, "application/json")
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body["key"] != "value" {
		t.Errorf("body[key] = %q, want %q", body["key"], "value")
	}
}

func TestWriteJSON_NonOKStatus(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]int{"count": 5}

	err := WriteJSON(w, http.StatusCreated, data)
	if err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	resp := w.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("status code = %d, want %d", resp.StatusCode, http.StatusCreated)
	}
}

func TestWriteJSON_UnencodableData(t *testing.T) {
	w := httptest.NewRecorder()
	data := make(chan int) // channels cannot be JSON-encoded

	err := WriteJSON(w, http.StatusOK, data)
	if err == nil {
		t.Error("expected error for unencodable data, got nil")
	}
}
