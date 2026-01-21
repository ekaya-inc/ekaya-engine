package llm

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/sashabaranov/go-openai"
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
	if e.Endpoint != "" {
		// Redact endpoint to host only to avoid leaking sensitive info (API keys, tokens)
		if u, err := url.Parse(e.Endpoint); err == nil && u.Host != "" {
			parts = append(parts, fmt.Sprintf("endpoint=%s", u.Host))
		} else {
			parts = append(parts, fmt.Sprintf("endpoint=%s", e.Endpoint))
		}
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

// statusCodePattern matches HTTP status codes in error messages with context.
// Matches patterns like "HTTP 503", "status 503", "status: 503", "code 503", "code: 503"
// to avoid false positives like "processed 503 records".
var statusCodePattern = regexp.MustCompile(`(?i)(?:HTTP|status[:\s]*|code[:\s]*)\s*(\d{3})`)

// extractStatusCode extracts an HTTP status code from an error string.
// Returns 0 if no status code is found with proper context.
func extractStatusCode(errStr string) int {
	matches := statusCodePattern.FindStringSubmatch(errStr)
	if len(matches) >= 2 {
		var code int
		if _, err := fmt.Sscanf(matches[1], "%d", &code); err == nil {
			// Only return valid HTTP status codes
			if code >= 100 && code < 600 {
				return code
			}
		}
	}
	return 0
}

// classifyRequestError handles openai.RequestError specifically, avoiding its
// broken Error() method which produces "%!s(<nil>)" when Err is nil.
func classifyRequestError(reqErr *openai.RequestError) *Error {
	statusCode := reqErr.HTTPStatusCode

	// Build a clean error message from the response body
	// The body contains the actual error details from the API
	message := string(reqErr.Body)
	if message == "" {
		message = reqErr.HTTPStatus
	}

	// Classify based on status code and body content
	lower := strings.ToLower(message)

	// CUDA/GPU errors (transient server-side issues, retryable)
	if strings.Contains(lower, "cuda error") || strings.Contains(lower, "gpu error") {
		return &Error{
			Type:       ErrorTypeEndpoint,
			Message:    message,
			Retryable:  true,
			Cause:      reqErr.Err, // Use underlying error, not reqErr itself
			StatusCode: statusCode,
		}
	}

	// Rate limiting
	if statusCode == 429 || strings.Contains(lower, "rate limit") {
		return &Error{
			Type:       ErrorTypeRateLimited,
			Message:    message,
			Retryable:  true,
			Cause:      reqErr.Err,
			StatusCode: statusCode,
		}
	}

	// 5xx server errors
	if statusCode >= 500 {
		return &Error{
			Type:       ErrorTypeEndpoint,
			Message:    message,
			Retryable:  true,
			Cause:      reqErr.Err,
			StatusCode: statusCode,
		}
	}

	// 4xx client errors (generally not retryable)
	return &Error{
		Type:       ErrorTypeEndpoint,
		Message:    message,
		Retryable:  false,
		Cause:      reqErr.Err,
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

	// Handle openai.RequestError specially - its Error() method has a bug where
	// it uses %s on a nil error, producing "%!s(<nil>)" in the output.
	// We extract the useful information and construct a clean error message.
	var reqErr *openai.RequestError
	if errors.As(err, &reqErr) {
		return classifyRequestError(reqErr)
	}

	errStr := err.Error()
	lower := strings.ToLower(errStr)

	// Extract HTTP status code from error string using precise pattern matching
	statusCode := extractStatusCode(errStr)

	// Authentication errors (not retryable)
	if statusCode == 401 || strings.Contains(lower, "unauthorized") ||
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
	if statusCode == 404 {
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

	// Context canceled - NOT retryable (user-initiated cancellation)
	if strings.Contains(lower, "context canceled") {
		llmErr := NewError(ErrorTypeEndpoint, "request cancelled", false, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// Timeout and deadline exceeded (retryable)
	if strings.Contains(lower, "timeout") || strings.Contains(lower, "deadline exceeded") {
		llmErr := NewError(ErrorTypeEndpoint, "request timeout", true, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// Rate limiting (retryable after backoff)
	if statusCode == 429 || strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "too many requests") {
		llmErr := NewError(ErrorTypeRateLimited, "rate limited", true, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// CUDA/GPU errors (transient server-side issues, retryable)
	if strings.Contains(lower, "cuda error") || strings.Contains(lower, "gpu error") {
		llmErr := NewError(ErrorTypeEndpoint, "GPU error", true, err)
		llmErr.StatusCode = statusCode
		return llmErr
	}

	// 5xx server errors (retryable)
	if statusCode >= 500 && statusCode < 600 {
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
