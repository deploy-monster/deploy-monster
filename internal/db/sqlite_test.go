package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func testDB(t *testing.T) *SQLiteDB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")

	db, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSQLite_Open_And_Migrate(t *testing.T) {
	db := testDB(t)
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestSQLite_Migration_Applied(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Check that roles table was seeded
	var count int
	err := db.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM roles WHERE is_builtin = 1").Scan(&count)
	if err != nil {
		t.Fatalf("query roles: %v", err)
	}
	if count != 6 {
		t.Errorf("expected 6 built-in roles, got %d", count)
	}
}

func TestSQLite_Tenant_CRUD(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Create
	tenant := &core.Tenant{
		Name:   "Test Team",
		Slug:   "test-team",
		Status: "active",
		PlanID: "free",
	}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if tenant.ID == "" {
		t.Error("tenant ID should be auto-generated")
	}

	// Read
	got, err := db.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if got.Name != "Test Team" {
		t.Errorf("expected name 'Test Team', got %q", got.Name)
	}
	if got.Slug != "test-team" {
		t.Errorf("expected slug 'test-team', got %q", got.Slug)
	}

	// Read by slug
	got2, err := db.GetTenantBySlug(ctx, "test-team")
	if err != nil {
		t.Fatalf("GetTenantBySlug: %v", err)
	}
	if got2.ID != tenant.ID {
		t.Error("GetTenantBySlug should return same tenant")
	}

	// Delete
	if err := db.DeleteTenant(ctx, tenant.ID); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}
}

func TestSQLite_User_CRUD(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		Email:        "test@example.com",
		PasswordHash: "$2a$12$fakehashhere",
		Name:         "Test User",
		Status:       "active",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := db.GetUserByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.Name != "Test User" {
		t.Errorf("expected 'Test User', got %q", got.Name)
	}

	count, err := db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 user, got %d", count)
	}
}

func TestSQLite_App_ListByTenant(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Create tenant and project first
	tenant := &core.Tenant{Name: "T", Slug: "t", Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)

	_, err := db.DB().ExecContext(ctx,
		"INSERT INTO projects (id, tenant_id, name) VALUES ('proj1', ?, 'P')", tenant.ID)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Create apps
	for _, name := range []string{"app-a", "app-b", "app-c"} {
		app := &core.Application{
			ProjectID: "proj1", TenantID: tenant.ID, Name: name,
			Type: "service", SourceType: "image", Status: "running", Replicas: 1,
		}
		if err := db.CreateApp(ctx, app); err != nil {
			t.Fatalf("CreateApp %s: %v", name, err)
		}
	}

	apps, total, err := db.ListAppsByTenant(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant: %v", err)
	}
	if total != 3 {
		t.Errorf("expected total 3, got %d", total)
	}
	if len(apps) != 3 {
		t.Errorf("expected 3 apps, got %d", len(apps))
	}
}

func TestSQLite_Deployment_Versioning(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Setup tenant, project, app
	tenant := &core.Tenant{Name: "T", Slug: "t2", Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)
	db.DB().ExecContext(ctx, "INSERT INTO projects (id, tenant_id, name) VALUES ('p2', ?, 'P')", tenant.ID)
	app := &core.Application{
		ProjectID: "p2", TenantID: tenant.ID, Name: "myapp",
		Type: "service", SourceType: "image", Status: "running", Replicas: 1,
	}
	db.CreateApp(ctx, app)

	// First deployment version should be 1
	v, err := db.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetNextDeployVersion: %v", err)
	}
	if v != 1 {
		t.Errorf("expected version 1, got %d", v)
	}

	// Create deployment
	dep := &core.Deployment{AppID: app.ID, Version: 1, Image: "nginx:latest", Status: "running"}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	// Next version should be 2
	v, _ = db.GetNextDeployVersion(ctx, app.ID)
	if v != 2 {
		t.Errorf("expected version 2, got %d", v)
	}
}

func TestSQLite_DatabasePath_Created(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "test.db")

	// Subdir doesn't exist yet — SQLite should handle creating the file
	// (but not the directory). Let's create the dir first.
	os.MkdirAll(filepath.Dir(path), 0755)

	db, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	defer db.Close()

	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}
