package discovery

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/ingress"
)

const watcherSyncInterval = 10 * time.Second

// Watcher monitors container changes and updates the route table.
type Watcher struct {
	runtime    core.ContainerRuntime
	routeTable *ingress.RouteTable
	events     *core.EventBus
	logger     *slog.Logger
	stopCh     chan struct{}
}

// NewWatcher creates a new container watcher.
func NewWatcher(runtime core.ContainerRuntime, rt *ingress.RouteTable, events *core.EventBus, logger *slog.Logger) *Watcher {
	return &Watcher{
		runtime:    runtime,
		routeTable: rt,
		events:     events,
		logger:     logger,
		stopCh:     make(chan struct{}),
	}
}

// Start begins the periodic container scan.
// It polls containers with monster.enable=true labels and syncs routes.
func (w *Watcher) Start(ctx context.Context) {
	w.logger.Info("container watcher started")

	// Initial sync
	w.syncRoutes(ctx)

	// Periodic resync every 10 seconds
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

// Stop signals the watcher to stop.
func (w *Watcher) Stop() {
	close(w.stopCh)
}

// syncRoutes lists all containers with monster labels and updates routes.
func (w *Watcher) syncRoutes(ctx context.Context) {
	containers, err := w.runtime.ListByLabels(ctx, map[string]string{
		"monster.enable": "true",
	})
	if err != nil {
		w.logger.Error("failed to list containers", "error", err)
		return
	}

	// Track which app IDs we found so we can clean up stale routes
	activeApps := make(map[string]bool)

	for _, c := range containers {
		if c.State != "running" {
			continue
		}

		appID := c.Labels["monster.app.id"]
		if appID == "" {
			continue
		}
		activeApps[appID] = true

		route := ParseLabelsToRoute(c.Labels, c.ID)
		if route == nil {
			continue
		}

		w.routeTable.Upsert(route)
	}

	w.logger.Debug("routes synced", "containers", len(containers), "routes", w.routeTable.Count())
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
