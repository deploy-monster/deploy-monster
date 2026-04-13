// Package main is the DeployMonster soak-test harness (Phase 5.3.6).
//
// A soak test answers one question: does the platform hold its shape
// over hours of sustained load, or does it slowly drift? The three
// drift modes this harness watches for are:
//
//  1. Goroutine leak — go_goroutines grows without bound. A healthy
//     steady-state API server holds a roughly constant count after
//     warmup. Linear or near-linear growth is a leak.
//
//  2. Heap climb — go_memstats_heap_inuse_bytes grows monotonically
//     and the GC cannot recover it. A healthy server oscillates with
//     GC cycles within a bounded envelope.
//
//  3. Audit-log / DB bloat — the SQLite file on disk grows without
//     bound during a read-only workload. Indicates a module is
//     writing audit rows per-request even for reads.
//
// The harness drives a low-intensity HTTP load (default: 2 workers,
// no artificial sleep, which is ~10% of the 20-worker peak used by
// the regression gate) against a running DeployMonster instance,
// samples /metrics/api at a fixed interval, and writes a per-sample
// JSON line + a final summary to disk.
//
// Usage:
//
//	# Full 24-hour run at 10% peak:
//	go run ./tests/soak \
//	    -url http://localhost:8888 \
//	    -duration 24h \
//	    -concurrency 2 \
//	    -sample-interval 1m \
//	    -out soak-results.json
//
//	# CI smoke test (5 minutes, 10s sampling):
//	go run ./tests/soak \
//	    -url http://localhost:8888 \
//	    -duration 5m \
//	    -sample-interval 10s \
//	    -out soak-smoke.json
//
// Exit codes:
//
//	0  — run completed and all drift gates passed
//	1  — infrastructure failure (server unreachable, metrics unparseable)
//	2  — drift gate tripped (goroutine leak / heap climb / DB bloat)
//
// Drift gates are deliberately conservative: the goal is to catch
// slow leaks that manual smoke testing misses, not to chase minor
// GC-cycle oscillation. Default thresholds:
//
//   - goroutine leak: final goroutine count > 1.5 × post-warmup count
//   - heap climb: final heap_inuse > 2 × post-warmup heap_inuse
//   - DB bloat: final file size > 2 × post-warmup file size
//
// Warmup is a fixed fraction of the total run (default 10%), so a
// 5-minute smoke run uses the first 30 seconds as warmup and a
// 24-hour run uses the first ~2.4 hours.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	baseURL := flag.String("url", "http://localhost:8888", "Base URL of the DeployMonster API")
	duration := flag.Duration("duration", 24*time.Hour, "Total soak duration")
	concurrency := flag.Int("concurrency", 2, "Number of concurrent workers driving load (~10% of peak)")
	sampleInterval := flag.Duration("sample-interval", 1*time.Minute, "Sample /metrics/api every N")
	warmupFrac := flag.Float64("warmup-fraction", 0.10, "Fraction of total duration treated as warmup before drift gates engage")
	goroutineMultiplier := flag.Float64("goroutine-multiplier", 1.5, "Fail if final goroutines > multiplier * post-warmup goroutines")
	heapMultiplier := flag.Float64("heap-multiplier", 2.0, "Fail if final heap_inuse > multiplier * post-warmup heap_inuse")
	dbFile := flag.String("db-file", "", "Optional path to SQLite DB file — tracked for bloat detection")
	dbMultiplier := flag.Float64("db-multiplier", 2.0, "Fail if final DB size > multiplier * post-warmup DB size")
	outPath := flag.String("out", "soak-results.json", "Final summary JSON path")
	tracePath := flag.String("trace", "", "Optional JSONL trace path (one line per sample)")
	flag.Parse()

	log := func(format string, a ...any) {
		fmt.Printf("[soak %s] %s\n", time.Now().Format("15:04:05"), fmt.Sprintf(format, a...))
	}

	log("starting soak test: url=%s duration=%s concurrency=%d interval=%s",
		*baseURL, *duration, *concurrency, *sampleInterval)

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	// Load generator — keeps the server warm at roughly 10% of peak.
	// Hits the same endpoints as the regression baseline so counters
	// accumulate on the code paths that matter. A tight loop with no
	// artificial sleep is fine at concurrency=2; the bottleneck is
	// request RTT, not spin.
	var (
		loadWG      sync.WaitGroup
		loadStopped atomic.Bool
		totalReqs   atomic.Int64
		totalErrs   atomic.Int64
	)
	endpoints := []string{
		"/health",
		"/api/v1/health",
		"/api/v1/marketplace",
		"/api/v1/openapi.json",
		"/metrics/api",
	}
	client := &http.Client{Timeout: 10 * time.Second}
	for i := 0; i < *concurrency; i++ {
		loadWG.Add(1)
		go func(workerID int) {
			defer loadWG.Done()
			j := workerID
			for {
				if loadStopped.Load() {
					return
				}
				path := endpoints[j%len(endpoints)]
				j++
				req, _ := http.NewRequestWithContext(ctx, "GET", *baseURL+path, nil)
				resp, err := client.Do(req)
				totalReqs.Add(1)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					totalErrs.Add(1)
					continue
				}
				defer resp.Body.Close()
				_, _ = io.Copy(io.Discard, resp.Body)
				if resp.StatusCode >= 400 {
					totalErrs.Add(1)
				}
			}
		}(i)
	}

	// Sample loop — read /metrics/api every sampleInterval, parse the
	// Prometheus text, and append a sample row to in-memory history +
	// optional JSONL trace file.
	var (
		samples     []sample
		traceWriter *bufio.Writer
	)
	if *tracePath != "" {
		f, err := os.Create(*tracePath)
		if err != nil {
			log("failed to open trace file %s: %v", *tracePath, err)
			os.Exit(1)
		}
		defer f.Close()
		traceWriter = bufio.NewWriter(f)
		defer traceWriter.Flush()
	}

	startTime := time.Now()
	ticker := time.NewTicker(*sampleInterval)
	defer ticker.Stop()

	// First sample immediately — it becomes t0.
	if s, err := takeSample(ctx, client, *baseURL, *dbFile); err == nil {
		s.ElapsedSeconds = 0
		samples = append(samples, s)
		writeTrace(traceWriter, s)
		log("sample 0: goroutines=%d heap_inuse=%s num_gc=%d db=%s",
			s.Goroutines, fmtBytes(s.HeapInuseBytes), s.NumGC, fmtBytes(s.DBFileBytes))
	} else {
		log("initial sample failed: %v", err)
		os.Exit(1)
	}

sampleLoop:
	for {
		select {
		case <-ctx.Done():
			break sampleLoop
		case <-ticker.C:
			s, err := takeSample(ctx, client, *baseURL, *dbFile)
			if err != nil {
				log("sample error: %v (continuing)", err)
				continue
			}
			s.ElapsedSeconds = time.Since(startTime).Seconds()
			samples = append(samples, s)
			writeTrace(traceWriter, s)
			log("sample %d @ %s: goroutines=%d heap_inuse=%s num_gc=%d reqs=%d errs=%d",
				len(samples)-1, formatDuration(time.Since(startTime)),
				s.Goroutines, fmtBytes(s.HeapInuseBytes), s.NumGC,
				totalReqs.Load(), totalErrs.Load())
		}
	}

	// Stop the load generator. Any in-flight requests were already
	// canceled by the parent ctx timeout; loadWG just drains goroutines.
	loadStopped.Store(true)
	loadWG.Wait()

	if len(samples) < 2 {
		log("not enough samples (%d) to evaluate drift gates", len(samples))
		os.Exit(1)
	}

	// Pick the "post-warmup" reference sample. Anything before the
	// warmupFrac mark is considered server warmup and is not allowed
	// to be the baseline — it would make every gate trivially pass.
	warmupElapsed := (*duration).Seconds() * *warmupFrac
	var baseline sample
	for _, s := range samples {
		if s.ElapsedSeconds >= warmupElapsed {
			baseline = s
			break
		}
	}
	if baseline.Goroutines == 0 {
		// Very short runs — warmupElapsed > anything sampled. Fall
		// back to the first sample so the gate still runs and tests
		// the plumbing, but log it loudly.
		baseline = samples[0]
		log("warning: warmup fraction too large for run length; using t0 as baseline")
	}
	final := samples[len(samples)-1]

	var regressions []string

	if float64(final.Goroutines) > *goroutineMultiplier*float64(baseline.Goroutines) && baseline.Goroutines > 0 {
		regressions = append(regressions, fmt.Sprintf(
			"goroutine leak: %d → %d (limit %.1f× = %.0f)",
			baseline.Goroutines, final.Goroutines, *goroutineMultiplier,
			*goroutineMultiplier*float64(baseline.Goroutines)))
	}
	if float64(final.HeapInuseBytes) > *heapMultiplier*float64(baseline.HeapInuseBytes) && baseline.HeapInuseBytes > 0 {
		regressions = append(regressions, fmt.Sprintf(
			"heap climb: %s → %s (limit %.1f× = %s)",
			fmtBytes(baseline.HeapInuseBytes), fmtBytes(final.HeapInuseBytes),
			*heapMultiplier, fmtBytes(int64(*heapMultiplier*float64(baseline.HeapInuseBytes)))))
	}
	if *dbFile != "" && baseline.DBFileBytes > 0 && float64(final.DBFileBytes) > *dbMultiplier*float64(baseline.DBFileBytes) {
		regressions = append(regressions, fmt.Sprintf(
			"db bloat: %s → %s (limit %.1f× = %s)",
			fmtBytes(baseline.DBFileBytes), fmtBytes(final.DBFileBytes),
			*dbMultiplier, fmtBytes(int64(*dbMultiplier*float64(baseline.DBFileBytes)))))
	}

	summary := soakSummary{
		DurationSeconds:     time.Since(startTime).Seconds(),
		Concurrency:         *concurrency,
		SampleCount:         len(samples),
		TotalRequests:       totalReqs.Load(),
		TotalErrors:         totalErrs.Load(),
		Baseline:            baseline,
		Final:               final,
		GoroutineMultiplier: *goroutineMultiplier,
		HeapMultiplier:      *heapMultiplier,
		DBMultiplier:        *dbMultiplier,
		Regressions:         regressions,
		Passed:              len(regressions) == 0,
	}
	if data, err := json.MarshalIndent(&summary, "", "  "); err == nil {
		_ = os.WriteFile(*outPath, data, 0o644)
	}

	log("final: samples=%d reqs=%d errs=%d goroutines %d→%d heap_inuse %s→%s",
		len(samples), totalReqs.Load(), totalErrs.Load(),
		baseline.Goroutines, final.Goroutines,
		fmtBytes(baseline.HeapInuseBytes), fmtBytes(final.HeapInuseBytes))

	if len(regressions) == 0 {
		log("PASS — no drift detected")
		return
	}
	log("FAIL — drift detected:")
	for _, r := range regressions {
		log("  %s", r)
	}
	os.Exit(2)
}

type sample struct {
	WallTime       string  `json:"wall_time"`
	ElapsedSeconds float64 `json:"elapsed_seconds"`

	// From /metrics/api — Go runtime block
	Goroutines     int   `json:"go_goroutines"`
	AllocBytes     int64 `json:"go_memstats_alloc_bytes"`
	HeapInuseBytes int64 `json:"go_memstats_heap_inuse_bytes"`
	HeapObjects    int64 `json:"go_memstats_heap_objects"`
	SysBytes       int64 `json:"go_memstats_sys_bytes"`
	NextGCBytes    int64 `json:"go_memstats_next_gc_bytes"`
	NumGC          int64 `json:"go_memstats_num_gc"`

	// From /metrics/api — API-level
	APIRequestsTotal int64 `json:"api_requests_total"`
	APIActive        int64 `json:"api_requests_active"`
	APIErrors        int64 `json:"api_errors_total"`

	// From /metrics/api — EventBus
	EventbusPublished int64 `json:"eventbus_published_total"`
	EventbusErrors    int64 `json:"eventbus_errors_total"`

	// Optional DB file size on disk
	DBFileBytes int64 `json:"db_file_bytes"`
}

type soakSummary struct {
	DurationSeconds     float64  `json:"duration_seconds"`
	Concurrency         int      `json:"concurrency"`
	SampleCount         int      `json:"sample_count"`
	TotalRequests       int64    `json:"total_requests"`
	TotalErrors         int64    `json:"total_errors"`
	Baseline            sample   `json:"baseline_sample"`
	Final               sample   `json:"final_sample"`
	GoroutineMultiplier float64  `json:"goroutine_multiplier"`
	HeapMultiplier      float64  `json:"heap_multiplier"`
	DBMultiplier        float64  `json:"db_multiplier"`
	Regressions         []string `json:"regressions"`
	Passed              bool     `json:"passed"`
}

func takeSample(ctx context.Context, client *http.Client, baseURL, dbFile string) (sample, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/metrics/api", nil)
	resp, err := client.Do(req)
	if err != nil {
		return sample{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return sample{}, fmt.Errorf("metrics endpoint returned %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return sample{}, err
	}

	s := sample{WallTime: time.Now().UTC().Format(time.RFC3339Nano)}
	metrics := parsePromText(string(data))

	s.Goroutines = int(metrics["go_goroutines"])
	s.AllocBytes = int64(metrics["go_memstats_alloc_bytes"])
	s.HeapInuseBytes = int64(metrics["go_memstats_heap_inuse_bytes"])
	s.HeapObjects = int64(metrics["go_memstats_heap_objects"])
	s.SysBytes = int64(metrics["go_memstats_sys_bytes"])
	s.NextGCBytes = int64(metrics["go_memstats_next_gc_bytes"])
	s.NumGC = int64(metrics["go_memstats_num_gc"])

	s.APIRequestsTotal = int64(metrics["api_requests_total"])
	s.APIActive = int64(metrics["api_requests_active"])
	s.APIErrors = int64(metrics["api_errors_total"])
	s.EventbusPublished = int64(metrics["eventbus_published_total"])
	s.EventbusErrors = int64(metrics["eventbus_errors_total"])

	if dbFile != "" {
		if fi, err := os.Stat(dbFile); err == nil {
			s.DBFileBytes = fi.Size()
		}
	}
	return s, nil
}

// parsePromText parses the subset of Prometheus text format that
// /metrics/api emits: `metric_name value` on its own line, optionally
// with braces and labels that we skip. Comments (#) are ignored.
// Only label-less metrics are captured — we don't need per-endpoint
// breakdowns for drift detection.
func parsePromText(body string) map[string]float64 {
	out := make(map[string]float64)
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsRune(line, '{') {
			// Skip labeled metrics — not needed for drift gates.
			continue
		}
		parts := strings.Fields(line)
		if len(parts) != 2 {
			continue
		}
		v, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}
		out[parts[0]] = v
	}
	return out
}

func writeTrace(w *bufio.Writer, s sample) {
	if w == nil {
		return
	}
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	_, _ = w.Write(data)
	_ = w.WriteByte('\n')
	w.Flush()
}

func fmtBytes(n int64) string {
	const (
		kb = 1024
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2fGiB", float64(n)/gb)
	case n >= mb:
		return fmt.Sprintf("%.2fMiB", float64(n)/mb)
	case n >= kb:
		return fmt.Sprintf("%.2fKiB", float64(n)/kb)
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	s := d / time.Second
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
