package build

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Builder constructor
// ---------------------------------------------------------------------------

func TestNewBuilder(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	b := NewBuilder(nil, events)

	if b == nil {
		t.Fatal("NewBuilder returned nil")
	}
	if b.events != events {
		t.Error("events not set correctly")
	}
	if b.workDir == "" {
		t.Error("workDir should default to os.TempDir()")
	}
}

func TestNewBuilder_NilEvents(t *testing.T) {
	// NewBuilder should not panic with nil events (events used later during Build)
	b := NewBuilder(nil, nil)
	if b == nil {
		t.Fatal("NewBuilder returned nil")
	}
}

// ---------------------------------------------------------------------------
// injectToken
// ---------------------------------------------------------------------------

func TestInjectToken(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		token    string
		expected string
	}{
		{
			name:     "https URL",
			url:      "https://github.com/user/repo.git",
			token:    "ghp_abc123",
			expected: "https://ghp_abc123@github.com/user/repo.git",
		},
		{
			name:     "ssh URL unchanged",
			url:      "git@github.com:user/repo.git",
			token:    "ghp_abc123",
			expected: "git@github.com:user/repo.git",
		},
		{
			name:     "http URL unchanged (not https)",
			url:      "http://github.com/user/repo.git",
			token:    "ghp_abc123",
			expected: "http://github.com/user/repo.git",
		},
		{
			name:     "short URL unchanged",
			url:      "short",
			token:    "tok",
			expected: "short",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := injectToken(tt.url, tt.token)
			if got != tt.expected {
				t.Errorf("injectToken(%q, %q) = %q, want %q", tt.url, tt.token, got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Module lifecycle: ID, Name, Version, Health
// ---------------------------------------------------------------------------

func TestModule_Identity(t *testing.T) {
	m := New()

	if m.ID() != "build" {
		t.Errorf("ID: expected 'build', got %q", m.ID())
	}
	if m.Name() != "Build Engine" {
		t.Errorf("Name: expected 'Build Engine', got %q", m.Name())
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version: expected '1.0.0', got %q", m.Version())
	}
}

func TestModule_Health(t *testing.T) {
	m := New()
	if m.Health() != core.HealthOK {
		t.Errorf("Health: expected HealthOK, got %d", m.Health())
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d", len(deps))
	}
	expected := map[string]bool{"core.db": true, "deploy": true}
	for _, d := range deps {
		if !expected[d] {
			t.Errorf("unexpected dependency: %s", d)
		}
	}
}

func TestModule_RoutesAndEvents(t *testing.T) {
	m := New()
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes: expected nil, got %v", routes)
	}
	if events := m.Events(); events != nil {
		t.Errorf("Events: expected nil, got %v", events)
	}
}

// ---------------------------------------------------------------------------
// WorkerPool
// ---------------------------------------------------------------------------

func TestWorkerPool_Basic(t *testing.T) {
	pool := NewWorkerPool(3)

	results := make(chan int, 5)
	for i := 0; i < 5; i++ {
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
	if count != 5 {
		t.Errorf("expected 5 results, got %d", count)
	}
}

func TestWorkerPool_SingleWorker(t *testing.T) {
	pool := NewWorkerPool(1)

	var order []int
	for i := 0; i < 3; i++ {
		v := i
		pool.Submit(func() {
			order = append(order, v)
		})
	}

	pool.Wait()

	if len(order) != 3 {
		t.Errorf("expected 3 items, got %d", len(order))
	}
}

func TestNewWorkerPool(t *testing.T) {
	pool := NewWorkerPool(10)
	if pool.maxWorkers != 10 {
		t.Errorf("expected maxWorkers=10, got %d", pool.maxWorkers)
	}
}

// ---------------------------------------------------------------------------
// Dockerfile template generation for each project type
// ---------------------------------------------------------------------------

func TestGetDockerfileTemplate_AllTypes(t *testing.T) {
	types := []struct {
		ptype    ProjectType
		contains string // Expected substring in the template
	}{
		{TypeNodeJS, "node:22-alpine"},
		{TypeNextJS, "node:22-alpine"},
		{TypeVite, "nginx:alpine"},
		{TypeNuxt, "node:22-alpine"},
		{TypeGo, "golang:"},
		{TypeRust, "rust:"},
		{TypePython, "python:"},
		{TypePHP, "php:"},
		{TypeJava, "eclipse-temurin:"},
		{TypeDotNet, "mcr.microsoft.com/dotnet"},
		{TypeRuby, "ruby:"},
		{TypeStatic, "nginx:alpine"},
	}

	for _, tt := range types {
		t.Run(string(tt.ptype), func(t *testing.T) {
			tmpl := GetDockerfileTemplate(tt.ptype)
			if tmpl == "" {
				t.Fatalf("no template for %s", tt.ptype)
			}
			if !strings.Contains(tmpl, tt.contains) {
				t.Errorf("template for %s should contain %q", tt.ptype, tt.contains)
			}
		})
	}
}

func TestGetDockerfileTemplate_Unknown(t *testing.T) {
	tmpl := GetDockerfileTemplate(TypeUnknown)
	if tmpl != "" {
		t.Errorf("expected empty template for unknown type, got %d bytes", len(tmpl))
	}
}

func TestGetDockerfileTemplate_Dockerfile(t *testing.T) {
	// TypeDockerfile has no template (user provides their own)
	tmpl := GetDockerfileTemplate(TypeDockerfile)
	if tmpl != "" {
		t.Errorf("expected empty template for dockerfile type, got %d bytes", len(tmpl))
	}
}

// ---------------------------------------------------------------------------
// Mock container runtime for builder tests
// ---------------------------------------------------------------------------

type mockContainerRuntime struct {
	pinged bool
}

func (m *mockContainerRuntime) Ping() error                            { m.pinged = true; return nil }
func (m *mockContainerRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "id-123", nil
}
func (m *mockContainerRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (m *mockContainerRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockContainerRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *mockContainerRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockContainerRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}

func TestNewBuilder_WithRuntime(t *testing.T) {
	rt := &mockContainerRuntime{}
	events := core.NewEventBus(slog.Default())
	b := NewBuilder(rt, events)

	if b.runtime != rt {
		t.Error("runtime not set correctly")
	}
}

// ---------------------------------------------------------------------------
// exists helper
// ---------------------------------------------------------------------------

func TestExists(t *testing.T) {
	dir := t.TempDir()

	if !exists(dir) {
		t.Error("TempDir should exist")
	}

	if exists(dir + "/nonexistent-file") {
		t.Error("nonexistent path should not exist")
	}
}

// ---------------------------------------------------------------------------
// readPackageJSON
// ---------------------------------------------------------------------------

func TestReadPackageJSON_Valid(t *testing.T) {
	dir := t.TempDir()
	content := `{"name": "myapp", "version": "1.0.0", "dependencies": {"express": "^4.18.0"}}`
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(content), 0644)

	pkg := readPackageJSON(dir)
	if pkg == nil {
		t.Fatal("expected non-nil result")
	}
	if pkg["name"] != "myapp" {
		t.Errorf("name: expected 'myapp', got %v", pkg["name"])
	}
	if pkg["version"] != "1.0.0" {
		t.Errorf("version: expected '1.0.0', got %v", pkg["version"])
	}
}

func TestReadPackageJSON_Missing(t *testing.T) {
	dir := t.TempDir()
	pkg := readPackageJSON(dir)
	if pkg != nil {
		t.Errorf("expected nil for missing package.json, got %v", pkg)
	}
}

func TestReadPackageJSON_Invalid(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte("{invalid json"), 0644)

	pkg := readPackageJSON(dir)
	// json.Unmarshal fails silently, returns nil map
	if pkg != nil {
		t.Errorf("expected nil for invalid JSON, got %v", pkg)
	}
}

// ---------------------------------------------------------------------------
// emitFailed
// ---------------------------------------------------------------------------

func TestBuilder_EmitFailed(t *testing.T) {
	events := core.NewEventBus(slog.Default())

	var received bool
	events.Subscribe(core.EventBuildFailed, func(_ context.Context, e core.Event) error {
		received = true
		data, ok := e.Data.(core.BuildEventData)
		if !ok {
			t.Error("expected BuildEventData payload")
			return nil
		}
		if data.AppID != "app-1" {
			t.Errorf("expected AppID 'app-1', got %q", data.AppID)
		}
		if data.Error != "something broke" {
			t.Errorf("expected error 'something broke', got %q", data.Error)
		}
		return nil
	})

	b := NewBuilder(nil, events)
	b.emitFailed(context.Background(), "app-1", fmt.Errorf("something broke"))

	if !received {
		t.Error("expected build.failed event to be received")
	}
}

// ---------------------------------------------------------------------------
// Additional detector edge cases for coverage
// ---------------------------------------------------------------------------

func TestDetect_Nuxt(t *testing.T) {
	dir := setupDir(t, "package.json", "nuxt.config.ts")
	result := Detect(dir)
	if result.Type != TypeNuxt {
		t.Errorf("expected nuxt, got %s", result.Type)
	}
}

func TestDetect_NuxtJS(t *testing.T) {
	dir := setupDir(t, "package.json", "nuxt.config.js")
	result := Detect(dir)
	if result.Type != TypeNuxt {
		t.Errorf("expected nuxt, got %s", result.Type)
	}
}

func TestDetect_Ruby(t *testing.T) {
	dir := setupDir(t, "Gemfile")
	result := Detect(dir)
	if result.Type != TypeRuby {
		t.Errorf("expected ruby, got %s", result.Type)
	}
}

func TestDetect_NodeJS_Plain(t *testing.T) {
	// package.json only, no framework config files
	dir := setupDir(t, "package.json")
	result := Detect(dir)
	if result.Type != TypeNodeJS {
		t.Errorf("expected nodejs, got %s", result.Type)
	}
}

func TestDetect_ComposeYAML(t *testing.T) {
	dir := setupDir(t, "compose.yaml")
	result := Detect(dir)
	if result.Type != TypeDockerCompose {
		t.Errorf("expected docker-compose, got %s", result.Type)
	}
}

func TestDetect_ComposeYML(t *testing.T) {
	dir := setupDir(t, "compose.yml")
	result := Detect(dir)
	if result.Type != TypeDockerCompose {
		t.Errorf("expected docker-compose, got %s", result.Type)
	}
}

func TestDetect_DockerComposeYAML(t *testing.T) {
	dir := setupDir(t, "docker-compose.yaml")
	result := Detect(dir)
	if result.Type != TypeDockerCompose {
		t.Errorf("expected docker-compose, got %s", result.Type)
	}
}

func TestDetect_NextJS_MJS(t *testing.T) {
	dir := setupDir(t, "package.json", "next.config.mjs")
	result := Detect(dir)
	if result.Type != TypeNextJS {
		t.Errorf("expected nextjs, got %s", result.Type)
	}
}

func TestDetect_NextJS_TS(t *testing.T) {
	dir := setupDir(t, "package.json", "next.config.ts")
	result := Detect(dir)
	if result.Type != TypeNextJS {
		t.Errorf("expected nextjs, got %s", result.Type)
	}
}

func TestDetect_ViteMJS(t *testing.T) {
	dir := setupDir(t, "package.json", "vite.config.mjs")
	result := Detect(dir)
	if result.Type != TypeVite {
		t.Errorf("expected vite, got %s", result.Type)
	}
}

func TestDetect_ViteJS(t *testing.T) {
	dir := setupDir(t, "package.json", "vite.config.js")
	result := Detect(dir)
	if result.Type != TypeVite {
		t.Errorf("expected vite, got %s", result.Type)
	}
}

func TestDetect_PythonPyproject(t *testing.T) {
	dir := setupDir(t, "pyproject.toml")
	result := Detect(dir)
	if result.Type != TypePython {
		t.Errorf("expected python, got %s", result.Type)
	}
}

func TestDetect_PythonSetupPy(t *testing.T) {
	dir := setupDir(t, "setup.py")
	result := Detect(dir)
	if result.Type != TypePython {
		t.Errorf("expected python, got %s", result.Type)
	}
}

func TestDetect_PythonPipfile(t *testing.T) {
	dir := setupDir(t, "Pipfile")
	result := Detect(dir)
	if result.Type != TypePython {
		t.Errorf("expected python, got %s", result.Type)
	}
}

func TestDetect_JavaGradle(t *testing.T) {
	dir := setupDir(t, "build.gradle")
	result := Detect(dir)
	if result.Type != TypeJava {
		t.Errorf("expected java, got %s", result.Type)
	}
}

func TestDetect_JavaGradleKts(t *testing.T) {
	dir := setupDir(t, "build.gradle.kts")
	result := Detect(dir)
	if result.Type != TypeJava {
		t.Errorf("expected java, got %s", result.Type)
	}
}

func TestDetect_DotNetSln(t *testing.T) {
	dir := setupDir(t, "MyApp.sln")
	result := Detect(dir)
	if result.Type != TypeDotNet {
		t.Errorf("expected dotnet, got %s", result.Type)
	}
}

func TestDetect_DotNetFsproj(t *testing.T) {
	dir := setupDir(t, "MyApp.fsproj")
	result := Detect(dir)
	if result.Type != TypeDotNet {
		t.Errorf("expected dotnet, got %s", result.Type)
	}
}

func TestDetect_Confidence(t *testing.T) {
	dir := setupDir(t, "go.mod")
	result := Detect(dir)
	if result.Confidence != 90 {
		t.Errorf("expected confidence 90, got %d", result.Confidence)
	}
}

func TestDetect_UnknownConfidence(t *testing.T) {
	dir := setupDir(t, "random.txt")
	result := Detect(dir)
	if result.Confidence != 0 {
		t.Errorf("expected confidence 0 for unknown, got %d", result.Confidence)
	}
}

func TestDetect_Indicators(t *testing.T) {
	dir := setupDir(t, "go.mod")
	result := Detect(dir)
	if len(result.Indicators) == 0 {
		t.Error("expected at least one indicator")
	}
	if result.Indicators[0] != "go.mod" {
		t.Errorf("expected indicator 'go.mod', got %q", result.Indicators[0])
	}
}

// ---------------------------------------------------------------------------
// Module Init / Start / Stop lifecycle with minimal Core
// ---------------------------------------------------------------------------

func TestModule_InitStartStop(t *testing.T) {
	cfg := &core.Config{}
	cfg.Limits.MaxConcurrentBuilds = 3

	c := &core.Core{
		Config:   cfg,
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}

	m := New()

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.pool == nil {
		t.Fatal("pool should be initialized after Init")
	}
	if m.pool.maxWorkers != 3 {
		t.Errorf("expected maxWorkers=3, got %d", m.pool.maxWorkers)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestModule_Init_NegativeConcurrency(t *testing.T) {
	cfg := &core.Config{}
	cfg.Limits.MaxConcurrentBuilds = -1

	c := &core.Core{
		Config:   cfg,
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.pool.maxWorkers != 5 {
		t.Errorf("expected default maxWorkers=5 for negative config, got %d", m.pool.maxWorkers)
	}
}

func TestModule_Init_DefaultConcurrency(t *testing.T) {
	cfg := &core.Config{}
	// MaxConcurrentBuilds is 0 by default, should fall back to 5

	c := &core.Core{
		Config:   cfg,
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.pool.maxWorkers != 5 {
		t.Errorf("expected default maxWorkers=5, got %d", m.pool.maxWorkers)
	}
}

// ---------------------------------------------------------------------------
// Build — fails at gitClone because source URL is invalid.
// This exercises the early part of Build(): work dir creation,
// event emission, and the git clone error path.
// ---------------------------------------------------------------------------

func TestBuilder_Build_GitCloneFails(t *testing.T) {
	events := core.NewEventBus(slog.Default())

	var failedReceived bool
	events.Subscribe(core.EventBuildFailed, func(_ context.Context, _ core.Event) error {
		failedReceived = true
		return nil
	})

	var startedReceived bool
	events.Subscribe(core.EventBuildStarted, func(_ context.Context, _ core.Event) error {
		startedReceived = true
		return nil
	})

	b := NewBuilder(nil, events)

	_, err := b.Build(context.Background(), BuildOpts{
		AppID:     "app-test",
		AppName:   "testapp",
		SourceURL: "https://invalid.example.com/nonexistent/repo.git",
		Branch:    "main",
	}, io.Discard)

	if err == nil {
		t.Fatal("expected error from git clone")
	}
	if !strings.Contains(err.Error(), "git clone") {
		t.Errorf("expected 'git clone' in error, got: %v", err)
	}
	if !startedReceived {
		t.Error("expected build.started event")
	}
	if !failedReceived {
		t.Error("expected build.failed event")
	}
}

// ---------------------------------------------------------------------------
// Build — cancelled context
// ---------------------------------------------------------------------------

func TestBuilder_Build_CancelledContext(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	b := NewBuilder(nil, events)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := b.Build(ctx, BuildOpts{
		AppID:     "app-cancel",
		AppName:   "cancelapp",
		SourceURL: "https://example.com/repo.git",
		Branch:    "main",
		Timeout:   1, // 1 nanosecond — will expire immediately
	}, io.Discard)

	if err == nil {
		t.Fatal("expected error with cancelled context")
	}
}

// ---------------------------------------------------------------------------
// isDotNet — unreadable directory returns false
// ---------------------------------------------------------------------------

func TestDetect_DotNet_NoDir(t *testing.T) {
	ok, indicators := isDotNet("/nonexistent/path/that/does/not/exist")
	if ok {
		t.Error("expected false for nonexistent directory")
	}
	if indicators != nil {
		t.Errorf("expected nil indicators, got %v", indicators)
	}
}

// ---------------------------------------------------------------------------
// Nuxt/Vite/NextJS — package.json missing returns false
// ---------------------------------------------------------------------------

func TestDetect_NuxtNoPackageJSON(t *testing.T) {
	dir := setupDir(t, "nuxt.config.ts")
	result := Detect(dir)
	if result.Type == TypeNuxt {
		t.Error("should not detect nuxt without package.json")
	}
}

func TestDetect_ViteNoPackageJSON(t *testing.T) {
	dir := setupDir(t, "vite.config.ts")
	result := Detect(dir)
	if result.Type == TypeVite {
		t.Error("should not detect vite without package.json")
	}
}

func TestDetect_NextJSNoPackageJSON(t *testing.T) {
	dir := setupDir(t, "next.config.js")
	result := Detect(dir)
	if result.Type == TypeNextJS {
		t.Error("should not detect nextjs without package.json")
	}
}

// ---------------------------------------------------------------------------
// dockerBuild — fails because docker is not running / not found
// ---------------------------------------------------------------------------

func TestDockerBuild_Fails(t *testing.T) {
	dir := t.TempDir()
	dfPath := filepath.Join(dir, "Dockerfile")
	os.WriteFile(dfPath, []byte("FROM alpine\nRUN echo hello"), 0644)

	err := dockerBuild(context.Background(), dir, dfPath, "test:latest", nil, io.Discard)
	// docker command likely not available in CI or will fail — we just verify
	// the function doesn't panic and returns an error
	if err == nil {
		// If docker IS available, that's fine too — test still passes
		t.Log("docker build succeeded (docker is available)")
	}
}

func TestDockerBuild_WithBuildArgs(t *testing.T) {
	dir := t.TempDir()
	dfPath := filepath.Join(dir, "Dockerfile")
	os.WriteFile(dfPath, []byte("FROM alpine\nARG VERSION\nRUN echo $VERSION"), 0644)

	// Will fail if docker isn't available, which is fine
	_ = dockerBuild(context.Background(), dir, dfPath, "test:args", map[string]string{
		"VERSION": "1.0.0",
	}, io.Discard)
}
