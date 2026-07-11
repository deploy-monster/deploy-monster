package build

import "testing"

// ---------------------------------------------------------------------------
// ValidateGitURL — uncovered branches
// NOTE: ValidateGitURL is a package-level function, not a Builder method.
// ---------------------------------------------------------------------------

func TestValidateGitURL_Malformed(t *testing.T) {
	err := ValidateGitURL("://invalid-url")
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
}

func TestValidateGitURL_SchemeGit(t *testing.T) {
	err := ValidateGitURL("git://github.com/org/repo.git")
	if err == nil {
		t.Fatal("expected error for git:// scheme")
	}
}

func TestValidateGitURL_SchemeHTTP(t *testing.T) {
	err := ValidateGitURL("http://github.com/org/repo.git")
	if err == nil {
		t.Fatal("expected error for http:// scheme")
	}
}

func TestValidateGitURL_SchemeFile(t *testing.T) {
	err := ValidateGitURL("file:///etc/passwd")
	if err == nil {
		t.Fatal("expected error for file:// scheme")
	}
}

func TestValidateGitURL_SchemeUnknown(t *testing.T) {
	err := ValidateGitURL("ftp://github.com/org/repo.git")
	if err == nil {
		t.Fatal("expected error for unknown scheme")
	}
}

func TestValidateGitURL_NoHost(t *testing.T) {
	err := ValidateGitURL("https://")
	if err == nil {
		t.Fatal("expected error for URL with no host")
	}
}

func TestValidateGitURL_SSHShorthand(t *testing.T) {
	err := ValidateGitURL("git@github.com:org/repo.git")
	if err != nil {
		t.Fatalf("unexpected error for SSH shorthand: %v", err)
	}
}

func TestValidateGitURL_HTTPS(t *testing.T) {
	err := ValidateGitURL("https://github.com/org/repo.git")
	if err != nil {
		t.Fatalf("unexpected error for HTTPS URL: %v", err)
	}
}

func TestValidateGitURL_Empty(t *testing.T) {
	err := ValidateGitURL("")
	if err == nil {
		t.Fatal("expected error for empty URL")
	}
}

// ---------------------------------------------------------------------------
// resolveDockerfilePath — uncovered branches
// ---------------------------------------------------------------------------

func TestResolveDockerfilePath_AbsoluteBase(t *testing.T) {
	_, err := resolveDockerfilePath("Dockerfile", "/abs/path")
	if err == nil {
		t.Fatal("expected error for absolute base path")
	}
}

// ---------------------------------------------------------------------------
// redactURL — edge cases
// ---------------------------------------------------------------------------

func TestRedactURL_Short(t *testing.T) {
	result := redactURL("ab")
	if result != "ab" {
		t.Errorf("got %q, want %q", result, "ab")
	}
}

func TestRedactURL_NoCredentialsShort(t *testing.T) {
	result := redactURL("https://github.com/org/repo.git")
	if result != "https://github.com/org/repo.git" {
		t.Errorf("got %q, want unchanged", result)
	}
}

// ---------------------------------------------------------------------------
// isAbsPath
// ---------------------------------------------------------------------------

func TestIsAbsPath(t *testing.T) {
	if !isAbsPath("/absolute") {
		t.Error("/absolute should be absolute")
	}
	if isAbsPath("relative") {
		t.Error("relative should not be absolute")
	}
}

// ---------------------------------------------------------------------------
// validateBuildArg
// ---------------------------------------------------------------------------

func TestValidateBuildArg_EmptyKey(t *testing.T) {
	err := validateBuildArg("", "")
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

func TestValidateBuildArg_InvalidKey(t *testing.T) {
	err := validateBuildArg("123bad", "")
	if err == nil {
		t.Fatal("expected error for invalid key format")
	}
}

func TestValidateBuildArg_Valid(t *testing.T) {
	err := validateBuildArg("KEY", "value")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateBuildArg_ValueWithControlChar(t *testing.T) {
	err := validateBuildArg("KEY", "val\nue")
	if err == nil {
		t.Fatal("expected error for value with control characters")
	}
}

func TestValidateBuildArg_ValueFlagInjection(t *testing.T) {
	err := validateBuildArg("KEY", "--dangerous")
	if err == nil {
		t.Fatal("expected error for value starting with -")
	}
}

// ---------------------------------------------------------------------------
// validateDockerImageTag
// ---------------------------------------------------------------------------

func TestValidateDockerImageTag_Empty(t *testing.T) {
	err := validateDockerImageTag("")
	if err == nil {
		t.Fatal("expected error for empty tag")
	}
}

func TestValidateDockerImageTag_Valid(t *testing.T) {
	err := validateDockerImageTag("myapp:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// NOTE: ValidateGitURL's isPrivateOrBlockedIP branch is hard to test
// without a DNS resolver that returns private IPs. The remaining
// uncovered build paths (gitClone, dockerBuild, Build pipeline) all
// require filesystem/git/Docker operations — integration test territory.
//
// The init() closure body (module.go:12) is only called by
// core.RegisterModule during module registration, not during unit tests.
