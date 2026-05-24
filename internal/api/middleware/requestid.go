package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	corepkg "github.com/deploy-monster/deploy-monster/internal/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}
type traceIDKey struct{}
type tracerSpanKey struct{}

var tpKey tracerSpanKey

// RequestID generates a unique ID per request for tracing and debugging.
// Supports W3C Trace Context: if a valid traceparent header is present,
// the trace ID is extracted and propagated. Otherwise a new trace ID is generated.
// Sets X-Request-ID and traceparent in response headers.
// When a Tracer is available on core.Core, also creates an OpenTelemetry span.
func RequestID(c *corepkg.Core) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := r.Header.Get("X-Request-ID")
			if id == "" {
				id = corepkg.GenerateID()
			}

			traceID, parentID := parseTraceparent(r.Header.Get("traceparent"))
			if traceID == "" {
				traceID = generateTraceID()
				parentID = generateSpanID()
			}

			w.Header().Set("X-Request-ID", id)
			w.Header().Set("traceparent", "00-"+traceID+"-"+parentID+"-01")

			ctx := context.WithValue(r.Context(), requestIDKey{}, id)
			ctx = context.WithValue(ctx, traceIDKey{}, traceID)
			ctx = corepkg.WithCorrelationID(ctx, id)

			// Create an OpenTelemetry span when a tracer is available.
			if c.Tracer != nil {
				spanName := r.Method + " " + r.URL.Path
				opts := []trace.SpanStartOption{
					trace.WithSpanKind(trace.SpanKindServer),
					trace.WithAttributes(
						attribute.String("http.method", r.Method),
						attribute.String("http.url", r.URL.String()),
						attribute.String("http.route", r.URL.Path),
						attribute.String("request_id", id),
						attribute.String("trace_id", traceID),
					),
				}
				// If we have an incoming traceparent, link to it as the parent.
				if parentID != "" {
					// Build a non-exporting span context from the parsed traceparent.
					tid, _ := trace.TraceIDFromHex(traceID)
					sid, _ := trace.SpanIDFromHex(parentID)
					parentSC := trace.NewSpanContext(trace.SpanContextConfig{
						TraceID:    tid,
						SpanID:     sid,
						TraceFlags: trace.FlagsSampled,
						Remote:     true,
					})
					opts = append(opts, trace.WithLinks(trace.Link{SpanContext: parentSC}))
				}
				var span trace.Span
				ctx, span = c.Tracer.Start(ctx, spanName, opts...)
				defer span.End()
				ctx = context.WithValue(ctx, tpKey, span)
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
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
