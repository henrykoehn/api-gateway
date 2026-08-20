package breaker

import (
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestBreaker(cfg Config) (*Breaker, *fakeClock) {
	clock := &fakeClock{t: time.Now()}
	b := New("test-backend", cfg)
	b.now = clock.now
	return b, clock
}

func TestBreaker_StartsClosed(t *testing.T) {
	b, _ := newTestBreaker(Config{FailureThreshold: 2, ResetTimeout: time.Second, SuccessThreshold: 1})
	if b.State() != Closed {
		t.Fatalf("expected initial state Closed, got %s", b.State())
	}
	if !b.Allow() {
		t.Fatal("expected Allow() true in closed state")
	}
}

func TestBreaker_TripsAfterConsecutiveFailures(t *testing.T) {
	b, _ := newTestBreaker(Config{FailureThreshold: 3, ResetTimeout: time.Second, SuccessThreshold: 1})

	b.RecordFailure()
	b.RecordFailure()
	if b.State() != Closed {
		t.Fatalf("expected still Closed after 2/3 failures, got %s", b.State())
	}

	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("expected Open after 3/3 failures, got %s", b.State())
	}
	if b.Allow() {
		t.Fatal("expected Allow() false immediately after tripping open")
	}
}

func TestBreaker_SuccessResetsFailureCount(t *testing.T) {
	b, _ := newTestBreaker(Config{FailureThreshold: 3, ResetTimeout: time.Second, SuccessThreshold: 1})

	b.RecordFailure()
	b.RecordFailure()
	b.RecordSuccess() // should reset the streak
	b.RecordFailure()
	b.RecordFailure()

	if b.State() != Closed {
		t.Fatalf("expected still Closed, failure streak should have reset on success, got %s", b.State())
	}
}

func TestBreaker_OpenToHalfOpenAfterResetTimeout(t *testing.T) {
	b, clock := newTestBreaker(Config{FailureThreshold: 1, ResetTimeout: 5 * time.Second, SuccessThreshold: 1})

	b.RecordFailure() // trips open
	if b.State() != Open {
		t.Fatalf("expected Open, got %s", b.State())
	}
	if b.Allow() {
		t.Fatal("expected Allow() false before reset timeout elapses")
	}

	clock.advance(5 * time.Second)
	if !b.Allow() {
		t.Fatal("expected Allow() true (half-open trial) after reset timeout elapses")
	}
	if b.State() != HalfOpen {
		t.Fatalf("expected HalfOpen after reset timeout elapses, got %s", b.State())
	}
}

func TestBreaker_HalfOpenClosesAfterSuccessThreshold(t *testing.T) {
	b, clock := newTestBreaker(Config{FailureThreshold: 1, ResetTimeout: time.Second, SuccessThreshold: 2})

	b.RecordFailure() // -> open
	clock.advance(time.Second)
	b.Allow() // -> half-open

	b.RecordSuccess()
	if b.State() != HalfOpen {
		t.Fatalf("expected still HalfOpen after 1/2 successes, got %s", b.State())
	}
	b.RecordSuccess()
	if b.State() != Closed {
		t.Fatalf("expected Closed after 2/2 successes, got %s", b.State())
	}
}

func TestBreaker_HalfOpenReopensOnFailure(t *testing.T) {
	b, clock := newTestBreaker(Config{FailureThreshold: 1, ResetTimeout: time.Second, SuccessThreshold: 2})

	b.RecordFailure() // -> open
	clock.advance(time.Second)
	b.Allow() // -> half-open

	b.RecordFailure()
	if b.State() != Open {
		t.Fatalf("expected Open again after failure in half-open, got %s", b.State())
	}
}

func TestBreaker_ForceStateOpenAndClosed(t *testing.T) {
	b, _ := newTestBreaker(Config{FailureThreshold: 5, ResetTimeout: time.Second, SuccessThreshold: 1})

	b.ForceState(Open)
	if b.State() != Open {
		t.Fatalf("expected Open after ForceState(Open), got %s", b.State())
	}

	b.ForceState(Closed)
	if b.State() != Closed {
		t.Fatalf("expected Closed after ForceState(Closed), got %s", b.State())
	}
}
