package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/henrykoehn/api-gateway/internal/metrics"
)

// Metrics records request count and latency for route. It should wrap
// the full per-route handler chain (auth, rate limit, breaker, proxy)
// so it captures the outcome of every layer — a 401 or 429 shows up in
// gateway_requests_total just as a proxied 200 does.
func Metrics(route string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(rec, r)

			metrics.RequestsTotal.WithLabelValues(route, strconv.Itoa(rec.status)).Inc()
			metrics.RequestDuration.WithLabelValues(route).Observe(time.Since(start).Seconds())
		})
	}
}
