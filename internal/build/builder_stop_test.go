package build

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestBuilder_Stop_NoInFlightBuild(t *testing.T) {
	b := NewBuilder(nil, core.NewEventBus(slog.Default()))

	if err := b.Stop("does-not-exist"); !errors.Is(err, ErrBuildNotFound) {
		t.Errorf("Stop(missing) = %v, want ErrBuildNotFound", err)
	}
	if err := b.Stop(""); !errors.Is(err, ErrBuildNotFound) {
		t.Errorf("Stop(empty) = %v, want ErrBuildNotFound", err)
	}
}

func TestBuilder_Stop_CancelsRegisteredContext(t *testing.T) {
	// White-box: register a cancel func directly and assert Stop
	// triggers it. Exercises the tracking without the full docker-build
	// pipeline (which would need a real runtime).
	b := NewBuilder(nil, core.NewEventBus(slog.Default()))

	ctx, cancel := context.WithCancel(context.Background())
	cleanup := b.registerInflight("app-42", cancel)
	defer cleanup()

	if err := b.Stop("app-42"); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	select {
	case <-ctx.Done():
		// expected — canceled
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel the registered context within 1s")
	}

	// Slot must now be empty — a second Stop returns ErrBuildNotFound.
	if err := b.Stop("app-42"); !errors.Is(err, ErrBuildNotFound) {
		t.Errorf("second Stop = %v, want ErrBuildNotFound", err)
	}
}

func TestBuilder_registerInflight_CleanupPreservesConcurrentEntry(t *testing.T) {
	// Regression: if Build #1 finishes after Build #2 has taken the
	// same AppID slot, Build #1's defer must NOT wipe Build #2's entry.
	b := NewBuilder(nil, core.NewEventBus(slog.Default()))

	_, cancel1 := context.WithCancel(context.Background())
	cleanup1 := b.registerInflight("app", cancel1)

	_, cancel2 := context.WithCancel(context.Background())
	cleanup2 := b.registerInflight("app", cancel2)

	// Build #1 finishes first and runs its cleanup — must leave
	// Build #2's entry in place.
	cleanup1()

	b.mu.Lock()
	_, present := b.inflight["app"]
	b.mu.Unlock()
	if !present {
		t.Fatal("cleanup1 wiped out cleanup2's entry — token match is broken")
	}

	// Stop must hit Build #2's cancel.
	if err := b.Stop("app"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	cleanup2() // idempotent after Stop already deleted the entry
}

func TestBuilder_Stop_Concurrent(t *testing.T) {
	// Exercise the lock paths under concurrent Stop/register calls.
	// No assertion beyond "doesn't deadlock or race" — run with -race.
	b := NewBuilder(nil, core.NewEventBus(slog.Default()))

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			appID := "app-" + string(rune('a'+(n%4)))
			_, cancel := context.WithCancel(context.Background())
			cleanup := b.registerInflight(appID, cancel)
			_ = b.Stop(appID)
			cleanup()
		}(i)
	}
	wg.Wait()
}
