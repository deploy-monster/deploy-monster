package mcp

import (
	"context"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the Model Context Protocol (MCP) server.
// Enables AI/LLM-driven infrastructure management via structured tool calls.
type Module struct {
	core   *core.Core
	store  core.Store
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "mcp" }
func (m *Module) Name() string                { return "MCP Server" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db", "deploy"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())
	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("MCP server started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }
