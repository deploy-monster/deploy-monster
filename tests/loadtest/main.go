// Package main provides a simple HTTP load test tool for DeployMonster API.
//
// Usage:
//
//	go run ./tests/loadtest -url http://localhost:8443 -duration 30s -concurrency 10
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

type result struct {
	endpoint string
	status   int
	latency  time.Duration
	err      error
}

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
	flag.Parse()

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/health"},
		{"GET", "/api/v1/health"},
		{"GET", "/api/v1/marketplace"},
		{"GET", "/api/v1/openapi.json"},
		{"GET", "/metrics/api"},
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

	// Output JSON summary for CI
	summary := map[string]any{
		"total_requests":      totalReqs,
		"successful":          totalOK,
		"errors":              totalErr,
		"requests_per_second": rps,
		"duration_seconds":    duration.Seconds(),
		"concurrency":         *concurrency,
	}
	jsonOut, _ := json.MarshalIndent(summary, "", "  ")
	os.WriteFile("loadtest-results.json", jsonOut, 0644)
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
