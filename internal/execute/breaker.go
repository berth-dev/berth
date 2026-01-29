// Package execute implements the bead execution loop.
package execute

import (
	"sync"
)

// CircuitBreaker pauses execution after consecutive failures.
type CircuitBreaker struct {
	mu                  sync.Mutex
	ConsecutiveFailures int
	Threshold           int
	Paused              bool
}

// NewCircuitBreaker creates a circuit breaker with the given threshold.
func NewCircuitBreaker(threshold int) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 3 // default
	}
	return &CircuitBreaker{
		Threshold: threshold,
	}
}

// RecordFailure increments the failure counter.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.ConsecutiveFailures++
	if cb.ConsecutiveFailures >= cb.Threshold {
		cb.Paused = true
	}
}

// RecordSuccess resets the failure counter.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.ConsecutiveFailures = 0
	cb.Paused = false
}

// ShouldPause returns true if threshold exceeded.
func (cb *CircuitBreaker) ShouldPause() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.Paused
}

// Reset clears the circuit breaker state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.ConsecutiveFailures = 0
	cb.Paused = false
}

// GetConsecutiveFailures returns the current failure count (thread-safe).
func (cb *CircuitBreaker) GetConsecutiveFailures() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.ConsecutiveFailures
}

// SetConsecutiveFailures sets the failure count (used for restoring state).
func (cb *CircuitBreaker) SetConsecutiveFailures(count int) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.ConsecutiveFailures = count
	if cb.ConsecutiveFailures >= cb.Threshold {
		cb.Paused = true
	} else {
		cb.Paused = false
	}
}
