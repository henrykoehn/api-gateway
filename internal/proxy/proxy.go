// Package proxy builds reverse proxies that forward requests to backend targets.
package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/henrykoehn/api-gateway/internal/breaker"
)

// New builds a reverse proxy that forwards all requests to target.
// The returned handler rewrites the request's scheme/host to target's
// and logs proxy-level errors (e.g. connection refused) instead of
// letting them crash the server.
//
// If brk is non-nil, connection-level errors and 5xx responses report
// a failure to it, and any other response reports a success — this is
// the breaker's passive signal, driven by live traffic.
func New(target string, brk *breaker.Breaker) (http.Handler, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	rp := &httputil.ReverseProxy{
		// Rewrite is the Director's replacement (Director is deprecated
		// as of Go 1.26). SetURL also rewrites the outbound Host header
		// to match the target, which is the behavior we want here.
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(targetURL)
		},
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy error", "target", target, "path", r.URL.Path, "error", err)
		if brk != nil {
			brk.RecordFailure()
		}
		w.WriteHeader(http.StatusBadGateway)
	}

	if brk != nil {
		rp.ModifyResponse = func(resp *http.Response) error {
			if resp.StatusCode >= 500 {
				brk.RecordFailure()
			} else {
				brk.RecordSuccess()
			}
			return nil
		}
	}

	return rp, nil
}
