package resource

import (
	"context"
	"log/slog"
	"math"
	"runtime"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Collector gathers server and container metrics. The host field is
// the platform-specific metrics provider — a /proc-parsing
// implementation on Linux, a stubbed fallback elsewhere — so the
// same Collector works for dev on Windows/macOS and the agent on a
// real Linux host.
type Collector struct {
	runtime core.ContainerRuntime
	logger  *slog.Logger
	host    *hostStats
}

// NewCollector creates a new metrics collector. A nil logger is
// tolerated and replaced with slog.Default() so the Tier 75 module
// panic-recovery branch cannot NPE on a struct-literal collector.
func NewCollector(cr core.ContainerRuntime, logger *slog.Logger) *Collector {
	if logger == nil {
		logger = slog.Default()
	}
	return &Collector{
		runtime: cr,
		logger:  logger,
		host:    newHostStats(),
	}
}

// CollectServer gathers host-level metrics. On Linux this walks
// /proc and statfs for real numbers; on other platforms every field
// except Containers and CPUCores degrades to zero or a rough
// runtime.MemStats estimate.
func (c *Collector) CollectServer(ctx context.Context) *core.ServerMetrics {
	if c.host == nil {
		c.host = newHostStats()
	}

	m := &core.ServerMetrics{
		ServerID:   "local",
		Timestamp:  time.Now(),
		Containers: c.countContainers(ctx),
	}

	if cpu, err := c.host.CPUPercent(); err != nil {
		c.logger.Debug("host cpu read failed", "error", err)
	} else {
		m.CPUPercent = cpu
	}

	if used, total, err := c.host.MemoryMB(); err != nil {
		c.logger.Debug("host memory read failed", "error", err)
	} else {
		m.RAMUsedMB = used
		m.RAMTotalMB = total
	}

	if used, total, err := c.host.DiskMB(); err != nil {
		c.logger.Debug("host disk read failed", "error", err)
	} else {
		m.DiskUsedMB = used
		m.DiskTotalMB = total
	}

	if rx, tx, err := c.host.NetworkMB(); err != nil {
		c.logger.Debug("host network read failed", "error", err)
	} else {
		m.NetworkRxMB = rx
		m.NetworkTxMB = tx
	}

	if load, err := c.host.LoadAvg(); err != nil {
		c.logger.Debug("host loadavg read failed", "error", err)
	} else {
		m.LoadAvg = load
	}

	// Fall back to a rough MemStats estimate only when the platform
	// provider returned no data at all (non-Linux + a nil ctx path).
	if m.RAMTotalMB == 0 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		sys := ms.Sys / (1024 * 1024)
		// Defensive: clamp uint64->int64 overflow
		if sys > (1<<63)-1 {
			m.RAMUsedMB = math.MaxInt64
			m.RAMTotalMB = math.MaxInt64
		} else {
			m.RAMUsedMB = int64(sys)
			m.RAMTotalMB = int64(sys)
		}
	}

	return m
}

// CollectContainers gathers per-container metrics via Docker Stats API.
// It discovers running DeployMonster containers and fetches real-time
// CPU, memory, network, and PID stats for each.
func (c *Collector) CollectContainers(ctx context.Context) []core.ContainerMetrics {
	if c.runtime == nil {
		return nil
	}

	containers, err := c.runtime.ListByLabels(ctx, map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		c.logger.Debug("failed to list containers for metrics", "error", err)
		return nil
	}

	var metrics []core.ContainerMetrics
	for _, ctr := range containers {
		if ctr.State != "running" {
			continue
		}

		m := core.ContainerMetrics{
			ContainerID: ctr.ID,
			AppID:       ctr.Labels["monster.app.id"],
			Timestamp:   time.Now(),
		}

		// Fetch real-time stats from Docker
		stats, err := c.runtime.Stats(ctx, ctr.ID)
		if err != nil {
			c.logger.Debug("failed to get container stats",
				"container", ctr.ID, "error", err)
		} else {
			m.CPUPercent = stats.CPUPercent
			m.RAMUsedMB = stats.MemoryUsage / (1024 * 1024)
			m.RAMLimitMB = stats.MemoryLimit / (1024 * 1024)
			m.NetworkRxMB = stats.NetworkRx / (1024 * 1024)
			m.NetworkTxMB = stats.NetworkTx / (1024 * 1024)
			m.PIDs = stats.PIDs
		}

		metrics = append(metrics, m)
	}

	return metrics
}

func (c *Collector) countContainers(ctx context.Context) int {
	if c.runtime == nil {
		return 0
	}
	containers, err := c.runtime.ListByLabels(ctx, map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		return 0
	}
	return len(containers)
}
