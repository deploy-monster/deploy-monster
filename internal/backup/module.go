package backup

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

var ErrBackupNotReady = errors.New("backup engine not ready")

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

type Module struct {
	core      *core.Core
	store     core.Store
	storages  map[string]core.BackupStorage
	scheduler *Scheduler
	mu        sync.RWMutex
	logger    *slog.Logger
}

func New() *Module {
	return &Module{storages: make(map[string]core.BackupStorage)}
}

func (m *Module) ID() string                  { return "backup" }
func (m *Module) Name() string                { return "Backup Engine" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	localPath := c.Config.Backup.StoragePath
	if localPath == "" {
		localPath = "backups"
	}
	var encryptionKey []byte
	if c.Config.Backup.EncryptionKey != "" {
		var err error
		encryptionKey, err = base64.StdEncoding.DecodeString(c.Config.Backup.EncryptionKey)
		if err != nil {
			return fmt.Errorf("invalid backup.encryption_key: %w", err)
		}
	}
	local := NewLocalStorage(localPath, encryptionKey)
	m.RegisterStorage("local", local)
	c.Services.RegisterBackupStorage("local", local)

	if s3Cfg := c.Config.Backup.S3; s3Cfg.Bucket != "" {
		s3 := NewS3Storage(S3Config{
			Endpoint:      s3Cfg.Endpoint,
			Bucket:        s3Cfg.Bucket,
			Region:        s3Cfg.Region,
			AccessKey:     s3Cfg.AccessKey,
			SecretKey:     s3Cfg.SecretKey,
			PathStyle:     s3Cfg.PathStyle,
			EncryptionKey: encryptionKey,
		}, m.logger)
		m.RegisterStorage("s3", s3)
		c.Services.RegisterBackupStorage("s3", s3)
		m.logger.Info("S3 backup storage registered", "bucket", s3Cfg.Bucket)
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	schedule := m.core.Config.Backup.Schedule
	if schedule == "" {
		schedule = "02:00"
	}
	var snapshotter core.DBSnapshotter
	if m.core.DB != nil {
		snapshotter = m.core.DB.Snapshotter
	}
	m.scheduler = NewScheduler(m.store, m.storages, m.core.Events, snapshotter, schedule, m.logger)
	m.scheduler.Start()

	m.logger.Info("backup engine started", "storages", m.StorageNames(), "schedule", schedule)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.scheduler == nil {
		return nil
	}
	if err := m.scheduler.StopCtx(ctx); err != nil {
		m.logger.Warn("backup scheduler drain exceeded shutdown deadline", "error", err)
	}
	return nil
}

func (m *Module) Health() core.HealthStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if len(m.storages) == 0 {
		return core.HealthDegraded
	}
	return core.HealthOK
}

func (m *Module) RegisterStorage(name string, storage core.BackupStorage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storages[name] = storage
}

func (m *Module) StorageNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.storages))
	for name := range m.storages {
		names = append(names, name)
	}
	return names
}

func (m *Module) TriggerNow(_ context.Context) error {
	if m.scheduler == nil {
		return ErrBackupNotReady
	}
	go m.scheduler.runBackups()
	return nil
}

// RestoreBackup downloads a backup and restores the app it belongs to.
// For incremental backups, it finds the preceding full backup and restores
// from that. If the app was deleted, it recreates it from the archive.
func (m *Module) RestoreBackup(ctx context.Context, backupID, tenantID string) error {
	backups, _, err := m.store.ListBackupsByTenant(ctx, tenantID, 1000, 0)
	if err != nil {
		return fmt.Errorf("list backups: %w", err)
	}
	var backup *core.Backup
	for i := range backups {
		if backups[i].ID == backupID {
			backup = &backups[i]
			break
		}
	}
	if backup == nil {
		return fmt.Errorf("backup not found: %s", backupID)
	}
	if backup.Status != "completed" {
		return fmt.Errorf("backup not completed: %s (status=%s)", backupID, backup.Status)
	}

	// For incremental backups, find the preceding full backup.
	targetBackup := backup
	if backup.BackupType == "incremental" {
		for i := range backups {
			if backups[i].SourceID == backup.SourceID &&
				backups[i].BackupType == "full" &&
				backups[i].CreatedAt.Before(backup.CreatedAt) {
				targetBackup = &backups[i]
			}
		}
		if targetBackup.BackupType != "full" {
			return fmt.Errorf("incremental %s has no preceding full backup", backupID)
		}
	}

	storage := m.storages[targetBackup.StorageTarget]
	if storage == nil {
		return fmt.Errorf("unknown storage target: %s", targetBackup.StorageTarget)
	}
	reader, err := storage.Download(ctx, targetBackup.FilePath)
	if err != nil {
		return fmt.Errorf("download backup: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read backup data: %w", err)
	}

	// The archived payload is the JSON of the Application struct.
	var archivedApp core.Application
	if err := json.Unmarshal(data, &archivedApp); err != nil {
		return fmt.Errorf("decode backup payload: %w", err)
	}

	// Check if the app still exists.
	apps, _, err := m.store.ListAppsByTenant(ctx, tenantID, 1000, 0)
	if err != nil {
		return fmt.Errorf("list tenant apps: %w", err)
	}
	var existing *core.Application
	for i := range apps {
		if apps[i].Name == archivedApp.Name && apps[i].TenantID == tenantID {
			existing = &apps[i]
			break
		}
	}

	if existing == nil {
		// Recreate from archive with a fresh ID.
		archivedApp.ID = core.GenerateID()
		archivedApp.TenantID = tenantID
		archivedApp.CreatedAt = time.Now()
		if err := m.store.CreateApp(ctx, &archivedApp); err != nil {
			return fmt.Errorf("recreate app from backup: %w", err)
		}
		m.logger.Info("app recreated from backup",
			"backup_id", backupID, "app_id", archivedApp.ID, "app_name", archivedApp.Name)
	} else {
		// Update in-place — only the fields that are actually stored in
		// the Application struct (Name, Type, SourceURL, Branch, Dockerfile,
		// BuildPack, EnvVarsEnc, LabelsJSON, Replicas, Port, Status).
		existing.Type = archivedApp.Type
		existing.SourceURL = archivedApp.SourceURL
		existing.Branch = archivedApp.Branch
		existing.Dockerfile = archivedApp.Dockerfile
		existing.BuildPack = archivedApp.BuildPack
		existing.EnvVarsEnc = archivedApp.EnvVarsEnc
		existing.LabelsJSON = archivedApp.LabelsJSON
		existing.Replicas = archivedApp.Replicas
		existing.Port = archivedApp.Port
		existing.UpdatedAt = time.Now()
		if err := m.store.UpdateApp(ctx, existing); err != nil {
			return fmt.Errorf("update app from backup: %w", err)
		}
		m.logger.Info("app updated from backup",
			"backup_id", backupID, "app_id", existing.ID, "app_name", archivedApp.Name)
	}

	return nil
}
