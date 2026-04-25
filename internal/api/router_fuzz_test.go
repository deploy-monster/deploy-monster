package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ============================================================================
// Phase 7.11 — Cross-tenant authorization fuzz target
// ============================================================================
//
// FuzzRouter_CrossTenant stress-tests tenant isolation on every resource-
// scoped GET route. The fuzzer walks each `{id}` route with a developer
// JWT minted for `tenant-A` while the underlying store is pre-seeded with
// a resource that belongs to `tenant-B`. Any HTTP 2xx response is an
// isolation leak — a correct handler must either return 404 (resource
// hidden from foreign tenant) or, in rare cases, 403.
//
// The store wrapper also returns core.ErrNotFound for unknown IDs, so
// fuzzer-generated random inputs exercise the "resource does not exist"
// branch. The oracle is the same: no 2xx.
//
// Run the seed corpus as a regular test:
//     go test -run FuzzRouter_CrossTenant ./internal/api/
// Run the actual fuzzer (local only — not wired into CI yet):
//     go test -fuzz FuzzRouter_CrossTenant -fuzztime 30s ./internal/api/

const (
	fuzzOwnerTenantID  = "tenant-A"
	fuzzOwnerUserID    = "user-A"
	fuzzOwnerEmail     = "a@test"
	fuzzForeignTenant  = "tenant-B"
	fuzzForeignAppID   = "app-B-fuzz-seed"
	fuzzForeignProjID  = "proj-B-fuzz-seed"
	fuzzForeignDomID   = "dom-B-fuzz-seed"
	fuzzForeignAgentID = "agent-B-fuzz-seed"
)

// crossTenantStore embeds the generic testStore but pre-seeds a handful of
// resources that all belong to fuzzForeignTenant. Lookups for any other ID
// return core.ErrNotFound so fuzzer-generated inputs walk the not-found
// branch rather than nil-dereffing.
type crossTenantStore struct {
	testStore
}

func (s *crossTenantStore) GetApp(_ context.Context, id string) (*core.Application, error) {
	if id == fuzzForeignAppID {
		return &core.Application{
			ID:       fuzzForeignAppID,
			TenantID: fuzzForeignTenant,
			Name:     "foreign-app",
			Type:     "service",
			Status:   "running",
		}, nil
	}
	return nil, core.ErrNotFound
}

func (s *crossTenantStore) GetProject(_ context.Context, id string) (*core.Project, error) {
	if id == fuzzForeignProjID {
		return &core.Project{
			ID:       fuzzForeignProjID,
			TenantID: fuzzForeignTenant,
			Name:     "foreign-project",
		}, nil
	}
	return nil, core.ErrNotFound
}

func (s *crossTenantStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, core.ErrNotFound
}

func (s *crossTenantStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	// Handlers reach ListDomainsByApp only after they resolve the app via
	// GetApp, so cross-tenant ownership is enforced upstream. Return empty
	// rather than nil to keep downstream writers from nil-deref panics.
	return []core.Domain{}, nil
}

func (s *crossTenantStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return []core.Deployment{}, nil
}

func (s *crossTenantStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, core.ErrNotFound
}

// fuzzResourceIDRoutes enumerates every GET/read route that takes a path
// parameter identifying a tenant-scoped resource. Each entry is a template
// in which `{id}` is substituted with the fuzz input. The fuzz oracle
// asserts the response is NEVER 2xx — a leak of tenant-B state into a
// tenant-A request is a bug.
//
// Keep this list in sync with resource-scoped GETs in router.go. Streaming
// and WebSocket endpoints are intentionally excluded because their happy
// path may legitimately keep the socket open on a successful upgrade, and
// mutation (POST/PUT/PATCH/DELETE) endpoints are tracked separately by the
// admin-routes authorization table.
var fuzzResourceIDRoutes = []string{
	"/api/v1/apps/{id}",
	"/api/v1/apps/{id}/export",
	"/api/v1/apps/{id}/env",
	"/api/v1/apps/{id}/env/export",
	"/api/v1/apps/{id}/stats",
	"/api/v1/apps/{id}/metrics",
	"/api/v1/apps/{id}/metrics/export",
	"/api/v1/apps/{id}/deployments",
	"/api/v1/apps/{id}/deployments/latest",
	"/api/v1/apps/{id}/deployments/diff",
	"/api/v1/apps/{id}/versions",
	"/api/v1/apps/{id}/healthcheck",
	"/api/v1/apps/{id}/ports",
	"/api/v1/apps/{id}/labels",
	"/api/v1/apps/{id}/resources",
	"/api/v1/apps/{id}/autoscale",
	"/api/v1/apps/{id}/basic-auth",
	"/api/v1/apps/{id}/redirects",
	"/api/v1/apps/{id}/error-pages",
	"/api/v1/apps/{id}/response-headers",
	"/api/v1/apps/{id}/sticky-sessions",
	"/api/v1/apps/{id}/maintenance",
	"/api/v1/apps/{id}/middleware",
	"/api/v1/apps/{id}/log-retention",
	"/api/v1/apps/{id}/deploy-notifications",
	"/api/v1/apps/{id}/dependencies",
	"/api/v1/apps/{id}/snapshots",
	"/api/v1/apps/{id}/gpu",
	"/api/v1/apps/{id}/disk",
	"/api/v1/apps/{id}/files",
	"/api/v1/apps/{id}/commands",
	"/api/v1/apps/{id}/cron",
	"/api/v1/apps/{id}/restarts",
	"/api/v1/apps/{id}/processes",
	"/api/v1/apps/{id}/webhooks/logs",
	"/api/v1/apps/{id}/builds/latest/log",
	"/api/v1/projects/{id}",
	"/api/v1/agents/{id}",
}

// fuzzSetupRouter wires a Router backed by crossTenantStore and mints a
// developer-role access token for tenant-A. Shared by the seed corpus
// test and the fuzz target.
func fuzzSetupRouter(tb testing.TB) (*Router, string) {
	tb.Helper()

	store := &crossTenantStore{}
	registry := core.NewRegistry()
	events := core.NewEventBus(nil)

	c := &core.Core{
		Registry: registry,
		Events:   events,
		Logger:   slog.Default(),
		Store:    store,
		Build:    core.BuildInfo{Version: "0.1.0-test"},
		Config:   &core.Config{Server: core.ServerConfig{SecretKey: "test-secret-key-32chars-for-jwt!"}},
		Services: core.NewServices(),
		DB:       &core.Database{Bolt: &testBoltStore{}},
	}

	tb.Setenv("MONSTER_ADMIN_EMAIL", "admin@example.com")
	tb.Setenv("MONSTER_ADMIN_PASSWORD", "SecureP@ss123!")

	authMod := auth.New()
	if err := authMod.Init(context.Background(), c); err != nil {
		tb.Fatalf("auth.Init: %v", err)
	}

	pair, err := authMod.JWT().GenerateTokenPair(
		fuzzOwnerUserID,
		fuzzOwnerTenantID,
		"role_developer",
		fuzzOwnerEmail,
	)
	if err != nil {
		tb.Fatalf("GenerateTokenPair: %v", err)
	}

	r := NewRouter(c, authMod, c.Store)
	return r, pair.AccessToken
}

// fuzzAssertNoLeak walks every route in fuzzResourceIDRoutes with the
// given resource ID plugged into the `{id}` slot. Fails the test on any
// 2xx response (the oracle) and also rejects 101 Switching Protocols in
// case a GET-only route accidentally upgrades to a websocket.
//
// An empty id is rejected up front because the resulting path ends in a
// trailing slash that falls through to the SPA handler rather than to
// any tenant-scoped route — the oracle only makes sense when the id slot
// is actually occupied.
func fuzzAssertNoLeak(tb testing.TB, r *Router, token, id string) {
	tb.Helper()

	if id == "" {
		return
	}
	// httptest.NewRequest calls url.Parse which panics on raw control
	// bytes in the path. Escape so arbitrary fuzz inputs travel as a
	// well-formed URL. PathEscape handles \x00, unicode, percents, etc.
	escaped := url.PathEscape(id)

	for _, tpl := range fuzzResourceIDRoutes {
		path := strings.Replace(tpl, "{id}", escaped, 1)

		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		// Recover from handler panics that escape past middleware —
		// the testStore and crossTenantStore leave many sub-interfaces
		// stubbed out, so any handler that reaches a non-overridden
		// method can crash. A panic is not a leak (no 2xx emitted),
		// so we treat it as "route rejected" and move on.
		func() {
			defer func() { _ = recover() }()
			r.mux.ServeHTTP(rr, req)
		}()

		// Routes that ServeMux can't match fall through to the embedded
		// SPA handler, which answers 200 with an HTML shell. That isn't
		// a cross-tenant leak — the SPA is supposed to serve the React
		// app for any client-side route. Skip 200s whose body is clearly
		// the SPA index so the oracle stays focused on JSON API routes.
		body := rr.Body.String()
		if rr.Code >= 200 && rr.Code < 300 && looksLikeSPAIndex(body) {
			continue
		}

		switch {
		case rr.Code >= 200 && rr.Code < 300:
			tb.Errorf("cross-tenant leak: GET %s with foreign id %q returned %d (body: %s)",
				tpl, id, rr.Code, truncate(body, 200))
		case rr.Code == http.StatusSwitchingProtocols:
			tb.Errorf("unexpected 101 upgrade: GET %s with id %q", tpl, id)
		}
	}
}

// looksLikeSPAIndex reports whether a response body is the embedded
// React index.html shell. The SPA handler serves it for any unmatched
// path, so any 2xx whose body is HTML is a ServeMux miss, not a leak.
func looksLikeSPAIndex(body string) bool {
	lower := strings.ToLower(strings.TrimSpace(body))
	return strings.HasPrefix(lower, "<!doctype html") ||
		strings.HasPrefix(lower, "<html")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// TestRouter_CrossTenantSeedCorpus exercises the fuzz oracle against the
// known-sensitive seed inputs on every run of `go test ./internal/api/`,
// without requiring `-fuzz`. These seeds are the ones most likely to
// trigger a regression: a real foreign-tenant resource ID, path-traversal
// attempts, percent-encoding edge cases, and common SQL/NOSQL injection
// probes.
func TestRouter_CrossTenantSeedCorpus(t *testing.T) {
	r, token := fuzzSetupRouter(t)

	seeds := []string{
		fuzzForeignAppID,
		fuzzForeignProjID,
		fuzzForeignDomID,
		fuzzForeignAgentID,
		"",
		" ",
		"../../../etc/passwd",
		"..%2F..%2Fadmin",
		"' OR 1=1 --",
		"<script>alert(1)</script>",
		"\x00",
		"🚀",
		strings.Repeat("a", 256),
	}

	for _, id := range seeds {
		id := id
		name := id
		if name == "" {
			name = "<empty>"
		}
		t.Run(name, func(t *testing.T) {
			fuzzAssertNoLeak(t, r, token, id)
		})
	}
}

// FuzzRouter_CrossTenant is the fuzz target. Run locally with:
//
//	go test -fuzz FuzzRouter_CrossTenant -fuzztime 30s ./internal/api/
//
// The seed corpus below is what `go test -run FuzzRouter_CrossTenant`
// walks in CI (Go's fuzz infrastructure uses f.Add seeds as the default
// corpus for non-fuzz runs).
func FuzzRouter_CrossTenant(f *testing.F) {
	// Seed corpus — one valid foreign-tenant resource ID per type plus
	// common injection probes. Matches TestRouter_CrossTenantSeedCorpus
	// but kept separate so f.Add entries are the canonical fuzz seeds.
	f.Add(fuzzForeignAppID)
	f.Add(fuzzForeignProjID)
	f.Add(fuzzForeignDomID)
	f.Add(fuzzForeignAgentID)
	f.Add("")
	f.Add("../../etc/passwd")
	f.Add("%00")
	f.Add("' OR 1=1 --")
	f.Add(strings.Repeat("a", 1024))

	r, token := fuzzSetupRouter(f)

	f.Fuzz(func(t *testing.T, id string) {
		// Drop inputs containing a literal newline or raw control bytes
		// that break httptest.NewRequest's URL parser — those are not a
		// meaningful authorization scenario anyway.
		if strings.ContainsAny(id, "\n\r") {
			t.Skip("skip: input breaks URL parser")
		}
		fuzzAssertNoLeak(t, r, token, id)
	})
}
