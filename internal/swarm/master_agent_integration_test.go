//go:build integration
// +build integration

// End-to-end master <-> agent wire protocol test.
//
// Spins up a real AgentServer (master side) on a random loopback TCP
// port and a real AgentClient (agent side) pointed at it. Exercises the
// full round-trip for ping/pong, metrics.collect, health.check, and the
// container.* surface. Every prior test in this package either mocks
// one side at the JSON level or drives the transport with hand-rolled
// raw TCP bytes — this is the first test that uses the AgentClient
// code path the way a real `deploymonster serve --agent` binary would.
//
// Gated behind //go:build integration so it stays out of the default
// `go test ./...` run. CI wires it in via the test-integration job.

package swarm

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// waitFor is a small helper that polls until fn returns true or the
// deadline expires. Used instead of time.Sleep so the test moves on as
// soon as the agent is connected instead of burning a fixed delay.
func waitFor(t *testing.T, timeout time.Duration, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// spinUpMaster starts a fresh AgentServer bound to http://127.0.0.1:<random>.
// The returned teardown closes the http.Server and calls s.Stop so the test
// does not leak goroutines.
func spinUpMaster(t *testing.T, token string) (*AgentServer, string, func()) {
	t.Helper()
	events := core.NewEventBus(slog.Default())
	s := NewAgentServer(events, token, slog.Default())

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	httpServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/v1/agent/ws" {
				s.HandleConnect(w, r)
				return
			}
			http.NotFound(w, r)
		}),
	}
	go func() { _ = httpServer.Serve(ln) }()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	url := fmt.Sprintf("http://%s:%s", host, port)

	teardown := func() {
		_ = httpServer.Close()
		s.Stop()
	}
	return s, url, teardown
}

// TestMasterAgent_Integration_FullProtocol brings up a master + agent
// pair and walks every message type the real agent protocol uses.
func TestMasterAgent_Integration_FullProtocol(t *testing.T) {
	const token = "integration-token"
	const agentID = "agent-integration-1"

	master, masterURL, teardownMaster := spinUpMaster(t, token)
	masterDown := false
	defer func() {
		if !masterDown {
			teardownMaster()
		}
	}()

	// Callback proves the master emits agent.connected events.
	connected := make(chan core.AgentInfo, 1)
	master.OnConnect(func(info core.AgentInfo) { connected <- info })
	disconnected := make(chan string, 1)
	master.OnDisconnect(func(id string) { disconnected <- id })

	// ---- Real AgentClient against the real AgentServer ------------------
	runtime := &mockRuntime{}
	client := NewAgentClient(masterURL, agentID, token, "integration", runtime, slog.Default())

	clientCtx, clientCancel := context.WithCancel(context.Background())
	clientDone := make(chan error, 1)
	go func() { clientDone <- client.Connect(clientCtx) }()
	// Ensure the agent goroutine is unblocked even if the test fails early.
	defer clientCancel()

	// ---- Wait for master to register the agent -------------------------
	select {
	case info := <-connected:
		if info.ServerID != agentID {
			t.Errorf("OnConnect ServerID = %q, want %q", info.ServerID, agentID)
		}
		if info.OS == "" || info.Arch == "" {
			t.Errorf("OnConnect missing os/arch: %+v", info)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for agent to connect")
	}

	waitFor(t, 2*time.Second, "agent in ConnectedAgents", func() bool {
		for _, a := range master.ConnectedAgents() {
			if a.ServerID == agentID {
				return true
			}
		}
		return false
	})

	// ---- RemoteExecutor is the exact API production deploy code uses ---
	exec, err := master.Get(agentID)
	if err != nil {
		t.Fatalf("master.Get(%q): %v", agentID, err)
	}
	if exec.IsLocal() {
		t.Error("Get returned the local executor for a remote agent")
	}
	if exec.ServerID() != agentID {
		t.Errorf("RemoteExecutor ServerID = %q, want %q", exec.ServerID(), agentID)
	}

	// ---- Ping / Pong ---------------------------------------------------
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := exec.Ping(pingCtx); err != nil {
		t.Errorf("RemoteExecutor.Ping: %v", err)
	}
	pingCancel()

	// SendPing is the master's heartbeat wrapper — verify it too.
	spingCtx, spingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := master.SendPing(spingCtx, agentID); err != nil {
		t.Errorf("master.SendPing: %v", err)
	}
	spingCancel()

	// ---- metrics.collect -----------------------------------------------
	metricsCtx, metricsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	metrics, err := exec.Metrics(metricsCtx)
	metricsCancel()
	if err != nil {
		t.Fatalf("RemoteExecutor.Metrics: %v", err)
	}
	if metrics == nil {
		t.Fatal("Metrics returned nil")
	}
	if metrics.ServerID != agentID {
		t.Errorf("metrics.ServerID = %q, want %q", metrics.ServerID, agentID)
	}
	// sys metrics on any real host should populate at least one of these.
	if metrics.RAMTotalMB == 0 && metrics.CPUPercent == 0 && len(metrics.LoadAvg) == 0 {
		t.Errorf("metrics look empty: %+v", metrics)
	}

	// After a metrics round-trip, the server should also have stored a
	// cached snapshot in its per-agent lastMetrics slot.
	waitFor(t, 1*time.Second, "lastMetrics cached on master", func() bool {
		for _, h := range master.Snapshot() {
			if h.ServerID == agentID && h.LastMetrics != nil {
				return true
			}
		}
		return false
	})

	// ---- container.list via RemoteExecutor.ListByLabels ---------------
	listCtx, listCancel := context.WithTimeout(context.Background(), 5*time.Second)
	containers, err := exec.ListByLabels(listCtx, map[string]string{"monster.enable": "true"})
	listCancel()
	if err != nil {
		t.Fatalf("RemoteExecutor.ListByLabels: %v", err)
	}
	if len(containers) != 1 || containers[0].ID != "c1" {
		t.Errorf("ListByLabels = %+v, want single container c1", containers)
	}

	// ---- container.create + stop + restart + remove via full chain ----
	runtime.createAndStartFn = func(_ context.Context, opts core.ContainerOpts) (string, error) {
		if opts.Name != "probe" {
			t.Errorf("agent-side CreateAndStart Name = %q, want %q", opts.Name, "probe")
		}
		return "container-xyz", nil
	}
	createCtx, createCancel := context.WithTimeout(context.Background(), 5*time.Second)
	containerID, err := exec.CreateAndStart(createCtx, core.ContainerOpts{
		Name:  "probe",
		Image: "alpine:latest",
	})
	createCancel()
	if err != nil {
		t.Fatalf("CreateAndStart: %v", err)
	}
	if containerID != "container-xyz" {
		t.Errorf("CreateAndStart returned %q, want %q", containerID, "container-xyz")
	}

	var stoppedID string
	runtime.stopFn = func(_ context.Context, id string, _ int) error {
		stoppedID = id
		return nil
	}
	stopCtx, stopCancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	if err := exec.Stop(stopCtx, containerID, 5); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	stopCancelFn()
	if stoppedID != containerID {
		t.Errorf("agent Stop received %q, want %q", stoppedID, containerID)
	}

	var restartedID string
	runtime.restartFn = func(_ context.Context, id string) error {
		restartedID = id
		return nil
	}
	restartCtx, restartCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := exec.Restart(restartCtx, containerID); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	restartCancel()
	if restartedID != containerID {
		t.Errorf("agent Restart received %q, want %q", restartedID, containerID)
	}

	var removedID string
	var removedForce bool
	runtime.removeFn = func(_ context.Context, id string, force bool) error {
		removedID = id
		removedForce = force
		return nil
	}
	removeCtx, removeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := exec.Remove(removeCtx, containerID, true); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	removeCancel()
	if removedID != containerID || !removedForce {
		t.Errorf("agent Remove received (%q, force=%v), want (%q, true)",
			removedID, removedForce, containerID)
	}

	// ---- container.logs ------------------------------------------------
	// Uses the mockRuntime default "log output" reader.
	logsCtx, logsCancel := context.WithTimeout(context.Background(), 5*time.Second)
	logsReader, err := exec.Logs(logsCtx, containerID, "100", false)
	logsCancel()
	if err != nil {
		t.Fatalf("Logs: %v", err)
	}
	buf := make([]byte, 1024)
	n, _ := logsReader.Read(buf)
	if string(buf[:n]) != "log output" {
		t.Errorf("Logs payload = %q, want %q", string(buf[:n]), "log output")
	}
	logsReader.Close()

	// ---- health.check --------------------------------------------------
	// No wrapper in RemoteExecutor — drive master.Send directly to prove
	// the raw health.check round-trip works.
	master.mu.RLock()
	ac := master.agents[agentID]
	master.mu.RUnlock()
	if ac == nil {
		t.Fatal("expected agent connection still registered on master")
	}
	healthCtx, healthCancel := context.WithTimeout(context.Background(), 5*time.Second)
	resp, err := master.Send(healthCtx, ac, core.AgentMessage{
		ID:        core.GenerateID(),
		Type:      core.AgentMsgHealthCheck,
		ServerID:  agentID,
		Timestamp: time.Now(),
	})
	healthCancel()
	if err != nil {
		t.Fatalf("health.check: %v", err)
	}
	health, err := decodePayload[map[string]any](resp.Payload)
	if err != nil {
		t.Fatalf("decode health payload: %v", err)
	}
	if health["status"] != "ok" {
		t.Errorf("health.status = %v, want ok", health["status"])
	}
	if health["server_id"] != agentID {
		t.Errorf("health.server_id = %v, want %q", health["server_id"], agentID)
	}

	// ---- Unknown command returns error, does not crash -----------------
	errCtx, errCancelFn := context.WithTimeout(context.Background(), 5*time.Second)
	_, sendErr := master.Send(errCtx, ac, core.AgentMessage{
		ID:        core.GenerateID(),
		Type:      "definitely-not-a-real-command",
		ServerID:  agentID,
		Timestamp: time.Now(),
	})
	errCancelFn()
	if sendErr == nil {
		t.Error("unknown command should have produced an agent error response")
	}

	// ---- Shutdown — tear down the master so the agent's blocking
	// decoder unblocks with an EOF. The master-side readLoop observes
	// the agent conn close and fires the disconnect callback; the
	// agent-side readLoop returns from Decode and Connect unwinds. We
	// do this BEFORE waiting on clientDone because AgentClient.readLoop
	// parks on Decode without observing the context, so cancelling
	// clientCtx alone would not wake it within the test deadline.
	teardownMaster()
	masterDown = true
	clientCancel()

	select {
	case <-clientDone:
		// expected — Connect returns once readLoop observes the closed conn
	case <-time.After(5 * time.Second):
		t.Error("agent client did not exit after master teardown")
	}

	// The master should observe the disconnect and fire the callback.
	select {
	case id := <-disconnected:
		if id != agentID {
			t.Errorf("OnDisconnect ServerID = %q, want %q", id, agentID)
		}
	case <-time.After(5 * time.Second):
		t.Error("timed out waiting for disconnect callback")
	}
}
