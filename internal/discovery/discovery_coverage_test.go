package discovery

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/ingress"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Module Init/Start lifecycle with minimal Core
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Init_MissingIngress(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := core.NewRegistry()
	// No ingress module registered
	c := &core.Core{
		Logger:   logger,
		Registry: reg,
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should return error when ingress module is not found")
	}
}

// newTestCore creates a minimal core.Core with an initialized ingress module.
func newTestCore(t *testing.T, container core.ContainerRuntime) *core.Core {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	reg := core.NewRegistry()

	cfg := &core.Config{}
	cfg.Ingress.HTTPPort = 18080
	cfg.Ingress.HTTPSPort = 18443

	svc := core.NewServices()
	svc.Container = container

	c := &core.Core{
		Logger:   logger,
		Events:   events,
		Registry: reg,
		Services: svc,
		Config:   cfg,
	}

	// Register and Init the ingress module so Router() returns a valid RouteTable
	ingressMod := ingress.New()
	reg.Register(ingressMod)
	if err := ingressMod.Init(context.Background(), c); err != nil {
		t.Fatalf("ingress Init: %v", err)
	}

	return c
}

func TestModule_Init_Success(t *testing.T) {
	c := newTestCore(t, nil)

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init should succeed: %v", err)
	}

	if m.core != c {
		t.Error("core should be set after Init")
	}
	if m.logger == nil {
		t.Error("logger should be set after Init")
	}
	if m.routeTable == nil {
		t.Error("routeTable should be set after Init")
	}
}

func TestModule_Start_WithoutContainer(t *testing.T) {
	c := newTestCore(t, nil)

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start should succeed without container runtime: %v", err)
	}

	// Watcher should NOT be started since container is nil
	if m.watcher != nil {
		t.Error("watcher should be nil when no container runtime")
	}
}

func TestModule_Start_WithContainer(t *testing.T) {
	c := newTestCore(t, &mockContainerRuntime{})

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start should succeed: %v", err)
	}

	// Watcher should be started since container runtime is available
	if m.watcher == nil {
		t.Error("watcher should be created when container runtime is available")
	}

	// Clean up
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestModule_Start_EventSubscriptions(t *testing.T) {
	c := newTestCore(t, nil)

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The module should have subscribed to container.* and app.deployed events
	stats := c.Events.Stats()
	if stats.SubscriptionCount < 2 {
		t.Errorf("expected at least 2 event subscriptions, got %d", stats.SubscriptionCount)
	}

	// Publish a container started event - should not panic
	c.Events.Publish(context.Background(), core.NewEvent(core.EventContainerStarted, "test", nil))

	// Publish an app deployed event with typed data - should not panic
	c.Events.Publish(context.Background(), core.NewEvent(core.EventAppDeployed, "test",
		core.DeployEventData{AppID: "app-1", ContainerID: "cid-1"}))

	// Publish a container stopped event - should not panic
	c.Events.Publish(context.Background(), core.NewEvent(core.EventContainerStopped, "test", nil))

	// Publish a container died event - should not panic
	c.Events.Publish(context.Background(), core.NewEvent(core.EventContainerDied, "test", nil))

	// Give async handlers time to run
	time.Sleep(50 * time.Millisecond)
}

// ═══════════════════════════════════════════════════════════════════════════════
// Watcher.Start — exercises the loop with context cancellation
// ═══════════════════════════════════════════════════════════════════════════════

func TestWatcher_Start_ContextCancel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "abc123def456789", State: "running",
				Labels: map[string]string{
					"monster.enable":                "true",
					"monster.app.id":                "app-1",
					"monster.app.name":              "web",
					"monster.http.routers.web.rule": "Host(`web.example.com`)",
				},
			},
		},
	}

	w := NewWatcher(runtime, rt, events, logger)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	// Let it run briefly for the initial sync
	time.Sleep(50 * time.Millisecond)

	// Cancel context should stop the watcher
	cancel()

	select {
	case <-done:
		// Good, watcher stopped
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop after context cancel")
	}

	// Verify initial sync happened
	if rt.Count() != 1 {
		t.Errorf("expected 1 route from initial sync, got %d", rt.Count())
	}
}

func TestWatcher_Start_StopChannel(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	runtime := &mockContainerRuntime{}

	w := NewWatcher(runtime, rt, events, logger)

	done := make(chan struct{})
	go func() {
		w.Start(context.Background())
		close(done)
	}()

	// Let it start
	time.Sleep(50 * time.Millisecond)

	// Stop via channel
	w.Stop()

	select {
	case <-done:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop via Stop()")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// HealthChecker — checkHTTP edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestHealthChecker_CheckHTTP_Redirect(t *testing.T) {
	redirects := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirects < 1 {
			redirects++
			http.Redirect(w, r, "/actual", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	backend := srv.Listener.Addr().String()
	hc.Register(backend, "http", "/health")

	hc.checkAll()

	if !hc.IsHealthy(backend) {
		t.Error("backend with redirect should be considered healthy")
	}
}

func TestHealthChecker_CheckHTTP_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	backend := srv.Listener.Addr().String()
	hc.Register(backend, "http", "/")

	// After 1 failure, still healthy
	hc.checkAll()
	if !hc.IsHealthy(backend) {
		t.Error("should still be healthy after 1 failure (threshold=3)")
	}

	// After 2 failures, still healthy
	hc.checkAll()
	if !hc.IsHealthy(backend) {
		t.Error("should still be healthy after 2 failures (threshold=3)")
	}

	// After 3 failures, unhealthy
	hc.checkAll()
	if hc.IsHealthy(backend) {
		t.Error("should be unhealthy after 3 failures")
	}
}

func TestHealthChecker_CheckHTTP_NotFoundIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	backend := srv.Listener.Addr().String()
	hc.Register(backend, "http", "/health")

	// 3 failures
	for i := 0; i < 3; i++ {
		hc.checkAll()
	}

	if hc.IsHealthy(backend) {
		t.Error("404 should be treated as unhealthy after threshold")
	}
}

func TestHealthChecker_CheckHTTP_UnreachableServer(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	// Use unreachable host
	hc.Register("127.0.0.1:1", "http", "/health")

	for i := 0; i < 3; i++ {
		hc.checkAll()
	}

	if hc.IsHealthy("127.0.0.1:1") {
		t.Error("unreachable host should be unhealthy")
	}
}

func TestHealthChecker_CheckAll_EmptyChecks(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	// Should not panic with no registered checks
	hc.checkAll()
}
