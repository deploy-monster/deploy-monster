package backup

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestScheduler_StopCtx_IdleReturnsImmediately(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	// No Start → wg is empty → StopCtx should return nil almost instantly
	// even with a zero deadline (context.Background never fires Done).
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := s.StopCtx(ctx); err != nil {
		t.Errorf("StopCtx on idle scheduler = %v, want nil", err)
	}
	if !s.Closed() {
		t.Error("Closed() = false after StopCtx, want true")
	}
}

func TestScheduler_StopCtx_Idempotent(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	if err := s.StopCtx(context.Background()); err != nil {
		t.Fatalf("first StopCtx: %v", err)
	}
	if err := s.StopCtx(context.Background()); err != nil {
		t.Errorf("second StopCtx = %v, want nil (idempotent)", err)
	}
}

func TestScheduler_StopCtx_DeadlineExceededOnStuckDrain(t *testing.T) {
	// White-box: add a phantom in-flight job to wg so StopCtx's
	// drain goroutine is genuinely blocked, and assert the ctx
	// deadline wins. Any Tier-67-style callers that switch from
	// Stop() to StopCtx(ctx) expect a context.DeadlineExceeded here
	// so they can log and move on instead of pinning the process.
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	s.wg.Add(1)
	defer s.wg.Done() // release after the test

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := s.StopCtx(ctx)
	if err == nil {
		t.Fatal("StopCtx returned nil with stuck in-flight job, want deadline error")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("err = %v, want context.DeadlineExceeded", err)
	}
}

func TestScheduler_StartAfterStop_IsNoOp(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	s.Stop()
	if !s.Closed() {
		t.Fatal("Closed() = false after Stop")
	}

	// Start must not spawn a loop goroutine on a stopped scheduler.
	// We can observe this by checking that wg is at zero both before
	// and after Start — a spawned loop would do wg.Add(1).
	s.Start()
	// If Start erroneously spawned, wg would be non-zero and a
	// subsequent StopCtx with a sub-ms deadline would race. Assert
	// the faster path: a fresh StopCtx with zero wait returns nil
	// immediately because wg is still empty.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := s.StopCtx(ctx); err != nil {
		t.Errorf("StopCtx after Start-after-Stop = %v, want nil (Start must have been a no-op)", err)
	}
}

func TestScheduler_Closed_FalseBeforeStop(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	if s.Closed() {
		t.Error("Closed() = true on fresh scheduler, want false")
	}
}

func TestScheduler_StopCtx_DrainsRunningLoop(t *testing.T) {
	// Spin a real loop goroutine, then stop it with a generous
	// deadline. Asserts the loop actually exits on stopCtx.Done so
	// StopCtx can complete with no error.
	s := NewScheduler(nil, nil, core.NewEventBus(testLogger()), nil, "02:00", testLogger())
	s.Start()

	// Give the goroutine a moment to enter its select.
	time.Sleep(5 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.StopCtx(ctx); err != nil {
		t.Errorf("StopCtx on running loop = %v, want nil", err)
	}
	if !s.Closed() {
		t.Error("Closed() = false after draining loop")
	}
}

// phantomCounter lets a test observe how many times runBackupsCtx
// runs before/after Stop. Unused for now but captured as a scaffold
// for a later integration test that asserts "no backup runs start
// after StopCtx returns".
type phantomCounter struct {
	count atomic.Int32
}

func (p *phantomCounter) Inc() { p.count.Add(1) }
func (p *phantomCounter) Get() int32 {
	return p.count.Load()
}

func TestPhantomCounter_Smoke(t *testing.T) {
	// Keeps the type exercised so unused-code linters don't strip it
	// before the integration test lands.
	var p phantomCounter
	p.Inc()
	p.Inc()
	if p.Get() != 2 {
		t.Errorf("phantomCounter.Get() = %d, want 2", p.Get())
	}
}
