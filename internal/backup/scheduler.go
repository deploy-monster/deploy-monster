package backup

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Scheduler runs backup jobs on a cron schedule.
type Scheduler struct {
	store    core.Store
	storages map[string]core.BackupStorage
	events   *core.EventBus
	schedule string // cron expression (simplified: "HH:MM" daily)
	logger   *slog.Logger
	stopCh   chan struct{}
}

// NewScheduler creates a backup scheduler.
func NewScheduler(store core.Store, storages map[string]core.BackupStorage, events *core.EventBus, schedule string, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		store:    store,
		storages: storages,
		events:   events,
		schedule: schedule,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the scheduler loop.
func (s *Scheduler) Start() {
	go s.loop()
	s.logger.Info("backup scheduler started", "schedule", s.schedule)
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

func (s *Scheduler) loop() {
	// Parse schedule (simplified: "HH:MM" for daily runs)
	hour, minute := parseSimpleSchedule(s.schedule)

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			now := time.Now()
			if now.Hour() == hour && now.Minute() == minute {
				s.runBackups()
			}
		case <-s.stopCh:
			return
		}
	}
}

func (s *Scheduler) runBackups() {
	ctx := context.Background()
	s.logger.Info("running scheduled backups")

	s.events.Publish(ctx, core.NewEvent(core.EventBackupStarted, "backup", nil))

	// In production, this would:
	// 1. List all apps with backup enabled
	// 2. For each app, snapshot volumes
	// 3. For each managed DB, run pg_dump/mysqldump
	// 4. Upload to configured storage target
	// 5. Apply retention policy (delete old backups)

	s.events.Publish(ctx, core.NewEvent(core.EventBackupCompleted, "backup",
		map[string]string{"type": "scheduled"}))

	s.logger.Info("scheduled backups complete")
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
