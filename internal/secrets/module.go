package secrets

import (
	"context"
	"database/sql"
	"errors"
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

// buildScopeHierarchy creates the scope fallback chain.
// Order: exact scope -> global (for non-global scopes)
func buildScopeHierarchy(scope string) []string {
	parts := strings.Split(scope, "/")
	if len(parts) < 1 {
		return []string{scope}
	}

	scopeType := parts[0]
	switch scopeType {
	case "global":
		return []string{"global"}
	case "tenant", "project", "app":
		// Try exact scope first, then fall back to global
		if len(parts) >= 2 {
			return []string{scope, "global"}
		}
		return []string{scope, "global"}
	default:
		return []string{scope}
	}
}

// Resolve implements core.SecretResolver.
// Looks up a secret by scope/name and returns the decrypted value.
// Falls back through scope hierarchy: exact scope -> global
func (m *Module) Resolve(scope, name string) (string, error) {
	if m.store == nil {
		return "", fmt.Errorf("secret %s/%s: not found (store not initialized)", scope, name)
	}
	ctx := context.Background()
	scopeHierarchy := buildScopeHierarchy(scope)

	var lastErr error
	for _, tryScope := range scopeHierarchy {
		secret, err := m.store.GetSecretByScopeAndName(ctx, tryScope, name)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
				continue
			}
			lastErr = err
			continue
		}

		if secret == nil {
			continue
		}

		// Get latest version
		version, err := m.store.GetLatestSecretVersion(ctx, secret.ID)
		if err != nil {
			lastErr = err
			continue
		}

		if version == nil {
			lastErr = fmt.Errorf("no versions found for secret %s", secret.ID)
			continue
		}

		// Decrypt value
		value, err := m.vault.Decrypt(version.ValueEnc)
		if err != nil {
			return "", fmt.Errorf("decrypt secret: %w", err)
		}

		if m.logger != nil {
			m.logger.Debug("resolved secret", "scope", tryScope, "name", name, "version", version.Version)
		}
		return value, nil
	}

	if lastErr != nil {
		return "", fmt.Errorf("secret %s/%s: %w", scope, name, lastErr)
	}
	return "", fmt.Errorf("secret %s/%s not found in any scope", scope, name)
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
