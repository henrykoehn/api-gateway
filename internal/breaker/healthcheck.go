package breaker

import (
	"context"
	"log/slog"
	"net/http"
	"time"
)

// HealthChecker periodically polls a backend's health endpoint and
// updates a Breaker directly. This is the breaker's active signal,
// complementing the passive one (RecordSuccess/RecordFailure driven by
// live traffic) — it can detect and react to an outage or a recovery
// without waiting for real requests to hit the backend.
type HealthChecker struct {
	url      string
	interval time.Duration
	breaker  *Breaker
	client   *http.Client
}

// NewHealthChecker polls url every interval, using timeout as both the
// HTTP client timeout and the per-check context deadline.
func NewHealthChecker(url string, interval, timeout time.Duration, breaker *Breaker) *HealthChecker {
	return &HealthChecker{
		url:      url,
		interval: interval,
		breaker:  breaker,
		client:   &http.Client{Timeout: timeout},
	}
}

// Run polls until ctx is canceled, checking once immediately and then
// on every tick thereafter.
func (h *HealthChecker) Run(ctx context.Context) {
	h.check(ctx)

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			h.check(ctx)
		}
	}
}

func (h *HealthChecker) check(ctx context.Context) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.url, nil)
	if err != nil {
		slog.Error("health check request build failed", "url", h.url, "error", err)
		return
	}

	resp, err := h.client.Do(req)
	if err != nil {
		slog.Warn("health check failed", "url", h.url, "error", err)
		h.breaker.ForceState(Open)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		h.breaker.ForceState(Closed)
		return
	}

	slog.Warn("health check unhealthy status", "url", h.url, "status", resp.StatusCode)
	h.breaker.ForceState(Open)
}
