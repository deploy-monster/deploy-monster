package middleware

import (
	"context"
	"encoding/hex"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// parentIDKey is used to store the parent span ID from the W3C traceparent header.
type parentIDKey struct{}

// getParentID extracts the parent span ID that was parsed by RequestID middleware.
func getParentID(ctx context.Context) string {
	id, _ := ctx.Value(parentIDKey{}).(string)
	return id
}

// WithParentID stores the parent span ID in context.
func WithParentID(ctx context.Context, parentID string) context.Context {
	return context.WithValue(ctx, parentIDKey{}, parentID)
}

// Tracing returns middleware that starts an OpenTelemetry span for each request.
// It reads the trace context already parsed by RequestID middleware and creates
// a linked span. The span end function is stored in context so the caller can defer it.
//
// Usage in a handler:
//
//	ctx, endSpan := middleware.StartSpan(r.Context(), c.Core)
//	defer endSpan()
func Tracing(c *core.Core) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, endSpan := StartSpan(r.Context(), c,
				attribute.String("http.method", r.Method),
				attribute.String("http.url", r.URL.String()),
				attribute.String("http.host", r.Host),
			)
			defer endSpan()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// StartSpan looks up the tracer on the Core and, if tracing is
// configured, starts a new span linked to the W3C trace context
// already parsed by RequestID middleware (trace ID + parent ID).
//
// Usage in a handler:
//
//	ctx, endSpan := middleware.StartSpan(r.Context(), c.Core)
//	defer endSpan()
func StartSpan(ctx context.Context, c *core.Core, extraAttrs ...attribute.KeyValue) (context.Context, func()) {
	if c == nil || c.Tracer == nil {
		return ctx, func() {}
	}

	traceIDStr := GetTraceID(ctx)

	var opts []trace.SpanStartOption
	if len(extraAttrs) > 0 {
		opts = append(opts, trace.WithAttributes(extraAttrs...))
	}

	if traceIDStr != "" {
		var traceID trace.TraceID
		n, err := hex.DecodeString(traceIDStr[:32])
		if err == nil && len(n) == 16 {
			copy(traceID[:], n)
			opts = append(opts, trace.WithLinks(trace.Link{
				SpanContext: trace.NewSpanContext(trace.SpanContextConfig{
					TraceID:    traceID,
					Remote:     true,
					TraceFlags: 0x01,
				}),
			}))
		}
	}

	ctx, span := c.Tracer.Start(ctx, "http.request", opts...)
	return ctx, func() { span.End() }
}