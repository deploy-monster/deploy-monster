package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// GracefulManager handles graceful shutdown and connection draining.
type GracefulManager struct {
	mu             sync.RWMutex
	draining       map[string]*DrainState // containerID -> drain state
	healthCheckers map[string]*HealthChecker
	logger         *slog.Logger
}

// DrainState tracks the draining state of a container.
type DrainState struct {
	ContainerID  string
	StartTime    time.Time
	ActiveConns  int64
	DrainTimeout time.Duration
	Done         chan struct{}
}

// HealthChecker performs health checks on a container.
type HealthChecker struct {
	ContainerID string
	HealthURL   string
	Interval    time.Duration
	Timeout     time.Duration
	Healthy     bool
	lastCheck   time.Time
}

// GracefulConfig holds configuration for graceful shutdown.
type GracefulConfig struct {
	// DrainTimeout is the maximum time to wait for connections to complete.
	DrainTimeout time.Duration
	// HealthCheckInterval is how often to check container health.
	HealthCheckInterval time.Duration
	// HealthCheckTimeout is the timeout for each health check request.
	HealthCheckTimeout time.Duration
	// HealthCheckPath is the path to check for health.
	HealthCheckPath string
	// StartupTimeout is the maximum time to wait for a container to become healthy.
	StartupTimeout time.Duration
}

// DefaultGracefulConfig returns the default graceful configuration.
func DefaultGracefulConfig() GracefulConfig {
	return GracefulConfig{
		DrainTimeout:        30 * time.Second,
		HealthCheckInterval: 500 * time.Millisecond,
		HealthCheckTimeout:  5 * time.Second,
		HealthCheckPath:     "/health",
		StartupTimeout:      60 * time.Second,
	}
}

// NewGracefulManager creates a new graceful manager.
func NewGracefulManager(logger *slog.Logger) *GracefulManager {
	return &GracefulManager{
		draining:       make(map[string]*DrainState),
		healthCheckers: make(map[string]*HealthChecker),
		logger:         logger,
	}
}

// IsDraining returns true if the container is being drained.
func (g *GracefulManager) IsDraining(containerID string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.draining[containerID]
	return ok
}

// StartDrain marks a container as draining and waits for connections to complete.
func (g *GracefulManager) StartDrain(ctx context.Context, containerID string, timeout time.Duration) error {
	g.mu.Lock()
	state := &DrainState{
		ContainerID:  containerID,
		StartTime:    time.Now(),
		DrainTimeout: timeout,
		Done:         make(chan struct{}),
	}
	g.draining[containerID] = state
	g.mu.Unlock()

	g.logger.Info("starting drain",
		"container", containerID,
		"timeout", timeout,
	)

	// Wait for drain to complete or timeout
	select {
	case <-state.Done:
		g.logger.Info("drain completed", "container", containerID)
		return nil
	case <-time.After(timeout):
		g.logger.Warn("drain timeout, forcing removal",
			"container", containerID,
			"active_conns", state.ActiveConns,
		)
		g.mu.Lock()
		delete(g.draining, containerID)
		g.mu.Unlock()
		return fmt.Errorf("drain timeout")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// CompleteDrain signals that draining is complete for a container.
func (g *GracefulManager) CompleteDrain(containerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if state, ok := g.draining[containerID]; ok {
		close(state.Done)
		delete(g.draining, containerID)
	}
}

// WaitForHealthy waits for a container to become healthy.
func (g *GracefulManager) WaitForHealthy(ctx context.Context, runtime core.ContainerRuntime, containerID string, port int, cfg GracefulConfig) error {
	healthURL := fmt.Sprintf("http://localhost:%d%s", port, cfg.HealthCheckPath)

	checker := &HealthChecker{
		ContainerID: containerID,
		HealthURL:   healthURL,
		Interval:    cfg.HealthCheckInterval,
		Timeout:     cfg.HealthCheckTimeout,
		Healthy:     false,
	}

	g.mu.Lock()
	g.healthCheckers[containerID] = checker
	g.mu.Unlock()

	defer func() {
		g.mu.Lock()
		delete(g.healthCheckers, containerID)
		g.mu.Unlock()
	}()

	ticker := time.NewTicker(cfg.HealthCheckInterval)
	defer ticker.Stop()

	timeout := time.After(cfg.StartupTimeout)

	for {
		select {
		case <-ticker.C:
			healthy, err := g.checkHealth(ctx, runtime, containerID, port, cfg)
			if err != nil {
				g.logger.Debug("health check failed",
					"container", containerID,
					"error", err,
				)
				continue
			}
			if healthy {
				g.logger.Info("container is healthy",
					"container", containerID,
				)
				return nil
			}

		case <-timeout:
			return fmt.Errorf("container did not become healthy within %v", cfg.StartupTimeout)

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// checkHealth performs a single health check.
func (g *GracefulManager) checkHealth(ctx context.Context, runtime core.ContainerRuntime, containerID string, port int, cfg GracefulConfig) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.HealthCheckTimeout)
	defer cancel()

	stats, err := runtime.Stats(ctx, containerID)
	if err != nil {
		return false, err
	}

	if stats == nil {
		return false, nil
	}

	// If container has a Docker healthcheck defined, use its status
	if stats.Health != "" {
		return stats.Health == "healthy", nil
	}

	// If no healthcheck defined, consider it healthy if running
	// Give it a brief moment to stabilize after startup
	return stats.Running, nil
}

// ConnectionTracker tracks active connections per container.
type ConnectionTracker struct {
	mu    sync.RWMutex
	conns map[string]int64 // containerID -> active connections
}

// NewConnectionTracker creates a new connection tracker.
func NewConnectionTracker() *ConnectionTracker {
	return &ConnectionTracker{
		conns: make(map[string]int64),
	}
}

// Increment adds a connection to the counter.
func (ct *ConnectionTracker) Increment(containerID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.conns[containerID]++
}

// Decrement removes a connection from the counter.
func (ct *ConnectionTracker) Decrement(containerID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	if ct.conns[containerID] > 0 {
		ct.conns[containerID]--
	}
}

// Active returns the number of active connections for a container.
func (ct *ConnectionTracker) Active(containerID string) int64 {
	ct.mu.RLock()
	defer ct.mu.RUnlock()
	return ct.conns[containerID]
}
