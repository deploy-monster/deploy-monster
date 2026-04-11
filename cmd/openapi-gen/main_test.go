package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTempFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
	return p
}

func TestParseRouter_ExtractsHandleAndHandleFunc(t *testing.T) {
	dir := t.TempDir()
	router := writeTempFile(t, dir, "router.go", `
package api

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("GET /health", r.handleHealth)
	r.mux.HandleFunc("GET /api/v1/health", r.handleHealth)
	r.mux.Handle("GET /api/v1/apps", protected(http.HandlerFunc(appH.List)))
	r.mux.Handle("POST /api/v1/apps", protected(http.HandlerFunc(appH.Create)))
	r.mux.Handle("DELETE /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Delete)))
	r.mux.Handle("PATCH /api/v1/apps/{id}", protected(http.HandlerFunc(appH.Update)))
}
`)
	set, err := parseRouter(router)
	if err != nil {
		t.Fatalf("parseRouter: %v", err)
	}
	wantKeys := []string{
		"GET /health",
		"GET /api/v1/health",
		"GET /api/v1/apps",
		"POST /api/v1/apps",
		"DELETE /api/v1/apps/{id}",
		"PATCH /api/v1/apps/{id}",
	}
	for _, k := range wantKeys {
		if !set.has(k) {
			t.Errorf("missing route %q", k)
		}
	}
	if got, want := len(set), len(wantKeys); got != want {
		t.Errorf("route count = %d, want %d", got, want)
	}
}

func TestParseRouter_ExcludesPprofAndMetrics(t *testing.T) {
	dir := t.TempDir()
	router := writeTempFile(t, dir, "router.go", `
package api

func (r *Router) registerRoutes() {
	r.mux.Handle("GET /debug/pprof/heap", nil)
	r.mux.Handle("GET /debug/pprof/goroutine", nil)
	r.mux.Handle("GET /metrics/api", nil)
	r.mux.Handle("GET /api/v1/apps", nil)
}
`)
	set, _ := parseRouter(router)
	if set.has("GET /debug/pprof/heap") {
		t.Error("pprof route should be excluded")
	}
	if set.has("GET /metrics/api") {
		t.Error("/metrics/api should be excluded")
	}
	if !set.has("GET /api/v1/apps") {
		t.Error("expected /api/v1/apps to survive filter")
	}
}

func TestParseRouter_IgnoresUnknownMethod(t *testing.T) {
	dir := t.TempDir()
	router := writeTempFile(t, dir, "router.go", `
package api

func (r *Router) registerRoutes() {
	// PROPFIND isn't in routeMethods — the tool must not claim it registered a route
	r.mux.Handle("PROPFIND /api/v1/apps", nil)
	r.mux.Handle("GET /api/v1/apps", nil)
}
`)
	set, _ := parseRouter(router)
	if set.has("PROPFIND /api/v1/apps") {
		t.Error("unknown method should be ignored")
	}
	if !set.has("GET /api/v1/apps") {
		t.Error("known method should be extracted")
	}
}

func TestParseSpec_ExtractsPathsAndMethods(t *testing.T) {
	dir := t.TempDir()
	spec := writeTempFile(t, dir, "openapi.yaml", `
openapi: 3.0.3
info:
  title: test
  version: 1.0.0
paths:
  /api/v1/apps:
    get:
      operationId: listApps
    post:
      operationId: createApp
  /api/v1/apps/{id}:
    get:
      operationId: getApp
    parameters:
      - name: id
        in: path
`)
	set, err := parseSpec(spec)
	if err != nil {
		t.Fatalf("parseSpec: %v", err)
	}
	for _, k := range []string{"GET /api/v1/apps", "POST /api/v1/apps", "GET /api/v1/apps/{id}"} {
		if !set.has(k) {
			t.Errorf("missing spec entry %q", k)
		}
	}
	// `parameters` is not a method — must not be extracted.
	if set.has("PARAMETERS /api/v1/apps/{id}") {
		t.Error("parameters entry should not be treated as a method")
	}
	if got, want := len(set), 3; got != want {
		t.Errorf("spec route count = %d, want %d", got, want)
	}
}

func TestLoadAllowlist_IgnoresBlankAndComments(t *testing.T) {
	dir := t.TempDir()
	f := writeTempFile(t, dir, "allow.txt", `
# header
# another comment

GET /api/v1/foo
POST /api/v1/bar

# trailing comment
`)
	al, err := loadAllowlist(f)
	if err != nil {
		t.Fatalf("loadAllowlist: %v", err)
	}
	if got, want := len(al), 2; got != want {
		t.Errorf("size = %d, want %d", got, want)
	}
	if _, ok := al["GET /api/v1/foo"]; !ok {
		t.Error("missing foo")
	}
	if _, ok := al["POST /api/v1/bar"]; !ok {
		t.Error("missing bar")
	}
}

func TestLoadAllowlist_MissingFileIsEmpty(t *testing.T) {
	al, err := loadAllowlist(filepath.Join(t.TempDir(), "nope.txt"))
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if len(al) != 0 {
		t.Errorf("expected empty allowlist, got %d entries", len(al))
	}
}

func TestWriteAllowlist_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "sub", "allow.txt")
	codeNotSpec := []string{"GET /api/v1/new-thing", "POST /api/v1/other"}
	specNotCode := []string{"DELETE /api/v1/dead"}
	if err := writeAllowlist(path, codeNotSpec, specNotCode); err != nil {
		t.Fatalf("writeAllowlist: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	text := string(body)
	for _, want := range append(codeNotSpec, specNotCode...) {
		if !strings.Contains(text, want) {
			t.Errorf("allowlist missing %q", want)
		}
	}
	// Round-trip parse.
	al, err := loadAllowlist(path)
	if err != nil {
		t.Fatalf("round-trip load: %v", err)
	}
	if got, want := len(al), 3; got != want {
		t.Errorf("round-trip size = %d, want %d", got, want)
	}
}

func TestFilterOut(t *testing.T) {
	al := map[string]struct{}{"GET /a": {}, "POST /b": {}}
	got := filterOut([]string{"GET /a", "GET /c", "POST /b", "DELETE /d"}, al)
	if len(got) != 2 {
		t.Errorf("want 2, got %v", got)
	}
	for _, k := range got {
		if _, ok := al[k]; ok {
			t.Errorf("%q should have been filtered out", k)
		}
	}
}

func TestMergeSets(t *testing.T) {
	m := mergeSets([]string{"a", "b"}, []string{"b", "c"})
	if len(m) != 3 {
		t.Errorf("want 3 unique entries, got %d", len(m))
	}
	for _, k := range []string{"a", "b", "c"} {
		if _, ok := m[k]; !ok {
			t.Errorf("missing %q", k)
		}
	}
}

func TestRouteSetKeysSorted(t *testing.T) {
	s := routeSet{}
	s.add(route{Method: "POST", Path: "/b"})
	s.add(route{Method: "GET", Path: "/a"})
	s.add(route{Method: "DELETE", Path: "/c"})
	got := s.keys()
	want := []string{"DELETE /c", "GET /a", "POST /b"}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
