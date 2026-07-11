package build

import (
	"testing"
)

// dockerAuthEnv error paths (ensure unique names by prefixing with "Edge")

func TestEdge_DockerAuthEnv_NoCreds(t *testing.T) {
	env, cleanup, err := dockerAuthEnv("nginx:latest", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleanup()
	if env != nil {
		t.Errorf("expected nil env, got %v", env)
	}
}

func TestEdge_DockerAuthEnv_PartialCreds(t *testing.T) {
	_, _, err := dockerAuthEnv("nginx:latest", "user", "")
	if err == nil {
		t.Fatal("expected error for partial credentials")
	}
}

func TestEdge_DockerAuthEnv_NoRegistryHost(t *testing.T) {
	_, _, err := dockerAuthEnv("nginx", "user", "pass")
	if err == nil {
		t.Fatal("expected error for no registry host")
	}
}

// resolveDockerfilePath edge cases

func TestEdge_ResolveDockerfilePath_NullByte(t *testing.T) {
	_, err := resolveDockerfilePath("/tmp", "Dockerfile\x00")
	if err == nil {
		t.Fatal("expected error for null byte")
	}
}

func TestEdge_ResolveDockerfilePath_AbsolutePath(t *testing.T) {
	_, err := resolveDockerfilePath("/tmp", "/etc/Dockerfile")
	if err == nil {
		t.Fatal("expected error for absolute path")
	}
}

func TestEdge_ResolveDockerfilePath_EscapesContext(t *testing.T) {
	_, err := resolveDockerfilePath("/tmp/build", "../etc/Dockerfile")
	if err == nil {
		t.Fatal("expected error for path escaping")
	}
}

func TestEdge_ResolveDockerfilePath_NormalizedEscapes(t *testing.T) {
	_, err := resolveDockerfilePath("/tmp/build", "sub/../../../etc/Dockerfile")
	if err == nil {
		t.Fatal("expected error for normalized escape")
	}
}

func TestEdge_ResolveDockerfilePath_ValidRelative(t *testing.T) {
	path, err := resolveDockerfilePath("/tmp/build", "sub/Dockerfile")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != "/tmp/build/sub/Dockerfile" {
		t.Errorf("expected /tmp/build/sub/Dockerfile, got %s", path)
	}
}

// validateBuildArg edge cases (unique names)

func TestEdge_ValidateBuildArg_ControlChars(t *testing.T) {
	err := validateBuildArg("MY_ARG", "value\x00null")
	if err == nil {
		t.Fatal("expected error for control chars")
	}
}

func TestEdge_ValidateBuildArg_DashPrefixValue(t *testing.T) {
	err := validateBuildArg("MY_ARG", "-flag")
	if err == nil {
		t.Fatal("expected error for dash-prefixed value")
	}
}

// validateDockerImageTag edge cases

func TestEdge_ValidateDockerImageTag_InvalidChars(t *testing.T) {
	err := validateDockerImageTag("image:tag$pecial")
	if err == nil {
		t.Fatal("expected error for invalid chars")
	}
}

func TestEdge_ValidateDockerImageTag_Valid(t *testing.T) {
	err := validateDockerImageTag("nginx:1.21")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// isPrivateOrBlockedIP edge cases

func TestEdge_IsPrivateOrBlockedIP_NonIP(t *testing.T) {
	if isPrivateOrBlockedIP("hostname") {
		t.Error("expected false for non-IP")
	}
}

func TestEdge_IsPrivateOrBlockedIP_Private10(t *testing.T) {
	if !isPrivateOrBlockedIP("10.0.0.1") {
		t.Error("expected true for 10.x.x.x")
	}
}

func TestEdge_IsPrivateOrBlockedIP_Loopback(t *testing.T) {
	if !isPrivateOrBlockedIP("127.0.0.1") {
		t.Error("expected true for loopback")
	}
}

func TestEdge_IsPrivateOrBlockedIP_Public(t *testing.T) {
	if isPrivateOrBlockedIP("8.8.8.8") {
		t.Error("expected false for public IP")
	}
}

// registryHostFromImage edge cases

func TestEdge_RegistryHostFromImage_NoSlash(t *testing.T) {
	if h := registryHostFromImage("nginx"); h != "" {
		t.Errorf("expected empty, got %s", h)
	}
}

func TestEdge_RegistryHostFromImage_LibraryName(t *testing.T) {
	if h := registryHostFromImage("library/nginx"); h != "" {
		t.Errorf("expected empty, got %s", h)
	}
}

func TestEdge_RegistryHostFromImage_WithRegistry(t *testing.T) {
	if h := registryHostFromImage("reg.example.com/app:v1"); h != "reg.example.com" {
		t.Errorf("expected reg.example.com, got %s", h)
	}
}

func TestEdge_RegistryHostFromImage_Localhost(t *testing.T) {
	if h := registryHostFromImage("localhost:5000/app:v1"); h != "localhost:5000" {
		t.Errorf("expected localhost:5000, got %s", h)
	}
}

// ValidateGitURL additions (unique names)

func TestEdge_ValidateGitURL_DashPrefix(t *testing.T) {
	err := ValidateGitURL("-arg")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestEdge_ValidateGitURL_LocalPathDisabled(t *testing.T) {
	t.Setenv("MONSTER_ALLOW_LOCAL_GIT_PATHS", "")
	err := ValidateGitURL("/tmp/repo")
	if err == nil {
		t.Fatal("expected error for local path when disabled")
	}
}

func TestEdge_ValidateGitURL_LocalPathEnabled(t *testing.T) {
	t.Setenv("MONSTER_ALLOW_LOCAL_GIT_PATHS", "true")
	err := ValidateGitURL("/tmp/repo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEdge_ValidateGitURL_HTTPSValid(t *testing.T) {
	err := ValidateGitURL("https://github.com/org/repo.git")
	if err != nil {
		t.Fatalf("expected nil for valid HTTPS, got %v", err)
	}
}

func TestEdge_ValidateGitURL_DockerRef(t *testing.T) {
	err := ValidateGitURL("nginx:latest")
	if err != nil {
		t.Fatalf("expected nil for docker ref, got %v", err)
	}
}

// isAbsPath edge cases

func TestEdge_IsAbsPath_UnixAbsolute(t *testing.T) {
	if !isAbsPath("/tmp/repo") {
		t.Error("expected true for unix absolute path")
	}
}

func TestEdge_IsAbsPath_RelativePath(t *testing.T) {
	if isAbsPath("relative/path") {
		t.Error("expected false for relative path")
	}
}

// redactingWriter edge cases

func TestEdge_RedactingWriter_NoSecrets(t *testing.T) {
	var buf testBufferCapture
	w := redactingWriter{dst: &buf}
	n, err := w.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 5 {
		t.Errorf("expected 5, got %d", n)
	}
	if buf.String() != "hello" {
		t.Errorf("expected 'hello', got '%s'", buf.String())
	}
}

func TestEdge_RedactingWriter_WithSecrets(t *testing.T) {
	var buf testBufferCapture
	w := redactingWriter{dst: &buf, secrets: []string{"secret", ""}}
	n, err := w.Write([]byte("my secret is here"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 17 {
		t.Errorf("expected 17, got %d", n)
	}
	if buf.String() != "my [redacted] is here" {
		t.Errorf("expected redacted, got '%s'", buf.String())
	}
}

// validateResolvedHost edge cases

func TestEdge_ValidateResolvedHost_UnknownScheme(t *testing.T) {
	err := validateResolvedHost("unknown://host/path")
	if err != nil {
		t.Fatalf("expected nil for unknown scheme, got %v", err)
	}
}

// testBufferCapture implements io.Writer for testing
type testBufferCapture struct {
	buf []byte
}

func (b *testBufferCapture) Write(p []byte) (int, error) {
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *testBufferCapture) String() string {
	return string(b.buf)
}
