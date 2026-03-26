package resource

import (
	"context"
	"log/slog"
	"runtime"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Collector gathers server and container metrics.
type Collector struct {
	runtime core.ContainerRuntime
	logger  *slog.Logger
}

// NewCollector creates a new metrics collector.
func NewCollector(cr core.ContainerRuntime, logger *slog.Logger) *Collector {
	return &Collector{runtime: cr, logger: logger}
}

// CollectServer gathers host-level metrics.
// Uses runtime package for basic metrics; /proc parsing would be added for Linux.
func (c *Collector) CollectServer(ctx context.Context) *core.ServerMetrics {
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &core.ServerMetrics{
		ServerID:   "local",
		Timestamp:  time.Now(),
		CPUPercent: 0, // Requires /proc parsing on Linux
		RAMUsedMB:  int64(memStats.Sys / 1024 / 1024),
		RAMTotalMB: int64(memStats.Sys / 1024 / 1024), // Approximation; agent provides real values
		Containers: c.countContainers(ctx),
	}
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
