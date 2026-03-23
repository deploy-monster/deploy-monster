package resource

import (
	"context"
	"log/slog"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the resource monitoring system.
// Collects server and container metrics, stores rollups, triggers alerts.
type Module struct {
	core      *core.Core
	collector *Collector
	alerter   *AlertEngine
	logger    *slog.Logger
	stopCh    chan struct{}
}

func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                  { return "resource" }
func (m *Module) Name() string                { return "Resource Monitor" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())
	m.stopCh = make(chan struct{})

	m.collector = NewCollector(c.Services.Container, m.logger)
	m.alerter = NewAlertEngine(c.Events, m.logger)

	return nil
}

func (m *Module) Start(_ context.Context) error {
	// Start metrics collection loop
	go m.collectionLoop()

	m.logger.Info("resource monitor started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	close(m.stopCh)
	return nil
}

func (m *Module) Health() core.HealthStatus {
	return core.HealthOK
}

func (m *Module) collectionLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx := context.Background()
			metrics := m.collector.CollectServer(ctx)
			if metrics != nil {
				m.alerter.Evaluate(ctx, metrics)
			}

			containerMetrics := m.collector.CollectContainers(ctx)
			_ = containerMetrics // Store in DB in future phase
		case <-m.stopCh:
			return
		}
	}
}
