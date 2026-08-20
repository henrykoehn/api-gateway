package breaker

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealthChecker_ForcesOpenOnFailingBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	b := New("backend", Config{FailureThreshold: 100, ResetTimeout: time.Second, SuccessThreshold: 1})
	checker := NewHealthChecker(backend.URL, time.Hour, time.Second, b)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker.check(ctx)

	if b.State() != Open {
		t.Fatalf("expected breaker forced Open by failing health check, got %s", b.State())
	}
}

func TestHealthChecker_ForcesClosedOnHealthyBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	b := New("backend", Config{FailureThreshold: 1, ResetTimeout: time.Second, SuccessThreshold: 1})
	b.ForceState(Open) // start unhealthy

	checker := NewHealthChecker(backend.URL, time.Hour, time.Second, b)
	checker.check(context.Background())

	if b.State() != Closed {
		t.Fatalf("expected breaker forced Closed by healthy check, got %s", b.State())
	}
}

func TestHealthChecker_ForcesOpenOnUnreachableBackend(t *testing.T) {
	b := New("backend", Config{FailureThreshold: 100, ResetTimeout: time.Second, SuccessThreshold: 1})
	// Port 1 is reserved and nothing will ever be listening there.
	checker := NewHealthChecker("http://127.0.0.1:1", time.Hour, 100*time.Millisecond, b)

	checker.check(context.Background())

	if b.State() != Open {
		t.Fatalf("expected breaker forced Open on connection failure, got %s", b.State())
	}
}

func TestHealthChecker_RunPollsUntilCanceled(t *testing.T) {
	var hits int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	b := New("backend", Config{FailureThreshold: 1, ResetTimeout: time.Second, SuccessThreshold: 1})
	checker := NewHealthChecker(backend.URL, 10*time.Millisecond, time.Second, b)

	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Millisecond)
	defer cancel()
	checker.Run(ctx)

	// Expect the immediate check plus a handful of ticks (~5), not zero.
	if atomic.LoadInt32(&hits) < 2 {
		t.Fatalf("expected at least 2 health check hits over 55ms at 10ms interval, got %d", hits)
	}
}
