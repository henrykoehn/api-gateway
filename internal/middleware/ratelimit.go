package middleware

import (
	"net"
	"net/http"
	"strings"

	"github.com/henrykoehn/api-gateway/internal/ratelimiter"
)

// KeyFunc extracts a rate-limit identity (e.g. client IP or, once auth
// is wired up, the authenticated subject) from a request.
type KeyFunc func(*http.Request) string

// ClientIPKey identifies a client by IP, preferring the first hop in
// X-Forwarded-For (as set by a trusted upstream load balancer) and
// falling back to the raw connection's remote address.
func ClientIPKey(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if idx := strings.IndexByte(fwd, ','); idx != -1 {
			return strings.TrimSpace(fwd[:idx])
		}
		return strings.TrimSpace(fwd)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimit rejects requests over limiter's rate with 429, identifying
// the client via keyFunc. When JWT auth is added (phase 5), keyFunc
// should prefer the authenticated subject over the raw IP.
func RateLimit(limiter *ratelimiter.Limiter, keyFunc KeyFunc) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow(keyFunc(r)) {
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
