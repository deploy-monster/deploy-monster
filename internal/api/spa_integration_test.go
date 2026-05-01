package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// newTestRouter spins up a real Router backed by temp SQLite + bbolt so
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

	bolt, err := db.NewBoltStore(filepath.Join(tmp, "test.bolt"))
	if err != nil {
		t.Fatalf("bolt init: %v", err)
	}
	t.Cleanup(func() { bolt.Close() })

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
		DB:       &core.Database{Bolt: bolt},
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
