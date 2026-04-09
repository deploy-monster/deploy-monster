package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Scheduler runs backup jobs on a cron schedule.
type Scheduler struct {
	store       core.Store
	storages    map[string]core.BackupStorage
	events      *core.EventBus
	snapshotter core.DBSnapshotter
	schedule    string // cron expression (simplified: "HH:MM" daily)
	logger      *slog.Logger
	stopCh      chan struct{}
}

// NewScheduler creates a backup scheduler.
func NewScheduler(store core.Store, storages map[string]core.BackupStorage, events *core.EventBus, snapshotter core.DBSnapshotter, schedule string, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		store:       store,
		storages:    storages,
		events:      events,
		snapshotter: snapshotter,
		schedule:    schedule,
		logger:      logger,
		stopCh:      make(chan struct{}),
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

	_ = s.events.Publish(ctx, core.NewEvent(core.EventBackupStarted, "backup", nil))

	// Pick first available storage target
	var storage core.BackupStorage
	var storageName string
	for name, st := range s.storages {
		if st != nil {
			storage = st
			storageName = name
			break
		}
	}
	if storage == nil {
		s.logger.Error("no backup storage configured")
		return
	}

	// Create database snapshot if snapshotter is available
	if s.snapshotter != nil {
		snapshotName := fmt.Sprintf("db-snapshot-%s.db", time.Now().Format("20060102-150405"))
		snapshotPath := fmt.Sprintf("/tmp/%s", snapshotName)
		if err := s.snapshotter.SnapshotBackup(ctx, snapshotPath); err != nil {
			s.logger.Error("database snapshot failed", "error", err)
		} else {
			// Upload snapshot to storage
			if storage != nil {
				key := fmt.Sprintf("_system/db/%s", snapshotName)
				if data, readErr := os.Open(snapshotPath); readErr == nil {
					defer data.Close()
					if fi, statErr := data.Stat(); statErr == nil {
						if uploadErr := storage.Upload(ctx, key, data, fi.Size()); uploadErr != nil {
							s.logger.Error("snapshot upload failed", "error", uploadErr)
						} else {
							s.logger.Info("database snapshot uploaded", "key", key, "size", fi.Size())
						}
					}
				}
				os.Remove(snapshotPath)
			}
		}
	}

	// Iterate all tenants and their apps
	_, totalTenants, err := s.store.ListAllTenants(ctx, 10000, 0)
	if err != nil {
		s.logger.Error("failed to list tenants for backup", "error", err)
		return
	}
	tenants, _, err := s.store.ListAllTenants(ctx, totalTenants, 0)
	if err != nil {
		s.logger.Error("failed to list tenants for backup", "error", err)
		return
	}

	backedUp := 0
	failed := 0
	for _, tenant := range tenants {
		apps, _, err := s.store.ListAppsByTenant(ctx, tenant.ID, 10000, 0)
		if err != nil {
			s.logger.Error("failed to list apps for backup", "tenant", tenant.ID, "error", err)
			continue
		}

		for _, app := range apps {
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
				failed++
				continue
			}

			// Serialize app config as backup payload
			payload, err := json.MarshalIndent(app, "", "  ")
			if err != nil {
				_ = s.store.UpdateBackupStatus(ctx, backupID, "failed", 0)
				failed++
				continue
			}

			key := fmt.Sprintf("%s/%s/%s.json", tenant.ID, app.ID, backupID)
			backupSize := int64(len(payload))
			if backupSize == 0 {
				s.logger.Warn("backup payload is empty, skipping", "app", app.ID)
				_ = s.store.UpdateBackupStatus(ctx, backupID, "failed", 0)
				failed++
				continue
			}

			if err := storage.Upload(ctx, key, bytes.NewReader(payload), backupSize); err != nil {
				s.logger.Error("failed to upload backup", "app", app.ID, "error", err)
				_ = s.store.UpdateBackupStatus(ctx, backupID, "failed", 0)
				failed++
				continue
			}

			if err := s.store.UpdateBackupStatus(ctx, backupID, "completed", backupSize); err != nil {
				s.logger.Error("failed to update backup status", "app", app.ID, "error", err)
			}
			s.logger.Debug("backup completed", "app", app.ID, "key", key, "size", backupSize)
			backedUp++
		}

		// Apply retention policy for this tenant
		prefix := fmt.Sprintf("%s/", tenant.ID)
		if deleted, err := CleanupOldBackups(ctx, storage, prefix, 30); err == nil && deleted > 0 {
			s.logger.Info("cleaned up old backups", "tenant", tenant.ID, "deleted", deleted)
		}
	}

	_ = s.events.Publish(ctx, core.NewEvent(core.EventBackupCompleted, "backup",
		map[string]string{"type": "scheduled", "backed_up": strconv.Itoa(backedUp), "failed": strconv.Itoa(failed)}))

	s.logger.Info("scheduled backups complete", "backedUp", backedUp, "failed", failed)
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
