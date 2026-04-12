package build

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestLogStore_CapturesLines(t *testing.T) {
	store := NewLogStore(nil, 0)
	w := store.Writer("app-1")

	io.WriteString(w, "step 1\nstep 2\nstep 3\n")

	lines := store.Lines("app-1")
	want := []string{"step 1", "step 2", "step 3"}
	if len(lines) != len(want) {
		t.Fatalf("len(lines) = %d, want %d: %v", len(lines), len(want), lines)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("lines[%d] = %q, want %q", i, lines[i], want[i])
		}
	}
}

func TestLogStore_PartialLineBuffered(t *testing.T) {
	store := NewLogStore(nil, 0)
	w := store.Writer("app-1")

	// A partial line should not be emitted until the newline arrives.
	io.WriteString(w, "hello ")
	if lines := store.Lines("app-1"); lines != nil {
		t.Errorf("expected no buffered lines, got %v", lines)
	}
	io.WriteString(w, "world\n")
	lines := store.Lines("app-1")
	if len(lines) != 1 || lines[0] != "hello world" {
		t.Errorf("expected [\"hello world\"], got %v", lines)
	}
}

func TestLogStore_RingDropsOldest(t *testing.T) {
	store := NewLogStore(nil, 3)
	w := store.Writer("app-1")

	for i := 0; i < 5; i++ {
		io.WriteString(w, "line\n")
	}
	io.WriteString(w, "keep-a\nkeep-b\nkeep-c\n")

	lines := store.Lines("app-1")
	if len(lines) != 3 {
		t.Fatalf("ring capacity broken: %d lines", len(lines))
	}
	if lines[0] != "keep-a" || lines[1] != "keep-b" || lines[2] != "keep-c" {
		t.Errorf("ring did not drop oldest correctly: %v", lines)
	}
}

func TestLogStore_Reset(t *testing.T) {
	store := NewLogStore(nil, 0)
	io.WriteString(store.Writer("app-1"), "before\n")
	store.Reset("app-1")
	if lines := store.Lines("app-1"); lines != nil {
		t.Errorf("Reset did not clear lines: %v", lines)
	}
}

func TestLogStore_EmitsBuildLogEvents(t *testing.T) {
	bus := core.NewEventBus(nil)

	var mu sync.Mutex
	var got []core.BuildLogEventData
	bus.SubscribeAsync(core.EventBuildLog, func(ctx context.Context, evt core.Event) error {
		if d, ok := evt.Data.(core.BuildLogEventData); ok {
			mu.Lock()
			got = append(got, d)
			mu.Unlock()
		}
		return nil
	})

	store := NewLogStore(bus, 0)
	io.WriteString(store.Writer("app-1"), "one\ntwo\n")

	// Drain the async dispatch so handlers have finished before the
	// assertions below. Tier 101: replaced a racy time.Sleep with the
	// deterministic asyncWG wait.
	bus.Drain()

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("expected 2 build.log events, got %d", len(got))
	}
	// SubscribeAsync handlers run in goroutines so arrival order isn't
	// deterministic. Verify both lines are present regardless of order.
	seen := map[string]bool{got[0].Line: true, got[1].Line: true}
	if !seen["one"] || !seen["two"] {
		t.Errorf("expected events for lines 'one' and 'two', got %+v", got)
	}
	for _, d := range got {
		if d.AppID != "app-1" {
			t.Errorf("event app_id = %q, want app-1", d.AppID)
		}
	}
}

func TestLogStore_StripsCarriageReturns(t *testing.T) {
	store := NewLogStore(nil, 0)
	io.WriteString(store.Writer("app-1"), "windows\r\nunix\n")

	lines := store.Lines("app-1")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if strings.ContainsRune(lines[0], '\r') {
		t.Errorf("trailing CR not stripped: %q", lines[0])
	}
	if lines[0] != "windows" || lines[1] != "unix" {
		t.Errorf("lines = %v, want [windows unix]", lines)
	}
}
