package llm

import (
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the current state of the circuit breaker.
type CircuitState int

const (
	// CircuitClosed means the circuit is operational and requests flow through.
	CircuitClosed CircuitState = iota
	// CircuitOpen means the circuit has tripped due to failures and requests are blocked.
	CircuitOpen
	// CircuitHalfOpen means the circuit is testing if the service has recovered.
	CircuitHalfOpen
)

// String returns a human-readable string for the circuit state.
func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig holds configuration for the circuit breaker.
type CircuitBreakerConfig struct {
	// Threshold is the number of consecutive failures before the circuit trips.
	Threshold int
	// ResetAfter is the duration to wait before attempting to close the circuit again.
	ResetAfter time.Duration
}

// DefaultCircuitBreakerConfig returns sensible defaults for the circuit breaker.
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		Threshold:  5,              // Trip after 5 consecutive failures
		ResetAfter: 30 * time.Second, // Try again after 30 seconds
	}
}

// CircuitBreaker implements the circuit breaker pattern for LLM calls.
// It trips open after N consecutive failures and resets after a timeout period.
type CircuitBreaker struct {
	mu               sync.RWMutex
	consecutiveFails int
	threshold        int
	resetAfter       time.Duration
	lastFailure      time.Time
	state            CircuitState
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration.
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		threshold:  config.Threshold,
		resetAfter: config.ResetAfter,
		state:      CircuitClosed,
	}
}

// Allow returns true if the circuit breaker allows a request to proceed.
// It transitions to half-open state after the reset timeout expires.
func (cb *CircuitBreaker) Allow() (bool, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true, nil
	case CircuitOpen:
		// Check if enough time has passed to try again
		if time.Since(cb.lastFailure) > cb.resetAfter {
			// Transition to half-open and allow one request through
			cb.state = CircuitHalfOpen
			return true, nil
		}
		return false, fmt.Errorf("circuit breaker open: LLM provider appears to be down (failed %d times, last failure %v ago)",
			cb.consecutiveFails, time.Since(cb.lastFailure).Round(time.Second))
	case CircuitHalfOpen:
		// Already have a test request in flight, reject additional requests
		return false, fmt.Errorf("circuit breaker half-open: testing if LLM provider has recovered")
	default:
		return false, fmt.Errorf("circuit breaker in unknown state: %v", cb.state)
	}
}

// RecordSuccess resets the failure count and closes the circuit.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails = 0
	cb.state = CircuitClosed
}

// RecordFailure increments the failure count and trips the circuit if threshold is reached.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails++
	cb.lastFailure = time.Now()

	// If in half-open, transition back to open
	if cb.state == CircuitHalfOpen {
		cb.state = CircuitOpen
		return
	}

	// If in closed state, check if we should trip
	if cb.consecutiveFails >= cb.threshold {
		cb.state = CircuitOpen
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// ConsecutiveFailures returns the current count of consecutive failures.
func (cb *CircuitBreaker) ConsecutiveFailures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.consecutiveFails
}

// Reset manually resets the circuit breaker to closed state.
// This should be used sparingly, typically only for testing or manual intervention.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.consecutiveFails = 0
	cb.state = CircuitClosed
}
