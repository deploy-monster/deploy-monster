package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// ModuleFactory is a function that creates a module.
// Used by registerAllModules to decouple core from module packages.
type ModuleFactory func() Module

// moduleFactories holds all registered module factories.
// Populated by init() functions in each module package via RegisterModule.
var moduleFactories []ModuleFactory

// RegisterModule adds a module factory to be instantiated during app startup.
func RegisterModule(factory ModuleFactory) {
	moduleFactories = append(moduleFactories, factory)
}

// BuildInfo holds binary build metadata injected via ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Core is the central application orchestrator.
// It holds the configuration, module registry, event bus, and shared resources.
type Core struct {
	Config     *Config
	ConfigPath string // Path used to load config (for hot-reload)
	Build      BuildInfo
	Registry   *Registry
	Events     *EventBus
	Scheduler  *Scheduler
	DB         *Database
	Store      Store
	Services   *Services
	Logger     *slog.Logger
	Router     *http.ServeMux
	draining   atomic.Bool // set during graceful shutdown; readiness probe returns 503

	// configMu guards in-place mutation of Config's hot-reloadable
	// fields by ReloadConfig. Concurrent readers that run alongside a
	// reload (e.g. in-flight deploy handlers reading Config.Limits)
	// must call ConfigRLock/ConfigRUnlock to observe a consistent
	// snapshot. Tier 101 introduced this lock after -race CI caught
	// the ReloadConfig writes in app.go racing with simulatedDeploy
	// reads in reload_integration_test.go.
	configMu sync.RWMutex
}

// ConfigRLock acquires a read lock on Config's hot-reloadable fields.
// Concurrent readers pair this with ConfigRUnlock so they observe a
// consistent snapshot even if ReloadConfig is mutating fields in place.
// The lock covers only the reloadable fields (server/registration/
// backup/limits blocks); the full Config struct is otherwise immutable
// after NewApp.
func (c *Core) ConfigRLock()   { c.configMu.RLock() }
func (c *Core) ConfigRUnlock() { c.configMu.RUnlock() }

// SetDraining marks the application as draining (shutting down).
// The /readyz endpoint starts returning 503 so load balancers stop routing.
func (c *Core) SetDraining() { c.draining.Store(true) }

// IsDraining returns true if the application is in drain mode.
func (c *Core) IsDraining() bool { return c.draining.Load() }

// NewApp creates a new Core application instance.
func NewApp(cfg *Config, build BuildInfo) (*Core, error) {
	logger := slog.Default()

	c := &Core{
		Config:    cfg,
		Build:     build,
		Registry:  NewRegistry(),
		Events:    NewEventBus(logger),
		Scheduler: NewScheduler(logger),
		Services:  NewServices(),
		Logger:    logger,
		Router:    http.NewServeMux(),
	}

	registerAllModules(c)

	return c, nil
}

// Run starts the application: resolve dependencies, init, start, wait for signal, stop.
func (c *Core) Run(ctx context.Context) error {
	c.Logger.Info("starting DeployMonster",
		"version", c.Build.Version,
		"commit", c.Build.Commit,
	)

	// 1. Resolve dependency graph
	if err := c.Registry.Resolve(); err != nil {
		return fmt.Errorf("dependency resolution: %w", err)
	}

	// 2. Init all modules (dependency order)
	if err := c.Registry.InitAll(ctx, c); err != nil {
		return fmt.Errorf("module init: %w", err)
	}

	// 3. Start all modules
	if err := c.Registry.StartAll(ctx); err != nil {
		return fmt.Errorf("module start: %w", err)
	}

	// 4. Start core scheduler
	c.Scheduler.Start()

	// Emit system started event
	c.Events.PublishAsync(ctx, NewEvent(EventSystemStarted, "core", map[string]string{
		"version": c.Build.Version,
	}))

	c.Logger.Info("DeployMonster is ready",
		"api", fmt.Sprintf("https://localhost:%d", c.Config.Server.Port),
	)

	// 5. Wait for shutdown signal
	<-ctx.Done()

	// Mark as draining so /readyz returns 503 — load balancers stop routing
	c.SetDraining()

	c.Events.PublishAsync(context.Background(), NewEvent(EventSystemStopping, "core", nil))
	c.Logger.Info("shutting down (draining)...")

	// 6. Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.Scheduler.Stop()
	c.Events.Drain() // wait for in-flight async event handlers
	return c.Registry.StopAll(shutdownCtx)
}

// ReloadConfig re-reads the config file and applies safe-to-reload fields
// (log level, log format, CORS origins, registration mode) without restart.
// Fields that require restart (port, database, docker host) are NOT changed.
func (c *Core) ReloadConfig() error {
	newCfg, err := LoadConfig(c.ConfigPath)
	if err != nil {
		return fmt.Errorf("reload config: %w", err)
	}

	c.configMu.Lock()

	var changed []string

	// Log level
	if newCfg.Server.LogLevel != c.Config.Server.LogLevel {
		changed = append(changed, "server.log_level")
		c.Config.Server.LogLevel = newCfg.Server.LogLevel
	}
	// Log format
	if newCfg.Server.LogFormat != c.Config.Server.LogFormat {
		changed = append(changed, "server.log_format")
		c.Config.Server.LogFormat = newCfg.Server.LogFormat
	}
	// CORS origins
	if newCfg.Server.CORSOrigins != c.Config.Server.CORSOrigins {
		changed = append(changed, "server.cors_origins")
		c.Config.Server.CORSOrigins = newCfg.Server.CORSOrigins
	}
	// Registration mode
	if newCfg.Registration.Mode != c.Config.Registration.Mode {
		changed = append(changed, "registration.mode")
		c.Config.Registration.Mode = newCfg.Registration.Mode
	}
	// Backup schedule
	if newCfg.Backup.Schedule != c.Config.Backup.Schedule {
		changed = append(changed, "backup.schedule")
		c.Config.Backup.Schedule = newCfg.Backup.Schedule
	}
	// Limits
	if newCfg.Limits.MaxAppsPerTenant != c.Config.Limits.MaxAppsPerTenant {
		changed = append(changed, "limits.max_apps_per_tenant")
		c.Config.Limits.MaxAppsPerTenant = newCfg.Limits.MaxAppsPerTenant
	}
	if newCfg.Limits.MaxConcurrentBuilds != c.Config.Limits.MaxConcurrentBuilds {
		changed = append(changed, "limits.max_concurrent_builds")
		c.Config.Limits.MaxConcurrentBuilds = newCfg.Limits.MaxConcurrentBuilds
	}
	if newCfg.Limits.MaxConcurrentBuildsPerTenant != c.Config.Limits.MaxConcurrentBuildsPerTenant {
		changed = append(changed, "limits.max_concurrent_builds_per_tenant")
		c.Config.Limits.MaxConcurrentBuildsPerTenant = newCfg.Limits.MaxConcurrentBuildsPerTenant
	}

	c.configMu.Unlock()

	if len(changed) == 0 {
		c.Logger.Info("config reload: no changes detected")
		return nil
	}

	// Apply logger changes immediately
	SetupLogger(c.Config.Server.LogLevel, c.Config.Server.LogFormat)

	c.Logger.Info("config reloaded", "changed", changed)
	c.Events.PublishAsync(context.Background(), NewEvent(EventConfigReloaded, "core", map[string]any{
		"changed_fields": changed,
	}))

	return nil
}

// registerAllModules registers all application modules.
// Modules are registered in approximate priority order; actual initialization
// order is determined by dependency resolution in Registry.Resolve().
func registerAllModules(c *Core) {
	for _, factory := range moduleFactories {
		if err := c.Registry.Register(factory()); err != nil {
			c.Logger.Error("failed to register module", "error", err)
		}
	}
}
