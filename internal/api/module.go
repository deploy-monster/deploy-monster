package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/handlers"
	"github.com/deploy-monster/deploy-monster/internal/api/ws"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the REST API module.
type Module struct {
	core    *core.Core
	server  *http.Server
	router  *Router
	authMod *auth.Module
	store   core.Store
	logger  *slog.Logger
}

// New creates a new API module.
func New() *Module {
	return &Module{}
}

func (m *Module) ID() string      { return "api" }
func (m *Module) Name() string    { return "REST API" }
func (m *Module) Version() string { return "1.0.0" }
func (m *Module) Dependencies() []string {
	return []string{"core.db", "core.auth", "marketplace", "billing"}
}
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())

	// Get module references
	m.authMod = c.Registry.Get("core.auth").(*auth.Module)
	m.store = c.Store

	// Create router with all handlers
	m.router = NewRouter(c, m.authMod, m.store)

	return nil
}

func (m *Module) Start(_ context.Context) error {
	addr := fmt.Sprintf("%s:%d", m.core.Config.Server.Host, m.core.Config.Server.Port)

	m.server = &http.Server{
		Addr:         addr,
		Handler:      m.router.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		m.logger.Info("API server starting", "addr", addr)
		if err := m.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			m.logger.Error("API server error", "error", err)
		}
	}()

	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.server != nil {
		m.logger.Info("shutting down API server")

		// Cancel server-scoped context so background goroutines (deploys, builds) stop
		if m.router != nil && m.router.serverCancel != nil {
			m.router.serverCancel()
		}

		// Signal drain mode — new requests get 503, in-flight requests complete
		if m.router != nil && m.router.gracefulShutdown != nil {
			m.router.gracefulShutdown.StartDraining()

			// Wait for in-flight requests to finish (poll every 100ms, respect ctx deadline)
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for m.router.gracefulShutdown.InFlight() > 0 {
				select {
				case <-ctx.Done():
					m.logger.Warn("drain timeout exceeded, forcing shutdown", "in_flight", m.router.gracefulShutdown.InFlight())
					goto shutdown
				case <-ticker.C:
				}
			}
			m.logger.Info("all in-flight requests drained")
		}

		// Wait for background goroutines (safeGo) to complete
		handlers.WaitForBackground()

		// Tier 77: drain WebSocket DeployHub handlers before shutting
		// down the HTTP server. The hub owns hijacked connections that
		// http.Server.Shutdown cannot see, so without this call
		// ServeWS goroutines would outlive the module's lifecycle and
		// race with the log/DB teardown that follows.
		if err := ws.Shutdown(ctx); err != nil {
			m.logger.Warn("deploy hub shutdown did not drain cleanly", "error", err)
		}

	shutdown:
		// Tier 72: the global rate limiter spawns a cleanup goroutine
		// in its constructor. Pre-Tier-72 this was never stopped, so
		// the goroutine leaked for the lifetime of the process and
		// every test that built a router created another orphan.
		if m.router != nil && m.router.globalRL != nil {
			m.router.globalRL.Stop()
		}
		return m.server.Shutdown(ctx)
	}
	return nil
}

func (m *Module) Health() core.HealthStatus {
	if m.server == nil {
		return core.HealthDown
	}
	return core.HealthOK
}
