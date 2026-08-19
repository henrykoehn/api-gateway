package middleware

import (
	"log/slog"
	"net/http"
)

// Recover catches panics from downstream handlers so a single bad
// request can't crash the gateway process, and returns a 500 instead.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "path", r.URL.Path, "error", err)
				w.WriteHeader(http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
