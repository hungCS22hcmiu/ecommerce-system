package client

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestCircuitBreaker_FullCycle exercises the entire CLOSED → OPEN → HALF_OPEN → CLOSED
// state machine using a short 50ms timeout so the test runs in ~100ms total.
func TestCircuitBreaker_FullCycle(t *testing.T) {
	cb := NewCircuitBreaker(5, 50*time.Millisecond)

	// CLOSED — Allow returns true; failures haven't reached threshold yet.
	for i := 0; i < 4; i++ {
		assert.True(t, cb.Allow(), "CLOSED state must allow request %d", i)
		cb.RecordFailure()
	}

	// 5th failure trips the breaker.
	assert.True(t, cb.Allow(), "5th request still allowed before recording failure")
	cb.RecordFailure()

	// OPEN — Allow returns false until the cool-down elapses.
	assert.False(t, cb.Allow(), "OPEN state must fast-fail")
	assert.False(t, cb.Allow(), "OPEN state still fast-fails on subsequent calls")

	// Wait for the cool-down window to expire.
	time.Sleep(60 * time.Millisecond)

	// HALF_OPEN — the first Allow after cool-down returns true (probe).
	assert.True(t, cb.Allow(), "HALF_OPEN must allow the probe request")
	// Subsequent Allow calls in HALF_OPEN return false until probe resolves.
	assert.False(t, cb.Allow(), "HALF_OPEN must block subsequent callers until probe resolves")

	// Probe succeeds → CLOSED.
	cb.RecordSuccess()
	assert.True(t, cb.Allow(), "after RecordSuccess the breaker is CLOSED again")

	// And failures counter must be reset — a single new failure must NOT immediately trip OPEN.
	cb.RecordFailure()
	assert.True(t, cb.Allow(), "failures counter resets after RecordSuccess; one failure is not enough to re-open")
}

// TestCircuitBreaker_HalfOpen_ProbeFails_ReturnsToOpen verifies that a failed probe
// drives HALF_OPEN back to OPEN (not CLOSED).
func TestCircuitBreaker_HalfOpen_ProbeFails_ReturnsToOpen(t *testing.T) {
	cb := NewCircuitBreaker(3, 50*time.Millisecond)

	// Drive to OPEN.
	for i := 0; i < 3; i++ {
		cb.Allow()
		cb.RecordFailure()
	}
	assert.False(t, cb.Allow(), "OPEN")

	// Wait, then send the probe.
	time.Sleep(60 * time.Millisecond)
	assert.True(t, cb.Allow(), "HALF_OPEN allows probe")

	// Probe fails — RecordFailure must push back to OPEN (failures ≥ threshold).
	cb.RecordFailure()
	assert.False(t, cb.Allow(), "after probe failure, breaker is OPEN again")
}

// TestCircuitBreaker_BelowThreshold_StaysClosed verifies that failures below the
// threshold do not open the breaker.
func TestCircuitBreaker_BelowThreshold_StaysClosed(t *testing.T) {
	cb := NewCircuitBreaker(5, 1*time.Second)
	for i := 0; i < 4; i++ {
		cb.Allow()
		cb.RecordFailure()
	}
	assert.True(t, cb.Allow(), "4 failures < threshold(5) — must stay CLOSED")
}
