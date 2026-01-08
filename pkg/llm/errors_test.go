package llm

import (
	"errors"
	"strings"
	"testing"
)

// TestError_Error_WithStatusCode tests Error.Error() includes status code
func TestError_Error_WithStatusCode(t *testing.T) {
	err := &Error{
		Type:       ErrorTypeEndpoint,
		Message:    "server error",
		StatusCode: 503,
	}

	result := err.Error()
	if !strings.Contains(result, "HTTP 503") {
		t.Errorf("expected error message to contain 'HTTP 503', got: %s", result)
	}
	if !strings.Contains(result, "server error") {
		t.Errorf("expected error message to contain 'server error', got: %s", result)
	}
}

// TestError_Error_WithModel tests Error.Error() includes model name
func TestError_Error_WithModel(t *testing.T) {
	err := &Error{
		Type:    ErrorTypeEndpoint,
		Message: "rate limited",
		Model:   "gpt-4o",
	}

	result := err.Error()
	if !strings.Contains(result, "model=gpt-4o") {
		t.Errorf("expected error message to contain 'model=gpt-4o', got: %s", result)
	}
}

// TestError_Error_WithEndpoint tests Error.Error() includes endpoint host (redacted for security)
func TestError_Error_WithEndpoint(t *testing.T) {
	err := &Error{
		Type:     ErrorTypeEndpoint,
		Message:  "connection failed",
		Endpoint: "https://api.openai.com/v1",
	}

	result := err.Error()
	// Should only contain host, not full URL (redacted for security)
	if !strings.Contains(result, "endpoint=api.openai.com") {
		t.Errorf("expected error message to contain 'endpoint=api.openai.com', got: %s", result)
	}
	// Should NOT contain full path
	if strings.Contains(result, "/v1") {
		t.Errorf("endpoint should be redacted to host only, but got full URL: %s", result)
	}
}

// TestError_Error_WithStatusCodeAndModel tests Error.Error() includes status code, model, and endpoint
func TestError_Error_WithStatusCodeAndModel(t *testing.T) {
	err := &Error{
		Type:       ErrorTypeEndpoint,
		Message:    "server error",
		StatusCode: 503,
		Model:      "gpt-4o",
		Endpoint:   "https://api.openai.com/v1",
	}

	result := err.Error()
	if !strings.Contains(result, "HTTP 503") {
		t.Errorf("expected error message to contain 'HTTP 503', got: %s", result)
	}
	if !strings.Contains(result, "model=gpt-4o") {
		t.Errorf("expected error message to contain 'model=gpt-4o', got: %s", result)
	}
	// Endpoint is redacted to host only
	if !strings.Contains(result, "endpoint=api.openai.com") {
		t.Errorf("expected error message to contain 'endpoint=api.openai.com', got: %s", result)
	}
	if !strings.Contains(result, "server error") {
		t.Errorf("expected error message to contain 'server error', got: %s", result)
	}
}

// TestError_Error_WithCause tests Error.Error() includes cause
func TestError_Error_WithCause(t *testing.T) {
	cause := errors.New("underlying connection error")
	err := &Error{
		Type:       ErrorTypeEndpoint,
		Message:    "connection failed",
		StatusCode: 503,
		Model:      "gpt-4o",
		Cause:      cause,
	}

	result := err.Error()
	if !strings.Contains(result, "underlying connection error") {
		t.Errorf("expected error message to contain cause, got: %s", result)
	}
}

// TestError_Error_MinimalContext tests Error.Error() without optional fields
func TestError_Error_MinimalContext(t *testing.T) {
	err := &Error{
		Type:    ErrorTypeAuth,
		Message: "authentication failed",
	}

	result := err.Error()
	expected := "auth authentication failed"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestClassifyError_ExtractsStatusCode tests ClassifyError extracts status codes
func TestClassifyError_ExtractsStatusCode(t *testing.T) {
	tests := []struct {
		name               string
		inputError         error
		expectedStatusCode int
		expectedType       ErrorType
	}{
		{
			name:               "503 service unavailable",
			inputError:         errors.New("HTTP 503 Service Unavailable"),
			expectedStatusCode: 503,
			expectedType:       ErrorTypeEndpoint,
		},
		{
			name:               "429 rate limit",
			inputError:         errors.New("HTTP 429 Too Many Requests"),
			expectedStatusCode: 429,
			expectedType:       ErrorTypeRateLimited,
		},
		{
			name:               "500 internal server error",
			inputError:         errors.New("HTTP 500 Internal Server Error"),
			expectedStatusCode: 500,
			expectedType:       ErrorTypeEndpoint,
		},
		{
			name:               "401 unauthorized",
			inputError:         errors.New("HTTP 401 Unauthorized"),
			expectedStatusCode: 401,
			expectedType:       ErrorTypeAuth,
		},
		{
			name:               "404 not found",
			inputError:         errors.New("HTTP 404 Not Found"),
			expectedStatusCode: 404,
			expectedType:       ErrorTypeEndpoint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ClassifyError(tt.inputError)
			if result.StatusCode != tt.expectedStatusCode {
				t.Errorf("expected status code %d, got %d", tt.expectedStatusCode, result.StatusCode)
			}
			if result.Type != tt.expectedType {
				t.Errorf("expected type %s, got %s", tt.expectedType, result.Type)
			}
		})
	}
}

// TestClassifyError_NoStatusCode tests ClassifyError with errors that don't have status codes
func TestClassifyError_NoStatusCode(t *testing.T) {
	err := errors.New("connection refused")
	result := ClassifyError(err)

	if result.StatusCode != 0 {
		t.Errorf("expected status code 0, got %d", result.StatusCode)
	}
	if result.Type != ErrorTypeEndpoint {
		t.Errorf("expected type %s, got %s", ErrorTypeEndpoint, result.Type)
	}
}

// TestNewErrorWithContext tests NewErrorWithContext constructor
func TestNewErrorWithContext(t *testing.T) {
	cause := errors.New("original error")
	err := NewErrorWithContext(
		ErrorTypeEndpoint,
		"server error",
		true,
		cause,
		"gpt-4o",
		"https://api.openai.com/v1",
		503,
	)

	if err.Type != ErrorTypeEndpoint {
		t.Errorf("expected type %s, got %s", ErrorTypeEndpoint, err.Type)
	}
	if err.Message != "server error" {
		t.Errorf("expected message 'server error', got %s", err.Message)
	}
	if !err.Retryable {
		t.Error("expected error to be retryable")
	}
	if err.Cause != cause {
		t.Error("expected cause to be set")
	}
	if err.Model != "gpt-4o" {
		t.Errorf("expected model 'gpt-4o', got %s", err.Model)
	}
	if err.Endpoint != "https://api.openai.com/v1" {
		t.Errorf("expected endpoint 'https://api.openai.com/v1', got %s", err.Endpoint)
	}
	if err.StatusCode != 503 {
		t.Errorf("expected status code 503, got %d", err.StatusCode)
	}
}

// TestNewErrorWithContext_ErrorMessage tests that NewErrorWithContext produces proper error messages
func TestNewErrorWithContext_ErrorMessage(t *testing.T) {
	err := NewErrorWithContext(
		ErrorTypeEndpoint,
		"server error",
		true,
		errors.New("underlying network issue"),
		"gpt-4o",
		"https://api.openai.com/v1",
		503,
	)

	result := err.Error()

	// Check that all context is included
	if !strings.Contains(result, "HTTP 503") {
		t.Errorf("expected error message to contain 'HTTP 503', got: %s", result)
	}
	if !strings.Contains(result, "model=gpt-4o") {
		t.Errorf("expected error message to contain 'model=gpt-4o', got: %s", result)
	}
	// Endpoint is redacted to host only
	if !strings.Contains(result, "endpoint=api.openai.com") {
		t.Errorf("expected error message to contain 'endpoint=api.openai.com', got: %s", result)
	}
	if !strings.Contains(result, "server error") {
		t.Errorf("expected error message to contain 'server error', got: %s", result)
	}
	if !strings.Contains(result, "underlying network issue") {
		t.Errorf("expected error message to contain 'underlying network issue', got: %s", result)
	}
}

// TestClassifyError_PreservesExistingError tests that ClassifyError returns existing *Error unchanged
func TestClassifyError_PreservesExistingError(t *testing.T) {
	original := &Error{
		Type:       ErrorTypeEndpoint,
		Message:    "server error",
		Retryable:  true,
		StatusCode: 503,
		Model:      "gpt-4o",
		Endpoint:   "https://api.openai.com/v1",
	}

	result := ClassifyError(original)

	if result != original {
		t.Error("expected ClassifyError to return the same *Error instance")
	}
}

// TestError_Unwrap tests that Unwrap returns the underlying cause
func TestError_Unwrap(t *testing.T) {
	cause := errors.New("underlying error")
	err := &Error{
		Type:    ErrorTypeEndpoint,
		Message: "server error",
		Cause:   cause,
	}

	unwrapped := err.Unwrap()
	if unwrapped != cause {
		t.Error("expected Unwrap to return the underlying cause")
	}
}

// TestError_IsRetryable tests the IsRetryable method
func TestError_IsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		retryable bool
	}{
		{"retryable error", true},
		{"non-retryable error", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &Error{
				Type:      ErrorTypeEndpoint,
				Message:   "test error",
				Retryable: tt.retryable,
			}

			if err.IsRetryable() != tt.retryable {
				t.Errorf("expected IsRetryable() to return %v", tt.retryable)
			}
		})
	}
}

// TestClassifyError_ContextCanceledNotRetryable tests that context canceled errors are not retryable
func TestClassifyError_ContextCanceledNotRetryable(t *testing.T) {
	err := errors.New("context canceled")
	result := ClassifyError(err)

	if result.Retryable {
		t.Error("context canceled should NOT be retryable")
	}
	if result.Message != "request cancelled" {
		t.Errorf("expected message 'request cancelled', got %s", result.Message)
	}
}

// TestExtractStatusCode_Precision tests that status code extraction avoids false positives
func TestExtractStatusCode_Precision(t *testing.T) {
	tests := []struct {
		name         string
		errStr       string
		expectedCode int
	}{
		{"HTTP prefix", "HTTP 503 Service Unavailable", 503},
		{"status prefix", "status 429 rate limited", 429},
		{"status colon", "status: 500", 500},
		{"code prefix", "code 502 bad gateway", 502},
		{"code colon", "code: 504 timeout", 504},
		{"no false positive - processed records", "processed 503 records", 0},
		{"no false positive - port number", "port 5432 connection failed", 0},
		{"no false positive - random number", "error after 429 seconds", 0},
		{"mixed case HTTP", "http 503 error", 503},
		{"case insensitive status", "Status: 404 Not Found", 404},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractStatusCode(tt.errStr)
			if result != tt.expectedCode {
				t.Errorf("extractStatusCode(%q) = %d, expected %d", tt.errStr, result, tt.expectedCode)
			}
		})
	}
}

// TestClassifyError_RateLimitedType tests that rate limit errors get proper type
func TestClassifyError_RateLimitedType(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected ErrorType
	}{
		{"HTTP 429", "HTTP 429 Too Many Requests", ErrorTypeRateLimited},
		{"rate limit text", "rate limit exceeded", ErrorTypeRateLimited},
		{"too many requests", "too many requests", ErrorTypeRateLimited},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errors.New(tt.errStr)
			result := ClassifyError(err)
			if result.Type != tt.expected {
				t.Errorf("expected type %s, got %s", tt.expected, result.Type)
			}
		})
	}
}
