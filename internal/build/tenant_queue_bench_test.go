package build

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

// Benchmarks for the Phase 3.3.7 per-tenant build gate. The queue is
// on the critical path of every deploy webhook, so microbenchmarks
// here guard against accidental regressions in Submit throughput and
// in the two-phase acquire/release pattern.
//
// Benchmarks intentionally use no-op funcs so the numbers measure the
// gate itself, not the work it guards. Real build durations are on
// the order of seconds, so gate overhead must stay orders of magnitude
// below that — microseconds, at worst.

// BenchmarkTenantQueue_Submit_SingleTenant measures the steady-state
// Submit cost when one tenant is repeatedly enqueueing into a queue
// whose global cap is large enough to absorb the traffic.
func BenchmarkTenantQueue_Submit_SingleTenant(b *testing.B) {
	q := NewTenantQueue(1000, 1000, tenantQueueLogger())
	defer q.Shutdown(context.Background())

	ctx := context.Background()
	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		if err := q.Submit(ctx, "tenant-single", func() { wg.Done() }); err != nil {
			b.Fatalf("Submit: %v", err)
		}
	}
	wg.Wait()
}

// BenchmarkTenantQueue_Submit_ManyTenants measures the cost of the
// tenantSlot map lookup under a realistic multi-tenant workload where
// many distinct tenantID strings hit the queue in round-robin order.
// A regression here would surface as map-allocation churn or lock
// contention on q.mu.
func BenchmarkTenantQueue_Submit_ManyTenants(b *testing.B) {
	q := NewTenantQueue(1000, 1000, tenantQueueLogger())
	defer q.Shutdown(context.Background())

	const tenants = 100
	tenantIDs := make([]string, tenants)
	for i := range tenantIDs {
		tenantIDs[i] = fmt.Sprintf("tenant-%d", i)
	}

	ctx := context.Background()
	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		tid := tenantIDs[i%tenants]
		if err := q.Submit(ctx, tid, func() { wg.Done() }); err != nil {
			b.Fatalf("Submit: %v", err)
		}
	}
	wg.Wait()
}

// BenchmarkTenantQueue_Submit_Parallel drives Submit from many
// goroutines simultaneously against a single queue. The queue's
// internal mutex is the natural bottleneck — a fast implementation
// hands the slot off under lock, then runs fn on a separate goroutine
// without the lock held. Regressions would show up as the ns/op
// counter climbing when we add cores.
func BenchmarkTenantQueue_Submit_Parallel(b *testing.B) {
	q := NewTenantQueue(1000, 1000, tenantQueueLogger())
	defer q.Shutdown(context.Background())

	ctx := context.Background()
	var wg sync.WaitGroup
	var counter atomic.Int64

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		var local int64
		for pb.Next() {
			wg.Add(1)
			tid := fmt.Sprintf("tenant-%d", local%16)
			local++
			if err := q.Submit(ctx, tid, func() {
				counter.Add(1)
				wg.Done()
			}); err != nil {
				b.Fatalf("Submit: %v", err)
			}
		}
	})
	wg.Wait()
}

// BenchmarkTenantQueue_Submit_SaturatedGlobal measures the hot path
// where Submit has to wait on the global semaphore channel because
// all slots are full. This is the worst case — every Submit blocks
// in the channel select before releasing. The goal is to prove that
// saturated Submits still complete at reasonable throughput rather
// than stall or allocate.
func BenchmarkTenantQueue_Submit_SaturatedGlobal(b *testing.B) {
	// Global cap 4, per-tenant cap 4 — small enough that Submit
	// must block on almost every call under parallel load.
	q := NewTenantQueue(4, 4, tenantQueueLogger())
	defer q.Shutdown(context.Background())

	ctx := context.Background()
	var wg sync.WaitGroup

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		if err := q.Submit(ctx, "t", func() { wg.Done() }); err != nil {
			b.Fatalf("Submit: %v", err)
		}
	}
	wg.Wait()
}
