// Package ratelimiter implements a goroutine-safe, per-client token
// bucket rate limiter. It has no HTTP dependency so the algorithm can
// be unit tested without a server.
package ratelimiter

import (
	"sync"
	"time"
)

// bucket holds one client's token state.
type bucket struct {
	mu         sync.Mutex
	tokens     float64
	lastRefill time.Time
	lastSeen   time.Time
}

// Limiter tracks one token bucket per client key. capacity is the
// maximum burst size; refillPerSecond is the sustained allowed rate.
type Limiter struct {
	capacity        float64
	refillPerSecond float64
	now             func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
}

// New creates a Limiter allowing bursts up to capacity tokens,
// refilling at refillPerSecond tokens/second.
func New(capacity float64, refillPerSecond float64) *Limiter {
	return &Limiter{
		capacity:        capacity,
		refillPerSecond: refillPerSecond,
		now:             time.Now,
		buckets:         make(map[string]*bucket),
	}
}

// Allow reports whether a request from key is permitted right now,
// consuming one token if so.
func (l *Limiter) Allow(key string) bool {
	b := l.bucketFor(key)

	b.mu.Lock()
	defer b.mu.Unlock()

	now := l.now()
	if elapsed := now.Sub(b.lastRefill).Seconds(); elapsed > 0 {
		b.tokens = min(l.capacity, b.tokens+elapsed*l.refillPerSecond)
		b.lastRefill = now
	}
	b.lastSeen = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *Limiter) bucketFor(key string) *bucket {
	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, lastRefill: l.now(), lastSeen: l.now()}
		l.buckets[key] = b
	}
	return b
}

// BucketCount returns the number of distinct clients currently tracked,
// for observability (see gateway_rate_limiter_active_buckets).
func (l *Limiter) BucketCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// EvictIdle removes buckets untouched for longer than maxIdle, bounding
// memory growth from clients seen once. Intended to be called
// periodically from a background goroutine.
func (l *Limiter) EvictIdle(maxIdle time.Duration) {
	cutoff := l.now().Add(-maxIdle)

	l.mu.Lock()
	defer l.mu.Unlock()

	for key, b := range l.buckets {
		b.mu.Lock()
		idle := b.lastSeen.Before(cutoff)
		b.mu.Unlock()
		if idle {
			delete(l.buckets, key)
		}
	}
}
