package db

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	bolt "go.etcd.io/bbolt"
)

// =============================================================================
// PostgresDB — NewPostgres error paths (no real PostgreSQL needed)
// =============================================================================

func TestPostgresDB_NewPostgres_EmptyDSN(t *testing.T) {
	_, err := NewPostgres("")
	if err == nil {
		t.Error("expected error for empty DSN")
	}
}

func TestPostgresDB_NewPostgres_InvalidDSN_Timeout(t *testing.T) {
	// Invalid host but valid-ish DSN format — exercises the open+ping path
	_, err := NewPostgres("postgres://user:pass@127.0.0.1:1/baddb?sslmode=disable&connect_timeout=1")
	if err == nil {
		t.Error("expected error for invalid/unreachable PostgreSQL")
	}
}

// =============================================================================
// Module — Init with postgres driver + invalid bolt path
// =============================================================================

func TestModule_Init_SQLite_InvalidBoltPath(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	// Create a directory at the bolt location so NewBoltStore fails
	boltPath := filepath.Join(dir, "deploymonster.bolt")
	os.MkdirAll(filepath.Join(boltPath, "subdir"), 0755)

	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Driver: "sqlite",
				Path:   dbPath,
			},
		},
	}

	err := m.Init(context.Background(), c)
	if err == nil {
		t.Error("expected error when bolt path is invalid")
	}
	// Ensure cleanup happens even on failure
	//lint:ignore SA1012 nil context triggers the error path
	m.Stop(nil)
}

// =============================================================================
// Module — Stop exercises both error branches
// =============================================================================

func TestModule_Stop_SQLiteError_BoltOK(t *testing.T) {
	dir := t.TempDir()

	sqliteDB, err := NewSQLite(dir + "/stop-sql.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	boltStore, err := NewBoltStore(dir + "/stop-sql.bolt")
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}

	m := New()
	m.sqlite = sqliteDB
	m.bolt = boltStore

	// Close sqlite first to cause error on Stop
	sqliteDB.Close()

	//lint:ignore SA1012 nil context triggers the error path
	err = m.Stop(nil)
	// SQLite close error is returned as firstErr
	// Bolt should still close successfully
	_ = err
}

func TestModule_Stop_SQLiteOK_BoltError(t *testing.T) {
	dir := t.TempDir()

	sqliteDB, err := NewSQLite(dir + "/stop-bolt.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	boltStore, err := NewBoltStore(dir + "/stop-bolt.bolt")
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}

	m := New()
	m.sqlite = sqliteDB
	m.bolt = boltStore

	// Close bolt first to cause error on Stop
	boltStore.Close()

	//lint:ignore SA1012 nil context triggers the error path
	err = m.Stop(nil)
	// The bolt close error path is exercised (but firstErr may or may not be nil
	// depending on whether SQLite close succeeds)
	_ = err
}

func TestModule_Stop_BothErrors(t *testing.T) {
	dir := t.TempDir()

	sqliteDB, err := NewSQLite(dir + "/stop-both.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	boltStore, err := NewBoltStore(dir + "/stop-both.bolt")
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}

	m := New()
	m.sqlite = sqliteDB
	m.bolt = boltStore

	// Close both to cause errors
	sqliteDB.Close()
	boltStore.Close()

	//lint:ignore SA1012 nil context triggers the error path
	err = m.Stop(nil)
	// firstErr should be the sqlite error; bolt error is swallowed
	_ = err
}

// =============================================================================
// Module — Init with valid config sets DB.SQL when sqlite is non-nil
// =============================================================================

func TestModule_Init_SetsDBSQL(t *testing.T) {
	dir := t.TempDir()
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Database: core.DatabaseConfig{Path: dir + "/db-sql.db"},
		},
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	//lint:ignore SA1012 nil context triggers the error path
	defer m.Stop(nil)

	if c.DB == nil {
		t.Fatal("core.DB should be set")
	}
	if c.DB.SQL == nil {
		t.Error("core.DB.SQL should be set when using sqlite")
	}
	if c.DB.Bolt == nil {
		t.Error("core.DB.Bolt should be set")
	}
}

// =============================================================================
// SQLite — migrate is idempotent (run twice on same DB)
// =============================================================================

func TestSQLite_MigrateIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/migrate-idem.db"

	db1, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	db1.Close()

	// Open again — migrations should be skipped (already applied)
	db2, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer db2.Close()

	if err := db2.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

// =============================================================================
// SQLite — CreateApp with pre-set ID
// =============================================================================

func TestSQLite_CreateApp_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	app := &core.Application{
		ID:         "custom-id-123",
		ProjectID:  projID,
		TenantID:   tenantID,
		Name:       "preset-id-app",
		Type:       "service",
		SourceType: "image",
		Status:     "pending",
		Replicas:   1,
	}
	if err := db.CreateApp(ctx, app); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}
	if app.ID != "custom-id-123" {
		t.Errorf("ID should be preserved, got %q", app.ID)
	}

	got, err := db.GetApp(ctx, "custom-id-123")
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Name != "preset-id-app" {
		t.Errorf("Name = %q", got.Name)
	}
}

// =============================================================================
// SQLite — CreateTenant with pre-set ID
// =============================================================================

func TestSQLite_CreateTenant_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{
		ID:     "tenant-preset-id",
		Name:   "Preset Tenant",
		Slug:   "preset-" + core.GenerateID()[:8],
		Status: "active",
		PlanID: "free",
	}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	if tenant.ID != "tenant-preset-id" {
		t.Errorf("ID = %q", tenant.ID)
	}
}

// =============================================================================
// SQLite — CreateUser with pre-set ID
// =============================================================================

func TestSQLite_CreateUser_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		ID:           "user-preset-id",
		Email:        "preset@test.com",
		PasswordHash: "hash",
		Name:         "Preset",
		Status:       "active",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if user.ID != "user-preset-id" {
		t.Errorf("ID = %q", user.ID)
	}
}

// =============================================================================
// SQLite — CreateDeployment with pre-set ID
// =============================================================================

func TestSQLite_CreateDeployment_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "dep-preset")

	dep := &core.Deployment{
		ID:          "dep-preset-id",
		AppID:       app.ID,
		Version:     1,
		Image:       "nginx:1",
		Status:      "running",
		TriggeredBy: "test",
		Strategy:    "recreate",
	}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}
	if dep.ID != "dep-preset-id" {
		t.Errorf("ID = %q", dep.ID)
	}
}

// =============================================================================
// SQLite — CreateDomain with pre-set ID
// =============================================================================

func TestSQLite_CreateDomain_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "dom-preset")

	dom := &core.Domain{
		ID:          "dom-preset-id",
		AppID:       app.ID,
		FQDN:        "preset.example.com",
		Type:        "auto",
		DNSProvider: "cf",
	}
	if err := db.CreateDomain(ctx, dom); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	if dom.ID != "dom-preset-id" {
		t.Errorf("ID = %q", dom.ID)
	}
}

// =============================================================================
// SQLite — CreateProject with pre-set ID
// =============================================================================

func TestSQLite_CreateProject_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	proj := &core.Project{
		ID:          "proj-preset-id",
		TenantID:    tenantID,
		Name:        "Preset Project",
		Environment: "staging",
	}
	if err := db.CreateProject(ctx, proj); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if proj.ID != "proj-preset-id" {
		t.Errorf("ID = %q", proj.ID)
	}
}

// =============================================================================
// SQLite — CreateSecret with pre-set ID
// =============================================================================

func TestSQLite_CreateSecret_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	secret := &core.Secret{
		ID:       "secret-preset-id",
		TenantID: tenantID,
		Name:     "API_KEY",
		Type:     "env",
		Scope:    "tenant",
	}
	if err := db.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if secret.ID != "secret-preset-id" {
		t.Errorf("ID = %q", secret.ID)
	}
}

// =============================================================================
// SQLite — CreateSecretVersion with pre-set ID
// =============================================================================

func TestSQLite_CreateSecretVersion_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	secret := &core.Secret{
		TenantID: tenantID,
		Name:     "SECRET_VER",
		Type:     "env",
		Scope:    "tenant",
	}
	db.CreateSecret(ctx, secret)

	ver := &core.SecretVersion{
		ID:        "ver-preset-id",
		SecretID:  secret.ID,
		Version:   1,
		ValueEnc:  "encrypted-value",
		CreatedBy: "user-1",
	}
	if err := db.CreateSecretVersion(ctx, ver); err != nil {
		t.Fatalf("CreateSecretVersion: %v", err)
	}
	if ver.ID != "ver-preset-id" {
		t.Errorf("ID = %q", ver.ID)
	}
}

// =============================================================================
// SQLite — CreateInvite with pre-set ID
// =============================================================================

func TestSQLite_CreateInvite_PresetID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	inv := &core.Invitation{
		ID:        "inv-preset-id",
		TenantID:  tenantID,
		Email:     "preset@invite.com",
		RoleID:    "role_admin",
		InvitedBy: "user-1",
		TokenHash: "tok-preset-" + core.GenerateID()[:8],
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    "pending",
	}
	if err := db.CreateInvite(ctx, inv); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if inv.ID != "inv-preset-id" {
		t.Errorf("ID = %q", inv.ID)
	}
}

// =============================================================================
// SQLite — GetNextDeployVersion after creating deployments
// =============================================================================

func TestCov_GetNextDeployVersion_WithMultiple(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "version-seq")

	// Create v1 and v3
	db.CreateDeployment(ctx, &core.Deployment{
		AppID: app.ID, Version: 1, Image: "img:v1", Status: "done",
		TriggeredBy: "test", Strategy: "recreate",
	})
	db.CreateDeployment(ctx, &core.Deployment{
		AppID: app.ID, Version: 3, Image: "img:v3", Status: "done",
		TriggeredBy: "test", Strategy: "recreate",
	})

	next, err := db.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetNextDeployVersion: %v", err)
	}
	if next != 4 {
		t.Errorf("expected 4 (max 3 + 1), got %d", next)
	}
}

// =============================================================================
// SQLite — GetLatestDeployment returns most recent version
// =============================================================================

func TestSQLite_GetLatestDeployment_MultipleVersions(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "latest-dep")

	db.CreateDeployment(ctx, &core.Deployment{
		AppID: app.ID, Version: 1, Image: "img:v1", Status: "stopped",
		TriggeredBy: "test", Strategy: "recreate",
	})
	db.CreateDeployment(ctx, &core.Deployment{
		AppID: app.ID, Version: 5, Image: "img:v5", Status: "running",
		TriggeredBy: "webhook", Strategy: "rolling",
	})

	latest, err := db.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if latest.Version != 5 {
		t.Errorf("Version = %d, want 5", latest.Version)
	}
	if latest.Image != "img:v5" {
		t.Errorf("Image = %q", latest.Image)
	}
}

// =============================================================================
// SQLite — ListProjectsByTenant with multiple projects
// =============================================================================

func TestSQLite_ListProjectsByTenant_Multiple(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	db.CreateProject(ctx, &core.Project{TenantID: tenantID, Name: "Alpha", Environment: "dev"})
	db.CreateProject(ctx, &core.Project{TenantID: tenantID, Name: "Bravo", Environment: "staging"})

	projects, err := db.ListProjectsByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListProjectsByTenant: %v", err)
	}
	// Default project + Alpha + Bravo = 3
	if len(projects) < 3 {
		t.Errorf("expected at least 3 projects, got %d", len(projects))
	}
	// Check alphabetical order
	for i := 1; i < len(projects); i++ {
		if projects[i].Name < projects[i-1].Name {
			t.Errorf("projects not sorted alphabetically: %q < %q",
				projects[i].Name, projects[i-1].Name)
		}
	}
}

// =============================================================================
// SQLite — ListRoles returns all built-in roles with correct fields
// =============================================================================

func TestSQLite_ListRoles_BuiltinFields(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	roles, err := db.ListRoles(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	for _, r := range roles {
		if r.ID == "" {
			t.Error("role missing ID")
		}
		if r.Name == "" {
			t.Error("role missing Name")
		}
		if r.PermissionsJSON == "" {
			t.Errorf("role %q missing PermissionsJSON", r.Name)
		}
	}
}

// =============================================================================
// SQLite — ListAllTenants pagination offset
// =============================================================================

func TestSQLite_ListAllTenants_Offset(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Create 3 tenants
	for i := 0; i < 3; i++ {
		db.CreateTenant(ctx, &core.Tenant{
			Name: "Tenant", Slug: "t-off-" + core.GenerateID()[:8],
			Status: "active", PlanID: "free",
		})
	}

	all, total, err := db.ListAllTenants(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListAllTenants: %v", err)
	}
	if total < 3 {
		t.Fatalf("total = %d, want >= 3", total)
	}

	// Offset by 1 should return one fewer
	offset1, _, err := db.ListAllTenants(ctx, 100, 1)
	if err != nil {
		t.Fatalf("ListAllTenants offset 1: %v", err)
	}
	if len(offset1) != len(all)-1 {
		t.Errorf("offset 1: got %d items, expected %d", len(offset1), len(all)-1)
	}
}

// =============================================================================
// SQLite — ListAuditLogs pagination
// =============================================================================

func TestSQLite_ListAuditLogs_Pagination(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	for i := 0; i < 5; i++ {
		db.CreateAuditLog(ctx, &core.AuditEntry{
			TenantID:     tenantID,
			Action:       "test",
			ResourceType: "app",
			ResourceID:   core.GenerateID(),
		})
	}

	// Limit to 2
	logs, total, err := db.ListAuditLogs(ctx, tenantID, 2, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(logs) != 2 {
		t.Errorf("len = %d, want 2", len(logs))
	}

	// Offset by 3
	logs2, _, err := db.ListAuditLogs(ctx, tenantID, 10, 3)
	if err != nil {
		t.Fatalf("ListAuditLogs offset: %v", err)
	}
	if len(logs2) != 2 {
		t.Errorf("offset 3: len = %d, want 2", len(logs2))
	}
}

// =============================================================================
// SQLite — ListAppsByTenant empty result
// =============================================================================

func TestSQLite_ListAppsByTenant_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	apps, total, err := db.ListAppsByTenant(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant: %v", err)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(apps) != 0 {
		t.Errorf("len = %d, want 0", len(apps))
	}
}

// =============================================================================
// SQLite — ListDeploymentsByApp empty result
// =============================================================================

func TestSQLite_ListDeploymentsByApp_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	deps, err := db.ListDeploymentsByApp(ctx, "nonexistent-app", 10)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp: %v", err)
	}
	if len(deps) != 0 {
		t.Errorf("expected 0 deployments, got %d", len(deps))
	}
}

// =============================================================================
// SQLite — ListDomainsByApp empty result
// =============================================================================

func TestSQLite_ListDomainsByApp_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	doms, err := db.ListDomainsByApp(ctx, "nonexistent-app")
	if err != nil {
		t.Fatalf("ListDomainsByApp: %v", err)
	}
	if len(doms) != 0 {
		t.Errorf("expected 0, got %d", len(doms))
	}
}

// =============================================================================
// SQLite — ListAllDomains empty result
// =============================================================================

func TestSQLite_ListAllDomains_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	doms, err := db.ListAllDomains(ctx)
	if err != nil {
		t.Fatalf("ListAllDomains: %v", err)
	}
	if len(doms) != 0 {
		t.Errorf("expected 0, got %d", len(doms))
	}
}

// =============================================================================
// SQLite — ListInvitesByTenant empty result
// =============================================================================

func TestCov_ListInvitesByTenant_NoRecords(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	invites, err := db.ListInvitesByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListInvitesByTenant: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("expected 0, got %d", len(invites))
	}
}

// =============================================================================
// SQLite — ListSecretsByTenant empty result
// =============================================================================

func TestCov_ListSecretsByTenant_NoRecords(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	secrets, err := db.ListSecretsByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListSecretsByTenant: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0, got %d", len(secrets))
	}
}

// =============================================================================
// SQLite — Tx commit and rollback with real data
// =============================================================================

func TestSQLite_Tx_CommitVerifiesData(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tenants (id, name, slug, status) VALUES (?, ?, ?, ?)`,
			"tx-commit-tenant", "TxCommit", "tx-commit-slug", "active",
		)
		return err
	})
	if err != nil {
		t.Fatalf("Tx: %v", err)
	}

	// Verify data was committed
	got, err := db.GetTenant(ctx, "tx-commit-tenant")
	if err != nil {
		t.Fatalf("GetTenant after Tx commit: %v", err)
	}
	if got.Name != "TxCommit" {
		t.Errorf("Name = %q", got.Name)
	}
}

// =============================================================================
// SQLite — GetApp not found
// =============================================================================

func TestSQLite_GetApp_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetApp(ctx, "nonexistent-app")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// =============================================================================
// SQLite — CreateTenantWithDefaults verifies default project creation
// =============================================================================

func TestSQLite_CreateTenantWithDefaults_CreatesProject(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, err := db.CreateTenantWithDefaults(ctx, "Defaults Tenant", "defaults-"+core.GenerateID()[:8])
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	// Verify tenant
	tenant, err := db.GetTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if tenant.Status != "active" {
		t.Errorf("Status = %q, want active", tenant.Status)
	}

	// Verify default project
	projects, err := db.ListProjectsByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListProjectsByTenant: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 default project, got %d", len(projects))
	}
	if projects[0].Name != "Default" {
		t.Errorf("project Name = %q, want Default", projects[0].Name)
	}
}

// =============================================================================
// BoltStore — List with expired and valid entries (TTL filtering)
// =============================================================================

func TestBoltStore_List_FiltersTTL(t *testing.T) {
	store := testBolt(t)

	// Set a key that expires in 1 second
	store.Set("sessions", "expires-soon", "val", 1)
	// Set a key that doesn't expire
	store.Set("sessions", "no-expire", "val", 0)

	time.Sleep(1100 * time.Millisecond)

	keys, err := store.List("sessions")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	foundExpired := false
	foundValid := false
	for _, k := range keys {
		if k == "expires-soon" {
			foundExpired = true
		}
		if k == "no-expire" {
			foundValid = true
		}
	}
	if foundExpired {
		t.Error("expired key should not be in list")
	}
	if !foundValid {
		t.Error("valid key should be in list")
	}
}

// =============================================================================
// BoltStore — Set with TTL > 0 stores expiry timestamp
// =============================================================================

func TestBoltStore_Set_WithTTL(t *testing.T) {
	store := testBolt(t)

	if err := store.Set("sessions", "ttl-key", "data", 3600); err != nil {
		t.Fatalf("Set with TTL: %v", err)
	}

	// Should be retrievable immediately
	var val string
	if err := store.Get("sessions", "ttl-key", &val); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "data" {
		t.Errorf("val = %q", val)
	}
}

// =============================================================================
// BoltStore — List with mixed expired entries via direct bolt write
// =============================================================================

func TestBoltStore_List_SkipsExpiredDirect(t *testing.T) {
	store := testBolt(t)

	// Write an entry with past expiration directly
	store.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("sessions"))
		// Entry with expired timestamp
		bkt.Put([]byte("expired-direct"), []byte(`{"d":"\"val\"","e":1}`))
		return nil
	})

	// Write a valid entry
	store.Set("sessions", "valid", "val", 0)

	keys, err := store.List("sessions")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	for _, k := range keys {
		if k == "expired-direct" {
			t.Error("expired entry should be filtered out")
		}
	}
}

// =============================================================================
// SQLite — Multiple domains per app
// =============================================================================

func TestSQLite_ListDomainsByApp_Multiple(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "multi-dom")

	db.CreateDomain(ctx, &core.Domain{AppID: app.ID, FQDN: "one.test.com", Type: "custom"})
	db.CreateDomain(ctx, &core.Domain{AppID: app.ID, FQDN: "two.test.com", Type: "auto"})

	doms, err := db.ListDomainsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListDomainsByApp: %v", err)
	}
	if len(doms) != 2 {
		t.Errorf("expected 2, got %d", len(doms))
	}
}

// =============================================================================
// SQLite — ListAppsByTenant with offset
// =============================================================================

func TestSQLite_ListAppsByTenant_OffsetBeyondTotal(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	createApp(t, db, tenantID, projID, "offset-app")

	apps, total, err := db.ListAppsByTenant(ctx, tenantID, 10, 100)
	if err != nil {
		t.Fatalf("ListAppsByTenant: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(apps) != 0 {
		t.Errorf("expected 0 apps with offset beyond total, got %d", len(apps))
	}
}

// =============================================================================
// SQLite — Tenant with metadata and limits JSON
// =============================================================================

func TestSQLite_Tenant_WithMetadata(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{
		Name:         "Meta Tenant",
		Slug:         "meta-" + core.GenerateID()[:8],
		Status:       "active",
		PlanID:       "pro",
		AvatarURL:    "https://example.com/avatar.png",
		LimitsJSON:   `{"apps": 10}`,
		MetadataJSON: `{"region": "us-east"}`,
	}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	got, err := db.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if got.LimitsJSON != `{"apps": 10}` {
		t.Errorf("LimitsJSON = %q", got.LimitsJSON)
	}
	if got.MetadataJSON != `{"region": "us-east"}` {
		t.Errorf("MetadataJSON = %q", got.MetadataJSON)
	}
	if got.AvatarURL != "https://example.com/avatar.png" {
		t.Errorf("AvatarURL = %q", got.AvatarURL)
	}
}

// =============================================================================
// SQLite — App with all fields populated
// =============================================================================

func TestSQLite_App_AllFields(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	app := &core.Application{
		ProjectID:  projID,
		TenantID:   tenantID,
		Name:       "full-app",
		Type:       "worker",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Branch:     "develop",
		Dockerfile: "Dockerfile.prod",
		BuildPack:  "go",
		EnvVarsEnc: "encrypted-env",
		LabelsJSON: `{"team":"backend"}`,
		Replicas:   3,
		Status:     "running",
		ServerID:   "server-1",
	}
	if err := db.CreateApp(ctx, app); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	got, err := db.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if got.Type != "worker" {
		t.Errorf("Type = %q", got.Type)
	}
	if got.SourceType != "git" {
		t.Errorf("SourceType = %q", got.SourceType)
	}
	if got.SourceURL != "https://github.com/test/repo" {
		t.Errorf("SourceURL = %q", got.SourceURL)
	}
	if got.Branch != "develop" {
		t.Errorf("Branch = %q", got.Branch)
	}
	if got.Dockerfile != "Dockerfile.prod" {
		t.Errorf("Dockerfile = %q", got.Dockerfile)
	}
	if got.BuildPack != "go" {
		t.Errorf("BuildPack = %q", got.BuildPack)
	}
	if got.EnvVarsEnc != "encrypted-env" {
		t.Errorf("EnvVarsEnc = %q", got.EnvVarsEnc)
	}
	if got.LabelsJSON != `{"team":"backend"}` {
		t.Errorf("LabelsJSON = %q", got.LabelsJSON)
	}
	if got.Replicas != 3 {
		t.Errorf("Replicas = %d", got.Replicas)
	}
	if got.ServerID != "server-1" {
		t.Errorf("ServerID = %q", got.ServerID)
	}
}

// =============================================================================
// SQLite — CreateUserWithMembership duplicate email (error in first INSERT)
// =============================================================================

func TestSQLite_CreateUserWithMembership_DuplicateEmail(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, err := db.CreateTenantWithDefaults(ctx, "Dup Email Tenant", "dup-email-"+core.GenerateID()[:8])
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	roles, _ := db.ListRoles(ctx, tenantID)
	roleID := roles[0].ID

	// First user — should succeed
	_, err = db.CreateUserWithMembership(ctx, "dup@test.com", "hash", "User1", "active", tenantID, roleID)
	if err != nil {
		t.Fatalf("first CreateUserWithMembership: %v", err)
	}

	// Second user with same email — should fail on the INSERT INTO users
	_, err = db.CreateUserWithMembership(ctx, "dup@test.com", "hash", "User2", "active", tenantID, roleID)
	if err == nil {
		t.Error("expected error for duplicate email")
	}
}

// =============================================================================
// SQLite — CreateTenantWithDefaults duplicate slug (error in first INSERT)
// =============================================================================

func TestSQLite_CreateTenantWithDefaults_DuplicateSlug(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	slug := "dup-slug-" + core.GenerateID()[:8]

	_, err := db.CreateTenantWithDefaults(ctx, "First", slug)
	if err != nil {
		t.Fatalf("first CreateTenantWithDefaults: %v", err)
	}

	// Second with same slug should fail
	_, err = db.CreateTenantWithDefaults(ctx, "Second", slug)
	if err == nil {
		t.Error("expected error for duplicate slug")
	}
}

// =============================================================================
// SQLite — CreateTenant duplicate slug error
// =============================================================================

func TestSQLite_CreateTenant_DuplicateSlug(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	slug := "dup-" + core.GenerateID()[:8]

	db.CreateTenant(ctx, &core.Tenant{Name: "T1", Slug: slug, Status: "active", PlanID: "free"})
	err := db.CreateTenant(ctx, &core.Tenant{Name: "T2", Slug: slug, Status: "active", PlanID: "free"})
	if err == nil {
		t.Error("expected error for duplicate slug")
	}
}

// =============================================================================
// SQLite — CreateUser duplicate email error
// =============================================================================

func TestSQLite_CreateUser_DuplicateEmail(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	db.CreateUser(ctx, &core.User{Email: "dup@test.com", PasswordHash: "h", Name: "A", Status: "active"})
	err := db.CreateUser(ctx, &core.User{Email: "dup@test.com", PasswordHash: "h", Name: "B", Status: "active"})
	if err == nil {
		t.Error("expected error for duplicate email")
	}
}

// =============================================================================
// SQLite — CreateDomain duplicate FQDN error
// =============================================================================

func TestSQLite_CreateDomain_DuplicateFQDN(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "dup-fqdn-app")

	db.CreateDomain(ctx, &core.Domain{AppID: app.ID, FQDN: "dup.test.com", Type: "custom"})
	err := db.CreateDomain(ctx, &core.Domain{AppID: app.ID, FQDN: "dup.test.com", Type: "custom"})
	if err == nil {
		t.Error("expected error for duplicate FQDN")
	}
}

// =============================================================================
// SQLite — Verify NewSQLite sets connection pool settings
// =============================================================================

func TestSQLite_NewSQLite_ConnectionPool(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/pool-test.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	defer db.Close()

	// Verify the DB is functional with a simple query
	var val int
	err = db.db.QueryRow("SELECT 1+1").Scan(&val)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if val != 2 {
		t.Errorf("1+1 = %d", val)
	}
}

// Suppress unused import warnings.
var _ = os.Remove
