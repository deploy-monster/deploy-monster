package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestMonitoringHandler_Metrics_WithContainerRuntime exercises the
// container-listing branch that the no-runtime test in
// monitoring_handler_test.go skips. Three containers are seeded with two
// in the "running" state and one stopped, so the response must report
// containers_total=3 and containers_running=2.
func TestMonitoringHandler_Metrics_WithContainerRuntime(t *testing.T) {
	c := monitoringTestCore()
	c.Services.Container = &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running"},
			{ID: "c2", State: "running"},
			{ID: "c3", State: "exited"},
		},
	}

	h := NewMonitoringHandler(c, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/server", nil)
	rr := httptest.NewRecorder()
	h.Metrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if got, _ := resp["containers_total"].(float64); got != 3 {
		t.Errorf("containers_total = %v, want 3", got)
	}
	if got, _ := resp["containers_running"].(float64); got != 2 {
		t.Errorf("containers_running = %v, want 2", got)
	}
}

// TestMonitoringHandler_Metrics_ContainerRuntimeError covers the error
// branch on ListByLabels — counts must fall back to zero rather than
// failing the whole response.
func TestMonitoringHandler_Metrics_ContainerRuntimeError(t *testing.T) {
	c := monitoringTestCore()
	c.Services.Container = &mockContainerRuntime{
		listErr: errBoom,
	}

	h := NewMonitoringHandler(c, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/server", nil)
	rr := httptest.NewRecorder()
	h.Metrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even on runtime error; body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if got, _ := resp["containers_total"].(float64); got != 0 {
		t.Errorf("containers_total = %v, want 0 when ListByLabels errors", got)
	}
}

// errBoom is a sentinel error for runtime mocks. Kept tiny and local.
var errBoom = &runtimeFailure{msg: "boom"}

type runtimeFailure struct{ msg string }

func (e *runtimeFailure) Error() string { return e.msg }

// ---------------------------------------------------------------------------
// SecretHandler.Delete
// ---------------------------------------------------------------------------

func TestSecretHandler_Delete_Unauthorized(t *testing.T) {
	h := NewSecretHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/sec-1", nil)
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestSecretHandler_Delete_MissingID(t *testing.T) {
	h := NewSecretHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))

	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when path id is empty", rr.Code)
	}
}

func TestSecretHandler_Delete_StoreDoesNotImplementInterface(t *testing.T) {
	// mockStore lacks DeleteSecret, so the handler must reply 501.
	h := NewSecretHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))

	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/sec-1", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	req.SetPathValue("id", "sec-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ServerHandler.List
// ---------------------------------------------------------------------------

func TestServerHandler_List_ReturnsLocalNode(t *testing.T) {
	h := NewServerHandler(newMockStore(), core.NewServices(), core.NewEventBus(slog.Default()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []struct {
			ID       string `json:"id"`
			Hostname string `json:"hostname"`
			Provider string `json:"provider"`
			Role     string `json:"role"`
			Status   string `json:"status"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("expected exactly one local server, got total=%d data=%d", resp.Total, len(resp.Data))
	}
	got := resp.Data[0]
	if got.ID != "local" || got.Provider != "local" || got.Role != "master" || got.Status != "active" {
		t.Fatalf("unexpected local server payload: %+v", got)
	}
	// Hostname mirrors the OS hostname when one is available; the
	// fallback is "local". Either is acceptable here.
	if hostname, _ := os.Hostname(); hostname != "" && got.Hostname != hostname && got.Hostname != "local" {
		t.Fatalf("hostname = %q, want OS hostname %q or fallback 'local'", got.Hostname, hostname)
	}
}
