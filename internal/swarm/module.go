package swarm

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// leaderLeaseDuration is how long each leadership claim lasts before
// requiring renewal. Renew is called at half this interval to avoid
// accidentally losing leadership between renewals.
const leaderLeaseDuration = 30 * time.Second
const leaderRenewalInterval = leaderLeaseDuration / 2

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

	// Leader election for HA. When LeaderElector is set, the Swarm module
	// only activates when this instance wins the "deploymonster:leader" election.
	// This prevents split-brain in multi-instance deployments.
	elector       core.LeaderElector
	isLeader      bool
	leaderMu      sync.RWMutex
	leaderCancel  context.CancelFunc
	leaderRenewCh chan struct{} // signals to the renewal goroutine
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
	m.elector = c.Services.LeaderElector

	if !c.Config.Swarm.Enabled {
		return nil
	}

	// Determine the join token — use config value or generate one
	token := c.Config.Swarm.JoinToken
	if token == "" {
		token = core.GenerateSecret(32)
		// Only log a prefix for operator reference — never log the full secret
		m.logger.Info("generated agent join token (set swarm.join_token in config to persist)",
			"token_prefix", core.ShortID(token, 8)+"...")
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

func (m *Module) Start(ctx context.Context) error {
	if !m.core.Config.Swarm.Enabled {
		m.logger.Info("swarm mode disabled")
		return nil
	}

	// If a leader elector is available, run leader election.
	// Without it (nil elector), always activate as master — single-instance.
	if m.elector != nil {
		return m.startWithElection(ctx)
	}

	// No elector — activate as master immediately.
	m.isLeader = true
	return m.startMaster()
}

func (m *Module) startWithElection(ctx context.Context) error {
	leaderCtx, cancel := context.WithCancel(context.Background())
	m.leaderCancel = cancel
	m.leaderRenewCh = make(chan struct{}, 1)

	// Attempt election immediately.
	if won, err := m.elector.Elect(ctx, "deploymonster:leader", leaderLeaseDuration); err != nil {
		m.logger.Warn("leader election error", "error", err)
		return nil // don't fail start — this instance just won't be master
	} else if !won {
		m.logger.Info("another instance holds master leadership — swarm inactive")
		return nil
	}

	m.isLeader = true
	m.logger.Info("won master leadership, activating swarm")
	if err := m.startMaster(); err != nil {
		if resignErr := m.elector.Resign(context.Background(), "deploymonster:leader"); resignErr != nil {
			m.logger.Warn("failed to resign swarm leadership after start failure", "error", resignErr)
		}
		return err
	}

	// Start background renewal goroutine.
	go m.leaderRenewLoop(leaderCtx)

	return nil
}

func (m *Module) leaderRenewLoop(ctx context.Context) {
	ticker := time.NewTicker(leaderRenewalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if m.elector == nil {
				return
			}
			held, err := m.elector.Renew(ctx, "deploymonster:leader", leaderLeaseDuration)
			if err != nil {
				m.logger.Warn("leader renewal error", "error", err)
			}
			if !held {
				m.logger.Warn("lost master leadership, shutting down swarm")
				m.isLeader = false
				m.leaderMu.Lock()
				if m.leaderCancel != nil {
					m.leaderCancel()
				}
				m.leaderMu.Unlock()
				if m.agentServer != nil {
					m.agentServer.Stop()
				}
				return
			}
		case <-m.leaderRenewCh:
			// Manual trigger for test/external callers.
		}
	}
}

func (m *Module) startMaster() error {
	if m.agentServer != nil {
		m.agentServer.StartHeartbeat()
	}
	m.logger.Info("swarm orchestrator started (master)",
		"agents", func() int {
			if m.agentServer == nil {
				return 0
			}
			return len(m.agentServer.ConnectedAgents())
		}(),
	)
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	// Resign leadership if we held it (graceful shutdown).
	if m.isLeader && m.elector != nil {
		_ = m.elector.Resign(ctx, "deploymonster:leader")
	}

	if m.leaderCancel != nil {
		m.leaderCancel()
	}

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
