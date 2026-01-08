package retry

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// Config defines retry behavior with exponential backoff
type Config struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
	JitterFactor float64 // 0.0-1.0, default 0.1 for +/-10% jitter to prevent thundering herd
}

// DefaultConfig returns sensible defaults for database operations
// 3 retries with 100ms initial delay, capped at 5s, doubling each time, with 10% jitter
func DefaultConfig() *Config {
	return &Config{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
		JitterFactor: 0.1, // +/-10% jitter to prevent thundering herd
	}
}

// applyJitter adds random jitter to a delay to prevent thundering herd.
// Returns the delay with jitter applied if jitterFactor > 0.
// Jitter is calculated as: delay +/- (delay * jitterFactor * random(-1 to +1))
func applyJitter(delay time.Duration, jitterFactor float64) time.Duration {
	if jitterFactor <= 0 {
		return delay
	}
	// Random value between -jitterFactor and +jitterFactor
	jitter := float64(delay) * jitterFactor * (rand.Float64()*2 - 1)
	return time.Duration(float64(delay) + jitter)
}

// Do executes fn with exponential backoff retry logic
// Returns nil on success, or last error after all retries exhausted
// Respects context cancellation during wait periods
func Do(ctx context.Context, cfg *Config, fn func() error) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err

			if attempt < cfg.MaxRetries {
				select {
				case <-time.After(applyJitter(delay, cfg.JitterFactor)):
					delay = time.Duration(float64(delay) * cfg.Multiplier)
					if delay > cfg.MaxDelay {
						delay = cfg.MaxDelay
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}

	return lastErr
}

// DoWithResult executes fn and returns both result and error
// Useful for functions that return values (like pgxpool.New)
// Respects context cancellation during wait periods
func DoWithResult[T any](ctx context.Context, cfg *Config, fn func() (T, error)) (T, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var result T
	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		r, err := fn()
		if err == nil {
			return r, nil
		}

		lastErr = err
		result = r // Keep last result even on error

		if attempt < cfg.MaxRetries {
			select {
			case <-time.After(applyJitter(delay, cfg.JitterFactor)):
				delay = time.Duration(float64(delay) * cfg.Multiplier)
				if delay > cfg.MaxDelay {
					delay = cfg.MaxDelay
				}
			case <-ctx.Done():
				return result, ctx.Err()
			}
		}
	}

	return result, lastErr
}

// RetryableError is an interface for errors that explicitly declare their retryability.
// LLM errors implement this interface to provide explicit retry behavior.
type RetryableError interface {
	error
	IsRetryable() bool
}

// IsRetryable determines if an error is transient and worth retrying
// This prevents wasting retries on permanent failures (auth errors, bad SQL, etc.)
//
// The function checks errors in this order:
// 1. If the error implements RetryableError interface, use its IsRetryable() method
// 2. Otherwise, pattern-match against known retryable error strings
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check if the error implements RetryableError interface
	// This allows LLM errors (and others) to explicitly declare retryability
	type retryable interface {
		IsRetryable() bool
	}
	if r, ok := err.(retryable); ok {
		return r.IsRetryable()
	}

	// Fall back to pattern matching for other error types
	errStr := strings.ToLower(err.Error())
	retryablePatterns := []string{
		// Connection errors
		"connection refused",
		"connection reset",
		"broken pipe",
		"no such host",
		"timeout",
		"temporary failure",
		"too many connections",
		"deadlock",
		"i/o timeout",
		"network is unreachable",
		"connection timed out",
		// HTTP status codes
		"429",
		"500",
		"502",
		"503",
		"504",
		// HTTP error messages
		"rate limit",
		"service busy",
		"service unavailable",
		"too many requests",
		// GPU/CUDA errors
		"cuda error",
		"gpu error",
		"out of memory",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// DoIfRetryable only retries if the error is transient
// For permanent errors (auth failures, bad SQL, etc.), it returns immediately
// Respects context cancellation during wait periods
func DoIfRetryable(ctx context.Context, cfg *Config, fn func() error) error {
	if cfg == nil {
		cfg = DefaultConfig()
	}

	var lastErr error
	delay := cfg.InitialDelay

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err

			// Don't retry non-transient errors
			if !IsRetryable(err) {
				return err
			}

			if attempt < cfg.MaxRetries {
				select {
				case <-time.After(applyJitter(delay, cfg.JitterFactor)):
					delay = time.Duration(float64(delay) * cfg.Multiplier)
					if delay > cfg.MaxDelay {
						delay = cfg.MaxDelay
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}

	return lastErr
}
