package db

import (
	"context"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func BenchmarkCreateApp(b *testing.B) {
	db := benchDB(b)
	ctx := context.Background()

	// Setup tenant and project
	tenant := &core.Tenant{Name: "Bench", Slug: "bench", Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)
	db.DB().ExecContext(ctx, "INSERT INTO projects (id, tenant_id, name) VALUES ('benchproj', ?, 'BenchProj')", tenant.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app := &core.Application{
			ProjectID: "benchproj", TenantID: tenant.ID, Name: "bench-app",
			Type: "service", SourceType: "image", Status: "running", Replicas: 1,
		}
		db.CreateApp(ctx, app)
	}
}

func BenchmarkGetApp(b *testing.B) {
	db := benchDB(b)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "Bench", Slug: "bench2", Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)
	db.DB().ExecContext(ctx, "INSERT INTO projects (id, tenant_id, name) VALUES ('benchproj2', ?, 'BenchProj')", tenant.ID)
	app := &core.Application{
		ProjectID: "benchproj2", TenantID: tenant.ID, Name: "bench-get",
		Type: "service", SourceType: "image", Status: "running", Replicas: 1,
	}
	db.CreateApp(ctx, app)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.GetApp(ctx, app.ID)
	}
}

func benchDB(b *testing.B) *SQLiteDB {
	b.Helper()
	dir := b.TempDir()
	db, err := NewSQLite(dir + "/bench.db")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { db.Close() })
	return db
}

// BenchmarkStore_ConcurrentWrites_64Workers exercises the single-writer
// SQLite pool (MaxOpenConns(1)) under a 64-way concurrent DeploymentStore
// fan-out. b.N deployments are created across 64 workers; per-op latency
// is tracked and reported as p50/p95/p99 via b.ReportMetric.
//
// The companion TestStore_ConcurrentWrites_BaselineGate in
// concurrent_writes_gate_test.go runs the same shape with a fixed op
// count and fails on a >10 % p95 regression against the committed
// baseline at internal/db/testdata/concurrent_writes_baseline.json.
func BenchmarkStore_ConcurrentWrites_64Workers(b *testing.B) {
	db := benchDB(b)
	appID := bootstrapBenchApp(b, db, "bench-cw")

	b.ResetTimer()
	stats := runConcurrentWrites(b, db, appID, 64, b.N)
	b.StopTimer()

	b.ReportMetric(float64(stats.p50.Microseconds()), "p50_us")
	b.ReportMetric(float64(stats.p95.Microseconds()), "p95_us")
	b.ReportMetric(float64(stats.p99.Microseconds()), "p99_us")
	if stats.errors > 0 {
		b.Fatalf("CreateDeployment reported %d errors under 64-way contention", stats.errors)
	}
}

// bootstrapBenchApp creates a tenant, project, and app so that
// DeploymentStore.Create has a valid foreign key target. Returns the app
// ID. testing.TB so the same helper works for both benchmarks and the
// baseline-gate test.
func bootstrapBenchApp(tb testing.TB, db *SQLiteDB, slug string) string {
	tb.Helper()
	ctx := context.Background()
	tenant := &core.Tenant{
		Name:   "Bench",
		Slug:   slug + "-" + core.GenerateID()[:6],
		Status: "active",
		PlanID: "free",
	}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		tb.Fatalf("CreateTenant: %v", err)
	}
	proj := &core.Project{
		TenantID:    tenant.ID,
		Name:        "BenchProject",
		Description: "writers-under-load bench",
		Environment: "dev",
	}
	if err := db.CreateProject(ctx, proj); err != nil {
		tb.Fatalf("CreateProject: %v", err)
	}
	app := &core.Application{
		ProjectID:  proj.ID,
		TenantID:   tenant.ID,
		Name:       "bench-app",
		Type:       "service",
		SourceType: "image",
		Status:     "running",
		Replicas:   1,
	}
	if err := db.CreateApp(ctx, app); err != nil {
		tb.Fatalf("CreateApp: %v", err)
	}
	return app.ID
}

// concurrentWriteStats holds the latency distribution of a
// runConcurrentWrites invocation. Samples are kept sorted so the caller
// can compute additional percentiles without re-sorting.
type concurrentWriteStats struct {
	samples  []time.Duration
	p50      time.Duration
	p95      time.Duration
	p99      time.Duration
	duration time.Duration
	errors   int
}

// runConcurrentWrites fans `totalOps` DeploymentStore.Create calls across
// `workers` goroutines, tracks per-op wall-clock latency, and returns the
// sorted sample vector plus p50/p95/p99. Errors are counted, not fatal —
// the caller decides how to react.
func runConcurrentWrites(tb testing.TB, db *SQLiteDB, appID string, workers, totalOps int) concurrentWriteStats {
	tb.Helper()
	if totalOps <= 0 {
		return concurrentWriteStats{}
	}
	latencies := make([]time.Duration, totalOps)
	errs := make([]error, totalOps)
	ch := make(chan int, totalOps)
	for i := 0; i < totalOps; i++ {
		ch <- i
	}
	close(ch)

	start := time.Now()
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			for idx := range ch {
				started := time.Now()
				dep := &core.Deployment{
					AppID:       appID,
					Version:     idx + 1,
					Image:       "bench/img:latest",
					Status:      "pending",
					TriggeredBy: "bench",
					Strategy:    "recreate",
					StartedAt:   &started,
				}
				err := db.CreateDeployment(ctx, dep)
				latencies[idx] = time.Since(started)
				errs[idx] = err
			}
		}()
	}
	wg.Wait()
	duration := time.Since(start)

	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	nErr := 0
	for _, e := range errs {
		if e != nil {
			nErr++
		}
	}
	return concurrentWriteStats{
		samples:  latencies,
		p50:      percentileDuration(latencies, 0.50),
		p95:      percentileDuration(latencies, 0.95),
		p99:      percentileDuration(latencies, 0.99),
		duration: duration,
		errors:   nErr,
	}
}

// percentileDuration returns the duration at the given percentile from a
// pre-sorted sample slice. Uses the nearest-rank method (simple,
// deterministic, matches tests/loadtest).
func percentileDuration(sorted []time.Duration, pct float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)) * pct)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
