package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/henrykoehn/api-gateway/internal/ratelimiter"
)

func TestRateLimit_AllowsWithinLimitRejectsOverLimit(t *testing.T) {
	limiter := ratelimiter.New(1, 0) // capacity 1, no refill during the test
	handler := RateLimit(limiter, func(r *http.Request) string { return "test-client" })(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	)

	req := httptest.NewRequest(http.MethodGet, "/", nil)

	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request: expected 200, got %d", rec1.Code)
	}

	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request: expected 429, got %d", rec2.Code)
	}
	if rec2.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestClientIPKey_PrefersForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 10.0.0.1")
	req.RemoteAddr = "10.0.0.2:12345"

	if got := ClientIPKey(req); got != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %q", got)
	}
}

func TestClientIPKey_FallsBackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "192.0.2.1:54321"

	if got := ClientIPKey(req); got != "192.0.2.1" {
		t.Fatalf("expected 192.0.2.1, got %q", got)
	}
}
