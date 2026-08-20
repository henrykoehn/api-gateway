// Package router builds an http.Handler that dispatches requests to
// per-route backend proxies based on the loaded configuration.
package router

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/henrykoehn/api-gateway/internal/breaker"
	"github.com/henrykoehn/api-gateway/internal/config"
	"github.com/henrykoehn/api-gateway/internal/metrics"
	"github.com/henrykoehn/api-gateway/internal/middleware"
	"github.com/henrykoehn/api-gateway/internal/proxy"
	"github.com/henrykoehn/api-gateway/internal/ratelimiter"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// idleBucketTTL bounds rate-limiter memory growth from clients seen once.
const idleBucketTTL = 5 * time.Minute

// Reserved paths for the gateway's own observability endpoints. These
// are registered as exact matches, which net/http.ServeMux always
// prefers over a broader backend route prefix (e.g. "/"), so a
// catch-all route can never shadow them.
const (
	MetricsPath = "/metrics"
	HealthzPath = "/healthz"
)

// Build constructs a ServeMux that routes each configured path prefix
// to its backend's reverse proxy, applying per-route rate limiting,
// circuit breaking, and auth where configured, plus the gateway's own
// /metrics and /healthz endpoints.
func Build(cfg *config.Config) (http.Handler, error) {
	mux := http.NewServeMux()

	mux.Handle(MetricsPath, promhttp.Handler())
	mux.HandleFunc(HealthzPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	for _, route := range cfg.Routes {
		var brk *breaker.Breaker
		if route.CircuitBreaker != nil {
			brk = breaker.New(route.Target, breaker.Config{
				FailureThreshold: route.CircuitBreaker.FailureThreshold,
				ResetTimeout:     secondsToDuration(route.CircuitBreaker.ResetTimeoutSeconds),
				SuccessThreshold: route.CircuitBreaker.SuccessThreshold,
			})
			go sampleBreakerState(route.Target, brk)

			if hc := route.CircuitBreaker.HealthCheck; hc != nil {
				checker := breaker.NewHealthChecker(
					joinURL(route.Target, hc.Path),
					secondsToDuration(hc.IntervalSeconds),
					secondsToDuration(hc.TimeoutSeconds),
					brk,
				)
				go checker.Run(context.Background())
			}
		}

		handler, err := proxy.New(route.Target, brk)
		if err != nil {
			return nil, err
		}

		if brk != nil {
			handler = middleware.CircuitBreaker(brk)(handler)
		}

		if route.RateLimit != nil {
			keyFunc := middleware.ClientIPKey
			if route.AuthRequired {
				// Auth (wrapped below) runs before rate limiting, so
				// claims are already in context by the time this fires.
				keyFunc = middleware.JWTSubjectKey
			}
			limiter := ratelimiter.New(float64(route.RateLimit.Burst), route.RateLimit.RequestsPerSecond)
			go sampleActiveBuckets(route.Path, limiter)
			handler = middleware.RateLimit(limiter, keyFunc)(handler)
		}

		if route.AuthRequired {
			handler = middleware.Auth(cfg.Auth.JWTSecret)(handler)
		}

		handler = middleware.Metrics(route.Path)(handler)

		mux.Handle(route.Path, http.StripPrefix(route.Path, handler))
	}

	return mux, nil
}

// sampleActiveBuckets periodically evicts idle rate-limiter state and
// publishes the remaining bucket count as a gauge. It runs for the
// lifetime of the process, matching the one-gateway-process-per-route
// lifecycle, so it needs no explicit shutdown.
func sampleActiveBuckets(route string, limiter *ratelimiter.Limiter) {
	update := func() {
		limiter.EvictIdle(idleBucketTTL)
		metrics.RateLimiterActiveBuckets.WithLabelValues(route).Set(float64(limiter.BucketCount()))
	}
	update()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		update()
	}
}

// sampleBreakerState periodically publishes a breaker's state as a
// gauge, for the lifetime of the process.
func sampleBreakerState(backend string, brk *breaker.Breaker) {
	update := func() {
		metrics.CircuitBreakerState.WithLabelValues(backend).Set(float64(brk.State()))
	}
	update()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for range ticker.C {
		update()
	}
}

func secondsToDuration(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}

// joinURL concatenates a target base URL and a health check path
// without producing a double slash.
func joinURL(target, path string) string {
	return strings.TrimSuffix(target, "/") + "/" + strings.TrimPrefix(path, "/")
}
