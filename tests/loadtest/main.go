// Package main provides a simple HTTP load test tool for DeployMonster API.
//
// Usage:
//
//	# Ad-hoc run — just print results:
//	go run ./tests/loadtest -url http://localhost:8443 -duration 30s -concurrency 10
//
//	# Capture a baseline (overwrite the baseline file):
//	go run ./tests/loadtest -url ... -save-baseline tests/loadtest/baselines/http.json
//
//	# Compare a run against the committed baseline — fail with exit code 2
//	# on a ≥10% regression in requests/sec or p95 latency (per-endpoint):
//	go run ./tests/loadtest -url ... -baseline tests/loadtest/baselines/http.json
//
// Regression semantics (Phase 5.3.5):
//   - throughput regression: current_rps < baseline_rps * (1 - threshold)
//   - latency   regression: current_p95 > baseline_p95 * (1 + threshold)
//
// Throughput and latency are compared *per endpoint*. If any single endpoint
// regresses by the threshold or more, the run fails. The default threshold
// is 0.10 (10%) and is configurable via -regression-threshold.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type stats struct {
	total   int64
	success int64
	errors  int64
	latency []time.Duration
}

func main() {
	baseURL := flag.String("url", "http://localhost:8443", "Base URL of the DeployMonster API")
	duration := flag.Duration("duration", 10*time.Second, "Test duration")
	concurrency := flag.Int("concurrency", 10, "Number of concurrent workers")
	outPath := flag.String("out", "loadtest-results.json", "Write per-run results to this path")
	baselinePath := flag.String("baseline", "", "Compare results against baseline JSON and exit non-zero on regression")
	saveBaseline := flag.String("save-baseline", "", "Write current results to this baseline path (overwrites)")
	threshold := flag.Float64("regression-threshold", 0.10, "Fractional regression threshold (0.10 = 10%)")
	flag.Parse()

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"GET", "/api/v1/health"},
		{"GET", "/api/v1/marketplace"},
		{"GET", "/api/v1/openapi.json"},
		{"GET", "/login"},
	}

	fmt.Printf("Load test: %s\n", *baseURL)
	fmt.Printf("Duration: %s, Concurrency: %d, Endpoints: %d\n\n", *duration, *concurrency, len(endpoints))

	client := &http.Client{Timeout: 10 * time.Second}

	var (
		mu      sync.Mutex
		results = make(map[string]*stats)
		running int64
		done    = make(chan struct{})
	)

	for _, ep := range endpoints {
		key := ep.method + " " + ep.path
		results[key] = &stats{}
	}

	deadline := time.After(*duration)
	var wg sync.WaitGroup

	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}

				for _, ep := range endpoints {
					select {
					case <-done:
						return
					default:
					}

					atomic.AddInt64(&running, 1)
					start := time.Now()
					url := *baseURL + ep.path
					req, _ := http.NewRequest(ep.method, url, nil)
					resp, err := client.Do(req)
					lat := time.Since(start)
					atomic.AddInt64(&running, -1)

					key := ep.method + " " + ep.path
					mu.Lock()
					s := results[key]
					s.total++
					s.latency = append(s.latency, lat)
					if err != nil {
						s.errors++
					} else {
						resp.Body.Close()
						if resp.StatusCode < 400 {
							s.success++
						} else {
							s.errors++
						}
					}
					mu.Unlock()
				}
			}
		}()
	}

	<-deadline
	close(done)
	wg.Wait()

	// Print results
	fmt.Println("Results:")
	fmt.Println(strings.Repeat("─", 90))
	fmt.Printf("%-30s %8s %8s %8s %10s %10s %10s\n", "Endpoint", "Total", "OK", "Err", "p50", "p95", "p99")
	fmt.Println(strings.Repeat("─", 90))

	var totalReqs, totalOK, totalErr int64

	keys := make([]string, 0, len(results))
	for k := range results {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		s := results[key]
		totalReqs += s.total
		totalOK += s.success
		totalErr += s.errors

		sort.Slice(s.latency, func(i, j int) bool { return s.latency[i] < s.latency[j] })
		p50 := percentile(s.latency, 0.50)
		p95 := percentile(s.latency, 0.95)
		p99 := percentile(s.latency, 0.99)

		fmt.Printf("%-30s %8d %8d %8d %10s %10s %10s\n",
			key, s.total, s.success, s.errors,
			p50.Truncate(time.Microsecond),
			p95.Truncate(time.Microsecond),
			p99.Truncate(time.Microsecond),
		)
	}

	fmt.Println(strings.Repeat("─", 90))
	rps := float64(totalReqs) / duration.Seconds()
	fmt.Printf("Total: %d requests, %d OK, %d errors, %.0f req/s\n", totalReqs, totalOK, totalErr, rps)

	// Build a rich per-endpoint report — this is what gets written to disk
	// and what baseline comparison reads back.
	report := loadtestReport{
		DurationSeconds: duration.Seconds(),
		Concurrency:     *concurrency,
		TotalRequests:   totalReqs,
		Successful:      totalOK,
		Errors:          totalErr,
		RequestsPerSec:  rps,
		Endpoints:       make(map[string]endpointStats, len(results)),
	}
	for _, key := range keys {
		s := results[key]
		p50 := percentile(s.latency, 0.50)
		p95 := percentile(s.latency, 0.95)
		p99 := percentile(s.latency, 0.99)
		var epRPS float64
		if duration.Seconds() > 0 {
			epRPS = float64(s.total) / duration.Seconds()
		}
		report.Endpoints[key] = endpointStats{
			Total:           s.total,
			Successful:      s.success,
			Errors:          s.errors,
			RequestsPerSec:  epRPS,
			P50Microseconds: p50.Microseconds(),
			P95Microseconds: p95.Microseconds(),
			P99Microseconds: p99.Microseconds(),
		}
	}

	// Always write the per-run report, regardless of baseline mode.
	if *outPath != "" {
		if err := writeReport(*outPath, &report); err != nil {
			fmt.Fprintf(os.Stderr, "failed to write %s: %v\n", *outPath, err)
			os.Exit(1)
		}
	}

	// -save-baseline overwrites the committed baseline with this run's
	// numbers. Used when intentionally capturing a new baseline after
	// config changes or hardware upgrades.
	if *saveBaseline != "" {
		if err := writeReport(*saveBaseline, &report); err != nil {
			fmt.Fprintf(os.Stderr, "failed to save baseline %s: %v\n", *saveBaseline, err)
			os.Exit(1)
		}
		fmt.Printf("\nBaseline saved to %s\n", *saveBaseline)
		return
	}

	// -baseline drives the regression gate. Load the committed baseline
	// and compare per-endpoint. Any endpoint whose RPS dropped by at
	// least threshold% OR whose p95 grew by at least threshold% counts
	// as a regression.
	if *baselinePath != "" {
		baseline, err := readReport(*baselinePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read baseline %s: %v\n", *baselinePath, err)
			os.Exit(1)
		}
		regs := compareReports(baseline, &report, *threshold)
		if len(regs) == 0 {
			fmt.Printf("\nBaseline check PASSED (threshold %.0f%%)\n", *threshold*100)
			return
		}
		fmt.Printf("\nBaseline check FAILED (threshold %.0f%%):\n", *threshold*100)
		for _, r := range regs {
			fmt.Printf("  %-30s %s\n", r.Endpoint, r.Reason)
		}
		os.Exit(2)
	}
}

type endpointStats struct {
	Total           int64   `json:"total"`
	Successful      int64   `json:"successful"`
	Errors          int64   `json:"errors"`
	RequestsPerSec  float64 `json:"requests_per_second"`
	P50Microseconds int64   `json:"p50_us"`
	P95Microseconds int64   `json:"p95_us"`
	P99Microseconds int64   `json:"p99_us"`
}

type loadtestReport struct {
	DurationSeconds float64                  `json:"duration_seconds"`
	Concurrency     int                      `json:"concurrency"`
	TotalRequests   int64                    `json:"total_requests"`
	Successful      int64                    `json:"successful"`
	Errors          int64                    `json:"errors"`
	RequestsPerSec  float64                  `json:"requests_per_second"`
	Endpoints       map[string]endpointStats `json:"endpoints"`
}

type regression struct {
	Endpoint string
	Reason   string
}

func writeReport(path string, r *loadtestReport) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func readReport(path string) (*loadtestReport, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var r loadtestReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// compareReports returns one regression per endpoint that exceeded the
// threshold on either the throughput-drop or latency-growth axis. An
// endpoint present in the baseline but missing from the current run is
// treated as a regression (the endpoint disappeared). An endpoint in the
// current run but missing from the baseline is ignored — it is new
// surface area that the baseline has not yet captured.
func compareReports(baseline, current *loadtestReport, threshold float64) []regression {
	var regs []regression
	for key, base := range baseline.Endpoints {
		cur, ok := current.Endpoints[key]
		if !ok {
			regs = append(regs, regression{
				Endpoint: key,
				Reason:   "endpoint missing from current run",
			})
			continue
		}
		if base.RequestsPerSec > 0 {
			minAllowed := base.RequestsPerSec * (1 - threshold)
			if cur.RequestsPerSec < minAllowed {
				regs = append(regs, regression{
					Endpoint: key,
					Reason: fmt.Sprintf("rps %.0f < baseline %.0f * (1-%.2f) = %.0f",
						cur.RequestsPerSec, base.RequestsPerSec, threshold, minAllowed),
				})
			}
		}
		if base.P95Microseconds > 0 {
			maxAllowed := float64(base.P95Microseconds) * (1 + threshold)
			if float64(cur.P95Microseconds) > maxAllowed {
				regs = append(regs, regression{
					Endpoint: key,
					Reason: fmt.Sprintf("p95 %dµs > baseline %dµs * (1+%.2f) = %.0fµs",
						cur.P95Microseconds, base.P95Microseconds, threshold, maxAllowed),
				})
			}
		}
	}
	sort.Slice(regs, func(i, j int) bool { return regs[i].Endpoint < regs[j].Endpoint })
	return regs
}

func percentile(sorted []time.Duration, pct float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * pct)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
