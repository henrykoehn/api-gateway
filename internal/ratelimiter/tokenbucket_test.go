package ratelimiter

import (
	"sync"
	"testing"
	"time"
)

// fakeClock lets tests control elapsed time deterministically instead
// of sleeping real wall-clock time.
type fakeClock struct {
	t time.Time
}

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func TestLimiter_AllowsBurstUpToCapacity(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	l := New(3, 1)
	l.now = clock.now

	for i := 0; i < 3; i++ {
		if !l.Allow("client-a") {
			t.Fatalf("request %d: expected allowed within burst capacity", i+1)
		}
	}
	if l.Allow("client-a") {
		t.Fatal("expected 4th request rejected, bucket should be empty")
	}
}

func TestLimiter_RefillsOverTime(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	l := New(1, 1) // capacity 1, refills 1 token/sec
	l.now = clock.now

	if !l.Allow("client-a") {
		t.Fatal("expected first request allowed")
	}
	if l.Allow("client-a") {
		t.Fatal("expected immediate second request rejected")
	}

	clock.advance(1 * time.Second)
	if !l.Allow("client-a") {
		t.Fatal("expected request allowed after 1s refill")
	}
}

func TestLimiter_PartialRefillDoesNotAllow(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	l := New(1, 1)
	l.now = clock.now

	l.Allow("client-a")
	clock.advance(500 * time.Millisecond) // only half a token refilled
	if l.Allow("client-a") {
		t.Fatal("expected request rejected, only 0.5 tokens refilled")
	}
}

func TestLimiter_TracksClientsIndependently(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	l := New(1, 1)
	l.now = clock.now

	if !l.Allow("client-a") {
		t.Fatal("client-a first request should be allowed")
	}
	if !l.Allow("client-b") {
		t.Fatal("client-b should have its own bucket, independent of client-a")
	}
}

func TestLimiter_EvictIdle(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	l := New(1, 1)
	l.now = clock.now

	l.Allow("client-a")
	if len(l.buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %d", len(l.buckets))
	}

	clock.advance(10 * time.Minute)
	l.EvictIdle(5 * time.Minute)

	if len(l.buckets) != 0 {
		t.Fatalf("expected idle bucket evicted, got %d remaining", len(l.buckets))
	}
}

func TestLimiter_ConcurrentAccessIsRaceFree(t *testing.T) {
	l := New(1000, 1000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Allow("shared-client")
		}()
	}
	wg.Wait()
	// Correctness here is enforced by `go test -race`; this test just
	// exercises concurrent access to the same bucket.
}
