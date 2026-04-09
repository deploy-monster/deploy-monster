package middleware

import (
	"context"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

type requestIDKey struct{}

// RequestID generates a unique ID per request for tracing and debugging.
// Sets X-Request-ID in response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use incoming X-Request-ID if provided, otherwise generate
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = core.GenerateID()
		}

		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		// Also set as event correlation ID so events emitted during
		// this request share the same trace.
		ctx = core.WithCorrelationID(ctx, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID extracts the request ID from context.
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}
