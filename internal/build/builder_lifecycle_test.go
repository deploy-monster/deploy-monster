package build

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// newTestBuilder returns a Builder wired with a silent event bus —
// we don't care about events in lifecycle tests, only that the
// closed/wg/recover machinery behaves the way Module.Stop needs.
func newTestBuilder() *Builder {
	return NewBuilder(nil, core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil))))
}

func TestBuilder_StopAll_CancelsAllInflight(t *testing.T) {
	b := newTestBuilder()

	// Register three in-flight builds from different app IDs.
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	ctx3, cancel3 := context.WithCancel(context.Background())
	defer cancel1()
	defer cancel2()
	defer cancel3()

	b.registerInflight("a", cancel1)
	b.registerInflight("b", cancel2)
	b.registerInflight("c", cancel3)

	if n := b.StopAll(); n != 3 {
		t.Errorf("StopAll returned %d, want 3", n)
	}
	if !b.Closed() {
		t.Error("Closed() = false, want true after StopAll")
	}

	for _, ctx := range []context.Context{ctx1, ctx2, ctx3} {
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("StopAll did not cancel every registered context within 1s")
		}
	}
}

func TestBuilder_StopAll_Idempotent(t *testing.T) {
	b := newTestBuilder()
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	b.registerInflight("a", cancel)

	if n := b.StopAll(); n != 1 {
		t.Errorf("first StopAll = %d, want 1", n)
	}
	if n := b.StopAll(); n != 0 {
		t.Errorf("second StopAll = %d, want 0 on already-closed builder", n)
	}
}

func TestBuilder_Build_RejectedAfterStopAll(t *testing.T) {
	b := newTestBuilder()
	b.StopAll()

	_, err := b.Build(context.Background(), BuildOpts{
		AppID:     "app-1",
		AppName:   "rejected",
		SourceURL: "https://example.com/repo.git",
	}, io.Discard)

	if !errors.Is(err, ErrBuilderClosed) {
		t.Errorf("Build after StopAll = %v, want ErrBuilderClosed", err)
	}
}

func TestBuilder_Wait_ReturnsWhenIdle(t *testing.T) {
	b := newTestBuilder()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := b.Wait(ctx); err != nil {
		t.Errorf("Wait on idle builder = %v, want nil", err)
	}
}

func TestBuilder_Wait_BlocksForInflight(t *testing.T) {
	// Drive wg directly — a real Build would do this through the
	// closed/Add sequence at Build entry, but a white-box manipulation
	// is cleaner than running the full docker pipeline.
	b := newTestBuilder()
	b.wg.Add(1)

	// First: Wait must time out while the "build" is pretending to run.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := b.Wait(ctx); err == nil {
		t.Error("Wait returned nil with in-flight build, want ctx deadline")
	}

	// Now finish the "build" and Wait should unblock.
	b.wg.Done()
	if err := b.Wait(context.Background()); err != nil {
		t.Errorf("Wait after drain = %v, want nil", err)
	}
}

func TestBuilder_Build_PanicRecovered(t *testing.T) {
	// White-box: trigger a panic by passing a log writer that panics on
	// the first write. Build's first Fprintf hits the writer before any
	// network / disk work, so this exercises the recover path without
	// needing a real docker runtime.
	b := newTestBuilder()
	w := &panicWriter{}

	_, err := b.Build(context.Background(), BuildOpts{
		AppID:     "panic-app",
		AppName:   "panicker",
		SourceURL: "https://example.com/repo.git",
	}, w)

	if err == nil {
		t.Fatal("Build returned nil error after panic, want recovered error")
	}
	if !containsString(err.Error(), "panic") {
		t.Errorf("err = %v, want message containing 'panic'", err)
	}
}

func TestBuilder_Wait_DrainsConcurrentInflight(t *testing.T) {
	// Spawn several goroutines pretending to be in-flight Builds, then
	// verify Wait blocks until they all complete.
	b := newTestBuilder()

	var startedWG sync.WaitGroup
	release := make(chan struct{})
	for i := 0; i < 5; i++ {
		b.wg.Add(1)
		startedWG.Add(1)
		go func() {
			startedWG.Done()
			<-release
			b.wg.Done()
		}()
	}
	startedWG.Wait()

	done := make(chan error, 1)
	go func() { done <- b.Wait(context.Background()) }()

	select {
	case <-done:
		t.Fatal("Wait returned before in-flight goroutines drained")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Wait = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait did not return within 1s of in-flight goroutines finishing")
	}
}

// panicWriter is an io.Writer that panics on the first Write. Used to
// trigger the Build recover path without needing docker or git.
type panicWriter struct {
	writes int
}

func (p *panicWriter) Write(b []byte) (int, error) {
	p.writes++
	panic("intentional test panic")
}

// containsString is a tiny substring helper so the test file doesn't
// need to import the `strings` package — keeps the dep surface minimal.
func containsString(haystack, needle string) bool {
	return bytes.Contains([]byte(haystack), []byte(needle))
}
