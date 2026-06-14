package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

type setupPingRuntime struct {
	*mockContainerRuntime
	err error
}

func (r setupPingRuntime) PingContext(context.Context) error { return r.err }

func TestSetupHandlerChecksDefaultsWhenCoreMissing(t *testing.T) {
	h := NewSetupHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/checks", nil)
	rr := httptest.NewRecorder()
	h.Checks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Data []setupCheck `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := setupChecksByLabel(resp.Data)
	if got["Docker Engine"].Status != "fail" {
		t.Fatalf("docker status = %+v", got["Docker Engine"])
	}
	if got["SSL"].Value != "Disabled (HTTP only)" || got["SSL"].Status != "warn" {
		t.Fatalf("ssl check = %+v", got["SSL"])
	}
	if got["Ports"].Value == "" || !validSetupStatus(got["Ports"].Status) {
		t.Fatalf("ports check = %+v", got["Ports"])
	}
}

func TestSetupHandlerChecksWithConfiguredServices(t *testing.T) {
	apiListener := listenOnFreePort(t)
	defer apiListener.Close()

	h := NewSetupHandler(&core.Core{
		Services: &core.Services{
			Container: setupPingRuntime{mockContainerRuntime: &mockContainerRuntime{}},
		},
		Config: &core.Config{
			Server:  core.ServerConfig{Port: apiListener.Addr().(*net.TCPAddr).Port},
			Ingress: core.IngressConfig{EnableHTTPS: true, HTTPPort: unusedPort(t), HTTPSPort: unusedPort(t)},
			ACME:    core.ACMEConfig{Email: "ops@example.com"},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/checks", nil)
	rr := httptest.NewRecorder()
	h.Checks(rr, req)

	var resp struct {
		Data []setupCheck `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := setupChecksByLabel(resp.Data)
	if got["Docker Engine"].Status != "ok" {
		t.Fatalf("docker check = %+v", got["Docker Engine"])
	}
	if got["SSL"].Status != "ok" {
		t.Fatalf("ssl check = %+v", got["SSL"])
	}
	if got["Ports"].Status != "warn" {
		t.Fatalf("ports check = %+v", got["Ports"])
	}
}

func TestSetupHandlerChecksReportsPingContextError(t *testing.T) {
	h := NewSetupHandler(&core.Core{
		Services: &core.Services{
			Container: setupPingRuntime{mockContainerRuntime: &mockContainerRuntime{}, err: errors.New("daemon down")},
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/setup/checks", nil)
	rr := httptest.NewRecorder()
	h.Checks(rr, req)

	var resp struct {
		Data []setupCheck `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	got := setupChecksByLabel(resp.Data)
	if got["Docker Engine"].Status != "fail" || got["Docker Engine"].Value != "daemon down" {
		t.Fatalf("docker check = %+v", got["Docker Engine"])
	}
}

func setupChecksByLabel(checks []setupCheck) map[string]setupCheck {
	out := make(map[string]setupCheck, len(checks))
	for _, check := range checks {
		out[check.Label] = check
	}
	return out
}

func validSetupStatus(status string) bool {
	return status == "ok" || status == "warn" || status == "fail"
}

func listenOnFreePort(t *testing.T) net.Listener {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return l
}

func unusedPort(t *testing.T) int {
	t.Helper()
	l := listenOnFreePort(t)
	addr := l.Addr().(*net.TCPAddr)
	if err := l.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	return addr.Port
}
