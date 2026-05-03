package build

import "testing"

// =============================================================================
// setupGitAskpass — token authentication stays out of clone URL
// =============================================================================

func TestSetupGitAskpass_HTTPS(t *testing.T) {
	if _, cleanup, err := setupGitAskpass(t.TempDir(), "https://github.com/user/repo.git", "ghp_abc123"); err != nil {
		t.Fatalf("setupGitAskpass returned error: %v", err)
	} else {
		cleanup()
	}
}

// =============================================================================
// exists() — file not found
// =============================================================================

func TestExists_NotFound(t *testing.T) {
	if exists("/nonexistent/path/to/file") {
		t.Error("expected false for non-existent file")
	}
}

// =============================================================================
// Build — commit SHA fallback (line 86-88)
// Note: We can't test the full Build pipeline easily without Docker,
// but we can test the helper functions it calls.
// =============================================================================

func TestSetupGitAskpass_ShortURL(t *testing.T) {
	if _, _, err := setupGitAskpass(t.TempDir(), "short", "tok"); err == nil {
		t.Fatal("expected malformed URL error")
	}
}

func TestSetupGitAskpass_EmptyURL(t *testing.T) {
	if _, _, err := setupGitAskpass(t.TempDir(), "", "tok"); err == nil {
		t.Fatal("expected empty URL error")
	}
}
