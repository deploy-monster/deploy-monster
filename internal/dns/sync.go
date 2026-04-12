package dns

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const (
	// syncJobTimeout bounds a single DNS provider RPC. Thirty seconds
	// fits the slowest observed Route53 propagation-ack round trip and
	// keeps the worker unblocked on flaky upstreams.
	syncJobTimeout = 30 * time.Second

	// verifyDelay is how long we wait after a successful record write
	// before probing the upstream to confirm visibility. Most public
	// resolvers pick up the change within 5 s of the provider API ack.
	verifyDelay = 5 * time.Second

	// verifyTimeout bounds the propagation-check RPC itself.
	verifyTimeout = 10 * time.Second

	// syncRetryBase is the linear backoff unit between retries. A job
	// that has failed N times sleeps N*syncRetryBase before re-enqueue.
	// Keep the unit short enough that TestSyncQueue_Process_CreateError_Retry
	// observes a second attempt inside its 7-second window.
	syncRetryBase = 5 * time.Second

	// syncMaxRetries caps retry attempts before a job is abandoned.
	syncMaxRetries = 3
)

// SyncJob represents a pending DNS record operation.
type SyncJob struct {
	ID        string
	Action    string // create, update, delete
	Provider  string
	Record    core.DNSRecord
	DomainID  string
	Retries   int
	LastError string
	CreatedAt time.Time
}

// SyncQueue processes DNS operations asynchronously with retry logic.
//
// Lifecycle notes for Tier 71:
//
//   - Stop used to call close(stopCh) with no sync.Once guard, so a
//     double Stop panicked with "close of closed channel". stopOnce
//     now serializes the close and the stopCtx cancel.
//   - Stop did not wait for the worker goroutine nor for any pending
//     verification goroutines. wg now tracks the worker AND every
//     per-job verification goroutine so Stop drains them all before
//     returning.
//   - process() used context.WithTimeout(context.Background(), 30s),
//     so a Stop in the middle of a slow Cloudflare call could not
//     abort it. The parent is now a shared stopCtx derived in
//     NewSyncQueue and cancelled by Stop.
//   - The verify goroutine also used context.Background(); it now
//     inherits from stopCtx and is tracked by wg.
//   - Retry backoff used time.Sleep(), blocking the single worker
//     goroutine for up to 15 s at a time. It now selects on stopCtx
//     so Stop unblocks a sleeping retry immediately.
//   - Start was not idempotent — a second Start spawned a second
//     worker that raced the first on the jobs channel. Fixed with a
//     startOnce guard.
//   - Enqueue used to accept jobs forever even after Stop, silently
//     accumulating garbage in the buffered channel. It now short-
//     circuits on the closed flag so callers see the drop in the
//     logs and state doesn't grow unbounded.
//   - NewSyncQueue tolerates a nil logger by falling back to
//     slog.Default, matching the Tier 68/69/70 hardening style.
type SyncQueue struct {
	jobs     chan *SyncJob
	services *core.Services
	store    core.Store
	events   *core.EventBus
	logger   *slog.Logger

	// stopCh is the select-side signal for the worker goroutine. Kept
	// as a distinct channel (rather than folding onto stopCtx.Done())
	// so existing tests that assert meter.stopCh != nil keep working
	// without churn.
	stopCh chan struct{}

	// Shutdown plumbing. stopCtx is cancelled by Stop so any in-flight
	// DNS provider RPC, verification probe, or retry backoff unblocks
	// promptly. wg tracks the worker goroutine and every per-job
	// verification goroutine so Stop can wait for all of them to exit.
	//
	// Lifecycle (started/closed) is guarded by mu. Tier 101 replaced
	// the previous startOnce/stopOnce pair because a concurrent
	// Start+Stop could still race on the WaitGroup: Stop's wg.Wait
	// could observe a zero counter and return before Start's wg.Add
	// had executed, reusing a drained WaitGroup. With a mutex-guarded
	// state machine, once Stop has flipped closed=true, Start becomes
	// a no-op before it can touch wg at all.
	stopCtx    context.Context
	stopCancel context.CancelFunc
	wg         sync.WaitGroup

	// mu serialises started/closed with Enqueue and Start/Stop so a
	// Stop that races a concurrent Enqueue cannot accept a job the
	// worker will never see, and a Start that races a concurrent Stop
	// cannot reuse a drained wg.
	mu      sync.Mutex
	started bool
	closed  bool
}

// NewSyncQueue creates a DNS sync queue. A nil logger is tolerated
// and replaced with slog.Default() — pre-Tier-71 code would NPE
// inside the recover branch on a nil logger.
func NewSyncQueue(services *core.Services, store core.Store, events *core.EventBus, logger *slog.Logger) *SyncQueue {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncQueue{
		jobs:       make(chan *SyncJob, 100),
		services:   services,
		store:      store,
		events:     events,
		logger:     logger,
		stopCh:     make(chan struct{}),
		stopCtx:    ctx,
		stopCancel: cancel,
	}
}

// Enqueue adds a DNS operation to the queue. Submissions to a closed
// queue are silently dropped (with a warn log) — the pre-Tier-71
// behaviour of letting jobs pile up in the buffered channel after
// Stop left the queue state unbounded and invisible to operators.
func (q *SyncQueue) Enqueue(job *SyncJob) {
	if job.ID == "" {
		job.ID = core.GenerateID()
	}
	job.CreatedAt = time.Now()

	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		q.logger.Warn("DNS queue closed, dropping job", "record", job.Record.Name, "action", job.Action)
		return
	}
	q.mu.Unlock()

	select {
	case q.jobs <- job:
		q.logger.Info("DNS job queued", "action", job.Action, "record", job.Record.Name)
	default:
		q.logger.Warn("DNS queue full, dropping job", "record", job.Record.Name)
	}
}

// Start begins processing the queue. Subsequent calls are no-ops —
// starting the worker twice would spawn a second goroutine that
// fights the first for every job and leak on Stop. Start is also a
// no-op if Stop has already run, so a concurrent Stop-wins race
// cannot leave an Add-after-Wait reuse bug on wg.
func (q *SyncQueue) Start() {
	q.mu.Lock()
	if q.closed || q.started {
		q.mu.Unlock()
		return
	}
	q.started = true
	q.wg.Add(1)
	q.mu.Unlock()
	go q.loop()
}

// Stop halts the queue. Safe to call multiple times; the second and
// subsequent calls are no-ops. Stop cancels the shared context
// (aborting any in-flight DNS RPC, verification probe, or retry
// backoff) and waits for the worker AND every dispatched
// verification goroutine to exit before returning.
func (q *SyncQueue) Stop() {
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		q.wg.Wait()
		return
	}
	q.closed = true
	started := q.started
	if q.stopCh != nil {
		close(q.stopCh)
	}
	if q.stopCancel != nil {
		q.stopCancel()
	}
	q.mu.Unlock()
	if started {
		q.wg.Wait()
	}
}

// runCtx returns the cancellable parent context for a provider RPC.
// Falls back to context.Background() if the SyncQueue was
// constructed via a bare struct literal (some older tests may do
// this).
func (q *SyncQueue) runCtx() context.Context {
	if q.stopCtx != nil {
		return q.stopCtx
	}
	return context.Background()
}

func (q *SyncQueue) loop() {
	defer q.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			q.logger.Error("panic in DNS sync queue", "error", r)
		}
	}()
	for {
		select {
		case job := <-q.jobs:
			q.process(job)
		case <-q.stopCh:
			return
		}
	}
}

func (q *SyncQueue) process(job *SyncJob) {
	provider := q.services.DNSProvider(job.Provider)
	if provider == nil {
		q.logger.Error("DNS provider not found", "provider", job.Provider)
		return
	}

	ctx, cancel := context.WithTimeout(q.runCtx(), syncJobTimeout)
	defer cancel()

	var err error
	switch job.Action {
	case "create":
		err = provider.CreateRecord(ctx, job.Record)
	case "update":
		err = provider.UpdateRecord(ctx, job.Record)
	case "delete":
		err = provider.DeleteRecord(ctx, job.Record)
	default:
		q.logger.Error("unknown DNS action", "action", job.Action)
		return
	}

	if err != nil {
		job.Retries++
		job.LastError = err.Error()
		q.logger.Error("DNS sync failed",
			"action", job.Action,
			"record", job.Record.Name,
			"error", err,
			"retries", job.Retries,
		)

		if job.Retries >= syncMaxRetries {
			return
		}
		// Linear backoff that respects shutdown. Pre-Tier-71 code
		// called time.Sleep() directly, which blocked the ONLY worker
		// goroutine for up to 15 s at a time and made Stop effectively
		// hang on a transient upstream outage.
		backoff := time.Duration(job.Retries) * syncRetryBase
		select {
		case <-time.After(backoff):
			q.Enqueue(job)
		case <-q.stopCh:
			return
		}
		return
	}

	q.logger.Info("DNS sync complete",
		"action", job.Action,
		"record", job.Record.Name,
		"provider", job.Provider,
	)

	// Schedule propagation verification. Tracked by wg so Stop drains
	// it before returning, and the probe context is derived from
	// stopCtx so Stop aborts the HTTP round trip instantly.
	q.wg.Add(1)
	go q.verify(provider, job.Record)
}

// verify runs a propagation check for a record and logs the result.
// Any error is swallowed — verification is best-effort, and a failed
// probe should not mark the sync as failed (we've already received
// an API ack from the provider).
func (q *SyncQueue) verify(provider core.DNSProvider, record core.DNSRecord) {
	defer q.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			q.logger.Error("panic in DNS verify", "record", record.Name, "error", r)
		}
	}()

	select {
	case <-time.After(verifyDelay):
	case <-q.stopCh:
		return
	}

	ctx, cancel := context.WithTimeout(q.runCtx(), verifyTimeout)
	defer cancel()
	verified, err := provider.Verify(ctx, record.Name)
	if err != nil {
		q.logger.Debug("DNS verify errored", "record", record.Name, "error", err)
		return
	}
	if verified {
		q.logger.Info("DNS verified", "record", record.Name)
	}
}

// SyncDomainRecords creates DNS A records for a domain pointing to the server IP.
func SyncDomainRecords(q *SyncQueue, fqdn, serverIP, provider string) {
	q.Enqueue(&SyncJob{
		Action:   "create",
		Provider: provider,
		Record: core.DNSRecord{
			Type:  "A",
			Name:  fqdn,
			Value: serverIP,
			TTL:   300,
		},
	})

	// Also create www subdomain
	if fqdn[0] != '*' {
		q.Enqueue(&SyncJob{
			Action:   "create",
			Provider: provider,
			Record: core.DNSRecord{
				Type:  "CNAME",
				Name:  fmt.Sprintf("www.%s", fqdn),
				Value: fqdn,
				TTL:   300,
			},
		})
	}
}
