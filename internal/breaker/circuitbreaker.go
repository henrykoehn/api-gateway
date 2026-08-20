// Package breaker implements a goroutine-safe three-state circuit
// breaker (closed/open/half-open), decoupled from HTTP so the state
// machine can be unit tested without a server.
package breaker

import (
	"log/slog"
	"sync"
	"time"
)

// State is one of the breaker's three states.
type State int

const (
	// Closed: requests flow through normally.
	Closed State = iota
	// Open: requests are rejected immediately without attempting the call.
	Open
	// HalfOpen: a limited trial of requests is allowed through to test recovery.
	HalfOpen
)

func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Config controls when the breaker trips and when it recovers.
type Config struct {
	// FailureThreshold is the number of consecutive failures in the
	// closed state that trips the breaker to open.
	FailureThreshold int
	// ResetTimeout is how long the breaker stays open before allowing
	// a half-open trial request.
	ResetTimeout time.Duration
	// SuccessThreshold is the number of consecutive successes in the
	// half-open state required to close the breaker again.
	SuccessThreshold int
}

// Breaker is a goroutine-safe circuit breaker for one backend.
type Breaker struct {
	name string
	cfg  Config
	now  func() time.Time

	mu               sync.RWMutex
	state            State
	consecutiveFails int
	consecutiveOK    int
	openedAt         time.Time
}

// New creates a Breaker starting in the closed state. name is used only
// for log context (e.g. the backend's target URL).
func New(name string, cfg Config) *Breaker {
	return &Breaker{name: name, cfg: cfg, now: time.Now, state: Closed}
}

// Allow reports whether a request should be attempted right now. If the
// breaker is open and the reset timeout has elapsed, it transitions to
// half-open and allows a single trial request through.
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == Open && b.now().Sub(b.openedAt) >= b.cfg.ResetTimeout {
		b.setState(HalfOpen)
		b.consecutiveOK = 0
	}

	return b.state != Open
}

// RecordSuccess reports that a call to the backend succeeded.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case HalfOpen:
		b.consecutiveOK++
		if b.consecutiveOK >= b.cfg.SuccessThreshold {
			b.setState(Closed)
			b.consecutiveFails = 0
			b.consecutiveOK = 0
		}
	case Closed:
		b.consecutiveFails = 0
	}
}

// RecordFailure reports that a call to the backend failed.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case HalfOpen:
		b.trip()
	case Closed:
		b.consecutiveFails++
		if b.consecutiveFails >= b.cfg.FailureThreshold {
			b.trip()
		}
	}
}

// ForceState sets the breaker's state directly, used by the active
// health checker so a confirmed-down (or confirmed-recovered) backend
// updates the breaker even before live traffic would have.
func (b *Breaker) ForceState(s State) {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch {
	case s == Open && b.state != Open:
		b.trip()
	case s == Closed && b.state != Closed:
		b.setState(Closed)
		b.consecutiveFails = 0
		b.consecutiveOK = 0
	}
}

// State returns the breaker's current state, for observability.
func (b *Breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}

// trip transitions to open. Callers must hold b.mu.
func (b *Breaker) trip() {
	b.setState(Open)
	b.openedAt = b.now()
	b.consecutiveFails = 0
	b.consecutiveOK = 0
}

// setState logs and applies a state transition. Callers must hold b.mu.
func (b *Breaker) setState(s State) {
	if s == b.state {
		return
	}
	slog.Info("circuit breaker state change", "backend", b.name, "from", b.state, "to", s)
	b.state = s
}
