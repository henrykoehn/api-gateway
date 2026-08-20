package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const claimsContextKey contextKey = "jwt_claims"

// ClaimsFromContext retrieves the validated JWT claims injected by
// Auth, if any were set on this request.
func ClaimsFromContext(ctx context.Context) (jwt.MapClaims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(jwt.MapClaims)
	return claims, ok
}

// Auth validates an HMAC-signed JWT bearer token, rejecting missing,
// malformed, or expired tokens with 401. On success, the parsed claims
// are injected into the request context for downstream use (see
// JWTSubjectKey).
func Auth(secret string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString, ok := bearerToken(r)
			if !ok {
				http.Error(w, "missing bearer token", http.StatusUnauthorized)
				return
			}

			token, err := jwt.Parse(tokenString, func(t *jwt.Token) (any, error) {
				if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				return []byte(secret), nil
			})
			if err != nil || !token.Valid {
				http.Error(w, "invalid or expired token", http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "invalid token claims", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), claimsContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	return strings.TrimPrefix(h, prefix), true
}

// JWTSubjectKey returns the authenticated JWT subject from context (set
// by Auth), falling back to ClientIPKey if no valid claims are present.
// Use this as a rate limiter KeyFunc on routes that require auth, so
// limits apply per authenticated user rather than per IP.
func JWTSubjectKey(r *http.Request) string {
	claims, ok := ClaimsFromContext(r.Context())
	if !ok {
		return ClientIPKey(r)
	}
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return ClientIPKey(r)
	}
	return sub
}
