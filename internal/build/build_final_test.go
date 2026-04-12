package build

import (
	"testing"
)

// =============================================================================
// gitClone — token injection into HTTPS URL (line 146-148)
// =============================================================================

func TestInjectToken_HTTPS(t *testing.T) {
	tests := []struct {
		url   string
		token string
		want  string
	}{
		{
			url:   "https://github.com/user/repo.git",
			token: "ghp_abc123",
			want:  "https://ghp_abc123@github.com/user/repo.git",
		},
		{
			url:   "git@github.com:user/repo.git",
			token: "ghp_abc123",
			want:  "git@github.com:user/repo.git", // SSH URL, no injection
		},
		{
			url:   "http://example.com/repo.git",
			token: "tok123",
			want:  "http://example.com/repo.git", // Only HTTPS, not HTTP
		},
		{
			url:   "https://gitlab.com/group/project.git",
			token: "glpat-xyz",
			want:  "https://glpat-xyz@gitlab.com/group/project.git",
		},
	}

	for _, tt := range tests {
		got := injectToken(tt.url, tt.token)
		if got != tt.want {
			t.Errorf("injectToken(%q, %q) = %q, want %q", tt.url, tt.token, got, tt.want)
		}
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

func TestInjectToken_Short(t *testing.T) {
	// URL shorter than 8 chars — no injection
	got := injectToken("short", "tok")
	if got != "short" {
		t.Errorf("injectToken short URL = %q, want %q", got, "short")
	}
}

func TestFinal_InjectToken_EmptyURL(t *testing.T) {
	got := injectToken("", "tok")
	if got != "" {
		t.Errorf("injectToken empty URL = %q, want empty", got)
	}
}
