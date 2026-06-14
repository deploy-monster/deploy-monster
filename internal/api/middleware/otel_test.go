package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace/noop"
)

func TestStartSpanNoTracerIsNoop(t *testing.T) {
	ctx := context.Background()
	got, end := StartSpan(ctx, nil)
	end()
	if got != ctx {
		t.Fatal("nil core should return original context")
	}

	c := &core.Core{}
	got, end = StartSpan(ctx, c)
	end()
	if got != ctx {
		t.Fatal("nil tracer should return original context")
	}
}

func TestStartSpanWithTraceIDAndAttributes(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	ctx := context.Background()
	ctx = context.WithValue(ctx, traceIDKey{}, "0123456789abcdef0123456789abcdef")
	ctx = WithParentID(ctx, "0123456789abcdef")

	got, end := StartSpan(ctx, &core.Core{Tracer: tracer}, attribute.String("k", "v"))
	defer end()
	if got == nil {
		t.Fatal("StartSpan returned nil context")
	}
}

func TestTracingMiddlewareWrapsRequest(t *testing.T) {
	tracer := noop.NewTracerProvider().Tracer("test")
	c := &core.Core{Tracer: tracer}
	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodGet, "/thing?x=1", nil)
	rr := httptest.NewRecorder()
	Tracing(c)(next).ServeHTTP(rr, req)

	if !called {
		t.Fatal("next handler was not called")
	}
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
}
