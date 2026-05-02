package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// Tests for the baseline comparison gate used by Phase 5.3.5. The gate
// protects a committed baseline from a ≥10% regression on either the
// per-endpoint RPS or the per-endpoint p95. These tests pin the
// semantics at the boundary so a future edit to compareReports cannot
// quietly loosen the guarantee.

func TestCompareReports_NoRegression(t *testing.T) {
	base := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1000, P95Microseconds: 5000},
		},
	}
	cur := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1050, P95Microseconds: 4800},
		},
	}
	if regs := compareReports(base, cur, 0.10); len(regs) != 0 {
		t.Fatalf("expected no regressions, got %+v", regs)
	}
}

func TestCompareReports_ThroughputRegression(t *testing.T) {
	base := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1000, P95Microseconds: 5000},
		},
	}
	// 11% drop in RPS — must flag with the default 10% threshold.
	cur := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 890, P95Microseconds: 5000},
		},
	}
	regs := compareReports(base, cur, 0.10)
	if len(regs) != 1 {
		t.Fatalf("expected 1 regression, got %d: %+v", len(regs), regs)
	}
	if regs[0].Endpoint != "GET /health" {
		t.Errorf("wrong endpoint: %s", regs[0].Endpoint)
	}
}

func TestCompareReports_LatencyRegression(t *testing.T) {
	base := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1000, P95Microseconds: 5000},
		},
	}
	// 12% growth in p95 — must flag.
	cur := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1000, P95Microseconds: 5600},
		},
	}
	regs := compareReports(base, cur, 0.10)
	if len(regs) != 1 {
		t.Fatalf("expected 1 regression, got %d: %+v", len(regs), regs)
	}
}

func TestCompareReports_AtThreshold(t *testing.T) {
	// Exactly at the boundary — must NOT trigger. Only strictly worse
	// than (baseline * (1 ± threshold)) is a regression. This guarantees
	// that a run that reproduces the baseline to the last µs passes.
	base := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1000, P95Microseconds: 5000},
		},
	}
	cur := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 900, P95Microseconds: 5500},
		},
	}
	if regs := compareReports(base, cur, 0.10); len(regs) != 0 {
		t.Fatalf("exact-threshold run must not regress, got %+v", regs)
	}
}

func TestCompareReports_MissingEndpoint(t *testing.T) {
	// Baseline has two endpoints; current run only has one. The missing
	// endpoint is a regression — a production path has gone dark.
	base := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health":   {RequestsPerSec: 1000, P95Microseconds: 5000},
			"GET /api/v1/x": {RequestsPerSec: 500, P95Microseconds: 10000},
		},
	}
	cur := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1000, P95Microseconds: 5000},
		},
	}
	regs := compareReports(base, cur, 0.10)
	if len(regs) != 1 || regs[0].Endpoint != "GET /api/v1/x" {
		t.Fatalf("expected 1 regression on /api/v1/x, got %+v", regs)
	}
}

func TestCompareReports_ExtraEndpoint(t *testing.T) {
	// Current run has an endpoint that baseline does not know about —
	// this is new surface area, not a regression. It must be ignored.
	base := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1000, P95Microseconds: 5000},
		},
	}
	cur := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health":   {RequestsPerSec: 1000, P95Microseconds: 5000},
			"GET /api/v1/y": {RequestsPerSec: 50, P95Microseconds: 99999},
		},
	}
	if regs := compareReports(base, cur, 0.10); len(regs) != 0 {
		t.Fatalf("new endpoints must be ignored, got %+v", regs)
	}
}

func TestCompareReports_BothAxesRegress(t *testing.T) {
	// When both RPS and p95 regress on the same endpoint, we emit TWO
	// regression entries so the operator sees the full picture rather
	// than only the first-detected axis.
	base := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 1000, P95Microseconds: 5000},
		},
	}
	cur := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /health": {RequestsPerSec: 800, P95Microseconds: 6000},
		},
	}
	regs := compareReports(base, cur, 0.10)
	if len(regs) != 2 {
		t.Fatalf("expected 2 regressions (rps + p95), got %d: %+v", len(regs), regs)
	}
}

func TestReadWriteReport_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.json")

	want := &loadtestReport{
		DurationSeconds: 10,
		Concurrency:     5,
		TotalRequests:   1234,
		Successful:      1200,
		Errors:          34,
		RequestsPerSec:  123.4,
		Endpoints: map[string]endpointStats{
			"GET /health": {Total: 100, Successful: 100, P50Microseconds: 500, P95Microseconds: 2000, P99Microseconds: 5000, RequestsPerSec: 10},
		},
	}
	if err := writeReport(path, want); err != nil {
		t.Fatalf("writeReport: %v", err)
	}
	got, err := readReport(path)
	if err != nil {
		t.Fatalf("readReport: %v", err)
	}
	if got.Concurrency != want.Concurrency || got.Endpoints["GET /health"].P95Microseconds != 2000 {
		t.Errorf("roundtrip mismatch: got %+v", got)
	}
}

func TestReportErrors_CatchesEndpointErrors(t *testing.T) {
	r := &loadtestReport{
		TotalRequests: 10,
		Successful:    8,
		Errors:        2,
		Endpoints: map[string]endpointStats{
			"GET /health": {Total: 8, Successful: 8},
			"GET /login":  {Total: 2, Errors: 2},
		},
	}
	errs := reportErrors(r)
	if len(errs) != 2 {
		t.Fatalf("expected total + endpoint errors, got %+v", errs)
	}
}

func TestReportErrors_CatchesZeroRequestEndpoint(t *testing.T) {
	r := &loadtestReport{
		TotalRequests: 1,
		Successful:    1,
		Endpoints: map[string]endpointStats{
			"GET /health": {Total: 1, Successful: 1},
			"GET /login":  {},
		},
	}
	errs := reportErrors(r)
	if len(errs) != 1 || errs[0] != "GET /login total=0" {
		t.Fatalf("expected zero-request endpoint error, got %+v", errs)
	}
}

func TestReport_JSON_Stable(t *testing.T) {
	// Lock the on-disk schema. Baseline files are committed to the repo
	// so an accidental field rename would silently break every historical
	// baseline. If this test fails, bump the baseline format explicitly
	// and regenerate all committed baselines.
	r := &loadtestReport{
		DurationSeconds: 1,
		Concurrency:     2,
		TotalRequests:   3,
		Successful:      3,
		Errors:          0,
		RequestsPerSec:  3,
		Endpoints: map[string]endpointStats{
			"GET /x": {Total: 3, Successful: 3, RequestsPerSec: 3, P50Microseconds: 1, P95Microseconds: 2, P99Microseconds: 3},
		},
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"duration_seconds"`, `"concurrency"`, `"total_requests"`,
		`"requests_per_second"`, `"endpoints"`,
		`"p50_us"`, `"p95_us"`, `"p99_us"`,
	} {
		if !contains(data, field) {
			t.Errorf("schema field %s missing from JSON: %s", field, string(data))
		}
	}
}

func contains(haystack []byte, needle string) bool {
	return len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(haystack []byte, needle string) int {
	nb := []byte(needle)
	for i := 0; i+len(nb) <= len(haystack); i++ {
		match := true
		for j := range nb {
			if haystack[i+j] != nb[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

// Ensure a temp-written baseline file survives a full readReport +
// compareReports loop against itself (no false-positive regressions).
func TestReadReport_SelfCompare_NoRegression(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "self.json")
	r := &loadtestReport{
		Endpoints: map[string]endpointStats{
			"GET /a": {RequestsPerSec: 500, P95Microseconds: 1000},
			"GET /b": {RequestsPerSec: 50, P95Microseconds: 50000},
		},
	}
	if err := writeReport(path, r); err != nil {
		t.Fatal(err)
	}
	loaded, err := readReport(path)
	if err != nil {
		t.Fatal(err)
	}
	if regs := compareReports(loaded, r, 0.10); len(regs) != 0 {
		t.Errorf("self-compare must not regress, got %+v", regs)
	}
}
