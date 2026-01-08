package llm

import (
	"errors"
	"fmt"
	"strings"
)

// Error represents a structured LLM error with classification.
type Error struct {
	Type       ErrorType // Classification of the error (uses ErrorType from tester.go)
	Message    string    // Human-readable message
	Retryable  bool      // Whether the operation can be retried
	Cause      error     // Underlying error
	StatusCode int       // HTTP status code if applicable
	Model      string    // Model name if known
	Endpoint   string    // Endpoint URL if known
}

// Error implements the error interface.
func (e *Error) Error() string {
	var parts []string
	parts = append(parts, string(e.Type))

	if e.StatusCode > 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", e.StatusCode))
	}
	if e.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", e.Model))
	}

	parts = append(parts, e.Message)

	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", strings.Join(parts, " "), e.Cause)
	}
	return strings.Join(parts, " ")
}

// Unwrap returns the underlying cause for errors.Is/As.
func (e *Error) Unwrap() error {
	return e.Cause
}

// IsRetryable implements the retry.RetryableError interface.
// This allows the retry package to check retryability without importing llm.
func (e *Error) IsRetryable() bool {
	return e.Retryable
}

// NewError creates a new structured LLM error.
func NewError(errType ErrorType, message string, retryable bool, cause error) *Error {
	return &Error{
		Type:      errType,
		Message:   message,
		Retryable: retryable,
		Cause:     cause,
	}
}

// NewErrorWithContext creates a new structured LLM error with additional context.
func NewErrorWithContext(errType ErrorType, message string, retryable bool, cause error, model, endpoint string, statusCode int) *Error {
	return &Error{
		Type:       errType,
		Message:    message,
		Retryable:  retryable,
		Cause:      cause,
		Model:      model,
		Endpoint:   endpoint,
		StatusCode: statusCode,
	}
}

// ClassifyError categorizes an error and returns a structured Error.
// This consolidates error classification logic for consistent handling.
func ClassifyError(err error) *Error {
	if err == nil {
		return nil
	}

	// Check if already an *Error
	var llmErr *Error
	if errors.As(err, &llmErr) {
		return llmErr
	}

	errStr := err.Error()
	lower := strings.ToLower(errStr)

	// Extract HTTP status code from error string
	statusCode := 0
	for _, code := range []int{400, 401, 403, 404, 429, 500, 502, 503, 504} {
		if strings.Contains(errStr, fmt.Sprintf("%d", code)) {
			statusCode = code
			break
		}
	}

	// Authentication errors (not retryable)
	if strings.Contains(errStr, "401") || strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "invalid api key") {
		llmErr := NewError(ErrorTypeAuth, "authentication failed", false, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// Model not found (not retryable without config change)
	if strings.Contains(lower, "model") && (strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist")) {
		llmErr := NewError(ErrorTypeModel, "model not found", false, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// Endpoint not found (not retryable without config change)
	if strings.Contains(errStr, "404") {
		llmErr := NewError(ErrorTypeEndpoint, "endpoint not found", false, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// Connection errors (may be retryable)
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") {
		llmErr := NewError(ErrorTypeEndpoint, "connection failed", true, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// Timeout and deadline exceeded (retryable)
	if strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded") ||
		strings.Contains(lower, "context canceled") {
		llmErr := NewError(ErrorTypeEndpoint, "request timeout", true, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// Rate limiting (retryable after backoff)
	if strings.Contains(errStr, "429") || strings.Contains(lower, "rate limit") {
		llmErr := NewError(ErrorTypeUnknown, "rate limited", true, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// CUDA/GPU errors (transient server-side issues, retryable)
	if strings.Contains(lower, "cuda error") || strings.Contains(lower, "gpu error") ||
		strings.Contains(lower, "out of memory") {
		llmErr := NewError(ErrorTypeEndpoint, "GPU error", true, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// 5xx server errors (retryable)
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") || strings.Contains(errStr, "504") {
		llmErr := NewError(ErrorTypeEndpoint, "server error", true, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// Unknown error
	llmErr = NewError(ErrorTypeUnknown, "llm error", false, err)
	llmErr.StatusCode = statusCode
	return llmErr
}

// IsRetryable returns true if the error is retryable.
func IsRetryable(err error) bool {
	var llmErr *Error
	if errors.As(err, &llmErr) {
		return llmErr.Retryable
	}
	return false
}

// GetErrorType extracts the ErrorType from an error.
func GetErrorType(err error) ErrorType {
	var llmErr *Error
	if errors.As(err, &llmErr) {
		return llmErr.Type
	}
	return ErrorTypeUnknown
}
