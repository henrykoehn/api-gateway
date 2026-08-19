// Package middleware provides composable http.Handler wrappers for the
// gateway's request pipeline (logging, recovery, auth, rate limiting).
package middleware

import "net/http"

// Middleware wraps an http.Handler to add behavior before/after it runs.
type Middleware func(http.Handler) http.Handler

// Chain applies mw to h in order, so the first middleware listed is the
// outermost — it runs first on the way in and last on the way out.
// Chain(h, A, B, C) behaves like A(B(C(h))).
func Chain(h http.Handler, mw ...Middleware) http.Handler {
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}
