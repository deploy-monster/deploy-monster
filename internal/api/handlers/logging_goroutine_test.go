package handlers

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── ctxLogger Tests ───────────────────────────────────────────────────────

func TestCtxLogger_WithCorrelationID(t *testing.T) {
	ctx := core.WithCorrelationID(context.Background(), "req-abc123")
	l := ctxLogger(ctx)
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}

func TestCtxLogger_WithoutCorrelationID(t *testing.T) {
	l := ctxLogger(context.Background())
	if l == nil {
		t.Fatal("expected non-nil logger even without correlation ID")
	}
}

// ─── safeGo WaitGroup Tests ───────────────────────────────────────────────

func TestSafeGo_WaitForBackground(t *testing.T) {
	var counter atomic.Int32

	for i := 0; i < 5; i++ {
		safeGo(func() {
			time.Sleep(50 * time.Millisecond)
			counter.Add(1)
		}, nil)
	}

	WaitForBackground()

	if counter.Load() != 5 {
		t.Errorf("expected 5 goroutines completed, got %d", counter.Load())
	}
}

func TestSafeGo_PanicDoesNotBlockWait(t *testing.T) {
	safeGo(func() {
		panic("test panic")
	}, nil)

	done := make(chan struct{})
	go func() {
		WaitForBackground()
		close(done)
	}()

	select {
	case <-done:
		// success
	case <-time.After(3 * time.Second):
		t.Fatal("WaitForBackground blocked after safeGo panic")
	}
}

// ─── internalErrorCtx Tests ───────────────────────────────────────────────

func TestInternalErrorCtx_DoesNotLeak(t *testing.T) {
	ctx := core.WithCorrelationID(context.Background(), "req-xyz")
	rr := httptest.NewRecorder()

	internalErrorCtx(ctx, rr, "something broke", fmt.Errorf("secret: host=10.0.0.1"))

	if rr.Code != 500 {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	body := rr.Body.String()
	if strings.Contains(body, "10.0.0.1") {
		t.Error("leaked internal details")
	}
}
