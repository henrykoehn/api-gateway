package middleware

import (
	"net/http"

	"github.com/henrykoehn/api-gateway/internal/breaker"
)

// CircuitBreaker rejects requests with 503 immediately, without
// attempting the backend call, whenever b is open. This must wrap the
// proxy handler directly (innermost in the chain) so a tripped breaker
// short-circuits before any backend I/O happens.
func CircuitBreaker(b *breaker.Breaker) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !b.Allow() {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
