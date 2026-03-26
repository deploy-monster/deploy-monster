package swarm

import (
	"bufio"
	"context"
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

	// pending tracks in-flight requests waiting for a response.
	pending   map[string]chan core.AgentMessage
	pendingMu sync.Mutex
}

// NewAgentServer creates a new master-side agent connection manager.
func NewAgentServer(events *core.EventBus, token string, logger *slog.Logger) *AgentServer {
	return &AgentServer{
		agents:        make(map[string]*AgentConn),
		events:        events,
		expectedToken: token,
		logger:        logger.With("component", "agent-server"),
	}
}

// SetLocal sets the local node executor so the master itself is available
// through the NodeManager interface.
func (s *AgentServer) SetLocal(local *LocalExecutor) {
	s.localExec = local
}

// HandleConnect is the HTTP handler that upgrades a connection to the agent
// protocol. It must be registered at GET /api/v1/agent/ws.
func (s *AgentServer) HandleConnect(w http.ResponseWriter, r *http.Request) {
	// 1. Verify auth token
	token := r.URL.Query().Get("token")
	if token == "" {
		token = r.Header.Get("X-Agent-Token")
	}
	if token != s.expectedToken {
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
		conn:    conn,
		encoder: json.NewEncoder(conn),
		decoder: json.NewDecoder(bufio.NewReader(conn)),
		ctx:     ctx,
		cancel:  cancel,
		pending: make(map[string]chan core.AgentMessage),
	}

	// 4. Read initial AgentInfo message (with timeout)
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	var initMsg core.AgentMessage
	if err := ac.decoder.Decode(&initMsg); err != nil {
		s.logger.Error("failed to read agent info", "error", err)
		cancel()
		conn.Close()
		return
	}
	_ = conn.SetReadDeadline(time.Time{}) // clear deadline

	// Parse AgentInfo from the payload
	info, err := decodePayload[core.AgentInfo](initMsg.Payload)
	if err != nil {
		s.logger.Error("invalid agent info payload", "error", err)
		cancel()
		conn.Close()
		return
	}

	ac.ServerID = info.ServerID
	ac.Info = info

	// 5. Register the agent
	s.mu.Lock()
	// Close existing connection for this server if any
	if old, exists := s.agents[info.ServerID]; exists {
		old.cancel()
		old.conn.Close()
	}
	s.agents[info.ServerID] = ac
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

	// 7. Start read loop in background
	go s.readLoop(ac)
}

// readLoop reads messages from a connected agent until the connection drops.
func (s *AgentServer) readLoop(ac *AgentConn) {
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
				return // context cancelled, graceful shutdown
			}
			s.logger.Warn("agent read error", "server_id", ac.ServerID, "error", err)
			return
		}

		s.handleAgentMessage(ac, msg)
	}
}

// handleAgentMessage dispatches an incoming message from an agent.
func (s *AgentServer) handleAgentMessage(ac *AgentConn, msg core.AgentMessage) {
	switch msg.Type {
	case core.AgentMsgPong:
		s.logger.Debug("pong received", "server_id", ac.ServerID)

	case core.AgentMsgResult, core.AgentMsgError:
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

// removeAgent cleans up after an agent disconnects.
func (s *AgentServer) removeAgent(ac *AgentConn) {
	ac.cancel()
	ac.conn.Close()

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

	if s.events != nil {
		s.events.PublishAsync(context.Background(), core.NewEvent("agent.disconnected", "swarm", core.ServerEventData{
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

// Stop gracefully shuts down all agent connections.
func (s *AgentServer) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, ac := range s.agents {
		ac.cancel()
		ac.conn.Close()
		delete(s.agents, id)
	}
	s.logger.Info("all agent connections closed")
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
