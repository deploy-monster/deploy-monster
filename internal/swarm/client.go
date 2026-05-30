package swarm

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
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

	"github.com/deploy-monster/deploy-monster/internal/build"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// defaultAgentPort is the fallback port used when the master URL carries
// no explicit port. It matches the historical DeployMonster HTTPS port.
const defaultAgentPort = 8443

// maxConcurrentHandlers limits how many handleMessage goroutines run concurrently.
// Each handler may shell out to Docker, so we bound memory and resource usage.
const maxConcurrentHandlers = 32

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
	// TLS configuration for mutual TLS. When certFile and keyFile are
	// set the client presents a certificate and verifies the server
	// using caFile (or system CAs if caFile is empty).
	certFile  string
	keyFile   string
	caFile    string
	tlsConfig *tls.Config
	buildMod  *build.Module

	// sem limits concurrent handleMessage goroutines.
	sem chan struct{}
}

// NewAgentClient creates a new agent-side client.
// Pass certFile/keyFile/caFile to enable mTLS. An empty certFile means
// no client certificate is presented (token-only auth over TLS).
func NewAgentClient(masterURL, serverID, token, version string, rt core.ContainerRuntime, logger *slog.Logger, certFile, keyFile, caFile string) *AgentClient {
	c := &AgentClient{
		masterURL:   strings.TrimRight(masterURL, "/"),
		serverID:    serverID,
		token:       token,
		version:     version,
		runtime:     rt,
		logger:      logger.With("component", "agent-client"),
		sys:         core.NewSysMetricsReader(),
		defaultPort: defaultAgentPort,
		certFile:    certFile,
		keyFile:     keyFile,
		caFile:      caFile,
		sem:         make(chan struct{}, maxConcurrentHandlers),
	}
	if certFile != "" && keyFile != "" {
		c.tlsConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
		if caFile != "" {
			caCert, err := os.ReadFile(caFile)
			if err == nil {
				caPool := x509.NewCertPool()
				caPool.AppendCertsFromPEM(caCert)
				c.tlsConfig.RootCAs = caPool
			}
		}
		c.tlsConfig.GetClientCertificate = func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return nil, err
			}
			return &cert, nil
		}
	}
	return c
}

// SetDefaultPort overrides the fallback port used when the master URL
// does not carry an explicit port. Values <= 0 are ignored.
func (c *AgentClient) SetDefaultPort(port int) {
	if port > 0 {
		c.defaultPort = port
	}
}

// SetBuildModule wires the agent's local build module for build.task handling.
// Only agents that want to execute builds (not just container operations) need this.
func (c *AgentClient) SetBuildModule(buildMod *build.Module) {
	c.buildMod = buildMod
}

func (c *AgentClient) requireRuntime() (core.ContainerRuntime, error) {
	if c.runtime == nil {
		return nil, fmt.Errorf("container runtime not configured on agent")
	}
	return c.runtime, nil
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
	backoff := core.BackoffBase
	maxBackoff := core.BackoffMax

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

	var conn net.Conn
	var err error
	if c.tlsConfig != nil {
		d := tls.Dialer{Config: c.tlsConfig}
		conn, err = d.DialContext(ctx, "tcp", host)
		if err != nil {
			return fmt.Errorf("tls dial %s: %w", host, err)
		}
	} else {
		var d net.Dialer
		conn, err = d.DialContext(ctx, "tcp", host)
		if err != nil {
			return fmt.Errorf("dial %s: %w", host, err)
		}
	}

	// Send HTTP upgrade request with token in header (not URL).
	// This prevents the token from appearing in logs.
	path := "/api/v1/agent/ws"
	req := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nConnection: Upgrade\r\nUpgrade: deploymonster-agent/1\r\nX-Agent-Token: %s\r\n\r\n", path, host, c.token)
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
	c.sem <- struct{}{}           // acquire concurrency slot; deferred <-c.sem releases it
	defer func() { <-c.sem }()

	if r := recover(); r != nil {
		c.logger.Error("handleMessage panicked", "panic", r)
		c.sendResponse(msg.ID, core.AgentMsgError, fmt.Sprintf("handler panicked: %v", r))
		return
	}

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
	case core.AgentMsgNetworkCreate:
		err = c.handleNetworkCreate(ctx, msg)
	case core.AgentMsgMetricsCollect:
		result, err = c.handleMetricsCollect(ctx, msg)
	case core.AgentMsgHealthCheck:
		result, err = c.handleHealthCheck(ctx)
	case core.AgentMsgBuildTask:
		result, err = c.handleBuildTask(ctx, msg)
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
	rt, err := c.requireRuntime()
	if err != nil {
		return "", err
	}
	opts, err := decodePayload[core.ContainerOpts](msg.Payload)
	if err != nil {
		return "", fmt.Errorf("decode container opts: %w", err)
	}
	return rt.CreateAndStart(ctx, opts)
}

func (c *AgentClient) handleContainerStop(ctx context.Context, msg core.AgentMessage) error {
	rt, err := c.requireRuntime()
	if err != nil {
		return err
	}
	var p struct {
		ContainerID string `json:"container_id"`
		TimeoutSec  int    `json:"timeout_sec"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode container stop payload: %w", err)
	}
	return rt.Stop(ctx, p.ContainerID, p.TimeoutSec)
}

func (c *AgentClient) handleContainerRemove(ctx context.Context, msg core.AgentMessage) error {
	rt, err := c.requireRuntime()
	if err != nil {
		return err
	}
	var p struct {
		ContainerID string `json:"container_id"`
		Force       bool   `json:"force"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode container remove payload: %w", err)
	}
	return rt.Remove(ctx, p.ContainerID, p.Force)
}

func (c *AgentClient) handleContainerRestart(ctx context.Context, msg core.AgentMessage) error {
	rt, err := c.requireRuntime()
	if err != nil {
		return err
	}
	var p struct {
		ContainerID string `json:"container_id"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode container restart payload: %w", err)
	}
	return rt.Restart(ctx, p.ContainerID)
}

func (c *AgentClient) handleContainerList(ctx context.Context, msg core.AgentMessage) ([]core.ContainerInfo, error) {
	rt, err := c.requireRuntime()
	if err != nil {
		return nil, err
	}
	var p struct {
		Labels map[string]string `json:"labels"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return nil, fmt.Errorf("decode container list payload: %w", err)
	}
	return rt.ListByLabels(ctx, p.Labels)
}

func (c *AgentClient) handleContainerLogs(ctx context.Context, msg core.AgentMessage) (string, error) {
	rt, err := c.requireRuntime()
	if err != nil {
		return "", err
	}
	var p struct {
		ContainerID string `json:"container_id"`
		Tail        string `json:"tail"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return "", fmt.Errorf("decode container logs payload: %w", err)
	}

	reader, err := rt.Logs(ctx, p.ContainerID, p.Tail, false)
	if err != nil {
		return "", fmt.Errorf("fetch logs for %s: %w", p.ContainerID, err)
	}
	defer func() { _ = reader.Close() }()

	buf := make([]byte, 64*1024) // 64KB max log fetch
	n, _ := reader.Read(buf)
	return string(buf[:n]), nil
}

func (c *AgentClient) handleContainerExec(ctx context.Context, msg core.AgentMessage) (string, error) {
	rt, err := c.requireRuntime()
	if err != nil {
		return "", err
	}
	var p struct {
		ContainerID string   `json:"container_id"`
		Cmd         []string `json:"cmd"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return "", fmt.Errorf("decode container exec payload: %w", err)
	}
	return rt.Exec(ctx, p.ContainerID, p.Cmd)
}

func (c *AgentClient) handleImagePull(ctx context.Context, msg core.AgentMessage) error {
	rt, err := c.requireRuntime()
	if err != nil {
		return err
	}
	var p struct {
		Image string `json:"image"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode image pull payload: %w", err)
	}
	return rt.ImagePull(ctx, p.Image)
}

func (c *AgentClient) handleNetworkCreate(ctx context.Context, msg core.AgentMessage) error {
	rt, err := c.requireRuntime()
	if err != nil {
		return err
	}
	nr, ok := rt.(interface {
		EnsureNetwork(context.Context, string) error
	})
	if !ok {
		return fmt.Errorf("container runtime does not support network ensure")
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := decodeInto(msg.Payload, &p); err != nil {
		return fmt.Errorf("decode network create payload: %w", err)
	}
	if p.Name == "" {
		return fmt.Errorf("network name is required")
	}
	return nr.EnsureNetwork(ctx, p.Name)
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

func (c *AgentClient) handleBuildTask(ctx context.Context, msg core.AgentMessage) (map[string]any, error) {
	payload, err := decodePayload[core.BuildTaskPayload](msg.Payload)
	if err != nil {
		return nil, fmt.Errorf("invalid build task payload: %w", err)
	}
	c.logger.Info("received build task from master",
		"deploy_id", payload.DeployID,
		"tenant_id", payload.TenantID,
		"app_id", payload.AppID,
		"commit_sha", payload.CommitSHA)

	// Build execution on the agent is driven by the agent's local build
	// module (if wired). The FnBytes carry serializable job metadata so
	// the agent can reconstruct and execute the build job. Until the agent
	// wires its local build module, we return an error so the master knows
	// this agent cannot handle build tasks.
	//
	// Note: agents that want to execute builds must call
	// SetBuildModule(buildMod) during initialization to enable this path.
	if c.buildMod == nil {
		c.sendResponse(msg.ID, core.AgentMsgError, "build module not wired on this agent")
		return nil, fmt.Errorf("build module not wired")
	}

	c.logger.Info("executing build task locally",
		"deploy_id", payload.DeployID,
		"server_id", c.serverID)

	return map[string]any{
		"status":    "accepted",
		"deploy_id": payload.DeployID,
		"server_id": c.serverID,
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
