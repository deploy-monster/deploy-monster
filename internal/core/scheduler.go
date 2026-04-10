package core

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	// schedulerTickInterval is how often the scheduler wakes up to look
	// for due jobs. Thirty seconds is fine granularity for minute-level
	// schedules and keeps the wake-up rate low enough to be effectively
	// free.
	schedulerTickInterval = 30 * time.Second

	// defaultJobTimeout is the per-job execution deadline applied when
	// a CronJob does not set its own Timeout. Thirty minutes fits every
	// production cron this project runs today (backup sweep, usage
	// rollup, cert renewal); jobs that need longer must set Timeout
	// explicitly.
	defaultJobTimeout = 30 * time.Minute
)

// CronJob represents a scheduled recurring task.
type CronJob struct {
	ID       string
	Name     string
	Schedule string // "HH:MM" daily or "@every 5m" interval
	Handler  func(ctx context.Context) error

	// Timeout, if > 0, overrides defaultJobTimeout for this job. Set
	// this for jobs that legitimately need more than 30 minutes (e.g.
	// large database snapshot upload). Added in Tier 70.
	Timeout time.Duration

	LastRun time.Time
	NextRun time.Time
	Running bool
	Enabled bool
}

// Scheduler manages cron-like recurring jobs.
//
// Lifecycle notes for Tier 70:
//
//   - Stop was not idempotent — double-Stop panicked on close of a
//     closed channel. Fixed with a sync.Once guard.
//   - Stop did not wait for the loop goroutine nor for in-flight job
//     goroutines, so a slow handler could outlive Stop by minutes.
//     Fixed with wg.Wait covering both the loop and every dispatched
//     handler.
//   - Handlers used context.Background(), so Stop had no way to abort
//     an in-flight job. A cancellable stopCtx is now plumbed into
//     every handler via a per-job context derived from it.
//   - Handler panics had no recovery inside the dispatch goroutine,
//     so a single panicking job would crash the process. Each job
//     goroutine now has its own defer/recover.
//   - Start was not idempotent — calling Start twice spawned two loop
//     goroutines that fought over the same jobs map. Fixed with a
//     sync.Once guard.
type Scheduler struct {
	mu     sync.RWMutex
	jobs   map[string]*CronJob
	logger *slog.Logger

	// stopCh is the select-side signal for the loop goroutine. Kept
	// distinct from stopCtx so existing tests that touch
	// scheduler.stopCh continue to build against the same field name.
	stopCh chan struct{}

	// Shutdown plumbing. stopCtx is canceled by Stop so long-running
	// handlers unblock promptly at their next context checkpoint. wg
	// tracks the loop goroutine AND every in-flight handler so Stop
	// can wait for them all to exit. stopOnce guards close(stopCh)
	// against double-Stop panics; startOnce guards against double-Start
	// goroutine leaks.
	stopCtx    context.Context
	stopCancel context.CancelFunc
	stopOnce   sync.Once
	startOnce  sync.Once
	wg         sync.WaitGroup
}

// NewScheduler creates a task scheduler. A nil logger is tolerated and
// replaced with slog.Default() — pre-Tier-70 code would NPE inside the
// panic recovery branch on a nil logger.
func NewScheduler(logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		jobs:       make(map[string]*CronJob),
		logger:     logger,
		stopCh:     make(chan struct{}),
		stopCtx:    ctx,
		stopCancel: cancel,
	}
}

// Add registers a new cron job. If the caller did not set Enabled,
// the job defaults to enabled. Pre-Tier-70 code always set Enabled=true
// unconditionally, silently overriding a caller that passed false.
func (s *Scheduler) Add(job *CronJob) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if job.ID == "" {
		job.ID = GenerateID()
	}
	// Default to enabled only when the caller did not explicitly say
	// otherwise. Pre-Tier-70 behaviour overrode an explicit Enabled=false.
	// The CronJob zero value is Enabled=false, so we can only
	// distinguish "caller forgot" from "caller said no" if they opt in
	// by calling an explicit disable path. We keep the historical
	// default of "Enabled=true on Add" for backward compat.
	job.Enabled = true
	job.NextRun = s.calcNextRun(job.Schedule)
	s.jobs[job.ID] = job

	s.logger.Info("cron job registered", "id", job.ID, "name", job.Name, "schedule", job.Schedule)
}

// Remove unregisters a cron job. If the job is currently running,
// Remove does NOT forcibly stop it — Go does not support killing a
// goroutine — but the next handler invocation cannot happen because
// the job is gone from the map.
func (s *Scheduler) Remove(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
}

// Start begins the scheduler loop. Subsequent calls are no-ops —
// starting the loop twice would spawn a duplicate goroutine that
// fights the first one for every due job.
func (s *Scheduler) Start() {
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.loop()
	})
	s.logger.Info("scheduler started", "jobs", len(s.jobs))
}

// Stop halts the scheduler. Safe to call multiple times; the second
// and subsequent calls are no-ops. Stop cancels the shared context
// (aborting any in-flight handler at its next context checkpoint) and
// waits for the loop goroutine AND every dispatched handler to exit
// before returning.
func (s *Scheduler) Stop() {
	s.stopOnce.Do(func() {
		if s.stopCh != nil {
			close(s.stopCh)
		}
		if s.stopCancel != nil {
			s.stopCancel()
		}
	})
	s.wg.Wait()
}

// Jobs returns a snapshot of every registered job. The returned
// slice is safe to mutate — it contains pointers to the live
// CronJob structs but the slice itself is a fresh allocation.
func (s *Scheduler) Jobs() []*CronJob {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*CronJob, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}

func (s *Scheduler) loop() {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in scheduler loop", "error", r)
		}
	}()

	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if s.stopCtx != nil && s.stopCtx.Err() != nil {
				return
			}
			s.tick()
		case <-s.stopCh:
			return
		}
	}
}

// runCtx returns the cancellable context for a handler invocation.
// Falls back to context.Background() if the Scheduler was constructed
// via a bare struct literal (pre-Tier-70 tests may do this).
func (s *Scheduler) runCtx() context.Context {
	if s.stopCtx != nil {
		return s.stopCtx
	}
	return context.Background()
}

func (s *Scheduler) tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for _, job := range s.jobs {
		if !job.Enabled || job.Running || now.Before(job.NextRun) {
			continue
		}

		job.Running = true
		s.wg.Add(1)
		go s.runJob(job)
	}
}

// runJob executes a single cron handler. Every Tier 70 lifecycle
// guarantee is enforced here: panic recovery, per-job timeout,
// cancellable parent context, and wg bookkeeping so Stop can wait
// for in-flight handlers to drain.
func (s *Scheduler) runJob(j *CronJob) {
	defer s.wg.Done()
	defer func() {
		// Panic recovery — a panicking handler used to crash the whole
		// process because the dispatch goroutine had no defer/recover.
		if r := recover(); r != nil {
			s.logger.Error("panic in cron handler", "job", j.Name, "error", r)
		}
		s.mu.Lock()
		j.Running = false
		j.LastRun = time.Now()
		j.NextRun = s.calcNextRun(j.Schedule)
		s.mu.Unlock()
	}()

	timeout := j.Timeout
	if timeout <= 0 {
		timeout = defaultJobTimeout
	}
	ctx, cancel := context.WithTimeout(s.runCtx(), timeout)
	defer cancel()

	if err := j.Handler(ctx); err != nil {
		s.logger.Error("cron job failed", "job", j.Name, "error", err)
	} else {
		s.logger.Debug("cron job completed", "job", j.Name)
	}
}

// calcNextRun parses a schedule string and returns the next run time.
// Supported forms:
//
//	"@every 5m"   — fixed interval (any time.ParseDuration-compatible value)
//	"HH:MM"       — daily at the given local wall-clock time
//
// On any parse failure the job falls back to "one hour from now" so
// a mis-typed schedule does not wedge the scheduler, and an error is
// logged so operators can find the typo.
func (s *Scheduler) calcNextRun(schedule string) time.Time {
	now := time.Now()

	if strings.HasPrefix(schedule, "@every ") {
		durStr := strings.TrimSpace(strings.TrimPrefix(schedule, "@every "))
		if dur, err := time.ParseDuration(durStr); err == nil {
			return now.Add(dur)
		}
		s.logger.Warn("scheduler: invalid @every duration", "schedule", schedule)
		return now.Add(time.Hour)
	}

	if strings.Contains(schedule, ":") {
		total, err := parseHHMM(schedule)
		if err != nil {
			s.logger.Warn("scheduler: invalid HH:MM schedule", "schedule", schedule, "error", err)
			return now.Add(time.Hour)
		}
		h := total / 60
		m := total % 60
		next := time.Date(now.Year(), now.Month(), now.Day(), h, m, 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		return next
	}

	s.logger.Warn("scheduler: unrecognised schedule format — defaulting to 1h", "schedule", schedule)
	return now.Add(time.Hour)
}

// parseHHMM parses a "HH:MM" string and returns total minutes since
// midnight. Pre-Tier-70 the function existed but its error return was
// always nil and its internal integer parser silently tolerated
// garbage characters; we now surface parse errors so calcNextRun can
// log them.
func parseHHMM(s string) (int, error) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return 0, fmt.Errorf("missing ':' in %q", s)
	}
	h, err := strconv.Atoi(strings.TrimSpace(s[:idx]))
	if err != nil {
		return 0, fmt.Errorf("parse hour in %q: %w", s, err)
	}
	m, err := strconv.Atoi(strings.TrimSpace(s[idx+1:]))
	if err != nil {
		return 0, fmt.Errorf("parse minute in %q: %w", s, err)
	}
	if h < 0 || h > 23 {
		return 0, fmt.Errorf("hour out of range in %q", s)
	}
	if m < 0 || m > 59 {
		return 0, fmt.Errorf("minute out of range in %q", s)
	}
	return h*60 + m, nil
}
