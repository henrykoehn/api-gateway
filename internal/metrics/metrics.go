// Package metrics defines the gateway's Prometheus collectors. They
// are package-level (via promauto) since there's exactly one gateway
// process and one registry per process — no need to thread a registry
// through every layer that wants to record something.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// RequestsTotal counts requests by route and final status code,
	// after all middleware (auth, rate limit, breaker) has run.
	RequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gateway_requests_total",
			Help: "Total requests handled, labeled by route and status code.",
		},
		[]string{"route", "status"},
	)

	// RequestDuration observes end-to-end request latency by route.
	RequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gateway_request_duration_seconds",
			Help:    "Request latency in seconds, labeled by route.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"route"},
	)

	// CircuitBreakerState reports each backend's breaker state:
	// 0=closed, 1=open, 2=half_open.
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_circuit_breaker_state",
			Help: "Circuit breaker state per backend (0=closed, 1=open, 2=half_open).",
		},
		[]string{"backend"},
	)

	// RateLimiterActiveBuckets reports how many distinct clients each
	// route's rate limiter currently tracks.
	RateLimiterActiveBuckets = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "gateway_rate_limiter_active_buckets",
			Help: "Distinct clients currently tracked by the rate limiter, labeled by route.",
		},
		[]string{"route"},
	)
)
