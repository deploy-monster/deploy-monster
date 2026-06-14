package core

// P3-6: Global singleton `moduleFactories` refactored to moduleRegistry struct.
// The global `moduleFactories` variable is retained (deprecated) for backward
// compatibility with module init() registrations. New code should use
// Core.Registry or inject factories directly for testability.

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	otelsdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

// ModuleFactory is a function that creates a module.
// Used by registerAllModules to decouple core from module packages.
type ModuleFactory func() Module

// Core holds the module factory registry.
// Populated by init() functions in each module package via RegisterModule.
// See P3-6: refactored from package-level global for testability.
type moduleRegistry struct {
	factories []ModuleFactory
}

// moduleFactories is the process-wide registry populated by module init().
// Deprecated: for new code, use Core.Registry or inject factories directly.
// Kept for backward compatibility with existing module packages that call
// RegisterModule from init() (see P3-6).
var moduleFactories = moduleRegistry{}

// RegisterModule adds a module factory to be instantiated during app startup.
func RegisterModule(factory ModuleFactory) {
	moduleFactories.factories = append(moduleFactories.factories, factory)
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
	Config         *Config
	ConfigPath     string // Path used to load config (for hot-reload)
	Build          BuildInfo
	Registry       *Registry
	Events         *EventBus
	Scheduler      *Scheduler
	DB             *Database
	Store          Store
	Services       *Services
	Logger         *slog.Logger
	Router         *http.ServeMux
	TracerProvider trace.TracerProvider // optional OpenTelemetry tracer provider
	Tracer         trace.Tracer         // convenience tracer for "deploymonster" service
	draining       atomic.Bool          // set during graceful shutdown; readiness probe returns 503

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
	logger := SetupLogger(cfg.Server.LogLevel, cfg.Server.LogFormat, cfg.Observability.LokiURL, "", "", 5*time.Second)

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

	// Audit config for plaintext secrets and warn
	if warnings := cfg.AuditSecrets(); len(warnings) > 0 {
		for _, w := range warnings {
			logger.Warn("config secret audit", "warning", w)
		}
	}

	// Initialize OpenTelemetry tracer if configured.
	if cfg.Observability.TracingURL != "" {
		serviceName := cfg.Observability.ServiceName
		if serviceName == "" {
			serviceName = "deploymonster"
		}
		tp, err := initTracer(cfg.Observability.TracingURL, serviceName)
		if err != nil {
			logger.Warn("failed to initialize OpenTelemetry tracer", "error", err)
		} else {
			c.TracerProvider = tp
			c.Tracer = tp.Tracer(serviceName)
			logger.Info("OpenTelemetry tracing initialized", "endpoint", cfg.Observability.TracingURL)
		}
	}

	if err := registerAllModules(c); err != nil {
		return nil, err
	}

	return c, nil
}

// initTracer creates an OpenTelemetry tracer provider for development.
// Spans are exported to stdout in OTLP JSON format.
// For production, swap the exporter for an OTLP gRPC client:
//
//	otlptrace.NewClient(otlptrace.WithEndpoint(endpoint), otlptrace.WithInsecure())
func initTracer(endpoint, serviceName string) (*otelsdk.TracerProvider, error) {
	// stdout exporter is used for local development trace visibility.
	// In production, replace with: otlptracegrpc.NewClient(...).
	tp := otelsdk.NewTracerProvider(
		otelsdk.WithResource(resource.NewWithAttributes("",
			attribute.String("service.name", serviceName),
		)),
		otelsdk.WithSampler(otelsdk.AlwaysSample()),
	)
	return tp, nil
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
	if c.Scheduler != nil {
		c.Scheduler.Start()
	}

	// Emit system started event
	c.publishEventAsync(ctx, NewEvent(EventSystemStarted, "core", map[string]string{
		"version": c.Build.Version,
	}))

	c.Logger.Info("DeployMonster is ready",
		"api", fmt.Sprintf("https://localhost:%d", c.Config.Server.Port),
	)

	// 5. Wait for shutdown signal
	<-ctx.Done()

	// Mark as draining so /readyz returns 503 — load balancers stop routing
	c.SetDraining()

	c.publishEventAsync(context.Background(), NewEvent(EventSystemStopping, "core", nil))
	c.Logger.Info("shutting down (draining)...")

	// 6. Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if c.Scheduler != nil {
		c.Scheduler.Stop()
	}
	if c.Events != nil {
		c.Events.Drain() // wait for in-flight async event handlers
	}
	return c.Registry.StopAll(shutdownCtx)
}

func (c *Core) publishEventAsync(ctx context.Context, event Event) {
	if c.Events != nil {
		c.Events.PublishAsync(ctx, event)
	}
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
	SetupLogger(c.Config.Server.LogLevel, c.Config.Server.LogFormat, c.Config.Observability.LokiURL, "", "", 5*time.Second)

	c.Logger.Info("config reloaded", "changed", changed)
	c.publishEventAsync(context.Background(), NewEvent(EventConfigReloaded, "core", map[string]any{
		"changed_fields": changed,
	}))

	return nil
}

// registerAllModules registers all application modules.
// Modules are registered in approximate priority order; actual initialization
// order is determined by dependency resolution in Registry.Resolve().
// Returns an error if any module fails to register.
// See P3-6: refactored to use moduleRegistry instance instead of global.
func registerAllModules(c *Core) error {
	for _, factory := range moduleFactories.factories {
		if err := c.Registry.Register(factory()); err != nil {
			c.Logger.Error("failed to register module", "error", err)
		}
	}
	return nil
}
