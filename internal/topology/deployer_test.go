package topology

import (
	"os"
	"path/filepath"
	"strings"
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

// minimalCompose builds a tiny ComposeConfig suitable for exercising
// Deploy's dry-run path. It has one service, one network, and one
// volume so the extract* helpers all get exercised alongside.
func minimalCompose() *ComposeConfig {
	return &ComposeConfig{
		Version: "3.9",
		Services: map[string]Service{
			"web": {
				Image: "nginx:alpine",
			},
		},
		Networks: map[string]Network{
			"frontend": {},
		},
		Volumes: map[string]VolumeSpec{
			"data": {},
		},
	}
}

// TestDeployer_Deploy_DryRun_EmptyFiles covers the dry-run branch with
// neither a Caddyfile nor an envFile — only docker-compose.yaml is
// written. This is the shortest path through Deploy and should
// succeed without Docker.
func TestDeployer_Deploy_DryRun_EmptyFiles(t *testing.T) {
	d := NewDeployer(filepath.Join(t.TempDir(), "work"))

	result, err := d.Deploy(t.Context(), minimalCompose(), "", "", true)
	if err != nil {
		t.Fatalf("Deploy dry-run failed: %v", err)
	}
	if !result.Success {
		t.Error("dry-run result.Success = false")
	}
	if result.ComposeYAML == "" {
		t.Error("expected ComposeYAML in result")
	}
	if !strings.Contains(result.Message, "Dry run") {
		t.Errorf("unexpected message: %q", result.Message)
	}
	if _, err := os.Stat(filepath.Join(d.workDir, "docker-compose.yaml")); err != nil {
		t.Errorf("docker-compose.yaml not written: %v", err)
	}
	// Caddyfile / .env must NOT exist since they were empty.
	if _, err := os.Stat(filepath.Join(d.workDir, "Caddyfile")); !os.IsNotExist(err) {
		t.Errorf("Caddyfile should not exist with empty input: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.workDir, ".env")); !os.IsNotExist(err) {
		t.Errorf(".env should not exist with empty input: %v", err)
	}
}

// TestDeployer_Deploy_DryRun_AllFiles covers the branches that write
// a Caddyfile and a .env file.
func TestDeployer_Deploy_DryRun_AllFiles(t *testing.T) {
	d := NewDeployer(filepath.Join(t.TempDir(), "work"))

	result, err := d.Deploy(t.Context(), minimalCompose(),
		"example.com {\n  reverse_proxy web:80\n}\n",
		"KEY=value\nOTHER=1\n",
		true)
	if err != nil {
		t.Fatalf("Deploy dry-run failed: %v", err)
	}
	if result.Caddyfile == "" {
		t.Error("Caddyfile not propagated into result")
	}
	if result.EnvFile == "" {
		t.Error("EnvFile not propagated into result")
	}
	if _, err := os.Stat(filepath.Join(d.workDir, "Caddyfile")); err != nil {
		t.Errorf("Caddyfile not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.workDir, ".env")); err != nil {
		t.Errorf(".env not written: %v", err)
	}
}

// TestDeployer_Deploy_WorkDirUnwritable_ReturnsError plants a FILE at
// the workDir path so MkdirAll fails immediately. Exercises the
// "create work directory" error branch of Deploy without needing an
// exotic file-system setup.
func TestDeployer_Deploy_WorkDirUnwritable_ReturnsError(t *testing.T) {
	parent := t.TempDir()
	blocker := filepath.Join(parent, "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0600); err != nil {
		t.Fatalf("seed blocker file: %v", err)
	}

	// workDir points to a path nested INSIDE a regular file, which
	// must fail MkdirAll on every platform.
	d := NewDeployer(filepath.Join(blocker, "work"))
	_, err := d.Deploy(t.Context(), minimalCompose(), "", "", true)
	if err == nil {
		t.Error("expected MkdirAll error, got nil")
	}
}

// TestDeployer_Extractors covers extractContainerNames for both the
// named and unnamed service branches and verifies that network and
// volume extraction returns the top-level keys.
func TestDeployer_Extractors(t *testing.T) {
	d := NewDeployer(t.TempDir())
	compose := &ComposeConfig{
		Services: map[string]Service{
			"named-svc":   {ContainerName: "custom-name", Image: "a"},
			"unnamed-svc": {Image: "b"},
		},
		Networks: map[string]Network{"n1": {}, "n2": {}},
		Volumes:  map[string]VolumeSpec{"v1": {}},
	}
	containers := d.extractContainerNames(compose)
	if len(containers) != 2 {
		t.Errorf("containers = %v, want 2 entries", containers)
	}
	foundNamed, foundUnnamed := false, false
	for _, n := range containers {
		if n == "custom-name" {
			foundNamed = true
		}
		if n == "unnamed-svc" {
			foundUnnamed = true
		}
	}
	if !foundNamed {
		t.Error("expected custom container_name in extractContainerNames output")
	}
	if !foundUnnamed {
		t.Error("expected unnamed service key in extractContainerNames output")
	}

	nets := d.extractNetworkNames(compose)
	if len(nets) != 2 {
		t.Errorf("networks = %v, want 2", nets)
	}
	vols := d.extractVolumeNames(compose)
	if len(vols) != 1 || vols[0] != "v1" {
		t.Errorf("volumes = %v, want [v1]", vols)
	}
}

// TestDeployer_Build_NoBuildServices is a fast happy-path for Build
// that skips the docker exec entirely: with no service declaring a
// Build block, Build returns nil immediately.
func TestDeployer_Build_NoBuildServices(t *testing.T) {
	d := NewDeployer(t.TempDir())
	if err := d.Build(t.Context(), minimalCompose(), false); err != nil {
		t.Errorf("Build with zero build services should be a no-op, got %v", err)
	}
}
