package core

import (
	"context"
	"io"
	"time"
)

// RunMode defines whether this instance is master or agent.
type RunMode string

const (
	RunModeMaster RunMode = "master"
	RunModeAgent  RunMode = "agent"
)

// =====================================================
// AGENT PROTOCOL
// Defines the communication contract between master
// and agent nodes. The master orchestrates, agents execute.
// Transport: WebSocket (JSON messages)
// =====================================================

// AgentMessage is the envelope for all master<->agent communication.
type AgentMessage struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	ServerID  string    `json:"server_id"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

// Agent message types (master -> agent)
const (
	AgentMsgPing             = "ping"
	AgentMsgContainerCreate  = "container.create"
	AgentMsgContainerStop    = "container.stop"
	AgentMsgContainerRemove  = "container.remove"
	AgentMsgContainerRestart = "container.restart"
	AgentMsgContainerList    = "container.list"
	AgentMsgContainerLogs    = "container.logs"
	AgentMsgContainerExec    = "container.exec"
	AgentMsgImagePull        = "image.pull"
	AgentMsgNetworkCreate    = "network.create"
	AgentMsgVolumeCreate     = "volume.create"
	AgentMsgMetricsCollect   = "metrics.collect"
	AgentMsgHealthCheck      = "health.check"
	AgentMsgConfigUpdate     = "config.update"
)

// Agent message types (agent -> master)
const (
	AgentMsgPong           = "pong"
	AgentMsgResult         = "result"
	AgentMsgError          = "error"
	AgentMsgMetricsReport  = "metrics.report"
	AgentMsgContainerEvent = "container.event"
	AgentMsgServerStatus   = "server.status"
	AgentMsgLogStream      = "log.stream"
)

// AgentInfo is reported by agents on connection.
type AgentInfo struct {
	ServerID      string `json:"server_id"`
	Hostname      string `json:"hostname"`
	IPAddress     string `json:"ip_address"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	DockerVersion string `json:"docker_version"`
	AgentVersion  string `json:"agent_version"`
	CPUCores      int    `json:"cpu_cores"`
	RAMMB         int64  `json:"ram_mb"`
	DiskMB        int64  `json:"disk_mb"`
}

// ServerMetrics is the periodic metrics report from an agent.
type ServerMetrics struct {
	ServerID    string     `json:"server_id"`
	Timestamp   time.Time  `json:"timestamp"`
	CPUPercent  float64    `json:"cpu_percent"`
	RAMUsedMB   int64      `json:"ram_used_mb"`
	RAMTotalMB  int64      `json:"ram_total_mb"`
	DiskUsedMB  int64      `json:"disk_used_mb"`
	DiskTotalMB int64      `json:"disk_total_mb"`
	NetworkRxMB int64      `json:"network_rx_mb"`
	NetworkTxMB int64      `json:"network_tx_mb"`
	LoadAvg     [3]float64 `json:"load_avg"`
	Containers  int        `json:"containers"`
}

// ContainerMetrics is per-container metrics from an agent.
type ContainerMetrics struct {
	ContainerID string    `json:"container_id"`
	AppID       string    `json:"app_id"`
	Timestamp   time.Time `json:"timestamp"`
	CPUPercent  float64   `json:"cpu_percent"`
	RAMUsedMB   int64     `json:"ram_used_mb"`
	RAMLimitMB  int64     `json:"ram_limit_mb"`
	NetworkRxMB int64     `json:"network_rx_mb"`
	NetworkTxMB int64     `json:"network_tx_mb"`
	PIDs        int       `json:"pids"`
}

// =====================================================
// REMOTE NODE INTERFACE
// Abstracts local vs remote execution. Master uses local
// Docker directly; for remote servers it proxies through
// the agent WebSocket connection.
// =====================================================

// NodeExecutor executes operations on a specific server node.
// On master (local): wraps Docker SDK directly
// On remote: proxies commands through agent WebSocket
type NodeExecutor interface {
	// Identity
	ServerID() string
	IsLocal() bool

	// Container operations (mirrors ContainerRuntime)
	CreateAndStart(ctx context.Context, opts ContainerOpts) (string, error)
	Stop(ctx context.Context, containerID string, timeoutSec int) error
	Remove(ctx context.Context, containerID string, force bool) error
	Restart(ctx context.Context, containerID string) error
	Logs(ctx context.Context, containerID string, tail string, follow bool) (io.ReadCloser, error)
	ListByLabels(ctx context.Context, labels map[string]string) ([]ContainerInfo, error)

	// Server operations
	Exec(ctx context.Context, command string) (string, error)
	Metrics(ctx context.Context) (*ServerMetrics, error)
	Ping(ctx context.Context) error
}

// NodeManager manages connections to all server nodes (local + remote agents).
type NodeManager interface {
	// Get returns the executor for a specific server.
	// Returns local executor for the master server, remote for agents.
	Get(serverID string) (NodeExecutor, error)

	// Local returns the local node executor (master's own Docker).
	Local() NodeExecutor

	// All returns all connected node IDs.
	All() []string

	// OnConnect registers a callback for when an agent connects.
	OnConnect(fn func(info AgentInfo))

	// OnDisconnect registers a callback for when an agent disconnects.
	OnDisconnect(fn func(serverID string))
}
