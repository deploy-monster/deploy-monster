package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

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

func (m *Module) ID() string                  { return "api" }
func (m *Module) Name() string                { return "REST API" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "core.auth", "marketplace"} }
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
