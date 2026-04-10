package swarm

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// newFakeAgent creates an AgentServer with a single registered AgentConn backed
// by net.Pipe. The returned cleanup function closes both sides. The test drives
// the server side of the pipe directly; the client side is available to the
// caller so it can write responses (pongs, metrics reports, etc.) back to the
// server's read loop — or just sit idle to simulate a silent agent.
func newFakeAgent(t *testing.T, serverID string) (*AgentServer, *AgentConn, net.Conn, func()) {
	t.Helper()

	events := core.NewEventBus(discardLogger())
	s := NewAgentServer(events, "token", discardLogger())

	serverConn, clientConn := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())

	ac := &AgentConn{
		ServerID: serverID,
		Info:     core.AgentInfo{ServerID: serverID, Hostname: "fake-" + serverID},
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  json.NewDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
		lastSeen: time.Now(),
	}

	s.mu.Lock()
	s.agents[serverID] = ac
	s.mu.Unlock()

	cleanup := func() {
		cancel()
		_ = serverConn.Close()
		_ = clientConn.Close()
	}
	return s, ac, clientConn, cleanup
}

// =============================================================================
// AgentConn.touch / LastSeen
// =============================================================================

func TestAgentConn_Touch_UpdatesLastSeen(t *testing.T) {
	ac := &AgentConn{lastSeen: time.Unix(1000, 0)}
	newer := time.Unix(2000, 0)
	ac.touch(newer)
	if got := ac.LastSeen(); !got.Equal(newer) {
		t.Errorf("LastSeen() = %v, want %v", got, newer)
	}
}

func TestAgentConn_Touch_ConcurrentSafe(t *testing.T) {
	ac := &AgentConn{lastSeen: time.Now()}
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ac.touch(time.Now())
			_ = ac.LastSeen()
		}()
	}
	wg.Wait()
	// No assertion — we're checking for a race detector crash under -race.
}

// =============================================================================
// AgentConn.setLastMetrics / LastMetrics
// =============================================================================

func TestAgentConn_LastMetrics_NilBeforeSet(t *testing.T) {
	ac := &AgentConn{}
	if got := ac.LastMetrics(); got != nil {
		t.Errorf("LastMetrics() = %v, want nil", got)
	}
}

func TestAgentConn_LastMetrics_SetAndGet(t *testing.T) {
	ac := &AgentConn{}
	src := &core.ServerMetrics{
		ServerID:   "node-1",
		CPUPercent: 42.5,
		RAMUsedMB:  1024,
		RAMTotalMB: 4096,
		Containers: 7,
		Timestamp:  time.Now(),
	}
	ac.setLastMetrics(src)

	got := ac.LastMetrics()
	if got == nil {
		t.Fatal("LastMetrics() returned nil after setLastMetrics")
	}
	if got.ServerID != "node-1" || got.CPUPercent != 42.5 || got.RAMUsedMB != 1024 || got.Containers != 7 {
		t.Errorf("unexpected metrics: %+v", got)
	}
	// Mutating the returned copy must not affect the stored snapshot.
	got.CPUPercent = 0
	if ac.LastMetrics().CPUPercent != 42.5 {
		t.Error("LastMetrics() did not return a defensive copy")
	}
}

func TestAgentConn_SetLastMetrics_NilIgnored(t *testing.T) {
	ac := &AgentConn{}
	ac.setLastMetrics(nil)
	if got := ac.LastMetrics(); got != nil {
		t.Errorf("LastMetrics() = %v, want nil after setLastMetrics(nil)", got)
	}
}

// =============================================================================
// tryDecodeServerMetrics
// =============================================================================

func TestTryDecodeServerMetrics(t *testing.T) {
	tests := []struct {
		name    string
		payload any
		wantOK  bool
	}{
		{"nil payload", nil, false},
		{"empty map", map[string]any{}, false},
		{"wrong type", "just a string", false},
		{"missing ServerID", map[string]any{"cpu_percent": 10.0}, false},
		{"happy path map", map[string]any{
			"server_id":   "agent-1",
			"cpu_percent": 55.0,
			"ram_used_mb": float64(2048),
		}, true},
		{"happy path struct", core.ServerMetrics{ServerID: "agent-2", CPUPercent: 10}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tryDecodeServerMetrics(tt.payload)
			if ok != tt.wantOK {
				t.Errorf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got == nil {
				t.Error("ok=true but metrics is nil")
			}
			if !ok && got != nil {
				t.Error("ok=false but metrics is non-nil")
			}
		})
	}
}

// =============================================================================
// heartbeatTick — dead-agent detection
// =============================================================================

func TestHeartbeatTick_RemovesDeadAgent(t *testing.T) {
	s, ac, _, cleanup := newFakeAgent(t, "dead-1")
	defer cleanup()

	// Mark the agent as last seen well outside the dead threshold.
	ac.touch(time.Now().Add(-10 * time.Minute))

	s.heartbeatTick(5 * time.Second)

	// ac.cancel() should have fired and the conn should be closed.
	select {
	case <-ac.ctx.Done():
		// expected
	case <-time.After(100 * time.Millisecond):
		t.Error("heartbeatTick did not cancel the dead agent's context")
	}
}

func TestHeartbeatTick_KeepsLiveAgent(t *testing.T) {
	s, ac, clientConn, cleanup := newFakeAgent(t, "live-1")
	defer cleanup()

	// Recent lastSeen — should not be removed.
	ac.touch(time.Now())

	// Drain the server's ping write so Send() doesn't block forever on the pipe.
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		buf := make([]byte, 4096)
		// Best-effort read — we don't care about the contents, only that the
		// server's write completes.
		_ = clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		_, _ = clientConn.Read(buf)
	}()

	s.heartbeatTick(5 * time.Second)

	// Agent must still be registered.
	s.mu.RLock()
	_, stillThere := s.agents["live-1"]
	s.mu.RUnlock()
	if !stillThere {
		t.Error("heartbeatTick removed a live agent")
	}
	<-drainDone
}

func TestHeartbeatTick_NoAgents_NoOp(t *testing.T) {
	events := core.NewEventBus(discardLogger())
	s := NewAgentServer(events, "token", discardLogger())
	// Should not panic with no registered agents.
	s.heartbeatTick(5 * time.Second)
}

// =============================================================================
// SetHeartbeat validation
// =============================================================================

func TestSetHeartbeat_AcceptsValidInput(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	s.SetHeartbeat(2*time.Second, 10*time.Second)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.heartbeatInterval != 2*time.Second || s.heartbeatDead != 10*time.Second {
		t.Errorf("heartbeat not updated: interval=%v dead=%v", s.heartbeatInterval, s.heartbeatDead)
	}
}

func TestSetHeartbeat_RejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		interval time.Duration
		dead     time.Duration
	}{
		{"zero interval", 0, 10 * time.Second},
		{"negative interval", -1, 10 * time.Second},
		{"zero dead", 5 * time.Second, 0},
		{"negative dead", 5 * time.Second, -1},
		{"dead < interval", 30 * time.Second, 5 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
			origInterval := s.heartbeatInterval
			origDead := s.heartbeatDead
			s.SetHeartbeat(tt.interval, tt.dead)
			s.mu.RLock()
			defer s.mu.RUnlock()
			if s.heartbeatInterval != origInterval || s.heartbeatDead != origDead {
				t.Errorf("invalid input accepted: interval=%v dead=%v", s.heartbeatInterval, s.heartbeatDead)
			}
		})
	}
}

// =============================================================================
// Snapshot
// =============================================================================

func TestSnapshot_EmptyServer(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	out := s.Snapshot()
	if len(out) != 0 {
		t.Errorf("Snapshot of empty server = %d entries, want 0", len(out))
	}
}

func TestSnapshot_SortedByServerID(t *testing.T) {
	s, _, _, cleanup1 := newFakeAgent(t, "zulu")
	defer cleanup1()
	// Register two more using the same underlying server but different pipes.
	for _, id := range []string{"alpha", "mike"} {
		serverConn, clientConn := net.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		ac := &AgentConn{
			ServerID: id,
			Info:     core.AgentInfo{ServerID: id},
			conn:     serverConn,
			encoder:  json.NewEncoder(serverConn),
			decoder:  json.NewDecoder(serverConn),
			ctx:      ctx,
			cancel:   cancel,
			pending:  make(map[string]chan core.AgentMessage),
			lastSeen: time.Now(),
		}
		s.mu.Lock()
		s.agents[id] = ac
		s.mu.Unlock()
		t.Cleanup(func() {
			cancel()
			_ = serverConn.Close()
			_ = clientConn.Close()
		})
	}

	out := s.Snapshot()
	if len(out) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(out))
	}
	if out[0].ServerID != "alpha" || out[1].ServerID != "mike" || out[2].ServerID != "zulu" {
		t.Errorf("unsorted snapshot: %v %v %v", out[0].ServerID, out[1].ServerID, out[2].ServerID)
	}
}

func TestSnapshot_HealthyReflectsLastSeen(t *testing.T) {
	s, ac, _, cleanup := newFakeAgent(t, "stale-1")
	defer cleanup()

	// Force dead threshold low for this test
	s.mu.Lock()
	s.heartbeatDead = 1 * time.Second
	s.mu.Unlock()

	ac.touch(time.Now().Add(-10 * time.Second))

	out := s.Snapshot()
	if len(out) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(out))
	}
	if out[0].Healthy {
		t.Error("expected Healthy=false for stale agent")
	}
	if out[0].SecondsIdle < 5 {
		t.Errorf("SecondsIdle = %d, want >= 5", out[0].SecondsIdle)
	}
}

func TestSnapshot_IncludesLastMetrics(t *testing.T) {
	s, ac, _, cleanup := newFakeAgent(t, "metrics-1")
	defer cleanup()

	ac.setLastMetrics(&core.ServerMetrics{
		ServerID:   "metrics-1",
		CPUPercent: 77.7,
		RAMUsedMB:  512,
		Containers: 3,
		Timestamp:  time.Now(),
	})

	out := s.Snapshot()
	if len(out) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(out))
	}
	if out[0].LastMetrics == nil {
		t.Fatal("LastMetrics is nil in snapshot")
	}
	if out[0].LastMetrics.CPUPercent != 77.7 || out[0].LastMetrics.Containers != 3 {
		t.Errorf("wrong metrics in snapshot: %+v", out[0].LastMetrics)
	}
}

// =============================================================================
// handleAgentMessage — metrics path updates lastSeen AND lastMetrics
// =============================================================================

func TestHandleAgentMessage_MetricsReport_RecordsBoth(t *testing.T) {
	s, ac, _, cleanup := newFakeAgent(t, "reporter-1")
	defer cleanup()

	// Force an old lastSeen so we can verify touch() ran.
	ac.touch(time.Unix(1000, 0))

	metrics := core.ServerMetrics{
		ServerID:   "reporter-1",
		CPUPercent: 33.3,
		Containers: 5,
		Timestamp:  time.Now(),
	}

	s.handleAgentMessage(ac, core.AgentMessage{
		Type:    core.AgentMsgMetricsReport,
		Payload: metrics,
	})

	// lastSeen should have been bumped to "roughly now".
	if time.Since(ac.LastSeen()) > time.Second {
		t.Errorf("touch did not update lastSeen: %v", ac.LastSeen())
	}

	// lastMetrics should have been recorded.
	got := ac.LastMetrics()
	if got == nil {
		t.Fatal("lastMetrics not recorded")
	}
	if got.CPUPercent != 33.3 || got.Containers != 5 {
		t.Errorf("wrong stored metrics: %+v", got)
	}
}

func TestHandleAgentMessage_PongUpdatesLastSeen(t *testing.T) {
	s, ac, _, cleanup := newFakeAgent(t, "ponger-1")
	defer cleanup()

	ac.touch(time.Unix(1000, 0))

	s.handleAgentMessage(ac, core.AgentMessage{Type: core.AgentMsgPong})

	if time.Since(ac.LastSeen()) > time.Second {
		t.Errorf("touch did not update lastSeen on pong: %v", ac.LastSeen())
	}
}

// =============================================================================
// LocalExecutor.Metrics — enhanced with container count
// =============================================================================

func TestLocalExecutor_Metrics_IncludesContainerCount(t *testing.T) {
	rt := &mockRuntime{
		listByLabelsFn: func(_ context.Context, labels map[string]string) ([]core.ContainerInfo, error) {
			if labels["monster.enable"] != "true" {
				t.Errorf("expected monster.enable=true filter, got %v", labels)
			}
			return []core.ContainerInfo{
				{ID: "c1", Name: "n1"},
				{ID: "c2", Name: "n2"},
				{ID: "c3", Name: "n3"},
			}, nil
		},
	}
	le := NewLocalExecutor(rt, "master-01")

	metrics, err := le.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if metrics.ServerID != "master-01" {
		t.Errorf("ServerID = %q", metrics.ServerID)
	}
	if metrics.Containers != 3 {
		t.Errorf("Containers = %d, want 3", metrics.Containers)
	}
	if metrics.Timestamp.IsZero() {
		t.Error("Timestamp was not populated")
	}
}

func TestLocalExecutor_Metrics_ContainerListError_StillReturns(t *testing.T) {
	rt := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return nil, context.DeadlineExceeded
		},
	}
	le := NewLocalExecutor(rt, "master-02")

	metrics, err := le.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics must not fail on container list errors: %v", err)
	}
	if metrics.Containers != 0 {
		t.Errorf("Containers = %d, want 0 on list error", metrics.Containers)
	}
}

// =============================================================================
// AgentClient.handleMetricsCollect
// =============================================================================

func TestAgentClient_HandleMetricsCollect_WithContainerCount(t *testing.T) {
	rt := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{{ID: "a"}, {ID: "b"}}, nil
		},
	}
	client := NewAgentClient("http://master", "agent-m-1", "token", "1.0.0", rt, discardLogger())

	got, err := client.handleMetricsCollect(context.Background(), core.AgentMessage{})
	if err != nil {
		t.Fatalf("handleMetricsCollect: %v", err)
	}
	if got.ServerID != "agent-m-1" {
		t.Errorf("ServerID = %q", got.ServerID)
	}
	if got.Containers != 2 {
		t.Errorf("Containers = %d, want 2", got.Containers)
	}
	if got.Timestamp.IsZero() {
		t.Error("Timestamp was not set")
	}
}

// =============================================================================
// StartHeartbeat / Stop lifecycle
// =============================================================================

func TestStartHeartbeat_StopCleanly(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	// Use a very short interval so the loop runs at least once before we stop.
	s.SetHeartbeat(10*time.Millisecond, 1*time.Second)

	s.StartHeartbeat()
	time.Sleep(25 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// expected
	case <-time.After(2 * time.Second):
		t.Error("Stop did not return within 2s after StartHeartbeat")
	}
}

func TestStop_Idempotent(t *testing.T) {
	s := NewAgentServer(core.NewEventBus(discardLogger()), "tok", discardLogger())
	s.Stop()
	// Second call must not panic on a closed channel.
	s.Stop()
}
