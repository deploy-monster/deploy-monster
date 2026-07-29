package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// === merged from api_final_test.go ===

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
	// version field removed from health endpoint to avoid information disclosure
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

// === merged from coverage_boost_top_test.go ===

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

// === merged from spa_integration_test.go ===

// newTestRouter spins up a real Router backed by temp SQLite + KV storage so
// tests can exercise the full middleware chain (rate limiter, CORS,
// CSRF, compression, SPA fallback) instead of hitting just the SPA
// handler in isolation.
func newTestRouter(t *testing.T) *httptest.Server {
	t.Helper()
	tmp := t.TempDir()
	sqlite, err := db.NewSQLite(filepath.Join(tmp, "test.db"))
	if err != nil {
		t.Fatalf("sqlite init: %v", err)
	}
	t.Cleanup(func() { sqlite.Close() })

	kv, err := db.NewKVStore(filepath.Join(tmp, "test.kv"))
	if err != nil {
		t.Fatalf("kv init: %v", err)
	}
	t.Cleanup(func() { kv.Close() })

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	events := core.NewEventBus(logger)
	registry := core.NewRegistry()
	services := core.NewServices()
	mpReg := marketplace.NewTemplateRegistry()
	mpReg.LoadBuiltins()
	authMod := newTestAuthServices(t, "test-secret-key-for-integration-tests-32b")
	cfg, _ := core.LoadConfig("")
	c := &core.Core{
		Config:   cfg,
		Store:    sqlite,
		Events:   events,
		Logger:   logger,
		Registry: registry,
		Services: services,
		DB:       &core.Database{KV: kv},
	}
	registry.Register(authMod)
	mpMod := marketplace.New()
	mpMod.Init(context.Background(), c)
	registry.Register(mpMod)

	router := NewRouter(c, authMod, sqlite)
	srv := httptest.NewServer(router.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// TestFullRouter_SPA_RegisterRouteServesHTML is a high-fidelity
// regression test for the Playwright E2E "Loading" hang: every fresh
// browser context that opened /register or /login got stuck on the
// Suspense fallback forever. A Go unit test can't run JavaScript, but
// it CAN verify the exact HTTP responses that Chromium sees:
//
//  1. GET /register must be 200 text/html containing <div id="root">
//     and the module entry script — no 30x, no empty body, no JSON.
//  2. GET /assets/<entry>.js must be 200 JS — not the SPA shell.
//  3. GET /chunks/<lazy>.js must be 200 JS — not the SPA shell.
//
// If any of these regress, the "status Loading" failure mode returns.
func TestFullRouter_SPA_RegisterRouteServesHTML(t *testing.T) {
	srv := newTestRouter(t)

	resp, err := http.Get(srv.URL + "/register")
	if err != nil {
		t.Fatalf("GET /register: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /register status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET /register content-type = %q, want text/html", ct)
	}
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)
	if !strings.Contains(bodyStr, `<div id="root">`) {
		t.Error("GET /register body missing React root div")
	}
	if !strings.Contains(bodyStr, `type="module"`) && !strings.Contains(bodyStr, `DeployMonster</h1>`) {
		// Either embedded index.html (module script) or dev placeholder
		t.Errorf("GET /register body is neither index.html nor placeholder; first 300 chars = %q", trunc(bodyStr, 300))
	}
}

// TestFullRouter_SPA_EntryAndChunksServedAsJS hits every /assets/ and
// /chunks/ path referenced by the committed index.html through the
// FULL router (not the SPA handler in isolation) and asserts each
// response is 200 with a non-HTML content type. This catches any
// middleware (rate limiter, CORS, compression) that might corrupt
// static asset responses and gives the "Loading" regression exactly
// zero places to hide.
func TestFullRouter_SPA_EntryAndChunksServedAsJS(t *testing.T) {
	srv := newTestRouter(t)

	// First fetch /register to get the HTML body with asset refs.
	resp, err := http.Get(srv.URL + "/register")
	if err != nil {
		t.Fatalf("GET /register: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	html := string(body)

	// Skip if running with the dev placeholder (no embed).
	if !strings.Contains(html, `/assets/`) && !strings.Contains(html, `/chunks/`) {
		t.Skip("no embedded UI in this build — skipping")
	}

	// Collect all /assets/* and /chunks/* references.
	var refs []string
	for _, piece := range strings.Split(html, `"`) {
		if strings.HasPrefix(piece, "/assets/") || strings.HasPrefix(piece, "/chunks/") {
			refs = append(refs, piece)
		}
	}
	if len(refs) == 0 {
		t.Fatal("no /assets/ or /chunks/ references found in index.html")
	}

	for _, ref := range refs {
		t.Run(ref, func(t *testing.T) {
			r, err := http.Get(srv.URL + ref)
			if err != nil {
				t.Fatalf("GET %s: %v", ref, err)
			}
			defer r.Body.Close()

			if r.StatusCode != http.StatusOK {
				t.Errorf("GET %s status = %d, want 200", ref, r.StatusCode)
			}
			ct := r.Header.Get("Content-Type")
			if strings.HasPrefix(ct, "text/html") {
				t.Errorf("GET %s content-type = %q — SPA fallback is swallowing the request", ref, ct)
			}
			// Assert the body isn't literally the index.html shell.
			b, _ := io.ReadAll(r.Body)
			if strings.Contains(string(b), `<div id="root">`) {
				t.Errorf("GET %s body looks like the React shell, not the actual asset", ref)
			}
		})
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
