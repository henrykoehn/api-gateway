// Package router builds an http.Handler that dispatches requests to
// per-route backend proxies based on the loaded configuration.
package router

import (
	"net/http"
	"time"

	"github.com/henrykoehn/api-gateway/internal/config"
	"github.com/henrykoehn/api-gateway/internal/middleware"
	"github.com/henrykoehn/api-gateway/internal/proxy"
	"github.com/henrykoehn/api-gateway/internal/ratelimiter"
)

// idleBucketTTL bounds rate-limiter memory growth from clients seen once.
const idleBucketTTL = 5 * time.Minute

// Build constructs a ServeMux that routes each configured path prefix
// to its backend's reverse proxy, applying per-route rate limiting
// where configured.
func Build(cfg *config.Config) (http.Handler, error) {
	mux := http.NewServeMux()

	for _, route := range cfg.Routes {
		handler, err := proxy.New(route.Target)
		if err != nil {
			return nil, err
		}

		if route.RateLimit != nil {
			limiter := ratelimiter.New(float64(route.RateLimit.Burst), route.RateLimit.RequestsPerSecond)
			go evictIdleBuckets(limiter)
			handler = middleware.RateLimit(limiter, middleware.ClientIPKey)(handler)
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
