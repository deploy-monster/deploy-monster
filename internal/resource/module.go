package resource

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const (
	// maxRingPoints is the maximum number of data points kept per app per period.
	maxRingPoints = 288 // 24h at 30s intervals = 2880, we keep 288 (5-min resolution by dropping)
	// resourceCollectInterval is how often resource metrics are collected.
	resourceCollectInterval = 30 * time.Second
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the resource monitoring system.
// Collects server and container metrics, stores rollups, triggers alerts.
//
// Lifecycle notes for Tier 75:
//
//   - Stop used to call close(stopCh) with no sync.Once guard, so a
//     double Stop crashed the resource module with "close of closed
//     channel". stopOnce now serializes the close and makes Stop
//     idempotent across the graceful-shutdown retry path.
//   - The collection loop was spawned as a bare `go m.collectionLoop()`
//     with no WaitGroup. Stop closed the channel but never waited
//     for the goroutine to exit, so the ticker + BoltBatchSet work
//     could race with the BBolt shutdown. wg now tracks the loop.
//   - collectionLoop had no defer/recover. A panic inside
//     collectOnce (e.g. nil pointer through a half-initialised
//     bolt store) would crash the whole process instead of the
//     goroutine. The loop now recovers and logs.
//   - collectOnce used a fresh context.Background for every tick,
//     which meant neither Stop nor a parent deadline could unblock
//     an in-flight Docker Stats call. Ticks now derive a per-tick
//     context from the module stopCtx so Stop aborts work in
//     progress.
//   - NewCollector/NewAlertEngine are tolerant of nil loggers via
//     slog.Default() fallback so a struct-literal constructed
//     Module in tests cannot NPE inside the panic recovery branch.
type Module struct {
	core      *core.Core
	collector *Collector
	alerter   *AlertEngine
	bolt      core.BoltStorer
	logger    *slog.Logger
	stopCh    chan struct{}

	// Shutdown plumbing. wg tracks the collection loop so Stop can
	// wait for it to exit. stopOnce guards close(stopCh) + cancel
	// against double Stop panics. stopCtx is the parent context for
	// every per-tick collection cycle.
	stopOnce   sync.Once
	wg         sync.WaitGroup
	stopCtx    context.Context
	stopCancel context.CancelFunc
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
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())

	m.collector = NewCollector(c.Services.Container, m.logger)
	m.alerter = NewAlertEngine(c.Events, m.logger)

	if c.DB != nil {
		m.bolt = c.DB.Bolt
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	// Start metrics collection loop. Tier 75: tracked by wg so Stop
	// can actually drain it instead of racing with BBolt shutdown.
	m.wg.Add(1)
	go m.collectionLoop()

	m.logger.Info("resource monitor started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	// Idempotent Stop. Pre-Tier-75 a second Stop panicked with
	// "close of closed channel" and no wg drain existed.
	m.stopOnce.Do(func() {
		if m.stopCh != nil {
			close(m.stopCh)
		}
		if m.stopCancel != nil {
			m.stopCancel()
		}
	})
	m.wg.Wait()
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

// runCtx returns the module's stopCtx, or context.Background() as a
// fallback if the module was constructed via struct literal in a
// test and never passed through Init.
func (m *Module) runCtx() context.Context {
	if m.stopCtx != nil {
		return m.stopCtx
	}
	return context.Background()
}

// collectOnce performs a single collection cycle.
// Extracted from collectionLoop for testability.
func (m *Module) collectOnce() {
	ctx := m.runCtx()
	metrics := m.collector.CollectServer(ctx)
	if metrics != nil {
		m.alerter.Evaluate(ctx, metrics)
	}

	containerMetrics := m.collector.CollectContainers(ctx)

	// Batch all metrics into a single BBolt transaction
	m.batchStoreMetrics(metrics, containerMetrics)
}

// batchStoreMetrics persists all collected metrics to BBolt in a single transaction.
func (m *Module) batchStoreMetrics(server *core.ServerMetrics, containers []core.ContainerMetrics) {
	if m.bolt == nil {
		return
	}

	var items []core.BoltBatchItem

	// Server metrics
	if server != nil {
		key := "server:" + server.ServerID + ":24h"
		ring := m.appendPoint(key, metricsPoint{
			Timestamp:  server.Timestamp,
			CPUPercent: server.CPUPercent,
			MemoryMB:   server.RAMUsedMB,
		})
		items = append(items, core.BoltBatchItem{Bucket: "metrics_ring", Key: key, Value: ring})
	}

	// Container metrics
	for _, cm := range containers {
		if cm.AppID == "" {
			continue
		}
		key := cm.AppID + ":24h"
		ring := m.appendPoint(key, metricsPoint{
			Timestamp:  cm.Timestamp,
			CPUPercent: cm.CPUPercent,
			MemoryMB:   cm.RAMUsedMB,
			NetworkRx:  cm.NetworkRxMB,
			NetworkTx:  cm.NetworkTxMB,
		})
		items = append(items, core.BoltBatchItem{Bucket: "metrics_ring", Key: key, Value: ring})
	}

	if len(items) == 0 {
		return
	}

	if err := m.bolt.BatchSet(items); err != nil {
		m.logger.Debug("failed to batch-persist metrics", "count", len(items), "error", err)
	}
}

// appendPoint reads the existing ring, appends a point, trims to max, and returns the updated ring.
func (m *Module) appendPoint(key string, point metricsPoint) metricsRing {
	var ring metricsRing
	_ = m.bolt.Get("metrics_ring", key, &ring) // ignore error if not found

	ring.Points = append(ring.Points, point)
	if len(ring.Points) > maxRingPoints {
		ring.Points = ring.Points[len(ring.Points)-maxRingPoints:]
	}
	return ring
}

func (m *Module) collectionLoop() {
	defer m.wg.Done()
	defer func() {
		// Pre-Tier-75 this loop had no recover. A panic inside
		// collectOnce (e.g. in the BBolt transaction or the alert
		// engine) would tear the whole process down. Recover keeps
		// the resource monitor alive even if a future enhancement
		// accidentally panics mid-collection.
		if r := recover(); r != nil {
			if m.logger != nil {
				m.logger.Error("panic in resource collection loop", "error", r)
			}
		}
	}()

	ticker := time.NewTicker(resourceCollectInterval)
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
