package core

import "time"

// PullTimeout for image pulls. Prevents a hanging Docker daemon
// from blocking API requests indefinitely.
const PullTimeout = 5 * time.Minute

// BuildTimeout is the default maximum time allowed for a full
// clone → detect → generate → docker build pipeline.
const BuildTimeout = 30 * time.Minute

// HeartbeatInterval is how often the master pings connected agents.
// Must be significantly shorter than HeartbeatDead so the monitor
// gets a chance to act before the read loop times out.
const HeartbeatInterval = 30 * time.Second

// HeartbeatDead is how long an agent can go without any message
// before the master considers it dead and force-closes the connection.
const HeartbeatDead = 90 * time.Second

// PingTimeout is the per-ping context deadline. Keeps a wedged agent
// from blocking the heartbeat monitor past one interval.
const PingTimeout = 5 * time.Second

// ReadDeadline is the socket read deadline on agent connections.
// Set to 3× HeartbeatInterval so one missed ping is tolerated before
// the read loop times out.
const ReadDeadline = 90 * time.Second

// AgentHandshakeTimeout is the deadline for reading the initial
// AgentInfo message during connection upgrade.
const AgentHandshakeTimeout = 10 * time.Second

// ContainerStopGrace is the additional time added to the caller-
// supplied timeoutSec when stopping a container.
const ContainerStopGrace = 30 * time.Second

// ContainerRemoveTimeout is the timeout for container removal.
const ContainerRemoveTimeout = 30 * time.Second

// ContainerRestartTimeout is the default timeout (in seconds) for
// container restarts when no explicit timeout is provided.
const ContainerRestartTimeout = 10

// AutoRestartCheckInterval is the period between sweeps of the
// crash-detecting ticker in the auto-restart monitor.
const AutoRestartCheckInterval = 30 * time.Second

// AutoRestartRetryDelay is the per-attempt linear backoff base for
// auto-restart retry sleep: delay = attempt * AutoRestartRetryDelay.
const AutoRestartRetryDelay = 5 * time.Second

// BackoffBase is the initial retry backoff for agent reconnection.
// It doubles each attempt up to BackoffMax.
const BackoffBase = time.Second

// BackoffMax is the cap on agent reconnection backoff.
const BackoffMax = 30 * time.Second