package auth

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
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

// TestCleanupBootstrapAdminCredentials_ReadErrorIsNotNotExist covers the
// rare branch where ReadFile fails for a reason other than IsNotExist —
// the easiest provocation is pointing the env-file path at a directory,
// which on every supported OS surfaces as "is a directory" rather than
// "no such file or directory". The function must Warn-and-return without
// panicking on the nil-data path that follows.
func TestCleanupBootstrapAdminCredentials_ReadErrorIsNotNotExist(t *testing.T) {
	dir := t.TempDir()
	// Path is a directory, not a file — ReadFile errors with
	// syscall.EISDIR (or platform equivalent) which is not IsNotExist.
	withBootstrapEnvFile(t, dir)

	m := New()
	m.logger = slog.Default()
	// Must not panic; function emits a Warn and returns.
	m.cleanupBootstrapAdminCredentials()

	// Sanity check: directory still exists (cleanup did not try to
	// remove a directory).
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("expected dir to still exist; stat err=%v", err)
	}
}

// TestCleanupBootstrapAdminCredentials_RemoveFailureWarns provokes the
// post-marker remove-failure branch by putting the env file inside a
// read-only parent directory so os.Remove returns EACCES. Skipped on
// Windows where 0o500 on directories does not block deletion the same
// way it does on POSIX.
func TestCleanupBootstrapAdminCredentials_RemoveFailureWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("read-only parent dir does not block file removal on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root user bypasses the read-only directory protection")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "deploymonster.env")
	contents := "MONSTER_ADMIN_PASSWORD=hunter2\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	// Make parent dir read+exec only so unlink fails with EACCES.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod parent: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) // restore so t.TempDir cleanup can run
	withBootstrapEnvFile(t, path)

	m := New()
	m.logger = slog.Default()
	m.cleanupBootstrapAdminCredentials()

	// File must still exist because Remove failed.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should still exist after blocked remove; stat err=%v", err)
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
