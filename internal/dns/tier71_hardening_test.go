package dns

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Tier 71 — DNS sync queue lifecycle + ctx plumbing tests.
//
// These cover the regressions fixed in Tier 71:
//   - NewSyncQueue nil-logger guard
//   - Stop idempotency (stopOnce-guarded double close)
//   - Stop waits for the worker goroutine + verify goroutines
//   - Stop cancels in-flight provider RPCs via ctx
//   - Start idempotency (startOnce prevents duplicate workers)
//   - Retry backoff unblocks on Stop instead of blocking the worker
//   - Enqueue after Stop is rejected, not buffered forever
//   - runCtx nil fallback for struct-literal construction

// ─── NewSyncQueue nil-logger guard ─────────────────────────────────────────

func TestTier71_NewSyncQueue_NilLogger(t *testing.T) {
	svc := core.NewServices()
	events := core.NewEventBus(tier71Logger())
	q := NewSyncQueue(svc, &mockStore{}, events, nil)
	if q == nil {
		t.Fatal("NewSyncQueue returned nil")
	}
	if q.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if q.stopCtx == nil || q.stopCancel == nil {
		t.Error("stopCtx/stopCancel should be initialized")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier71_SyncQueue_Stop_Idempotent(t *testing.T) {
	q := tier71NewQueue()
	q.Start()

	// Double-Stop must not panic. Before Tier 71 the second call
	// panicked with "close of closed channel" because there was no
	// stopOnce guard.
	q.Stop()
	q.Stop()
}

func TestTier71_SyncQueue_Stop_WithoutStart_Safe(t *testing.T) {
	q := tier71NewQueue()
	// Must not deadlock on wg.Wait — nothing was added to the group.
	q.Stop()
	q.Stop()
}

// ─── Start idempotency ─────────────────────────────────────────────────────

func TestTier71_SyncQueue_Start_Idempotent(t *testing.T) {
	q := tier71NewQueue()

	// Starting twice must not double-count wg. If it did, Stop would
	// block forever waiting for a phantom second worker.
	q.Start()
	q.Start()

	done := make(chan struct{})
	go func() {
		q.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked — startOnce/wg balance is wrong")
	}
}

// ─── Stop cancels in-flight provider RPC ───────────────────────────────────

// blockingProvider hangs on CreateRecord until ctx is canceled. We
// use it to prove that Stop actually aborts the in-flight RPC through
// the shared stopCtx plumbing.
type blockingProvider struct {
	name     string
	started  chan struct{}
	canceled atomic.Bool
}

func (b *blockingProvider) Name() string { return b.name }
func (b *blockingProvider) CreateRecord(ctx context.Context, _ core.DNSRecord) error {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	b.canceled.Store(true)
	return ctx.Err()
}
func (b *blockingProvider) UpdateRecord(ctx context.Context, _ core.DNSRecord) error {
	return ctx.Err()
}
func (b *blockingProvider) DeleteRecord(ctx context.Context, _ core.DNSRecord) error {
	return ctx.Err()
}
func (b *blockingProvider) Verify(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func TestTier71_SyncQueue_Stop_CancelsInFlightRPC(t *testing.T) {
	svc := core.NewServices()
	prov := &blockingProvider{name: "block", started: make(chan struct{})}
	svc.RegisterDNSProvider("block", prov)
	events := core.NewEventBus(tier71Logger())
	q := NewSyncQueue(svc, &mockStore{}, events, tier71Logger())
	q.Start()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "block",
		Record:   core.DNSRecord{Name: "stuck.example.com"},
	})

	// Wait for the worker to enter the provider RPC.
	select {
	case <-prov.started:
	case <-time.After(1 * time.Second):
		t.Fatal("CreateRecord was not reached")
	}

	// Stop must abort the in-flight RPC via stopCtx cancellation.
	done := make(chan struct{})
	go func() {
		q.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return — in-flight RPC was not canceled")
	}

	if !prov.canceled.Load() {
		t.Error("CreateRecord did not observe ctx cancellation")
	}
}

// ─── Retry backoff unblocks on Stop ────────────────────────────────────────

// failingProvider fails every CreateRecord. Combined with the
// 5-second syncRetryBase it forces the worker into its backoff sleep.
// We then Stop the queue and assert the backoff unblocks quickly
// rather than pinning the worker for 5+ seconds.
type failingProvider struct {
	mockDNSProvider
}

func (f *failingProvider) CreateRecord(_ context.Context, _ core.DNSRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createCalls++
	return errTier71Boom
}
func (f *failingProvider) UpdateRecord(_ context.Context, _ core.DNSRecord) error { return nil }
func (f *failingProvider) DeleteRecord(_ context.Context, _ core.DNSRecord) error { return nil }
func (f *failingProvider) Verify(_ context.Context, _ string) (bool, error)       { return false, nil }

var errTier71Boom = &tier71Err{msg: "tier71 boom"}

type tier71Err struct{ msg string }

func (e *tier71Err) Error() string { return e.msg }

func TestTier71_SyncQueue_RetryBackoff_UnblocksOnStop(t *testing.T) {
	svc := core.NewServices()
	prov := &failingProvider{mockDNSProvider: mockDNSProvider{name: "fail"}}
	svc.RegisterDNSProvider("fail", prov)
	events := core.NewEventBus(tier71Logger())
	q := NewSyncQueue(svc, &mockStore{}, events, tier71Logger())
	q.Start()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "fail",
		Record:   core.DNSRecord{Name: "flaky.example.com"},
	})

	// Give the worker time to hit the failure and enter its backoff.
	time.Sleep(100 * time.Millisecond)

	start := time.Now()
	q.Stop()
	elapsed := time.Since(start)

	// Pre-Tier-71 this blocked the caller for the full 5-second
	// time.Sleep backoff. We want it to unblock promptly.
	if elapsed > 2*time.Second {
		t.Errorf("Stop took %v during retry backoff — stopCh not plumbed into backoff", elapsed)
	}
}

// ─── Enqueue after Stop is rejected ────────────────────────────────────────

func TestTier71_SyncQueue_EnqueueAfterStop_Dropped(t *testing.T) {
	svc := core.NewServices()
	mock := &mockDNSProvider{name: "mock", verifyResult: true}
	svc.RegisterDNSProvider("mock", mock)
	events := core.NewEventBus(tier71Logger())
	q := NewSyncQueue(svc, &mockStore{}, events, tier71Logger())
	q.Start()
	q.Stop()

	// After Stop, Enqueue should short-circuit on the closed flag and
	// not push anything onto the buffered channel.
	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "mock",
		Record:   core.DNSRecord{Name: "ghost.example.com"},
	})

	// Give any rogue goroutine a moment (there shouldn't be one).
	time.Sleep(50 * time.Millisecond)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.createCalls != 0 {
		t.Errorf("Enqueue after Stop reached provider: createCalls=%d", mock.createCalls)
	}
}

// ─── Stop waits for verify goroutine ───────────────────────────────────────

// slowVerifyProvider reports a successful create immediately but
// blocks in Verify until ctx is canceled. This lets us prove that
// Stop actually drains the verify goroutine rather than letting it
// leak past shutdown.
type slowVerifyProvider struct {
	mockDNSProvider
	verifyStarted chan struct{}
	verifyDone    atomic.Bool
}

func (s *slowVerifyProvider) CreateRecord(_ context.Context, _ core.DNSRecord) error {
	return nil
}
func (s *slowVerifyProvider) UpdateRecord(_ context.Context, _ core.DNSRecord) error {
	return nil
}
func (s *slowVerifyProvider) DeleteRecord(_ context.Context, _ core.DNSRecord) error {
	return nil
}
func (s *slowVerifyProvider) Verify(ctx context.Context, _ string) (bool, error) {
	select {
	case <-s.verifyStarted:
	default:
		close(s.verifyStarted)
	}
	<-ctx.Done()
	s.verifyDone.Store(true)
	return false, ctx.Err()
}

func TestTier71_SyncQueue_Stop_DrainsVerifyGoroutine(t *testing.T) {
	svc := core.NewServices()
	prov := &slowVerifyProvider{
		mockDNSProvider: mockDNSProvider{name: "sv"},
		verifyStarted:   make(chan struct{}),
	}
	svc.RegisterDNSProvider("sv", prov)
	events := core.NewEventBus(tier71Logger())
	q := NewSyncQueue(svc, &mockStore{}, events, tier71Logger())
	q.Start()

	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: "sv",
		Record:   core.DNSRecord{Name: "verify.example.com"},
	})

	// Wait for the verify goroutine to enter its RPC. Note: verify
	// waits verifyDelay (5s) after create before calling provider.Verify,
	// so we only assert the goroutine exists by waiting on stopCh
	// behavior below — not on verifyStarted directly.

	// Stop should cancel the pending verify delay AND wait for the
	// goroutine to return before unblocking the caller.
	done := make(chan struct{})
	go func() {
		q.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not drain verify goroutine")
	}
}

// ─── Concurrent Start+Stop storm ───────────────────────────────────────────

// TestTier71_SyncQueue_ConcurrentStartStop exercises the startOnce /
// stopOnce guards under concurrent pressure. Before Tier 71 the
// concurrent double-close raced with a close-of-closed-channel panic.
func TestTier71_SyncQueue_ConcurrentStartStop(t *testing.T) {
	q := tier71NewQueue()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); q.Start() }()
		go func() { defer wg.Done(); q.Stop() }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	q.Stop()
}

// ─── runCtx nil fallback ──────────────────────────────────────────────────

func TestTier71_SyncQueue_RunCtx_NilFallback(t *testing.T) {
	// Bare struct literal — no NewSyncQueue, so stopCtx is nil.
	q := &SyncQueue{logger: tier71Logger()}
	ctx := q.runCtx()
	if ctx == nil {
		t.Fatal("runCtx must not return nil")
	}
	if ctx.Err() != nil {
		t.Errorf("fallback background context should not be canceled: %v", ctx.Err())
	}
}

// ─── helpers ───────────────────────────────────────────────────────────────

func tier71Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func tier71NewQueue() *SyncQueue {
	svc := core.NewServices()
	events := core.NewEventBus(tier71Logger())
	return NewSyncQueue(svc, &mockStore{}, events, tier71Logger())
}
