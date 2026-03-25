package integrations

import (
	"context"
	"encoding/json"
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

func (m *mockModule) ID() string                           { return m.id }
func (m *mockModule) Name() string                         { return m.id }
func (m *mockModule) Version() string                      { return "1.0.0" }
func (m *mockModule) Dependencies() []string               { return nil }
func (m *mockModule) Init(_ context.Context, _ *core.Core) error { return nil }
func (m *mockModule) Start(_ context.Context) error        { return nil }
func (m *mockModule) Stop(_ context.Context) error         { return nil }
func (m *mockModule) Health() core.HealthStatus            { return m.health }
func (m *mockModule) Routes() []core.Route                 { return nil }
func (m *mockModule) Events() []core.EventHandler          { return nil }

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

// ===========================================================================
// WHMCSBridge tests
// ===========================================================================

func TestNewWHMCSBridge(t *testing.T) {
	w := NewWHMCSBridge("https://whmcs.example.com", "api-id", "api-secret")
	if w == nil {
		t.Fatal("NewWHMCSBridge returned nil")
	}
	if w.apiURL != "https://whmcs.example.com" {
		t.Errorf("apiURL = %q", w.apiURL)
	}
	if w.apiID != "api-id" {
		t.Errorf("apiID = %q", w.apiID)
	}
	if w.apiSecret != "api-secret" {
		t.Errorf("apiSecret = %q", w.apiSecret)
	}
	if w.client == nil {
		t.Fatal("client should not be nil")
	}
}

func TestWHMCSBridge_SyncModuleCommand_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if !strings.HasSuffix(r.URL.Path, "/includes/api.php") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		ct := r.Header.Get("Content-Type")
		if ct != "application/x-www-form-urlencoded" {
			t.Errorf("Content-Type = %q, want application/x-www-form-urlencoded", ct)
		}

		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "action=ModuleCustom") {
			t.Errorf("expected ModuleCustom action in body: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "id=42") {
			t.Errorf("expected id=42 in body: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "custom=provision") {
			t.Errorf("expected custom=provision in body: %s", bodyStr)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"result":  "success",
			"message": "done",
		})
	}))
	defer srv.Close()

	w := NewWHMCSBridge(srv.URL, "test-id", "test-secret")
	err := w.SyncModuleCommand(context.Background(), "provision", 42)
	if err != nil {
		t.Fatalf("SyncModuleCommand() error: %v", err)
	}
}

func TestWHMCSBridge_SyncModuleCommand_FailureResult(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"result":  "error",
			"message": "service not found",
		})
	}))
	defer srv.Close()

	w := NewWHMCSBridge(srv.URL, "test-id", "test-secret")
	err := w.SyncModuleCommand(context.Background(), "suspend", 99)
	if err == nil {
		t.Fatal("expected error for failed result")
	}
	if !strings.Contains(err.Error(), "service not found") {
		t.Errorf("expected 'service not found' error, got: %v", err)
	}
}

func TestWHMCSBridge_SyncModuleCommand_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	w := NewWHMCSBridge(srv.URL, "test-id", "test-secret")
	err := w.SyncModuleCommand(context.Background(), "terminate", 1)
	if err == nil {
		t.Fatal("expected error for HTTP 500")
	}
	if !strings.Contains(err.Error(), "WHMCS error") {
		t.Errorf("expected 'WHMCS error', got: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 500") {
		t.Errorf("expected 'HTTP 500' in error, got: %v", err)
	}
}

func TestWHMCSBridge_SyncModuleCommand_NetworkError(t *testing.T) {
	w := NewWHMCSBridge("http://localhost:1", "test-id", "test-secret")
	err := w.SyncModuleCommand(context.Background(), "provision", 1)
	if err == nil {
		t.Fatal("expected network error")
	}
	if !strings.Contains(err.Error(), "WHMCS API") {
		t.Errorf("expected 'WHMCS API' error prefix, got: %v", err)
	}
}

func TestWHMCSBridge_GetClientDetails_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)
		if !strings.Contains(bodyStr, "action=GetClientsDetails") {
			t.Errorf("expected GetClientsDetails action: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "clientid=123") {
			t.Errorf("expected clientid=123: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "responsetype=json") {
			t.Errorf("expected responsetype=json: %s", bodyStr)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"result":    "success",
			"email":     "client@example.com",
			"firstname": "John",
			"lastname":  "Doe",
			"clientid":  123,
		})
	}))
	defer srv.Close()

	w := NewWHMCSBridge(srv.URL, "test-id", "test-secret")
	result, err := w.GetClientDetails(context.Background(), 123)
	if err != nil {
		t.Fatalf("GetClientDetails() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result["email"] != "client@example.com" {
		t.Errorf("email = %v, want 'client@example.com'", result["email"])
	}
	if result["firstname"] != "John" {
		t.Errorf("firstname = %v, want 'John'", result["firstname"])
	}
}

func TestWHMCSBridge_GetClientDetails_NetworkError(t *testing.T) {
	w := NewWHMCSBridge("http://localhost:1", "test-id", "test-secret")
	_, err := w.GetClientDetails(context.Background(), 1)
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestWHMCSBridge_GetClientDetails_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	w := NewWHMCSBridge(srv.URL, "test-id", "test-secret")
	result, err := w.GetClientDetails(context.Background(), 999)
	if err != nil {
		t.Fatalf("GetClientDetails() error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result even for empty response")
	}
}

func TestProvisionRequest_Fields(t *testing.T) {
	req := ProvisionRequest{
		ServiceID: 1,
		ClientID:  2,
		Email:     "test@example.com",
		Package:   "starter",
		Domain:    "example.com",
		Username:  "testuser",
	}
	if req.ServiceID != 1 {
		t.Errorf("ServiceID = %d", req.ServiceID)
	}
	if req.Email != "test@example.com" {
		t.Errorf("Email = %q", req.Email)
	}
}

func TestProvisionResponse_Fields(t *testing.T) {
	resp := ProvisionResponse{
		Success:  true,
		TenantID: "tenant-1",
		LoginURL: "https://app.example.com/login",
		Message:  "provisioned",
	}
	if !resp.Success {
		t.Error("expected Success = true")
	}
	if resp.TenantID != "tenant-1" {
		t.Errorf("TenantID = %q", resp.TenantID)
	}
}

func TestProvisionRequest_JSON(t *testing.T) {
	req := ProvisionRequest{
		ServiceID: 42,
		ClientID:  7,
		Email:     "json@example.com",
		Package:   "pro",
		Domain:    "pro.example.com",
		Username:  "prouser",
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ProvisionRequest
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if parsed.ServiceID != 42 {
		t.Errorf("ServiceID = %d, want 42", parsed.ServiceID)
	}
	if parsed.Package != "pro" {
		t.Errorf("Package = %q, want 'pro'", parsed.Package)
	}
}

func TestProvisionResponse_JSON(t *testing.T) {
	resp := ProvisionResponse{
		Success:  true,
		TenantID: "t-1",
		LoginURL: "https://login.example.com",
		Message:  "all good",
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var parsed ProvisionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !parsed.Success {
		t.Error("expected Success = true")
	}
	if parsed.LoginURL != "https://login.example.com" {
		t.Errorf("LoginURL = %q", parsed.LoginURL)
	}
}

func TestWHMCSBridge_SyncModuleCommand_InvalidURL(t *testing.T) {
	// Trigger http.NewRequestWithContext error with control character in URL
	w := NewWHMCSBridge("http://invalid\x7f.example.com", "id", "secret")
	err := w.SyncModuleCommand(context.Background(), "provision", 1)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestWHMCSBridge_GetClientDetails_InvalidURL(t *testing.T) {
	w := NewWHMCSBridge("http://invalid\x7f.example.com", "id", "secret")
	_, err := w.GetClientDetails(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestWHMCSBridge_SyncModuleCommand_VerifiesParams(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodyStr := string(body)

		// Verify identifier and secret are passed
		if !strings.Contains(bodyStr, "identifier=my-id") {
			t.Errorf("expected identifier=my-id in body: %s", bodyStr)
		}
		if !strings.Contains(bodyStr, "secret=my-secret") {
			t.Errorf("expected secret=my-secret in body: %s", bodyStr)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"result": "success"})
	}))
	defer srv.Close()

	w := NewWHMCSBridge(srv.URL, "my-id", "my-secret")
	err := w.SyncModuleCommand(context.Background(), "test", 1)
	if err != nil {
		t.Fatalf("SyncModuleCommand() error: %v", err)
	}
}
