package secrets

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Bolt bucket + key used to persist the per-deployment Argon2id salt
// for the vault KDF. Exported constants so tests and ops tooling can
// inspect the stored value without guessing layout.
const (
	VaultBucket  = "vault"
	VaultSaltKey = "salt"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the secret vault.
// Provides AES-256-GCM encrypted secret storage with scoping
// (global → tenant → project → app) and versioning.
// When Core.Services.KeyStore is set, the module delegates key
// operations to the external KMS instead of the built-in Vault.
type Module struct {
	core   *core.Core
	vault  *Vault
	store  core.Store
	bolt   core.BoltStorer
	logger *slog.Logger

	// keyStore is the optional external KMS (AWS KMS, SoftHSM, etc).
	// When set, Encrypt/Decrypt delegate to it instead of vault.
	keyStore core.KeyStore

	// masterSecret is kept so Start can re-derive a new vault after a
	// legacy-salt migration without re-reading config.
	masterSecret string
}

func New() *Module { return &Module{} }

func (m *Module) ID() string                  { return "secrets" }
func (m *Module) Name() string                { return "Secret Vault" }
func (m *Module) Version() string             { return "1.0.0" }
func (m *Module) Dependencies() []string      { return []string{"core.db"} }
func (m *Module) Routes() []core.Route        { return nil }
func (m *Module) Events() []core.EventHandler { return nil }

func (m *Module) Init(ctx context.Context, c *core.Core) error {
	m.core = c
	m.store = c.Store
	m.logger = c.Logger.With("module", m.ID())
	if c.DB != nil {
		m.bolt = c.DB.Bolt
	}
	m.keyStore = c.Services.KeyStore
	if m.keyStore != nil {
		m.logger.Info("external key store wired", "type", fmt.Sprintf("%T", m.keyStore))
	}

	// Create vault with the server's secret key
	secret := c.Config.Server.SecretKey
	if c.Config.Secrets.EncryptionKey != "" {
		secret = c.Config.Secrets.EncryptionKey
	}
	m.masterSecret = secret

	// Resolve the per-deployment salt. The returned usedLegacy flag is
	// true when the bolt store had no persisted salt AND existing
	// secret versions decrypt under the legacy salt — in that case we
	// start up with a legacy-keyed vault and rekey on Start() so the
	// first boot after upgrade migrates transparently.
	salt, usedLegacy, err := m.resolveVaultSalt(ctx)
	if err != nil {
		return fmt.Errorf("resolve vault salt: %w", err)
	}

	if usedLegacy {
		m.vault = newLegacyVault(secret)
		m.logger.Warn("vault booting with legacy salt — migration will run on Start",
			"reason", "no persisted salt found but existing secrets present")
	} else {
		m.vault = NewVaultWithSalt(secret, salt)
	}

	// Register as the secret resolver in Services
	c.Services.Secrets = m

	return nil
}

// resolveVaultSalt loads the per-deployment salt from bolt, generates
// a new one if the store is empty and no legacy secrets exist, or
// signals "use legacy salt and migrate" when existing encrypted data
// predates per-deployment salts.
//
// Returns (salt, usedLegacy, err). When usedLegacy is true, salt is
// the freshly generated salt that the migration should end up with.
func (m *Module) resolveVaultSalt(ctx context.Context) ([]byte, bool, error) {
	// Bolt may be absent in some test fixtures — fall back to the
	// legacy salt so tests that construct a minimal Core still work.
	if m.bolt == nil {
		return LegacyVaultSalt(), false, nil
	}

	var stored string
	if err := m.bolt.Get(VaultBucket, VaultSaltKey, &stored); err == nil && stored != "" {
		decoded, derr := base64.StdEncoding.DecodeString(stored)
		if derr != nil {
			return nil, false, fmt.Errorf("decode stored salt: %w", derr)
		}
		if len(decoded) < 16 {
			return nil, false, fmt.Errorf("stored salt too short (%d bytes)", len(decoded))
		}
		return decoded, false, nil
	}

	// No salt persisted. Decide between fresh install and legacy
	// upgrade by checking whether any secret versions already exist.
	hasLegacySecrets := false
	if m.store != nil {
		if versions, verr := m.store.ListAllSecretVersions(ctx); verr == nil && len(versions) > 0 {
			hasLegacySecrets = true
		}
	}

	newSalt, err := GenerateVaultSalt()
	if err != nil {
		return nil, false, err
	}

	if hasLegacySecrets {
		// Don't persist the new salt yet — Start() will re-encrypt
		// existing secrets first, then persist. This keeps the module
		// idempotent: if the process is killed mid-migration, the
		// next boot still sees the legacy state and retries.
		return newSalt, true, nil
	}

	// Fresh install: persist immediately so subsequent boots skip
	// this branch entirely.
	if err := m.persistSalt(newSalt); err != nil {
		return nil, false, fmt.Errorf("persist new salt: %w", err)
	}
	m.logger.Info("generated new per-deployment vault salt")
	return newSalt, false, nil
}

func (m *Module) persistSalt(salt []byte) error {
	if m.bolt == nil {
		return nil
	}
	return m.bolt.Set(VaultBucket, VaultSaltKey, base64.StdEncoding.EncodeToString(salt), 0)
}

func (m *Module) Start(ctx context.Context) error {
	m.logger.Info("secret vault started")

	// If Init detected a legacy-salt upgrade, perform the re-encrypt
	// migration now. This runs after the DB module has finished its
	// own Start, so the store is definitely ready.
	if m.bolt != nil {
		var stored string
		if err := m.bolt.Get(VaultBucket, VaultSaltKey, &stored); err != nil || stored == "" {
			// Still no persisted salt — we're in the legacy branch.
			if err := m.migrateLegacyVault(ctx); err != nil {
				return fmt.Errorf("vault migration: %w", err)
			}
		}
	}
	return nil
}

// migrateLegacyVault re-encrypts all existing secret versions from
// the legacy hardcoded salt to a freshly generated per-deployment
// salt. Runs only on the first boot after upgrade; subsequent boots
// find the persisted salt and skip this path entirely.
func (m *Module) migrateLegacyVault(ctx context.Context) error {
	newSalt, err := GenerateVaultSalt()
	if err != nil {
		return err
	}
	newVault := NewVaultWithSalt(m.masterSecret, newSalt)
	legacyVault := newLegacyVault(m.masterSecret)

	versions, err := m.store.ListAllSecretVersions(ctx)
	if err != nil {
		return fmt.Errorf("list versions: %w", err)
	}

	rotated := 0
	for _, v := range versions {
		plaintext, err := legacyVault.Decrypt(v.ValueEnc)
		if err != nil {
			return fmt.Errorf("decrypt legacy version %s: %w", v.ID, err)
		}
		newEnc, err := newVault.Encrypt(plaintext)
		if err != nil {
			return fmt.Errorf("re-encrypt version %s: %w", v.ID, err)
		}
		if err := m.store.UpdateSecretVersionValue(ctx, v.ID, newEnc); err != nil {
			return fmt.Errorf("update version %s: %w", v.ID, err)
		}
		rotated++
	}

	// Only persist the salt after every version is successfully
	// re-encrypted. A mid-migration crash leaves the legacy state
	// intact and the next boot retries.
	if err := m.persistSalt(newSalt); err != nil {
		return fmt.Errorf("persist salt: %w", err)
	}
	m.vault = newVault
	m.logger.Info("vault migrated to per-deployment salt", "versions_rotated", rotated)
	return nil
}

func (m *Module) Stop(_ context.Context) error { return nil }
func (m *Module) Health() core.HealthStatus {
	if m.vault == nil {
		return core.HealthDown
	}
	return core.HealthOK
}

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
func (m *Module) Resolve(ctx context.Context, scope, name string) (string, error) {
	if m.store == nil {
		return "", fmt.Errorf("secret %s/%s: not found (store not initialized)", scope, name)
	}
	// ctx is passed from caller — respects their timeout/shutdown
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

// RotateEncryptionKey decrypts all secret versions with the current key and
// re-encrypts them with a new key. Returns the number of versions rotated.
// This is an admin-only offline operation (run during maintenance window).
//
// SECURITY FIX (CRYPTO-002): A fresh per-deployment salt is generated for the
// new vault and persisted after a successful re-encrypt. Previously this used
// NewVault (the legacy hardcoded salt), which silently downgraded at-rest
// encryption back to the pre-migration weakness on every key rotation — two
// installs sharing a master secret would derive the same AES key. The new salt
// is persisted only after every version is re-encrypted, mirroring
// migrateLegacyVault: a mid-rotation crash leaves the prior (salt, ciphertext)
// pair intact so the next boot can still decrypt.
func (m *Module) RotateEncryptionKey(ctx context.Context, newMasterSecret string) (int, error) {
	if m.store == nil {
		return 0, fmt.Errorf("store not initialized")
	}

	newSalt, err := GenerateVaultSalt()
	if err != nil {
		return 0, fmt.Errorf("generate vault salt: %w", err)
	}
	newVault := NewVaultWithSalt(newMasterSecret, newSalt)

	versions, err := m.store.ListAllSecretVersions(ctx)
	if err != nil {
		return 0, fmt.Errorf("list secret versions: %w", err)
	}

	rotated := 0
	for _, v := range versions {
		// Decrypt with old key
		plaintext, err := m.vault.Decrypt(v.ValueEnc)
		if err != nil {
			return rotated, fmt.Errorf("decrypt version %s (secret %s): %w", v.ID, v.SecretID, err)
		}

		// Re-encrypt with new key
		newEnc, err := newVault.Encrypt(plaintext)
		if err != nil {
			return rotated, fmt.Errorf("re-encrypt version %s: %w", v.ID, err)
		}

		// Update in store
		if err := m.store.UpdateSecretVersionValue(ctx, v.ID, newEnc); err != nil {
			return rotated, fmt.Errorf("update version %s: %w", v.ID, err)
		}

		rotated++
	}

	// Persist the new salt only after every version is re-encrypted, so a
	// crash mid-rotation leaves the previous (salt, ciphertext) intact.
	if err := m.persistSalt(newSalt); err != nil {
		return rotated, fmt.Errorf("persist rotated salt: %w", err)
	}

	// Switch to new vault + master secret for runtime so a subsequent
	// legacy-migration check on the next boot stays consistent.
	m.vault = newVault
	m.masterSecret = newMasterSecret

	if m.logger != nil {
		m.logger.Info("encryption key rotated", "versions_rotated", rotated)
	}

	return rotated, nil
}

// ResolveAll implements core.SecretResolver.
// Replaces all ${SECRET:name} references in a template string.
func (m *Module) ResolveAll(ctx context.Context, scope, template string) (string, error) {
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
		value, err := m.Resolve(ctx, scope, ref)
		if err != nil {
			return "", fmt.Errorf("resolve secret %q: %w", ref, err)
		}

		result = result[:idx] + value + result[idx+end+1:]
	}

	return result, nil
}
