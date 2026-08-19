// Package router builds an http.Handler that dispatches requests to
// per-route backend proxies based on the loaded configuration.
package router

import (
	"net/http"

	"github.com/henrykoehn/api-gateway/internal/config"
	"github.com/henrykoehn/api-gateway/internal/proxy"
)

// Build constructs a ServeMux that routes each configured path prefix
// to its backend's reverse proxy.
func Build(cfg *config.Config) (http.Handler, error) {
	mux := http.NewServeMux()

	for _, route := range cfg.Routes {
		handler, err := proxy.New(route.Target)
		if err != nil {
			return nil, err
		}
		mux.Handle(route.Path, http.StripPrefix(route.Path, handler))
	}

	return mux, nil
}
