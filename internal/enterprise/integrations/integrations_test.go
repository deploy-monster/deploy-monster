package integrations

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Mock ContainerRuntime
// ---------------------------------------------------------------------------

type mockContainerRuntime struct {
	containers []core.ContainerInfo
	listErr    error
}

func (m *mockContainerRuntime) Ping() error { return nil }
func (m *mockContainerRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (m *mockContainerRuntime) Stop(_ context.Context, _ string, _ int) error { return nil }
func (m *mockContainerRuntime) Remove(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockContainerRuntime) Restart(_ context.Context, _ string) error { return nil }
func (m *mockContainerRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockContainerRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return m.containers, m.listErr
}
func (m *mockContainerRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *mockContainerRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}
func (m *mockContainerRuntime) ImagePull(_ context.Context, _ string) error { return nil }
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

// ---------------------------------------------------------------------------
// Mock Module for Registry
// ---------------------------------------------------------------------------

type mockModule struct {
	id     string
	health core.HealthStatus
}

func (m *mockModule) ID() string                                 { return m.id }
func (m *mockModule) Name() string                               { return m.id }
func (m *mockModule) Version() string                            { return "1.0.0" }
func (m *mockModule) Dependencies() []string                     { return nil }
func (m *mockModule) Init(_ context.Context, _ *core.Core) error { return nil }
func (m *mockModule) Start(_ context.Context) error              { return nil }
func (m *mockModule) Stop(_ context.Context) error               { return nil }
func (m *mockModule) Health() core.HealthStatus                  { return m.health }
func (m *mockModule) Routes() []core.Route                       { return nil }
func (m *mockModule) Events() []core.EventHandler                { return nil }

// ===========================================================================
// PrometheusExporter tests
// ===========================================================================

func TestNewPrometheusExporter(t *testing.T) {
	reg := core.NewRegistry()
	events := core.NewEventBus(nil)
	svc := core.NewServices()

	p := NewPrometheusExporter(reg, events, svc)
	if p == nil {
		t.Fatal("NewPrometheusExporter returned nil")
	}
	if p.registry != reg {
		t.Error("registry not set")
	}
	if p.events != events {
		t.Error("events not set")
	}
	if p.services != svc {
		t.Error("services not set")
	}
	if p.startTime.IsZero() {
		t.Error("startTime should not be zero")
	}
}

func TestPrometheusExporter_Handler_BasicMetrics(t *testing.T) {
	reg := core.NewRegistry()
	events := core.NewEventBus(nil)
	svc := core.NewServices()

	p := NewPrometheusExporter(reg, events, svc)
	handler := p.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()

	// Check content type
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type = %q, want text/plain", ct)
	}

	// Check expected metric names
	expectedMetrics := []string{
		"deploymonster_uptime_seconds",
		"deploymonster_go_goroutines",
		"deploymonster_go_memory_bytes",
		"deploymonster_events_published_total",
		"deploymonster_events_errors_total",
		"deploymonster_events_subscriptions",
		"deploymonster_module_health",
	}
	for _, metric := range expectedMetrics {
		if !strings.Contains(body, metric) {
			t.Errorf("expected metric %q in output", metric)
		}
	}

	// Check HELP and TYPE annotations
	if !strings.Contains(body, "# HELP deploymonster_uptime_seconds") {
		t.Error("missing HELP for uptime_seconds")
	}
	if !strings.Contains(body, "# TYPE deploymonster_uptime_seconds gauge") {
		t.Error("missing TYPE for uptime_seconds")
	}
	if !strings.Contains(body, "# TYPE deploymonster_go_goroutines gauge") {
		t.Error("missing TYPE for go_goroutines")
	}
	if !strings.Contains(body, `type="alloc"`) {
		t.Error("missing alloc memory metric")
	}
	if !strings.Contains(body, `type="sys"`) {
		t.Error("missing sys memory metric")
	}
}

func TestPrometheusExporter_Handler_WithModules(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(&mockModule{id: "core.db", health: core.HealthOK})
	reg.Register(&mockModule{id: "api", health: core.HealthDegraded})
	reg.Resolve()

	events := core.NewEventBus(nil)
	svc := core.NewServices()

	p := NewPrometheusExporter(reg, events, svc)
	handler := p.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler(rec, req)

	body := rec.Body.String()

	if !strings.Contains(body, `deploymonster_module_health{module="core.db"} 0`) {
		t.Error("expected core.db health metric with value 0 (HealthOK)")
	}
	if !strings.Contains(body, `deploymonster_module_health{module="api"} 1`) {
		t.Error("expected api health metric with value 1 (HealthDegraded)")
	}
}

func TestPrometheusExporter_Handler_WithEventStats(t *testing.T) {
	reg := core.NewRegistry()
	events := core.NewEventBus(nil)
	svc := core.NewServices()

	// Publish some events to populate stats
	events.Subscribe("test.*", func(_ context.Context, _ core.Event) error {
		return nil
	})
	events.Publish(context.Background(), core.Event{Type: "test.one"})
	events.Publish(context.Background(), core.Event{Type: "test.two"})

	p := NewPrometheusExporter(reg, events, svc)
	handler := p.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler(rec, req)

	body := rec.Body.String()

	if !strings.Contains(body, "deploymonster_events_published_total 2") {
		t.Errorf("expected events_published_total 2, body:\n%s", body)
	}
	if !strings.Contains(body, "deploymonster_events_subscriptions 1") {
		t.Errorf("expected events_subscriptions 1, body:\n%s", body)
	}
}

func TestPrometheusExporter_Handler_WithContainers(t *testing.T) {
	reg := core.NewRegistry()
	events := core.NewEventBus(nil)
	svc := core.NewServices()

	mock := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", Name: "app1"},
			{ID: "c2", Name: "app2"},
			{ID: "c3", Name: "app3"},
		},
	}
	svc.Container = mock

	p := NewPrometheusExporter(reg, events, svc)
	handler := p.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler(rec, req)

	body := rec.Body.String()

	if !strings.Contains(body, "deploymonster_containers_total 3") {
		t.Errorf("expected containers_total 3, body:\n%s", body)
	}
}

func TestPrometheusExporter_Handler_WithContainerError(t *testing.T) {
	reg := core.NewRegistry()
	events := core.NewEventBus(nil)
	svc := core.NewServices()

	mock := &mockContainerRuntime{
		listErr: fmt.Errorf("docker unavailable"),
	}
	svc.Container = mock

	p := NewPrometheusExporter(reg, events, svc)
	handler := p.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler(rec, req)

	body := rec.Body.String()

	// Should NOT contain containers_total since listing errored
	if strings.Contains(body, "deploymonster_containers_total") {
		t.Error("containers_total should not appear when listing fails")
	}

	// But other metrics should still be present
	if !strings.Contains(body, "deploymonster_uptime_seconds") {
		t.Error("uptime metric should still be present")
	}
}

func TestPrometheusExporter_Handler_NoContainer(t *testing.T) {
	reg := core.NewRegistry()
	events := core.NewEventBus(nil)
	svc := core.NewServices()
	// svc.Container is nil

	p := NewPrometheusExporter(reg, events, svc)
	handler := p.Handler()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	handler(rec, req)

	body := rec.Body.String()

	// Should NOT contain containers_total
	if strings.Contains(body, "deploymonster_containers_total") {
		t.Error("containers_total should not appear when no container runtime")
	}
}

