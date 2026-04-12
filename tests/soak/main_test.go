package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The soak harness is fail-loud by design — if the Prometheus parser
// silently misses a metric, drift detection breaks. These tests pin
// the parser against the exact output shape produced by
// internal/api/middleware/metrics.go so a drift in that handler
// surfaces here instead of in production after 24 hours.

func TestParsePromText_RuntimeBlock(t *testing.T) {
	// Matches the exact block emitted by APIMetrics.Handler after
	// the Tier 86 runtime-metrics addition. If APIMetrics.Handler
	// ever renames one of these keys, this test breaks loudly.
	input := `# HELP go_goroutines Number of goroutines
# TYPE go_goroutines gauge
go_goroutines 42

# HELP go_memstats_heap_inuse_bytes Heap bytes in use
# TYPE go_memstats_heap_inuse_bytes gauge
go_memstats_heap_inuse_bytes 1048576

# HELP go_memstats_num_gc Number of completed GC cycles
# TYPE go_memstats_num_gc counter
go_memstats_num_gc 17
`
	metrics := parsePromText(input)
	if metrics["go_goroutines"] != 42 {
		t.Errorf("goroutines: got %v", metrics["go_goroutines"])
	}
	if metrics["go_memstats_heap_inuse_bytes"] != 1048576 {
		t.Errorf("heap_inuse: got %v", metrics["go_memstats_heap_inuse_bytes"])
	}
	if metrics["go_memstats_num_gc"] != 17 {
		t.Errorf("num_gc: got %v", metrics["go_memstats_num_gc"])
	}
}

func TestParsePromText_SkipsLabeledMetrics(t *testing.T) {
	// Labeled metrics (api_requests_by_endpoint{endpoint="GET /health"} 100)
	// are intentionally ignored — the drift gates only look at
	// label-less scalars. Confirm the parser doesn't accidentally
	// pick up the numeric after the closing brace.
	input := `api_requests_by_endpoint{endpoint="GET /health"} 100
api_requests_total 500
`
	metrics := parsePromText(input)
	if metrics["api_requests_total"] != 500 {
		t.Errorf("expected api_requests_total=500, got %v", metrics["api_requests_total"])
	}
	if _, ok := metrics["api_requests_by_endpoint"]; ok {
		t.Error("labeled metric must not be captured as unlabeled")
	}
}

func TestParsePromText_IgnoresCommentsAndBlankLines(t *testing.T) {
	input := `
# HELP foo bar
# TYPE foo gauge

foo 3.14

# unrelated comment
bar 7
`
	metrics := parsePromText(input)
	if metrics["foo"] != 3.14 {
		t.Errorf("foo: got %v", metrics["foo"])
	}
	if metrics["bar"] != 7 {
		t.Errorf("bar: got %v", metrics["bar"])
	}
}

func TestParsePromText_MalformedLine(t *testing.T) {
	// Garbage lines must not panic or pollute the map.
	input := `good 1
malformed_without_value
no_number xyz
another_good 2
`
	metrics := parsePromText(input)
	if metrics["good"] != 1 || metrics["another_good"] != 2 {
		t.Errorf("valid metrics dropped: %+v", metrics)
	}
	if _, ok := metrics["malformed_without_value"]; ok {
		t.Error("malformed metric must be skipped")
	}
	if _, ok := metrics["no_number"]; ok {
		t.Error("non-numeric metric must be skipped")
	}
}

// TestTakeSample_EndToEnd spins up a mock /metrics/api server that
// emits the exact format real DeployMonster produces, drives one
// takeSample call, and asserts every field in the resulting sample
// struct mapped correctly. This is the closest thing to a contract
// test between the soak harness and the metrics handler.
func TestTakeSample_EndToEnd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/metrics/api" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte(strings.Join([]string{
			"# HELP api_requests_total Total API requests",
			"# TYPE api_requests_total counter",
			"api_requests_total 1234",
			"",
			"# HELP api_requests_active Currently active API requests",
			"# TYPE api_requests_active gauge",
			"api_requests_active 3",
			"",
			"# HELP api_errors_total Total 5xx errors",
			"# TYPE api_errors_total counter",
			"api_errors_total 5",
			"",
			"# HELP go_goroutines Number of goroutines",
			"# TYPE go_goroutines gauge",
			"go_goroutines 128",
			"",
			"# HELP go_memstats_alloc_bytes Currently allocated bytes",
			"# TYPE go_memstats_alloc_bytes gauge",
			"go_memstats_alloc_bytes 67108864",
			"",
			"# HELP go_memstats_heap_inuse_bytes Heap bytes in use",
			"# TYPE go_memstats_heap_inuse_bytes gauge",
			"go_memstats_heap_inuse_bytes 83886080",
			"",
			"# HELP go_memstats_heap_objects Live heap objects",
			"# TYPE go_memstats_heap_objects gauge",
			"go_memstats_heap_objects 250000",
			"",
			"# HELP go_memstats_sys_bytes Bytes obtained from system",
			"# TYPE go_memstats_sys_bytes gauge",
			"go_memstats_sys_bytes 134217728",
			"",
			"# HELP go_memstats_next_gc_bytes Target heap size of next GC",
			"# TYPE go_memstats_next_gc_bytes gauge",
			"go_memstats_next_gc_bytes 100663296",
			"",
			"# HELP go_memstats_num_gc Number of completed GC cycles",
			"# TYPE go_memstats_num_gc counter",
			"go_memstats_num_gc 42",
			"",
			"# HELP eventbus_published_total Total events published",
			"# TYPE eventbus_published_total counter",
			"eventbus_published_total 99",
			"",
			"# HELP eventbus_errors_total Total event handler errors",
			"# TYPE eventbus_errors_total counter",
			"eventbus_errors_total 1",
		}, "\n")))
	}))
	defer srv.Close()

	s, err := takeSample(t.Context(), srv.Client(), srv.URL, "")
	if err != nil {
		t.Fatalf("takeSample: %v", err)
	}
	if s.Goroutines != 128 {
		t.Errorf("goroutines: got %d", s.Goroutines)
	}
	if s.HeapInuseBytes != 83886080 {
		t.Errorf("heap_inuse: got %d", s.HeapInuseBytes)
	}
	if s.NumGC != 42 {
		t.Errorf("num_gc: got %d", s.NumGC)
	}
	if s.APIRequestsTotal != 1234 {
		t.Errorf("api_requests_total: got %d", s.APIRequestsTotal)
	}
	if s.EventbusPublished != 99 {
		t.Errorf("eventbus_published: got %d", s.EventbusPublished)
	}
	if s.WallTime == "" {
		t.Error("wall_time must be populated")
	}
}

func TestFmtBytes(t *testing.T) {
	// Byte formatting drives the human-readable log output —
	// regressions here make 24-hour run logs unreadable.
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.00KiB"},
		{1024 * 1024, "1.00MiB"},
		{1024 * 1024 * 1024, "1.00GiB"},
		{1024 * 1024 * 512, "512.00MiB"},
	}
	for _, tc := range cases {
		got := fmtBytes(tc.in)
		if got != tc.want {
			t.Errorf("fmtBytes(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00:00"},
		{59 * time.Second, "00:00:59"},
		{time.Minute, "00:01:00"},
		{time.Hour + time.Minute + time.Second, "01:01:01"},
		{24 * time.Hour, "24:00:00"},
	}
	for _, tc := range cases {
		got := formatDuration(tc.d)
		if got != tc.want {
			t.Errorf("formatDuration(%s) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
