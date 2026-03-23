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
		ServerID:    "local",
		Timestamp:   time.Now(),
		CPUPercent:  0, // Requires /proc parsing on Linux
		RAMUsedMB:   int64(memStats.Sys / 1024 / 1024),
		RAMTotalMB:  int64(memStats.Sys / 1024 / 1024), // Approximation; agent provides real values
		Containers:  c.countContainers(ctx),
	}
}

// CollectContainers gathers per-container metrics via Docker Stats API.
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
	for _, container := range containers {
		if container.State != "running" {
			continue
		}
		metrics = append(metrics, core.ContainerMetrics{
			ContainerID: container.ID,
			AppID:       container.Labels["monster.app.id"],
			Timestamp:   time.Now(),
		})
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
