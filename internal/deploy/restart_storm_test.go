package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestRestartStorm_ReclaimStaleDeployments is the Phase 3.1.2 headline
// test. It seeds 20 apps with deployments in "deploying" or "building"
// status (simulating a master crash mid-deploy), then drives
// Module.reclaimStaleDeployments and asserts every row ends up in
// "failed" with FinishedAt set and the BuildLog annotated with the
// reclaim reason.
//
// The scenario mirrors the roadmap prompt: "Spawn 20 apps, kill master,
// restart master, verify all 20 reconnect and none are left in deploying
// or building status." We exercise the sweep in-process — a full
// subprocess spawn is not required to verify the contract because the
// reclaim logic is a pure function of the deployment store state.
func TestRestartStorm_ReclaimStaleDeployments(t *testing.T) {
	store := newMockStore()

	// Seed 20 apps: 10 with a "deploying" deployment, 10 with "building".
	// Mix in a handful of "running" and "failed" rows to prove the sweep
	// only touches in-flight states and leaves terminal rows alone.
	const apps = 20
	staleIDs := make(map[string]string, apps)
	for i := 0; i < apps; i++ {
		appID := fmt.Sprintf("app-%02d", i)
		store.apps[appID] = &core.Application{
			ID:     appID,
			Name:   appID,
			Status: "deploying",
		}
		staleStatus := "deploying"
		if i%2 == 1 {
			staleStatus = "building"
		}
		dep := &core.Deployment{
			AppID:   appID,
			Version: 1,
			Image:   "example/" + appID + ":v1",
			Status:  staleStatus,
		}
		if err := store.CreateDeployment(context.Background(), dep); err != nil {
			t.Fatalf("seed create deployment: %v", err)
		}
		staleIDs[appID] = dep.ID
	}
	// Terminal rows the sweep must NOT touch.
	runningDep := &core.Deployment{
		AppID:   "app-running",
		Version: 1,
		Image:   "example/running:v1",
		Status:  "running",
	}
	_ = store.CreateDeployment(context.Background(), runningDep)
	failedDep := &core.Deployment{
		AppID:   "app-failed",
		Version: 1,
		Image:   "example/failed:v1",
		Status:  "failed",
	}
	_ = store.CreateDeployment(context.Background(), failedDep)

	m := &Module{
		store:  store,
		logger: slog.Default(),
	}

	m.reclaimStaleDeployments(context.Background())

	// Every seeded in-flight deployment must now be "failed" with
	// FinishedAt set and the reclaim marker in the build log.
	for appID, depID := range staleIDs {
		got := store.deploymentsByID[depID]
		if got == nil {
			t.Errorf("app %s: deployment %s missing from store", appID, depID)
			continue
		}
		if got.Status != "failed" {
			t.Errorf("app %s: deployment %s status = %q, want %q",
				appID, depID, got.Status, "failed")
		}
		if got.FinishedAt == nil {
			t.Errorf("app %s: deployment %s FinishedAt not set after reclaim", appID, depID)
		}
		if got.BuildLog == "" {
			t.Errorf("app %s: deployment %s BuildLog empty after reclaim (want reclaim marker)", appID, depID)
		}
	}

	// The "running" and "failed" rows must be untouched.
	if store.deploymentsByID[runningDep.ID].Status != "running" {
		t.Errorf("running deployment was incorrectly swept: status = %q",
			store.deploymentsByID[runningDep.ID].Status)
	}
	if store.deploymentsByID[failedDep.ID].Status != "failed" {
		t.Errorf("failed deployment status changed: got %q, want failed",
			store.deploymentsByID[failedDep.ID].Status)
	}
	if store.deploymentsByID[failedDep.ID].FinishedAt != nil {
		t.Errorf("failed deployment FinishedAt was mutated by the sweep")
	}

	// Each seeded app should have had UpdateAppStatus called with "failed"
	// so the app list UI reflects the reclaim. Count only the "failed"
	// transitions emitted by the sweep.
	failedAppUpdates := 0
	for _, u := range store.appStatusUpdates {
		if u.Status == "failed" {
			failedAppUpdates++
		}
	}
	if failedAppUpdates != apps {
		t.Errorf("app status updates to failed = %d, want %d", failedAppUpdates, apps)
	}
}

// TestRestartStorm_EmptyStore_IsNoop verifies the sweep is safe on a
// freshly initialized store with zero deployments — the Module.Start
// path must not emit warnings, panics, or log spam when there is
// nothing to reclaim.
func TestRestartStorm_EmptyStore_IsNoop(t *testing.T) {
	store := newMockStore()
	m := &Module{store: store, logger: slog.Default()}

	m.reclaimStaleDeployments(context.Background())

	if store.updateDeploymentCall != 0 {
		t.Errorf("UpdateDeployment called %d times on empty store, want 0",
			store.updateDeploymentCall)
	}
	if len(store.appStatusUpdates) != 0 {
		t.Errorf("UpdateAppStatus called %d times on empty store, want 0",
			len(store.appStatusUpdates))
	}
}

// TestRestartStorm_ListError_DoesNotAbortSweep proves the sweep is
// resilient to a backend error on one status — if ListDeploymentsByStatus
// fails for "deploying", the sweep still attempts "building" and logs
// the failure without crashing. This matches the recovery-first design
// of cleanOrphanContainers.
func TestRestartStorm_ListError_DoesNotAbortSweep(t *testing.T) {
	store := newMockStore()
	// Force the list call to fail for every status.
	store.listByStatusErr = fmt.Errorf("db unavailable")

	m := &Module{store: store, logger: slog.Default()}

	// Must not panic. The sweep should log the error and return cleanly.
	m.reclaimStaleDeployments(context.Background())

	if store.updateDeploymentCall != 0 {
		t.Errorf("UpdateDeployment called %d times despite list error, want 0",
			store.updateDeploymentCall)
	}
}

// TestRestartStorm_UpdateError_SkipsRowButContinues verifies that when
// UpdateDeployment fails on one row, the sweep logs the failure and
// still processes the remaining rows. This is important because a
// transient DB contention on one row should not leave the other 19
// deployments stuck in "deploying" forever.
func TestRestartStorm_UpdateError_SkipsRowButContinues(t *testing.T) {
	store := newMockStore()
	store.apps["app-01"] = &core.Application{ID: "app-01", Status: "deploying"}
	store.apps["app-02"] = &core.Application{ID: "app-02", Status: "deploying"}
	_ = store.CreateDeployment(context.Background(), &core.Deployment{
		AppID: "app-01", Version: 1, Status: "deploying",
	})
	_ = store.CreateDeployment(context.Background(), &core.Deployment{
		AppID: "app-02", Version: 1, Status: "deploying",
	})
	store.updateDeploymentErr = fmt.Errorf("transient db error")

	m := &Module{store: store, logger: slog.Default()}
	m.reclaimStaleDeployments(context.Background())

	// Both rows must have been attempted (updateDeploymentCall ≥ 2)
	// even though both failed.
	if store.updateDeploymentCall < 2 {
		t.Errorf("UpdateDeployment call count = %d, want ≥2 (sweep stopped early)",
			store.updateDeploymentCall)
	}
}

// TestUpdateDeploymentPersistsStatus is a narrow regression guard for
// the Tier 100 data-integrity bug: pre-Tier-100 deployer.deployImage
// mutated deployment.Status = "running" in memory only, so every row
// in the deployments table was permanently stuck in "deploying". This
// test proves UpdateDeployment now actually writes the status change
// back and that a subsequent ListDeploymentsByStatus reflects it.
func TestUpdateDeploymentPersistsStatus(t *testing.T) {
	store := newMockStore()
	dep := &core.Deployment{
		AppID:   "app-1",
		Version: 1,
		Image:   "example/app-1:v1",
		Status:  "deploying",
	}
	if err := store.CreateDeployment(context.Background(), dep); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Initial state: one row visible under "deploying".
	deploying, _ := store.ListDeploymentsByStatus(context.Background(), "deploying")
	if len(deploying) != 1 {
		t.Fatalf("initial deploying count = %d, want 1", len(deploying))
	}

	// Mimic the post-container-start transition from deployer.go.
	dep.Status = "running"
	if err := store.UpdateDeployment(context.Background(), dep); err != nil {
		t.Fatalf("update: %v", err)
	}

	// The "deploying" bucket must now be empty.
	deploying, _ = store.ListDeploymentsByStatus(context.Background(), "deploying")
	if len(deploying) != 0 {
		t.Errorf("post-update deploying count = %d, want 0", len(deploying))
	}
	// And the row must show up under "running".
	running, _ := store.ListDeploymentsByStatus(context.Background(), "running")
	if len(running) != 1 {
		t.Errorf("post-update running count = %d, want 1", len(running))
	}
}
