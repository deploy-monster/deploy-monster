package deploy

import (
	"context"
	"testing"
	"time"
)

type autoRollbackTestContextKey string

func TestAutoRollbackManager_runCtx_WithStopCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ar := &AutoRollbackManager{stopCtx: ctx}
	got := ar.runCtx(context.Background())
	if got != ctx {
		t.Error("expected runCtx to return stopCtx when set")
	}
}

func TestAutoRollbackManager_runCtx_WithEventCtx(t *testing.T) {
	ar := &AutoRollbackManager{}
	eventCtx := context.WithValue(context.Background(), autoRollbackTestContextKey("key"), "val")
	got := ar.runCtx(eventCtx)
	if got != eventCtx {
		t.Error("expected runCtx to return eventCtx when stopCtx is nil")
	}
}

func TestAutoRollbackManager_runCtx_Fallback(t *testing.T) {
	ar := &AutoRollbackManager{}
	//lint:ignore SA1012 runCtx explicitly supports nil as a legacy fallback path.
	got := ar.runCtx(nil)

	// Should return a background context
	if _, ok := got.Deadline(); ok {
		t.Error("fallback context should not have a deadline")
	}
}

func TestAutoRollbackManager_runCtx_CanceledStopCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ar := &AutoRollbackManager{stopCtx: ctx}
	got := ar.runCtx(context.Background())

	select {
	case <-got.Done():
		// Expected — canceled context
	case <-time.After(100 * time.Millisecond):
		t.Error("expected canceled stopCtx to be returned")
	}
}
