package llm

import (
	"sync"
	"time"
)

type State int

const (
	StateClosed   State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed: return "CLOSED"
	case StateOpen: return "OPEN"
	case StateHalfOpen: return "HALF_OPEN"
	default: return "UNKNOWN"
	}
}

type CircuitBreaker struct {
	mu          sync.RWMutex
	state       State
	failures    int
	threshold   int
	openUntil   time.Time
	openDuration time.Duration
	totalSuccess int64
	totalFailure int64
}

func NewCircuitBreaker(threshold int, openDuration time.Duration) *CircuitBreaker {
	if threshold <= 0 { threshold = 2 }
	if openDuration <= 0 { openDuration = 30 * time.Second }
	return &CircuitBreaker{state: StateClosed, threshold: threshold, openDuration: openDuration}
}

func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Now().After(cb.openUntil) {
			cb.state = StateHalfOpen // transition here
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return true
	}
}

func (cb *CircuitBreaker) MarkSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.totalSuccess++

	switch cb.state {
	case StateHalfOpen:
		cb.state = StateClosed
		cb.failures = 0
	case StateClosed:
		cb.failures = 0
	}
}

func (cb *CircuitBreaker) MarkFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.totalFailure++
	cb.failures++

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.threshold {
			cb.state = StateOpen
			cb.openUntil = time.Now().Add(cb.openDuration)
		}
	case StateHalfOpen:
		cb.state = StateOpen
		cb.openUntil = time.Now().Add(cb.openDuration)
	}
}

func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
