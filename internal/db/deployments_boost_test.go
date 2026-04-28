package db

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

var testCounter atomic.Int64

func createTestApp(t *testing.T, db *SQLiteDB, ctx context.Context) *core.Application {
	t.Helper()
	n := testCounter.Add(1)
	tenant := &core.Tenant{Name: fmt.Sprintf("Test%d", n), Slug: fmt.Sprintf("test%d", n), Status: "active", PlanID: "free"}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	db.DB().ExecContext(ctx, "INSERT INTO projects (id, tenant_id, name) VALUES (?, ?, ?)", "p-"+tenant.ID, tenant.ID, "Project")
	app := &core.Application{
		ProjectID:  "p-" + tenant.ID,
		TenantID:   tenant.ID,
		Name:       fmt.Sprintf("test-app%d", n),
		Type:       "service",
		SourceType: "image",
		Status:     "running",
		Replicas:   1,
	}
	if err := db.CreateApp(ctx, app); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	return app
}

func TestSQLite_UpdateDeployment(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	app := createTestApp(t, db, ctx)

	dep := &core.Deployment{
		AppID:   app.ID,
		Version: 1,
		Status:  "deploying",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	// Update deployment
	now := time.Now()
	dep.Status = "running"
	dep.ContainerID = "abc123"
	dep.BuildLog = "build ok"
	dep.FinishedAt = &now

	if err := db.UpdateDeployment(ctx, dep); err != nil {
		t.Fatalf("UpdateDeployment: %v", err)
	}

	// Verify update
	latest, err := db.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if latest.Status != "running" {
		t.Errorf("status = %q, want running", latest.Status)
	}
	if latest.ContainerID != "abc123" {
		t.Errorf("container_id = %q, want abc123", latest.ContainerID)
	}
	// GetLatestDeployment does not select build_log, so verify directly
	var buildLog string
	_ = db.DB().QueryRowContext(ctx, "SELECT build_log FROM deployments WHERE id = ?", dep.ID).Scan(&buildLog)
	if buildLog != "build ok" {
		t.Errorf("build_log = %q, want build ok", buildLog)
	}
	if latest.FinishedAt == nil {
		t.Error("finished_at should be set")
	}
}

func TestSQLite_ListDeploymentsByStatus(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	app := createTestApp(t, db, ctx)

	// Create two deployments with different statuses
	dep1 := &core.Deployment{AppID: app.ID, Version: 1, Status: "success"}
	dep2 := &core.Deployment{AppID: app.ID, Version: 2, Status: "failed"}
	dep3 := &core.Deployment{AppID: app.ID, Version: 3, Status: "success"}
	for _, d := range []*core.Deployment{dep1, dep2, dep3} {
		if err := db.CreateDeployment(ctx, d); err != nil {
			t.Fatalf("CreateDeployment: %v", err)
		}
	}

	// List by success status
	success, err := db.ListDeploymentsByStatus(ctx, "success")
	if err != nil {
		t.Fatalf("ListDeploymentsByStatus: %v", err)
	}
	if len(success) != 2 {
		t.Errorf("len(success) = %d, want 2", len(success))
	}

	// List by failed status
	failed, err := db.ListDeploymentsByStatus(ctx, "failed")
	if err != nil {
		t.Fatalf("ListDeploymentsByStatus: %v", err)
	}
	if len(failed) != 1 {
		t.Errorf("len(failed) = %d, want 1", len(failed))
	}

	// Empty list for unknown status
	empty, err := db.ListDeploymentsByStatus(ctx, "unknown")
	if err != nil {
		t.Fatalf("ListDeploymentsByStatus: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("len(empty) = %d, want 0", len(empty))
	}
}

func TestSQLite_AtomicNextDeployVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	app := createTestApp(t, db, ctx)

	// First deployment should be version 1
	v1, err := db.AtomicNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("AtomicNextDeployVersion: %v", err)
	}
	if v1 != 1 {
		t.Errorf("first version = %d, want 1", v1)
	}

	// Create a deployment at version 1
	dep := &core.Deployment{AppID: app.ID, Version: v1, Status: "success"}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	// Next should be version 2
	v2, err := db.AtomicNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("AtomicNextDeployVersion: %v", err)
	}
	if v2 != 2 {
		t.Errorf("second version = %d, want 2", v2)
	}

	// Another app should start at 1
	app2 := createTestApp(t, db, ctx)
	vOther, err := db.AtomicNextDeployVersion(ctx, app2.ID)
	if err != nil {
		t.Fatalf("AtomicNextDeployVersion: %v", err)
	}
	if vOther != 1 {
		t.Errorf("other app first version = %d, want 1", vOther)
	}
}
