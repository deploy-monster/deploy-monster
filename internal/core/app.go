package core

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
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
	Config    *Config
	Build     BuildInfo
	Registry  *Registry
	Events    *EventBus
	Scheduler *Scheduler
	DB        *Database
	Store     Store
	Services  *Services
	Logger    *slog.Logger
	Router    *http.ServeMux
}

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

	c.Events.PublishAsync(context.Background(), NewEvent(EventSystemStopping, "core", nil))
	c.Logger.Info("shutting down...")

	// 6. Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c.Scheduler.Stop()
	return c.Registry.StopAll(shutdownCtx)
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
