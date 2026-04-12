package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// handleHealth — covers router.go:629 (the HealthDown degraded branch)
// We test handleHealth directly by constructing a minimal Router with a
// Registry containing a module that reports HealthDown.
// ═══════════════════════════════════════════════════════════════════════════════

func TestHandleHealth_DegradedWhenModuleDown(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(&downModule{id: "test.down", down: true})
	reg.Resolve()

	r := &Router{
		core: &core.Core{
			Registry: reg,
			Build:    core.BuildInfo{Version: "1.0.0-test"},
		},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	r.handleHealth(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (degraded)", rr.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %q, want degraded", body["status"])
	}
}

func TestHandleHealth_OKWhenAllHealthy(t *testing.T) {
	reg := core.NewRegistry()
	reg.Register(&downModule{id: "test.ok", down: false})
	reg.Resolve()

	r := &Router{
		core: &core.Core{
			Registry: reg,
			Build:    core.BuildInfo{Version: "2.0.0"},
		},
	}

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	r.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %q, want ok", body["status"])
	}
	if body["version"] != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", body["version"])
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// newSPAHandler — covers spa.go:22
// The embedded static dir exists, so the normal path is taken.
// We test both the exact file serving and the SPA fallback to index.html.
// ═══════════════════════════════════════════════════════════════════════════════

func TestSPAHandler_ServeIndexHTML(t *testing.T) {
	h := newSPAHandler()

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestSPAHandler_FallbackToIndex(t *testing.T) {
	h := newSPAHandler()

	// Request a non-existent path — SPA should fallback to index.html
	req := httptest.NewRequest("GET", "/app/dashboard/settings", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (SPA fallback)", rr.Code)
	}
}

func TestSPAHandler_ServeStaticAsset(t *testing.T) {
	h := newSPAHandler()

	req := httptest.NewRequest("GET", "/favicon.svg", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for static asset", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// init — covers module.go:14
// ═══════════════════════════════════════════════════════════════════════════════

func TestInit_RegisteredAsModule(t *testing.T) {
	m := New()
	var _ core.Module = m
	if m.ID() != "api" {
		t.Errorf("ID() = %q, want api", m.ID())
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// downModule is a stub that reports HealthDown for testing handleHealth.
// ═══════════════════════════════════════════════════════════════════════════════

type downModule struct {
	id   string
	down bool
}

func (d *downModule) ID() string                  { return d.id }
func (d *downModule) Name() string                { return d.id }
func (d *downModule) Version() string             { return "1.0.0" }
func (d *downModule) Dependencies() []string      { return nil }
func (d *downModule) Routes() []core.Route        { return nil }
func (d *downModule) Events() []core.EventHandler { return nil }

func (d *downModule) Init(_ context.Context, _ *core.Core) error { return nil }
func (d *downModule) Start(_ context.Context) error              { return nil }
func (d *downModule) Stop(_ context.Context) error               { return nil }

func (d *downModule) Health() core.HealthStatus {
	if d.down {
		return core.HealthDown
	}
	return core.HealthOK
}
