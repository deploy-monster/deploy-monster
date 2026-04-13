package core

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tier 70 — core scheduler lifecycle + ctx plumbing tests.
//
// These cover the regressions fixed in Tier 70:
//   - NewScheduler nil-logger guard
//   - Stop idempotency (stopOnce-guarded double close)
//   - Stop waits for loop AND in-flight handlers (wg.Wait)
//   - Start idempotency (startOnce prevents duplicate goroutines)
//   - Stop without Start does not deadlock on wg.Wait
//   - Cancellable stopCtx plumbed to every handler
//   - Handler panic recovery keeps the scheduler alive
//   - Per-job Timeout override
//   - parseHHMM rejects garbage input
//   - calcNextRun falls back safely on bad schedules
//   - runCtx nil fallback for struct-literal construction

// ─── NewScheduler nil-logger guard ─────────────────────────────────────────

func TestTier70_NewScheduler_NilLogger(t *testing.T) {
	s := NewScheduler(nil)
	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if s.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if s.stopCtx == nil || s.stopCancel == nil {
		t.Error("stopCtx/stopCancel should be initialized")
	}
	if s.stopCh == nil {
		t.Error("stopCh should be initialized")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier70_Scheduler_Stop_Idempotent(t *testing.T) {
	s := NewScheduler(tier70Logger())
	s.Start()

	// Double-Stop must not panic. Before Tier 70 the second call
	// panicked with "close of closed channel" because there was no
	// stopOnce guard.
	s.Stop()
	s.Stop()
}

func TestTier70_Scheduler_Stop_WithoutStart_Safe(t *testing.T) {
	s := NewScheduler(tier70Logger())
	// Must not deadlock on wg.Wait — nothing was added to the group.
	s.Stop()
	s.Stop()
}

// ─── Start idempotency ─────────────────────────────────────────────────────

func TestTier70_Scheduler_Start_Idempotent(t *testing.T) {
	s := NewScheduler(tier70Logger())

	// Starting twice must not double-count wg. If it did, Stop would
	// block forever waiting for a phantom second goroutine.
	s.Start()
	s.Start()

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked — startOnce/wg balance is wrong")
	}
}

// ─── Stop waits for the loop goroutine ─────────────────────────────────────

func TestTier70_Scheduler_Stop_WaitsForLoop(t *testing.T) {
	s := NewScheduler(tier70Logger())
	s.Start()

	// Give the goroutine a moment to enter its select.
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return — wg.Wait missing or deadlock")
	}
}

// ─── Stop waits for in-flight handlers ─────────────────────────────────────

// TestTier70_Scheduler_Stop_WaitsForInFlightHandler proves that Stop
// actually blocks until dispatched handler goroutines drain. Before
// Tier 70 the handler goroutine was not tracked by wg, so a slow job
// could keep running long after Stop returned.
func TestTier70_Scheduler_Stop_WaitsForInFlightHandler(t *testing.T) {
	s := NewScheduler(tier70Logger())

	started := make(chan struct{})
	finished := atomic.Bool{}
	s.Add(&CronJob{
		Name:     "slow",
		Schedule: "@every 1h",
		Handler: func(ctx context.Context) error {
			close(started)
			// Sleep briefly but longer than the test's wait-before-Stop.
			select {
			case <-ctx.Done():
			case <-time.After(200 * time.Millisecond):
			}
			finished.Store(true)
			return nil
		},
	})

	// Force the job to be due immediately and dispatch it through tick.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()
	s.tick()

	// Wait for the handler to enter.
	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("handler never started")
	}

	// Stop must block until the handler returns.
	s.Stop()
	if !finished.Load() {
		t.Error("Stop returned before in-flight handler finished")
	}
}

// ─── Stop cancels in-flight handler via ctx ────────────────────────────────

// TestTier70_Scheduler_Stop_CancelsInFlightHandler proves the handler
// context is derived from stopCtx so Stop can abort a long-running job
// at its next context checkpoint.
func TestTier70_Scheduler_Stop_CancelsInFlightHandler(t *testing.T) {
	s := NewScheduler(tier70Logger())

	started := make(chan struct{})
	var observedCancel atomic.Bool
	s.Add(&CronJob{
		Name:     "blocker",
		Schedule: "@every 1h",
		Handler: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			observedCancel.Store(true)
			return ctx.Err()
		},
	})

	// Make it due and dispatch.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()
	s.tick()

	select {
	case <-started:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocker handler never started")
	}

	// Stop cancels the parent ctx; the handler must observe cancellation.
	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return — handler ctx cancellation is not plumbed")
	}

	if !observedCancel.Load() {
		t.Error("handler did not observe ctx cancellation")
	}
}

// ─── Handler panic recovery ────────────────────────────────────────────────

// TestTier70_Scheduler_HandlerPanic_Recovered proves a panicking
// handler does not tear the whole process down. Before Tier 70 the
// dispatch goroutine had no defer/recover so one bad job would take
// the scheduler with it.
func TestTier70_Scheduler_HandlerPanic_Recovered(t *testing.T) {
	s := NewScheduler(tier70Logger())

	s.Add(&CronJob{
		Name:     "kaboom",
		Schedule: "@every 1h",
		Handler: func(_ context.Context) error {
			panic("boom")
		},
	})

	// Force the job to be due.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()

	// tick must not panic. If the recover is missing, this test
	// crashes the whole test binary.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic escaped dispatch goroutine: %v", r)
		}
	}()
	s.tick()

	// Give the goroutine a moment to run + recover.
	time.Sleep(100 * time.Millisecond)

	// After a panic, Running should be reset so the job can run again.
	s.mu.RLock()
	var stillRunning bool
	for _, j := range s.jobs {
		if j.Running {
			stillRunning = true
		}
	}
	s.mu.RUnlock()
	if stillRunning {
		t.Error("job.Running was not reset after panic recovery")
	}

	// Stop must also still work cleanly.
	s.Stop()
}

// ─── Per-job Timeout override ──────────────────────────────────────────────

// TestTier70_Scheduler_Job_TimeoutOverride proves that CronJob.Timeout,
// when set, bounds a single handler invocation. We set a 30ms timeout
// and block in the handler until the ctx fires.
func TestTier70_Scheduler_Job_TimeoutOverride(t *testing.T) {
	s := NewScheduler(tier70Logger())
	defer s.Stop()

	observed := make(chan time.Duration, 1)
	s.Add(&CronJob{
		Name:     "bounded",
		Schedule: "@every 1h",
		Timeout:  30 * time.Millisecond,
		Handler: func(ctx context.Context) error {
			start := time.Now()
			<-ctx.Done()
			observed <- time.Since(start)
			return ctx.Err()
		},
	})

	// Force the job to be due.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()
	s.tick()

	select {
	case d := <-observed:
		if d > 500*time.Millisecond {
			t.Errorf("handler ctx fired after %v — Timeout override ignored", d)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler never observed ctx cancellation — Timeout override not plumbed")
	}
}

// ─── parseHHMM validation ──────────────────────────────────────────────────

func TestTier70_ParseHHMM_InvalidInput(t *testing.T) {
	bad := []string{
		"",         // empty
		"abc",      // no colon
		"99:00",    // hour out of range
		"12:99",    // minute out of range
		"-1:00",    // negative hour
		"12:-1",    // negative minute
		"ab:cd",    // non-numeric
		"12",       // missing colon
		"12:00:00", // too many parts — third field treated as minute
		"  :  ",    // only a colon
	}
	for _, in := range bad {
		if _, err := parseHHMM(in); err == nil {
			t.Errorf("parseHHMM(%q) = nil error, expected failure", in)
		}
	}
}

func TestTier70_ParseHHMM_ValidInput(t *testing.T) {
	cases := []struct {
		in  string
		out int
	}{
		{"00:00", 0},
		{"12:34", 12*60 + 34},
		{"23:59", 23*60 + 59},
		{"  7:05  ", 7*60 + 5}, // TrimSpace handling
	}
	for _, c := range cases {
		got, err := parseHHMM(c.in)
		if err != nil {
			t.Errorf("parseHHMM(%q) errored: %v", c.in, err)
			continue
		}
		if got != c.out {
			t.Errorf("parseHHMM(%q) = %d, want %d", c.in, got, c.out)
		}
	}
}

// ─── calcNextRun fallback on bad schedules ─────────────────────────────────

func TestTier70_CalcNextRun_InvalidSchedule_FallsBackTo1h(t *testing.T) {
	s := NewScheduler(tier70Logger())

	inputs := []string{
		"@every garbage",
		"not-a-schedule",
		"99:99",
	}
	for _, in := range inputs {
		before := time.Now()
		next := s.calcNextRun(in)
		diff := next.Sub(before)
		// Should fall back to ~1h, never wedge or panic.
		if diff < 55*time.Minute || diff > 65*time.Minute {
			t.Errorf("calcNextRun(%q) = %v from now, expected ~1h fallback", in, diff)
		}
	}
}

// ─── runCtx nil fallback ──────────────────────────────────────────────────

func TestTier70_Scheduler_RunCtx_NilFallback(t *testing.T) {
	// Bare struct literal — no NewScheduler, so stopCtx is nil.
	s := &Scheduler{logger: tier70Logger()}
	ctx := s.runCtx()
	if ctx == nil {
		t.Fatal("runCtx must not return nil")
	}
	if ctx.Err() != nil {
		t.Errorf("fallback background context should not be canceled: %v", ctx.Err())
	}
}

// ─── Concurrent Start+Stop storm ───────────────────────────────────────────

// TestTier70_Scheduler_ConcurrentStartStop exercises the startOnce /
// stopOnce guards under concurrent pressure. Before Tier 70 the
// concurrent double-close would race with a close-of-closed-channel
// panic.
func TestTier70_Scheduler_ConcurrentStartStop(t *testing.T) {
	s := NewScheduler(tier70Logger())

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); s.Start() }()
		go func() { defer wg.Done(); s.Stop() }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	s.Stop()
}

// ─── Disabled jobs are skipped ─────────────────────────────────────────────

func TestTier70_Scheduler_DisabledJob_NotDispatched(t *testing.T) {
	s := NewScheduler(tier70Logger())
	defer s.Stop()

	var called atomic.Bool
	s.Add(&CronJob{
		Name:     "disabled",
		Schedule: "@every 1h",
		Handler: func(_ context.Context) error {
			called.Store(true)
			return nil
		},
	})

	// Disable it and make it due.
	s.mu.Lock()
	for _, j := range s.jobs {
		j.Enabled = false
		j.NextRun = time.Now().Add(-time.Second)
	}
	s.mu.Unlock()

	s.tick()
	time.Sleep(50 * time.Millisecond)

	if called.Load() {
		t.Error("disabled job was dispatched")
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier70Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
