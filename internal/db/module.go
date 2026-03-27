package db

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the database module for DeployMonster.
// It manages SQLite/PostgreSQL (relational) and BBolt (KV) stores.
type Module struct {
	core     *core.Core
	sqlite   *SQLiteDB
	postgres *PostgresDB
	bolt     *BoltStore
	driver   string
	logger   *slog.Logger
}

// New creates a new database module.
func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                  { return "core.db" }
func (m *Module) Name() string                { return "Database" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return nil }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(ctx context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())

	// Initialize relational store based on driver config
	m.driver = c.Config.Database.Driver
	if m.driver == "" {
		m.driver = "sqlite"
	}

	switch m.driver {
	case "sqlite":
		sqliteDB, err := NewSQLite(c.Config.Database.Path)
		if err != nil {
			return fmt.Errorf("sqlite: %w", err)
		}
		m.sqlite = sqliteDB
		c.Store = sqliteDB
		m.logger.Info("sqlite initialized", "path", c.Config.Database.Path)

	case "postgres", "postgresql":
		pgDB, err := NewPostgres(c.Config.Database.URL)
		if err != nil {
			return fmt.Errorf("postgres: %w", err)
		}
		m.postgres = pgDB
		c.Store = pgDB
		m.logger.Info("postgres initialized")

	default:
		return fmt.Errorf("unsupported database driver: %s (supported: sqlite, postgres)", m.driver)
	}

	// Initialize BBolt KV store
	boltPath := filepath.Join(filepath.Dir(c.Config.Database.Path), "deploymonster.bolt")
	boltStore, err := NewBoltStore(boltPath)
	if err != nil {
		return fmt.Errorf("bbolt: %w", err)
	}
	m.bolt = boltStore
	m.logger.Info("bbolt initialized", "path", boltPath)

	// Set the shared database reference on core
	c.DB = &core.Database{
		Bolt: boltStore,
	}
	if m.sqlite != nil {
		c.DB.SQL = m.sqlite.DB()
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	var firstErr error
	if m.sqlite != nil {
		if err := m.sqlite.Close(); err != nil {
			firstErr = fmt.Errorf("sqlite close: %w", err)
		}
	}
	if m.postgres != nil {
		if err := m.postgres.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("postgres close: %w", err)
		}
	}
	if m.bolt != nil {
		if err := m.bolt.Close(); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("bbolt close: %w", err)
		}
	}
	return firstErr
}

func (m *Module) Health() core.HealthStatus {
	if m.bolt == nil {
		return core.HealthDown
	}

	ctx := context.Background()
	switch m.driver {
	case "sqlite":
		if m.sqlite == nil {
			return core.HealthDown
		}
		if err := m.sqlite.Ping(ctx); err != nil {
			return core.HealthDown
		}
	case "postgres", "postgresql":
		if m.postgres == nil {
			return core.HealthDown
		}
		if err := m.postgres.Ping(ctx); err != nil {
			return core.HealthDown
		}
	}
	return core.HealthOK
}

// Store returns the underlying Store interface.
func (m *Module) Store() core.Store {
	if m.postgres != nil {
		return m.postgres
	}
	return m.sqlite
}

// SQLite returns the underlying SQLite database.
func (m *Module) SQLite() *SQLiteDB {
	return m.sqlite
}

// Bolt returns the underlying BBolt store.
func (m *Module) Bolt() *BoltStore {
	return m.bolt
}
