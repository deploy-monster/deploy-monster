package db

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// Module Init — unsupported driver error path
// =============================================================================

func TestModule_Init_UnsupportedDriver(t *testing.T) {
	dir := t.TempDir()
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Driver: "mysql",
				Path:   dir + "/unsupported.db",
			},
		},
	}

	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for unsupported driver")
	}
	if err.Error() != "unsupported database driver: mysql (supported: sqlite, postgres)" {
		t.Errorf("error = %q", err)
	}
}

// =============================================================================
// Module Init — postgres driver (will fail but exercises the switch branch)
// =============================================================================

func TestModule_Init_PostgresDriver_InvalidDSN(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Driver: "postgres",
				URL:    "postgres://invalid:5432/nonexistent?sslmode=disable&connect_timeout=1",
				Path:   "/tmp/does-not-matter.db",
			},
		},
	}

	// This will fail because there is no PostgreSQL server
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for invalid postgres DSN")
	}
}

func TestModule_Init_PostgreSQLDriver_InvalidDSN(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Driver: "postgresql",
				URL:    "postgres://invalid:5432/nonexistent?sslmode=disable&connect_timeout=1",
				Path:   "/tmp/does-not-matter.db",
			},
		},
	}

	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for invalid postgresql DSN")
	}
}

// =============================================================================
// Module Init — empty driver defaults to sqlite
// =============================================================================

func TestModule_Init_EmptyDriver_DefaultsSQLite(t *testing.T) {
	dir := t.TempDir()
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Driver: "", // Empty driver should default to sqlite
				Path:   dir + "/default-driver.db",
			},
		},
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init with empty driver: %v", err)
	}
	defer m.Stop(nil)

	if m.SQLite() == nil {
		t.Error("SQLite should be initialized when driver is empty")
	}
}

// =============================================================================
// SQLite closed DB error paths for list functions not yet covered
// =============================================================================

func TestSQLite_ListAppsByProject_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-proj.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.ListAppsByProject(context.Background(), "proj-1")
	if err == nil {
		t.Error("expected error for ListAppsByProject on closed DB")
	}
}

func TestSQLite_ListDeploymentsByApp_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-deploy.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.ListDeploymentsByApp(context.Background(), "app-1", 10)
	if err == nil {
		t.Error("expected error for ListDeploymentsByApp on closed DB")
	}
}

func TestSQLite_GetNextDeployVersion_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-version.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetNextDeployVersion(context.Background(), "app-1")
	if err == nil {
		t.Error("expected error for GetNextDeployVersion on closed DB")
	}
}

func TestSQLite_ListDomainsByApp_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-dom.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.ListDomainsByApp(context.Background(), "app-1")
	if err == nil {
		t.Error("expected error for ListDomainsByApp on closed DB")
	}
}

func TestSQLite_ListAllDomains_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-alldom.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.ListAllDomains(context.Background())
	if err == nil {
		t.Error("expected error for ListAllDomains on closed DB")
	}
}

func TestSQLite_ListInvitesByTenant_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-invite.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.ListInvitesByTenant(context.Background(), "t-1")
	if err == nil {
		t.Error("expected error for ListInvitesByTenant on closed DB")
	}
}

func TestSQLite_ListSecretsByTenant_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-secret.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.ListSecretsByTenant(context.Background(), "t-1")
	if err == nil {
		t.Error("expected error for ListSecretsByTenant on closed DB")
	}
}

func TestSQLite_ListRoles_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-roles.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.ListRoles(context.Background(), "t-1")
	if err == nil {
		t.Error("expected error for ListRoles on closed DB")
	}
}

func TestSQLite_ListProjectsByTenant_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-proj-list.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.ListProjectsByTenant(context.Background(), "t-1")
	if err == nil {
		t.Error("expected error for ListProjectsByTenant on closed DB")
	}
}

// =============================================================================
// SQLite CRUD closed-DB error paths for single-row operations
// =============================================================================

func TestSQLite_GetApp_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-getapp.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetApp(context.Background(), "app-1")
	if err == nil {
		t.Error("expected error for GetApp on closed DB")
	}
}

func TestSQLite_GetLatestDeployment_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-latestdep.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetLatestDeployment(context.Background(), "app-1")
	if err == nil {
		t.Error("expected error for GetLatestDeployment on closed DB")
	}
}

func TestSQLite_GetDomainByFQDN_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-domfqdn.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetDomainByFQDN(context.Background(), "x.example.com")
	if err == nil {
		t.Error("expected error for GetDomainByFQDN on closed DB")
	}
}

func TestSQLite_GetTenant_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-gettenant.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetTenant(context.Background(), "t-1")
	if err == nil {
		t.Error("expected error for GetTenant on closed DB")
	}
}

func TestSQLite_GetTenantBySlug_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-tenantslug.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetTenantBySlug(context.Background(), "slug")
	if err == nil {
		t.Error("expected error for GetTenantBySlug on closed DB")
	}
}

func TestSQLite_GetUser_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-getuser.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetUser(context.Background(), "u-1")
	if err == nil {
		t.Error("expected error for GetUser on closed DB")
	}
}

func TestSQLite_GetUserByEmail_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-useremail.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetUserByEmail(context.Background(), "a@b.com")
	if err == nil {
		t.Error("expected error for GetUserByEmail on closed DB")
	}
}

func TestSQLite_GetRole_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-getrole.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetRole(context.Background(), "role-1")
	if err == nil {
		t.Error("expected error for GetRole on closed DB")
	}
}

func TestSQLite_GetProject_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-getproj.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetProject(context.Background(), "p-1")
	if err == nil {
		t.Error("expected error for GetProject on closed DB")
	}
}

func TestSQLite_GetUserMembership_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-membership.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.GetUserMembership(context.Background(), "u-1")
	if err == nil {
		t.Error("expected error for GetUserMembership on closed DB")
	}
}

func TestSQLite_CountUsers_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-countusers.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.CountUsers(context.Background())
	if err == nil {
		t.Error("expected error for CountUsers on closed DB")
	}
}

func TestSQLite_Ping_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-ping.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.Ping(context.Background())
	if err == nil {
		t.Error("expected error for Ping on closed DB")
	}
}

// =============================================================================
// SQLite write operations on closed DB (Tx error propagation)
// =============================================================================

func TestSQLite_CreateTenant_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-createtenant.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateTenant(context.Background(), &core.Tenant{
		Name: "test", Slug: "test-closed", Status: "active", PlanID: "free",
	})
	if err == nil {
		t.Error("expected error for CreateTenant on closed DB")
	}
}

func TestSQLite_UpdateTenant_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-uptenant.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.UpdateTenant(context.Background(), &core.Tenant{ID: "t-1"})
	if err == nil {
		t.Error("expected error for UpdateTenant on closed DB")
	}
}

func TestSQLite_DeleteTenant_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-deltenant.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.DeleteTenant(context.Background(), "t-1")
	if err == nil {
		t.Error("expected error for DeleteTenant on closed DB")
	}
}

func TestSQLite_CreateUser_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-createuser.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateUser(context.Background(), &core.User{
		Email: "x@y.com", PasswordHash: "h", Name: "n", Status: "active",
	})
	if err == nil {
		t.Error("expected error for CreateUser on closed DB")
	}
}

func TestSQLite_UpdateUser_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-upuser.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.UpdateUser(context.Background(), &core.User{ID: "u-1"})
	if err == nil {
		t.Error("expected error for UpdateUser on closed DB")
	}
}

func TestSQLite_UpdatePassword_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-uppw.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.UpdatePassword(context.Background(), "u-1", "newhash")
	if err == nil {
		t.Error("expected error for UpdatePassword on closed DB")
	}
}

func TestSQLite_UpdateLastLogin_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-login.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.UpdateLastLogin(context.Background(), "u-1")
	if err == nil {
		t.Error("expected error for UpdateLastLogin on closed DB")
	}
}

func TestSQLite_UpdateApp_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-upapp.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.UpdateApp(context.Background(), &core.Application{ID: "a-1"})
	if err == nil {
		t.Error("expected error for UpdateApp on closed DB")
	}
}

func TestSQLite_UpdateAppStatus_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-appstatus.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.UpdateAppStatus(context.Background(), "a-1", "stopped")
	if err == nil {
		t.Error("expected error for UpdateAppStatus on closed DB")
	}
}

func TestSQLite_DeleteApp_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-delapp.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.DeleteApp(context.Background(), "a-1")
	if err == nil {
		t.Error("expected error for DeleteApp on closed DB")
	}
}

func TestSQLite_CreateDeployment_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-createdep.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateDeployment(context.Background(), &core.Deployment{
		AppID: "a-1", Version: 1, Image: "x", Status: "pending",
		TriggeredBy: "test", Strategy: "recreate",
	})
	if err == nil {
		t.Error("expected error for CreateDeployment on closed DB")
	}
}

func TestSQLite_CreateDomain_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-createdom.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateDomain(context.Background(), &core.Domain{
		AppID: "a-1", FQDN: "test.com", Type: "custom",
	})
	if err == nil {
		t.Error("expected error for CreateDomain on closed DB")
	}
}

func TestSQLite_DeleteDomain_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-deldom.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.DeleteDomain(context.Background(), "d-1")
	if err == nil {
		t.Error("expected error for DeleteDomain on closed DB")
	}
}

func TestSQLite_CreateProject_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-createproj.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateProject(context.Background(), &core.Project{
		TenantID: "t-1", Name: "proj", Environment: "dev",
	})
	if err == nil {
		t.Error("expected error for CreateProject on closed DB")
	}
}

func TestSQLite_DeleteProject_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-delproj.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.DeleteProject(context.Background(), "p-1")
	if err == nil {
		t.Error("expected error for DeleteProject on closed DB")
	}
}

func TestSQLite_CreateAuditLog_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-audit.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateAuditLog(context.Background(), &core.AuditEntry{
		TenantID: "t-1", Action: "test", ResourceType: "x", ResourceID: "1",
	})
	if err == nil {
		t.Error("expected error for CreateAuditLog on closed DB")
	}
}

func TestSQLite_CreateSecret_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-createsecret.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateSecret(context.Background(), &core.Secret{
		TenantID: "t-1", Name: "KEY", Type: "env", Scope: "tenant",
	})
	if err == nil {
		t.Error("expected error for CreateSecret on closed DB")
	}
}

func TestSQLite_CreateSecretVersion_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-secretver.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateSecretVersion(context.Background(), &core.SecretVersion{
		SecretID: "s-1", Version: 1, ValueEnc: "enc",
	})
	if err == nil {
		t.Error("expected error for CreateSecretVersion on closed DB")
	}
}

func TestSQLite_CreateInvite_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-createinvite.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.CreateInvite(context.Background(), &core.Invitation{
		TenantID: "t-1", Email: "x@y.com", RoleID: "r", TokenHash: "h", Status: "pending",
	})
	if err == nil {
		t.Error("expected error for CreateInvite on closed DB")
	}
}

func TestSQLite_CreateTenantWithDefaults_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-tenantdef.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.CreateTenantWithDefaults(context.Background(), "test", "test-slug")
	if err == nil {
		t.Error("expected error for CreateTenantWithDefaults on closed DB")
	}
}

func TestSQLite_CreateUserWithMembership_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-usermem.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, err = db.CreateUserWithMembership(context.Background(), "a@b.com", "hash", "name", "active", "t-1", "r-1")
	if err == nil {
		t.Error("expected error for CreateUserWithMembership on closed DB")
	}
}

// =============================================================================
// Additional functional tests to cover partial coverage
// =============================================================================

func TestSQLite_GetLatestDeployment_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetLatestDeployment(ctx, "nonexistent-app")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLite_GetDomainByFQDN_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetDomainByFQDN(ctx, "notfound.example.com")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLite_GetProject_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetProject(ctx, "nonexistent-project")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLite_GetUserByEmail_NotFound_Extra(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetUserByEmail(ctx, "doesnotexist@example.com")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLite_CountUsers(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	count, err := db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 users initially, got %d", count)
	}

	// Create a user
	db.CreateUser(ctx, &core.User{
		Email: "count@example.com", PasswordHash: "hash",
		Name: "Counter", Status: "active",
	})

	count, err = db.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 user, got %d", count)
	}
}

func TestSQLite_DeleteTenant(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "DelMe", Slug: "del-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)

	if err := db.DeleteTenant(ctx, tenant.ID); err != nil {
		t.Fatalf("DeleteTenant: %v", err)
	}

	_, err := db.GetTenant(ctx, tenant.ID)
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSQLite_UpdateTenant(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "Original", Slug: "up-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)

	tenant.Name = "Updated"
	tenant.Status = "suspended"
	if err := db.UpdateTenant(ctx, tenant); err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}

	got, err := db.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant after update: %v", err)
	}
	if got.Name != "Updated" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Status != "suspended" {
		t.Errorf("Status = %q", got.Status)
	}
}

func TestSQLite_DeleteApp(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projectID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projectID, "delete-me-app")

	if err := db.DeleteApp(ctx, app.ID); err != nil {
		t.Fatalf("DeleteApp: %v", err)
	}

	_, err := db.GetApp(ctx, app.ID)
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSQLite_DeleteDomain(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projectID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projectID, "dom-del-app")

	dom := &core.Domain{AppID: app.ID, FQDN: "delete.example.com", Type: "custom", DNSProvider: "cf"}
	db.CreateDomain(ctx, dom)

	if err := db.DeleteDomain(ctx, dom.ID); err != nil {
		t.Fatalf("DeleteDomain: %v", err)
	}

	_, err := db.GetDomainByFQDN(ctx, "delete.example.com")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSQLite_DeleteProject(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	proj := &core.Project{TenantID: tenantID, Name: "DeleteMe", Environment: "dev"}
	db.CreateProject(ctx, proj)

	if err := db.DeleteProject(ctx, proj.ID); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	_, err := db.GetProject(ctx, proj.ID)
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestSQLite_UpdateAppStatus(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projectID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projectID, "status-app")

	if err := db.UpdateAppStatus(ctx, app.ID, "stopped"); err != nil {
		t.Fatalf("UpdateAppStatus: %v", err)
	}

	got, err := db.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Status != "stopped" {
		t.Errorf("Status = %q, want stopped", got.Status)
	}
}

func TestSQLite_GetUserByEmail(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		Email: "byemail@example.com", PasswordHash: "hash",
		Name: "Email User", Status: "active",
	}
	db.CreateUser(ctx, user)

	got, err := db.GetUserByEmail(ctx, "byemail@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got.Name != "Email User" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestSQLite_GetTenantBySlug(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	slug := "slug-" + core.GenerateID()[:8]
	tenant := &core.Tenant{Name: "Sluggy", Slug: slug, Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)

	got, err := db.GetTenantBySlug(ctx, slug)
	if err != nil {
		t.Fatalf("GetTenantBySlug: %v", err)
	}
	if got.Name != "Sluggy" {
		t.Errorf("Name = %q", got.Name)
	}
}

func TestSQLite_GetNextDeployVersion_NoDeployments(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	version, err := db.GetNextDeployVersion(ctx, "app-with-no-deployments")
	if err != nil {
		t.Fatalf("GetNextDeployVersion: %v", err)
	}
	if version != 1 {
		t.Errorf("expected version 1 for first deployment, got %d", version)
	}
}

// =============================================================================
// BoltStore — TTL expiration
// =============================================================================

func TestBoltStore_Get_ExpiredEntry(t *testing.T) {
	store := testBolt(t)

	// Set with 1 second TTL
	if err := store.Set("sessions", "ttl-key", "value", 1); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Wait for expiration
	time.Sleep(1100 * time.Millisecond)

	var val string
	err := store.Get("sessions", "ttl-key", &val)
	if err == nil {
		t.Error("expected error for expired key")
	}
}

// =============================================================================
// BoltStore — Delete valid key
// =============================================================================

func TestBoltStore_Delete_ExistingKey(t *testing.T) {
	store := testBolt(t)

	store.Set("sessions", "delete-me", "val", 0)

	if err := store.Delete("sessions", "delete-me"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var val string
	err := store.Get("sessions", "delete-me", &val)
	if err == nil {
		t.Error("expected error after deleting key")
	}
}

// =============================================================================
// SQLiteDB — DB() accessor
// =============================================================================

func TestSQLite_DB_Accessor(t *testing.T) {
	db := testDB(t)

	rawDB := db.DB()
	if rawDB == nil {
		t.Fatal("DB() should return non-nil *sql.DB")
	}

	// Verify it works by executing a simple query
	var n int
	err := rawDB.QueryRow("SELECT 1").Scan(&n)
	if err != nil {
		t.Fatalf("query through DB(): %v", err)
	}
	if n != 1 {
		t.Errorf("SELECT 1 = %d", n)
	}
}

// =============================================================================
// SQLiteDB — Close is idempotent-ish
// =============================================================================

func TestSQLite_Close_Twice(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/close-twice.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}

	// First close
	if err := db.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}

	// Second close — should not panic
	_ = db.Close()
}
