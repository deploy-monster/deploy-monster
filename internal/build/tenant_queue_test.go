package build

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// tenantQueueLogger produces a logger that discards everything so
// the Tier 69 test output stays readable even under panic-recovery
// logs produced by the stress test.
func tenantQueueLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// waitForInflight polls until the queue reports at least want
// in-flight jobs on the named tenant, or the deadline elapses. This
// is preferred over a bare time.Sleep because CI boxes vary wildly
// in scheduler latency.
func waitForTenantInflight(t *testing.T, q *TenantQueue, tenantID string, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if q.TenantInFlight(tenantID) >= want {
			return
		}
		time.Sleep(500 * time.Microsecond)
	}
	t.Fatalf("tenant %s inflight = %d, want >= %d within %s",
		tenantID, q.TenantInFlight(tenantID), want, within)
}

func waitForGlobalInflight(t *testing.T, q *TenantQueue, want int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if q.GlobalInFlight() >= want {
			return
		}
		time.Sleep(500 * time.Microsecond)
	}
	t.Fatalf("global inflight = %d, want >= %d within %s",
		q.GlobalInFlight(), want, within)
}

// TestTenantQueue_PerTenantCap_BlocksSameTenant proves the
// per-tenant limit holds back excess jobs from the same tenant even
// when global slots are wide open. Without this the "per-tenant"
// cap would be meaningless.
func TestTenantQueue_PerTenantCap_BlocksSameTenant(t *testing.T) {
	q := NewTenantQueue(10, 2, tenantQueueLogger()) // global 10, per-tenant 2

	// release is closed by the test body to let all held jobs finish.
	release := make(chan struct{})
	start := make(chan struct{})

	// Submit 2 jobs that will park until release closes.
	for i := 0; i < 2; i++ {
		if err := q.Submit(context.Background(), "tenant-a", func() {
			start <- struct{}{}
			<-release
		}); err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	// Wait for both to start so we're at the tenant cap.
	<-start
	<-start
	waitForTenantInflight(t, q, "tenant-a", 2, time.Second)

	// A 3rd submission for tenant-a must block inside Submit. We
	// observe this by racing Submit against a short context deadline
	// — if the gate works, Submit returns ctx.DeadlineExceeded
	// without ever running the func.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	var thirdRan atomic.Bool
	err := q.Submit(ctx, "tenant-a", func() { thirdRan.Store(true) })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("3rd Submit err = %v, want DeadlineExceeded", err)
	}
	if thirdRan.Load() {
		t.Error("3rd tenant-a job ran despite per-tenant cap")
	}

	// Releasing the held jobs must let a subsequent submission
	// succeed immediately — proves the slot was actually freed.
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := q.Submit(ctx2, "tenant-a", func() {}); err != nil {
		t.Errorf("post-release Submit: %v", err)
	}
	_ = q.Shutdown(context.Background())
}

// TestTenantQueue_TenantAIsolationFromTenantB is the headline
// fairness test: tenant A saturates its per-tenant cap (and all the
// global slots it can). Tenant B must still be able to enqueue
// work. Before Phase 3.3.7 a single tenant could occupy every global
// slot and block the rest of the platform.
func TestTenantQueue_TenantAIsolationFromTenantB(t *testing.T) {
	// Global cap = 4, per-tenant cap = 3. Tenant A can take at most
	// 3 of the 4 global slots, leaving a guaranteed slot free for any
	// other tenant.
	q := NewTenantQueue(4, 3, tenantQueueLogger())

	release := make(chan struct{})
	started := make(chan string, 10)

	// Saturate tenant A to its per-tenant cap.
	for i := 0; i < 3; i++ {
		if err := q.Submit(context.Background(), "tenant-a", func() {
			started <- "a"
			<-release
		}); err != nil {
			t.Fatalf("A submit %d: %v", i, err)
		}
	}
	for i := 0; i < 3; i++ {
		<-started
	}

	// Tenant B must get through the gate — this is the starvation
	// check. We bound the wait at 200ms because any gate hold
	// longer than that is effectively a failure in a single-digit-
	// milliseconds-per-submit codebase.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	bDone := make(chan struct{})
	if err := q.Submit(ctx, "tenant-b", func() {
		started <- "b"
		close(bDone)
	}); err != nil {
		t.Fatalf("B Submit blocked by tenant-a saturation: %v", err)
	}

	select {
	case <-bDone:
		// good — tenant B completed while tenant A was parked
	case <-time.After(500 * time.Millisecond):
		t.Fatal("tenant-b job never ran while tenant-a was saturated")
	}

	// Now let A drain and shut down.
	close(release)
	_ = q.Shutdown(context.Background())
}

// TestTenantQueue_GlobalCapEnforced verifies the global cap is
// enforced across all tenants combined. We set global=2,
// per-tenant=2 so tenant A alone could fill the global, but we want
// to see the global block a CROSS-tenant job when A is using its
// full share.
func TestTenantQueue_GlobalCapEnforced(t *testing.T) {
	q := NewTenantQueue(2, 5, tenantQueueLogger()) // global 2, per-tenant 5

	release := make(chan struct{})
	started := make(chan struct{}, 10)

	if err := q.Submit(context.Background(), "tenant-a", func() {
		started <- struct{}{}
		<-release
	}); err != nil {
		t.Fatalf("A1 submit: %v", err)
	}
	if err := q.Submit(context.Background(), "tenant-b", func() {
		started <- struct{}{}
		<-release
	}); err != nil {
		t.Fatalf("B1 submit: %v", err)
	}
	<-started
	<-started
	waitForGlobalInflight(t, q, 2, time.Second)

	// A 3rd job on any tenant must block on the global cap. Use a
	// fresh tenant so it cannot possibly be tripped by the per-tenant
	// cap — only the global cap could stop it.
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	err := q.Submit(ctx, "tenant-c", func() {})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cross-tenant Submit under global cap err = %v, want DeadlineExceeded", err)
	}

	close(release)
	_ = q.Shutdown(context.Background())
}

// TestTenantQueue_FIFOWakeupOnSlotRelease proves that a queued job
// actually runs when its blocking slot is released (rather than
// only being unblocked by context cancel / shutdown, which would
// make the queue effectively useless as a fairness mechanism).
func TestTenantQueue_FIFOWakeupOnSlotRelease(t *testing.T) {
	q := NewTenantQueue(1, 1, tenantQueueLogger()) // one job at a time total

	first := make(chan struct{})
	firstRelease := make(chan struct{})
	if err := q.Submit(context.Background(), "t", func() {
		close(first)
		<-firstRelease
	}); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	<-first // first job is running

	// Kick off a second submit that must wait for the first to free
	// a slot. We run it in a goroutine because Submit itself blocks.
	secondStarted := make(chan struct{})
	submitErr := make(chan error, 1)
	go func() {
		submitErr <- q.Submit(context.Background(), "t", func() {
			close(secondStarted)
		})
	}()

	// Release the first job — the second must wake up and run.
	time.Sleep(20 * time.Millisecond) // let the goroutine actually block
	close(firstRelease)

	select {
	case <-secondStarted:
		// good
	case <-time.After(time.Second):
		t.Fatal("second job did not start after first released its slot")
	}
	if err := <-submitErr; err != nil {
		t.Errorf("second Submit err = %v, want nil", err)
	}
	_ = q.Shutdown(context.Background())
}

// TestTenantQueue_ShutdownUnblocksPendingSubmits catches the wedge
// scenario: a process-level shutdown lands while Submit is parked
// in the global-slot wait, and must return ErrTenantQueueClosed
// instead of hanging forever. This is the tenant-queue equivalent
// of the Tier 69 WorkerPool shutdown-unblock guard.
func TestTenantQueue_ShutdownUnblocksPendingSubmits(t *testing.T) {
	q := NewTenantQueue(1, 1, tenantQueueLogger())

	// Saturate the global cap.
	occupyRelease := make(chan struct{})
	occupyStarted := make(chan struct{})
	if err := q.Submit(context.Background(), "parker", func() {
		close(occupyStarted)
		<-occupyRelease
	}); err != nil {
		t.Fatalf("parker submit: %v", err)
	}
	<-occupyStarted

	// Queue a second job that MUST block on the global cap.
	pendingErr := make(chan error, 1)
	go func() {
		pendingErr <- q.Submit(context.Background(), "waiter", func() {})
	}()

	// Give the goroutine time to enter Submit and actually start
	// waiting on the global slot.
	time.Sleep(30 * time.Millisecond)

	// Run Shutdown concurrently — it will close stopCh immediately,
	// then block in wg.Wait until the parker's goroutine finishes.
	// The parker stays held throughout, keeping the global slot
	// full, so the waiter's phase-2 select has exactly one ready
	// case (<-stopCh) and must return ErrTenantQueueClosed. Releasing
	// the parker before the waiter has definitively returned would
	// race: the parker's deferred cleanup would free the global slot,
	// Go's select could pick the now-ready `global <- struct{}{}`
	// case instead of stopCh, and the waiter's fn would run to
	// completion with Submit returning nil.
	shutdownDone := make(chan error, 1)
	go func() {
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		shutdownDone <- q.Shutdown(shutCtx)
	}()

	// Wait for Shutdown to flip closed=true (i.e. stopCh closed).
	closedDeadline := time.Now().Add(time.Second)
	for !q.Closed() {
		if time.Now().After(closedDeadline) {
			t.Fatal("Shutdown did not flip Closed() within 1s")
		}
		time.Sleep(time.Millisecond)
	}

	// The waiter observes stopCh while the parker still holds the
	// global slot, so its Submit must now resolve with ErrClosed.
	select {
	case err := <-pendingErr:
		if !errors.Is(err, ErrTenantQueueClosed) {
			t.Errorf("pending Submit err = %v, want ErrTenantQueueClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending Submit never returned after Shutdown closed stopCh")
	}

	// Now release the parker so Shutdown's wg.Wait can drain.
	close(occupyRelease)
	if err := <-shutdownDone; err != nil {
		t.Errorf("Shutdown err = %v, want nil", err)
	}
}

// TestTenantQueue_SubmitAfterShutdown is the post-shutdown guard:
// any Submit after Shutdown returned must immediately refuse with
// ErrTenantQueueClosed rather than run fn or hang.
func TestTenantQueue_SubmitAfterShutdown(t *testing.T) {
	q := NewTenantQueue(2, 2, tenantQueueLogger())
	if err := q.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if !q.Closed() {
		t.Fatal("Closed() = false after Shutdown")
	}
	var ran atomic.Bool
	err := q.Submit(context.Background(), "t", func() { ran.Store(true) })
	if !errors.Is(err, ErrTenantQueueClosed) {
		t.Errorf("Submit after Shutdown err = %v, want ErrTenantQueueClosed", err)
	}
	if ran.Load() {
		t.Error("Submit ran fn after Shutdown")
	}
}

// TestTenantQueue_ContextCancelReleasesSlots catches a slow leak: a
// Submit that aborts via ctx cancel must release any slots it had
// acquired so subsequent work can use them. Forgetting to release
// the tenant slot on the second-phase global wait was the classic
// bug this test guards.
func TestTenantQueue_ContextCancelReleasesSlots(t *testing.T) {
	q := NewTenantQueue(1, 1, tenantQueueLogger()) // one slot total

	// Occupy the only slot.
	release := make(chan struct{})
	started := make(chan struct{})
	if err := q.Submit(context.Background(), "tenant-a", func() {
		close(started)
		<-release
	}); err != nil {
		t.Fatalf("occupy submit: %v", err)
	}
	<-started

	// Attempt a second submit that must block on the global slot,
	// then cancel it.
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	err := q.Submit(ctx, "tenant-b", func() {})
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked Submit err = %v, want DeadlineExceeded", err)
	}

	// Free the occupier. A fresh tenant-b submit must succeed
	// immediately — if ctx cancel had leaked a slot, this would
	// wedge.
	close(release)
	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if err := q.Submit(ctx2, "tenant-b", func() {}); err != nil {
		t.Errorf("post-cancel Submit leaked a slot: %v", err)
	}
	_ = q.Shutdown(context.Background())
}

// TestTenantQueue_NoStarvationUnderLoad is a coarse fairness stress
// test: two tenants race to submit 50 jobs each into a queue with
// global=4 and per-tenant=2. Both tenants must complete their 50
// jobs within a reasonable deadline — if per-tenant fairness were
// broken, one tenant could monopolize and the other would see
// timeouts.
func TestTenantQueue_NoStarvationUnderLoad(t *testing.T) {
	q := NewTenantQueue(4, 2, tenantQueueLogger())

	const perTenant = 50
	var aDone, bDone atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	fire := func(tenant string, counter *atomic.Int32) {
		defer wg.Done()
		for i := 0; i < perTenant; i++ {
			// Every Submit is bounded by ctx so a fairness bug surfaces
			// as a DeadlineExceeded rather than a test hang.
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			err := q.Submit(ctx, tenant, func() {
				// Short sleep so jobs actually overlap.
				time.Sleep(time.Millisecond)
				counter.Add(1)
			})
			cancel()
			if err != nil {
				t.Errorf("%s submit %d: %v", tenant, i, err)
				return
			}
		}
	}
	go fire("tenant-a", &aDone)
	go fire("tenant-b", &bDone)
	wg.Wait()

	// Drain remaining jobs.
	if err := q.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown: %v", err)
	}

	if aDone.Load() != perTenant {
		t.Errorf("tenant-a completed %d, want %d", aDone.Load(), perTenant)
	}
	if bDone.Load() != perTenant {
		t.Errorf("tenant-b completed %d, want %d", bDone.Load(), perTenant)
	}
}

// TestTenantQueue_NegativeCapsClamp catches the happy-path for the
// config-validation invariant: a misconfigured limits block cannot
// produce a zero-capacity semaphore that deadlocks every submit.
func TestTenantQueue_NegativeCapsClamp(t *testing.T) {
	q := NewTenantQueue(-1, -5, tenantQueueLogger())
	// Capacities should be at least 1 each, so Submit must succeed.
	if err := q.Submit(context.Background(), "t", func() {}); err != nil {
		t.Errorf("clamped queue refused Submit: %v", err)
	}
	_ = q.Shutdown(context.Background())
}

// TestTenantQueue_PanicRecovery ensures a panicking job does not
// tear down the queue. The job is allowed to panic; the queue's
// deferred recover logs it and releases the slots. A subsequent
// submit must still succeed.
func TestTenantQueue_PanicRecovery(t *testing.T) {
	q := NewTenantQueue(1, 1, tenantQueueLogger())

	done := make(chan struct{})
	if err := q.Submit(context.Background(), "t", func() {
		defer close(done)
		panic("boom")
	}); err != nil {
		t.Fatalf("panicking submit: %v", err)
	}
	<-done
	// Give the deferred slot-release a tick to run.
	time.Sleep(10 * time.Millisecond)

	// Next submit must go through.
	ranAfter := make(chan struct{})
	if err := q.Submit(context.Background(), "t", func() { close(ranAfter) }); err != nil {
		t.Errorf("post-panic Submit: %v", err)
	}
	select {
	case <-ranAfter:
	case <-time.After(time.Second):
		t.Fatal("post-panic submit never ran — panic leaked a slot")
	}
	_ = q.Shutdown(context.Background())
}
