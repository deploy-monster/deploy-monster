package auth

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

// TestModule_TOTP mirrors the pattern of TestModule_JWT — before
// initialization the accessor returns nil, and once the field is set
// the accessor returns the same pointer.
func TestModule_TOTP(t *testing.T) {
	m := New()
	if m.TOTP() != nil {
		t.Error("TOTP() should be nil before initialization")
	}

	svc := NewTOTPService(nil)
	m.totp = svc
	if m.TOTP() != svc {
		t.Error("TOTP() should return the configured TOTP service")
	}
}

// TestCleanupBootstrapAdminCredentials_EnvVarsAlwaysCleared verifies the
// pre-file-handling branch: the two MONSTER_ADMIN_* env vars are unset
// regardless of file state, even when the configured env-file path does
// not exist on disk.
func TestCleanupBootstrapAdminCredentials_EnvVarsAlwaysCleared(t *testing.T) {
	withBootstrapEnvFile(t, filepath.Join(t.TempDir(), "missing.env"))

	t.Setenv("MONSTER_ADMIN_EMAIL", "admin@example.com")
	t.Setenv("MONSTER_ADMIN_PASSWORD", "hunter2")

	m := New()
	m.logger = slog.Default()
	m.cleanupBootstrapAdminCredentials()

	if v, ok := os.LookupEnv("MONSTER_ADMIN_EMAIL"); ok {
		t.Fatalf("MONSTER_ADMIN_EMAIL still set: %q", v)
	}
	if v, ok := os.LookupEnv("MONSTER_ADMIN_PASSWORD"); ok {
		t.Fatalf("MONSTER_ADMIN_PASSWORD still set: %q", v)
	}
}

// TestCleanupBootstrapAdminCredentials_FileWithoutMarkerKept covers the
// early-return where the env file exists but contains no
// MONSTER_ADMIN_PASSWORD line — the cleanup must leave the file alone.
func TestCleanupBootstrapAdminCredentials_FileWithoutMarkerKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploymonster.env")
	if err := os.WriteFile(path, []byte("MONSTER_PORT=8443\n"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	withBootstrapEnvFile(t, path)

	m := New()
	m.logger = slog.Default()
	m.cleanupBootstrapAdminCredentials()

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("env file should still exist when no password marker is present: %v", err)
	}
}

// TestCleanupBootstrapAdminCredentials_FileWithMarkerRemoved exercises
// the success path where the password marker is present — the file is
// expected to be deleted.
func TestCleanupBootstrapAdminCredentials_FileWithMarkerRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deploymonster.env")
	contents := "MONSTER_ADMIN_EMAIL=admin@example.com\nMONSTER_ADMIN_PASSWORD=hunter2\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	withBootstrapEnvFile(t, path)

	m := New()
	m.logger = slog.Default()
	m.cleanupBootstrapAdminCredentials()

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected env file to be removed; stat err=%v", err)
	}
}

// withBootstrapEnvFile temporarily redirects the package-level
// bootstrapAdminEnvFile pointer at a caller-supplied path and restores
// the original value when the test ends.
func withBootstrapEnvFile(t *testing.T, path string) {
	t.Helper()
	original := bootstrapAdminEnvFile
	bootstrapAdminEnvFile = path
	t.Cleanup(func() { bootstrapAdminEnvFile = original })
}
