package retry

import (
	"context"
	"strings"
	"time"
)

// Config defines retry behavior with exponential backoff
type Config struct {
	MaxRetries   int
	InitialDelay time.Duration
	MaxDelay     time.Duration
	Multiplier   float64
}

// DefaultConfig returns sensible defaults for database operations
// 3 retries with 100ms initial delay, capped at 5s, doubling each time
func DefaultConfig() *Config {
	return &Config{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     5 * time.Second,
		Multiplier:   2.0,
	}
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
				case <-time.After(delay):
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
			case <-time.After(delay):
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

// IsRetryable determines if an error is transient and worth retrying
// This prevents wasting retries on permanent failures (auth errors, bad SQL, etc.)
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())
	retryablePatterns := []string{
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
				case <-time.After(delay):
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
