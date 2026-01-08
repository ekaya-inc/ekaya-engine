package llm

import (
	"errors"
	"fmt"
	"strings"
)

// Error represents a structured LLM error with classification.
type Error struct {
	Type      ErrorType // Classification of the error (uses ErrorType from tester.go)
	Message   string    // Human-readable message
	Retryable bool      // Whether the operation can be retried
	Cause     error     // Underlying error
}

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Type, e.Message)
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

	// Authentication errors (not retryable)
	if strings.Contains(errStr, "401") || strings.Contains(lower, "unauthorized") ||
		strings.Contains(lower, "invalid api key") {
		return NewError(ErrorTypeAuth, "authentication failed", false, err)
	}

	// Model not found (not retryable without config change)
	if strings.Contains(lower, "model") && (strings.Contains(lower, "not found") ||
		strings.Contains(lower, "does not exist")) {
		return NewError(ErrorTypeModel, "model not found", false, err)
	}

	// Endpoint not found (not retryable without config change)
	if strings.Contains(errStr, "404") {
		return NewError(ErrorTypeEndpoint, "endpoint not found", false, err)
	}

	// Connection errors (may be retryable)
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") {
		return NewError(ErrorTypeEndpoint, "connection failed", true, err)
	}

	// Timeout and deadline exceeded (retryable)
	if strings.Contains(lower, "timeout") ||
		strings.Contains(lower, "deadline exceeded") ||
		strings.Contains(lower, "context canceled") {
		return NewError(ErrorTypeEndpoint, "request timeout", true, err)
	}

	// Rate limiting (retryable after backoff)
	if strings.Contains(errStr, "429") || strings.Contains(lower, "rate limit") {
		return NewError(ErrorTypeUnknown, "rate limited", true, err)
	}

	// CUDA/GPU errors (transient server-side issues, retryable)
	if strings.Contains(lower, "cuda error") || strings.Contains(lower, "gpu error") ||
		strings.Contains(lower, "out of memory") {
		return NewError(ErrorTypeEndpoint, "GPU error", true, err)
	}

	// 5xx server errors (retryable)
	if strings.Contains(errStr, "500") || strings.Contains(errStr, "502") ||
		strings.Contains(errStr, "503") || strings.Contains(errStr, "504") {
		return NewError(ErrorTypeEndpoint, "server error", true, err)
	}

	// Unknown error
	return NewError(ErrorTypeUnknown, "llm error", false, err)
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
