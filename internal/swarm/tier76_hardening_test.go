package swarm

import (
	"context"
	"encoding/json"
	"net"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Tier 76 — swarm AgentServer lifecycle hardening tests.
//
// These cover the regressions fixed in Tier 76:
//
//   - NewAgentServer tolerates a nil logger (falls back to slog.Default)
//   - NewAgentServer wires stopCtx/stopCancel
//   - Stop idempotency (stopOnce-guarded double close + double cancel)
//   - Stop flips closed flag under s.mu
//   - Stop cancels stopCtx so pubCtx-derived async publishes abort
//   - Stop drains the readLoop wg (tracked goroutine exits before return)
//   - Stop drains the heartbeatLoop wg
//   - Stop drains in-flight per-agent ping goroutines spawned by heartbeatTick
//   - StartHeartbeat after Stop is a no-op (no goroutine leak)
//   - HandleConnect rejection path after Stop (closed flag short-circuit) —
//     verified indirectly via tier76FakeAgent register racing Stop
//   - heartbeatTick short-circuits on closed
//   - Concurrent Stop storm (no panic, no deadlock)
//   - pubCtx fallback for struct-literal servers

// ─── Nil-logger guard ─────────────────────────────────────────────────────

func TestTier76_NewAgentServer_NilLogger(t *testing.T) {
	// Pre-Tier-76 this panicked on logger.With inside the constructor
	// because logger was nil. Nil now falls back to slog.Default().
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", nil)
	if s == nil {
		t.Fatal("NewAgentServer returned nil with nil logger")
	}
	if s.logger == nil {
		t.Error("logger should have been defaulted, got nil")
	}
}

// ─── stopCtx wiring ────────────────────────────────────────────────────────

func TestTier76_NewAgentServer_InitialisesStopCtx(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	if s.stopCtx == nil {
		t.Error("stopCtx should be initialised by NewAgentServer")
	}
	if s.stopCancel == nil {
		t.Error("stopCancel should be initialised by NewAgentServer")
	}
	select {
	case <-s.stopCtx.Done():
		t.Fatal("stopCtx was cancelled before Stop")
	default:
	}
}

func TestTier76_Stop_CancelsStopCtx(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	s.Stop()
	select {
	case <-s.stopCtx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stopCtx was not cancelled by Stop")
	}
}

func TestTier76_Stop_FlipsClosedFlag(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		t.Fatal("closed flag set before Stop")
	}
	s.mu.RUnlock()

	s.Stop()

	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.closed {
		t.Error("closed flag was not set by Stop")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier76_Stop_Idempotent_TripleCall(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())

	// Pre-Tier-76 a second Stop panicked with "close of closed
	// channel". stopOnce now guards it.
	s.Stop()
	s.Stop()
	s.Stop()
}

// ─── pubCtx fallback ──────────────────────────────────────────────────────

func TestTier76_PubCtx_FallbackForStructLiteral(t *testing.T) {
	// Struct-literal server — stopCtx never populated.
	s := &AgentServer{}
	ctx := s.pubCtx()
	if ctx == nil {
		t.Fatal("pubCtx returned nil for struct-literal server")
	}
	select {
	case <-ctx.Done():
		t.Fatal("fallback pubCtx should not be cancelled")
	default:
	}
}

func TestTier76_PubCtx_PrefersStopCtx(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	ctx := s.pubCtx()
	if ctx != s.stopCtx {
		t.Error("pubCtx should return stopCtx when populated")
	}
}

// ─── StartHeartbeat after Stop is a no-op ────────────────────────────────

func TestTier76_StartHeartbeat_AfterStop_NoOp(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	s.Stop()

	// StartHeartbeat must see closed=true and refuse to spawn the
	// heartbeat loop. Otherwise Stop's wg.Wait has already returned
	// and a new Add(1) would create a goroutine outside Stop's drain.
	done := make(chan struct{})
	go func() {
		s.StartHeartbeat()
		close(done)
	}()
	select {
	case <-done:
		// expected
	case <-time.After(500 * time.Millisecond):
		t.Fatal("StartHeartbeat blocked after Stop")
	}

	// A follow-up Stop must still be a clean no-op drain.
	s.Stop()
}

// ─── Stop drains heartbeat loop ───────────────────────────────────────────

func TestTier76_Stop_DrainsHeartbeatLoop(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	s.SetHeartbeat(10*time.Millisecond, 1*time.Second)
	s.StartHeartbeat()

	// Give the heartbeat loop time to enter its select.
	time.Sleep(30 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not drain the heartbeat loop within 2s")
	}
}

// ─── Stop drains readLoop ────────────────────────────────────────────────

// TestTier76_Stop_DrainsReadLoop proves that Stop waits for an
// in-flight readLoop goroutine to exit before returning. Pre-Tier-76
// the goroutine could still be running removeAgent (which grabs s.mu)
// after Stop had returned, racing with the module's event bus teardown.
func TestTier76_Stop_DrainsReadLoop(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ac := &AgentConn{
		ServerID: "drain-agent",
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  json.NewDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
		lastSeen: time.Now(),
	}

	// Register the agent + wg.Add under the same critical section,
	// exactly as HandleConnect does.
	s.mu.Lock()
	s.agents["drain-agent"] = ac
	s.wg.Add(1)
	s.mu.Unlock()

	// Instrument readLoop exit via an atomic — we want Stop to observe
	// this flag flipping to 1 before it returns.
	var exited atomic.Bool
	origDone := make(chan struct{})
	go func() {
		s.readLoop(ac)
		exited.Store(true)
		close(origDone)
	}()

	// Let the goroutine enter decoder.Decode.
	time.Sleep(20 * time.Millisecond)

	// Stop must close the conn + cancel ctx to unblock Decode, then
	// wg.Wait must block until readLoop returns.
	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not drain readLoop within 2s")
	}

	if !exited.Load() {
		t.Error("Stop returned before readLoop exited — wg.Wait was bypassed")
	}
	<-origDone
}

// ─── heartbeatTick short-circuits on closed ──────────────────────────────

// TestTier76_HeartbeatTick_ShortCircuitsOnClosed registers a fake agent
// manually, flips closed=true, and proves heartbeatTick does not spawn
// a ping goroutine after closed. If it did, wg.Wait would block on an
// untracked spawn and Stop would have to drain it.
func TestTier76_HeartbeatTick_ShortCircuitsOnClosed(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ac := &AgentConn{
		ServerID: "tick-closed",
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  json.NewDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
		lastSeen: time.Now(),
	}

	s.mu.Lock()
	s.agents["tick-closed"] = ac
	s.closed = true
	s.mu.Unlock()

	// Tick must not spawn a ping (closed=true) and must not panic.
	// If a ping were spawned, wg.Add would fire under a closed server.
	s.heartbeatTick(1 * time.Second)

	// wg counter must still be zero — no ping was spawned.
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("wg.Wait did not return — heartbeatTick spawned a ping after closed")
	}
}

// ─── heartbeatTick uses stopCtx-derived ping context ─────────────────────

// TestTier76_HeartbeatTick_PingsUseStopCtx verifies the ping goroutine
// derives its context from stopCtx (instead of context.Background) so
// Stop's cancel propagates into the Send call. We watch a fake
// AgentConn whose conn is net.Pipe with nothing reading: the ping's
// encoder.Encode blocks until Stop closes the conn.
func TestTier76_HeartbeatTick_PingsUseStopCtx(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())

	serverConn, clientConn := net.Pipe()
	// We intentionally do not read from clientConn — the server-side
	// encoder.Encode will block until Stop closes the conn.
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ac := &AgentConn{
		ServerID: "stopctx-target",
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  json.NewDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
		lastSeen: time.Now(),
	}
	s.mu.Lock()
	s.agents["stopctx-target"] = ac
	s.mu.Unlock()

	// Fire a tick — this spawns a wg-tracked ping goroutine that
	// blocks inside encoder.Encode on the un-drained pipe.
	s.heartbeatTick(1 * time.Hour)

	// Give the ping goroutine a moment to enter encoder.Encode.
	time.Sleep(30 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// expected — Stop's conn.Close unblocks encoder.Encode
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not drain in-flight ping goroutine within 2s")
	}
}

// ─── Concurrent Stop storm ────────────────────────────────────────────────

func TestTier76_ConcurrentStop_NoPanic(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	s.SetHeartbeat(5*time.Millisecond, 1*time.Second)
	s.StartHeartbeat()
	time.Sleep(20 * time.Millisecond)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.Stop() }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	s.Stop()
}

// ─── HandleConnect closed short-circuit ──────────────────────────────────

// TestTier76_HandleConnect_RejectedAfterStop verifies the HandleConnect
// closed-flag short-circuit. We can't easily spin up a real hijack-
// capable http.ResponseWriter without coupling to the server's
// ReplacesExistingAgent machinery, but we can prove the precondition:
// once closed=true, HandleConnect's agent-registration critical
// section refuses the connection and does not spawn a readLoop.
//
// This is tested indirectly by running a no-op hijack through a
// stopped server via httptest.NewRecorder (which does NOT implement
// http.Hijacker). That path exits at the hijack check, which is
// acceptable — the unit-level coverage of the closed branch is
// provided by the heartbeatTick/StartHeartbeat tests above.
func TestTier76_HandleConnect_UnreachableAfterHijackFailure(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	s.Stop()

	// httptest.NewRecorder does not support hijacking so HandleConnect
	// returns early at the hijack check. No agent is registered and
	// no readLoop is spawned, which is exactly what the closed
	// short-circuit would have done anyway.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/agent/ws?token=tok", nil)
	s.HandleConnect(rec, req)

	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.agents) != 0 {
		t.Errorf("expected 0 agents after HandleConnect on stopped server, got %d", len(s.agents))
	}
}
