package discovery

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/ingress"
)

const watcherSyncInterval = 10 * time.Second

// Watcher monitors container changes and updates the route table.
//
// The watcher polls the container runtime every watcherSyncInterval and
// reconciles the ingress route table against the set of running containers
// with the monster.enable=true label. It also garbage-collects routes for
// app IDs that have disappeared so stopped containers do not accumulate
// stale entries forever.
type Watcher struct {
	runtime    core.ContainerRuntime
	routeTable *ingress.RouteTable
	events     *core.EventBus
	logger     *slog.Logger

	// Lifecycle plumbing. Tier 101 replaced the previous sync.Once pair
	// with a mutex-guarded state machine because concurrent Start+Stop
	// could still race on the WaitGroup: Stop's wg.Wait could observe a
	// zero counter and return before Start's wg.Add had executed,
	// reusing a drained WaitGroup. With mu/started/stopped, once Stop
	// has flipped stopped=true, Start becomes a no-op before it can
	// touch wg at all.
	stopCh  chan struct{}
	mu      sync.Mutex
	started bool
	stopped bool
	wg      sync.WaitGroup
}

// NewWatcher creates a new container watcher.
func NewWatcher(runtime core.ContainerRuntime, rt *ingress.RouteTable, events *core.EventBus, logger *slog.Logger) *Watcher {
	if logger == nil {
		logger = slog.Default()
	}
	return &Watcher{
		runtime:    runtime,
		routeTable: rt,
		events:     events,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// Start begins the periodic container scan. It blocks until either
// Stop is called or ctx is canceled — callers who want it in the
// background must invoke it in a goroutine and track the lifetime via
// the watcher's own Stop. Start is a no-op if already running or if
// Stop has already been called; that avoids the WaitGroup-reuse race
// where a goroutine starts Add-ing after Stop's Wait has returned.
func (w *Watcher) Start(ctx context.Context) {
	w.mu.Lock()
	if w.started || w.stopped {
		w.mu.Unlock()
		return
	}
	w.started = true
	w.wg.Add(1)
	w.mu.Unlock()

	defer w.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			w.logger.Error("panic in container watcher", "error", r)
		}
	}()

	w.logger.Info("container watcher started")

	// Initial sync so routes are populated immediately after start
	// rather than after the first tick delay.
	w.syncRoutes(ctx)

	ticker := time.NewTicker(watcherSyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.syncRoutes(ctx)
		case <-w.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// Stop signals the watcher to stop. Safe to call multiple times; the
// second and subsequent calls are no-ops. Stop waits for the Start
// goroutine to fully exit before returning.
func (w *Watcher) Stop() {
	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		w.wg.Wait()
		return
	}
	w.stopped = true
	started := w.started
	close(w.stopCh)
	w.mu.Unlock()
	if started {
		w.wg.Wait()
	}
}

// syncRoutes lists all containers with monster labels and reconciles
// the ingress route table: live containers get upserted, routes whose
// app IDs are no longer present are removed.
func (w *Watcher) syncRoutes(ctx context.Context) {
	containers, err := w.runtime.ListByLabels(ctx, map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		w.logger.Error("failed to list containers", "error", err)
		return
	}

	// Track app IDs that currently have at least one running container
	// so we can remove stale routes in the second pass below.
	activeApps := make(map[string]struct{}, len(containers))

	for _, c := range containers {
		if c.State != "running" {
			continue
		}

		appID := c.Labels["monster.app.id"]
		if appID == "" {
			continue
		}
		activeApps[appID] = struct{}{}

		route := ParseLabelsToRoute(c.Labels, c.ID)
		if route == nil {
			continue
		}

		w.routeTable.Upsert(route)
	}

	// Second pass: drop routes whose AppID no longer has a running
	// container. Without this, scaling a container to zero or stopping
	// an app would leave the route in the table forever and the proxy
	// would keep sending traffic to a dead backend.
	removed := 0
	seen := make(map[string]struct{})
	for _, r := range w.routeTable.All() {
		if r.AppID == "" {
			continue
		}
		if _, ok := activeApps[r.AppID]; ok {
			continue
		}
		// Multiple routes may share an app ID; de-dupe the removal so
		// we only call RemoveByAppID once per stale app.
		if _, already := seen[r.AppID]; already {
			continue
		}
		seen[r.AppID] = struct{}{}
		w.routeTable.RemoveByAppID(r.AppID)
		removed++
	}

	if removed > 0 {
		w.logger.Info("stale routes removed", "count", removed)
	}
	w.logger.Debug("routes synced",
		"containers", len(containers),
		"active_apps", len(activeApps),
		"routes", w.routeTable.Count(),
	)
}

// ParseLabelsToRoute converts monster.* Docker labels to a RouteEntry.
// Label format:
//
//	monster.enable=true
//	monster.http.routers.myapp.rule=Host(`app.example.com`)
//	monster.http.services.myapp.loadbalancer.server.port=3000
//	monster.http.routers.myapp.middlewares=ratelimit,cors
func ParseLabelsToRoute(labels map[string]string, containerID string) *ingress.RouteEntry {
	appID := labels["monster.app.id"]
	appName := labels["monster.app.name"]

	// Find router rule
	var host, pathPrefix string
	for key, val := range labels {
		if strings.HasSuffix(key, ".rule") && strings.HasPrefix(key, "monster.http.routers.") {
			host, pathPrefix = parseRule(val)
			break
		}
	}

	// If no router rule, try to build from app name
	if host == "" {
		return nil
	}

	// Find service port
	port := "80"
	for key, val := range labels {
		if strings.HasSuffix(key, ".server.port") && strings.HasPrefix(key, "monster.http.services.") {
			port = val
			break
		}
	}

	// Build backend address using container ID network alias
	backend := containerID[:12] + ":" + port

	// Middlewares
	var middlewares []string
	for key, val := range labels {
		if strings.HasSuffix(key, ".middlewares") && strings.HasPrefix(key, "monster.http.routers.") {
			middlewares = strings.Split(val, ",")
			for i := range middlewares {
				middlewares[i] = strings.TrimSpace(middlewares[i])
			}
			break
		}
	}

	// LB strategy
	lbStrategy := "round-robin"
	for key, val := range labels {
		if strings.HasSuffix(key, ".strategy") {
			lbStrategy = val
			_ = key
			break
		}
	}

	if pathPrefix == "" {
		pathPrefix = "/"
	}

	return &ingress.RouteEntry{
		Host:        host,
		PathPrefix:  pathPrefix,
		ServiceName: appName,
		Backends:    []string{backend},
		LBStrategy:  lbStrategy,
		Middlewares: middlewares,
		AppID:       appID,
		Priority:    100,
	}
}

// parseRule extracts host and path from a Traefik-style rule.
// e.g., Host(`example.com`) && PathPrefix(`/api`)
func parseRule(rule string) (host, pathPrefix string) {
	// Extract Host(...)
	if idx := strings.Index(rule, "Host(`"); idx >= 0 {
		start := idx + 6
		end := strings.Index(rule[start:], "`)")
		if end >= 0 {
			host = rule[start : start+end]
		}
	}

	// Extract PathPrefix(...)
	if idx := strings.Index(rule, "PathPrefix(`"); idx >= 0 {
		start := idx + 12
		end := strings.Index(rule[start:], "`)")
		if end >= 0 {
			pathPrefix = rule[start : start+end]
		}
	}

	return
}
