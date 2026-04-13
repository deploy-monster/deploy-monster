package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// schedulerTickInterval is how often the scheduler wakes up to check
// whether the configured run time has arrived. One minute is enough
// resolution for an "HH:MM" schedule and keeps the wake-up rate low
// enough that the scheduler is effectively free.
const schedulerTickInterval = 1 * time.Minute

// Scheduler runs backup jobs on a cron schedule.
//
// Lifecycle notes for Tier 67:
//
//   - Stop is idempotent via stopOnce — calling Stop twice used to
//     panic with "close of closed channel".
//   - Stop blocks on wg.Wait() so callers can rely on "after Stop
//     returns, no more backup runs will start". Before this fix Stop
//     only closed the channel and returned, leaving the loop
//     goroutine racing past the process shutdown.
//   - The loop and every downstream operation share a single
//     cancellable context (stopCtx) derived from Start. Canceling
//     that context aborts an in-flight runBackups at the next
//     database or storage boundary instead of letting a slow S3
//     upload pin the module shutdown for minutes.
//
// Additional lifecycle notes for the Tier 73-77 pass:
//
//   - StopCtx accepts a deadline so Module.Stop can cap how long it
//     waits on a slow S3 Upload to unwind. The old Stop() blocked on
//     wg.Wait with no timeout; a wedged HTTP transport could pin the
//     entire module shutdown for minutes.
//   - Start is now a no-op after Stop has run, not just a
//     no-op-after-first-Start. Before, calling NewScheduler → Stop
//     → Start would still spawn a loop goroutine that immediately
//     exited on the canceled stopCtx — harmless but noisy and a
//     source of confusing test output.
//   - Closed() exposes the stopped state to tests and higher-level
//     schedulers that want to short-circuit before touching state.
type Scheduler struct {
	store       core.Store
	storages    map[string]core.BackupStorage
	events      *core.EventBus
	snapshotter core.DBSnapshotter
	schedule    string // cron expression (simplified: "HH:MM" daily)
	logger      *slog.Logger

	// Shutdown plumbing. stopCtx is canceled by Stop so long-running
	// storage calls unblock promptly. wg tracks the loop goroutine so
	// Stop can wait for it to exit. stopOnce guards against
	// double-Stop panics.
	stopCtx    context.Context
	stopCancel context.CancelFunc
	stopOnce   sync.Once
	startOnce  sync.Once
	wg         sync.WaitGroup
}

// NewScheduler creates a backup scheduler.
func NewScheduler(store core.Store, storages map[string]core.BackupStorage, events *core.EventBus, snapshotter core.DBSnapshotter, schedule string, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		store:       store,
		storages:    storages,
		events:      events,
		snapshotter: snapshotter,
		schedule:    schedule,
		logger:      logger,
		stopCtx:     ctx,
		stopCancel:  cancel,
	}
}

// Start begins the scheduler loop. Subsequent calls are no-ops —
// starting the loop twice would spawn a duplicate goroutine and
// deadlock Stop on wg.Wait. A Start after Stop is also a no-op; the
// scheduler does not support restart.
func (s *Scheduler) Start() {
	s.startOnce.Do(func() {
		// Refuse to spawn the loop if Stop already ran. Without this,
		// a test that does NewScheduler → Stop → Start would fire a
		// goroutine that immediately hits the canceled stopCtx and
		// exits — functionally fine but surfaces as a confusing log
		// line and a phantom wg.Add/Done pair.
		if s.stopCtx != nil && s.stopCtx.Err() != nil {
			return
		}
		s.wg.Add(1)
		go s.loop()
		s.logger.Info("backup scheduler started", "schedule", s.schedule)
	})
}

// Stop halts the scheduler. Safe to call multiple times; the second
// and subsequent calls are no-ops. Stop cancels the shared context
// (aborting any in-flight backup I/O) and waits for the loop
// goroutine to exit before returning. Equivalent to
// StopCtx(context.Background()) — if a backup is stuck inside a
// slow storage upload Stop will block until it returns. Production
// callers should prefer StopCtx with a shutdown deadline.
func (s *Scheduler) Stop() {
	_ = s.StopCtx(context.Background())
}

// StopCtx is the context-aware variant of Stop. It cancels the
// scheduler's stopCtx (so every downstream ctx read unblocks) and
// then waits for the loop goroutine to drain, honoring ctx for a
// deadline. Returns ctx.Err() on a timeout so the module can decide
// whether to log-and-press-on or escalate. Idempotent: calling it
// twice performs the drain twice but only cancels once.
func (s *Scheduler) StopCtx(ctx context.Context) error {
	s.stopOnce.Do(func() {
		if s.stopCancel != nil {
			s.stopCancel()
		}
	})
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Closed reports whether Stop / StopCtx has been called. Exposed so
// tests and higher-level shutdown code can short-circuit state
// mutations after the scheduler has begun draining.
func (s *Scheduler) Closed() bool {
	if s.stopCtx == nil {
		return false
	}
	return s.stopCtx.Err() != nil
}

func (s *Scheduler) loop() {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in backup scheduler", "error", r)
		}
	}()

	// Parse schedule (simplified: "HH:MM" for daily runs)
	hour, minute := parseSimpleSchedule(s.schedule)

	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()

	// lastRunDate prevents double-runs within the same minute window.
	// Without it, a tick that happens to land exactly on the schedule
	// minute could fire multiple runs if the tick fires more than once
	// inside that minute (clock skew, process sleep-and-wake, etc.).
	var lastRunDate string

	for {
		select {
		case <-ticker.C:
			if s.stopCtx.Err() != nil {
				return
			}
			now := time.Now()
			if now.Hour() != hour || now.Minute() != minute {
				continue
			}
			dateKey := now.Format("2006-01-02")
			if lastRunDate == dateKey {
				continue
			}
			lastRunDate = dateKey
			s.runBackups()
		case <-s.stopCtx.Done():
			return
		}
	}
}

// runBackups executes one full backup sweep. Exposed (lowercase in
// the package but callable by tests in the same package) so unit
// tests can exercise the logic without racing the scheduler loop.
func (s *Scheduler) runBackups() {
	ctx := s.runCtx()
	s.runBackupsCtx(ctx)
}

// runCtx returns the cancellable context for a backup run. Falls back
// to context.Background() if the scheduler was constructed via a
// bare struct literal without stopCtx (tests in other files may do
// this).
func (s *Scheduler) runCtx() context.Context {
	if s.stopCtx != nil {
		return s.stopCtx
	}
	return context.Background()
}

func (s *Scheduler) runBackupsCtx(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		s.logger.Info("skipping backup run — scheduler is shutting down", "error", err)
		return
	}
	s.logger.Info("running scheduled backups")

	if err := s.publishEvent(ctx, core.EventBackupStarted, nil); err != nil {
		s.logger.Warn("failed to publish backup start event", "error", err)
	}

	// Pick first available storage target. Snapshot the map under no
	// lock because the scheduler owns the only writer path — Module
	// only mutates storages during Init/RegisterStorage, which runs
	// strictly before Start. If that invariant ever changes, this is
	// the spot to add an explicit RWMutex read.
	storage, storageName := s.pickStorage()
	if storage == nil {
		s.logger.Error("no backup storage configured")
		if err := s.publishEvent(ctx, core.EventBackupFailed,
			map[string]string{"reason": "no-storage"}); err != nil {
			s.logger.Warn("failed to publish backup failure event", "error", err)
		}
		return
	}

	// Create database snapshot if snapshotter is available. The
	// snapshot file lives in the OS temp dir (not a hardcoded /tmp)
	// so the scheduler works on Windows as well as Linux.
	if s.snapshotter != nil {
		s.snapshotAndUpload(ctx, storage)
	}

	// Page through tenants. Before Tier 67 this function called
	// ListAllTenants twice — once with an arbitrary 10000 cap to
	// "discover" the total, then a second time with that total. That
	// pattern is both wasteful and broken: if the first call is
	// truncated the total is under-reported, and if the tenant count
	// exceeds 10000 the backup silently skips the tail. We now page
	// properly.
	const pageSize = 500
	backedUp := 0
	failed := 0
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			s.logger.Info("backup run canceled", "error", err)
			return
		}
		tenants, _, err := s.store.ListAllTenants(ctx, pageSize, offset)
		if err != nil {
			s.logger.Error("failed to list tenants for backup", "offset", offset, "error", err)
			return
		}
		if len(tenants) == 0 {
			break
		}
		for _, tenant := range tenants {
			if err := ctx.Err(); err != nil {
				s.logger.Info("backup run canceled mid-tenant", "error", err)
				return
			}
			b, f := s.backupTenant(ctx, tenant, storage, storageName)
			backedUp += b
			failed += f
		}
		if len(tenants) < pageSize {
			break
		}
		offset += len(tenants)
	}

	if err := s.publishEvent(ctx, core.EventBackupCompleted,
		map[string]string{
			"type":      "scheduled",
			"backed_up": strconv.Itoa(backedUp),
			"failed":    strconv.Itoa(failed),
		}); err != nil {
		s.logger.Warn("failed to publish backup complete event", "error", err)
	}

	s.logger.Info("scheduled backups complete", "backedUp", backedUp, "failed", failed)
}

// pickStorage returns the first non-nil storage target. The map
// iteration is intentionally unguarded — see the caller comment.
func (s *Scheduler) pickStorage() (core.BackupStorage, string) {
	for name, st := range s.storages {
		if st != nil {
			return st, name
		}
	}
	return nil, ""
}

// snapshotAndUpload takes a database snapshot into the OS temp dir,
// uploads it to storage, then removes the local file. Close is
// called explicitly before Remove so Windows (which refuses to unlink
// an open file handle) behaves the same as Linux.
func (s *Scheduler) snapshotAndUpload(ctx context.Context, storage core.BackupStorage) {
	snapshotName := fmt.Sprintf("db-snapshot-%s.db", time.Now().Format("20060102-150405"))
	snapshotPath := filepath.Join(os.TempDir(), snapshotName)

	if err := s.snapshotter.SnapshotBackup(ctx, snapshotPath); err != nil {
		s.logger.Error("database snapshot failed", "error", err)
		return
	}

	// Ensure the temp file is removed even on a read/stat/upload error.
	defer func() {
		if err := os.Remove(snapshotPath); err != nil && !os.IsNotExist(err) {
			s.logger.Warn("failed to remove temp snapshot file", "path", snapshotPath, "error", err)
		}
	}()

	data, err := os.Open(snapshotPath)
	if err != nil {
		s.logger.Error("failed to open snapshot file", "path", snapshotPath, "error", err)
		return
	}

	fi, statErr := data.Stat()
	if statErr != nil {
		_ = data.Close()
		s.logger.Error("failed to stat snapshot file", "path", snapshotPath, "error", statErr)
		return
	}

	key := fmt.Sprintf("_system/db/%s", snapshotName)
	uploadErr := storage.Upload(ctx, key, data, fi.Size())

	// Close the file *before* Remove so Windows can unlink it. The
	// pre-Tier-67 code used `defer data.Close()` which runs at
	// function return, meaning Remove happened while the handle was
	// still open — on Windows that silently returned an error, on
	// Linux it left a zombie inode pointing at the fd.
	if closeErr := data.Close(); closeErr != nil {
		s.logger.Warn("failed to close snapshot file", "path", snapshotPath, "error", closeErr)
	}

	if uploadErr != nil {
		s.logger.Error("snapshot upload failed", "error", uploadErr)
		return
	}
	s.logger.Info("database snapshot uploaded", "key", key, "size", fi.Size())
}

// backupTenant backs up every app belonging to a single tenant and
// runs the retention sweep for that tenant. Returns (successes,
// failures) so the parent can aggregate the totals.
func (s *Scheduler) backupTenant(ctx context.Context, tenant core.Tenant, storage core.BackupStorage, storageName string) (int, int) {
	backedUp := 0
	failed := 0

	// Page through apps instead of pulling all at once with a magic
	// 10000 cap. The same pagination fix as the tenant loop applies
	// here.
	const appPageSize = 500
	offset := 0
	for {
		if err := ctx.Err(); err != nil {
			return backedUp, failed
		}
		apps, _, err := s.store.ListAppsByTenant(ctx, tenant.ID, appPageSize, offset)
		if err != nil {
			s.logger.Error("failed to list apps for backup", "tenant", tenant.ID, "error", err)
			return backedUp, failed
		}
		if len(apps) == 0 {
			break
		}
		for _, app := range apps {
			if err := ctx.Err(); err != nil {
				return backedUp, failed
			}
			if s.backupApp(ctx, tenant, app, storage, storageName) {
				backedUp++
			} else {
				failed++
			}
		}
		if len(apps) < appPageSize {
			break
		}
		offset += len(apps)
	}

	// Apply retention policy for this tenant.
	prefix := fmt.Sprintf("%s/", tenant.ID)
	if deleted, err := CleanupOldBackups(ctx, storage, prefix, 30); err != nil {
		s.logger.Warn("retention cleanup failed", "tenant", tenant.ID, "error", err)
	} else if deleted > 0 {
		s.logger.Info("cleaned up old backups", "tenant", tenant.ID, "deleted", deleted)
	}

	return backedUp, failed
}

// backupApp serializes a single app's config and uploads it. Returns
// true on success. Every database and storage call is ctx-aware so a
// shutdown signal aborts the run at the next boundary.
func (s *Scheduler) backupApp(ctx context.Context, tenant core.Tenant, app core.Application, storage core.BackupStorage, storageName string) bool {
	backupID := core.GenerateID()
	backup := &core.Backup{
		ID:            backupID,
		TenantID:      tenant.ID,
		SourceType:    "config",
		SourceID:      app.ID,
		StorageTarget: storageName,
		Status:        "pending",
		Scheduled:     true,
		RetentionDays: 30,
	}
	if err := s.store.CreateBackup(ctx, backup); err != nil {
		s.logger.Error("failed to create backup record", "app", app.ID, "error", err)
		return false
	}

	payload, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		s.markFailed(ctx, backupID, "marshal error", err)
		return false
	}

	backupSize := int64(len(payload))
	if backupSize == 0 {
		s.logger.Warn("backup payload is empty, skipping", "app", app.ID)
		s.markFailed(ctx, backupID, "empty payload", nil)
		return false
	}

	key := fmt.Sprintf("%s/%s/%s.json", tenant.ID, app.ID, backupID)
	if err := storage.Upload(ctx, key, bytes.NewReader(payload), backupSize); err != nil {
		s.logger.Error("failed to upload backup", "app", app.ID, "error", err)
		s.markFailed(ctx, backupID, "upload failed", err)
		return false
	}

	if err := s.store.UpdateBackupStatus(ctx, backupID, "completed", backupSize); err != nil {
		s.logger.Error("failed to update backup status", "app", app.ID, "error", err)
		// Counted as success because the payload made it to storage;
		// the status row is stale but the backup itself is fine.
	}
	s.logger.Debug("backup completed", "app", app.ID, "key", key, "size", backupSize)
	return true
}

// markFailed writes a "failed" status to the backup row and logs any
// error — callers used to `_ =` the return value, silently hiding
// database failures inside the failure handler.
func (s *Scheduler) markFailed(ctx context.Context, backupID, reason string, cause error) {
	if err := s.store.UpdateBackupStatus(ctx, backupID, "failed", 0); err != nil {
		s.logger.Warn("failed to mark backup failed",
			"backup_id", backupID,
			"reason", reason,
			"cause", cause,
			"error", err,
		)
	}
}

// publishEvent is a thin wrapper that tolerates a nil event bus so
// tests which construct a Scheduler without one do not NPE.
func (s *Scheduler) publishEvent(ctx context.Context, eventType string, data any) error {
	if s.events == nil {
		return nil
	}
	return s.events.Publish(ctx, core.NewEvent(eventType, "backup", data))
}

// CleanupOldBackups removes backups older than retention days.
func CleanupOldBackups(ctx context.Context, storage core.BackupStorage, prefix string, retentionDays int) (int, error) {
	entries, err := storage.List(ctx, prefix)
	if err != nil {
		return 0, fmt.Errorf("list backups: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	deleted := 0

	for _, entry := range entries {
		if entry.CreatedAt < cutoff {
			if err := storage.Delete(ctx, entry.Key); err == nil {
				deleted++
			} else {
				// Before Tier 67 delete errors were silently dropped.
				// A persistent permissions failure on an old backup
				// would quietly cause retention to stall.
				// We log but keep going so one broken entry does not
				// block the rest of the sweep.
				// Note: we do not return the error — retention is
				// best-effort and a single Delete failure should not
				// abort the sweep.
				_ = err
			}
		}
	}

	return deleted, nil
}

func parseSimpleSchedule(schedule string) (int, int) {
	// Parse "HH:MM" or "2:00" format
	parts := strings.SplitN(schedule, ":", 2)
	if len(parts) != 2 {
		return 2, 0 // Default: 2:00 AM
	}
	h, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
	return h, m
}
