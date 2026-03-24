package db

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// User CRUD — additional coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLiteCoverage_User_GetByID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		Email: "getbyid@example.com", PasswordHash: "$2a$12$fakehash",
		Name: "GetByID User", Status: "active",
	}
	db.CreateUser(ctx, user)

	got, err := db.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.Email != "getbyid@example.com" {
		t.Errorf("email = %q, want getbyid@example.com", got.Email)
	}
}

func TestSQLiteCoverage_User_GetByID_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetUser(ctx, "nonexistent-user-id")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteCoverage_User_UpdateUser(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		Email: "update@example.com", PasswordHash: "$2a$12$fakehash",
		Name: "Before Update", Status: "active",
	}
	db.CreateUser(ctx, user)

	user.Name = "After Update"
	user.Status = "suspended"
	if err := db.UpdateUser(ctx, user); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	got, err := db.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser after update: %v", err)
	}
	if got.Name != "After Update" {
		t.Errorf("name = %q, want 'After Update'", got.Name)
	}
	if got.Status != "suspended" {
		t.Errorf("status = %q, want suspended", got.Status)
	}
}

func TestSQLiteCoverage_User_UpdatePassword(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		Email: "passwd@example.com", PasswordHash: "$2a$12$oldhash",
		Name: "Password User", Status: "active",
	}
	db.CreateUser(ctx, user)

	if err := db.UpdatePassword(ctx, user.ID, "$2a$12$newhash"); err != nil {
		t.Fatalf("UpdatePassword: %v", err)
	}

	got, err := db.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.PasswordHash != "$2a$12$newhash" {
		t.Errorf("password hash not updated")
	}
}

func TestSQLiteCoverage_User_UpdateLastLogin(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		Email: "login@example.com", PasswordHash: "$2a$12$hash",
		Name: "Login User", Status: "active",
	}
	db.CreateUser(ctx, user)

	if err := db.UpdateLastLogin(ctx, user.ID); err != nil {
		t.Fatalf("UpdateLastLogin: %v", err)
	}

	got, err := db.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if got.LastLoginAt == nil {
		t.Error("last_login_at should be set after UpdateLastLogin")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CreateUserWithMembership
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLiteCoverage_CreateUserWithMembership(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Create tenant first
	tenantID, err := db.CreateTenantWithDefaults(ctx, "Membership Tenant", "membership-"+core.GenerateID()[:8])
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	// Get the admin role ID
	roles, err := db.ListRoles(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) == 0 {
		t.Fatal("expected at least one role")
	}
	roleID := roles[0].ID

	userID, err := db.CreateUserWithMembership(ctx, "member@test.com", "$2a$12$hash", "Member User", "active", tenantID, roleID)
	if err != nil {
		t.Fatalf("CreateUserWithMembership: %v", err)
	}
	if userID == "" {
		t.Fatal("expected non-empty user ID")
	}

	// Verify membership
	tm, err := db.GetUserMembership(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserMembership: %v", err)
	}
	if tm.TenantID != tenantID {
		t.Errorf("TenantID = %q, want %q", tm.TenantID, tenantID)
	}
	if tm.RoleID != roleID {
		t.Errorf("RoleID = %q, want %q", tm.RoleID, roleID)
	}
}

func TestSQLiteCoverage_GetUserMembership_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetUserMembership(ctx, "nonexistent-user")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Roles
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLiteCoverage_ListRoles(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := db.CreateTenantWithDefaults(ctx, "Role Tenant", "role-"+core.GenerateID()[:8])

	roles, err := db.ListRoles(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) < 6 {
		t.Errorf("expected at least 6 built-in roles, got %d", len(roles))
	}
}

func TestSQLiteCoverage_GetRole(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Get the built-in Admin role by known ID
	role, err := db.GetRole(ctx, "role_admin")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if role.Name != "Admin" {
		t.Errorf("name = %q, want Admin", role.Name)
	}
	if !role.IsBuiltin {
		t.Error("expected IsBuiltin = true")
	}
}

func TestSQLiteCoverage_GetRole_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetRole(ctx, "nonexistent-role")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// App — UpdateApp
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLiteCoverage_App_UpdateApp(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	app := createApp(t, db, tenantID, projID, "update-test-app")
	app.Name = "new-name"
	app.Replicas = 5
	app.Branch = "develop"
	app.Dockerfile = "Dockerfile.prod"

	if err := db.UpdateApp(ctx, app); err != nil {
		t.Fatalf("UpdateApp: %v", err)
	}

	got, err := db.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Name != "new-name" {
		t.Errorf("Name = %q, want new-name", got.Name)
	}
	if got.Replicas != 5 {
		t.Errorf("Replicas = %d, want 5", got.Replicas)
	}
	if got.Branch != "develop" {
		t.Errorf("Branch = %q, want develop", got.Branch)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Tx — commit and rollback
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLiteCoverage_Tx_Success(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, "SELECT 1")
		return err
	})
	if err != nil {
		t.Fatalf("Tx success case: %v", err)
	}
}

func TestSQLiteCoverage_Tx_Rollback(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "TxTest", Slug: "txtest-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)

	// Tx that fails partway through should rollback
	err := db.Tx(ctx, func(tx *sql.Tx) error {
		tx.ExecContext(ctx, "INSERT INTO projects (id, tenant_id, name) VALUES (?, ?, ?)",
			"proj-txtest", tenant.ID, "TxProject")
		return core.ErrNotFound // Force rollback
	})
	if err != core.ErrNotFound {
		t.Fatalf("expected ErrNotFound from tx, got %v", err)
	}

	// Project should not exist
	_, err = db.GetProject(ctx, "proj-txtest")
	if err != core.ErrNotFound {
		t.Errorf("project should not exist after rollback, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Tenant — GetNotFound edge case
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLiteCoverage_Tenant_GetNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetTenant(ctx, "nonexistent-tenant")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSQLiteCoverage_Tenant_GetBySlugNotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetTenantBySlug(ctx, "nonexistent-slug")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module — Init with valid config
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Init_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	m := New()

	c := &core.Core{
		Logger: testLogger(),
		Config: &core.Config{
			Database: core.DatabaseConfig{Path: dir + "/init-test.db"},
		},
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	defer m.Stop(nil)

	if m.Store() == nil {
		t.Error("Store() should not be nil after Init")
	}
	if m.SQLite() == nil {
		t.Error("SQLite() should not be nil after Init")
	}
	if m.Bolt() == nil {
		t.Error("Bolt() should not be nil after Init")
	}
	if c.Store == nil {
		t.Error("core.Store should be set after Init")
	}
	if c.DB == nil {
		t.Error("core.DB should be set after Init")
	}
}

func testLogger() *slog.Logger {
	return slog.Default()
}
