// Command openapi-gen audits docs/openapi.yaml against the routes
// registered in internal/api/*.go and fails on drift.
//
// It does not emit a full spec — that is a much larger job that would
// have to introspect handler types, request/response schemas, and
// auth annotations. This tool answers a narrower and more valuable
// question: does every route in code have a matching entry in the
// spec, and does every spec entry still correspond to real code?
//
// Usage:
//
//	go run ./cmd/openapi-gen
//	go run ./cmd/openapi-gen -router=internal/api -spec=docs/openapi.yaml
//	go run ./cmd/openapi-gen -bootstrap         # rewrite the allowlist with the current gap
//
// Exit codes:
//
//	0 — spec and code agree (modulo the allowlist).
//	2 — drift detected.
//	3 — usage / I/O error.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const defaultAllowlistPath = "docs/openapi-drift-allowlist.txt"

// routeMethods are the HTTP methods the router actually uses.
// Anything else in router.go should be treated as a false match on a
// string that happens to sit inside a Handle() call.
var routeMethods = map[string]struct{}{
	"GET": {}, "POST": {}, "PUT": {}, "PATCH": {},
	"DELETE": {}, "HEAD": {}, "OPTIONS": {},
}

// routeRegex matches r.mux.Handle("METHOD /path", ...) and
// r.mux.HandleFunc("METHOD /path", ...) calls.
var routeRegex = regexp.MustCompile(
	`r\.mux\.Handle(?:Func)?\(\s*"([A-Z]+)\s+([^"]+)"`,
)

// exclusionPrefixes are path prefixes that are not part of the API
// surface and should never appear in the spec.
var exclusionPrefixes = []string{
	"/debug/pprof",
	"/metrics", // raw Prometheus exposition, not an API
}

type route struct {
	Method string
	Path   string
}

func (r route) Key() string { return r.Method + " " + r.Path }

type routeSet map[string]route

func (s routeSet) add(r route)         { s[r.Key()] = r }
func (s routeSet) has(key string) bool { _, ok := s[key]; return ok }
func (s routeSet) keys() []string {
	out := make([]string, 0, len(s))
	for k := range s {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func main() {
	routerPath := flag.String("router", "internal/api", "path to router.go or an API package directory")
	specPath := flag.String("spec", "docs/openapi.yaml", "path to openapi.yaml")
	allowlistPath := flag.String("allowlist", defaultAllowlistPath, "path to drift-allowlist file")
	bootstrap := flag.Bool("bootstrap", false, "rewrite the allowlist from the current gap (do not run in CI)")
	verbose := flag.Bool("v", false, "print matched routes too")
	flag.Parse()

	code, err := parseRouter(*routerPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: parse router: %v\n", err)
		os.Exit(3)
	}
	spec, err := parseSpec(*specPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: parse spec: %v\n", err)
		os.Exit(3)
	}

	// Routes in code that are not in spec.
	var codeNotSpec []string
	for _, k := range code.keys() {
		if !spec.has(k) {
			codeNotSpec = append(codeNotSpec, k)
		}
	}
	// Routes in spec that are not in code (dead documentation).
	var specNotCode []string
	for _, k := range spec.keys() {
		if !code.has(k) {
			specNotCode = append(specNotCode, k)
		}
	}

	allowlist, err := loadAllowlist(*allowlistPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "openapi-gen: load allowlist: %v\n", err)
		os.Exit(3)
	}

	if *bootstrap {
		if err := writeAllowlist(*allowlistPath, codeNotSpec, specNotCode); err != nil {
			fmt.Fprintf(os.Stderr, "openapi-gen: write allowlist: %v\n", err)
			os.Exit(3)
		}
		fmt.Printf("openapi-gen: bootstrapped %s with %d code-only routes and %d spec-only routes\n",
			*allowlistPath, len(codeNotSpec), len(specNotCode))
		os.Exit(0)
	}

	// Filter out allowlisted drift.
	newCodeNotSpec := filterOut(codeNotSpec, allowlist)
	newSpecNotCode := filterOut(specNotCode, allowlist)

	// Check for stale allowlist entries (listed but no longer drifted).
	drifted := mergeSets(codeNotSpec, specNotCode)
	var staleAllowlist []string
	for a := range allowlist {
		if _, ok := drifted[a]; !ok {
			staleAllowlist = append(staleAllowlist, a)
		}
	}
	sort.Strings(staleAllowlist)

	// Report.
	fmt.Printf("openapi-gen: %d routes in code, %d routes in spec, allowlist=%d\n",
		len(code), len(spec), len(allowlist))

	if *verbose {
		fmt.Println("\n== routes in code ==")
		for _, k := range code.keys() {
			fmt.Println("  " + k)
		}
	}

	fail := false

	if len(newCodeNotSpec) > 0 {
		fmt.Printf("\nFAIL: %d route(s) in code but not in spec (and not allowlisted):\n", len(newCodeNotSpec))
		for _, k := range newCodeNotSpec {
			fmt.Println("  + " + k)
		}
		fail = true
	}
	if len(newSpecNotCode) > 0 {
		fmt.Printf("\nFAIL: %d route(s) in spec but not in code (dead documentation, and not allowlisted):\n", len(newSpecNotCode))
		for _, k := range newSpecNotCode {
			fmt.Println("  - " + k)
		}
		fail = true
	}
	if len(staleAllowlist) > 0 {
		fmt.Printf("\nFAIL: %d stale allowlist entry/entries (drift has been closed, remove them):\n", len(staleAllowlist))
		for _, k := range staleAllowlist {
			fmt.Println("  ! " + k)
		}
		fail = true
	}

	if fail {
		fmt.Printf("\nopenapi-gen: drift detected. Fix the spec at %s or update %s.\n",
			*specPath, *allowlistPath)
		os.Exit(2)
	}

	fmt.Printf("openapi-gen: OK — %d routes match (allowlist drift: %d code-only, %d spec-only)\n",
		len(code)-countInAllowlist(codeNotSpec, allowlist),
		countInAllowlist(codeNotSpec, allowlist),
		countInAllowlist(specNotCode, allowlist),
	)
}

// parseRouter extracts every routeMethod+path registered in the router source.
func parseRouter(path string) (routeSet, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	set := routeSet{}
	if !stat.IsDir() {
		if err := parseRouterFile(path, set); err != nil {
			return nil, err
		}
		return set, nil
	}

	err = filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			return parseRouterFile(p, set)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return set, nil
}

func parseRouterFile(path string, set routeSet) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, m := range routeRegex.FindAllSubmatch(data, -1) {
		method := string(m[1])
		p := string(m[2])
		if _, ok := routeMethods[method]; !ok {
			continue
		}
		if excluded(p) {
			continue
		}
		set.add(route{Method: method, Path: p})
	}
	return nil
}

// parseSpec extracts every path+method pair from the OpenAPI document.
func parseSpec(path string) (routeSet, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("yaml parse: %w", err)
	}
	set := routeSet{}
	for p, methods := range doc.Paths {
		if excluded(p) {
			continue
		}
		for m := range methods {
			upper := strings.ToUpper(m)
			if _, ok := routeMethods[upper]; !ok {
				continue // skip parameters, summary, etc.
			}
			set.add(route{Method: upper, Path: p})
		}
	}
	return set, nil
}

func excluded(path string) bool {
	for _, p := range exclusionPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// loadAllowlist reads a newline-delimited file where each line is a
// route key ("GET /api/v1/apps/{id}"). Blank lines and `# comment`
// lines are ignored.
func loadAllowlist(path string) (map[string]struct{}, error) {
	out := map[string]struct{}{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = struct{}{}
	}
	return out, sc.Err()
}

// writeAllowlist rewrites the allowlist file with the given drift
// entries, sorted and commented.
func writeAllowlist(path string, codeNotSpec, specNotCode []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf strings.Builder
	buf.WriteString("# openapi-gen drift allowlist\n")
	buf.WriteString("# One route per line as \"METHOD /path\".\n")
	buf.WriteString("# Regenerate with: go run ./cmd/openapi-gen -bootstrap\n")
	buf.WriteString("#\n")
	buf.WriteString("# Anything listed here is known-drifted between router.go and openapi.yaml\n")
	buf.WriteString("# and is tolerated in CI. The goal is for this file to shrink over time — every\n")
	buf.WriteString("# entry represents either an undocumented route (fix the spec) or a documented\n")
	buf.WriteString("# route that has been removed from code (fix the spec).\n")
	buf.WriteString("#\n")
	buf.WriteString("# A stale allowlist entry (one that no longer corresponds to real drift) fails CI,\n")
	buf.WriteString("# so the file cannot rot silently.\n")
	buf.WriteString("\n")
	if len(codeNotSpec) > 0 {
		buf.WriteString("# --- Routes present in code but missing from openapi.yaml ---\n")
		sorted := append([]string(nil), codeNotSpec...)
		sort.Strings(sorted)
		for _, k := range sorted {
			buf.WriteString(k + "\n")
		}
	}
	if len(specNotCode) > 0 {
		buf.WriteString("\n# --- Routes documented in openapi.yaml but absent from code ---\n")
		sorted := append([]string(nil), specNotCode...)
		sort.Strings(sorted)
		for _, k := range sorted {
			buf.WriteString(k + "\n")
		}
	}
	return os.WriteFile(path, []byte(buf.String()), 0o644)
}

func filterOut(list []string, allowlist map[string]struct{}) []string {
	var out []string
	for _, x := range list {
		if _, ok := allowlist[x]; !ok {
			out = append(out, x)
		}
	}
	return out
}

func mergeSets(a, b []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, x := range a {
		out[x] = struct{}{}
	}
	for _, x := range b {
		out[x] = struct{}{}
	}
	return out
}

func countInAllowlist(list []string, allowlist map[string]struct{}) int {
	n := 0
	for _, x := range list {
		if _, ok := allowlist[x]; ok {
			n++
		}
	}
	return n
}
