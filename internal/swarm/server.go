package swarm

import (
	"bufio"
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AgentServer accepts WebSocket-style connections from remote agents.
// Master side: listens on GET /api/v1/agent/ws (HTTP hijacked to raw TCP).
// Each agent authenticates with a join token, sends its AgentInfo, and then
// enters a bidirectional JSON message loop.
//
// Lifecycle notes for Tier 76:
//
//   - Pre-Tier-76 readLoop, heartbeatLoop, and per-agent ping goroutines
//     were all fire-and-forget. Stop closed s.stop and canceled agent
//     contexts but never waited for any of them, so Module.Stop could
//     return while goroutines were still mid-write into an
//     about-to-be-closed net.Conn or mid-PublishAsync into an
//     about-to-shutdown EventBus. All three lifetimes are now tracked
//     by a single wg that Stop drains after signaling shutdown.
//   - readLoop, heartbeatLoop, and the ping goroutine had no defer
//     /recover. A panic inside handleAgentMessage or Send would crash
//     the whole master process. All three now recover and log.
//   - NewAgentServer called logger.With on an un-nil-checked logger,
//     so a struct-literal caller (or a test with nil) would NPE before
//     the field was even assigned. Nil now falls back to slog.Default.
//   - removeAgent and heartbeatTick used context.Background() for
//     PublishAsync and per-ping Send, so Stop could not abort work in
//     progress. Both now derive from stopCtx so Stop cancels in-flight
//     work in addition to waking the heartbeat loop.
//   - HandleConnect did not check the closed flag, so an HTTP request
//     arriving between Stop signaling shutdown and Module.Stop
//     returning would spawn a brand-new untracked readLoop on an
//     already-shutting-down server. The closed flag now short-circuits
//     HandleConnect (and StartHeartbeat) after Stop has run.
type AgentServer struct {
	agents        map[string]*AgentConn
	mu            sync.RWMutex
	events        *core.EventBus
	logger        *slog.Logger
	localExec     *LocalExecutor
	expectedToken string

	onConnectMu    sync.RWMutex
	onConnect      []func(core.AgentInfo)
	onDisconnectMu sync.RWMutex
	onDisconnect   []func(string)

	// Heartbeat configuration. Set via SetHeartbeat before Start.
	// interval is how often the master pings each connected agent.
	// deadAfter is how long an agent can go without any message (including
	// a pong) before the master considers it dead and force-closes the conn.
	heartbeatInterval time.Duration
	heartbeatDead     time.Duration

	// Shutdown plumbing. stop is closed by Stop to unblock the
	// heartbeat loop. stopOnce guards close(stop) + stopCancel against
	// concurrent Stop calls. stopCtx is the parent context for every
	// background goroutine (readLoop, heartbeatLoop, per-agent pings,
	// disconnect event publish); Stop cancels it so in-flight work
	// observes the shutdown. wg tracks every background goroutine so
	// Stop can wait for them before returning. closed (mu-guarded) is
	// the single source of truth the spawn sites check before calling
	// wg.Add — this preserves the Add-before-Wait happens-before
	// contract even under a concurrent Stop storm.
	stop       chan struct{}
	stopOnce   sync.Once
	stopCtx    context.Context
	stopCancel context.CancelFunc
	wg         sync.WaitGroup
	closed     bool // guarded by mu
}

// AgentConn represents a single connected agent.
type AgentConn struct {
	ServerID string
	Info     core.AgentInfo
	conn     net.Conn
	encoder  *json.Encoder
	decoder  *json.Decoder
	sendMu   sync.Mutex
	ctx      context.Context
	cancel   context.CancelFunc

	// lastSeen is updated on every message received from this agent (pong,
	// result, metrics report, etc.). The heartbeat monitor uses it to detect
	// silent connections.
	lastSeen   time.Time
	lastSeenMu sync.RWMutex

	// lastMetrics is the most recent ServerMetrics report the agent pushed
	// on its own (via AgentMsgMetricsReport) or the response to a collect
	// request. May be nil if the agent has never reported.
	lastMetrics   *core.ServerMetrics
	lastMetricsMu sync.RWMutex

	// pending tracks in-flight requests waiting for a response.
	pending   map[string]chan core.AgentMessage
	pendingMu sync.Mutex
}

// touch marks that a message from this agent was just observed.
func (ac *AgentConn) touch(now time.Time) {
	ac.lastSeenMu.Lock()
	ac.lastSeen = now
	ac.lastSeenMu.Unlock()
}

// LastSeen returns when the last message from this agent was received.
// Returns the connection's original "now" if nothing has arrived yet.
func (ac *AgentConn) LastSeen() time.Time {
	ac.lastSeenMu.RLock()
	defer ac.lastSeenMu.RUnlock()
	return ac.lastSeen
}

// setLastMetrics records the most recent ServerMetrics report.
func (ac *AgentConn) setLastMetrics(m *core.ServerMetrics) {
	if m == nil {
		return
	}
	cp := *m
	ac.lastMetricsMu.Lock()
	ac.lastMetrics = &cp
	ac.lastMetricsMu.Unlock()
}

// LastMetrics returns a copy of the most recent ServerMetrics report, or nil.
func (ac *AgentConn) LastMetrics() *core.ServerMetrics {
	ac.lastMetricsMu.RLock()
	defer ac.lastMetricsMu.RUnlock()
	if ac.lastMetrics == nil {
		return nil
	}
	cp := *ac.lastMetrics
	return &cp
}

// Default heartbeat cadence and death threshold. The interval must be
// significantly shorter than the connection read deadline (90s) so that a
// single missed ping does not trip the read-loop before the monitor gets a
// chance to decide.
const (
	defaultHeartbeatInterval = 30 * time.Second
	defaultHeartbeatDead     = 90 * time.Second
)

// NewAgentServer creates a new master-side agent connection manager.
// A nil logger is tolerated and replaced with slog.Default() so the
// Tier 76 panic-recovery branches in readLoop/heartbeatLoop cannot NPE
// on a struct-literal or test-constructed server.
func NewAgentServer(events *core.EventBus, token string, logger *slog.Logger) *AgentServer {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &AgentServer{
		agents:            make(map[string]*AgentConn),
		events:            events,
		expectedToken:     token,
		logger:            logger.With("component", "agent-server"),
		heartbeatInterval: defaultHeartbeatInterval,
		heartbeatDead:     defaultHeartbeatDead,
		stop:              make(chan struct{}),
		stopCtx:           ctx,
		stopCancel:        cancel,
	}
}

// SetHeartbeat overrides the default heartbeat interval and death threshold.
// Must be called before StartHeartbeat; both values must be positive, and
// dead must be >= interval. Invalid inputs are ignored.
func (s *AgentServer) SetHeartbeat(interval, dead time.Duration) {
	if interval <= 0 || dead <= 0 || dead < interval {
		return
	}
	s.mu.Lock()
	s.heartbeatInterval = interval
	s.heartbeatDead = dead
	s.mu.Unlock()
}

// SetLocal sets the local node executor so the master itself is available
// through the NodeManager interface.
func (s *AgentServer) SetLocal(local *LocalExecutor) {
	s.localExec = local
}

// HandleConnect is the HTTP handler that upgrades a connection to the agent
// protocol. It must be registered at GET /api/v1/agent/ws.
func (s *AgentServer) HandleConnect(w http.ResponseWriter, r *http.Request) {
	// 1. Verify auth token — prefer X-Agent-Token header (not in URLs), fall back to query
	token := r.Header.Get("X-Agent-Token")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.expectedToken)) != 1 {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Hijack the HTTP connection
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "server does not support hijacking", http.StatusInternalServerError)
		return
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		s.logger.Error("hijack failed", "error", err)
		return
	}

	// Send HTTP 101 Switching Protocols response
	_, _ = buf.WriteString("HTTP/1.1 101 Switching Protocols\r\n")
	_, _ = buf.WriteString("Connection: Upgrade\r\n")
	_, _ = buf.WriteString("Upgrade: deploymonster-agent/1\r\n")
	_, _ = buf.WriteString("\r\n")
	_ = buf.Flush()

	// 3. Create agent connection
	ctx, cancel := context.WithCancel(context.Background())
	ac := &AgentConn{
		conn:     conn,
		encoder:  json.NewEncoder(conn),
		decoder:  json.NewDecoder(bufio.NewReader(conn)),
		ctx:      ctx,
		cancel:   cancel,
		pending:  make(map[string]chan core.AgentMessage),
		lastSeen: time.Now(),
	}

	// 4. Read initial AgentInfo message (with timeout)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var initMsg core.AgentMessage
	if err := ac.decoder.Decode(&initMsg); err != nil {
		s.logger.Error("failed to read agent info", "error", err)
		cancel()
		_ = conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{}) // clear deadline

	// Parse AgentInfo from the payload
	info, err := decodePayload[core.AgentInfo](initMsg.Payload)
	if err != nil {
		s.logger.Error("invalid agent info payload", "error", err)
		cancel()
		_ = conn.Close()
		return
	}

	ac.ServerID = info.ServerID
	ac.Info = info

	// 5. Register the agent. Tier 76: the closed flag + wg.Add happen
	// under the same critical section so a concurrent Stop cannot race
	// past wg.Wait while a new readLoop is being spawned. Once Stop
	// has set closed=true, HandleConnect refuses the connection and
	// tears down the half-initialized ac instead.
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		cancel()
		_ = conn.Close()
		s.logger.Info("rejecting agent connect — server is stopping", "server_id", info.ServerID)
		return
	}
	// Close existing connection for this server if any
	if old, exists := s.agents[info.ServerID]; exists {
		old.cancel()
		_ = old.conn.Close()
	}
	s.agents[info.ServerID] = ac
	s.wg.Add(1)
	s.mu.Unlock()

	s.logger.Info("agent connected",
		"server_id", info.ServerID,
		"hostname", info.Hostname,
		"ip", info.IPAddress,
		"docker", info.DockerVersion,
	)

	// 6. Emit agent.connected event
	if s.events != nil {
		s.events.PublishAsync(ctx, core.NewEvent("agent.connected", "swarm", core.ServerEventData{
			ServerID: info.ServerID,
			Hostname: info.Hostname,
			IP:       info.IPAddress,
		}))
	}

	// Notify callbacks
	s.onConnectMu.RLock()
	for _, fn := range s.onConnect {
		fn(info)
	}
	s.onConnectMu.RUnlock()

	// 7. Start read loop in background. wg.Add already happened under
	// the closed/register critical section above so the happens-before
	// contract with Stop's wg.Wait is preserved.
	go s.readLoop(ac)
}

// readLoop reads messages from a connected agent until the connection drops.
// Tier 76: wg.Done + panic recovery are wired so a crash in
// handleAgentMessage or a slow removeAgent cannot leak the goroutine or
// crash the master.
func (s *AgentServer) readLoop(ac *AgentConn) {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in agent read loop",
				"server_id", ac.ServerID, "error", r)
		}
	}()
	defer s.removeAgent(ac)

	for {
		select {
		case <-ac.ctx.Done():
			return
		default:
		}

		// Set a read deadline to detect dead connections
		_ = ac.conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		var msg core.AgentMessage
		if err := ac.decoder.Decode(&msg); err != nil {
			if ac.ctx.Err() != nil {
				return // context canceled, graceful shutdown
			}
			s.logger.Warn("agent read error", "server_id", ac.ServerID, "error", err)
			return
		}

		s.handleAgentMessage(ac, msg)
	}
}

// handleAgentMessage dispatches an incoming message from an agent.
func (s *AgentServer) handleAgentMessage(ac *AgentConn, msg core.AgentMessage) {
	// Any inbound message is evidence the agent is alive. Update lastSeen
	// before dispatching so the heartbeat monitor sees fresh state even when
	// the handler below blocks on something slow.
	ac.touch(time.Now())

	switch msg.Type {
	case core.AgentMsgPong:
		s.logger.Debug("pong received", "server_id", ac.ServerID)
		// Route pong responses back to pending callers the same way
		// result/error are routed. Callers like SendPing and
		// RemoteExecutor.Ping put the request ID into ac.pending and
		// block on s.Send — if we drop the pong here every ping round
		// trip times out instead of completing. The heartbeat loop
		// relied on this too and was silently burning its 5s context
		// on every tick.
		ac.pendingMu.Lock()
		ch, ok := ac.pending[msg.ID]
		if ok {
			delete(ac.pending, msg.ID)
		}
		ac.pendingMu.Unlock()
		if ok {
			ch <- msg
		}

	case core.AgentMsgResult, core.AgentMsgError:
		// If this result is a metrics response, record it as the most
		// recent snapshot before routing it to the pending waiter.
		if metrics, ok := tryDecodeServerMetrics(msg.Payload); ok {
			ac.setLastMetrics(metrics)
		}
		// Route to pending request
		ac.pendingMu.Lock()
		ch, ok := ac.pending[msg.ID]
		if ok {
			delete(ac.pending, msg.ID)
		}
		ac.pendingMu.Unlock()
		if ok {
			ch <- msg
		}

	case core.AgentMsgMetricsReport:
		if metrics, ok := tryDecodeServerMetrics(msg.Payload); ok {
			ac.setLastMetrics(metrics)
		}
		if s.events != nil {
			s.events.PublishAsync(ac.ctx, core.NewEvent("agent.metrics", "swarm", msg.Payload))
		}

	case core.AgentMsgContainerEvent:
		if s.events != nil {
			s.events.PublishAsync(ac.ctx, core.NewEvent("agent.container_event", "swarm", msg.Payload))
		}

	case core.AgentMsgServerStatus:
		if s.events != nil {
			s.events.PublishAsync(ac.ctx, core.NewEvent("agent.server_status", "swarm", msg.Payload))
		}

	case core.AgentMsgLogStream:
		if s.events != nil {
			s.events.PublishAsync(ac.ctx, core.NewEvent("agent.log_stream", "swarm", msg.Payload))
		}

	default:
		s.logger.Warn("unknown agent message type", "type", msg.Type, "server_id", ac.ServerID)
	}
}

// tryDecodeServerMetrics attempts to decode a payload as core.ServerMetrics.
// Returns (nil, false) if the payload isn't a metrics report — callers should
// treat that as "not a metrics message" rather than an error.
func tryDecodeServerMetrics(payload any) (*core.ServerMetrics, bool) {
	if payload == nil {
		return nil, false
	}
	m, err := decodePayload[core.ServerMetrics](payload)
	if err != nil {
		return nil, false
	}
	// ServerID is the cheapest field to use as a "is this really a metrics
	// report" check — a zero-value decoded struct from an unrelated payload
	// will have empty ServerID.
	if m.ServerID == "" {
		return nil, false
	}
	return &m, true
}

// removeAgent cleans up after an agent disconnects.
func (s *AgentServer) removeAgent(ac *AgentConn) {
	ac.cancel()
	_ = ac.conn.Close()

	s.mu.Lock()
	delete(s.agents, ac.ServerID)
	s.mu.Unlock()

	// Cancel all pending requests
	ac.pendingMu.Lock()
	for id, ch := range ac.pending {
		close(ch)
		delete(ac.pending, id)
	}
	ac.pendingMu.Unlock()

	s.logger.Info("agent disconnected", "server_id", ac.ServerID)

	// Tier 76: publish under the module stopCtx instead of a fresh
	// context.Background() so that Stop can observe the async event
	// drain via EventBus.Drain(), and so that a disconnect racing
	// shutdown does not queue dispatch work the EventBus is already
	// refusing.
	if s.events != nil {
		s.events.PublishAsync(s.pubCtx(), core.NewEvent("agent.disconnected", "swarm", core.ServerEventData{
			ServerID: ac.ServerID,
		}))
	}

	s.onDisconnectMu.RLock()
	for _, fn := range s.onDisconnect {
		fn(ac.ServerID)
	}
	s.onDisconnectMu.RUnlock()
}

// Send sends a message to an agent and waits for the response.
// Returns the response message or an error on timeout.
func (s *AgentServer) Send(ctx context.Context, ac *AgentConn, msg core.AgentMessage) (core.AgentMessage, error) {
	// Create a channel for the response
	ch := make(chan core.AgentMessage, 1)

	ac.pendingMu.Lock()
	ac.pending[msg.ID] = ch
	ac.pendingMu.Unlock()

	// Send the message
	ac.sendMu.Lock()
	err := ac.encoder.Encode(msg)
	ac.sendMu.Unlock()

	if err != nil {
		ac.pendingMu.Lock()
		delete(ac.pending, msg.ID)
		ac.pendingMu.Unlock()
		return core.AgentMessage{}, fmt.Errorf("send to agent %s: %w", ac.ServerID, err)
	}

	// Wait for response with timeout
	select {
	case resp, ok := <-ch:
		if !ok {
			return core.AgentMessage{}, fmt.Errorf("agent %s disconnected", ac.ServerID)
		}
		if resp.Type == core.AgentMsgError {
			errMsg, _ := decodePayload[string](resp.Payload)
			return resp, fmt.Errorf("agent error: %s", errMsg)
		}
		return resp, nil
	case <-ctx.Done():
		ac.pendingMu.Lock()
		delete(ac.pending, msg.ID)
		ac.pendingMu.Unlock()
		return core.AgentMessage{}, ctx.Err()
	}
}

// SendPing sends a ping to the specified agent and waits for pong.
func (s *AgentServer) SendPing(ctx context.Context, serverID string) error {
	s.mu.RLock()
	ac, ok := s.agents[serverID]
	s.mu.RUnlock()
	if !ok {
		return fmt.Errorf("agent %s not connected", serverID)
	}

	msg := core.AgentMessage{
		ID:        core.GenerateID(),
		Type:      core.AgentMsgPing,
		Timestamp: time.Now(),
	}

	_, err := s.Send(ctx, ac, msg)
	return err
}

// ---- NodeManager interface ----

// Get returns a NodeExecutor for the given server ID.
// Returns the local executor for the master, or a remote executor for agents.
func (s *AgentServer) Get(serverID string) (core.NodeExecutor, error) {
	if s.localExec != nil && s.localExec.serverID == serverID {
		return s.localExec, nil
	}

	s.mu.RLock()
	ac, ok := s.agents[serverID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("server %s not connected", serverID)
	}

	return &RemoteExecutor{conn: ac, server: s}, nil
}

// Local returns the local node executor (master's own Docker).
func (s *AgentServer) Local() core.NodeExecutor {
	return s.localExec
}

// All returns all connected server IDs (including local).
func (s *AgentServer) All() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.agents)+1)
	if s.localExec != nil {
		ids = append(ids, s.localExec.serverID)
	}
	for id := range s.agents {
		ids = append(ids, id)
	}
	return ids
}

// OnConnect registers a callback for when an agent connects.
func (s *AgentServer) OnConnect(fn func(info core.AgentInfo)) {
	s.onConnectMu.Lock()
	defer s.onConnectMu.Unlock()
	s.onConnect = append(s.onConnect, fn)
}

// OnDisconnect registers a callback for when an agent disconnects.
func (s *AgentServer) OnDisconnect(fn func(serverID string)) {
	s.onDisconnectMu.Lock()
	defer s.onDisconnectMu.Unlock()
	s.onDisconnect = append(s.onDisconnect, fn)
}

// ConnectedAgents returns info about all currently connected agents.
func (s *AgentServer) ConnectedAgents() []core.AgentInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	agents := make([]core.AgentInfo, 0, len(s.agents))
	for _, ac := range s.agents {
		agents = append(agents, ac.Info)
	}
	return agents
}

// pubCtx returns a context suitable for background PublishAsync calls.
// It prefers the module stopCtx so Stop can cancel dispatch work, but
// falls back to context.Background() if the server was built via struct
// literal and stopCtx was never populated.
func (s *AgentServer) pubCtx() context.Context {
	if s.stopCtx != nil {
		return s.stopCtx
	}
	return context.Background()
}

// Stop gracefully shuts down all agent connections, the heartbeat
// monitor, and every per-agent ping goroutine.
//
// Tier 76 guarantees:
//
//   - stopOnce serializes the close(stop) + stopCancel so concurrent
//     Stop calls never panic with "close of closed channel".
//   - closed is flipped under s.mu BEFORE any wg-tracked work can be
//     newly spawned, which is what gives HandleConnect/StartHeartbeat/
//     heartbeatTick a safe check against wg.Wait.
//   - All agent conns are canceled + closed inside the Do so the
//     readLoops observe EOF on decoder.Decode and exit, and in-flight
//     pings error out of encoder.Encode on the now-closed conn.
//   - wg.Wait drains readLoop, heartbeatLoop, AND per-agent ping
//     goroutines before Stop returns, so Module.Stop can safely tear
//     down the EventBus and the rest of the module graph.
func (s *AgentServer) Stop() {
	s.stopOnce.Do(func() {
		close(s.stop)
		if s.stopCancel != nil {
			s.stopCancel()
		}

		s.mu.Lock()
		s.closed = true
		for id, ac := range s.agents {
			ac.cancel()
			_ = ac.conn.Close()
			delete(s.agents, id)
		}
		s.mu.Unlock()

		s.logger.Info("all agent connections closed")
	})

	// wg.Wait must happen OUTSIDE the Do so a concurrent second Stop
	// call still blocks on the drain instead of returning early.
	s.wg.Wait()
}

// StartHeartbeat launches the background heartbeat monitor. It pings each
// connected agent once per heartbeatInterval, and disconnects any agent whose
// LastSeen is older than heartbeatDead. The loop stops when Stop() is called.
// Safe to call from Module.Start once the server is ready to accept agents.
//
// Tier 76: the closed flag is checked under s.mu before wg.Add so a
// StartHeartbeat racing a Stop cannot register a goroutine after
// wg.Wait has already fired.
func (s *AgentServer) StartHeartbeat() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go s.heartbeatLoop()
}

// heartbeatLoop is the background monitor. It runs until the stop channel is
// closed. On every tick it snapshots the current agent list, sends a ping to
// each one, and force-closes any whose last-seen exceeds the dead threshold.
// Ping sends use a short per-request context so a wedged agent can't block
// the monitor past one interval.
//
// Tier 76: wg.Done + panic recovery are wired so a crash in
// heartbeatTick cannot leak the goroutine or crash the master.
func (s *AgentServer) heartbeatLoop() {
	defer s.wg.Done()
	defer func() {
		if r := recover(); r != nil {
			s.logger.Error("panic in heartbeat loop", "error", r)
		}
	}()

	s.mu.RLock()
	interval := s.heartbeatInterval
	dead := s.heartbeatDead
	s.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.heartbeatTick(dead)
		}
	}
}

// heartbeatTick performs a single sweep across connected agents: deadline-
// check each one, then send an async ping to the rest. Exposed so tests can
// drive it synchronously without waiting for the ticker.
func (s *AgentServer) heartbeatTick(dead time.Duration) {
	now := time.Now()

	// Snapshot the current agents under the read lock so we can release it
	// before touching connection state.
	s.mu.RLock()
	snapshot := make([]*AgentConn, 0, len(s.agents))
	for _, ac := range s.agents {
		snapshot = append(snapshot, ac)
	}
	s.mu.RUnlock()

	for _, ac := range snapshot {
		if now.Sub(ac.LastSeen()) > dead {
			s.logger.Warn("agent heartbeat timeout, disconnecting",
				"server_id", ac.ServerID,
				"last_seen", ac.LastSeen(),
				"dead_after", dead,
			)
			// removeAgent handles the cleanup + event emission. The read
			// loop will observe the cancel/close and return naturally.
			ac.cancel()
			_ = ac.conn.Close()
			continue
		}

		// Fire a ping with a short context derived from the module
		// stopCtx so Stop can abort any in-flight ping instead of
		// waiting the full 5s timeout. Tier 76: the wg.Add is guarded
		// by the closed check under s.mu so a ping spawned during
		// Stop cannot race past wg.Wait, and the ping goroutine wears
		// a defer/recover so a Send panic cannot tear down the master.
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			return
		}
		s.wg.Add(1)
		s.mu.Unlock()

		go func(target *AgentConn) {
			defer s.wg.Done()
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error("panic in heartbeat ping",
						"server_id", target.ServerID, "error", r)
				}
			}()
			parent := s.stopCtx
			if parent == nil {
				parent = context.Background()
			}
			ctx, cancel := context.WithTimeout(parent, 5*time.Second)
			defer cancel()
			msg := core.AgentMessage{
				ID:        core.GenerateID(),
				Type:      core.AgentMsgPing,
				ServerID:  target.ServerID,
				Timestamp: time.Now(),
			}
			_, _ = s.Send(ctx, target, msg)
		}(ac)
	}
}

// AgentHealth is a point-in-time view of one connected agent.
type AgentHealth struct {
	ServerID    string              `json:"server_id"`
	Info        core.AgentInfo      `json:"info"`
	LastSeen    time.Time           `json:"last_seen"`
	SecondsIdle int64               `json:"seconds_idle"`
	Healthy     bool                `json:"healthy"`
	LastMetrics *core.ServerMetrics `json:"last_metrics,omitempty"`
}

// Snapshot returns a sorted-by-ServerID view of all connected agents with
// their heartbeat + metrics state. Safe to call from HTTP handlers.
func (s *AgentServer) Snapshot() []AgentHealth {
	s.mu.RLock()
	dead := s.heartbeatDead
	out := make([]AgentHealth, 0, len(s.agents))
	for _, ac := range s.agents {
		last := ac.LastSeen()
		out = append(out, AgentHealth{
			ServerID:    ac.ServerID,
			Info:        ac.Info,
			LastSeen:    last,
			SecondsIdle: int64(time.Since(last).Seconds()),
			Healthy:     time.Since(last) <= dead,
			LastMetrics: ac.LastMetrics(),
		})
	}
	s.mu.RUnlock()

	// Stable, lexicographic order — makes tests and UIs deterministic.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1].ServerID > out[j].ServerID; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// decodePayload converts an any payload (typically map from JSON) into a typed struct.
func decodePayload[T any](payload any) (T, error) {
	var result T

	// If it's already the right type, return directly
	if typed, ok := payload.(T); ok {
		return typed, nil
	}

	// Otherwise re-marshal and unmarshal
	data, err := json.Marshal(payload)
	if err != nil {
		return result, fmt.Errorf("marshal payload: %w", err)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("unmarshal payload to %T: %w", result, err)
	}
	return result, nil
}
