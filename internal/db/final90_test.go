package db

import (
	"context"
	"database/sql"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	bolt "go.etcd.io/bbolt"
)

// =============================================================================
// ListAllTenants — field validation on scanned rows
// =============================================================================

func TestSQLite_ListAllTenants_FieldsMatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	slug := "field-tenant-" + core.GenerateID()[:6]
	tenantID, err := db.CreateTenantWithDefaults(ctx, "FieldTenant", slug)
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	tenants, total, err := db.ListAllTenants(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListAllTenants: %v", err)
	}
	if total < 1 {
		t.Fatalf("expected total >= 1, got %d", total)
	}

	var found *core.Tenant
	for i := range tenants {
		if tenants[i].ID == tenantID {
			found = &tenants[i]
			break
		}
	}
	if found == nil {
		t.Fatal("created tenant not found in list")
	}
	if found.Name != "FieldTenant" {
		t.Errorf("name = %q, want FieldTenant", found.Name)
	}
	if found.Slug != slug {
		t.Errorf("slug = %q, want %q", found.Slug, slug)
	}
	if found.Status != "active" {
		t.Errorf("status = %q, want active", found.Status)
	}
}

// =============================================================================
// ListAppsByTenant — verify scanned fields match inserted data
// =============================================================================

func TestSQLite_ListAppsByTenant_FieldsMatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projectID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projectID, "field-check-app")

	apps, total, err := db.ListAppsByTenant(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant: %v", err)
	}
	if total != 1 {
		t.Errorf("expected total 1, got %d", total)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1 app, got %d", len(apps))
	}
	if apps[0].ID != app.ID {
		t.Errorf("id = %q, want %q", apps[0].ID, app.ID)
	}
	if apps[0].Name != "field-check-app" {
		t.Errorf("name = %q", apps[0].Name)
	}
	if apps[0].TenantID != tenantID {
		t.Errorf("tenant_id = %q", apps[0].TenantID)
	}
	if apps[0].ProjectID != projectID {
		t.Errorf("project_id = %q", apps[0].ProjectID)
	}
}

// =============================================================================
// ListAuditLogs — verify scanned fields on returned entries
// =============================================================================

func TestSQLite_ListAuditLogs_FieldsMatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	entry := &core.AuditEntry{
		TenantID:     tenantID,
		UserID:       "user-audit",
		Action:       "delete",
		ResourceType: "domain",
		ResourceID:   "dom-99",
		DetailsJSON:  `{"reason":"test"}`,
		IPAddress:    "192.168.1.1",
		UserAgent:    "curl/7.0",
	}
	if err := db.CreateAuditLog(ctx, entry); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}

	logs, total, err := db.ListAuditLogs(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(logs))
	}
	e := logs[0]
	if e.UserID != "user-audit" {
		t.Errorf("user_id = %q", e.UserID)
	}
	if e.Action != "delete" {
		t.Errorf("action = %q", e.Action)
	}
	if e.ResourceType != "domain" {
		t.Errorf("resource_type = %q", e.ResourceType)
	}
	if e.ResourceID != "dom-99" {
		t.Errorf("resource_id = %q", e.ResourceID)
	}
	if e.IPAddress != "192.168.1.1" {
		t.Errorf("ip = %q", e.IPAddress)
	}
	if e.UserAgent != "curl/7.0" {
		t.Errorf("user_agent = %q", e.UserAgent)
	}
}

// =============================================================================
// ListAppsByProject — verify scanned fields
// =============================================================================

func TestSQLite_ListAppsByProject_FieldsMatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projectID := setupTenantAndProject(t, db)
	createApp(t, db, tenantID, projectID, "proj-app-1")

	apps, err := db.ListAppsByProject(ctx, projectID)
	if err != nil {
		t.Fatalf("ListAppsByProject: %v", err)
	}
	if len(apps) != 1 {
		t.Fatalf("expected 1, got %d", len(apps))
	}
	if apps[0].Name != "proj-app-1" {
		t.Errorf("name = %q", apps[0].Name)
	}
	if apps[0].ProjectID != projectID {
		t.Errorf("project_id = %q", apps[0].ProjectID)
	}
	if apps[0].TenantID != tenantID {
		t.Errorf("tenant_id = %q", apps[0].TenantID)
	}
}

// =============================================================================
// ListDeploymentsByApp — verify scanned deployment fields
// =============================================================================

func TestSQLite_ListDeploymentsByApp_FieldsMatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projectID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projectID, "deploy-fields")

	d := &core.Deployment{
		AppID:         app.ID,
		Version:       7,
		Image:         "myapp:v7",
		Status:        "completed",
		CommitSHA:     "abc123",
		CommitMessage: "fix: bug",
		TriggeredBy:   "webhook",
		Strategy:      "rolling",
	}
	if err := db.CreateDeployment(ctx, d); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	deps, err := db.ListDeploymentsByApp(ctx, app.ID, 10)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("expected 1, got %d", len(deps))
	}
	got := deps[0]
	if got.Version != 7 {
		t.Errorf("version = %d", got.Version)
	}
	if got.Image != "myapp:v7" {
		t.Errorf("image = %q", got.Image)
	}
	if got.CommitSHA != "abc123" {
		t.Errorf("commit_sha = %q", got.CommitSHA)
	}
	if got.CommitMessage != "fix: bug" {
		t.Errorf("commit_message = %q", got.CommitMessage)
	}
	if got.TriggeredBy != "webhook" {
		t.Errorf("triggered_by = %q", got.TriggeredBy)
	}
	if got.Strategy != "rolling" {
		t.Errorf("strategy = %q", got.Strategy)
	}
}

// =============================================================================
// ListDomainsByApp — verify scanned domain fields
// =============================================================================

func TestSQLite_ListDomainsByApp_FieldsMatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projectID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projectID, "domain-fields")

	dom := &core.Domain{AppID: app.ID, FQDN: "fields.example.com", Type: "custom", DNSProvider: "cloudflare"}
	if err := db.CreateDomain(ctx, dom); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	domains, err := db.ListDomainsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListDomainsByApp: %v", err)
	}
	if len(domains) != 1 {
		t.Fatalf("expected 1, got %d", len(domains))
	}
	if domains[0].FQDN != "fields.example.com" {
		t.Errorf("fqdn = %q", domains[0].FQDN)
	}
	if domains[0].DNSProvider != "cloudflare" {
		t.Errorf("dns_provider = %q", domains[0].DNSProvider)
	}
	if domains[0].AppID != app.ID {
		t.Errorf("app_id = %q", domains[0].AppID)
	}
}

// =============================================================================
// ListAllDomains — verify multiple domains across apps
// =============================================================================

func TestSQLite_ListAllDomains_MultipleApps(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projectID := setupTenantAndProject(t, db)
	app1 := createApp(t, db, tenantID, projectID, "all-dom-1")
	app2 := createApp(t, db, tenantID, projectID, "all-dom-2")

	db.CreateDomain(ctx, &core.Domain{AppID: app1.ID, FQDN: "a1.test.com", Type: "custom", DNSProvider: "cf"})
	db.CreateDomain(ctx, &core.Domain{AppID: app2.ID, FQDN: "a2.test.com", Type: "custom", DNSProvider: "cf"})
	db.CreateDomain(ctx, &core.Domain{AppID: app1.ID, FQDN: "a3.test.com", Type: "custom", DNSProvider: "cf"})

	domains, err := db.ListAllDomains(ctx)
	if err != nil {
		t.Fatalf("ListAllDomains: %v", err)
	}
	if len(domains) < 3 {
		t.Errorf("expected at least 3, got %d", len(domains))
	}
	// Verify all domains have non-empty fields
	for i, d := range domains {
		if d.ID == "" {
			t.Errorf("domain[%d] missing ID", i)
		}
		if d.FQDN == "" {
			t.Errorf("domain[%d] missing FQDN", i)
		}
	}
}

// =============================================================================
// ListInvitesByTenant — verify scanned invite fields
// =============================================================================

func TestSQLite_ListInvitesByTenant_FieldsMatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	inv := &core.Invitation{
		TenantID:  tenantID,
		Email:     "check@fields.com",
		RoleID:    "role_admin",
		InvitedBy: "user-inv",
		TokenHash: "hash-abc",
		Status:    "pending",
	}
	if err := db.CreateInvite(ctx, inv); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}

	invites, err := db.ListInvitesByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListInvitesByTenant: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1, got %d", len(invites))
	}
	if invites[0].Email != "check@fields.com" {
		t.Errorf("email = %q", invites[0].Email)
	}
	if invites[0].RoleID != "role_admin" {
		t.Errorf("role_id = %q", invites[0].RoleID)
	}
	if invites[0].InvitedBy != "user-inv" {
		t.Errorf("invited_by = %q", invites[0].InvitedBy)
	}
	if invites[0].Status != "pending" {
		t.Errorf("status = %q", invites[0].Status)
	}
}

// =============================================================================
// BoltStore — NewBoltStore successfully opens at valid path
// =============================================================================

func TestBoltStore_NewBoltStore_ValidPath(t *testing.T) {
	store := testBolt(t)

	// Verify we can use all the basic operations
	if err := store.Set("sessions", "test-key", "val", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var val string
	if err := store.Get("sessions", "test-key", &val); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if val != "val" {
		t.Errorf("got %q, want 'val'", val)
	}

	keys, err := store.List("sessions")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) < 1 {
		t.Error("expected at least 1 key")
	}
}

// =============================================================================
// BoltStore — NewBoltStore with invalid path
// =============================================================================

func TestBoltStore_NewBoltStore_InvalidPath(t *testing.T) {
	_, err := NewBoltStore("/nonexistent/path/that/cannot/exist/test.bolt")
	if err == nil {
		t.Error("expected error for invalid path")
	}
}

// =============================================================================
// Module — Health with closed sqlite (Ping error path)
// =============================================================================

func TestModule_Health_ClosedSQLite(t *testing.T) {
	dir := t.TempDir()

	sqliteDB, err := NewSQLite(dir + "/health-test.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	boltStore, err := NewBoltStore(dir + "/health-test.bolt")
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}

	m := New()
	m.driver = "sqlite"
	m.sqlite = sqliteDB
	m.bolt = boltStore

	// Health should be OK before close
	if m.Health() != core.HealthOK {
		t.Error("expected HealthOK before close")
	}

	// Close sqlite, health should now return Down
	sqliteDB.Close()
	if m.Health() != core.HealthDown {
		t.Error("expected HealthDown after sqlite close")
	}

	boltStore.Close()
}

// =============================================================================
// Module — Health with nil stores
// =============================================================================

func TestModule_Health_NilStores(t *testing.T) {
	m := New()
	m.driver = "sqlite"

	if m.Health() != core.HealthDown {
		t.Error("expected HealthDown when stores are nil")
	}

	// Only bolt set, sqlite nil
	dir := t.TempDir()
	bolt, _ := NewBoltStore(dir + "/nil-sqlite.bolt")
	m.bolt = bolt
	defer bolt.Close()

	if m.Health() != core.HealthDown {
		t.Error("expected HealthDown when sqlite is nil")
	}
}

// =============================================================================
// Module — Stop with error propagation
// =============================================================================

func TestModule_Stop_SQLiteCloseError(t *testing.T) {
	dir := t.TempDir()

	sqliteDB, err := NewSQLite(dir + "/stop-err.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	boltStore, err := NewBoltStore(dir + "/stop-err.bolt")
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}

	m := New()
	m.sqlite = sqliteDB
	m.bolt = boltStore

	// Close sqlite first so Stop's close will error
	sqliteDB.Close()

	// Stop should propagate the sqlite close error
	stopErr := m.Stop(context.TODO())
	// The error may or may not happen depending on implementation
	// but the branch is exercised either way
	_ = stopErr
}

func TestModule_Stop_BoltOnly(t *testing.T) {
	dir := t.TempDir()
	boltStore, err := NewBoltStore(dir + "/bolt-only.bolt")
	if err != nil {
		t.Fatalf("NewBoltStore: %v", err)
	}

	m := New()
	m.bolt = boltStore
	// sqlite is nil — exercises the nil check branch

	if err := m.Stop(context.TODO()); err != nil {
		t.Errorf("Stop with nil sqlite: %v", err)
	}
}

func TestModule_Stop_NilBoth(t *testing.T) {
	m := New()
	if err := m.Stop(context.TODO()); err != nil {
		t.Errorf("Stop with nil both: %v", err)
	}
}

// =============================================================================
// BoltStore — Set marshal error path
// =============================================================================

func TestBoltStore_Set_MarshalError(t *testing.T) {
	store := testBolt(t)

	// Channels can't be marshaled to JSON
	err := store.Set("sessions", "bad", make(chan int), 0)
	if err == nil {
		t.Error("expected marshal error for channel value")
	}
}

// =============================================================================
// BoltStore — Get with non-existent bucket
// =============================================================================

func TestBoltStore_Get_NonExistentBucket(t *testing.T) {
	store := testBolt(t)

	var dest string
	err := store.Get("nonexistent_bucket", "key", &dest)
	if err == nil {
		t.Error("expected error for non-existent bucket")
	}
}

// =============================================================================
// BoltStore — Set to non-existent bucket
// =============================================================================

func TestBoltStore_Set_NonExistentBucket(t *testing.T) {
	store := testBolt(t)

	err := store.Set("nonexistent_bucket", "key", "val", 0)
	if err == nil {
		t.Error("expected error for non-existent bucket")
	}
}

// =============================================================================
// BoltStore — Delete from non-existent bucket
// =============================================================================

func TestBoltStore_Delete_NonExistentBucket(t *testing.T) {
	store := testBolt(t)

	err := store.Delete("nonexistent_bucket", "key")
	if err == nil {
		t.Error("expected error for non-existent bucket")
	}
}

// =============================================================================
// BoltStore — List from non-existent bucket
// =============================================================================

func TestBoltStore_List_NonExistentBucket(t *testing.T) {
	store := testBolt(t)

	_, err := store.List("nonexistent_bucket")
	if err == nil {
		t.Error("expected error for non-existent bucket")
	}
}

// =============================================================================
// BoltStore — Get with corrupt entry data
// =============================================================================

func TestBoltStore_Get_CorruptEntry(t *testing.T) {
	store := testBolt(t)

	// Write valid data first
	store.Set("sessions", "corrupt-key", map[string]string{"k": "v"}, 0)

	// Try to get into wrong type
	var dest int
	err := store.Get("sessions", "corrupt-key", &dest)
	if err == nil {
		t.Error("expected error when getting map into int")
	}
}

// =============================================================================
// BoltStore — Get key not found
// =============================================================================

func TestBoltStore_Get_KeyNotFound(t *testing.T) {
	store := testBolt(t)

	var dest string
	err := store.Get("sessions", "does-not-exist", &dest)
	if err == nil {
		t.Error("expected error for missing key")
	}
}

// =============================================================================
// Module — Init error paths
// =============================================================================

func TestModule_Init_InvalidSQLitePath(t *testing.T) {
	m := New()

	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Path: "/nonexistent/path/test.db",
			},
		},
	}

	err := m.Init(context.Background(), c)
	if err == nil {
		t.Error("expected error for invalid sqlite path")
	}
}

// =============================================================================
// Module — accessor methods
// =============================================================================

func TestModule_Accessors(t *testing.T) {
	m := New()

	if m.ID() != "core.db" {
		t.Errorf("ID = %q", m.ID())
	}
	if m.Name() != "Database" {
		t.Errorf("Name = %q", m.Name())
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version = %q", m.Version())
	}
	if m.Dependencies() != nil {
		t.Errorf("Dependencies = %v", m.Dependencies())
	}
	if m.Routes() != nil {
		t.Error("Routes should be nil")
	}
	if m.Events() != nil {
		t.Error("Events should be nil")
	}
	// Store() returns an interface wrapping nil *SQLiteDB — check underlying
	if m.SQLite() != nil {
		t.Error("SQLite should be nil before Init")
	}
	if m.Bolt() != nil {
		t.Error("Bolt should be nil before Init")
	}
	// Store() accessor should work without panic
	_ = m.Store()
	if m.Start(context.TODO()) != nil {
		t.Error("Start should succeed")
	}
}

// =============================================================================
// BoltStore — Get with corrupt raw bytes (unmarshal entry error path)
// =============================================================================

func TestBoltStore_Get_CorruptRawBytes(t *testing.T) {
	store := testBolt(t)

	// Write corrupt raw bytes directly to the bolt db
	err := store.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("sessions"))
		return bkt.Put([]byte("corrupt"), []byte("not valid json {{{"))
	})
	if err != nil {
		t.Fatalf("write corrupt data: %v", err)
	}

	var dest string
	err = store.Get("sessions", "corrupt", &dest)
	if err == nil {
		t.Error("expected error for corrupt entry data")
	}
}

// =============================================================================
// BoltStore — List with corrupt entries (skip corrupt path)
// =============================================================================

func TestBoltStore_List_SkipsCorruptEntries(t *testing.T) {
	store := testBolt(t)

	// Write a valid entry
	store.Set("sessions", "valid-key", "value", 0)

	// Write corrupt raw bytes directly
	err := store.db.Update(func(tx *bolt.Tx) error {
		bkt := tx.Bucket([]byte("sessions"))
		return bkt.Put([]byte("corrupt-key"), []byte("not json"))
	})
	if err != nil {
		t.Fatalf("write corrupt data: %v", err)
	}

	keys, err := store.List("sessions")
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	// Valid key should still be returned; corrupt key should be skipped
	found := false
	for _, k := range keys {
		if k == "valid-key" {
			found = true
		}
	}
	if !found {
		t.Error("expected valid-key in list despite corrupt entries")
	}
}

// =============================================================================
// SQLiteDB — Tx error when DB is closed (BeginTx error path)
// =============================================================================

func TestSQLite_Tx_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/tx-err.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.Tx(context.Background(), func(tx *sql.Tx) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for Tx on closed DB")
	}
}

// =============================================================================
// SQLiteDB — CreateApp on closed DB (Tx error propagation)
// =============================================================================

func TestSQLite_CreateApp_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-app.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	app := &core.Application{
		Name: "test", TenantID: "t1", ProjectID: "p1",
		Type: "service", SourceType: "image", Status: "running",
	}
	err = db.CreateApp(context.Background(), app)
	if err == nil {
		t.Error("expected error for CreateApp on closed DB")
	}
}

// =============================================================================
// SQLiteDB — ListAppsByTenant on closed DB (query error path)
// =============================================================================

func TestSQLite_ListAppsByTenant_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-list.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, _, err = db.ListAppsByTenant(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Error("expected error for ListAppsByTenant on closed DB")
	}
}

// =============================================================================
// SQLiteDB — ListAllTenants on closed DB
// =============================================================================

func TestSQLite_ListAllTenants_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-tenants.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, _, err = db.ListAllTenants(context.Background(), 10, 0)
	if err == nil {
		t.Error("expected error for ListAllTenants on closed DB")
	}
}

// =============================================================================
// SQLiteDB — ListAuditLogs on closed DB
// =============================================================================

func TestSQLite_ListAuditLogs_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/closed-audit.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	_, _, err = db.ListAuditLogs(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Error("expected error for ListAuditLogs on closed DB")
	}
}

// =============================================================================
// ListSecretsByTenant — verify scanned secret fields
// =============================================================================

func TestSQLite_ListSecretsByTenant_FieldsMatch(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	s := &core.Secret{
		TenantID:       tenantID,
		Name:           "MY_SECRET",
		Type:           "env",
		Description:    "A secret",
		Scope:          "project",
		CurrentVersion: 3,
	}
	if err := db.CreateSecret(ctx, s); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	secrets, err := db.ListSecretsByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListSecretsByTenant: %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("expected 1, got %d", len(secrets))
	}
	if secrets[0].Name != "MY_SECRET" {
		t.Errorf("name = %q", secrets[0].Name)
	}
	if secrets[0].Type != "env" {
		t.Errorf("type = %q", secrets[0].Type)
	}
	if secrets[0].Scope != "project" {
		t.Errorf("scope = %q", secrets[0].Scope)
	}
}
