package topology

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectBuildPack_Dockerfile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine"), 0644)
	if got := DetectBuildPack(dir); got != "dockerfile" {
		t.Errorf("DetectBuildPack = %q, want %q", got, "dockerfile")
	}
}

func TestDetectBuildPack_DockerfileTakesPrecedence(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM alpine"), 0644)
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"express":"1"}}`), 0644)
	if got := DetectBuildPack(dir); got != "dockerfile" {
		t.Errorf("Dockerfile should take precedence, got %q", got)
	}
}

func TestDetectBuildPack_NodeJS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"express":"4"}}`), 0644)
	if got := DetectBuildPack(dir); got != "nodejs" {
		t.Errorf("DetectBuildPack = %q, want %q", got, "nodejs")
	}
}

func TestDetectBuildPack_NextJS(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"dependencies":{"next":"14","react":"18"}}`), 0644)
	if got := DetectBuildPack(dir); got != "nextjs" {
		t.Errorf("DetectBuildPack = %q, want %q", got, "nextjs")
	}
}

func TestDetectBuildPack_Go(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/app"), 0644)
	if got := DetectBuildPack(dir); got != "go" {
		t.Errorf("DetectBuildPack = %q, want %q", got, "go")
	}
}

func TestDetectBuildPack_PythonRequirements(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask==2.0"), 0644)
	if got := DetectBuildPack(dir); got != "python" {
		t.Errorf("DetectBuildPack = %q, want %q", got, "python")
	}
}

func TestDetectBuildPack_PythonPyproject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.poetry]"), 0644)
	if got := DetectBuildPack(dir); got != "python" {
		t.Errorf("DetectBuildPack = %q, want %q", got, "python")
	}
}

func TestDetectBuildPack_Rust(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]"), 0644)
	if got := DetectBuildPack(dir); got != "rust" {
		t.Errorf("DetectBuildPack = %q, want %q", got, "rust")
	}
}

func TestDetectBuildPack_EmptyDir_DefaultsToDockerfile(t *testing.T) {
	dir := t.TempDir()
	if got := DetectBuildPack(dir); got != "dockerfile" {
		t.Errorf("DetectBuildPack (empty dir) = %q, want %q", got, "dockerfile")
	}
}

func TestDetectBuildPack_NonexistentDir_DefaultsToDockerfile(t *testing.T) {
	if got := DetectBuildPack("/nonexistent/path/xyz"); got != "dockerfile" {
		t.Errorf("DetectBuildPack (bad path) = %q, want %q", got, "dockerfile")
	}
}

func TestCloneGitRepo_InvalidURL(t *testing.T) {
	err := CloneGitRepo(t.Context(), "not-a-valid-url", "main", t.TempDir())
	if err == nil {
		t.Fatal("expected error for invalid git URL")
	}
}
