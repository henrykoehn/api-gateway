// Package proxy builds reverse proxies that forward requests to backend targets.
package proxy

import (
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

// New builds a reverse proxy that forwards all requests to target.
// The returned handler rewrites the request's scheme/host to target's
// and logs proxy-level errors (e.g. connection refused) instead of
// letting them crash the server.
func New(target string) (http.Handler, error) {
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	rp := httputil.NewSingleHostReverseProxy(targetURL)

	origDirector := rp.Director
	rp.Director = func(r *http.Request) {
		origDirector(r)
		r.Host = targetURL.Host
	}

	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		slog.Error("proxy error", "target", target, "path", r.URL.Path, "error", err)
		w.WriteHeader(http.StatusBadGateway)
	}

	return rp, nil
}
