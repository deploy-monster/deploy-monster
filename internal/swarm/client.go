package swarm

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// defaultAgentPort is the fallback port used when the master URL carries
// no explicit port. It matches the historical DeployMonster HTTPS port.
const defaultAgentPort = 8443

// AgentClient connects to the master server and executes commands
// received over the hijacked HTTP connection. This runs on agent nodes.
type AgentClient struct {
	masterURL   string
	serverID    string
	token       string
	version     string
	runtime     core.ContainerRuntime
	conn        net.Conn
	encoder     *json.Encoder
	decoder     *json.Decoder
	sendMu      sync.Mutex
	logger      *slog.Logger
	sys         core.SysMetricsReader
	defaultPort int
}

// NewAgentClient creates a new agent-side client.
func NewAgentClient(masterURL, serverID, token, version string, rt core.ContainerRuntime, logger *slog.Logger) *AgentClient {
	return &AgentClient{
		masterURL:   strings.TrimRight(masterURL, "/"),
		serverID:    serverID,
		token:       token,
		version:     version,
		runtime:     rt,
		logger:      logger.With("component", "agent-client"),
		sys:         core.NewSysMetricsReader(),
		defaultPort: defaultAgentPort,
	}
}

// SetDefaultPort overrides the fallback port used when the master URL
// does not carry an explicit port. Values <= 0 are ignored.
func (c *AgentClient) SetDefaultPort(port int) {
	if port > 0 {
		c.defaultPort = port
	}
}

// Connect establishes a connection to the master and enters the message loop.
// It blocks until the context is canceled or the connection drops.
func (c *AgentClient) Connect(ctx context.Context) error {
	if err := c.dial(ctx); err != nil {
		return fmt.Errorf("connect to master: %w", err)
	}
	defer func() { _ = c.conn.Close() }()

	// Send initial AgentInfo
	info := c.collectAgentInfo()
	initMsg := core.AgentMessage{
		ID:        core.GenerateID(),
		Type:      "agent.info",
		ServerID:  c.serverID,
		Timestamp: time.Now(),
		Payload:   info,
	}
	if err := c.encoder.Encode(initMsg); err != nil {
		return fmt.Errorf("send agent info: %w", err)
	}

	c.logger.Info("connected to master",
		"master", c.masterURL,
		"server_id", c.serverID,
	)

	// Start read loop
	return c.readLoop(ctx)
}

// ConnectWithRetry connects to the master with exponential backoff.
// It retries indefinitely until the context is canceled.
func (c *AgentClient) ConnectWithRetry(ctx context.Context) error {
	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		err := c.Connect(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		c.logger.Warn("disconnected from master, reconnecting...",
			"error", err,
			"backoff", backoff,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// dial performs the HTTP request and hijacks the connection.
func (c *AgentClient) dial(ctx context.Context) error {
	// Parse master URL to get host:port
	host := c.masterURL
	host = strings.TrimPrefix(host, "https://")
	host = strings.TrimPrefix(host, "http://")

	// Ensure port. A defaultPort of 0 should never happen in practice
	// because NewAgentClient seeds it, but guard anyway.
	if !strings.Contains(host, ":") {
		port := c.defaultPort
		if port <= 0 {
			port = defaultAgentPort
		}
		host = fmt.Sprintf("%s:%d", host, port)
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return fmt.Errorf("dial %s: %w", host, err)
	}

	// Send HTTP upgrade request
	path := fmt.Sprintf("/api/v1/agent/ws?token=%s", c.token)
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: deploymonster-agent/1\r\n\r\n", path, host)
	if _, err := conn.Write([]byte(req)); err != nil {
		_ = conn.Close()
		return fmt.Errorf("send upgrade request: %w", err)
	}

	// Read HTTP response
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("read upgrade response: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return fmt.Errorf("master rejected connection: HTTP %d", resp.StatusCode)
	}

	c.conn = conn
	c.encoder = json.NewEncoder(conn)
	c.decoder = json.NewDecoder(reader)

	return nil
}

// readLoop reads commands from the master and dispatches them.
func (c *AgentClient) readLoop(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_ = c.conn.SetReadDeadline(time.Now().Add(90 * time.Second))

		var msg core.AgentMessage
		if err := c.decoder.Decode(&msg); err != nil {
			return fmt.Errorf("read from master: %w", err)
		}

		go c.handleMessage(ctx, msg)
	}
}

// handleMessage dispatches a command from the master.
func (c *AgentClient) handleMessage(ctx context.Context, msg core.AgentMessage) {
	c.logger.Debug("received command", "type", msg.Type, "id", msg.ID)

	var result any
	var err error

	switch msg.Type {
	case core.AgentMsgPing:
		c.sendResponse(msg.ID, core.AgentMsgPong, nil)
		return

	case core.AgentMsgContainerCreate:
		result, err = c.handleContainerCreate(ctx, msg)
	case core.AgentMsgContainerStop:
		err = c.handleContainerStop(ctx, msg)
	case core.AgentMsgContainerRemove:
		err = c.handleContainerRemove(ctx, msg)
	case core.AgentMsgContainerRestart:
		err = c.handleContainerRestart(ctx, msg)
	case core.AgentMsgContainerList:
		result, err = c.handleContainerList(ctx, msg)
	case core.AgentMsgContainerLogs:
		result, err = c.handleContainerLogs(ctx, msg)
	case core.AgentMsgContainerExec:
		result, err = c.handleContainerExec(ctx, msg)
	case core.AgentMsgImagePull:
		err = c.handleImagePull(ctx, msg)
	case core.AgentMsgMetricsCollect:
		result, err = c.handleMetricsCollect(ctx, msg)
	case core.AgentMsgHealthCheck:
		result, err = c.handleHealthCheck(ctx)
	default:
		err = fmt.Errorf("unknown command type: %s", msg.Type)
	}

	if err != nil {
		c.sendResponse(msg.ID, core.AgentMsgError, err.Error())
		return
	}

	c.sendResponse(msg.ID, core.AgentMsgResult, result)
}

func (c *AgentClient) handleContainerCreate(ctx context.Context, msg core.AgentMessage) (string, error) {
	opts, err := decodePayload[core.ContainerOpts](msg.Payload)
	if err != nil {
		return "", fmt.Errorf("decode container opts: %w", err)
	}
	return c.runtime.CreateAndStart(ctx, opts)
}

func (c *AgentClient) handleContainerStop(ctx context.Context, msg core.AgentMessage) error {
	var p struct {
		ContainerID string `json:"container_id"`
		TimeoutSec  int    `json:"timeout_sec"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode container stop payload: %w", err)
	}
	return c.runtime.Stop(ctx, p.ContainerID, p.TimeoutSec)
}

func (c *AgentClient) handleContainerRemove(ctx context.Context, msg core.AgentMessage) error {
	var p struct {
		ContainerID string `json:"container_id"`
		Force       bool   `json:"force"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode container remove payload: %w", err)
	}
	return c.runtime.Remove(ctx, p.ContainerID, p.Force)
}

func (c *AgentClient) handleContainerRestart(ctx context.Context, msg core.AgentMessage) error {
	var p struct {
		ContainerID string `json:"container_id"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode container restart payload: %w", err)
	}
	return c.runtime.Restart(ctx, p.ContainerID)
}

func (c *AgentClient) handleContainerList(ctx context.Context, msg core.AgentMessage) ([]core.ContainerInfo, error) {
	var p struct {
		Labels map[string]string `json:"labels"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode container list payload: %w", err)
	}
	return c.runtime.ListByLabels(ctx, p.Labels)
}

func (c *AgentClient) handleContainerLogs(ctx context.Context, msg core.AgentMessage) (string, error) {
	var p struct {
		ContainerID string `json:"container_id"`
		Tail        string `json:"tail"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return "", fmt.Errorf("decode container logs payload: %w", err)
	}

	reader, err := c.runtime.Logs(ctx, p.ContainerID, p.Tail, false)
	if err != nil {
		return "", fmt.Errorf("fetch logs for %s: %w", p.ContainerID, err)
	}
	defer func() { _ = reader.Close() }()

	buf := make([]byte, 64*1024) // 64KB max log fetch
	n, _ := reader.Read(buf)
	return string(buf[:n]), nil
}

func (c *AgentClient) handleContainerExec(ctx context.Context, msg core.AgentMessage) (string, error) {
	var p struct {
		ContainerID string   `json:"container_id"`
		Cmd         []string `json:"cmd"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return "", fmt.Errorf("decode container exec payload: %w", err)
	}
	return c.runtime.Exec(ctx, p.ContainerID, p.Cmd)
}

func (c *AgentClient) handleImagePull(ctx context.Context, msg core.AgentMessage) error {
	var p struct {
		Image string `json:"image"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode image pull payload: %w", err)
	}
	return c.runtime.ImagePull(ctx, p.Image)
}

func (c *AgentClient) handleMetricsCollect(ctx context.Context, _ core.AgentMessage) (core.ServerMetrics, error) {
	snap, _ := c.sys.Read()

	metrics := core.ServerMetrics{
		ServerID:    c.serverID,
		Timestamp:   time.Now(),
		CPUPercent:  snap.CPUPercent,
		RAMUsedMB:   snap.RAMUsedMB,
		RAMTotalMB:  snap.RAMTotalMB,
		DiskUsedMB:  snap.DiskUsedMB,
		DiskTotalMB: snap.DiskTotalMB,
		LoadAvg:     snap.LoadAvg,
	}

	// Container count — best-effort, only what DeployMonster manages.
	if c.runtime != nil {
		if containers, err := c.runtime.ListByLabels(ctx, map[string]string{"monster.enable": "true"}); err == nil {
			metrics.Containers = len(containers)
		}
	}

	return metrics, nil
}

func (c *AgentClient) handleHealthCheck(_ context.Context) (map[string]any, error) {
	return map[string]any{
		"status":    "ok",
		"server_id": c.serverID,
		"version":   c.version,
		"runtime":   runtime.GOOS + "/" + runtime.GOARCH,
		"timestamp": time.Now(),
	}, nil
}

// sendResponse sends a response message back to the master.
func (c *AgentClient) sendResponse(requestID, msgType string, payload any) {
	msg := core.AgentMessage{
		ID:        requestID,
		Type:      msgType,
		ServerID:  c.serverID,
		Timestamp: time.Now(),
		Payload:   payload,
	}

	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if err := c.encoder.Encode(msg); err != nil {
		c.logger.Error("failed to send response", "type", msgType, "error", err)
	}
}

// collectAgentInfo gathers information about this agent.
func (c *AgentClient) collectAgentInfo() core.AgentInfo {
	hostname, _ := os.Hostname()

	info := core.AgentInfo{
		ServerID:     c.serverID,
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		AgentVersion: c.version,
		CPUCores:     runtime.NumCPU(),
	}

	// Try to get Docker version from the runtime
	if c.runtime != nil {
		if err := c.runtime.Ping(); err == nil {
			info.DockerVersion = "available"
		}
	}

	return info
}

// decodeInto converts an any payload into the target struct.
func decodeInto(payload any, target any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return json.Unmarshal(data, target)
}
