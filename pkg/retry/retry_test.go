package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", cfg.MaxRetries)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("expected InitialDelay=100ms, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 5*time.Second {
		t.Errorf("expected MaxDelay=5s, got %v", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("expected Multiplier=2.0, got %f", cfg.Multiplier)
	}
}

func TestDo_Success(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := Do(ctx, cfg, func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestDo_SuccessAfterRetries(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := Do(ctx, cfg, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("transient error")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected no error after retries, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestDo_MaxRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	expectedErr := errors.New("persistent error")
	callCount := 0
	err := Do(ctx, cfg, func() error {
		callCount++
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	// MaxRetries=2 means: initial attempt + 2 retries = 3 total calls
	if callCount != 3 {
		t.Errorf("expected 3 calls (1 initial + 2 retries), got %d", callCount)
	}
}

func TestDo_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := &Config{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}

	callCount := 0
	start := time.Now()

	// Cancel context after first failure
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Do(ctx, cfg, func() error {
		callCount++
		return errors.New("error")
	})

	elapsed := time.Since(start)

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	// Should only make 1 call before context is canceled
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
	// Should return quickly after context cancellation
	if elapsed > 200*time.Millisecond {
		t.Errorf("expected quick cancellation, took %v", elapsed)
	}
}

func TestDo_ExponentialBackoff(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2.0,
	}

	callTimes := []time.Time{}
	err := Do(ctx, cfg, func() error {
		callTimes = append(callTimes, time.Now())
		return errors.New("error")
	})

	if err == nil {
		t.Error("expected error after exhausting retries")
	}

	// Should have 4 calls: initial + 3 retries
	if len(callTimes) != 4 {
		t.Errorf("expected 4 calls, got %d", len(callTimes))
	}

	// Check delay between calls (with some tolerance for timing)
	if len(callTimes) >= 2 {
		delay1 := callTimes[1].Sub(callTimes[0])
		if delay1 < 45*time.Millisecond || delay1 > 70*time.Millisecond {
			t.Errorf("expected ~50ms delay, got %v", delay1)
		}
	}

	if len(callTimes) >= 3 {
		delay2 := callTimes[2].Sub(callTimes[1])
		if delay2 < 90*time.Millisecond || delay2 > 130*time.Millisecond {
			t.Errorf("expected ~100ms delay, got %v", delay2)
		}
	}

	if len(callTimes) >= 4 {
		delay3 := callTimes[3].Sub(callTimes[2])
		if delay3 < 180*time.Millisecond || delay3 > 240*time.Millisecond {
			t.Errorf("expected ~200ms delay, got %v", delay3)
		}
	}
}

func TestDo_MaxDelayRespected(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     150 * time.Millisecond,
		Multiplier:   2.0,
	}

	callTimes := []time.Time{}
	err := Do(ctx, cfg, func() error {
		callTimes = append(callTimes, time.Now())
		return errors.New("error")
	})

	if err == nil {
		t.Error("expected error after exhausting retries")
	}

	// Check that delays never exceed MaxDelay
	for i := 1; i < len(callTimes); i++ {
		delay := callTimes[i].Sub(callTimes[i-1])
		if delay > 200*time.Millisecond {
			t.Errorf("delay %v exceeds MaxDelay (150ms) by too much", delay)
		}
	}
}

func TestDo_NilConfig(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	err := Do(ctx, nil, func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("expected no error with nil config, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestDoWithResult_Success(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	result, err := DoWithResult(ctx, cfg, func() (string, error) {
		callCount++
		return "success", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %s", result)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestDoWithResult_SuccessAfterRetries(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	result, err := DoWithResult(ctx, cfg, func() (int, error) {
		callCount++
		if callCount < 3 {
			return 0, errors.New("transient error")
		}
		return 42, nil
	})

	if err != nil {
		t.Errorf("expected no error after retries, got %v", err)
	}
	if result != 42 {
		t.Errorf("expected 42, got %d", result)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestDoWithResult_MaxRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	expectedErr := errors.New("persistent error")
	callCount := 0
	result, err := DoWithResult(ctx, cfg, func() (string, error) {
		callCount++
		return "partial", expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if result != "partial" {
		t.Errorf("expected 'partial' result, got %s", result)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestDoWithResult_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := &Config{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}

	callCount := 0

	// Cancel context after first failure
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	result, err := DoWithResult(ctx, cfg, func() (int, error) {
		callCount++
		return callCount, errors.New("error")
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if result != 1 {
		t.Errorf("expected result=1 (last attempt), got %d", result)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestDoWithResult_NilConfig(t *testing.T) {
	ctx := context.Background()

	result, err := DoWithResult(ctx, nil, func() (bool, error) {
		return true, nil
	})

	if err != nil {
		t.Errorf("expected no error with nil config, got %v", err)
	}
	if !result {
		t.Error("expected true result")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		// Connection errors
		{"connection refused", errors.New("connection refused"), true},
		{"Connection Refused (uppercase)", errors.New("Connection Refused"), true},
		{"connection reset", errors.New("connection reset by peer"), true},
		{"broken pipe", errors.New("write: broken pipe"), true},
		{"no such host", errors.New("no such host"), true},
		{"timeout", errors.New("context deadline exceeded: timeout"), true},
		{"i/o timeout", errors.New("i/o timeout"), true},
		{"connection timed out", errors.New("connection timed out"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"temporary failure", errors.New("temporary failure in name resolution"), true},
		{"too many connections", errors.New("too many connections"), true},
		{"deadlock", errors.New("deadlock detected"), true},
		// HTTP status codes
		{"HTTP 429", errors.New("HTTP 429 Too Many Requests"), true},
		{"HTTP 500", errors.New("HTTP 500 Internal Server Error"), true},
		{"HTTP 502", errors.New("HTTP 502 Bad Gateway"), true},
		{"HTTP 503", errors.New("HTTP 503 Service Unavailable"), true},
		{"HTTP 504", errors.New("HTTP 504 Gateway Timeout"), true},
		// HTTP error messages
		{"rate limit", errors.New("rate limit exceeded"), true},
		{"service busy", errors.New("service busy, try again later"), true},
		{"service unavailable", errors.New("service unavailable"), true},
		{"too many requests", errors.New("too many requests"), true},
		// GPU/CUDA errors
		{"cuda error", errors.New("CUDA error: out of memory"), true},
		{"gpu error", errors.New("GPU error occurred"), true},
		{"out of memory", errors.New("out of memory"), true},
		// Non-retryable errors
		{"auth error", errors.New("authentication failed"), false},
		{"permission denied", errors.New("permission denied"), false},
		{"syntax error", errors.New("syntax error at position 10"), false},
		{"invalid credentials", errors.New("invalid credentials"), false},
		{"not found", errors.New("table not found"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

// mockRetryableError implements the RetryableError interface for testing
type mockRetryableError struct {
	message   string
	retryable bool
}

func (e *mockRetryableError) Error() string {
	return e.message
}

func (e *mockRetryableError) IsRetryable() bool {
	return e.retryable
}

func TestIsRetryable_WithRetryableInterface(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "retryable error via interface",
			err:      &mockRetryableError{message: "transient error", retryable: true},
			expected: true,
		},
		{
			name:     "non-retryable error via interface",
			err:      &mockRetryableError{message: "permanent error", retryable: false},
			expected: false,
		},
		{
			name: "retryable interface takes precedence over pattern matching",
			// Even though message contains "timeout" (would match pattern), interface says not retryable
			err:      &mockRetryableError{message: "timeout error", retryable: false},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, expected %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestDoIfRetryable_RetryableError(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := DoIfRetryable(ctx, cfg, func() error {
		callCount++
		if callCount < 3 {
			return errors.New("connection timeout")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected no error after retries, got %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestDoIfRetryable_NonRetryableError(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	expectedErr := errors.New("authentication failed")
	callCount := 0
	err := DoIfRetryable(ctx, cfg, func() error {
		callCount++
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	// Should NOT retry non-retryable errors
	if callCount != 1 {
		t.Errorf("expected 1 call (no retries), got %d", callCount)
	}
}

func TestDoIfRetryable_MaxRetriesExhausted(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	expectedErr := errors.New("connection refused")
	callCount := 0
	err := DoIfRetryable(ctx, cfg, func() error {
		callCount++
		return expectedErr
	})

	if err != expectedErr {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	// Should retry retryable errors until max retries
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestDoIfRetryable_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := &Config{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
	}

	callCount := 0

	// Cancel context after first failure
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := DoIfRetryable(ctx, cfg, func() error {
		callCount++
		return errors.New("connection timeout")
	})

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestDoIfRetryable_NilConfig(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	err := DoIfRetryable(ctx, nil, func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("expected no error with nil config, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestDoIfRetryable_Success(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
	}

	callCount := 0
	err := DoIfRetryable(ctx, cfg, func() error {
		callCount++
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestApplyJitter_NoJitter(t *testing.T) {
	delay := 100 * time.Millisecond
	result := applyJitter(delay, 0)
	if result != delay {
		t.Errorf("expected no jitter with factor=0, got %v instead of %v", result, delay)
	}

	result = applyJitter(delay, -0.1)
	if result != delay {
		t.Errorf("expected no jitter with negative factor, got %v instead of %v", result, delay)
	}
}

func TestApplyJitter_WithJitter(t *testing.T) {
	delay := 100 * time.Millisecond
	jitterFactor := 0.1 // +/-10%

	// Run multiple times to verify randomness is within bounds
	for i := 0; i < 100; i++ {
		result := applyJitter(delay, jitterFactor)

		// Result should be within +/-10% of delay
		minDelay := time.Duration(float64(delay) * (1.0 - jitterFactor))
		maxDelay := time.Duration(float64(delay) * (1.0 + jitterFactor))

		if result < minDelay || result > maxDelay {
			t.Errorf("jittered delay %v is outside expected range [%v, %v]", result, minDelay, maxDelay)
		}
	}
}

func TestDefaultConfig_HasJitter(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.JitterFactor != 0.1 {
		t.Errorf("expected JitterFactor=0.1, got %f", cfg.JitterFactor)
	}
}

func TestDo_WithJitter(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   3,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     500 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0.2, // 20% jitter for easier detection
	}

	callTimes := []time.Time{}
	err := Do(ctx, cfg, func() error {
		callTimes = append(callTimes, time.Now())
		return errors.New("error")
	})

	if err == nil {
		t.Error("expected error after exhausting retries")
	}

	// Should have 4 calls: initial + 3 retries
	if len(callTimes) != 4 {
		t.Errorf("expected 4 calls, got %d", len(callTimes))
	}

	// Check that delays are within jittered range (not exact)
	// First delay should be ~50ms +/- 20% = [40ms, 60ms]
	if len(callTimes) >= 2 {
		delay1 := callTimes[1].Sub(callTimes[0])
		if delay1 < 35*time.Millisecond || delay1 > 70*time.Millisecond {
			t.Errorf("expected ~50ms delay with jitter, got %v", delay1)
		}
	}

	// Second delay should be ~100ms +/- 20% = [80ms, 120ms]
	if len(callTimes) >= 3 {
		delay2 := callTimes[2].Sub(callTimes[1])
		if delay2 < 75*time.Millisecond || delay2 > 135*time.Millisecond {
			t.Errorf("expected ~100ms delay with jitter, got %v", delay2)
		}
	}
}

func TestDoWithResult_WithJitter(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   2,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0.2, // 20% jitter
	}

	callTimes := []time.Time{}
	result, err := DoWithResult(ctx, cfg, func() (string, error) {
		callTimes = append(callTimes, time.Now())
		return "test", errors.New("error")
	})

	if err == nil {
		t.Error("expected error after exhausting retries")
	}
	if result != "test" {
		t.Errorf("expected 'test' result, got %s", result)
	}

	// Should have 3 calls: initial + 2 retries
	if len(callTimes) != 3 {
		t.Errorf("expected 3 calls, got %d", len(callTimes))
	}

	// Verify delays are within jittered range
	if len(callTimes) >= 2 {
		delay1 := callTimes[1].Sub(callTimes[0])
		if delay1 < 35*time.Millisecond || delay1 > 70*time.Millisecond {
			t.Errorf("expected ~50ms delay with jitter, got %v", delay1)
		}
	}
}

func TestDoIfRetryable_WithJitter(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		MaxRetries:   2,
		InitialDelay: 50 * time.Millisecond,
		MaxDelay:     200 * time.Millisecond,
		Multiplier:   2.0,
		JitterFactor: 0.2, // 20% jitter
	}

	callTimes := []time.Time{}
	err := DoIfRetryable(ctx, cfg, func() error {
		callTimes = append(callTimes, time.Now())
		return errors.New("connection timeout") // Retryable error
	})

	if err == nil {
		t.Error("expected error after exhausting retries")
	}

	// Should have 3 calls: initial + 2 retries
	if len(callTimes) != 3 {
		t.Errorf("expected 3 calls, got %d", len(callTimes))
	}

	// Verify delays are within jittered range
	if len(callTimes) >= 2 {
		delay1 := callTimes[1].Sub(callTimes[0])
		if delay1 < 35*time.Millisecond || delay1 > 70*time.Millisecond {
			t.Errorf("expected ~50ms delay with jitter, got %v", delay1)
		}
	}
}
