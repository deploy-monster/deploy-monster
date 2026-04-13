package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Tier 77 — api/ws DeployHub lifecycle + concurrent-write hardening.
//
// These cover the regressions fixed in Tier 77:
//
//   - DeployHub.Shutdown (new): stopOnce-guarded, closes all registered
//     connections, drains in-flight ServeWS handlers via wg.Wait
//   - Shutdown idempotency (triple call)
//   - Concurrent Shutdown storm (no panic, no deadlock)
//   - Shutdown respects ctx deadline (returns ctx.Err on timeout)
//   - Register refuses new clients after Shutdown
//   - ServeWS rejects new connections with 503 after Shutdown
//   - Broadcasts no longer hold h.mu while writing to the network
//     (snapshot-and-release pattern)
//   - Per-conn writeMu prevents gorilla/websocket frame corruption
//     under concurrent broadcasts on the same conn
//   - Dead clients are evicted when a broadcast write fails
//   - Unregister is idempotent (double unregister is a no-op)
//   - ws.Shutdown package-level helper forwards to the global hub

// ─── Shutdown idempotency ────────────────────────────────────────────────

func TestTier77_Hub_Shutdown_Idempotent(t *testing.T) {
	hub := NewDeployHub()

	ctx := context.Background()
	if err := hub.Shutdown(ctx); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	// Pre-Tier-77 there was no Shutdown at all; post-Tier-77 stopOnce
	// guards the closed-flip and connection-close pass so subsequent
	// calls just re-run wg.Wait on an already-drained counter.
	if err := hub.Shutdown(ctx); err != nil {
		t.Fatalf("second Shutdown: %v", err)
	}
	if err := hub.Shutdown(ctx); err != nil {
		t.Fatalf("third Shutdown: %v", err)
	}
}

// ─── Shutdown flips closed flag ──────────────────────────────────────────

func TestTier77_Hub_Shutdown_SetsClosedFlag(t *testing.T) {
	hub := NewDeployHub()

	hub.mu.RLock()
	if hub.closed {
		hub.mu.RUnlock()
		t.Fatal("new hub should not be closed")
	}
	hub.mu.RUnlock()

	if err := hub.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	hub.mu.RLock()
	defer hub.mu.RUnlock()
	if !hub.closed {
		t.Error("hub.closed should be true after Shutdown")
	}
}

// ─── Register after Shutdown returns nil ─────────────────────────────────

func TestTier77_Hub_Register_RejectedAfterShutdown(t *testing.T) {
	hub := NewDeployHub()
	if err := hub.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Register hits the closed check before it dereferences conn, so
	// passing nil here is safe and exercises the closed short-circuit.
	if got := hub.Register("proj-x", nil); got != nil {
		t.Error("Register should return nil after Shutdown")
	}
}

// ─── ServeWS after Shutdown returns 503 ──────────────────────────────────

func TestTier77_Hub_ServeWS_RejectedAfterShutdown(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("*")
	if err := hub.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "proj-rejected")
	}))
	defer srv.Close()

	// A plain HTTP GET suffices — ServeWS's enter() check fires before
	// the Upgrade call and http.Error(503) is written.
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
}

// ─── Shutdown closes registered connections + drains handlers ───────────

// TestTier77_Hub_Shutdown_ClosesRegisteredConnections proves the full
// Shutdown flow end-to-end: a real ServeWS handler is running, the
// client's conn is registered, Shutdown closes the server-side conn,
// the read loop errors out, the handler exits, wg drains, Shutdown
// returns. Pre-Tier-77 none of this existed — the handler would have
// kept running after the API module was stopped.
func TestTier77_Hub_Shutdown_ClosesRegisteredConnections(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("*")

	handlerExited := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.ServeWS(w, r, "proj-shutdown")
		close(handlerExited)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	// Wait until ServeWS has actually registered the client.
	waitFor(t, 2*time.Second, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		return len(hub.clients["proj-shutdown"]) > 0
	}, "client never registered")

	// Shutdown must close the server-side conn (unblocking ReadMessage)
	// and drain the handler via wg.Wait.
	shutdownDone := make(chan error, 1)
	go func() {
		shutdownDone <- hub.Shutdown(context.Background())
	}()

	select {
	case err := <-shutdownDone:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Shutdown did not return within 3s — wg.Wait missing or deadlocked")
	}

	select {
	case <-handlerExited:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ServeWS handler did not exit after Shutdown")
	}

	hub.mu.RLock()
	n := len(hub.clients["proj-shutdown"])
	hub.mu.RUnlock()
	if n != 0 {
		t.Errorf("expected 0 clients after Shutdown, got %d", n)
	}
}

// ─── Shutdown respects ctx deadline ──────────────────────────────────────

// TestTier77_Hub_Shutdown_ContextDeadline artificially pins wg at 1 so
// wg.Wait blocks, then calls Shutdown with a short ctx and verifies it
// returns ctx.Err() instead of hanging forever.
func TestTier77_Hub_Shutdown_ContextDeadline(t *testing.T) {
	hub := NewDeployHub()

	// Pretend a handler is in flight so wg.Wait blocks.
	hub.wg.Add(1)
	defer hub.wg.Done() // release at test end so the drain goroutine can exit

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := hub.Shutdown(ctx)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("Shutdown should return ctx.Err() when drain exceeds deadline")
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Shutdown took %v with 50ms deadline — ctx was not respected", elapsed)
	}
	// closed flag must still be set even though drain didn't complete.
	hub.mu.RLock()
	defer hub.mu.RUnlock()
	if !hub.closed {
		t.Error("closed flag should be set even on deadline exit")
	}
}

// ─── Broadcast evicts dead clients ───────────────────────────────────────

// TestTier77_Hub_Broadcast_EvictsDeadClients registers a real ws conn,
// hard-closes the client side, then calls BroadcastProgress. The
// server-side write fails and evictDead removes the cc from the map.
// Pre-Tier-77 the dead conn would have lived in the map forever.
func TestTier77_Hub_Broadcast_EvictsDeadClients(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("*")

	serverRegistered := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if hub.Register("proj-dead", conn) == nil {
			return
		}
		defer hub.Unregister("proj-dead", conn)
		close(serverRegistered)
		// Park here until the client disconnects — the read loop will
		// return an error and we unwind.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	<-serverRegistered

	// Hard-close the client side so the server's next write fails.
	_ = ws.Close()

	// Give the TCP close a moment to propagate.
	time.Sleep(80 * time.Millisecond)

	// This broadcast's write will fail → evictDead → cc removed from map.
	hub.BroadcastProgress("proj-dead", "building", "test", 50)

	// evictDead runs synchronously inside Broadcast, so the assertion
	// can run immediately. Allow a brief grace period for any
	// concurrent Unregister that the server handler might do.
	waitFor(t, 1*time.Second, func() bool {
		hub.mu.RLock()
		defer hub.mu.RUnlock()
		return len(hub.clients["proj-dead"]) == 0
	}, "dead client was not evicted")
}

// ─── Concurrent broadcasts — no frame corruption ────────────────────────

// TestTier77_Hub_ConcurrentBroadcast_NoCorruption fires 20 concurrent
// BroadcastProgress calls against one registered client and verifies
// the client reads 20 well-formed JSON messages with distinct Progress
// values. Pre-Tier-77 the broadcasts raced inside gorilla's
// beginMessage / writeFrame pair, interleaving frames on the wire and
// producing malformed JSON on the client side (writeMu fixes this).
func TestTier77_Hub_ConcurrentBroadcast_NoCorruption(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("*")

	ready := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if hub.Register("proj-race", conn) == nil {
			return
		}
		defer hub.Unregister("proj-race", conn)
		close(ready)
		// Park until client disconnects.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer ws.Close()

	<-ready

	const N = 20
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			hub.BroadcastProgress("proj-race", "stage", "concurrent message", i)
		}(i)
	}
	wg.Wait()

	_ = ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	seen := make(map[int]bool)
	for i := 0; i < N; i++ {
		_, data, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		var msg DeployProgressMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			t.Fatalf("unmarshal %d: %v (data=%q)", i, err, string(data))
		}
		if msg.Type != "deploy_progress" {
			t.Errorf("msg %d: Type = %q, want deploy_progress", i, msg.Type)
		}
		if msg.Stage != "stage" {
			t.Errorf("msg %d: Stage = %q, want stage", i, msg.Stage)
		}
		if seen[msg.Progress] {
			t.Errorf("duplicate Progress value %d — frame interleaving?", msg.Progress)
		}
		seen[msg.Progress] = true
	}
	if len(seen) != N {
		t.Errorf("expected %d distinct Progress values, got %d", N, len(seen))
	}
}

// ─── Unregister is idempotent ────────────────────────────────────────────

func TestTier77_Hub_Unregister_Idempotent(t *testing.T) {
	hub := NewDeployHub()
	// Unregister without Register must be a no-op — no panic, no
	// negative-counter drama.
	hub.Unregister("never-registered", nil)
	hub.Unregister("never-registered", nil)
	hub.Unregister("never-registered", nil)
}

// ─── Concurrent Shutdown storm ───────────────────────────────────────────

func TestTier77_Hub_ConcurrentShutdown_NoPanic(t *testing.T) {
	hub := NewDeployHub()

	var wg sync.WaitGroup
	var errs atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := hub.Shutdown(context.Background()); err != nil {
				errs.Add(1)
			}
		}()
	}
	wg.Wait()

	if n := errs.Load(); n != 0 {
		t.Errorf("concurrent Shutdown produced %d errors", n)
	}
	// Final Shutdown must still be a clean no-op.
	if err := hub.Shutdown(context.Background()); err != nil {
		t.Fatalf("final Shutdown: %v", err)
	}
}

// ─── Broadcast does not starve Register/Unregister ──────────────────────

// TestTier77_Hub_Broadcast_DoesNotHoldWriteLock proves the
// snapshot-and-release refactor: a broadcast that takes a long time to
// write must NOT block a concurrent Unregister. Pre-Tier-77 the
// broadcast held h.mu.RLock for the entire iteration, which (combined
// with an RWMutex's writer-preference semantics under contention)
// could starve Register/Unregister for the full write duration.
//
// We simulate a slow write by registering a bogus *clientConn whose
// writeMu is held by the test; the broadcast blocks in safeWrite. A
// concurrent Unregister for a DIFFERENT conn must still complete
// promptly.
func TestTier77_Hub_Broadcast_DoesNotHoldWriteLock(t *testing.T) {
	hub := NewDeployHub()
	hub.SetAllowedOrigins("*")

	// Build two bogus clientConns by hand — we don't need real sockets
	// because we only care about whether Register/Unregister are
	// serialized with the broadcast's write phase.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	dial := func() *websocket.Conn {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		return ws
	}

	slowConn := dial()
	defer slowConn.Close()
	otherConn := dial()
	defer otherConn.Close()

	slowCC := hub.Register("proj-slow", slowConn)
	if slowCC == nil {
		t.Fatal("Register returned nil")
	}
	otherCC := hub.Register("proj-slow", otherConn)
	if otherCC == nil {
		t.Fatal("Register returned nil")
	}

	// Pin slowCC's writeMu so the broadcast's safeWrite on slowCC blocks.
	slowCC.writeMu.Lock()

	broadcastStarted := make(chan struct{})
	broadcastDone := make(chan struct{})
	go func() {
		close(broadcastStarted)
		hub.BroadcastProgress("proj-slow", "stage", "msg", 1)
		close(broadcastDone)
	}()

	<-broadcastStarted
	// Give the broadcast a moment to acquire writeMu on otherCC (fast)
	// and then block on slowCC.writeMu.
	time.Sleep(50 * time.Millisecond)

	// Unregister otherConn — this needs h.mu.Lock, which should be
	// FREE because the broadcast released h.mu right after snapshotting.
	unregDone := make(chan struct{})
	go func() {
		hub.Unregister("proj-slow", otherConn)
		close(unregDone)
	}()

	select {
	case <-unregDone:
		// expected — broadcast does not hold h.mu while writing
	case <-time.After(500 * time.Millisecond):
		slowCC.writeMu.Unlock() // unblock broadcast so we don't leak
		t.Fatal("Unregister was blocked by in-flight broadcast — snapshot-and-release broken")
	}

	// Release slow write, let broadcast finish.
	slowCC.writeMu.Unlock()
	<-broadcastDone
}

// ─── Package-level Shutdown helper compile check ─────────────────────────

// TestTier77_PackageShutdown_HasRightSignature ensures the package-level
// ws.Shutdown function exists with the expected signature. It does NOT
// actually shut down the global hub, because other tests in the package
// (and other callers in the test binary) may still depend on it.
func TestTier77_PackageShutdown_HasRightSignature(t *testing.T) {
	var _ func(context.Context) error = Shutdown
}

// ─── helpers ─────────────────────────────────────────────────────────────

// waitFor polls cond every 10ms until it returns true or the timeout
// expires. Fails the test with msg on timeout.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}
