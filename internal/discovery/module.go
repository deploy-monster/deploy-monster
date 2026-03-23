package discovery

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/ingress"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements service discovery by watching Docker container events
// and automatically registering/deregistering routes in the ingress.
type Module struct {
	core       *core.Core
	watcher    *Watcher
	routeTable *ingress.RouteTable
	logger     *slog.Logger
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                  { return "discovery" }
func (m *Module) Name() string                { return "Service Discovery" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"deploy", "ingress"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())

	// Get ingress route table
	ingressMod := c.Registry.Get("ingress")
	if ingressMod == nil {
		return core.NewAppError(500, "ingress module not found", nil)
	}
	m.routeTable = ingressMod.(*ingress.Module).Router()

	return nil
}

func (m *Module) Start(_ context.Context) error {
	// Watch container events via EventBus
	// When containers start/stop, update the route table
	m.core.Events.SubscribeAsync("container.*", func(ctx context.Context, event core.Event) error {
		switch event.Type {
		case core.EventContainerStarted:
			m.logger.Info("container started, checking for routes", "event", event.Type)
			// Parse labels and register route (will be implemented with Docker event watcher)
		case core.EventContainerStopped, core.EventContainerDied:
			m.logger.Info("container stopped, removing routes", "event", event.Type)
		}
		return nil
	})

	// Also watch app deployment events to register routes
	m.core.Events.SubscribeAsync(core.EventAppDeployed, func(ctx context.Context, event core.Event) error {
		if data, ok := event.Data.(core.DeployEventData); ok {
			m.logger.Info("app deployed, registering route",
				"app_id", data.AppID,
				"container_id", data.ContainerID,
			)
		}
		return nil
	})

	// Start Docker event watcher if container runtime is available
	if m.core.Services.Container != nil {
		m.watcher = NewWatcher(m.core.Services.Container, m.routeTable, m.core.Events, m.logger)
		go m.watcher.Start(context.Background())

		// Start backend health checker
		healthChecker := NewHealthChecker(m.logger)
		healthChecker.Start()
	}

	m.logger.Info("service discovery started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	if m.watcher != nil {
		m.watcher.Stop()
	}
	return nil
}

func (m *Module) Health() core.HealthStatus {
	return core.HealthOK
}
