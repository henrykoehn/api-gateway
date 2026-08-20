package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/henrykoehn/api-gateway/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_RecordsRequestCountByRouteAndStatus(t *testing.T) {
	route := "/metrics-test-route/"
	handler := Metrics(route)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))

	got := testutil.ToFloat64(metrics.RequestsTotal.WithLabelValues(route, "418"))
	if got != 2 {
		t.Fatalf("expected 2 requests recorded for route=%s status=418, got %v", route, got)
	}
}
