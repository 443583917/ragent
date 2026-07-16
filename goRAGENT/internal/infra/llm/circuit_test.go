package llm

import (
	"testing"
	"time"
)

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 30*time.Second)

	if !cb.Allow() { t.Error("CLOSED state should allow requests") }
	cb.MarkFailure()
	if !cb.Allow() { t.Error("after 1 failure should still allow") }
	cb.MarkFailure()

	if cb.Allow() { t.Error("after 2 failures should be OPEN and deny") }
	if cb.State() != StateOpen { t.Errorf("expected OPEN, got %s", cb.State()) }
}

func TestCircuitBreaker_OpenToHalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 100*time.Millisecond)

	// Go to OPEN
	cb.MarkFailure()
	cb.MarkFailure()
	if cb.State() != StateOpen { t.Fatal("expected OPEN") }

	// Wait for OPEN_DURATION to expire
	time.Sleep(150 * time.Millisecond)

	// Should allow a probe request
	if !cb.Allow() { t.Error("after OPEN_DURATION should allow in HALF_OPEN") }
}

func TestCircuitBreaker_HalfOpenRecovery(t *testing.T) {
	cb := NewCircuitBreaker(2, 10*time.Millisecond)
	cb.MarkFailure(); cb.MarkFailure()
	time.Sleep(20 * time.Millisecond)

	// Probe succeeds
	cb.Allow()
	cb.MarkSuccess()

	if cb.State() != StateClosed {
		t.Errorf("probe success should recover to CLOSED, got %s", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenFailure(t *testing.T) {
	cb := NewCircuitBreaker(2, 10*time.Millisecond)
	cb.MarkFailure(); cb.MarkFailure()
	time.Sleep(20 * time.Millisecond)

	// Probe fails
	cb.Allow()
	cb.MarkFailure()

	if cb.State() != StateOpen {
		t.Errorf("probe failure should go back to OPEN, got %s", cb.State())
	}
}

func TestCircuitBreaker_SuccessResetsCount(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Minute)

	cb.MarkFailure()
	cb.MarkFailure() // 2 failures, not yet open

	cb.MarkSuccess() // success should reset counter

	// Only 1 failure after reset, should still be CLOSED
	if !cb.Allow() { t.Error("after 1 failure with threshold=3 should still allow") }
}
