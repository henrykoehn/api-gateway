// Command mock-backend is a trivial HTTP service used only to demo the
// gateway in docker-compose. It is not part of the gateway itself — a
// real deployment points routes at real backends (e.g. an ML inference
// API), not this.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"sync/atomic"
)

var unhealthy atomic.Bool

func main() {
	name := envOr("BACKEND_NAME", "mock-backend")
	port := envOr("PORT", "9000")

	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if unhealthy.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"backend": name, "error": "simulated failure"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"backend": name, "path": r.URL.Path})
	})

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if unhealthy.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// /fail and /recover let the compose demo simulate an outage on
	// demand (curl them directly, bypassing the gateway) to show the
	// circuit breaker trip and recover.
	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) {
		unhealthy.Store(true)
		_, _ = w.Write([]byte("backend now failing\n"))
	})
	mux.HandleFunc("/recover", func(w http.ResponseWriter, r *http.Request) {
		unhealthy.Store(false)
		_, _ = w.Write([]byte("backend recovered\n"))
	})

	log.Printf("mock backend %q listening on :%s", name, port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
