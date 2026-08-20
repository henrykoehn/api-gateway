package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/henrykoehn/api-gateway/internal/breaker"
)

func TestCircuitBreaker_RejectsWhenOpen(t *testing.T) {
	b := breaker.New("test", breaker.Config{FailureThreshold: 1, ResetTimeout: time.Minute, SuccessThreshold: 1})
	b.ForceState(breaker.Open)

	handler := CircuitBreaker(b)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when breaker open, got %d", rec.Code)
	}
}

func TestCircuitBreaker_PassesThroughWhenClosed(t *testing.T) {
	b := breaker.New("test", breaker.Config{FailureThreshold: 1, ResetTimeout: time.Minute, SuccessThreshold: 1})

	called := false
	handler := CircuitBreaker(b)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK || !called {
		t.Fatalf("expected request to pass through when breaker closed, got status %d, called=%v", rec.Code, called)
	}
}
