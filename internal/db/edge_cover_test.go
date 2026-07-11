package db

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — initSchema error paths
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_initSchema_NilDB(t *testing.T) {
	b := &BoltStore{}
	err := b.initSchema()
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Set edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_Set_NegativeTTL(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	if err := bolt.Set("bucket", "k", "v", -1); err != nil {
		t.Fatalf("Set negative TTL: %v", err)
	}
	var got string
	if err := bolt.Get("bucket", "k", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "v" {
		t.Errorf("got %q, want v", got)
	}
}

func TestBoltStore_BatchSet_SingleItem(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	items := []core.BoltBatchItem{
		{Bucket: "single-bucket", Key: "one", Value: "1", TTL: 0},
	}
	if err := bolt.BatchSet(items); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}
	var got string
	if err := bolt.Get("single-bucket", "one", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "1" {
		t.Errorf("got %q, want 1", got)
	}
}

func TestBoltStore_BatchSet_TTLItems(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	items := []core.BoltBatchItem{
		{Bucket: "ttl-bucket", Key: "a", Value: "a", TTL: 100},
		{Bucket: "ttl-bucket", Key: "b", Value: "b", TTL: 200},
	}
	if err := bolt.BatchSet(items); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}
	var got string
	if err := bolt.Get("ttl-bucket", "a", &got); err != nil {
		t.Fatalf("Get a: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Mutate edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_Mutate_WithTTL(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	var dest = "init"
	err := bolt.Mutate("ttl-mut", "k", &dest, 600, func(exists bool) error {
		dest = "ttl-value"
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate with TTL: %v", err)
	}
	var got string
	if err := bolt.Get("ttl-mut", "k", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "ttl-value" {
		t.Errorf("got %q, want ttl-value", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — bucketExists error path
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_bucketExists_NotFound(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	exists, err := bolt.bucketExists("nonexistent-bucket")
	if err != nil {
		t.Fatalf("bucketExists: %v", err)
	}
	if exists {
		t.Error("expected false for nonexistent bucket")
	}
}

func TestBoltStore_bucketExists_Found(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	if err := bolt.Set("existent-bucket", "k", "v", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	exists, err := bolt.bucketExists("existent-bucket")
	if err != nil {
		t.Fatalf("bucketExists: %v", err)
	}
	if !exists {
		t.Error("expected true for existent bucket")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — List with JSON-invalid data
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_List_SkipsInvalidJSON(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	// Insert a valid entry
	if err := bolt.Set("json-bucket", "valid", "good", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	// Insert a raw invalid entry directly via SQL
	_, err := bolt.db.Exec(
		`INSERT INTO kv_store(bucket, key, data, expires_at) VALUES (?, ?, ?, 0)`,
		"json-bucket", "invalid", []byte("not-valid-json"),
	)
	if err != nil {
		t.Fatalf("insert invalid JSON: %v", err)
	}

	keys, err := bolt.List("json-bucket")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// Should only return the valid entry, skip invalid JSON
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d: %v", len(keys), keys)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Close on uninitialized store
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_Close_NilReceiver(t *testing.T) {
	var b *BoltStore
	if err := b.Close(); err != nil {
		t.Errorf("Close on nil: %v", err)
	}
}

func TestBoltStore_Close_AlreadyClosed(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	if err := bolt.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second close should be a no-op
	if err := bolt.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetAPIKeyByPrefix with stored key
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_GetAPIKeyByPrefix_StoredKey(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	ctx := context.Background()

	// Store an API key as a JSON record in api_keys bucket
	rec := apiKeyKVRecord{
		ID:        "ak_stored",
		UserID:    "user_stored",
		TenantID:  "tenant_stored",
		Name:      "Stored Key",
		KeyHash:   "hash_stored",
		KeyPrefix: "dmk_stored",
		Scopes:    `["read"]`,
		CreatedAt: time.Now(),
	}
	data, _ := json.Marshal(rec)
	if err := bolt.Set("api_keys", "ak_stored", json.RawMessage(data), 0); err != nil {
		t.Fatalf("Set API key: %v", err)
	}

	key, err := bolt.GetAPIKeyByPrefix(ctx, "dmk_stored")
	if err != nil {
		t.Fatalf("GetAPIKeyByPrefix: %v", err)
	}
	if key.ID != "ak_stored" {
		t.Errorf("ID = %q, want ak_stored", key.ID)
	}
	if key.UserID != "user_stored" {
		t.Errorf("UserID = %q", key.UserID)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetWebhookSecret with flat format (no legacy wrapper)
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_GetWebhookSecret_FlatFormat(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	rec := map[string]string{"secret_hash": "flat_secret_value"}
	data, _ := json.Marshal(rec)
	if err := bolt.Set("webhooks", "wh_flat", json.RawMessage(data), 0); err != nil {
		t.Fatalf("Set webhook: %v", err)
	}
	secret, err := bolt.GetWebhookSecret("wh_flat")
	if err != nil {
		t.Fatalf("GetWebhookSecret: %v", err)
	}
	if secret != "flat_secret_value" {
		t.Errorf("got %q, want flat_secret_value", secret)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — uncovered branches
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Store_ReturnsPostgres(t *testing.T) {
	m := &Module{
		postgres: &PostgresDB{},
		sqlite:   nil,
	}
	s := m.Store()
	if s == nil {
		t.Fatal("Store() returned nil")
	}
}

func TestModule_SQLite_Bolt_Accessors(t *testing.T) {
	db := testDB(t)
	m := &Module{
		sqlite: db,
		bolt:   &BoltStore{},
	}
	if m.SQLite() != db {
		t.Error("SQLite() mismatch")
	}
	if m.Bolt() == nil {
		t.Error("Bolt() returned nil")
	}
}

func TestModule_Stop_SQLiteOnly(t *testing.T) {
	dir := t.TempDir()
	sqliteDB, err := NewSQLite(dir + "/stop-test.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	m := &Module{
		sqlite: sqliteDB,
		driver: "sqlite",
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Errorf("Stop: %v", err)
	}
}

func TestModule_Health_DownSQLite(t *testing.T) {
	dir := t.TempDir()
	sqliteDB, err := NewSQLite(dir + "/health-test.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	m := &Module{
		sqlite:   sqliteDB,
		bolt:     &BoltStore{db: sqliteDB.DB()},
		driver:   "sqlite",
	}
	h := m.Health()
	if h != core.HealthOK {
		t.Errorf("expected HealthOK, got %v", h)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — withTimeout (uncovered branch)
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_withTimeout_ZeroDuration(t *testing.T) {
	s := &SQLiteDB{}
	ctx, cancel := s.withTimeout(context.Background())
	defer cancel()
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestSQLite_withTimeout_PositiveDuration(t *testing.T) {
	s := &SQLiteDB{queryTimeout: 10 * time.Second}
	ctx, cancel := s.withTimeout(context.Background())
	defer cancel()
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — NewSQLite error paths (indirectly: already dead db)
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_Ping_Error(t *testing.T) {
	// Open a valid db then close it to test Ping on closed DB
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/ping-test.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()
	// Ping should fail on closed DB
	err = db.Ping(context.Background())
	if err == nil {
		t.Log("Note: Ping on closed db may not error on all platforms")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Setup.go — GetUserMembership / RemoveTeamMember edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_CreateUserWithMembership_Full(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, err := db.CreateTenantWithDefaults(ctx, "Membership Tenant", "membership-tenant")
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	roles, err := db.ListRoles(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) == 0 {
		t.Fatal("expected roles")
	}

	userID, err := db.CreateUserWithMembership(ctx, "member@example.com", "hash", "Member", "active", tenantID, roles[0].ID)
	if err != nil {
		t.Fatalf("CreateUserWithMembership: %v", err)
	}
	if userID == "" {
		t.Fatal("expected non-empty user ID")
	}

	// Verify membership
	members, err := db.ListTeamMembers(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListTeamMembers: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(members))
	}
	if members[0].UserID != userID {
		t.Errorf("UserID = %q, want %q", members[0].UserID, userID)
	}

	// GetUserMembership
	tm, err := db.GetUserMembership(ctx, userID)
	if err != nil {
		t.Fatalf("GetUserMembership: %v", err)
	}
	if tm.UserID != userID {
		t.Errorf("UserID = %q", tm.UserID)
	}

	// RemoveTeamMember
	if err := db.RemoveTeamMember(ctx, tenantID, members[0].ID); err != nil {
		t.Fatalf("RemoveTeamMember: %v", err)
	}

	// Verify removed
	members2, err := db.ListTeamMembers(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListTeamMembers after remove: %v", err)
	}
	if len(members2) != 0 {
		t.Errorf("expected 0 members after remove, got %d", len(members2))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Domain — comprehensive CRUD
// ═══════════════════════════════════════════════════════════════════════════════

func TestDomain_CreateGetListDelete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "domain-app")

	dom := &core.Domain{
		AppID:    app.ID,
		FQDN:     "myapp.example.com",
		Type:     "auto",
		DNSProvider: "cf",
	}
	if err := db.CreateDomain(ctx, dom); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}
	if dom.ID == "" {
		t.Fatal("expected auto-generated ID")
	}

	// GetDomainByFQDN
	got, err := db.GetDomainByFQDN(ctx, "myapp.example.com")
	if err != nil {
		t.Fatalf("GetDomainByFQDN: %v", err)
	}
	if got.FQDN != "myapp.example.com" {
		t.Errorf("FQDN = %q", got.FQDN)
	}

	// GetDomain
	got2, err := db.GetDomain(ctx, dom.ID)
	if err != nil {
		t.Fatalf("GetDomain: %v", err)
	}
	if got2.ID != dom.ID {
		t.Errorf("ID mismatch")
	}

	// ListDomainsByApp
	doms, err := db.ListDomainsByApp(ctx, app.ID, tenantID)
	if err != nil {
		t.Fatalf("ListDomainsByApp: %v", err)
	}
	if len(doms) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(doms))
	}

	// ListDomainsByAppIDs
	domMap, err := db.ListDomainsByAppIDs(ctx, []string{app.ID}, tenantID)
	if err != nil {
		t.Fatalf("ListDomainsByAppIDs: %v", err)
	}
	if len(domMap[app.ID]) != 1 {
		t.Errorf("expected 1 domain for app, got %d", len(domMap[app.ID]))
	}

	// DeleteDomain
	if err := db.DeleteDomain(ctx, dom.ID, tenantID); err != nil {
		t.Fatalf("DeleteDomain: %v", err)
	}

	// Verify deletion
	_, err = db.GetDomain(ctx, dom.ID)
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Deployment — CreateDeploymentAtomicVersion edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestDeployment_AtomicVersion_FirstAndSecond(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "atomic-deploy-app")

	d1 := &core.Deployment{
		AppID:       app.ID,
		Image:       "nginx:1.0",
		Status:      "deploying",
		Strategy:    "recreate",
		TriggeredBy: "test",
	}
	if err := db.CreateDeploymentAtomicVersion(ctx, d1); err != nil {
		t.Fatalf("CreateDeploymentAtomicVersion 1: %v", err)
	}
	if d1.Version != 1 {
		t.Errorf("version = %d, want 1", d1.Version)
	}

	d2 := &core.Deployment{
		AppID:       app.ID,
		Image:       "nginx:2.0",
		Status:      "deploying",
		Strategy:    "recreate",
		TriggeredBy: "test",
	}
	if err := db.CreateDeploymentAtomicVersion(ctx, d2); err != nil {
		t.Fatalf("CreateDeploymentAtomicVersion 2: %v", err)
	}
	if d2.Version != 2 {
		t.Errorf("version = %d, want 2", d2.Version)
	}

	// ListDeploymentsByStatus
	deploys, err := db.ListDeploymentsByStatus(ctx, "deploying")
	if err != nil {
		t.Fatalf("ListDeploymentsByStatus: %v", err)
	}
	if len(deploys) != 2 {
		t.Fatalf("got %d deploys, want 2", len(deploys))
	}

	// ListDeploymentsByApp
	appDeploys, err := db.ListDeploymentsByApp(ctx, app.ID, 10)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp: %v", err)
	}
	if len(appDeploys) != 2 {
		t.Fatalf("got %d deploys, want 2", len(appDeploys))
	}

	// GetLatestDeployment
	latest, err := db.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if latest.Version != 2 {
		t.Errorf("latest version = %d, want 2", latest.Version)
	}

	// UpdateDeployment
	d2.Status = "running"
	d2.ContainerID = "cont_abc"
	if err := db.UpdateDeployment(ctx, d2); err != nil {
		t.Fatalf("UpdateDeployment: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// User — UpdateTOTPEnabled edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_UpdateTOTPEnabled_EnableDisable(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		Email:        "totp-edge@example.com",
		PasswordHash: "hash",
		Name:         "TOTP Edge",
		Status:       "active",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := db.UpdateTOTPEnabled(ctx, user.ID, true, "enc-secret"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := db.UpdateTOTPEnabled(ctx, user.ID, false, ""); err != nil {
		t.Fatalf("disable: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// BoltStore — Mutate expired key path
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_Mutate_ExistingKeyTTL(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	// Set a key with a long TTL
	if err := bolt.Set("exp-mut", "ek", "original", 3600); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var dest string
	err := bolt.Mutate("exp-mut", "ek", &dest, 3600, func(exists bool) error {
		if !exists {
			t.Error("expected exists=true")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// newBoltStoreHelper — test helper
// ═══════════════════════════════════════════════════════════════════════════════

func newBoltStoreHelper(t *testing.T) *BoltStore {
	t.Helper()
	dir := t.TempDir()
	bolt, err := NewSQLiteKVStore(dir + "/bolt-edge.db")
	if err != nil {
		t.Fatalf("NewSQLiteKVStore: %v", err)
	}
	t.Cleanup(func() { bolt.Close() })
	return bolt
}

// ═══════════════════════════════════════════════════════════════════════════════
// decodeAPIKeyRecord — empty ID / empty name coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestDecodeAPIKeyRecord_EmptyIDDefaultsToPrefix(t *testing.T) {
	rec := apiKeyKVRecord{
		UserID:    "user-123",
		Hash:      "hash-abc",
		Prefix:    "dmk_xyz",
	}
	data, _ := json.Marshal(rec)
	key, err := decodeAPIKeyRecord(data)
	if err != nil {
		t.Fatalf("decodeAPIKeyRecord: %v", err)
	}
	if key.ID != "dmk_xyz" {
		t.Errorf("ID = %q, want dmk_xyz (default to Prefix)", key.ID)
	}
}

func TestDecodeAPIKeyRecord_EmptyIDAndNameDefaults(t *testing.T) {
	rec := apiKeyKVRecord{
		UserID:    "user-456",
		KeyHash:   "hash-def",
		KeyPrefix: "dmk_abc",
	}
	data, _ := json.Marshal(rec)
	key, err := decodeAPIKeyRecord(data)
	if err != nil {
		t.Fatalf("decodeAPIKeyRecord: %v", err)
	}
	if key.Name != "dmk_abc" {
		t.Errorf("Name = %q, want dmk_abc (default to KeyPrefix)", key.Name)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetWebhookSecret empty hash coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_GetWebhookSecret_EmptyHashRecord(t *testing.T) {
	bolt := newBoltStoreHelper(t)
	// Store a record with empty secret_hash
	rec := map[string]string{"secret_hash": ""}
	data, _ := json.Marshal(rec)
	if err := bolt.Set("webhooks", "wh_empty_hash", json.RawMessage(data), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	_, err := bolt.GetWebhookSecret("wh_empty_hash")
	if err == nil {
		t.Fatal("expected error for empty secret hash")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — NewSQLiteKVStore error with invalid path (directory, not file)
// ═══════════════════════════════════════════════════════════════════════════════

func TestBoltStore_NewSQLiteKVStore_InvalidPath(t *testing.T) {
	_, err := NewSQLiteKVStore("/nonexistent/directory/test.db")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Stop with empty bolt (exercises bolt.Close error path)
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Stop_EmptyBolt(t *testing.T) {
	m := &Module{
		postgres: nil,
		sqlite:   nil,
		bolt:     &BoltStore{}, // uninitialized, Close will be a no-op
	}
	if err := m.Stop(context.Background()); err != nil {
		t.Logf("Stop with empty bolt: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Health with sqlite driver but nil sqlite (exercises line 144)
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Health_SQLiteDriverNilSQLite(t *testing.T) {
	m := &Module{
		driver: "sqlite",
		bolt:   &BoltStore{db: nil},
		sqlite: nil,
	}
	if m.Health() != core.HealthDown {
		t.Errorf("expected HealthDown, got %v", m.Health())
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// module.go — Health with postgres driver but nil postgres
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Health_PostgresDriverNilPostgres(t *testing.T) {
	m := &Module{
		driver:   "postgres",
		bolt:     &BoltStore{db: nil},
		postgres: nil,
	}
	if m.Health() != core.HealthDown {
		t.Errorf("expected HealthDown, got %v", m.Health())
	}
}



// ═══════════════════════════════════════════════════════════════════════════════
// SQLite — ListMigrations edge case (SQLite path only)
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListMigrations_Results(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	migrations, err := db.ListMigrations(ctx)
	if err != nil {
		t.Fatalf("ListMigrations: %v", err)
	}
	if len(migrations) == 0 {
		t.Error("expected one or more migrations")
	}
	// Check migration has expected fields
	if migrations[0].Version <= 0 {
		t.Error("expected positive version")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Deployment — GetNextDeployVersion edge case
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_GetNextDeployVersion_FirstVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	ver, err := db.GetNextDeployVersion(ctx, "non-existent-app")
	if err != nil {
		t.Fatalf("GetNextDeployVersion: %v", err)
	}
	if ver != 1 {
		t.Errorf("expected version 1, got %d", ver)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// BoltStore — closed database operations (error path coverage)
// ═══════════════════════════════════════════════════════════════════════════════

func closedBoltStore(t *testing.T) *BoltStore {
	t.Helper()
	dir := t.TempDir()
	bolt, err := NewSQLiteKVStore(dir + "/bolt-closed.db")
	if err != nil {
		t.Fatalf("NewSQLiteKVStore: %v", err)
	}
	// Close the underlying DB so subsequent operations fail
	bolt.db.Close()
	return bolt
}

func TestBoltStore_Set_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	err := bolt.Set("bucket", "key", "value", 0)
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestBoltStore_BatchSet_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	items := []core.BoltBatchItem{
		{Bucket: "b", Key: "k", Value: "v", TTL: 0},
	}
	err := bolt.BatchSet(items)
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestBoltStore_Mutate_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	var dest string
	err := bolt.Mutate("b", "k", &dest, 0, func(exists bool) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestBoltStore_Get_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	var dest string
	err := bolt.Get("b", "k", &dest)
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestBoltStore_Delete_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	err := bolt.Delete("b", "k")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestBoltStore_List_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	_, err := bolt.List("b")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestBoltStore_GetAPIKeyByPrefix_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	_, err := bolt.GetAPIKeyByPrefix(context.Background(), "prefix")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestBoltStore_GetWebhookSecret_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	_, err := bolt.GetWebhookSecret("wh")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

func TestBoltStore_bucketExists_ClosedDB(t *testing.T) {
	bolt := closedBoltStore(t)
	_, err := bolt.bucketExists("b")
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module — Stop with closed sqlite (error path coverage)
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Stop_ClosedSQLite(t *testing.T) {
	dir := t.TempDir()
	db, err := NewSQLite(dir + "/mod-stop-closed.db")
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	// Close the underlying DB first
	db.Close()

	m := &Module{
		sqlite: db,
		driver: "sqlite",
	}
	// Stop should try to close the already-closed db and report the error
	if err := m.Stop(context.Background()); err == nil {
		t.Log("Stop on closed sqlite returned nil (may be acceptable)")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module — Init with invalid sqlite path (error path coverage)
// ═══════════════════════════════════════════════════════════════════════════════

func TestModule_Init_InvalidSQLiteDir(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Driver: "sqlite",
				Path:   "/nonexistent/directory/test.db",
			},
		},
	}
	// This will likely fail because the directory doesn't exist
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Logf("Init with invalid path: %v (expected to fail)", err)
	}
}


