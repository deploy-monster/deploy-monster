package db

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------- helpers ----------

// setupTenantAndProject creates a tenant and project for use in app/domain/deployment tests.
func setupTenantAndProject(t *testing.T, db *SQLiteDB) (tenantID, projectID string) {
	t.Helper()
	ctx := context.Background()

	tenant := &core.Tenant{Name: "ExtraTest", Slug: "extra-test-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	project := &core.Project{TenantID: tenant.ID, Name: "TestProject", Description: "test", Environment: "dev"}
	if err := db.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	return tenant.ID, project.ID
}

// createApp is a test helper that inserts an app and returns it.
func createApp(t *testing.T, db *SQLiteDB, tenantID, projectID, name string) *core.Application {
	t.Helper()
	ctx := context.Background()
	app := &core.Application{
		ProjectID:  projectID,
		TenantID:   tenantID,
		Name:       name,
		Type:       "service",
		SourceType: "image",
		Status:     "running",
		Replicas:   1,
	}
	if err := db.CreateApp(ctx, app); err != nil {
		t.Fatalf("CreateApp(%s): %v", name, err)
	}
	return app
}

// ---------- App CRUD tests ----------

func TestSQLiteExtra_App_CreateAndGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	app := createApp(t, db, tenantID, projID, "my-web-app")

	if app.ID == "" {
		t.Fatal("app ID should be auto-generated")
	}

	got, err := db.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Name != "my-web-app" {
		t.Errorf("expected name 'my-web-app', got %q", got.Name)
	}
	if got.TenantID != tenantID {
		t.Errorf("expected tenant_id %q, got %q", tenantID, got.TenantID)
	}
	if got.ProjectID != projID {
		t.Errorf("expected project_id %q, got %q", projID, got.ProjectID)
	}
	if got.Status != "running" {
		t.Errorf("expected status 'running', got %q", got.Status)
	}
	if got.Replicas != 1 {
		t.Errorf("expected replicas 1, got %d", got.Replicas)
	}
}

func TestSQLiteExtra_App_Update(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	app := createApp(t, db, tenantID, projID, "updatable-app")

	// Update fields
	app.Name = "renamed-app"
	app.Status = "stopped"
	app.Replicas = 3
	app.Branch = "main"
	if err := db.UpdateApp(ctx, app); err != nil {
		t.Fatalf("UpdateApp: %v", err)
	}

	got, err := db.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp after update: %v", err)
	}
	if got.Name != "renamed-app" {
		t.Errorf("expected name 'renamed-app', got %q", got.Name)
	}
	if got.Status != "stopped" {
		t.Errorf("expected status 'stopped', got %q", got.Status)
	}
	if got.Replicas != 3 {
		t.Errorf("expected replicas 3, got %d", got.Replicas)
	}
}

func TestSQLiteExtra_App_UpdateStatus(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	app := createApp(t, db, tenantID, projID, "status-app")

	if err := db.UpdateAppStatus(ctx, app.ID, "deploying"); err != nil {
		t.Fatalf("UpdateAppStatus: %v", err)
	}

	got, err := db.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Status != "deploying" {
		t.Errorf("expected status 'deploying', got %q", got.Status)
	}
}

func TestSQLiteExtra_App_Delete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	app := createApp(t, db, tenantID, projID, "delete-me")

	if err := db.DeleteApp(ctx, app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}

	_, err := db.GetApp(ctx, app.ID)
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSQLiteExtra_App_GetNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetApp(ctx, "nonexistent-id-12345")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ---------- ListByTenant and pagination ----------

func TestSQLiteExtra_App_ListByTenant_Filtering(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantA, projA := setupTenantAndProject(t, db)
	tenantB, projB := setupTenantAndProject(t, db)

	// Create apps for tenant A
	for i := 0; i < 5; i++ {
		createApp(t, db, tenantA, projA, "app-a-"+core.GenerateID()[:6])
	}
	// Create apps for tenant B
	for i := 0; i < 3; i++ {
		createApp(t, db, tenantB, projB, "app-b-"+core.GenerateID()[:6])
	}

	// List tenant A
	appsA, totalA, err := db.ListAppsByTenant(ctx, tenantA, 100, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant A: %v", err)
	}
	if totalA != 5 {
		t.Errorf("expected total 5 for tenant A, got %d", totalA)
	}
	if len(appsA) != 5 {
		t.Errorf("expected 5 apps for tenant A, got %d", len(appsA))
	}

	// List tenant B
	appsB, totalB, err := db.ListAppsByTenant(ctx, tenantB, 100, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant B: %v", err)
	}
	if totalB != 3 {
		t.Errorf("expected total 3 for tenant B, got %d", totalB)
	}
	if len(appsB) != 3 {
		t.Errorf("expected 3 apps for tenant B, got %d", len(appsB))
	}
}

func TestSQLiteExtra_App_ListByTenant_Pagination(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	// Create 10 apps
	for i := 0; i < 10; i++ {
		createApp(t, db, tenantID, projID, "page-app-"+core.GenerateID()[:6])
	}

	tests := []struct {
		name          string
		limit, offset int
		wantLen       int
		wantTotal     int
	}{
		{"first page", 3, 0, 3, 10},
		{"second page", 3, 3, 3, 10},
		{"third page", 3, 6, 3, 10},
		{"last partial page", 3, 9, 1, 10},
		{"beyond end", 3, 15, 0, 10},
		{"all at once", 100, 0, 10, 10},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			apps, total, err := db.ListAppsByTenant(ctx, tenantID, tc.limit, tc.offset)
			if err != nil {
				t.Fatalf("ListAppsByTenant: %v", err)
			}
			if total != tc.wantTotal {
				t.Errorf("total: expected %d, got %d", tc.wantTotal, total)
			}
			if len(apps) != tc.wantLen {
				t.Errorf("len: expected %d, got %d", tc.wantLen, len(apps))
			}
		})
	}
}

func TestSQLiteExtra_App_ListByTenant_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	apps, total, err := db.ListAppsByTenant(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(apps) != 0 {
		t.Errorf("expected 0 apps, got %d", len(apps))
	}
}

func TestSQLiteExtra_App_ListByProject(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	createApp(t, db, tenantID, projID, "proj-app-1")
	createApp(t, db, tenantID, projID, "proj-app-2")

	apps, err := db.ListAppsByProject(ctx, projID)
	if err != nil {
		t.Fatalf("ListAppsByProject: %v", err)
	}
	if len(apps) != 2 {
		t.Errorf("expected 2 apps, got %d", len(apps))
	}
}

// ---------- Domain CRUD tests ----------

func TestSQLiteExtra_Domain_CreateAndGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "domain-app")

	domain := &core.Domain{
		AppID:       app.ID,
		FQDN:        "example.com",
		Type:        "custom",
		DNSProvider: "cloudflare",
		DNSSynced:   false,
		Verified:    false,
	}
	if err := db.CreateDomain(ctx, domain); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	if domain.ID == "" {
		t.Fatal("domain ID should be auto-generated")
	}

	got, err := db.GetDomainByFQDN(ctx, "example.com")
	if err != nil {
		t.Fatalf("GetDomainByFQDN: %v", err)
	}
	if got.AppID != app.ID {
		t.Errorf("expected app_id %q, got %q", app.ID, got.AppID)
	}
	if got.FQDN != "example.com" {
		t.Errorf("expected fqdn 'example.com', got %q", got.FQDN)
	}
	if got.Type != "custom" {
		t.Errorf("expected type 'custom', got %q", got.Type)
	}
	if got.DNSProvider != "cloudflare" {
		t.Errorf("expected dns_provider 'cloudflare', got %q", got.DNSProvider)
	}
}

func TestSQLiteExtra_Domain_GetNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetDomainByFQDN(ctx, "nonexistent.example.com")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteExtra_Domain_ListByApp(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "multi-domain-app")

	fqdns := []string{"a.example.com", "b.example.com", "c.example.com"}
	for _, fqdn := range fqdns {
		d := &core.Domain{AppID: app.ID, FQDN: fqdn, Type: "custom"}
		if err := db.CreateDomain(ctx, d); err != nil {
			t.Fatalf("CreateDomain(%s): %v", fqdn, err)
		}
	}

	domains, err := db.ListDomainsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListDomainsByApp: %v", err)
	}
	if len(domains) != 3 {
		t.Errorf("expected 3 domains, got %d", len(domains))
	}
}

func TestSQLiteExtra_Domain_ListAllDomains(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app1 := createApp(t, db, tenantID, projID, "app-1")
	app2 := createApp(t, db, tenantID, projID, "app-2")

	db.CreateDomain(ctx, &core.Domain{AppID: app1.ID, FQDN: "one.example.com", Type: "custom"})
	db.CreateDomain(ctx, &core.Domain{AppID: app2.ID, FQDN: "two.example.com", Type: "custom"})

	all, err := db.ListAllDomains(ctx)
	if err != nil {
		t.Fatalf("ListAllDomains: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 domains, got %d", len(all))
	}
}

func TestSQLiteExtra_Domain_Delete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "del-domain-app")

	domain := &core.Domain{AppID: app.ID, FQDN: "delete-me.example.com", Type: "custom"}
	db.CreateDomain(ctx, domain)

	if err := db.DeleteDomain(ctx, domain.ID); err != nil {
		t.Fatalf("DeleteDomain: %v", err)
	}

	_, err := db.GetDomainByFQDN(ctx, "delete-me.example.com")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

// ---------- Deployment CRUD tests ----------

func TestSQLiteExtra_Deployment_CreateAndGetLatest(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "deploy-app")

	dep := &core.Deployment{
		AppID:   app.ID,
		Version: 1,
		Image:   "nginx:1.25",
		Status:  "running",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if dep.ID == "" {
		t.Fatal("deployment ID should be auto-generated")
	}

	latest, err := db.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if latest.Version != 1 {
		t.Errorf("expected version 1, got %d", latest.Version)
	}
	if latest.Image != "nginx:1.25" {
		t.Errorf("expected image 'nginx:1.25', got %q", latest.Image)
	}
}

func TestSQLiteExtra_Deployment_GetLatest_ReturnsNewest(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "multi-deploy-app")

	// Create 3 deployments
	for i := 1; i <= 3; i++ {
		dep := &core.Deployment{
			AppID:   app.ID,
			Version: i,
			Image:   "nginx:" + core.GenerateID()[:4],
			Status:  "completed",
		}
		if err := db.CreateDeployment(ctx, dep); err != nil {
			t.Fatalf("CreateDeployment v%d: %v", i, err)
		}
	}

	latest, err := db.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if latest.Version != 3 {
		t.Errorf("expected latest version 3, got %d", latest.Version)
	}
}

func TestSQLiteExtra_Deployment_GetLatest_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetLatestDeployment(ctx, "nonexistent-app-id")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteExtra_Deployment_ListByApp(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "list-deploy-app")

	for i := 1; i <= 5; i++ {
		dep := &core.Deployment{
			AppID:   app.ID,
			Version: i,
			Image:   "app:v" + core.GenerateID()[:4],
			Status:  "completed",
		}
		db.CreateDeployment(ctx, dep)
	}

	// Get all
	all, err := db.ListDeploymentsByApp(ctx, app.ID, 100)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp: %v", err)
	}
	if len(all) != 5 {
		t.Errorf("expected 5 deployments, got %d", len(all))
	}

	// Verify ordering (newest first)
	if len(all) >= 2 && all[0].Version < all[1].Version {
		t.Error("deployments should be ordered newest first")
	}

	// Limit results
	limited, err := db.ListDeploymentsByApp(ctx, app.ID, 2)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp limited: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2 deployments with limit, got %d", len(limited))
	}
}

func TestSQLiteExtra_Deployment_GetNextVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "version-app")

	// First version should be 1
	v, err := db.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetNextDeployVersion: %v", err)
	}
	if v != 1 {
		t.Errorf("expected version 1, got %d", v)
	}

	// After creating version 1, next should be 2
	dep := &core.Deployment{AppID: app.ID, Version: 1, Image: "img:1", Status: "running"}
	db.CreateDeployment(ctx, dep)

	v, err = db.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetNextDeployVersion: %v", err)
	}
	if v != 2 {
		t.Errorf("expected version 2, got %d", v)
	}

	// After creating version 5, next should be 6
	dep2 := &core.Deployment{AppID: app.ID, Version: 5, Image: "img:5", Status: "running"}
	db.CreateDeployment(ctx, dep2)

	v, err = db.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetNextDeployVersion: %v", err)
	}
	if v != 6 {
		t.Errorf("expected version 6 (max+1), got %d", v)
	}
}

// ---------- Tenant extra tests ----------

func TestSQLiteExtra_Tenant_UpdateAndGetBySlug(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "Original", Slug: "original-slug", Status: "active", PlanID: "free"}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	tenant.Name = "Renamed"
	if err := db.UpdateTenant(ctx, tenant); err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}

	got, err := db.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if got.Name != "Renamed" {
		t.Errorf("expected name 'Renamed', got %q", got.Name)
	}
}

// ---------- Transaction rollback test ----------

func TestSQLiteExtra_Tx_RollbackOnError(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	// Create an app, then try to create a duplicate within a transaction
	app := createApp(t, db, tenantID, projID, "tx-test-app")

	// Try to create an app with the same ID (should fail due to PK constraint)
	dupApp := &core.Application{
		ID:         app.ID, // same ID — duplicate
		ProjectID:  projID,
		TenantID:   tenantID,
		Name:       "duplicate",
		Type:       "service",
		SourceType: "image",
		Status:     "running",
		Replicas:   1,
	}
	err := db.CreateApp(ctx, dupApp)
	if err == nil {
		t.Error("expected error when creating app with duplicate ID")
	}
}

// ---------- Ping test ----------

func TestSQLiteExtra_Ping(t *testing.T) {
	db := testDB(t)
	if err := db.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// ---------- Close and reopen test ----------

func TestSQLiteExtra_CloseAndReopen(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/reopen.db"

	db1, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}

	ctx := context.Background()
	tenant := &core.Tenant{Name: "Persist", Slug: "persist-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	db1.CreateTenant(ctx, tenant)
	db1.Close()

	// Reopen
	db2, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer db2.Close()

	got, err := db2.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant after reopen: %v", err)
	}
	if got.Name != "Persist" {
		t.Errorf("expected name 'Persist', got %q", got.Name)
	}
}

// ---------- Project CRUD tests ----------

func TestSQLiteExtra_Project_CreateAndGet(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "ProjTenant", Slug: "proj-tenant-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)

	proj := &core.Project{
		TenantID:    tenant.ID,
		Name:        "My Project",
		Description: "A test project",
		Environment: "staging",
	}
	if err := db.CreateProject(ctx, proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if proj.ID == "" {
		t.Fatal("project ID should be auto-generated")
	}

	got, err := db.GetProject(ctx, proj.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if got.Name != "My Project" {
		t.Errorf("expected name 'My Project', got %q", got.Name)
	}
	if got.Environment != "staging" {
		t.Errorf("expected environment 'staging', got %q", got.Environment)
	}
}

func TestSQLiteExtra_Project_GetNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetProject(ctx, "nonexistent-proj")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteExtra_Project_ListByTenant(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "ListProjT", Slug: "list-proj-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)

	for _, name := range []string{"Alpha", "Bravo", "Charlie"} {
		p := &core.Project{TenantID: tenant.ID, Name: name, Environment: "production"}
		db.CreateProject(ctx, p)
	}

	projects, err := db.ListProjectsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListProjectsByTenant: %v", err)
	}
	if len(projects) != 3 {
		t.Errorf("expected 3 projects, got %d", len(projects))
	}
}

func TestSQLiteExtra_Project_Delete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "DelProjT", Slug: "del-proj-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)

	proj := &core.Project{TenantID: tenant.ID, Name: "ToDelete", Environment: "dev"}
	db.CreateProject(ctx, proj)

	if err := db.DeleteProject(ctx, proj.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	_, err := db.GetProject(ctx, proj.ID)
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

// ---------- CreateTenantWithDefaults ----------

func TestSQLiteExtra_CreateTenantWithDefaults(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, err := db.CreateTenantWithDefaults(ctx, "Default Tenant", "default-"+core.GenerateID()[:8])
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}
	if tenantID == "" {
		t.Fatal("expected non-empty tenant ID")
	}

	// Should have a default project
	projects, err := db.ListProjectsByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListProjectsByTenant: %v", err)
	}
	if len(projects) != 1 {
		t.Errorf("expected 1 default project, got %d", len(projects))
	}
	if projects[0].Name != "Default" {
		t.Errorf("expected project name 'Default', got %q", projects[0].Name)
	}
}
