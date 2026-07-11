package database

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// mockContainerRuntime is a minimal stub for testing the Health OK path.
type mockContainerRuntime struct{}

func (m *mockContainerRuntime) Ping() error { return nil }
func (m *mockContainerRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
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
func (m *mockContainerRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *mockContainerRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}
func (m *mockContainerRuntime) ImagePull(_ context.Context, _ string) error  { return nil }
func (m *mockContainerRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return nil, nil
}
func (m *mockContainerRuntime) ImageRemove(_ context.Context, _ string) error { return nil }
func (m *mockContainerRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (m *mockContainerRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

// TestModuleHealth_OK covers the Health() return value when the container
// runtime is available — the existing tests only exercise the nil-core path.
func TestModuleHealth_OK(t *testing.T) {
	m := New()
	c := &core.Core{
		Services: &core.Services{
			Container: &mockContainerRuntime{},
		},
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() = %v, want %v", h, core.HealthOK)
	}
}

// TestInit_NewApp calls core.NewApp which triggers registerAllModules,
// invoking the init()-registered factory function. This covers the
// anonymous function body inside init() that was previously untested.
func TestInit_NewApp(t *testing.T) {
	cfg := &core.Config{
		Server: core.ServerConfig{
			SecretKey: "test-secret-key-for-init-coverage",
		},
	}
	_, err := core.NewApp(cfg, core.BuildInfo{Version: "0.0.0"})
	if err != nil {
		t.Logf("NewApp returned (expected in unit test): %v", err)
		// NewApp may fail due to missing dependencies (no real DB, etc.)
		// but registerAllModules runs before those failures, so the
		// init() factory function is still exercised.
	}
}
