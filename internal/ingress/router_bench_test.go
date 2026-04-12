package ingress

import (
	"fmt"
	"sync/atomic"
	"testing"
)

// Benchmarks for the lock-free routing table.
//
// BenchmarkRouteTable_Match_Parallel drives Match from many goroutines
// simultaneously and is the primary win of the atomic-snapshot rewrite:
// readers never contend on a lock, so throughput scales linearly with
// GOMAXPROCS on workloads where writes are rare (the real DeployMonster
// ingress pattern — writes happen on deploy events, reads happen on
// every request).
//
// BenchmarkRouteTable_Match_Concurrent_WithWriter additionally drives
// one writer goroutine in the background to confirm that writes under
// contention do not pause reads.

func buildRouteTable(n int) *RouteTable {
	rt := NewRouteTable()
	for i := 0; i < n; i++ {
		rt.Upsert(&RouteEntry{
			Host:        fmt.Sprintf("app-%d.example.com", i),
			PathPrefix:  "/",
			Backends:    []string{fmt.Sprintf("10.0.0.%d:80", i%255)},
			Priority:    100,
			ServiceName: fmt.Sprintf("svc-%d", i),
			AppID:       fmt.Sprintf("app-%d", i),
		})
	}
	return rt
}

func BenchmarkRouteTable_Match_SingleThreaded(b *testing.B) {
	rt := buildRouteTable(200)
	host := "app-100.example.com"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if m := rt.Match(host, "/users"); m == nil {
			b.Fatal("expected match")
		}
	}
}

func BenchmarkRouteTable_Match_Parallel(b *testing.B) {
	rt := buildRouteTable(200)
	host := "app-100.example.com"

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if m := rt.Match(host, "/users"); m == nil {
				b.Fatal("expected match")
			}
		}
	})
}

// BenchmarkRouteTable_Match_Concurrent_WithWriter drives a single
// writer goroutine in the background to confirm that contended writes
// do not pause reads. A pre-rewrite sync.RWMutex implementation would
// show readers parking on RLock every time the writer held the Lock;
// the atomic-snapshot implementation shows no such stalls.
func BenchmarkRouteTable_Match_Concurrent_WithWriter(b *testing.B) {
	rt := buildRouteTable(200)
	host := "app-100.example.com"

	var stop atomic.Bool
	defer stop.Store(true)
	go func() {
		var rev int
		for !stop.Load() {
			rev++
			rt.Upsert(&RouteEntry{
				Host:     fmt.Sprintf("churn-%d.example.com", rev%16),
				Backends: []string{"10.0.0.1:80"},
				Priority: 50,
				AppID:    "churn",
			})
		}
	}()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if m := rt.Match(host, "/users"); m == nil {
				b.Fatal("expected match")
			}
		}
	})
}

func BenchmarkRouteTable_Upsert(b *testing.B) {
	rt := buildRouteTable(200)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.Upsert(&RouteEntry{
			Host:     fmt.Sprintf("bench-%d.example.com", i%1024),
			Backends: []string{"10.0.0.1:80"},
			Priority: 50,
			AppID:    "bench",
		})
	}
}
