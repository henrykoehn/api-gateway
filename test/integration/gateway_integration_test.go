// Package integration drives real HTTP requests through the fully
// assembled gateway handler (router.Build's output), exercising auth,
// rate limiting, and circuit breaking together against real
// httptest backends — the closest thing to an end-to-end test without
// actually binding a port.
package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/henrykoehn/api-gateway/internal/config"
	"github.com/henrykoehn/api-gateway/internal/router"
)

const testJWTSecret = "integration-test-secret"

func signToken(t *testing.T) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "integration-user",
		"exp": time.Now().Add(time.Hour).Unix(),
	}).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return tok
}

func TestGateway_ProxiesToBackend(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from backend"))
	}))
	defer backend.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Addr: ":0"},
		Routes: []config.RouteConfig{
			{Path: "/api/plain/", Target: backend.URL},
		},
	}

	handler, err := router.Build(cfg)
	if err != nil {
		t.Fatalf("router.Build: %v", err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/plain/anything", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "hello from backend" {
		t.Fatalf("expected proxied body, got %q", rec.Body.String())
	}
}

func TestGateway_EnforcesAuthAndRateLimitTogether(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Addr: ":0"},
		Auth:   config.AuthConfig{JWTSecret: testJWTSecret},
		Routes: []config.RouteConfig{
			{
				Path:         "/api/secure/",
				Target:       backend.URL,
				AuthRequired: true,
				RateLimit:    &config.RateLimitConfig{RequestsPerSecond: 0.001, Burst: 2},
			},
		},
	}

	handler, err := router.Build(cfg)
	if err != nil {
		t.Fatalf("router.Build: %v", err)
	}

	// No token at all -> 401, and shouldn't consume a rate-limit token
	// (auth runs before rate limiting).
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/secure/x", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", rec.Code)
	}

	token := signToken(t)
	authedReq := func() *http.Request {
		r := httptest.NewRequest(http.MethodGet, "/api/secure/x", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		return r
	}

	// Burst of 2 should succeed.
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, authedReq())
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200 within burst, got %d", i+1, rec.Code)
		}
	}

	// Third should be rate limited.
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, authedReq())
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 after burst exhausted, got %d", rec.Code)
	}
}

func TestGateway_CircuitBreakerFastFailsAfterRepeatedFailures(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer backend.Close()

	cfg := &config.Config{
		Server: config.ServerConfig{Addr: ":0"},
		Routes: []config.RouteConfig{
			{
				Path:   "/api/flaky/",
				Target: backend.URL,
				CircuitBreaker: &config.BreakerConfig{
					FailureThreshold:    2,
					ResetTimeoutSeconds: 60, // long enough not to recover mid-test
					SuccessThreshold:    1,
				},
			},
		},
	}

	handler, err := router.Build(cfg)
	if err != nil {
		t.Fatalf("router.Build: %v", err)
	}

	req := func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/flaky/x", nil) }

	// First 2 requests reach the backend directly, which forwards its
	// real 500 response as-is (502 is reserved for connection-level
	// failures, not a backend responding normally with a 5xx body).
	for i := 0; i < 2; i++ {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req())
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("request %d: expected 500 forwarded from backend, got %d", i+1, rec.Code)
		}
	}

	// Breaker should now be open: fast-fail with 503, not another 502.
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req())
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 once breaker trips, got %d", rec.Code)
	}
}
