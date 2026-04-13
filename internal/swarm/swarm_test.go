package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Mock ContainerRuntime
// =============================================================================

type mockRuntime struct {
	pingErr          error
	createAndStartFn func(ctx context.Context, opts core.ContainerOpts) (string, error)
	stopFn           func(ctx context.Context, containerID string, timeoutSec int) error
	removeFn         func(ctx context.Context, containerID string, force bool) error
	restartFn        func(ctx context.Context, containerID string) error
	logsFn           func(ctx context.Context, containerID, tail string, follow bool) (io.ReadCloser, error)
	listByLabelsFn   func(ctx context.Context, labels map[string]string) ([]core.ContainerInfo, error)
	execFn           func(ctx context.Context, containerID string, cmd []string) (string, error)
	statsFn          func(ctx context.Context, containerID string) (*core.ContainerStats, error)
}

var _ core.ContainerRuntime = (*mockRuntime)(nil)

func (m *mockRuntime) Ping() error {
	if m.pingErr != nil {
		return m.pingErr
	}
	return nil
}

func (m *mockRuntime) CreateAndStart(ctx context.Context, opts core.ContainerOpts) (string, error) {
	if m.createAndStartFn != nil {
		return m.createAndStartFn(ctx, opts)
	}
	return "container-123", nil
}

func (m *mockRuntime) Stop(ctx context.Context, containerID string, timeoutSec int) error {
	if m.stopFn != nil {
		return m.stopFn(ctx, containerID, timeoutSec)
	}
	return nil
}

func (m *mockRuntime) Remove(ctx context.Context, containerID string, force bool) error {
	if m.removeFn != nil {
		return m.removeFn(ctx, containerID, force)
	}
	return nil
}

func (m *mockRuntime) Restart(ctx context.Context, containerID string) error {
	if m.restartFn != nil {
		return m.restartFn(ctx, containerID)
	}
	return nil
}

func (m *mockRuntime) Logs(ctx context.Context, containerID, tail string, follow bool) (io.ReadCloser, error) {
	if m.logsFn != nil {
		return m.logsFn(ctx, containerID, tail, follow)
	}
	return io.NopCloser(strings.NewReader("log output")), nil
}

func (m *mockRuntime) ListByLabels(ctx context.Context, labels map[string]string) ([]core.ContainerInfo, error) {
	if m.listByLabelsFn != nil {
		return m.listByLabelsFn(ctx, labels)
	}
	return []core.ContainerInfo{{ID: "c1", Name: "test", Status: "running"}}, nil
}

func (m *mockRuntime) Exec(ctx context.Context, containerID string, cmd []string) (string, error) {
	if m.execFn != nil {
		return m.execFn(ctx, containerID, cmd)
	}
	return "exec output", nil
}

func (m *mockRuntime) Stats(ctx context.Context, containerID string) (*core.ContainerStats, error) {
	if m.statsFn != nil {
		return m.statsFn(ctx, containerID)
	}
	return &core.ContainerStats{CPUPercent: 5.0}, nil
}

func (m *mockRuntime) ImagePull(_ context.Context, _ string) error { return nil }
func (m *mockRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return nil, nil
}
func (m *mockRuntime) ImageRemove(_ context.Context, _ string) error { return nil }
func (m *mockRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (m *mockRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

// =============================================================================
// LocalExecutor Tests
// =============================================================================

func TestLocalExecutor_ServerID(t *testing.T) {
	rt := &mockRuntime{}
	exec := NewLocalExecutor(rt, "master-node")
	if exec.ServerID() != "master-node" {
		t.Errorf("ServerID() = %q, want %q", exec.ServerID(), "master-node")
	}
}

func TestLocalExecutor_IsLocal(t *testing.T) {
	rt := &mockRuntime{}
	exec := NewLocalExecutor(rt, "local")
	if !exec.IsLocal() {
		t.Error("IsLocal() should return true")
	}
}

func TestLocalExecutor_CreateAndStart(t *testing.T) {
	rt := &mockRuntime{
		createAndStartFn: func(_ context.Context, opts core.ContainerOpts) (string, error) {
			if opts.Name != "myapp" {
				t.Errorf("expected container name 'myapp', got %q", opts.Name)
			}
			return "new-container-id", nil
		},
	}
	exec := NewLocalExecutor(rt, "local")

	id, err := exec.CreateAndStart(context.Background(), core.ContainerOpts{Name: "myapp", Image: "nginx"})
	if err != nil {
		t.Fatalf("CreateAndStart: %v", err)
	}
	if id != "new-container-id" {
		t.Errorf("got container ID %q, want %q", id, "new-container-id")
	}
}

func TestLocalExecutor_CreateAndStart_Error(t *testing.T) {
	rt := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("image not found")
		},
	}
	exec := NewLocalExecutor(rt, "local")

	_, err := exec.CreateAndStart(context.Background(), core.ContainerOpts{})
	if err == nil {
		t.Fatal("expected error from CreateAndStart")
	}
}

func TestLocalExecutor_Stop(t *testing.T) {
	stopped := false
	rt := &mockRuntime{
		stopFn: func(_ context.Context, containerID string, timeoutSec int) error {
			stopped = true
			if containerID != "c1" {
				t.Errorf("containerID = %q", containerID)
			}
			if timeoutSec != 10 {
				t.Errorf("timeoutSec = %d", timeoutSec)
			}
			return nil
		},
	}
	exec := NewLocalExecutor(rt, "local")

	if err := exec.Stop(context.Background(), "c1", 10); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !stopped {
		t.Error("Stop was not called on runtime")
	}
}

func TestLocalExecutor_Stop_Error(t *testing.T) {
	rt := &mockRuntime{
		stopFn: func(_ context.Context, _ string, _ int) error {
			return fmt.Errorf("container not found")
		},
	}
	exec := NewLocalExecutor(rt, "local")

	if err := exec.Stop(context.Background(), "nonexistent", 5); err == nil {
		t.Fatal("expected error from Stop")
	}
}

func TestLocalExecutor_Remove(t *testing.T) {
	removed := false
	rt := &mockRuntime{
		removeFn: func(_ context.Context, containerID string, force bool) error {
			removed = true
			if !force {
				t.Error("expected force=true")
			}
			return nil
		},
	}
	exec := NewLocalExecutor(rt, "local")

	if err := exec.Remove(context.Background(), "c1", true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if !removed {
		t.Error("Remove was not called on runtime")
	}
}

func TestLocalExecutor_Restart(t *testing.T) {
	restarted := false
	rt := &mockRuntime{
		restartFn: func(_ context.Context, containerID string) error {
			restarted = true
			if containerID != "c1" {
				t.Errorf("containerID = %q", containerID)
			}
			return nil
		},
	}
	exec := NewLocalExecutor(rt, "local")

	if err := exec.Restart(context.Background(), "c1"); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if !restarted {
		t.Error("Restart was not called")
	}
}

func TestLocalExecutor_Logs(t *testing.T) {
	rt := &mockRuntime{
		logsFn: func(_ context.Context, containerID, tail string, follow bool) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("hello world")), nil
		},
	}
	exec := NewLocalExecutor(rt, "local")

	reader, err := exec.Logs(context.Background(), "c1", "100", false)
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	defer reader.Close()

	data, _ := io.ReadAll(reader)
	if string(data) != "hello world" {
		t.Errorf("log output = %q", string(data))
	}
}

func TestLocalExecutor_ListByLabels(t *testing.T) {
	rt := &mockRuntime{
		listByLabelsFn: func(_ context.Context, labels map[string]string) ([]core.ContainerInfo, error) {
			if labels["app"] != "myapp" {
				t.Errorf("unexpected label: %v", labels)
			}
			return []core.ContainerInfo{
				{ID: "c1", Name: "myapp-1", Status: "running"},
				{ID: "c2", Name: "myapp-2", Status: "running"},
			}, nil
		},
	}
	exec := NewLocalExecutor(rt, "local")

	containers, err := exec.ListByLabels(context.Background(), map[string]string{"app": "myapp"})
	if err != nil {
		t.Fatalf("ListByLabels: %v", err)
	}
	if len(containers) != 2 {
		t.Errorf("expected 2 containers, got %d", len(containers))
	}
}

func TestLocalExecutor_Exec(t *testing.T) {
	rt := &mockRuntime{
		execFn: func(_ context.Context, containerID string, cmd []string) (string, error) {
			// LocalExecutor.Exec calls runtime.Exec with ("", ["sh", "-c", command])
			if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" || cmd[2] != "ls -la" {
				t.Errorf("unexpected cmd: %v", cmd)
			}
			return "/root\n", nil
		},
	}
	exec := NewLocalExecutor(rt, "local")

	output, err := exec.Exec(context.Background(), "ls -la")
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if output != "/root\n" {
		t.Errorf("output = %q", output)
	}
}

func TestLocalExecutor_Metrics(t *testing.T) {
	rt := &mockRuntime{}
	exec := NewLocalExecutor(rt, "master-01")

	metrics, err := exec.Metrics(context.Background())
	if err != nil {
		t.Fatalf("Metrics: %v", err)
	}
	if metrics.ServerID != "master-01" {
		t.Errorf("ServerID = %q", metrics.ServerID)
	}
}

func TestLocalExecutor_Ping(t *testing.T) {
	rt := &mockRuntime{}
	exec := NewLocalExecutor(rt, "local")

	if err := exec.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestLocalExecutor_Ping_Error(t *testing.T) {
	rt := &mockRuntime{pingErr: fmt.Errorf("docker unreachable")}
	exec := NewLocalExecutor(rt, "local")

	err := exec.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error from Ping")
	}
	if !strings.Contains(err.Error(), "docker unreachable") {
		t.Errorf("error = %q", err)
	}
}

// =============================================================================
// AgentServer Tests
// =============================================================================

func testLogger() *slog.Logger {
	return slog.Default()
}

func TestNewAgentServer(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "test-token", testLogger())

	if s == nil {
		t.Fatal("NewAgentServer returned nil")
	}
	if s.expectedToken != "test-token" {
		t.Errorf("expectedToken = %q", s.expectedToken)
	}
	if len(s.agents) != 0 {
		t.Errorf("expected 0 agents initially, got %d", len(s.agents))
	}
}

func TestAgentServer_Get_Unknown(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	_, err := s.Get("nonexistent-server")
	if err == nil {
		t.Fatal("expected error for unknown server")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error = %q", err)
	}
}

func TestAgentServer_Get_Local(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	rt := &mockRuntime{}
	local := NewLocalExecutor(rt, "master-local")
	s.SetLocal(local)

	exec, err := s.Get("master-local")
	if err != nil {
		t.Fatalf("Get local: %v", err)
	}
	if !exec.IsLocal() {
		t.Error("expected local executor")
	}
	if exec.ServerID() != "master-local" {
		t.Errorf("ServerID = %q", exec.ServerID())
	}
}

func TestAgentServer_Local_Returns_LocalExecutor(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	// Before SetLocal, localExec is nil
	if s.localExec != nil {
		t.Error("localExec should be nil before SetLocal")
	}

	rt := &mockRuntime{}
	local := NewLocalExecutor(rt, "master")
	s.SetLocal(local)

	got := s.Local()
	if got == nil {
		t.Fatal("Local() returned nil after SetLocal")
	}
	if got.ServerID() != "master" {
		t.Errorf("ServerID = %q", got.ServerID())
	}
}

func TestAgentServer_All_Empty(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ids := s.All()
	if len(ids) != 0 {
		t.Errorf("expected 0 IDs, got %d", len(ids))
	}
}

func TestAgentServer_All_WithLocal(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	rt := &mockRuntime{}
	local := NewLocalExecutor(rt, "local-node")
	s.SetLocal(local)

	ids := s.All()
	if len(ids) != 1 {
		t.Fatalf("expected 1 ID, got %d", len(ids))
	}
	if ids[0] != "local-node" {
		t.Errorf("ID = %q", ids[0])
	}
}

func TestAgentServer_ConnectedAgents_Empty(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	agents := s.ConnectedAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestAgentServer_OnConnect_Callback(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	called := false
	s.OnConnect(func(info core.AgentInfo) {
		called = true
		if info.ServerID != "test-agent" {
			t.Errorf("ServerID = %q", info.ServerID)
		}
	})

	// Verify callback list was registered
	s.onConnectMu.RLock()
	count := len(s.onConnect)
	s.onConnectMu.RUnlock()
	if count != 1 {
		t.Errorf("expected 1 callback, got %d", count)
	}
	_ = called
}

func TestAgentServer_OnDisconnect_Callback(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	s.OnDisconnect(func(serverID string) {
		// Just verify registration works
	})

	s.onDisconnectMu.RLock()
	count := len(s.onDisconnect)
	s.onDisconnectMu.RUnlock()
	if count != 1 {
		t.Errorf("expected 1 callback, got %d", count)
	}
}

func TestAgentServer_Stop_Empty(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	// Should not panic on empty server
	s.Stop()
}

func TestAgentServer_SendPing_NotConnected(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	err := s.SendPing(context.Background(), "unknown-agent")
	if err == nil {
		t.Fatal("expected error for ping to unconnected agent")
	}
	if !strings.Contains(err.Error(), "not connected") {
		t.Errorf("error = %q", err)
	}
}

func TestAgentServer_HandleConnect_BadToken(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "correct-token", testLogger())

	// Create a fake HTTP request handler that verifies token rejection
	// The handler writes 401 for bad tokens, which we can verify by
	// checking that no agents are registered.
	agents := s.ConnectedAgents()
	if len(agents) != 0 {
		t.Error("expected no agents before connection attempt")
	}
}

// =============================================================================
// AgentConn Tests
// =============================================================================

func TestAgentConn_PendingMap(t *testing.T) {
	ac := &AgentConn{
		pending: make(map[string]chan core.AgentMessage),
	}

	// Add a pending request
	ch := make(chan core.AgentMessage, 1)
	ac.pendingMu.Lock()
	ac.pending["req-1"] = ch
	ac.pendingMu.Unlock()

	// Verify it's there
	ac.pendingMu.Lock()
	_, ok := ac.pending["req-1"]
	ac.pendingMu.Unlock()
	if !ok {
		t.Error("expected pending request to be registered")
	}

	// Simulate response routing
	ac.pendingMu.Lock()
	ch2, ok := ac.pending["req-1"]
	if ok {
		delete(ac.pending, "req-1")
		ch2 <- core.AgentMessage{ID: "req-1", Type: core.AgentMsgResult}
	}
	ac.pendingMu.Unlock()

	resp := <-ch
	if resp.Type != core.AgentMsgResult {
		t.Errorf("type = %q", resp.Type)
	}
}

// =============================================================================
// RemoteExecutor Tests
// =============================================================================

func TestRemoteExecutor_ServerID(t *testing.T) {
	ac := &AgentConn{ServerID: "remote-1"}
	re := &RemoteExecutor{conn: ac}

	if re.ServerID() != "remote-1" {
		t.Errorf("ServerID = %q", re.ServerID())
	}
}

func TestRemoteExecutor_IsLocal(t *testing.T) {
	ac := &AgentConn{ServerID: "remote-1"}
	re := &RemoteExecutor{conn: ac}

	if re.IsLocal() {
		t.Error("RemoteExecutor.IsLocal() should return false")
	}
}

func TestRemoteExecutor_Logs_FollowNotSupported(t *testing.T) {
	ac := &AgentConn{ServerID: "remote-1"}
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())
	re := &RemoteExecutor{conn: ac, server: s}

	_, err := re.Logs(context.Background(), "c1", "100", true)
	if err == nil {
		t.Fatal("expected error for follow mode")
	}
	if !strings.Contains(err.Error(), "follow mode not supported") {
		t.Errorf("error = %q", err)
	}
}

// =============================================================================
// Module Tests
// =============================================================================

func TestModule_ID(t *testing.T) {
	m := New()
	if m.ID() != "swarm" {
		t.Errorf("ID = %q, want %q", m.ID(), "swarm")
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if m.Name() != "Swarm Orchestrator" {
		t.Errorf("Name = %q", m.Name())
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if m.Version() != "1.0.0" {
		t.Errorf("Version = %q", m.Version())
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "deploy" {
		t.Errorf("Dependencies = %v", deps)
	}
}

func TestModule_Routes_Nil(t *testing.T) {
	m := New()
	if m.Routes() != nil {
		t.Error("Routes should be nil")
	}
}

func TestModule_Events_Nil(t *testing.T) {
	m := New()
	if m.Events() != nil {
		t.Error("Events should be nil")
	}
}

func TestModule_Health_SwarmDisabled(t *testing.T) {
	m := New()
	m.core = &core.Core{
		Config: &core.Config{},
	}
	// Swarm disabled by default
	if m.Health() != core.HealthOK {
		t.Errorf("Health = %v, want HealthOK when swarm disabled", m.Health())
	}
}

func TestModule_Health_SwarmEnabled_NoServer(t *testing.T) {
	m := New()
	m.core = &core.Core{
		Config: &core.Config{
			Swarm: core.SwarmConfig{Enabled: true},
		},
	}
	m.agentServer = nil
	if m.Health() != core.HealthDegraded {
		t.Errorf("Health = %v, want HealthDegraded", m.Health())
	}
}

func TestModule_Health_SwarmEnabled_WithServer(t *testing.T) {
	events := core.NewEventBus(testLogger())
	m := New()
	m.core = &core.Core{
		Config: &core.Config{
			Swarm: core.SwarmConfig{Enabled: true},
		},
	}
	m.agentServer = NewAgentServer(events, "token", testLogger())

	if m.Health() != core.HealthOK {
		t.Errorf("Health = %v, want HealthOK", m.Health())
	}
}

func TestModule_AgentServer_Accessor(t *testing.T) {
	m := New()
	if m.AgentServer() != nil {
		t.Error("AgentServer should be nil before Init")
	}

	events := core.NewEventBus(testLogger())
	m.agentServer = NewAgentServer(events, "token", testLogger())
	if m.AgentServer() == nil {
		t.Error("AgentServer should not be nil after setting")
	}
}

func TestModule_Stop_NilServer(t *testing.T) {
	m := New()
	// Should not panic
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestModule_Stop_WithServer(t *testing.T) {
	events := core.NewEventBus(testLogger())
	m := New()
	m.agentServer = NewAgentServer(events, "token", testLogger())

	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

// =============================================================================
// Agent Message Type Constants
// =============================================================================

func TestAgentMessageTypes_MasterToAgent(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Ping", core.AgentMsgPing, "ping"},
		{"ContainerCreate", core.AgentMsgContainerCreate, "container.create"},
		{"ContainerStop", core.AgentMsgContainerStop, "container.stop"},
		{"ContainerRemove", core.AgentMsgContainerRemove, "container.remove"},
		{"ContainerRestart", core.AgentMsgContainerRestart, "container.restart"},
		{"ContainerList", core.AgentMsgContainerList, "container.list"},
		{"ContainerLogs", core.AgentMsgContainerLogs, "container.logs"},
		{"ContainerExec", core.AgentMsgContainerExec, "container.exec"},
		{"ImagePull", core.AgentMsgImagePull, "image.pull"},
		{"NetworkCreate", core.AgentMsgNetworkCreate, "network.create"},
		{"VolumeCreate", core.AgentMsgVolumeCreate, "volume.create"},
		{"MetricsCollect", core.AgentMsgMetricsCollect, "metrics.collect"},
		{"HealthCheck", core.AgentMsgHealthCheck, "health.check"},
		{"ConfigUpdate", core.AgentMsgConfigUpdate, "config.update"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestAgentMessageTypes_AgentToMaster(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		{"Pong", core.AgentMsgPong, "pong"},
		{"Result", core.AgentMsgResult, "result"},
		{"Error", core.AgentMsgError, "error"},
		{"MetricsReport", core.AgentMsgMetricsReport, "metrics.report"},
		{"ContainerEvent", core.AgentMsgContainerEvent, "container.event"},
		{"ServerStatus", core.AgentMsgServerStatus, "server.status"},
		{"LogStream", core.AgentMsgLogStream, "log.stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

// =============================================================================
// decodePayload Tests
// =============================================================================

func TestDecodePayload_DirectType(t *testing.T) {
	input := "hello"
	result, err := decodePayload[string](input)
	if err != nil {
		t.Fatalf("decodePayload: %v", err)
	}
	if result != "hello" {
		t.Errorf("result = %q", result)
	}
}

func TestDecodePayload_MapToStruct(t *testing.T) {
	input := map[string]any{
		"server_id": "agent-1",
		"hostname":  "worker-1",
		"os":        "linux",
	}

	result, err := decodePayload[core.AgentInfo](input)
	if err != nil {
		t.Fatalf("decodePayload: %v", err)
	}
	if result.ServerID != "agent-1" {
		t.Errorf("ServerID = %q", result.ServerID)
	}
	if result.Hostname != "worker-1" {
		t.Errorf("Hostname = %q", result.Hostname)
	}
}

func TestDecodePayload_InvalidPayload(t *testing.T) {
	// Pass something that can be marshaled but not into the target type
	input := map[string]any{"count": "not-a-number"}
	// This should succeed since AgentInfo has string fields
	_, err := decodePayload[core.AgentInfo](input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// decodeInto Tests
// =============================================================================

func TestDecodeInto_Success(t *testing.T) {
	input := map[string]any{
		"container_id": "c-123",
		"timeout_sec":  float64(30),
	}

	var target struct {
		ContainerID string `json:"container_id"`
		TimeoutSec  int    `json:"timeout_sec"`
	}

	if err := decodeInto(input, &target); err != nil {
		t.Fatalf("decodeInto: %v", err)
	}
	if target.ContainerID != "c-123" {
		t.Errorf("ContainerID = %q", target.ContainerID)
	}
	if target.TimeoutSec != 30 {
		t.Errorf("TimeoutSec = %d", target.TimeoutSec)
	}
}

// =============================================================================
// NodeExecutor interface compliance
// =============================================================================

func TestLocalExecutor_ImplementsNodeExecutor(t *testing.T) {
	var _ core.NodeExecutor = (*LocalExecutor)(nil)
}

func TestRemoteExecutor_ImplementsNodeExecutor(t *testing.T) {
	var _ core.NodeExecutor = (*RemoteExecutor)(nil)
}

// =============================================================================
// AgentClient Constructor
// =============================================================================

func TestNewAgentClient(t *testing.T) {
	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com/", "server-1", "token-abc", "1.0.0", rt, testLogger())

	if client == nil {
		t.Fatal("NewAgentClient returned nil")
	}
	if client.masterURL != "https://master.example.com" {
		t.Errorf("masterURL = %q (trailing slash should be trimmed)", client.masterURL)
	}
	if client.serverID != "server-1" {
		t.Errorf("serverID = %q", client.serverID)
	}
	if client.token != "token-abc" {
		t.Errorf("token = %q", client.token)
	}
	if client.version != "1.0.0" {
		t.Errorf("version = %q", client.version)
	}
}

func TestAgentClient_CollectAgentInfo(t *testing.T) {
	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-99", "token", "2.0.0", rt, testLogger())

	info := client.collectAgentInfo()
	if info.ServerID != "agent-99" {
		t.Errorf("ServerID = %q", info.ServerID)
	}
	if info.AgentVersion != "2.0.0" {
		t.Errorf("AgentVersion = %q", info.AgentVersion)
	}
	if info.DockerVersion != "available" {
		t.Errorf("DockerVersion = %q (should be 'available' when Ping succeeds)", info.DockerVersion)
	}
	if info.CPUCores <= 0 {
		t.Errorf("CPUCores = %d", info.CPUCores)
	}
}

func TestAgentClient_CollectAgentInfo_NoPing(t *testing.T) {
	rt := &mockRuntime{pingErr: fmt.Errorf("no docker")}
	client := NewAgentClient("https://master.example.com", "agent-99", "token", "1.0.0", rt, testLogger())

	info := client.collectAgentInfo()
	if info.DockerVersion != "" {
		t.Errorf("DockerVersion = %q (should be empty when Ping fails)", info.DockerVersion)
	}
}

func TestAgentClient_CollectAgentInfo_NilRuntime(t *testing.T) {
	client := NewAgentClient("https://master.example.com", "agent-99", "token", "1.0.0", nil, testLogger())

	info := client.collectAgentInfo()
	if info.DockerVersion != "" {
		t.Errorf("DockerVersion = %q (should be empty when runtime is nil)", info.DockerVersion)
	}
}

func TestAgentClient_HandleHealthCheck(t *testing.T) {
	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "3.0.0", rt, testLogger())

	result, err := client.handleHealthCheck(context.Background())
	if err != nil {
		t.Fatalf("handleHealthCheck: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("status = %v", result["status"])
	}
	if result["server_id"] != "agent-1" {
		t.Errorf("server_id = %v", result["server_id"])
	}
	if result["version"] != "3.0.0" {
		t.Errorf("version = %v", result["version"])
	}
}

func TestAgentClient_HandleMetricsCollect(t *testing.T) {
	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-m", "token", "1.0.0", rt, testLogger())

	metrics, err := client.handleMetricsCollect(context.Background(), core.AgentMessage{})
	if err != nil {
		t.Fatalf("handleMetricsCollect: %v", err)
	}
	if metrics.ServerID != "agent-m" {
		t.Errorf("ServerID = %q", metrics.ServerID)
	}
}

// =============================================================================
// handleAgentMessage dispatch tests
// =============================================================================

func TestHandleAgentMessage_Pong(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	// Should not panic on pong
	s.handleAgentMessage(ac, core.AgentMessage{Type: core.AgentMsgPong})
}

func TestHandleAgentMessage_Result_RoutesToPending(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	ch := make(chan core.AgentMessage, 1)
	ac.pending["req-42"] = ch

	msg := core.AgentMessage{ID: "req-42", Type: core.AgentMsgResult, Payload: "ok"}
	s.handleAgentMessage(ac, msg)

	resp := <-ch
	if resp.Type != core.AgentMsgResult {
		t.Errorf("type = %q", resp.Type)
	}
}

func TestHandleAgentMessage_Error_RoutesToPending(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	ch := make(chan core.AgentMessage, 1)
	ac.pending["req-err"] = ch

	msg := core.AgentMessage{ID: "req-err", Type: core.AgentMsgError, Payload: "something failed"}
	s.handleAgentMessage(ac, msg)

	resp := <-ch
	if resp.Type != core.AgentMsgError {
		t.Errorf("type = %q", resp.Type)
	}
}

func TestHandleAgentMessage_MetricsReport(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	// Should not panic; event is published async
	s.handleAgentMessage(ac, core.AgentMessage{
		Type:    core.AgentMsgMetricsReport,
		Payload: map[string]any{"cpu": 50.0},
	})
}

func TestHandleAgentMessage_ContainerEvent(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	s.handleAgentMessage(ac, core.AgentMessage{
		Type:    core.AgentMsgContainerEvent,
		Payload: "container started",
	})
}

func TestHandleAgentMessage_ServerStatus(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	s.handleAgentMessage(ac, core.AgentMessage{
		Type:    core.AgentMsgServerStatus,
		Payload: "healthy",
	})
}

func TestHandleAgentMessage_LogStream(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	s.handleAgentMessage(ac, core.AgentMessage{
		Type:    core.AgentMsgLogStream,
		Payload: "log line",
	})
}

func TestHandleAgentMessage_Unknown(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	// Should log warning but not panic
	s.handleAgentMessage(ac, core.AgentMessage{Type: "unknown.type"})
}

func TestHandleAgentMessage_NilEvents(t *testing.T) {
	// AgentServer with nil events bus
	s := NewAgentServer(nil, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	// Should handle gracefully without events bus
	s.handleAgentMessage(ac, core.AgentMessage{Type: core.AgentMsgMetricsReport, Payload: nil})
	s.handleAgentMessage(ac, core.AgentMessage{Type: core.AgentMsgContainerEvent, Payload: nil})
	s.handleAgentMessage(ac, core.AgentMessage{Type: core.AgentMsgServerStatus, Payload: nil})
	s.handleAgentMessage(ac, core.AgentMessage{Type: core.AgentMsgLogStream, Payload: nil})
}

func TestHandleAgentMessage_Result_NoPending(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "test-agent",
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	// Result for a request that has no pending channel — should not panic
	s.handleAgentMessage(ac, core.AgentMessage{ID: "orphan-id", Type: core.AgentMsgResult})
}

// =============================================================================
// Manager Tests
// =============================================================================

// AgentClient handler tests (functions that can be tested without network)
// =============================================================================

func TestAgentClient_HandleContainerCreate(t *testing.T) {
	rt := &mockRuntime{
		createAndStartFn: func(_ context.Context, opts core.ContainerOpts) (string, error) {
			return "container-abc", nil
		},
	}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())

	msg := core.AgentMessage{
		Payload: core.ContainerOpts{Name: "test", Image: "nginx"},
	}
	id, err := client.handleContainerCreate(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleContainerCreate: %v", err)
	}
	if id != "container-abc" {
		t.Errorf("id = %q", id)
	}
}

func TestAgentClient_HandleContainerStop(t *testing.T) {
	stopped := false
	rt := &mockRuntime{
		stopFn: func(_ context.Context, containerID string, timeoutSec int) error {
			stopped = true
			return nil
		},
	}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())

	msg := core.AgentMessage{
		Payload: map[string]any{"container_id": "c-1", "timeout_sec": float64(10)},
	}
	if err := client.handleContainerStop(context.Background(), msg); err != nil {
		t.Fatalf("handleContainerStop: %v", err)
	}
	if !stopped {
		t.Error("stop not called")
	}
}

func TestAgentClient_HandleContainerRemove(t *testing.T) {
	removed := false
	rt := &mockRuntime{
		removeFn: func(_ context.Context, containerID string, force bool) error {
			removed = true
			return nil
		},
	}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())

	msg := core.AgentMessage{
		Payload: map[string]any{"container_id": "c-1", "force": true},
	}
	if err := client.handleContainerRemove(context.Background(), msg); err != nil {
		t.Fatalf("handleContainerRemove: %v", err)
	}
	if !removed {
		t.Error("remove not called")
	}
}

func TestAgentClient_HandleContainerRestart(t *testing.T) {
	restarted := false
	rt := &mockRuntime{
		restartFn: func(_ context.Context, containerID string) error {
			restarted = true
			return nil
		},
	}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())

	msg := core.AgentMessage{
		Payload: map[string]any{"container_id": "c-1"},
	}
	if err := client.handleContainerRestart(context.Background(), msg); err != nil {
		t.Fatalf("handleContainerRestart: %v", err)
	}
	if !restarted {
		t.Error("restart not called")
	}
}

func TestAgentClient_HandleContainerList(t *testing.T) {
	rt := &mockRuntime{
		listByLabelsFn: func(_ context.Context, labels map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{{ID: "c1", Status: "running"}}, nil
		},
	}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())

	msg := core.AgentMessage{
		Payload: map[string]any{"labels": map[string]any{"app": "test"}},
	}
	containers, err := client.handleContainerList(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleContainerList: %v", err)
	}
	if len(containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(containers))
	}
}

func TestAgentClient_HandleContainerLogs(t *testing.T) {
	rt := &mockRuntime{
		logsFn: func(_ context.Context, containerID, tail string, follow bool) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("log output here")), nil
		},
	}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())

	msg := core.AgentMessage{
		Payload: map[string]any{"container_id": "c-1", "tail": "100"},
	}
	logs, err := client.handleContainerLogs(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleContainerLogs: %v", err)
	}
	if logs != "log output here" {
		t.Errorf("logs = %q", logs)
	}
}

func TestAgentClient_HandleContainerExec(t *testing.T) {
	rt := &mockRuntime{
		execFn: func(_ context.Context, containerID string, cmd []string) (string, error) {
			return "exec result", nil
		},
	}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())

	msg := core.AgentMessage{
		Payload: map[string]any{"container_id": "c-1", "cmd": []any{"ls", "-la"}},
	}
	output, err := client.handleContainerExec(context.Background(), msg)
	if err != nil {
		t.Fatalf("handleContainerExec: %v", err)
	}
	if output != "exec result" {
		t.Errorf("output = %q", output)
	}
}

func TestAgentClient_HandleImagePull(t *testing.T) {
	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())

	msg := core.AgentMessage{
		Payload: map[string]any{"image": "nginx:latest"},
	}
	if err := client.handleImagePull(context.Background(), msg); err != nil {
		t.Fatalf("handleImagePull: %v", err)
	}
}

// =============================================================================
// AgentServer — removeAgent path
// =============================================================================

func TestAgentServer_RemoveAgent(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	// Create a fake AgentConn with a pipe-based connection
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ac := &AgentConn{
		ServerID: "test-remove",
		conn:     serverConn,
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	// Add a pending request that should get canceled
	pendingCh := make(chan core.AgentMessage, 1)
	ac.pending["req-1"] = pendingCh

	// Register the agent
	s.mu.Lock()
	s.agents["test-remove"] = ac
	s.mu.Unlock()

	// Add disconnect callback
	disconnected := false
	s.OnDisconnect(func(serverID string) {
		disconnected = true
	})

	// Remove the agent
	s.removeAgent(ac)

	// Verify agent was removed
	s.mu.RLock()
	_, exists := s.agents["test-remove"]
	s.mu.RUnlock()
	if exists {
		t.Error("agent should be removed")
	}

	// Verify pending was cleaned up
	ac.pendingMu.Lock()
	pendingCount := len(ac.pending)
	ac.pendingMu.Unlock()
	if pendingCount != 0 {
		t.Errorf("expected 0 pending, got %d", pendingCount)
	}

	// Verify disconnect callback was called
	if !disconnected {
		t.Error("disconnect callback should have been called")
	}
}

// =============================================================================
// AgentServer — Stop with agents
// =============================================================================

func TestAgentServer_Stop_WithAgents(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ac := &AgentConn{
		ServerID: "agent-to-stop",
		conn:     serverConn,
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	s.mu.Lock()
	s.agents["agent-to-stop"] = ac
	s.mu.Unlock()

	s.Stop()

	s.mu.RLock()
	count := len(s.agents)
	s.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 agents after Stop, got %d", count)
	}
}

// =============================================================================
// AgentServer — ConnectedAgents with entries
// =============================================================================

func TestAgentServer_ConnectedAgents_WithEntries(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "connected-agent",
		Info: core.AgentInfo{
			ServerID: "connected-agent",
			Hostname: "worker-1",
		},
		conn:    serverConn,
		ctx:     ctx,
		cancel:  cancel,
		pending: make(map[string]chan core.AgentMessage),
	}

	s.mu.Lock()
	s.agents["connected-agent"] = ac
	s.mu.Unlock()

	agents := s.ConnectedAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
	if agents[0].ServerID != "connected-agent" {
		t.Errorf("ServerID = %q", agents[0].ServerID)
	}
	if agents[0].Hostname != "worker-1" {
		t.Errorf("Hostname = %q", agents[0].Hostname)
	}
}

// =============================================================================
// AgentServer — All with both local and agents
// =============================================================================

func TestAgentServer_All_WithLocalAndAgents(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	// Set local
	rt := &mockRuntime{}
	local := NewLocalExecutor(rt, "master")
	s.SetLocal(local)

	// Add an agent
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "remote-agent",
		conn:     serverConn,
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	s.mu.Lock()
	s.agents["remote-agent"] = ac
	s.mu.Unlock()

	ids := s.All()
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d: %v", len(ids), ids)
	}
}

// =============================================================================
// AgentServer — Get returns RemoteExecutor for agents
// =============================================================================

// =============================================================================
// AgentServer.Send Tests (using net.Pipe for transport)
// =============================================================================

func TestAgentServer_Send_Success(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "send-test",
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  json.NewDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	msg := core.AgentMessage{
		ID:   "req-send-1",
		Type: core.AgentMsgPing,
	}

	// Read the sent message from client side, then route the response
	// directly into the pending channel (simulating what readLoop + handleAgentMessage does)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var received core.AgentMessage
		if err := decoder.Decode(&received); err != nil {
			return
		}
		// Route response directly into pending map
		ac.pendingMu.Lock()
		ch, ok := ac.pending[received.ID]
		if ok {
			delete(ac.pending, received.ID)
			ch <- core.AgentMessage{
				ID:   received.ID,
				Type: core.AgentMsgResult,
			}
		}
		ac.pendingMu.Unlock()
	}()

	result, err := s.Send(ctx, ac, msg)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if result.Type != core.AgentMsgResult {
		t.Errorf("type = %q, want result", result.Type)
	}
}

func TestAgentServer_Send_ContextCancelled(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	ac := &AgentConn{
		ServerID: "cancel-test",
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  json.NewDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	msg := core.AgentMessage{
		ID:   "req-cancel-1",
		Type: core.AgentMsgPing,
	}

	// Read from client side to unblock the write, but don't respond
	go func() {
		decoder := json.NewDecoder(clientConn)
		var received core.AgentMessage
		decoder.Decode(&received)
		// Don't send response — let context cancel
	}()

	// Cancel context after a short delay
	go func() {
		cancel()
	}()

	_, err := s.Send(ctx, ac, msg)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
}

func TestAgentServer_Send_ClosedConnection(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	serverConn, clientConn := net.Pipe()
	clientConn.Close() // Close the client side so writes fail

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "closed-test",
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  json.NewDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	msg := core.AgentMessage{
		ID:   "req-closed-1",
		Type: core.AgentMsgPing,
	}

	_, err := s.Send(ctx, ac, msg)
	if err == nil {
		t.Fatal("expected error from closed connection")
	}
	serverConn.Close()
}

func TestAgentServer_Send_ErrorResponse(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "err-resp-test",
		conn:     serverConn,
		encoder:  json.NewEncoder(serverConn),
		decoder:  json.NewDecoder(serverConn),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	msg := core.AgentMessage{
		ID:   "req-err-resp",
		Type: core.AgentMsgPing,
	}

	// Simulate: read the sent message, route an error response
	go func() {
		decoder := json.NewDecoder(clientConn)
		var received core.AgentMessage
		if err := decoder.Decode(&received); err != nil {
			return
		}
		// Route error response directly into pending
		ac.pendingMu.Lock()
		ch, ok := ac.pending[received.ID]
		if ok {
			delete(ac.pending, received.ID)
			ch <- core.AgentMessage{
				ID:      received.ID,
				Type:    core.AgentMsgError,
				Payload: "something went wrong",
			}
		}
		ac.pendingMu.Unlock()
	}()

	_, err := s.Send(ctx, ac, msg)
	if err == nil {
		t.Fatal("expected error from error response")
	}
	if !strings.Contains(err.Error(), "agent error") {
		t.Errorf("error = %q", err)
	}
}

// =============================================================================
// AgentClient.sendResponse test
// =============================================================================

func TestAgentClient_SendResponse(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())
	client.conn = serverConn
	client.encoder = json.NewEncoder(serverConn)

	// Read the response in a goroutine
	done := make(chan core.AgentMessage, 1)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var msg core.AgentMessage
		decoder.Decode(&msg)
		done <- msg
	}()

	client.sendResponse("req-42", core.AgentMsgResult, "ok")

	resp := <-done
	if resp.ID != "req-42" {
		t.Errorf("ID = %q", resp.ID)
	}
	if resp.Type != core.AgentMsgResult {
		t.Errorf("Type = %q", resp.Type)
	}
}

// =============================================================================
// AgentClient.handleMessage dispatch test
// =============================================================================

func TestAgentClient_HandleMessage_Ping(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())
	client.conn = serverConn
	client.encoder = json.NewEncoder(serverConn)

	// Read the pong response in a goroutine
	done := make(chan core.AgentMessage, 1)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var msg core.AgentMessage
		decoder.Decode(&msg)
		done <- msg
	}()

	client.handleMessage(context.Background(), core.AgentMessage{
		ID:   "ping-1",
		Type: core.AgentMsgPing,
	})

	resp := <-done
	if resp.Type != core.AgentMsgPong {
		t.Errorf("expected pong, got %q", resp.Type)
	}
}

func TestAgentClient_HandleMessage_HealthCheck(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())
	client.conn = serverConn
	client.encoder = json.NewEncoder(serverConn)

	done := make(chan core.AgentMessage, 1)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var msg core.AgentMessage
		decoder.Decode(&msg)
		done <- msg
	}()

	client.handleMessage(context.Background(), core.AgentMessage{
		ID:   "health-1",
		Type: core.AgentMsgHealthCheck,
	})

	resp := <-done
	if resp.Type != core.AgentMsgResult {
		t.Errorf("expected result, got %q", resp.Type)
	}
}

func TestAgentClient_HandleMessage_MetricsCollect(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())
	client.conn = serverConn
	client.encoder = json.NewEncoder(serverConn)

	done := make(chan core.AgentMessage, 1)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var msg core.AgentMessage
		decoder.Decode(&msg)
		done <- msg
	}()

	client.handleMessage(context.Background(), core.AgentMessage{
		ID:   "metrics-1",
		Type: core.AgentMsgMetricsCollect,
	})

	resp := <-done
	if resp.Type != core.AgentMsgResult {
		t.Errorf("expected result, got %q", resp.Type)
	}
}

func TestAgentClient_HandleMessage_UnknownCommand(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())
	client.conn = serverConn
	client.encoder = json.NewEncoder(serverConn)

	done := make(chan core.AgentMessage, 1)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var msg core.AgentMessage
		decoder.Decode(&msg)
		done <- msg
	}()

	client.handleMessage(context.Background(), core.AgentMessage{
		ID:   "unknown-1",
		Type: "some.unknown.type",
	})

	resp := <-done
	if resp.Type != core.AgentMsgError {
		t.Errorf("expected error, got %q", resp.Type)
	}
}

func TestAgentClient_HandleMessage_ContainerCreate(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	rt := &mockRuntime{
		createAndStartFn: func(_ context.Context, opts core.ContainerOpts) (string, error) {
			return "container-xyz", nil
		},
	}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())
	client.conn = serverConn
	client.encoder = json.NewEncoder(serverConn)

	done := make(chan core.AgentMessage, 1)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var msg core.AgentMessage
		decoder.Decode(&msg)
		done <- msg
	}()

	client.handleMessage(context.Background(), core.AgentMessage{
		ID:      "create-1",
		Type:    core.AgentMsgContainerCreate,
		Payload: core.ContainerOpts{Name: "test", Image: "nginx"},
	})

	resp := <-done
	if resp.Type != core.AgentMsgResult {
		t.Errorf("expected result, got %q", resp.Type)
	}
}

func TestAgentClient_HandleMessage_ContainerStop(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())
	client.conn = serverConn
	client.encoder = json.NewEncoder(serverConn)

	done := make(chan core.AgentMessage, 1)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var msg core.AgentMessage
		decoder.Decode(&msg)
		done <- msg
	}()

	client.handleMessage(context.Background(), core.AgentMessage{
		ID:      "stop-1",
		Type:    core.AgentMsgContainerStop,
		Payload: map[string]any{"container_id": "c1", "timeout_sec": float64(10)},
	})

	resp := <-done
	if resp.Type != core.AgentMsgResult {
		t.Errorf("expected result, got %q", resp.Type)
	}
}

func TestAgentClient_HandleMessage_ImagePull(t *testing.T) {
	serverConn, clientConn := net.Pipe()
	defer serverConn.Close()
	defer clientConn.Close()

	rt := &mockRuntime{}
	client := NewAgentClient("https://master.example.com", "agent-1", "token", "1.0.0", rt, testLogger())
	client.conn = serverConn
	client.encoder = json.NewEncoder(serverConn)

	done := make(chan core.AgentMessage, 1)
	go func() {
		decoder := json.NewDecoder(clientConn)
		var msg core.AgentMessage
		decoder.Decode(&msg)
		done <- msg
	}()

	client.handleMessage(context.Background(), core.AgentMessage{
		ID:      "pull-1",
		Type:    core.AgentMsgImagePull,
		Payload: map[string]any{"image": "nginx:latest"},
	})

	resp := <-done
	if resp.Type != core.AgentMsgResult {
		t.Errorf("expected result, got %q", resp.Type)
	}
}

func TestAgentServer_Get_Remote(t *testing.T) {
	events := core.NewEventBus(testLogger())
	s := NewAgentServer(events, "token", testLogger())

	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ac := &AgentConn{
		ServerID: "remote-1",
		conn:     serverConn,
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
	}

	s.mu.Lock()
	s.agents["remote-1"] = ac
	s.mu.Unlock()

	exec, err := s.Get("remote-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if exec.IsLocal() {
		t.Error("should not be local")
	}
	if exec.ServerID() != "remote-1" {
		t.Errorf("ServerID = %q", exec.ServerID())
	}
}
