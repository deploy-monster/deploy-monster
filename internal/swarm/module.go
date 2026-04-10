package swarm

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module manages master/agent communication and cluster operations.
// On the master side it runs an AgentServer that accepts agent connections.
// It also provides the NodeManager interface so other modules can execute
// operations on any connected server (local or remote).
type Module struct {
	core        *core.Core
	logger      *slog.Logger
	agentServer *AgentServer
}

func New() *Module { return &Module{} }

func (m *Module) ID() string             { return "swarm" }
func (m *Module) Name() string           { return "Swarm Orchestrator" }
func (m *Module) Version() string        { return "1.0.0" }
func (m *Module) Dependencies() []string { return []string{"deploy"} }

func (m *Module) Routes() []core.Route { return nil }

func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())

	if !c.Config.Swarm.Enabled {
		return nil
	}

	// Determine the join token — use config value or generate one
	token := c.Config.Swarm.JoinToken
	if token == "" {
		token = core.GenerateSecret(32)
		m.logger.Info("generated agent join token (set swarm.join_token in config to persist)")
	}

	// Create the agent server
	m.agentServer = NewAgentServer(c.Events, token, m.logger)

	// Set up local executor if container runtime is available
	localID := "local"
	if c.Config.Swarm.ManagerIP != "" {
		localID = c.Config.Swarm.ManagerIP
	}
	if c.Services.Container != nil {
		local := NewLocalExecutor(c.Services.Container, localID)
		m.agentServer.SetLocal(local)
	}

	// Register the WebSocket handler on the main router
	c.Router.HandleFunc("GET /api/v1/agent/ws", func(w http.ResponseWriter, r *http.Request) {
		m.agentServer.HandleConnect(w, r)
	})

	m.logger.Info("agent WebSocket endpoint registered", "path", "/api/v1/agent/ws")

	return nil
}

func (m *Module) Start(_ context.Context) error {
	if !m.core.Config.Swarm.Enabled {
		m.logger.Info("swarm mode disabled")
		return nil
	}

	// Start the master-side heartbeat monitor so dead agents are cleaned up
	// deterministically instead of waiting for the 90s read deadline.
	if m.agentServer != nil {
		m.agentServer.StartHeartbeat()
	}

	m.logger.Info("swarm orchestrator started",
		"agents", len(m.agentServer.ConnectedAgents()),
	)
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	if m.agentServer != nil {
		m.agentServer.Stop()
	}
	return nil
}

func (m *Module) Health() core.HealthStatus {
	if !m.core.Config.Swarm.Enabled {
		return core.HealthOK
	}
	if m.agentServer == nil {
		return core.HealthDegraded
	}
	return core.HealthOK
}

// AgentServer returns the agent server instance for use by other modules.
func (m *Module) AgentServer() *AgentServer {
	return m.agentServer
}
