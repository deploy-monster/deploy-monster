package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

type requestIDKey struct{}
type traceIDKey struct{}

// RequestID generates a unique ID per request for tracing and debugging.
// Supports W3C Trace Context: if a valid traceparent header is present,
// the trace ID is extracted and propagated. Otherwise a new trace ID is generated.
// Sets X-Request-ID and traceparent in response headers.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use incoming X-Request-ID if provided, otherwise generate
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = core.GenerateID()
		}

		// Parse W3C traceparent header for distributed tracing
		traceID, parentID := parseTraceparent(r.Header.Get("traceparent"))
		if traceID == "" {
			traceID = generateTraceID()
			parentID = generateSpanID()
		}

		// Set response headers
		w.Header().Set("X-Request-ID", id)
		w.Header().Set("traceparent", "00-"+traceID+"-"+parentID+"-01")

		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		ctx = context.WithValue(ctx, traceIDKey{}, traceID)
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

// GetTraceID extracts the W3C trace ID from context.
func GetTraceID(ctx context.Context) string {
	id, _ := ctx.Value(traceIDKey{}).(string)
	return id
}

// parseTraceparent extracts trace-id and parent-id from a W3C traceparent header.
// Format: {version}-{trace-id}-{parent-id}-{trace-flags}
// Returns empty strings if the header is missing or malformed.
func parseTraceparent(header string) (traceID, parentID string) {
	if header == "" {
		return "", ""
	}
	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return "", ""
	}
	// version must be "00", trace-id 32 hex, parent-id 16 hex, flags 2 hex
	if parts[0] != "00" || len(parts[1]) != 32 || len(parts[2]) != 16 || len(parts[3]) != 2 {
		return "", ""
	}
	return parts[1], parts[2]
}

// generateTraceID returns a 16-byte (32 hex char) random trace ID.
func generateTraceID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}

// generateSpanID returns an 8-byte (16 hex char) random span/parent ID.
func generateSpanID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
