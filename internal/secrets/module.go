package secrets

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the secret vault.
// Provides AES-256-GCM encrypted secret storage with scoping
// (global → tenant → project → app) and versioning.
type Module struct {
	core   *core.Core
	vault  *Vault
	store  core.Store
	logger *slog.Logger
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "secrets" }
func (m *Module) Name() string                { return "Secret Vault" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())

	// Create vault with the server's secret key
	secret := c.Config.Server.SecretKey
	if c.Config.Secrets.EncryptionKey != "" {
		secret = c.Config.Secrets.EncryptionKey
	}
	m.vault = NewVault(secret)

	// Register as the secret resolver in Services
	c.Services.Secrets = m

	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("secret vault started")
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus    { return core.HealthOK }

// Vault returns the encryption vault.
func (m *Module) Vault() *Vault { return m.vault }

// Resolve implements core.SecretResolver.
// Looks up a secret by scope/name and returns the decrypted value.
func (m *Module) Resolve(scope, name string) (string, error) {
	// Placeholder — full implementation queries secret_versions table
	return "", fmt.Errorf("secret %s/%s not found", scope, name)
}

// ResolveAll implements core.SecretResolver.
// Replaces all ${SECRET:name} references in a template string.
func (m *Module) ResolveAll(scope, template string) (string, error) {
	result := template

	for {
		idx := strings.Index(result, "${SECRET:")
		if idx < 0 {
			break
		}

		end := strings.Index(result[idx:], "}")
		if end < 0 {
			break
		}

		ref := result[idx+9 : idx+end] // Extract name from ${SECRET:name}
		value, err := m.Resolve(scope, ref)
		if err != nil {
			return "", fmt.Errorf("resolve secret %q: %w", ref, err)
		}

		result = result[:idx] + value + result[idx+end+1:]
	}

	return result, nil
}
