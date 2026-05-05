package client

import (
	"sync"
	"time"
)

type cbState int

const (
	cbClosed   cbState = iota // normal operation
	cbOpen                    // failing fast, no requests allowed
	cbHalfOpen                // testing recovery with one request
)

// CircuitBreaker is a simple three-state machine: CLOSED → OPEN → HALF_OPEN → CLOSED.
// Not safe to copy after first use.
type CircuitBreaker struct {
	mu        sync.Mutex
	state     cbState
	failures  int
	threshold int
	openUntil time.Time
	timeout   time.Duration
}

func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		threshold: threshold,
		timeout:   timeout,
	}
}

// Allow returns true if a request should be attempted.
// In OPEN state, returns false until the timeout elapses (then transitions to HALF_OPEN).
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case cbClosed:
		return true
	case cbOpen:
		if time.Now().After(cb.openUntil) {
			cb.state = cbHalfOpen
			return true
		}
		return false
	case cbHalfOpen:
		// Allow only the one probe request; subsequent callers block until it resolves.
		return false
	}
	return false
}

// RecordSuccess resets the breaker to CLOSED on a successful call.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.state = cbClosed
}

// RecordFailure increments the failure counter and opens the circuit after threshold failures.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	if cb.failures >= cb.threshold {
		cb.state = cbOpen
		cb.openUntil = time.Now().Add(cb.timeout)
	}
}
