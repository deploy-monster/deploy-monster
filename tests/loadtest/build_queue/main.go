// Package main is a standalone load-test harness for the per-tenant
// build queue (internal/build.TenantQueue).
//
// Unlike tests/loadtest/main.go (which drives the HTTP API from the
// outside), this harness constructs a TenantQueue in-process and
// measures the fairness and throughput of the queue itself: how many
// Submits per second can it accept, how long do jobs wait before
// running under tenant contention, and does any one tenant get
// starved relative to its peers.
//
// Usage:
//
//	go run ./tests/loadtest/build_queue \
//	    -tenants 16 \
//	    -jobs 200 \
//	    -global 8 \
//	    -per-tenant 2 \
//	    -work 20ms
//
// Reports:
//   - Per-tenant job completion latency (p50, p95, p99, max)
//   - Overall throughput (jobs/sec)
//   - Worst-case per-tenant slowdown (max-tenant-p99 / median-tenant-p99)
//     as a fairness score; a fair queue stays close to 1.0.
//
// A healthy run with -global 8 -per-tenant 2 -tenants 16 -jobs 200
// should complete all 3200 jobs in well under 5 seconds on a modern
// workstation, with per-tenant p99 latencies clustered tightly — if
// one tenant's p99 is >10x the median, fairness has regressed.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/build"
)

type tenantResult struct {
	tenantID   string
	latencies  []time.Duration
	submitErrs int
}

func (r *tenantResult) percentile(p float64) time.Duration {
	if len(r.latencies) == 0 {
		return 0
	}
	idx := int(float64(len(r.latencies)) * p)
	if idx >= len(r.latencies) {
		idx = len(r.latencies) - 1
	}
	return r.latencies[idx]
}

func main() {
	tenants := flag.Int("tenants", 16, "number of concurrent tenants")
	jobs := flag.Int("jobs", 200, "jobs per tenant")
	globalCap := flag.Int("global", 8, "global concurrency cap")
	perTenantCap := flag.Int("per-tenant", 2, "per-tenant concurrency cap")
	work := flag.Duration("work", 20*time.Millisecond, "simulated build duration per job")
	submitTimeout := flag.Duration("submit-timeout", 30*time.Second, "max Submit wait before counting as error")
	flag.Parse()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := build.NewTenantQueue(*globalCap, *perTenantCap, logger)

	fmt.Printf("build_queue loadtest: tenants=%d jobs=%d global=%d per-tenant=%d work=%s\n",
		*tenants, *jobs, *globalCap, *perTenantCap, *work)
	fmt.Printf("total jobs=%d, ideal runtime ~%s\n\n",
		*tenants**jobs,
		time.Duration(float64(*tenants**jobs)/float64(*globalCap))*(*work))

	results := make([]tenantResult, *tenants)
	for i := range results {
		results[i] = tenantResult{
			tenantID:  fmt.Sprintf("tenant-%02d", i),
			latencies: make([]time.Duration, 0, *jobs),
		}
	}

	var wg sync.WaitGroup
	start := time.Now()

	for i := 0; i < *tenants; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			r := &results[idx]
			for j := 0; j < *jobs; j++ {
				submitStart := time.Now()
				done := make(chan time.Duration, 1)
				ctx, cancel := context.WithTimeout(context.Background(), *submitTimeout)
				err := q.Submit(ctx, r.tenantID, func() {
					time.Sleep(*work)
					done <- time.Since(submitStart)
				})
				cancel()
				if err != nil {
					r.submitErrs++
					continue
				}
				r.latencies = append(r.latencies, <-done)
			}
		}(i)
	}

	wg.Wait()
	total := time.Since(start)

	if err := q.Shutdown(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "shutdown error: %v\n", err)
	}

	for i := range results {
		sort.Slice(results[i].latencies, func(a, b int) bool {
			return results[i].latencies[a] < results[i].latencies[b]
		})
	}

	fmt.Printf("%-12s %8s %8s %10s %10s %10s %10s %6s\n",
		"tenant", "jobs", "errors", "p50", "p95", "p99", "max", "")
	fmt.Println(hline(80))
	var allP99 []time.Duration
	var completed, errors int
	for i := range results {
		r := &results[i]
		completed += len(r.latencies)
		errors += r.submitErrs
		p99 := r.percentile(0.99)
		allP99 = append(allP99, p99)
		maxLat := time.Duration(0)
		if len(r.latencies) > 0 {
			maxLat = r.latencies[len(r.latencies)-1]
		}
		fmt.Printf("%-12s %8d %8d %10s %10s %10s %10s\n",
			r.tenantID, len(r.latencies), r.submitErrs,
			r.percentile(0.50).Truncate(time.Microsecond),
			r.percentile(0.95).Truncate(time.Microsecond),
			p99.Truncate(time.Microsecond),
			maxLat.Truncate(time.Microsecond),
		)
	}

	sort.Slice(allP99, func(a, b int) bool { return allP99[a] < allP99[b] })
	medianP99 := allP99[len(allP99)/2]
	maxP99 := allP99[len(allP99)-1]
	var fairness float64
	if medianP99 > 0 {
		fairness = float64(maxP99) / float64(medianP99)
	}

	fmt.Println(hline(80))
	fmt.Printf("jobs completed: %d/%d (%d errors)\n", completed, *tenants**jobs, errors)
	fmt.Printf("total wall time: %s\n", total.Truncate(time.Millisecond))
	fmt.Printf("throughput: %.0f jobs/sec\n", float64(completed)/total.Seconds())
	fmt.Printf("p99 fairness (max/median): %.2fx\n", fairness)
	if fairness > 2.0 {
		fmt.Printf("\nWARNING: worst-case tenant p99 is %.2fx the median — fairness guarantee may be weakening\n", fairness)
		os.Exit(2)
	}
}

func hline(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = '-'
	}
	return string(b)
}
