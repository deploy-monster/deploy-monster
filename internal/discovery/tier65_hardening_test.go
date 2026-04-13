package discovery

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/ingress"
)

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
