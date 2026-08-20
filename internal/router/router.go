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
	"github.com/henrykoehn/api-gateway/internal/middleware"
	"github.com/henrykoehn/api-gateway/internal/proxy"
	"github.com/henrykoehn/api-gateway/internal/ratelimiter"
)

// idleBucketTTL bounds rate-limiter memory growth from clients seen once.
const idleBucketTTL = 5 * time.Minute

// Build constructs a ServeMux that routes each configured path prefix
// to its backend's reverse proxy, applying per-route rate limiting and
// circuit breaking where configured.
func Build(cfg *config.Config) (http.Handler, error) {
	mux := http.NewServeMux()

	for _, route := range cfg.Routes {
		var brk *breaker.Breaker
		if route.CircuitBreaker != nil {
			brk = breaker.New(route.Target, breaker.Config{
				FailureThreshold: route.CircuitBreaker.FailureThreshold,
				ResetTimeout:     secondsToDuration(route.CircuitBreaker.ResetTimeoutSeconds),
				SuccessThreshold: route.CircuitBreaker.SuccessThreshold,
			})

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
			go evictIdleBuckets(limiter)
			handler = middleware.RateLimit(limiter, keyFunc)(handler)
		}

		if route.AuthRequired {
			handler = middleware.Auth(cfg.Auth.JWTSecret)(handler)
		}

		mux.Handle(route.Path, http.StripPrefix(route.Path, handler))
	}

	return mux, nil
}

// evictIdleBuckets periodically prunes idle rate-limiter state. It runs
// for the lifetime of the process; there's only ever one gateway
// process per route, so it needs no explicit shutdown.
func evictIdleBuckets(limiter *ratelimiter.Limiter) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		limiter.EvictIdle(idleBucketTTL)
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
