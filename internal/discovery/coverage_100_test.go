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

func (m *fakeIngressModule) ID() string                               { return "ingress" }
func (m *fakeIngressModule) Name() string                             { return "Fake" }
func (m *fakeIngressModule) Version() string                          { return "1.0.0" }
func (m *fakeIngressModule) Dependencies() []string                   { return nil }
func (m *fakeIngressModule) Routes() []core.Route                     { return nil }
func (m *fakeIngressModule) Events() []core.EventHandler              { return nil }
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

func (p *panicRuntime) Ping() error                                                    { return nil }
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
func (p *panicRuntime) ImagePull(_ context.Context, _ string) error    { return nil }
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
