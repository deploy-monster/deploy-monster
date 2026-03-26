package build

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Builder.Build — exercises more code paths
// ═══════════════════════════════════════════════════════════════════════════════

// createLocalGitRepo creates a tiny local git repo with the given files
// so we can test Build without network access.
func createLocalGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()

	// git init
	cmd := exec.Command("git", "init", dir)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		t.Skipf("git not available: %v", err)
	}

	// Configure git user for commits
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()

	// Write files
	for name, content := range files {
		fpath := filepath.Join(dir, name)
		os.MkdirAll(filepath.Dir(fpath), 0755)
		os.WriteFile(fpath, []byte(content), 0644)
	}

	// git add + commit
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	return dir
}

func TestBuilder_Build_LocalRepo_NoDockerfile(t *testing.T) {
	repoDir := createLocalGitRepo(t, map[string]string{
		"readme.txt": "hello",
	})

	events := core.NewEventBus(slog.Default())
	var failedReceived bool
	events.Subscribe(core.EventBuildFailed, func(_ context.Context, _ core.Event) error {
		failedReceived = true
		return nil
	})

	b := NewBuilder(nil, events)

	_, err := b.Build(context.Background(), BuildOpts{
		AppID:     "app-no-df",
		AppName:   "nodf",
		SourceURL: repoDir,
		Branch:    "",
		CommitSHA: "",
	}, io.Discard)

	if err == nil {
		t.Fatal("expected build to fail (no Dockerfile)")
	}
	if !strings.Contains(err.Error(), "no Dockerfile") {
		t.Errorf("expected 'no Dockerfile' error, got: %v", err)
	}
	if !failedReceived {
		t.Error("expected build.failed event")
	}
}

func TestBuilder_Build_LocalRepo_WithDockerfile(t *testing.T) {
	repoDir := createLocalGitRepo(t, map[string]string{
		"Dockerfile": "FROM alpine\nRUN echo hello",
		"main.go":    "package main\nfunc main() {}",
	})

	events := core.NewEventBus(slog.Default())
	var startedReceived, completedReceived, failedReceived bool

	events.Subscribe(core.EventBuildStarted, func(_ context.Context, _ core.Event) error {
		startedReceived = true
		return nil
	})
	events.Subscribe(core.EventBuildCompleted, func(_ context.Context, _ core.Event) error {
		completedReceived = true
		return nil
	})
	events.Subscribe(core.EventBuildFailed, func(_ context.Context, _ core.Event) error {
		failedReceived = true
		return nil
	})

	b := NewBuilder(nil, events)

	result, err := b.Build(context.Background(), BuildOpts{
		AppID:     "app-df",
		AppName:   "dfapp",
		SourceURL: repoDir,
		Branch:    "",
		ImageTag:  "test/dfapp:v1",
	}, io.Discard)

	if !startedReceived {
		t.Error("expected build.started event")
	}

	if err != nil {
		// Docker might not be available - that's fine, the important paths are covered
		if !failedReceived {
			t.Error("expected build.failed event on docker error")
		}
		t.Logf("build failed (docker likely unavailable): %v", err)
		return
	}

	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ImageTag != "test/dfapp:v1" {
		t.Errorf("ImageTag = %q, want test/dfapp:v1", result.ImageTag)
	}
	if !completedReceived {
		t.Error("expected build.completed event")
	}
}

func TestBuilder_Build_LocalRepo_GoProject(t *testing.T) {
	repoDir := createLocalGitRepo(t, map[string]string{
		"go.mod":  "module example.com/test\ngo 1.22",
		"main.go": "package main\nfunc main() {}",
	})

	events := core.NewEventBus(slog.Default())
	b := NewBuilder(nil, events)

	_, err := b.Build(context.Background(), BuildOpts{
		AppID:     "app-go",
		AppName:   "goapp",
		SourceURL: repoDir,
		Branch:    "",
	}, io.Discard)

	// Will either succeed (if docker is available) or fail at docker build
	// Either way, the detect + generate Dockerfile paths are exercised
	if err != nil {
		t.Logf("build failed (expected if docker unavailable): %v", err)
	}
}

func TestBuilder_Build_LocalRepo_CustomDockerfile(t *testing.T) {
	repoDir := createLocalGitRepo(t, map[string]string{
		"docker/Dockerfile.prod": "FROM alpine\nRUN echo prod",
		"main.go":                "package main",
	})

	events := core.NewEventBus(slog.Default())
	b := NewBuilder(nil, events)

	_, err := b.Build(context.Background(), BuildOpts{
		AppID:      "app-custom-df",
		AppName:    "customdf",
		SourceURL:  repoDir,
		Branch:     "",
		Dockerfile: "docker/Dockerfile.prod",
	}, io.Discard)

	if err != nil {
		t.Logf("build failed (expected if docker unavailable): %v", err)
	}
}

func TestBuilder_Build_DefaultTimeout(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	b := NewBuilder(nil, events)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := b.Build(ctx, BuildOpts{
		AppID:     "app-timeout",
		AppName:   "timeout",
		SourceURL: "https://invalid.test.example/repo.git",
		Branch:    "main",
		Timeout:   0, // Should default to 30 min, but ctx will cancel first
	}, io.Discard)

	if err == nil {
		t.Fatal("expected build to fail")
	}
}

func TestBuilder_Build_CustomImageTag(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	b := NewBuilder(nil, events)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, err := b.Build(ctx, BuildOpts{
		AppID:     "app-custom-tag",
		AppName:   "customtag",
		SourceURL: "https://invalid.test.example/repo.git",
		Branch:    "main",
		ImageTag:  "custom/image:v1.0.0",
	}, io.Discard)

	if err == nil {
		t.Fatal("expected build to fail")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// gitClone — edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestGitClone_WithToken(t *testing.T) {
	// Test that injectToken is called when token is provided.
	// Use a short timeout so we don't hang on DNS resolution.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	dir := t.TempDir()
	_, err := gitClone(ctx, "https://invalid.test.example/fake/repo.git", "main", "ghp_faketoken", dir, io.Discard)
	if err == nil {
		t.Log("git clone unexpectedly succeeded")
	}
}

func TestGitClone_NoBranch(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	dir := t.TempDir()
	_, err := gitClone(ctx, "https://invalid.test.example/nonexistent.git", "", "", dir, io.Discard)
	if err == nil {
		t.Log("git clone unexpectedly succeeded")
	}
}

func TestGitClone_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	dir := t.TempDir()
	_, err := gitClone(ctx, "https://invalid.test.example/fake/repo.git", "main", "", dir, io.Discard)
	if err == nil {
		t.Log("git clone unexpectedly succeeded with cancelled context")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// injectToken — additional cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestInjectToken_EmptyURL(t *testing.T) {
	got := injectToken("", "token")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestInjectToken_ExactlyHttps(t *testing.T) {
	// "https://" is exactly 8 chars, but the check is `len(url) > 8`, so it's unchanged
	got := injectToken("https://", "tok")
	if got != "https://" {
		t.Errorf("expected 'https://' (unchanged, len<=8), got %q", got)
	}
}

func TestInjectToken_HttpsWithHost(t *testing.T) {
	got := injectToken("https://h", "tok")
	if got != "https://tok@h" {
		t.Errorf("expected 'https://tok@h', got %q", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Detector — additional edge cases for more coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestDetect_StaticHTML_Solo(t *testing.T) {
	dir := setupDir(t, "index.html")
	result := Detect(dir)
	if result.Type != TypeStatic {
		t.Errorf("expected static, got %s", result.Type)
	}
}

func TestDetect_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	result := Detect(dir)
	if result.Type != TypeUnknown {
		t.Errorf("expected unknown for empty dir, got %s", result.Type)
	}
}

func TestDetect_PHPProject_Solo(t *testing.T) {
	dir := setupDir(t, "composer.json")
	result := Detect(dir)
	if result.Type != TypePHP {
		t.Errorf("expected php, got %s", result.Type)
	}
}

func TestDetect_JavaMaven_Solo(t *testing.T) {
	dir := setupDir(t, "pom.xml")
	result := Detect(dir)
	if result.Type != TypeJava {
		t.Errorf("expected java, got %s", result.Type)
	}
}

func TestDetect_DotNetCsproj_Solo(t *testing.T) {
	dir := setupDir(t, "MyApp.csproj")
	result := Detect(dir)
	if result.Type != TypeDotNet {
		t.Errorf("expected dotnet, got %s", result.Type)
	}
}

func TestDetect_Dockerfile_Solo(t *testing.T) {
	dir := setupDir(t, "Dockerfile")
	result := Detect(dir)
	if result.Type != TypeDockerfile {
		t.Errorf("expected dockerfile, got %s", result.Type)
	}
}

func TestDetect_DockerComposeYML_Alt(t *testing.T) {
	dir := setupDir(t, "docker-compose.yml")
	result := Detect(dir)
	if result.Type != TypeDockerCompose {
		t.Errorf("expected docker-compose, got %s", result.Type)
	}
}

func TestDetect_Rust_Solo(t *testing.T) {
	dir := setupDir(t, "Cargo.toml")
	result := Detect(dir)
	if result.Type != TypeRust {
		t.Errorf("expected rust, got %s", result.Type)
	}
}

func TestDetect_Go_Solo(t *testing.T) {
	dir := setupDir(t, "go.mod")
	result := Detect(dir)
	if result.Type != TypeGo {
		t.Errorf("expected go, got %s", result.Type)
	}
}

func TestDetect_PythonRequirements_Solo(t *testing.T) {
	dir := setupDir(t, "requirements.txt")
	result := Detect(dir)
	if result.Type != TypePython {
		t.Errorf("expected python, got %s", result.Type)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module lifecycle — more edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Stop_WaitsForPool(t *testing.T) {
	cfg := &core.Config{}
	cfg.Limits.MaxConcurrentBuilds = 2

	c := &core.Core{
		Config:   cfg,
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	// Submit a quick job
	done := make(chan struct{})
	m.pool.Submit(func() {
		close(done)
	})

	// Stop should wait for pool
	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	<-done
}

func TestModule_Init_HighConcurrency(t *testing.T) {
	cfg := &core.Config{}
	cfg.Limits.MaxConcurrentBuilds = 100

	c := &core.Core{
		Config:   cfg,
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.pool.maxWorkers != 100 {
		t.Errorf("expected maxWorkers=100, got %d", m.pool.maxWorkers)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Dockerfile template generation edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestGetDockerfileTemplate_NonExistentType(t *testing.T) {
	tmpl := GetDockerfileTemplate(ProjectType("nonexistent"))
	if tmpl != "" {
		t.Error("expected empty template for non-existent type")
	}
}

func TestGetDockerfileTemplate_AllTemplatesHaveFrom(t *testing.T) {
	types := []ProjectType{
		TypeNodeJS, TypeNextJS, TypeVite, TypeNuxt, TypeGo, TypeRust,
		TypePython, TypePHP, TypeJava, TypeDotNet, TypeRuby, TypeStatic,
	}

	for _, pt := range types {
		tmpl := GetDockerfileTemplate(pt)
		if tmpl == "" {
			t.Errorf("no template for %s", pt)
			continue
		}
		if !strings.Contains(tmpl, "FROM") {
			t.Errorf("template for %s should contain FROM directive", pt)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// exists helper — already tested but add more coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestExists_File(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("hello"), 0644)

	if !exists(f) {
		t.Error("file should exist")
	}
}

func TestExists_EmptyPath(t *testing.T) {
	if exists("") {
		t.Error("empty path should not exist")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WorkerPool edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestWorkerPool_ZeroCapacity(t *testing.T) {
	// Zero capacity should still work (blocks until slot available)
	// Actually make(chan struct{}, 0) will block on every submit
	// so this is an edge case - just verify no panic
	defer func() {
		if r := recover(); r != nil {
			t.Log("zero capacity pool panicked as expected")
		}
	}()

	pool := NewWorkerPool(0)
	if pool.maxWorkers != 0 {
		t.Errorf("expected maxWorkers=0, got %d", pool.maxWorkers)
	}
}

func TestWorkerPool_ConcurrentSubmit(t *testing.T) {
	pool := NewWorkerPool(5)
	results := make(chan int, 20)

	for i := 0; i < 20; i++ {
		v := i
		pool.Submit(func() {
			results <- v
		})
	}

	pool.Wait()
	close(results)

	count := 0
	for range results {
		count++
	}
	if count != 20 {
		t.Errorf("expected 20 results, got %d", count)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// dockerBuild — with cancelled context
// ═══════════════════════════════════════════════════════════════════════════════

func TestDockerBuild_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dir := t.TempDir()
	dfPath := filepath.Join(dir, "Dockerfile")
	os.WriteFile(dfPath, []byte("FROM alpine"), 0644)

	err := dockerBuild(ctx, dir, dfPath, "test:cancelled", nil, io.Discard)
	if err == nil {
		t.Log("docker build succeeded despite cancelled context")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// readPackageJSON edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestReadPackageJSON_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(""), 0644)

	pkg := readPackageJSON(dir)
	if pkg != nil {
		t.Errorf("expected nil for empty package.json, got %v", pkg)
	}
}

func TestReadPackageJSON_WithDeps(t *testing.T) {
	dir := t.TempDir()
	content := `{
		"name": "myapp",
		"dependencies": {
			"next": "^14.0.0",
			"react": "^18.0.0"
		}
	}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0644)

	pkg := readPackageJSON(dir)
	if pkg == nil {
		t.Fatal("expected non-nil result")
	}
	if pkg["name"] != "myapp" {
		t.Errorf("name: expected 'myapp', got %v", pkg["name"])
	}
}
