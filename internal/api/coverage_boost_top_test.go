package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestGenerateCSPNonce_Success(t *testing.T) {
	nonce := generateCSPNonce()
	if nonce == "" {
		t.Fatal("expected non-empty nonce")
	}
	if nonce == "DEPLOYMONSTER-FALLBACK" {
		t.Fatal("expected real nonce, got fallback")
	}
	if len(nonce) < 16 {
		t.Errorf("nonce too short: %q (len=%d)", nonce, len(nonce))
	}
}

func TestSPAHandler_ServeIndexHTMLWithNonceReadError(t *testing.T) {
	// Create a minimal FS that has files but NOT index.html
	mapFS := fstest.MapFS{
		"other.txt": &fstest.MapFile{Data: []byte("hello")},
	}
	h := &spaHandler{
		fileServer: http.FileServer(http.FS(mapFS)),
		fsys:       mapFS,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.serveIndexHTMLWithNonce(rr, req, "test-nonce")

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 when index.html missing, got %d", rr.Code)
	}
}

func TestSPAHandler_ServeIndexHTMLWithNonceSuccess(t *testing.T) {
	mapFS := fstest.MapFS{
		"index.html": &fstest.MapFile{
			Data: []byte(`<!DOCTYPE html><html><head><title>Test</title></head><body></body></html>`),
		},
	}
	h := &spaHandler{
		fileServer: http.FileServer(http.FS(mapFS)),
		fsys:       mapFS,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.serveIndexHTMLWithNonce(rr, req, "test-nonce-123")

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("CSP header should be set")
	}
	if !strings.Contains(csp, "nonce-test-nonce-123") {
		t.Errorf("CSP header should contain nonce, got: %s", csp)
	}
}

func TestSPAHandler_ServeFileWithNonceJS(t *testing.T) {
	mapFS := fstest.MapFS{
		"chunks/app.js": &fstest.MapFile{
			Data: []byte(`console.log("hello");`),
		},
	}
	h := &spaHandler{
		fileServer: http.FileServer(http.FS(mapFS)),
		fsys:       mapFS,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/chunks/app.js", nil)
	h.serveFileWithNonce(rr, req, "chunks/app.js")

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "strict-dynamic") {
		t.Errorf("JS CSP should include strict-dynamic, got: %s", csp)
	}
}

func TestSPAHandler_ServeFileWithNonceCSS(t *testing.T) {
	mapFS := fstest.MapFS{
		"assets/style.css": &fstest.MapFile{
			Data: []byte(`body { color: red; }`),
		},
	}
	h := &spaHandler{
		fileServer: http.FileServer(http.FS(mapFS)),
		fsys:       mapFS,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/style.css", nil)
	h.serveFileWithNonce(rr, req, "assets/style.css")

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRouter_HandleReadiness_ContainerRuntimePingError(t *testing.T) {
	c := &core.Core{
		Store:    &testStore{},
		Services: core.NewServices(),
	}
	c.Services.Container = &testContainerRuntimePingErr{}
	r := &Router{core: c}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	r.handleReadiness(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when container runtime unreachable, got %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "not_ready" {
		t.Errorf("expected status not_ready, got %v", body["status"])
	}
	reasons, ok := body["reasons"].([]any)
	if !ok {
		t.Fatal("expected reasons array")
	}
	found := false
	for _, r := range reasons {
		if fmt.Sprint(r) == "container runtime unreachable" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("reasons should include 'container runtime unreachable', got %v", reasons)
	}
}

func TestRouter_HandleReadiness_DBAndContainerBothFail(t *testing.T) {
	c := &core.Core{
		Store:    &testStorePingErr{},
		Services: core.NewServices(),
	}
	c.Services.Container = &testContainerRuntimePingErr{}
	r := &Router{core: c}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	r.handleReadiness(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// testContainerRuntimePingErr - container runtime that fails Ping.
type testContainerRuntimePingErr struct{}

func (t *testContainerRuntimePingErr) Ping() error { return fmt.Errorf("runtime down") }
func (t *testContainerRuntimePingErr) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (t *testContainerRuntimePingErr) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (t *testContainerRuntimePingErr) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (t *testContainerRuntimePingErr) Restart(_ context.Context, _ string) error        { return nil }
func (t *testContainerRuntimePingErr) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (t *testContainerRuntimePingErr) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (t *testContainerRuntimePingErr) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (t *testContainerRuntimePingErr) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}
func (t *testContainerRuntimePingErr) ImagePull(_ context.Context, _ string) error { return nil }
func (t *testContainerRuntimePingErr) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return nil, nil
}
func (t *testContainerRuntimePingErr) ImageRemove(_ context.Context, _ string) error { return nil }
func (t *testContainerRuntimePingErr) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (t *testContainerRuntimePingErr) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

func TestRouter_HandleHealth_Degraded(t *testing.T) {
	// Register a minimal module that reports down
	c := &core.Core{}
	reg := core.NewRegistry()
	reg.Register(&degradedModule{})
	reg.Resolve()
	c.Registry = reg
	r := &Router{core: c}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.handleHealth(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when module down, got %d", rr.Code)
	}
	var body map[string]any
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["status"] != "degraded" {
		t.Errorf("expected status degraded, got %v", body["status"])
	}
}

// degradedModule implements core.Module but reports HealthDown.
// NOTE: Name prefixed to avoid conflict with downModule in api_final_test.go.
type degradedModule struct{}

func (d *degradedModule) ID() string                             { return "test-degraded" }
func (d *degradedModule) Name() string                           { return "Test Degraded" }
func (d *degradedModule) Version() string                        { return "1.0" }
func (d *degradedModule) Dependencies() []string                 { return nil }
func (d *degradedModule) Routes() []core.Route                   { return nil }
func (d *degradedModule) Events() []core.EventHandler            { return nil }
func (d *degradedModule) Init(context.Context, *core.Core) error { return nil }
func (d *degradedModule) Start(context.Context) error            { return nil }
func (d *degradedModule) Stop(context.Context) error             { return nil }
func (d *degradedModule) Health() core.HealthStatus              { return core.HealthDown }

func TestWriteJSON_Success(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"key": "value"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["key"] != "value" {
		t.Errorf("body = %v, want key=value", body)
	}
}

func TestWriteJSON_NilData(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, nil)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestNewRouter_PprofEndpointsRegisteredWhenEnabled(t *testing.T) {
	c, authMod := testCoreSetup(t)
	c.Config.Server.EnablePprof = true
	r := NewRouter(c, authMod, c.Store)

	// Pprof routes should be registered. Since they're behind auth,
	// they return 401 (unauthorized) instead of 404.
	pprofPaths := []string{
		"/debug/pprof/",
		"/debug/pprof/cmdline",
		"/debug/pprof/symbol",
		"/debug/pprof/trace",
	}
	for _, path := range pprofPaths {
		t.Run(path, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, path, nil)
			r.mux.ServeHTTP(rr, req)
			if rr.Code == http.StatusNotFound {
				t.Errorf("pprof route %q should be registered but got 404", path)
			}
		})
	}
}

func TestNewRouter_PprofEndpointsNotRegisteredWhenDisabled(t *testing.T) {
	c, authMod := testCoreSetup(t)
	c.Config.Server.EnablePprof = false
	r := NewRouter(c, authMod, c.Store)

	// With EnablePprof=false, pprof routes are NOT registered.
	// Requests fall through to the SPA handler which returns 200.
	// The important thing is that the pprof route registration code
	// is NOT executed (covered by the enabled test above).
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	r.mux.ServeHTTP(rr, req)
	// The SPA handler catches unmatched routes, so it returns 200,
	// not the pprof handler.
	if rr.Code == http.StatusUnauthorized {
		t.Error("pprof route should NOT be registered when disabled, but got 401 (auth blocked)")
	}
	if !strings.Contains(rr.Body.String(), "DeployMonster") {
		t.Errorf("expected SPA fallback content, got: %s", rr.Body.String())
	}
}

func TestSPAHandler_ServeFileWithNonceOther(t *testing.T) {
	// Test serving a non-js, non-css file (plain asset)
	mapFS := fstest.MapFS{
		"favicon.svg": &fstest.MapFile{
			Data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`),
		},
	}
	h := &spaHandler{
		fileServer: http.FileServer(http.FS(mapFS)),
		fsys:       mapFS,
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)
	h.serveFileWithNonce(rr, req, "favicon.svg")

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	csp := rr.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") {
		t.Errorf("non-script/style CSP should have default-src 'self', got: %s", csp)
	}
	if strings.Contains(csp, "strict-dynamic") {
		t.Error("non-script assets should not have strict-dynamic")
	}
}

func TestNewSPAHandler_Fallback(t *testing.T) {
	// Verify that newSPAHandler returns a non-nil handler
	// that responds 200 (either real SPA or placeholder).
	h := newSPAHandler()
	if h == nil {
		t.Fatal("newSPAHandler returned nil")
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}
