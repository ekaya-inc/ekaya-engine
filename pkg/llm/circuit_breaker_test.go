package llm

import (
	"strings"
	"testing"
	"time"
)

func TestCircuitBreaker_InitialState(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  5,
		ResetAfter: 30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	if cb.State() != CircuitClosed {
		t.Errorf("expected initial state to be CircuitClosed, got %v", cb.State())
	}
	if cb.ConsecutiveFailures() != 0 {
		t.Errorf("expected initial consecutive failures to be 0, got %d", cb.ConsecutiveFailures())
	}

	allowed, err := cb.Allow()
	if !allowed {
		t.Errorf("expected Allow() to return true for closed circuit")
	}
	if err != nil {
		t.Errorf("expected no error for closed circuit, got %v", err)
	}
}

func TestCircuitBreaker_TripsAfterThreshold(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  3,
		ResetAfter: 30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record failures up to threshold
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Errorf("expected state to be CircuitOpen after %d failures, got %v", config.Threshold, cb.State())
	}
	if cb.ConsecutiveFailures() != 3 {
		t.Errorf("expected consecutive failures to be 3, got %d", cb.ConsecutiveFailures())
	}

	allowed, err := cb.Allow()
	if allowed {
		t.Errorf("expected Allow() to return false for open circuit")
	}
	if err == nil {
		t.Errorf("expected error for open circuit, got nil")
	}
	if !strings.Contains(err.Error(), "circuit breaker open") {
		t.Errorf("expected error to mention circuit breaker open, got: %v", err)
	}
}

func TestCircuitBreaker_DoesNotTripBeforeThreshold(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  5,
		ResetAfter: 30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record failures below threshold
	for i := 0; i < 4; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitClosed {
		t.Errorf("expected state to be CircuitClosed with failures below threshold, got %v", cb.State())
	}

	allowed, err := cb.Allow()
	if !allowed {
		t.Errorf("expected Allow() to return true when below threshold")
	}
	if err != nil {
		t.Errorf("expected no error when below threshold, got %v", err)
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  5,
		ResetAfter: 30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Record some failures
	cb.RecordFailure()
	cb.RecordFailure()
	cb.RecordFailure()

	if cb.ConsecutiveFailures() != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", cb.ConsecutiveFailures())
	}

	// Record success
	cb.RecordSuccess()

	if cb.ConsecutiveFailures() != 0 {
		t.Errorf("expected consecutive failures to be reset to 0 after success, got %d", cb.ConsecutiveFailures())
	}
	if cb.State() != CircuitClosed {
		t.Errorf("expected state to be CircuitClosed after success, got %v", cb.State())
	}
}

func TestCircuitBreaker_TransitionsToHalfOpen(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  3,
		ResetAfter: 100 * time.Millisecond, // Short timeout for test
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit to be open, got %v", cb.State())
	}

	// Try immediately - should fail
	allowed, err := cb.Allow()
	if allowed {
		t.Errorf("expected Allow() to return false immediately after tripping")
	}
	if err == nil {
		t.Errorf("expected error immediately after tripping")
	}

	// Wait for reset timeout
	time.Sleep(150 * time.Millisecond)

	// Try again - should transition to half-open
	allowed, err = cb.Allow()
	if !allowed {
		t.Errorf("expected Allow() to return true after reset timeout")
	}
	if err != nil {
		t.Errorf("expected no error after reset timeout, got %v", err)
	}
	if cb.State() != CircuitHalfOpen {
		t.Errorf("expected state to be CircuitHalfOpen after reset timeout, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenSuccessClosesCircuit(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  3,
		ResetAfter: 50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Wait for reset timeout and transition to half-open
	time.Sleep(60 * time.Millisecond)
	_, _ = cb.Allow() // Transition to half-open

	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected circuit to be half-open, got %v", cb.State())
	}

	// Record success in half-open state
	cb.RecordSuccess()

	if cb.State() != CircuitClosed {
		t.Errorf("expected state to be CircuitClosed after success in half-open, got %v", cb.State())
	}
	if cb.ConsecutiveFailures() != 0 {
		t.Errorf("expected consecutive failures to be 0 after success, got %d", cb.ConsecutiveFailures())
	}
}

func TestCircuitBreaker_HalfOpenFailureReopensCircuit(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  3,
		ResetAfter: 50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Wait for reset timeout and transition to half-open
	time.Sleep(60 * time.Millisecond)
	_, _ = cb.Allow() // Transition to half-open

	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected circuit to be half-open, got %v", cb.State())
	}

	// Record failure in half-open state
	cb.RecordFailure()

	if cb.State() != CircuitOpen {
		t.Errorf("expected state to be CircuitOpen after failure in half-open, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenRejectsAdditionalRequests(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  3,
		ResetAfter: 50 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	// Wait for reset timeout and transition to half-open
	time.Sleep(60 * time.Millisecond)
	allowed, err := cb.Allow() // Transition to half-open
	if !allowed || err != nil {
		t.Fatalf("expected first Allow() to succeed and transition to half-open")
	}

	if cb.State() != CircuitHalfOpen {
		t.Fatalf("expected circuit to be half-open, got %v", cb.State())
	}

	// Try another request while in half-open
	allowed, err = cb.Allow()
	if allowed {
		t.Errorf("expected Allow() to return false for additional requests in half-open state")
	}
	if err == nil {
		t.Errorf("expected error for additional requests in half-open state")
	}
	if !strings.Contains(err.Error(), "half-open") {
		t.Errorf("expected error to mention half-open state, got: %v", err)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  3,
		ResetAfter: 30 * time.Second,
	}
	cb := NewCircuitBreaker(config)

	// Trip the circuit
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}

	if cb.State() != CircuitOpen {
		t.Fatalf("expected circuit to be open, got %v", cb.State())
	}

	// Manually reset
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("expected state to be CircuitClosed after reset, got %v", cb.State())
	}
	if cb.ConsecutiveFailures() != 0 {
		t.Errorf("expected consecutive failures to be 0 after reset, got %d", cb.ConsecutiveFailures())
	}

	allowed, err := cb.Allow()
	if !allowed {
		t.Errorf("expected Allow() to return true after reset")
	}
	if err != nil {
		t.Errorf("expected no error after reset, got %v", err)
	}
}

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.Threshold != 5 {
		t.Errorf("expected default threshold to be 5, got %d", config.Threshold)
	}
	if config.ResetAfter != 30*time.Second {
		t.Errorf("expected default reset timeout to be 30s, got %v", config.ResetAfter)
	}
}

func TestCircuitState_String(t *testing.T) {
	tests := []struct {
		state    CircuitState
		expected string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(999), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.expected {
			t.Errorf("CircuitState(%d).String() = %q, expected %q", tt.state, got, tt.expected)
		}
	}
}

func TestCircuitBreaker_ConcurrentAccess(t *testing.T) {
	config := CircuitBreakerConfig{
		Threshold:  10,
		ResetAfter: 100 * time.Millisecond,
	}
	cb := NewCircuitBreaker(config)

	// Launch multiple goroutines that call Allow, RecordSuccess, and RecordFailure
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = cb.Allow()
				if j%2 == 0 {
					cb.RecordSuccess()
				} else {
					cb.RecordFailure()
				}
				_ = cb.State()
				_ = cb.ConsecutiveFailures()
			}
			done <- true
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// No assertion needed - the test passes if there's no race condition detected
	// Run with: go test -race
}
