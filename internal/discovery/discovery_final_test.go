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
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/ingress"
)

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
