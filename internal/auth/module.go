package auth

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the authentication module.
type Module struct {
	core   *core.Core
	jwt    *JWTService
	totp   *TOTPService
	store  core.Store
	logger *slog.Logger
}

// New creates a new authentication module.
func New() *Module {
	return &Module{}
}

func (m *Module) ID() string                  { return "core.auth" }
func (m *Module) Name() string                { return "Authentication" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(ctx context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())

	// Get the Store interface from core
	if c.Store == nil {
		return core.NewAppError(500, "database store not available", nil)
	}
	m.store = c.Store

	// Create JWT service with key rotation support
	jwtSvc, err := NewJWTService(c.Config.Server.SecretKey, c.Config.Server.PreviousSecretKeys...)
	if err != nil {
		return fmt.Errorf("init jwt service: %w", err)
	}
	m.jwt = jwtSvc

	// Create TOTP service for MFA support
	m.totp = NewTOTPService(m.store)

	// Set the secrets vault on TOTP service if available
	if c.Registry != nil {
		if secretsMod := c.Registry.Get("secrets"); secretsMod != nil {
			type vaultProvider interface {
				Vault() interface {
					Encrypt(string) (string, error)
					Decrypt(string) (string, error)
				}
			}
			if vp, ok := secretsMod.(vaultProvider); ok {
				m.totp.SetVault(vp.Vault())
				m.logger.Info("TOTP vault configured")
			}
		}
	}

	// First-run setup: create super admin if no users exist
	if err := m.firstRunSetup(ctx); err != nil {
		return fmt.Errorf("first-run setup: %w", err)
	}

	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.logger.Info("authentication module started")
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	return nil
}

func (m *Module) Health() core.HealthStatus {
	if m.jwt == nil {
		return core.HealthDown
	}
	return core.HealthOK
}

func (m *Module) Routes() []core.Route {
	return nil // Routes are registered via the API module
}

// JWT returns the JWT service for use by other modules.
func (m *Module) JWT() *JWTService {
	return m.jwt
}

// Store returns the data store for auth-related queries.
func (m *Module) Store() core.Store {
	return m.store
}

// TOTP returns the TOTP service for MFA support.
func (m *Module) TOTP() *TOTPService {
	return m.totp
}

// firstRunSetup creates a super admin user and default tenant if no users exist.
func (m *Module) firstRunSetup(ctx context.Context) error {
	count, err := m.store.CountUsers(ctx)
	if err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	m.logger.Info("first run detected, creating super admin")

	email := os.Getenv("MONSTER_ADMIN_EMAIL")
	password := os.Getenv("MONSTER_ADMIN_PASSWORD")

	if email == "" {
		return fmt.Errorf("MONSTER_ADMIN_EMAIL environment variable is required for first-run setup")
	}
	if password == "" {
		return fmt.Errorf("MONSTER_ADMIN_PASSWORD environment variable is required for first-run setup")
	}

	hash, err := HashPassword(password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Create default tenant
	tenantID, err2 := m.store.CreateTenantWithDefaults(ctx, "Platform", "platform")
	if err2 != nil {
		return fmt.Errorf("create default tenant: %w", err2)
	}

	// Create super admin user
	if _, err3 := m.store.CreateUserWithMembership(ctx, email, hash, "Super Admin", "active", tenantID, "role_super_admin"); err3 != nil {
		return fmt.Errorf("create super admin: %w", err3)
	}

	m.logger.Info("super admin created", "email", email)

	return nil
}
