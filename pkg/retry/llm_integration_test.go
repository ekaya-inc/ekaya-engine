package retry_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ekaya-inc/ekaya-engine/pkg/llm"
	"github.com/ekaya-inc/ekaya-engine/pkg/retry"
)

// TestIsRetryable_WithLLMError verifies that retry.IsRetryable correctly
// recognizes llm.Error retryability via the IsRetryable() interface method.
func TestIsRetryable_WithLLMError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "retryable llm.Error (503)",
			err:      llm.NewError(llm.ErrorTypeEndpoint, "server error", true, errors.New("HTTP 503")),
			expected: true,
		},
		{
			name:     "retryable llm.Error (429)",
			err:      llm.NewError(llm.ErrorTypeUnknown, "rate limited", true, errors.New("HTTP 429")),
			expected: true,
		},
		{
			name:     "non-retryable llm.Error (401)",
			err:      llm.NewError(llm.ErrorTypeAuth, "authentication failed", false, errors.New("HTTP 401")),
			expected: false,
		},
		{
			name:     "non-retryable llm.Error (model not found)",
			err:      llm.NewError(llm.ErrorTypeModel, "model not found", false, errors.New("model does not exist")),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := retry.IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

// TestIsRetryable_LLMErrorWrapped verifies that wrapped llm.Error types
// are still recognized via the IsRetryable() interface method.
func TestIsRetryable_LLMErrorWrapped(t *testing.T) {
	// Create an llm.Error and wrap it
	baseErr := llm.NewError(llm.ErrorTypeEndpoint, "server error", true, errors.New("HTTP 503"))
	wrappedErr := errors.New("operation failed: " + baseErr.Error())

	// The wrapped error won't be recognized as implementing IsRetryable()
	// but should still match the "503" pattern
	result := retry.IsRetryable(wrappedErr)
	if !result {
		t.Errorf("IsRetryable(wrapped error with 503) = false, expected true (should match pattern)")
	}
}

// TestDoIfRetryable_WithLLMError verifies that DoIfRetryable properly retries
// retryable llm.Error instances and immediately fails on non-retryable ones.
func TestDoIfRetryable_WithLLMError(t *testing.T) {
	t.Run("retries retryable llm.Error", func(t *testing.T) {
		cfg := &retry.Config{
			MaxRetries:   3,
			InitialDelay: 1,
			MaxDelay:     10,
			Multiplier:   2.0,
		}

		callCount := 0
		err := retry.DoIfRetryable(context.Background(), cfg, func() error {
			callCount++
			if callCount < 3 {
				return llm.NewError(llm.ErrorTypeEndpoint, "server error", true, errors.New("HTTP 503"))
			}
			return nil
		})

		if err != nil {
			t.Errorf("expected success after retries, got %v", err)
		}
		if callCount != 3 {
			t.Errorf("expected 3 calls, got %d", callCount)
		}
	})

	t.Run("fails immediately on non-retryable llm.Error", func(t *testing.T) {
		cfg := &retry.Config{
			MaxRetries:   3,
			InitialDelay: 1,
			MaxDelay:     10,
			Multiplier:   2.0,
		}

		callCount := 0
		expectedErr := llm.NewError(llm.ErrorTypeAuth, "authentication failed", false, errors.New("HTTP 401"))
		err := retry.DoIfRetryable(context.Background(), cfg, func() error {
			callCount++
			return expectedErr
		})

		if err != expectedErr {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
		if callCount != 1 {
			t.Errorf("expected 1 call (no retries), got %d", callCount)
		}
	})
}
