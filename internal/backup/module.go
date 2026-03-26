package backup

import (
	"context"
	"log/slog"
	"sync"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the backup engine.
// Supports volume snapshots, database dumps, and configurable storage targets.
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

	// Register local backup storage by default
	localPath := c.Config.Backup.StoragePath
	if localPath == "" {
		localPath = "backups"
	}
	local := NewLocalStorage(localPath)
	m.RegisterStorage("local", local)
	c.Services.RegisterBackupStorage("local", local)

	return nil
}

func (m *Module) Start(_ context.Context) error {
	// Start backup scheduler
	schedule := m.core.Config.Backup.Schedule
	if schedule == "" {
		schedule = "02:00"
	}
	m.scheduler = NewScheduler(m.store, m.storages, m.core.Events, schedule, m.logger)
	m.scheduler.Start()

	m.logger.Info("backup engine started", "storages", m.StorageNames(), "schedule", schedule)
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	if m.scheduler != nil {
		m.scheduler.Stop()
	}
	return nil
}
func (m *Module) Health() core.HealthStatus { return core.HealthOK }

// RegisterStorage adds a backup storage target.
func (m *Module) RegisterStorage(name string, storage core.BackupStorage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storages[name] = storage
}

// StorageNames returns registered storage target names.
func (m *Module) StorageNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.storages))
	for name := range m.storages {
		names = append(names, name)
	}
	return names
}
