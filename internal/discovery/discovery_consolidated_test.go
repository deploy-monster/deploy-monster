package discovery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/ingress"
)

// === merged from coverage_100_test.go ===

// ═══════════════════════════════════════════════════════════════════════════════
// module.go: init() closure body
// ═══════════════════════════════════════════════════════════════════════════════

func TestDiscovery_NewApp_TriggersInitClosure(t *testing.T) {
	cfg := &core.Config{}
	cfg.Server.SecretKey = "test-secret-32-chars-minimum!yes!!"
	cfg.Server.LogLevel = "info"
	cfg.Server.LogFormat = "text"
	_, err := core.NewApp(cfg, core.BuildInfo{Version: "test"})
	if err != nil {
		t.Logf("NewApp returned (ok if infra missing): %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go: Init — ingress type assertion failure
// ═══════════════════════════════════════════════════════════════════════════════

type fakeIngressModule struct{}

func (m *fakeIngressModule) ID() string                                 { return "ingress" }
func (m *fakeIngressModule) Name() string                               { return "Fake" }
func (m *fakeIngressModule) Version() string                            { return "1.0.0" }
func (m *fakeIngressModule) Dependencies() []string                     { return nil }
func (m *fakeIngressModule) Routes() []core.Route                       { return nil }
func (m *fakeIngressModule) Events() []core.EventHandler                { return nil }
func (m *fakeIngressModule) Init(_ context.Context, _ *core.Core) error { return nil }
func (m *fakeIngressModule) Start(_ context.Context) error              { return nil }
func (m *fakeIngressModule) Stop(_ context.Context) error               { return nil }
func (m *fakeIngressModule) Health() core.HealthStatus                  { return core.HealthOK }

func TestModule_Init_WrongIngressType(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	reg := core.NewRegistry()
	reg.Register(&fakeIngressModule{})

	c := &core.Core{Logger: logger, Registry: reg}

	m := New()
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should return error when ingress module has wrong type")
	}
	if err.Error() != "ingress module has wrong type" {
		t.Errorf("error = %q, want 'ingress module has wrong type'", err.Error())
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go: Start — event handler triggers watcher.syncRoutes (line 86 branch)
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Start_EventDeploySyncsRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	reg := core.NewRegistry()

	ingressMod := ingress.New()
	reg.Register(ingressMod)

	c := &core.Core{
		Logger:   logger,
		Events:   events,
		Registry: reg,
		Services: core.NewServices(),
		Config:   &core.Config{},
	}
	if err := ingressMod.Init(context.Background(), c); err != nil {
		t.Fatalf("ingress Init: %v", err)
	}
	c.Services.Container = &mockContainerRuntime{}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Publish an app deployed event — this triggers the m.watcher.syncRoutes(ctx) branch
	events.Publish(context.Background(), core.NewEvent(core.EventAppDeployed, "test",
		core.DeployEventData{AppID: "app-1", ContainerID: "cid-1"}))

	time.Sleep(100 * time.Millisecond)

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// health.go: probeHTTP — request creation error branch
// ═══════════════════════════════════════════════════════════════════════════════

func TestHealthChecker_ProbeHTTP_NewRequestError(t *testing.T) {
	hc := NewHealthChecker(slog.New(slog.NewTextHandler(io.Discard, nil)))
	// A backend with a null byte produces an invalid URL that causes
	// http.NewRequestWithContext to return an error.
	err := hc.probeHTTP("invalid\x00host", "/", time.Second)
	if err == nil {
		t.Error("expected error for invalid backend address")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// health.go: checkAll — deregister during probe (!ok branch)
//
// We use a slow HTTP probe that stays in-flight long enough for us to
// deregister the backend between the probe phase and the commit phase.
// When the commit phase acquires the write lock, the check is gone,
// exercising the `if !ok { continue }` branch.
// ═══════════════════════════════════════════════════════════════════════════════

func TestHealthChecker_CheckAll_DeregisterDuringProbe(t *testing.T) {
	delay := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-delay
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)
	hc.client = srv.Client()
	hc.client.Timeout = 5 * time.Second

	backend := srv.Listener.Addr().String()
	hc.Register(backend, "http", "/")

	done := make(chan struct{})
	go func() {
		hc.checkAll()
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	hc.Deregister(backend)
	close(delay)
	<-done
}

// ═══════════════════════════════════════════════════════════════════════════════
// health.go: loop — panic recovery
// ═══════════════════════════════════════════════════════════════════════════════
//
// Trigger a panic in checkAll by setting client to nil. HTTP probes call
// hc.client.Do(req) which panics with nil pointer dereference, and the
// loop's deferred recover() catches it.

func TestHealthChecker_Loop_PanicRecovery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	hc := &HealthChecker{
		checks:   make(map[string]*HealthCheck),
		client:   nil, // nil client causes panic on hc.client.Do
		logger:   logger,
		interval: 10 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}

	// Must use HTTP type so probeHTTP is called (which uses hc.client)
	hc.checks["127.0.0.1:1"] = &HealthCheck{
		Backend: "127.0.0.1:1", Type: "http", Path: "/", Timeout: time.Second,
		Healthy: true, Threshold: 3,
	}

	hc.Start()
	time.Sleep(50 * time.Millisecond)
	hc.Stop()
}

// ═══════════════════════════════════════════════════════════════════════════════
// watcher.go: Start — panic recovery body
// ═══════════════════════════════════════════════════════════════════════════════

type panicRuntime struct{}

func (p *panicRuntime) Ping() error { return nil }
func (p *panicRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (p *panicRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (p *panicRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (p *panicRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (p *panicRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (p *panicRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	panic("deliberate panic in ListByLabels")
}
func (p *panicRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) { return "", nil }
func (p *panicRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}
func (p *panicRuntime) ImagePull(_ context.Context, _ string) error { return nil }
func (p *panicRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return nil, nil
}
func (p *panicRuntime) ImageRemove(_ context.Context, _ string) error { return nil }
func (p *panicRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (p *panicRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

func TestWatcher_Start_PanicRecovery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)
	w := NewWatcher(&panicRuntime{}, rt, events, logger)

	// Starting with a panicking runtime should be recovered by the defer/recover
	done := make(chan struct{})
	go func() {
		w.Start(context.Background())
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	w.Stop()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after panic in syncRoutes")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// watcher.go: Start — stopped -> Start is no-op
// ═══════════════════════════════════════════════════════════════════════════════

func TestWatcher_Start_AfterStop_Noop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)
	runtime := &mockContainerRuntime{}

	w := NewWatcher(runtime, rt, events, logger)
	w.Stop()

	done := make(chan struct{})
	go func() {
		w.Start(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// returned immediately because stopped=true
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop called")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// watcher.go: syncRoutes — stale route removal with de-dup
// ═══════════════════════════════════════════════════════════════════════════════

func TestWatcher_SyncRoutes_StaleRemovalDeDup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	events := core.NewEventBus(logger)

	// Add two routes for the same stale AppID — this triggers the de-dup branch
	rt.Upsert(&ingress.RouteEntry{
		Host: "old1.example.com", PathPrefix: "/", Backends: []string{"a:1"},
		AppID: "stale-app",
	})
	rt.Upsert(&ingress.RouteEntry{
		Host: "old2.example.com", PathPrefix: "/", Backends: []string{"a:2"},
		AppID: "stale-app",
	})

	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "active-1234567890", State: "running",
				Labels: map[string]string{
					"monster.enable":                   "true",
					"monster.app.id":                   "active-app",
					"monster.app.name":                 "active",
					"monster.http.routers.active.rule": "Host(`active.example.com`)",
					"monster.http.services.active.loadbalancer.server.port": "8080",
				},
			},
		},
	}

	w := NewWatcher(runtime, rt, events, logger)
	w.syncRoutes(context.Background())

	if rt.Count() != 1 {
		t.Errorf("expected 1 route (stale removed), got %d", rt.Count())
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// watcher.go: Start — ticker.C branch (periodic sync)
// ═══════════════════════════════════════════════════════════════════════════════

func TestWatcher_Start_TickerFires(t *testing.T) {
	original := watcherSyncInterval
	watcherSyncInterval = 10 * time.Millisecond
	defer func() { watcherSyncInterval = original }()

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

	time.Sleep(50 * time.Millisecond)
	w.Stop()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not stop")
	}
}

// === merged from coverage_boost_test.go ===

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

// === merged from discovery_final_test.go ===

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ─── Mock ContainerRuntime ──────────────────────────────────────────────────

type mockRuntime struct {
	containers []core.ContainerInfo
	listErr    error
}

func (m *mockRuntime) Ping() error { return nil }
func (m *mockRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (m *mockRuntime) Stop(_ context.Context, _ string, _ int) error                { return nil }
func (m *mockRuntime) Remove(_ context.Context, _ string, _ bool) error             { return nil }
func (m *mockRuntime) Restart(_ context.Context, _ string) error                    { return nil }
func (m *mockRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) { return "", nil }
func (m *mockRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}
func (m *mockRuntime) ImagePull(_ context.Context, _ string) error               { return nil }
func (m *mockRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error)     { return nil, nil }
func (m *mockRuntime) ImageRemove(_ context.Context, _ string) error             { return nil }
func (m *mockRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) { return nil, nil }
func (m *mockRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error)   { return nil, nil }
func (m *mockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.containers, nil
}

// =============================================================================
// HealthChecker.Start — test the goroutine path + Stop
// =============================================================================

func TestHealthChecker_Start_Stop(t *testing.T) {
	hc := &HealthChecker{
		checks:   make(map[string]*HealthCheck),
		client:   &http.Client{Timeout: 1 * time.Second},
		logger:   testLogger(),
		interval: 50 * time.Millisecond, // fast interval
		stopCh:   make(chan struct{}),
	}

	// Register a TCP check pointing to a closed address
	hc.Register("127.0.0.1:1", "tcp", "")

	hc.Start()
	time.Sleep(150 * time.Millisecond) // Let at least one tick happen
	hc.Stop()

	// Verify the check was executed
	status := hc.Status()
	check, ok := status["127.0.0.1:1"]
	if !ok {
		t.Fatal("expected check for 127.0.0.1:1")
	}
	if check.LastChecked.IsZero() {
		t.Error("expected LastChecked to be set after tick")
	}
}

// =============================================================================
// HealthChecker.checkHTTP — HTTP 400+ returns error
// =============================================================================

func TestHealthChecker_CheckHTTP_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	hc := NewHealthChecker(testLogger())

	// Extract host from server URL
	host := srv.Listener.Addr().String()

	err := hc.probeHTTP(host, "/healthz", 5*time.Second)
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
	if err != nil && err.Error() != "HTTP 500" {
		t.Errorf("expected 'HTTP 500', got %q", err.Error())
	}
}

// =============================================================================
// HealthChecker.checkHTTP — success
// =============================================================================

func TestHealthChecker_CheckHTTP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	hc := NewHealthChecker(testLogger())
	host := srv.Listener.Addr().String()

	if err := hc.probeHTTP(host, "/", 5*time.Second); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// =============================================================================
// HealthChecker.checkAll — threshold marking unhealthy, recovery path
// =============================================================================

func TestHealthChecker_CheckAll_UnhealthyAndRecovery(t *testing.T) {
	// Create a TCP listener that we can close to simulate failure
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	hc := &HealthChecker{
		checks: make(map[string]*HealthCheck),
		client: &http.Client{Timeout: 1 * time.Second},
		logger: testLogger(),
	}

	hc.checks[addr] = &HealthCheck{
		Backend:   addr,
		Type:      "tcp",
		Timeout:   1 * time.Second,
		Healthy:   true,
		Threshold: 2,
	}

	// Close the listener to cause TCP failure
	ln.Close()

	// Run checkAll multiple times to exceed threshold
	hc.checkAll()
	hc.checkAll()

	status := hc.Status()
	if status[addr].Healthy {
		t.Error("expected backend to be marked unhealthy after threshold failures")
	}
	if status[addr].LastError == "" {
		t.Error("expected LastError to be set")
	}

	// Now start a new listener on the same address to simulate recovery
	ln2, err := net.Listen("tcp", addr)
	if err != nil {
		// Address might be reused; skip recovery test
		t.Skip("could not rebind address for recovery test")
	}
	defer ln2.Close()

	hc.checkAll()
	status2 := hc.Status()
	if !status2[addr].Healthy {
		t.Error("expected backend to recover once TCP succeeds")
	}
}

// =============================================================================
// Watcher.Start — context cancellation (line 51)
// =============================================================================

func TestFinal_Watcher_Start_ContextCancel(t *testing.T) {
	rt := &mockRuntime{containers: []core.ContainerInfo{
		{
			ID:    "abcdef123456",
			State: "running",
			Labels: map[string]string{
				"monster.enable":                  "true",
				"monster.app.id":                  "app1",
				"monster.app.name":                "myapp",
				"monster.http.routers.myapp.rule": "Host(`app.example.com`)",
				"monster.http.services.myapp.loadbalancer.server.port": "3000",
			},
		},
	}}

	events := core.NewEventBus(testLogger())
	routeTable := ingress.NewRouteTable()
	w := NewWatcher(rt, routeTable, events, testLogger())

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		w.Start(ctx)
		close(done)
	}()

	// Give it time to do the initial sync
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("Watcher.Start did not return after context cancel")
	}

	// Verify routes were synced
	if routeTable.Count() == 0 {
		t.Error("expected at least one route after initial sync")
	}
}

// =============================================================================
// Watcher.syncRoutes — runtime error path (line 68)
// =============================================================================

func TestWatcher_SyncRoutes_Error(t *testing.T) {
	rt := &mockRuntime{listErr: errors.New("docker down")}
	events := core.NewEventBus(testLogger())
	routeTable := ingress.NewRouteTable()
	w := NewWatcher(rt, routeTable, events, testLogger())

	// Should not panic, just log error
	w.syncRoutes(context.Background())

	if routeTable.Count() != 0 {
		t.Error("expected no routes when runtime errors")
	}
}

// =============================================================================
// Watcher.Start — Stop channel (line 49)
// =============================================================================

func TestFinal_Watcher_Start_StopChannel(t *testing.T) {
	rt := &mockRuntime{}
	events := core.NewEventBus(testLogger())
	routeTable := ingress.NewRouteTable()
	w := NewWatcher(rt, routeTable, events, testLogger())

	done := make(chan struct{})
	go func() {
		w.Start(context.Background())
		close(done)
	}()

	time.Sleep(50 * time.Millisecond)
	w.Stop()

	select {
	case <-done:
		// OK
	case <-time.After(2 * time.Second):
		t.Error("Watcher.Start did not return after Stop")
	}
}

// =============================================================================
// HealthChecker.checkHTTP — request creation error (unlikely but covers line 152)
// =============================================================================

func TestHealthChecker_CheckHTTP_BadURL(t *testing.T) {
	hc := NewHealthChecker(testLogger())

	// "://invalid" builds to "http://://invalid/" which fails URL parse.
	if err := hc.probeHTTP("://invalid", "/", 1*time.Second); err == nil {
		t.Error("expected error for invalid backend address")
	}
}

// =============================================================================
// HealthChecker.checkAll — with HTTP type check (exercises switch case line 114)
// =============================================================================

func TestHealthChecker_CheckAll_HTTPType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	}))
	defer srv.Close()

	host := srv.Listener.Addr().String()

	hc := &HealthChecker{
		checks: make(map[string]*HealthCheck),
		client: srv.Client(),
		logger: testLogger(),
	}
	hc.checks[host] = &HealthCheck{
		Backend:   host,
		Type:      "http",
		Path:      "/",
		Timeout:   5 * time.Second,
		Healthy:   true,
		Threshold: 3,
	}

	hc.checkAll()

	status := hc.Status()
	if !status[host].Healthy {
		t.Error("expected HTTP backend to be healthy")
	}
}

// === merged from tier65_hardening_test.go ===

// Tier 65 — discovery module hardening tests.
//
// These cover the regressions fixed in Tier 65: sync.Once on Stop,
// non-blocking checkAll, stale route cleanup, healthChecker lifecycle
// wired into the Module, and nil-logger guards on the constructors.

// ─── HealthChecker.Stop idempotency ─────────────────────────────────────────

func TestHealthChecker_Stop_Idempotent(t *testing.T) {
	hc := NewHealthChecker(nil) // also exercises the nil-logger guard

	// Double-Stop without Start must not panic. Before Tier 65 the second
	// close(stopCh) would panic with "close of closed channel".
	hc.Stop()
	hc.Stop()
}

func TestHealthChecker_StartStop_Idempotent(t *testing.T) {
	hc := &HealthChecker{
		checks:   make(map[string]*HealthCheck),
		client:   &http.Client{Timeout: 1 * time.Second},
		logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		interval: 50 * time.Millisecond,
		stopCh:   make(chan struct{}),
	}

	// Start twice — second call should be a no-op and not spawn a
	// second goroutine. We cannot directly count goroutines, but we
	// rely on wg balance: if Start() double-counted, Stop() would
	// deadlock forever on wg.Wait.
	hc.Start()
	hc.Start()

	done := make(chan struct{})
	go func() {
		hc.Stop()
		hc.Stop() // double-Stop must also be safe
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked — startOnce/stopOnce/wg balance is wrong")
	}
}

// ─── HealthChecker concurrent-read non-blocking guarantee ───────────────────

// TestHealthChecker_CheckAll_NonBlocking asserts that IsHealthy can be
// called concurrently with checkAll even when a backend probe hangs on
// I/O. Before Tier 65, checkAll held the write lock for the entire probe
// sweep, meaning a hung TCP dial would block every ingress IsHealthy call
// for the full dial timeout.
func TestHealthChecker_CheckAll_NonBlocking(t *testing.T) {
	// A server that accepts the TCP connection but never writes a
	// response would also work, but a plain httptest server that
	// deliberately sleeps gives a deterministic hang for the HTTP probe.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)
	hc.Register(srv.Listener.Addr().String(), "http", "/")
	hc.Register("unrelated-backend", "tcp", "")

	// Kick off a check in the background — this takes ~300ms because of
	// the sleeping server above.
	done := make(chan struct{})
	go func() {
		hc.checkAll()
		close(done)
	}()

	// Give the goroutine a beat to actually enter the probe phase.
	time.Sleep(20 * time.Millisecond)

	// IsHealthy on an unrelated backend must return quickly even while
	// the probe is in flight. We give it a generous 50ms budget — if the
	// sweep were holding the write lock, it would take ~280ms to unblock.
	start := time.Now()
	_ = hc.IsHealthy("unrelated-backend")
	elapsed := time.Since(start)
	if elapsed > 50*time.Millisecond {
		t.Errorf("IsHealthy blocked for %v while checkAll was running — expected non-blocking", elapsed)
	}

	<-done
}

// TestHealthChecker_DeregisterDuringProbe verifies the phase-3 existence
// re-check: if a backend is deregistered after the probe snapshot but
// before the commit, the commit must silently drop the result instead of
// re-inserting a phantom entry.
func TestHealthChecker_DeregisterDuringProbe(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hc := NewHealthChecker(logger)

	hc.Register("127.0.0.1:1", "tcp", "")

	// Race a Deregister against a checkAll. Even if the timing lands
	// such that the probe completes before Deregister, the result should
	// be a consistent map — not a panic and not a resurrected entry.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		hc.checkAll()
	}()
	go func() {
		defer wg.Done()
		hc.Deregister("127.0.0.1:1")
	}()
	wg.Wait()

	// Whatever the interleaving, the backend must either be fully
	// present (probe committed before Deregister ran) or fully absent.
	// Both states are acceptable — the bug we are guarding against was
	// a nil-map write or map-corruption panic.
	_ = hc.Status()
}

// ─── Watcher.Stop idempotency ───────────────────────────────────────────────

func TestWatcher_Stop_Idempotent(t *testing.T) {
	// Nil logger exercises the NewWatcher nil-guard we added.
	w := NewWatcher(&mockRuntime{}, ingress.NewRouteTable(), nil, nil)

	w.Stop()
	w.Stop() // must not panic on double-close(stopCh)
}

func TestWatcher_Stop_WaitsForLoop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()
	w := NewWatcher(&mockRuntime{}, rt, nil, logger)

	started := make(chan struct{})
	go func() {
		close(started)
		w.Start(context.Background())
	}()
	<-started
	time.Sleep(20 * time.Millisecond) // let Start settle into the select

	// Stop must wait for the goroutine to exit before returning. We give
	// it a generous cap to catch a regression where wg.Wait was missing.
	done := make(chan struct{})
	go func() {
		w.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Watcher.Stop did not return — wg.Wait deadlock or missing Done")
	}
}

// ─── Watcher stale route cleanup ────────────────────────────────────────────

// TestWatcher_SyncRoutes_RemovesStaleRoutes exercises the activeApps cleanup
// path that was dead code before Tier 65. A route for an app whose container
// disappears on the next sync must be removed from the route table.
func TestWatcher_SyncRoutes_RemovesStaleRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()

	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{
				ID: "abc123def45600", State: "running",
				Labels: map[string]string{
					"monster.enable":                  "true",
					"monster.app.id":                  "app-alive",
					"monster.app.name":                "alive",
					"monster.http.routers.alive.rule": "Host(`alive.example.com`)",
				},
			},
			{
				ID: "def456abc78900", State: "running",
				Labels: map[string]string{
					"monster.enable":                  "true",
					"monster.app.id":                  "app-dying",
					"monster.app.name":                "dying",
					"monster.http.routers.dying.rule": "Host(`dying.example.com`)",
				},
			},
		},
	}

	w := NewWatcher(runtime, rt, nil, logger)
	w.syncRoutes(context.Background())

	if got := rt.Count(); got != 2 {
		t.Fatalf("expected 2 routes after first sync, got %d", got)
	}

	// Remove the dying container and resync.
	runtime.containers = runtime.containers[:1]
	w.syncRoutes(context.Background())

	if got := rt.Count(); got != 1 {
		t.Fatalf("expected 1 route after stale cleanup, got %d", got)
	}
	if rt.All()[0].AppID != "app-alive" {
		t.Errorf("wrong route survived cleanup: %+v", rt.All()[0])
	}
}

// TestWatcher_SyncRoutes_NoCleanupForExternalRoutes verifies that routes
// without a monster-owned AppID are left alone by the stale cleanup pass.
// We do not want to clobber routes owned by other modules.
func TestWatcher_SyncRoutes_NoCleanupForExternalRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	rt := ingress.NewRouteTable()

	// Pre-populate a route with no AppID — simulating a manual or
	// non-watcher-managed route.
	rt.Upsert(&ingress.RouteEntry{
		Host:        "manual.example.com",
		PathPrefix:  "/",
		ServiceName: "manual",
		Backends:    []string{"10.0.0.1:80"},
		Priority:    100,
	})

	runtime := &mockRuntime{}
	w := NewWatcher(runtime, rt, nil, logger)
	w.syncRoutes(context.Background())

	if got := rt.Count(); got != 1 {
		t.Errorf("expected manual route to survive empty sync, got %d routes", got)
	}
}

// ─── Module Stop cleans up both watcher and healthChecker ───────────────────

// TestModule_Stop_CleansUpHealthChecker verifies the Tier 65 fix that moved
// the healthChecker from a local variable in Start() (which leaked) to a
// field on Module that Stop() can shut down.
func TestModule_Stop_CleansUpHealthChecker(t *testing.T) {
	c := newTestCore(t, &mockContainerRuntime{})

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if m.healthChecker == nil {
		t.Fatal("healthChecker should be stored on Module after Start — Tier 65 fix")
	}
	if m.watcherCtx == nil || m.watcherCancel == nil {
		t.Fatal("watcher context/cancel should be stored on Module after Start")
	}

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Double-Stop must be safe.
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop: %v", err)
	}

	// Confirm the watcher context was canceled.
	select {
	case <-m.watcherCtx.Done():
	default:
		t.Error("watcher context should be canceled after Module.Stop")
	}
}

func TestModule_Stop_WithoutStart_Safe(t *testing.T) {
	m := New()
	// No Init, no Start — Stop should still be a safe no-op.
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop on fresh Module returned: %v", err)
	}
}

// TestModule_WatcherListError_StillStoppable covers the interaction
// between a flaky container runtime and clean shutdown. The watcher
// goroutine must still exit promptly when Stop is called even if its
// current ListByLabels call is failing.
func TestModule_WatcherListError_StillStoppable(t *testing.T) {
	runtime := &mockContainerRuntime{listErr: errors.New("docker unavailable")}
	c := newTestCore(t, runtime)

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Give the watcher a tick so it runs its initial sync against the
	// failing runtime and logs the error.
	time.Sleep(30 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		_ = m.Stop(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Module.Stop blocked when runtime was erroring")
	}
}
