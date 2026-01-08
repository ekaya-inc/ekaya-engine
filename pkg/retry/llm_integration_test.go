package retry_test

import (
	"context"
	"errors"
	"strings"
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
			err:      llm.NewError(llm.ErrorTypeRateLimited, "rate limited", true, errors.New("HTTP 429")),
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

// TestDoIfRetryable_LLMErrorEscalation verifies that repeated same-type LLM errors
// escalate to permanent failure after MaxSameErrorType consecutive failures.
func TestDoIfRetryable_LLMErrorEscalation(t *testing.T) {
	t.Run("escalates after repeated 503 errors", func(t *testing.T) {
		cfg := &retry.Config{
			MaxRetries:       10,                      // Allow many retries
			InitialDelay:     1,                       // Fast for testing
			MaxDelay:         10,                      // Fast for testing
			Multiplier:       2.0,                     //
			MaxSameErrorType: 3,                       // Escalate after 3 same-type errors
			JitterFactor:     0,                       // No jitter for deterministic test
		}

		callCount := 0
		err := retry.DoIfRetryable(context.Background(), cfg, func() error {
			callCount++
			// Always return 503 error
			return llm.NewErrorWithContext(
				llm.ErrorTypeEndpoint,
				"server error",
				true, // Marked retryable
				errors.New("HTTP 503 Service Busy"),
				"trtllm-qwen3-30b",
				"https://sparkone.example.com/v1",
				503,
			)
		})

		// Should escalate to permanent failure after 3 consecutive same-type errors
		if err == nil {
			t.Error("expected error after escalation, got nil")
		}
		if callCount != 3 {
			t.Errorf("expected 3 calls before escalation, got %d", callCount)
		}
		// Error message should indicate repeated error
		if err != nil {
			errMsg := err.Error()
			if !strings.Contains(errMsg, "repeated error") {
				t.Errorf("expected error message to contain 'repeated error', got: %s", errMsg)
			}
			if !strings.Contains(errMsg, "type=503") {
				t.Errorf("expected error message to contain 'type=503', got: %s", errMsg)
			}
		}
	})

	t.Run("resets counter on different error type", func(t *testing.T) {
		cfg := &retry.Config{
			MaxRetries:       10,
			InitialDelay:     1,
			MaxDelay:         10,
			Multiplier:       2.0,
			MaxSameErrorType: 3, // Escalate after 3 same-type errors
			JitterFactor:     0,
		}

		callCount := 0
		err := retry.DoIfRetryable(context.Background(), cfg, func() error {
			callCount++
			// Alternate between 503 and 429 errors
			if callCount%2 == 1 {
				return llm.NewError(llm.ErrorTypeEndpoint, "server error", true, errors.New("HTTP 503"))
			}
			return llm.NewError(llm.ErrorTypeRateLimited, "rate limited", true, errors.New("HTTP 429"))
		})

		// Should exhaust all retries since error types keep changing
		if err == nil {
			t.Error("expected error after exhausting retries, got nil")
		}
		// Should have done MaxRetries+1 calls (initial + 10 retries = 11)
		if callCount != 11 {
			t.Errorf("expected 11 calls (alternating types shouldn't escalate), got %d", callCount)
		}
	})

	t.Run("GPU errors grouped together for escalation", func(t *testing.T) {
		cfg := &retry.Config{
			MaxRetries:       10,
			InitialDelay:     1,
			MaxDelay:         10,
			Multiplier:       2.0,
			MaxSameErrorType: 3, // Escalate after 3 same-type errors
			JitterFactor:     0,
		}

		callCount := 0
		err := retry.DoIfRetryable(context.Background(), cfg, func() error {
			callCount++
			// Rotate through GPU-related errors (should all be grouped as "gpu" type)
			switch callCount {
			case 1:
				return llm.NewError(llm.ErrorTypeEndpoint, "GPU error", true, errors.New("CUDA error"))
			case 2:
				return llm.NewError(llm.ErrorTypeEndpoint, "GPU error", true, errors.New("out of memory"))
			default:
				return llm.NewError(llm.ErrorTypeEndpoint, "GPU error", true, errors.New("GPU error occurred"))
			}
		})

		// Should escalate after 3 calls since all GPU errors are grouped
		if err == nil {
			t.Error("expected error after GPU error escalation, got nil")
		}
		if callCount != 3 {
			t.Errorf("expected 3 calls before GPU error escalation, got %d", callCount)
		}
	})
}
