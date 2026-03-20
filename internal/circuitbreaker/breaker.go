package circuitbreaker

import (
	"sync"
	"time"
)

type State string

const (
	StateClosed   State = "closed"
	StateOpen     State = "open"
	StateHalfOpen State = "half_open"
)

type Breaker struct {
	mu                       sync.RWMutex
	failureThreshold         int
	openInterval             time.Duration
	recoverySuccessThreshold int
	failures                 int
	successes                int
	state                    State
	openedAt                 time.Time
}

func New(failureThreshold int, openInterval time.Duration, recoverySuccessThreshold int) *Breaker {
	return &Breaker{
		failureThreshold:         failureThreshold,
		openInterval:             openInterval,
		recoverySuccessThreshold: recoverySuccessThreshold,
		state:                    StateClosed,
	}
}

func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.currentStateLocked() {
	case StateHalfOpen:
		b.state = StateOpen
		b.openedAt = time.Now()
		b.successes = 0
		return
	case StateOpen:
		return
	}

	b.failures++
	if b.failures >= b.failureThreshold {
		b.state = StateOpen
		b.openedAt = time.Now()
		b.failures = 0
	}
}

func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.currentStateLocked() {
	case StateHalfOpen:
		b.successes++
		if b.successes >= b.recoverySuccessThreshold {
			b.state = StateClosed
			b.successes = 0
			b.failures = 0
		}
	default:
		b.failures = 0
	}
}

func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.currentStateLocked()
}

func (b *Breaker) currentStateLocked() State {
	if b.state == StateOpen && time.Since(b.openedAt) >= b.openInterval {
		b.state = StateHalfOpen
	}

	return b.state
}
