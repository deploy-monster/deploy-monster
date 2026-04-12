package discovery

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/ingress"
)

// ═══════════════════════════════════════════════════════════════════════════════
// mockRuntime for Watcher tests
// ═══════════════════════════════════════════════════════════════════════════════

type mockContainerRuntime struct {
	containers []core.ContainerInfo
	listErr    error
}

func (m *mockContainerRuntime) Ping() error { return nil }
func (m *mockContainerRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "mock-id", nil
}
func (m *mockContainerRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (m *mockContainerRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockContainerRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *mockContainerRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockContainerRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.containers, nil
}

func (m *mockContainerRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}

func (m *mockContainerRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}

func (m *mockContainerRuntime) ImagePull(_ context.Context, _ string) error { return nil }

func (m *mockContainerRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return nil, nil
}

func (m *mockContainerRuntime) ImageRemove(_ context.Context, _ string) error { return nil }

func (m *mockContainerRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}

func (m *mockContainerRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// Watcher.syncRoutes — various container scenarios
// ═══════════════════════════════════════════════════════════════════════════════

func TestWatcher_SyncRoutes_WithRunningContainers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "abc123def456789",
				State: "running",
				Labels: map[string]string{
					"monster.enable":                   "true",
					"monster.app.id":                   "app-1",
					"monster.app.name":                 "webapp",
					"monster.http.routers.webapp.rule": "Host(`webapp.example.com`)",
					"monster.http.services.webapp.loadbalancer.server.port": "3000",
				},
			},
		},
	}

	w := NewWatcher(runtime, rt, events, logger)
	w.syncRoutes(context.Background())

	if rt.Count() != 1 {
		t.Errorf("expected 1 route, got %d", rt.Count())
	}
}

func TestWatcher_SyncRoutes_SkipsNonRunning(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "abc123def456789",
				State: "exited",
				Labels: map[string]string{
					"monster.enable":                   "true",
					"monster.app.id":                   "app-1",
					"monster.http.routers.webapp.rule": "Host(`webapp.example.com`)",
				},
			},
		},
	}

	w := NewWatcher(runtime, rt, events, logger)
	w.syncRoutes(context.Background())

	if rt.Count() != 0 {
		t.Errorf("expected 0 routes for non-running containers, got %d", rt.Count())
	}
}

func TestWatcher_SyncRoutes_SkipsMissingAppID(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "abc123def456789",
				State: "running",
				Labels: map[string]string{
					"monster.enable":                   "true",
					"monster.http.routers.webapp.rule": "Host(`webapp.example.com`)",
					// No monster.app.id label
				},
			},
		},
	}

	w := NewWatcher(runtime, rt, events, logger)
	w.syncRoutes(context.Background())

	if rt.Count() != 0 {
		t.Errorf("expected 0 routes for missing app ID, got %d", rt.Count())
	}
}

func TestWatcher_SyncRoutes_ListError(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	runtime := &mockContainerRuntime{
		listErr: context.DeadlineExceeded,
	}

	w := NewWatcher(runtime, rt, events, logger)
	// Should not panic
	w.syncRoutes(context.Background())

	if rt.Count() != 0 {
		t.Errorf("expected 0 routes on error, got %d", rt.Count())
	}
}

func TestWatcher_SyncRoutes_NoRouteRule(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID:    "abc123def456789",
				State: "running",
				Labels: map[string]string{
					"monster.enable":   "true",
					"monster.app.id":   "app-1",
					"monster.app.name": "noroute",
					// No router rule
				},
			},
		},
	}

	w := NewWatcher(runtime, rt, events, logger)
	w.syncRoutes(context.Background())

	if rt.Count() != 0 {
		t.Errorf("expected 0 routes when no router rule, got %d", rt.Count())
	}
}

func TestWatcher_SyncRoutes_MultipleContainers(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "aaa111bbb222333", State: "running",
				Labels: map[string]string{
					"monster.enable":                 "true",
					"monster.app.id":                 "app-1",
					"monster.app.name":               "web1",
					"monster.http.routers.web1.rule": "Host(`web1.example.com`)",
					"monster.http.services.web1.loadbalancer.server.port": "3000",
				},
			},
			{
				ID: "ccc333ddd444555", State: "running",
				Labels: map[string]string{
					"monster.enable":                 "true",
					"monster.app.id":                 "app-2",
					"monster.app.name":               "web2",
					"monster.http.routers.web2.rule": "Host(`web2.example.com`)",
					"monster.http.services.web2.loadbalancer.server.port": "8080",
				},
			},
		},
	}

	w := NewWatcher(runtime, rt, events, logger)
	w.syncRoutes(context.Background())

	if rt.Count() != 2 {
		t.Errorf("expected 2 routes, got %d", rt.Count())
	}
}
