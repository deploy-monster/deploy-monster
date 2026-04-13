package middleware

import (
	"context"
	"net/http"
	"time"
)

// Timeout adds a context deadline to each request.
// If the handler takes longer than the timeout, the context is canceled.
func Timeout(duration time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), duration)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
