package compose

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

// =============================================================================
// Coverage targets:
//   deployer.go:40  Deploy         81.2%  — EnsureNetwork interface path (lines 48-54)
//   deployer.go:79  deployService  88.0%  — resource limits path (lines 121-124)
//   parser.go:107   Parse          83.3%  — nil service path (lines 119-121)
//   parser.go:131   ParseFile      0.0%   — entire function uncovered
// =============================================================================

// ---------------------------------------------------------------------------
// ParseFile — reads and parses a compose file from disk (0% coverage)
// ---------------------------------------------------------------------------

func TestFinal_ParseFile_ValidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "docker-compose.yml")

	content := `
services:
  web:
    image: nginx:alpine
    ports:
      - "80:80"
  db:
    image: postgres:16
    environment:
      POSTGRES_PASSWORD: secret
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cf, err := ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	if len(cf.Services) != 2 {
		t.Errorf("expected 2 services, got %d", len(cf.Services))
	}
	if cf.Services["web"] == nil {
		t.Error("expected web service")
	}
	if cf.Services["db"] == nil {
		t.Error("expected db service")
	}
	if cf.Services["web"].Image != "nginx:alpine" {
		t.Errorf("web image = %q, want nginx:alpine", cf.Services["web"].Image)
	}
}

func TestFinal_ParseFile_NonExistent(t *testing.T) {
	_, err := ParseFile("/nonexistent/path/docker-compose.yml")
	if err == nil {
		t.Fatal("expected error for non-existent file")
	}
	if !strings.Contains(err.Error(), "read compose file") {
		t.Errorf("expected 'read compose file' error, got: %v", err)
	}
}

func TestFinal_ParseFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yml")

	if err := os.WriteFile(path, []byte("{{invalid yaml}}"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestFinal_ParseFile_NoServices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.yml")

	if err := os.WriteFile(path, []byte("version: '3'\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	_, err := ParseFile(path)
	if err == nil {
		t.Fatal("expected error for file with no services")
	}
	if !strings.Contains(err.Error(), "no services") {
		t.Errorf("expected 'no services' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Parse — nil service in services map (lines 119-121)
// ---------------------------------------------------------------------------

func TestFinal_Parse_NilServiceValue(t *testing.T) {
	// YAML where a service key exists but has no value (null)
	yaml := `
services:
  web:
    image: nginx
  empty_svc:
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// The nil service should be replaced with an empty ServiceConfig
	svc := cf.Services["empty_svc"]
	if svc == nil {
		t.Fatal("expected empty_svc to be non-nil after Parse")
	}
	if svc.ResolvedEnv == nil {
		t.Error("expected ResolvedEnv to be initialized")
	}
}

// ---------------------------------------------------------------------------
// Deploy — EnsureNetwork interface assertion path (lines 48-54)
// ---------------------------------------------------------------------------

// networkRuntime implements ContainerRuntime + EnsureNetwork.
type networkRuntime struct {
	mockFinalRuntime
	ensureNetworkCalled bool
	ensureNetworkErr    error
}

func (nr *networkRuntime) EnsureNetwork(_ context.Context, name string) error {
	nr.ensureNetworkCalled = true
	if nr.ensureNetworkErr != nil {
		return nr.ensureNetworkErr
	}
	return nil
}

func TestFinal_Deploy_EnsureNetworkCalled(t *testing.T) {
	rt := &networkRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		AppID:     "app-1",
		TenantID:  "t-1",
		StackName: "teststack",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if !rt.ensureNetworkCalled {
		t.Error("expected EnsureNetwork to be called")
	}
}

func TestFinal_Deploy_EnsureNetworkError(t *testing.T) {
	rt := &networkRuntime{
		ensureNetworkErr: fmt.Errorf("network creation failed"),
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "teststack",
		Compose:   cf,
	})
	if err == nil {
		t.Fatal("expected error when EnsureNetwork fails")
	}
	if !strings.Contains(err.Error(), "create stack network") {
		t.Errorf("expected 'create stack network' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// deployService — resource limits path (lines 121-124)
// ---------------------------------------------------------------------------

func TestFinal_Deploy_WithResourceLimits(t *testing.T) {
	rt := &mockFinalRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
    deploy:
      resources:
        limits:
          memory: 512m
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		AppID:     "app-1",
		TenantID:  "t-1",
		StackName: "resource-test",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if len(rt.created) != 1 {
		t.Fatalf("expected 1 container, got %d", len(rt.created))
	}

	if rt.created[0].MemoryMB != 512 {
		t.Errorf("MemoryMB = %d, want 512", rt.created[0].MemoryMB)
	}
}

func TestFinal_Deploy_WithResourceLimitsGB(t *testing.T) {
	rt := &mockFinalRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
    deploy:
      resources:
        limits:
          memory: 2g
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "gb-test",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if rt.created[0].MemoryMB != 2048 {
		t.Errorf("MemoryMB = %d, want 2048", rt.created[0].MemoryMB)
	}
}

// ---------------------------------------------------------------------------
// Deploy — nil service in dependency order (line 66-67 skip nil svc)
// ---------------------------------------------------------------------------

func TestFinal_Deploy_SkipsNilServiceInOrder(t *testing.T) {
	rt := &mockFinalRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
`))

	// Manually inject a nil entry to simulate edge case
	cf.Services["ghost"] = nil

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "nil-svc-test",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	// Only "app" should be deployed, "ghost" should be skipped
	if len(rt.created) != 1 {
		t.Errorf("expected 1 container (ghost skipped), got %d", len(rt.created))
	}
}

// ---------------------------------------------------------------------------
// Deploy — with custom labels on service
// ---------------------------------------------------------------------------

func TestFinal_Deploy_CustomServiceLabels(t *testing.T) {
	rt := &mockFinalRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
    labels:
      custom.label: "my-value"
      another.label: "another-value"
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		AppID:     "app-1",
		TenantID:  "t-1",
		StackName: "label-test",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	labels := rt.created[0].Labels
	if labels["custom.label"] != "my-value" {
		t.Errorf("custom.label = %q, want my-value", labels["custom.label"])
	}
	if labels["another.label"] != "another-value" {
		t.Errorf("another.label = %q, want another-value", labels["another.label"])
	}
	// Standard monster labels should also be present
	if labels["monster.enable"] != "true" {
		t.Error("monster.enable label should be set")
	}
}

// ---------------------------------------------------------------------------
// Deploy — deploy resource limits with nil sub-fields
// ---------------------------------------------------------------------------

func TestFinal_Deploy_ResourcesWithNilLimits(t *testing.T) {
	rt := &mockFinalRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
    deploy:
      replicas: 1
`))

	// deploy.Resources is nil
	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "no-limits-test",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if rt.created[0].MemoryMB != 0 {
		t.Errorf("MemoryMB = %d, want 0 (no limits set)", rt.created[0].MemoryMB)
	}
}

// ---------------------------------------------------------------------------
// Mock runtime for final tests (does not collide with parser_extra_test.go)
// ---------------------------------------------------------------------------

type mockFinalRuntime struct {
	created []core.ContainerOpts
}

func (m *mockFinalRuntime) Ping() error { return nil }
func (m *mockFinalRuntime) CreateAndStart(_ context.Context, opts core.ContainerOpts) (string, error) {
	m.created = append(m.created, opts)
	return "container-" + opts.Name, nil
}
func (m *mockFinalRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (m *mockFinalRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockFinalRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *mockFinalRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockFinalRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (m *mockFinalRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *mockFinalRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}
func (m *mockFinalRuntime) ImagePull(_ context.Context, _ string) error           { return nil }
func (m *mockFinalRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }
func (m *mockFinalRuntime) ImageRemove(_ context.Context, _ string) error         { return nil }
func (m *mockFinalRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (m *mockFinalRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}
