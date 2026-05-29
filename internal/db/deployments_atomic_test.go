package db

import (
	"context"
	"sync"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestSQLite_CreateDeploymentAtomicVersion_NoDuplicates is the RACE-002b
// regression: concurrent deploys of the same app must each receive a distinct
// version. Before the fix, version allocation (SELECT MAX) and row insertion
// were separate steps, so two deploys could read the same MAX and both claim
// it. CreateDeploymentAtomicVersion allocates and inserts in one atomic step.
func TestSQLite_CreateDeploymentAtomicVersion_NoDuplicates(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	app := createTestApp(t, db, ctx)

	const n = 50
	versions := make([]int, n)

	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			dep := &core.Deployment{
				AppID:       app.ID,
				Image:       "nginx:latest",
				Status:      "deploying",
				TriggeredBy: "test",
				Strategy:    "recreate",
			}
			if err := db.CreateDeploymentAtomicVersion(ctx, dep); err != nil {
				t.Errorf("CreateDeploymentAtomicVersion: %v", err)
				return
			}
			versions[idx] = dep.Version
		}(i)
	}
	wg.Wait()

	seen := make(map[int]bool, n)
	for _, v := range versions {
		if v == 0 {
			t.Fatalf("got version 0 — allocation failed for at least one deploy")
		}
		if seen[v] {
			t.Fatalf("duplicate version %d allocated across concurrent deploys: %v", v, versions)
		}
		seen[v] = true
	}
	// The allocated versions must be exactly 1..n with no gaps.
	for i := 1; i <= n; i++ {
		if !seen[i] {
			t.Errorf("missing version %d; got %v", i, versions)
		}
	}

	// The table must hold exactly n rows for the app.
	deps, err := db.ListDeploymentsByApp(ctx, app.ID, n+10)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp: %v", err)
	}
	if len(deps) != n {
		t.Errorf("expected %d deployment rows, got %d", n, len(deps))
	}
}
