package resource

import (
	"context"
	"log/slog"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const (
	// maxRingPoints is the maximum number of data points kept per app per period.
	maxRingPoints = 288 // 24h at 30s intervals = 2880, we keep 288 (5-min resolution by dropping)
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
	bolt      core.BoltStorer
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

	if c.DB != nil {
		m.bolt = c.DB.Bolt
	}

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

// metricsPoint matches the MetricsPoint struct in the API handlers.
type metricsPoint struct {
	Timestamp  time.Time `json:"timestamp"`
	CPUPercent float64   `json:"cpu_percent"`
	MemoryMB   int64     `json:"memory_mb"`
	NetworkRx  int64     `json:"network_rx_mb"`
	NetworkTx  int64     `json:"network_tx_mb"`
}

type metricsRing struct {
	Points []metricsPoint `json:"points"`
}

// collectOnce performs a single collection cycle.
// Extracted from collectionLoop for testability.
func (m *Module) collectOnce() {
	ctx := context.Background()
	metrics := m.collector.CollectServer(ctx)
	if metrics != nil {
		m.alerter.Evaluate(ctx, metrics)
		m.storeServerMetrics(metrics)
	}

	containerMetrics := m.collector.CollectContainers(ctx)
	for _, cm := range containerMetrics {
		if cm.AppID != "" {
			m.storeAppMetrics(cm)
		}
	}
}

// storeServerMetrics persists server-level metrics to BBolt for the history API.
func (m *Module) storeServerMetrics(sm *core.ServerMetrics) {
	if m.bolt == nil {
		return
	}

	key := "server:" + sm.ServerID + ":24h"
	point := metricsPoint{
		Timestamp:  sm.Timestamp,
		CPUPercent: sm.CPUPercent,
		MemoryMB:   sm.RAMUsedMB,
	}
	m.appendToRing(key, point)
}

// storeAppMetrics persists per-app container metrics to BBolt for the history API.
func (m *Module) storeAppMetrics(cm core.ContainerMetrics) {
	if m.bolt == nil {
		return
	}

	key := cm.AppID + ":24h"
	point := metricsPoint{
		Timestamp:  cm.Timestamp,
		CPUPercent: cm.CPUPercent,
		MemoryMB:   cm.RAMUsedMB,
		NetworkRx:  cm.NetworkRxMB,
		NetworkTx:  cm.NetworkTxMB,
	}
	m.appendToRing(key, point)
}

// appendToRing appends a data point to a ring buffer in BBolt, capping at maxRingPoints.
func (m *Module) appendToRing(key string, point metricsPoint) {
	var ring metricsRing
	_ = m.bolt.Get("metrics_ring", key, &ring) // ignore error if not found

	ring.Points = append(ring.Points, point)

	// Trim to max ring size
	if len(ring.Points) > maxRingPoints {
		ring.Points = ring.Points[len(ring.Points)-maxRingPoints:]
	}

	if err := m.bolt.Set("metrics_ring", key, ring, 0); err != nil {
		m.logger.Debug("failed to persist metrics", "key", key, "error", err)
	}
}

func (m *Module) collectionLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.collectOnce()
		case <-m.stopCh:
			return
		}
	}
}
