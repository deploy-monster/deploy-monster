package secrets

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Module Init — uses server secret key
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Init_WithServerSecretKey(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger:   slog.Default(),
		Services: core.NewServices(),
		Config: &core.Config{
			Server:  core.ServerConfig{SecretKey: "server-master-key"},
			Secrets: core.SecretsConfig{EncryptionKey: ""},
		},
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.vault == nil {
		t.Fatal("vault should be initialized after Init")
	}
	if m.Vault() == nil {
		t.Fatal("Vault() should return non-nil after Init")
	}
	if m.store != c.Store {
		t.Error("store should be set from core.Store")
	}
	if m.logger == nil {
		t.Error("logger should be set")
	}
	if m.core != c {
		t.Error("core reference should be set")
	}

	// Verify vault works with the server key
	enc, err := m.vault.Encrypt("test-value")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	dec, err := m.vault.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "test-value" {
		t.Errorf("expected 'test-value', got %q", dec)
	}
}

func TestModule_Init_WithEncryptionKey(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger:   slog.Default(),
		Services: core.NewServices(),
		Config: &core.Config{
			Server:  core.ServerConfig{SecretKey: "server-key"},
			Secrets: core.SecretsConfig{EncryptionKey: "custom-encryption-key"},
		},
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.vault == nil {
		t.Fatal("vault should be initialized")
	}

	// Vault should use the custom encryption key, not the server key
	vaultWithServerKey := NewVault("server-key")
	vaultWithCustomKey := NewVault("custom-encryption-key")

	enc, _ := m.vault.Encrypt("check-key")

	// Should fail with server key
	_, err := vaultWithServerKey.Decrypt(enc)
	if err == nil {
		t.Error("decrypting with server key should fail when custom key is used")
	}

	// Should succeed with custom key
	dec, err := vaultWithCustomKey.Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt with custom key: %v", err)
	}
	if dec != "check-key" {
		t.Errorf("expected 'check-key', got %q", dec)
	}
}

func TestModule_Init_RegistersAsSecretResolver(t *testing.T) {
	m := New()

	services := core.NewServices()
	c := &core.Core{
		Logger:   slog.Default(),
		Services: services,
		Config: &core.Config{
			Server:  core.ServerConfig{SecretKey: "test-key"},
			Secrets: core.SecretsConfig{},
		},
	}

	m.Init(context.Background(), c)

	if c.Services.Secrets == nil {
		t.Error("expected Services.Secrets to be set after Init")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module Start — logs and returns nil
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Start_Success(t *testing.T) {
	m := New()
	m.logger = slog.Default()

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
}

func TestModule_Start_AfterInit(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger:   slog.Default(),
		Services: core.NewServices(),
		Config: &core.Config{
			Server:  core.ServerConfig{SecretKey: "test-key"},
			Secrets: core.SecretsConfig{},
		},
	}

	m.Init(context.Background(), c)

	err := m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start after Init: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Full lifecycle: Init → Start → use → Stop
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_FullLifecycle(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger:   slog.Default(),
		Services: core.NewServices(),
		Config: &core.Config{
			Server:  core.ServerConfig{SecretKey: "lifecycle-key"},
			Secrets: core.SecretsConfig{},
		},
	}

	// Init
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Start
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Use vault
	enc, err := m.Vault().Encrypt("lifecycle-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	dec, err := m.Vault().Decrypt(enc)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if dec != "lifecycle-secret" {
		t.Errorf("expected 'lifecycle-secret', got %q", dec)
	}

	// Resolve (stub — returns error)
	_, err = m.Resolve("global", "some-secret")
	if err == nil {
		t.Error("Resolve stub should return error")
	}

	// Health check
	if m.Health() != core.HealthOK {
		t.Errorf("expected HealthOK, got %v", m.Health())
	}

	// Stop
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ResolveAll — edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_ResolveAll_AdjacentSecrets(t *testing.T) {
	m := New()
	// Two secret references back-to-back: the first will fail
	_, err := m.ResolveAll("global", "${SECRET:a}${SECRET:b}")
	if err == nil {
		t.Fatal("expected error from Resolve stub")
	}
	if !strings.Contains(err.Error(), "a") {
		t.Errorf("error should mention first secret 'a', got: %v", err)
	}
}

func TestModule_ResolveAll_OnlySecretRef(t *testing.T) {
	m := New()
	_, err := m.ResolveAll("tenant", "${SECRET:only}")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "only") {
		t.Errorf("error should mention 'only', got: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Vault — encrypt error paths (unreachable under normal conditions but
// we verify the cipher and GCM construction paths are fully covered)
// ═══════════════════════════════════════════════════════════════════════════════

func TestVault_EncryptDecrypt_SpecialChars(t *testing.T) {
	vault := NewVault("special-chars-key")

	tests := []struct {
		name      string
		plaintext string
	}{
		{"unicode", "emoji: and CJK: 中文日本語"},
		{"newlines", "line1\nline2\nline3\r\n"},
		{"null bytes", "before\x00after"},
		{"json", `{"key":"value","nested":{"a":1}}`},
		{"url", "https://example.com/path?key=val&other=123#fragment"},
		{"multiline yaml", "server:\n  host: 0.0.0.0\n  port: 8080\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			enc, err := vault.Encrypt(tt.plaintext)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}

			dec, err := vault.Decrypt(enc)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if dec != tt.plaintext {
				t.Errorf("round-trip mismatch")
			}
		})
	}
}

func TestVault_Decrypt_EmptyBase64(t *testing.T) {
	vault := NewVault("empty-test")

	_, err := vault.Decrypt("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}
