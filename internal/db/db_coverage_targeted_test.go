package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Mutate with nil callback
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_Mutate_NilCallback(t *testing.T) {
	bs := testBolt(t)

	var dest string
	err := bs.Mutate("sessions", "key", &dest, 0, nil)
	if err == nil {
		t.Fatal("expected error for nil mutate callback")
	}
}

func TestBolt_Mutate_NewKey(t *testing.T) {
	bs := testBolt(t)

	var val string
	err := bs.Mutate("sessions", "brand-new-key", &val, 0, func(exists bool) error {
		if exists {
			t.Fatal("expected exists=false for new key")
		}
		val = "created"
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate new key: %v", err)
	}

	var got string
	if err := bs.Get("sessions", "brand-new-key", &got); err != nil {
		t.Fatalf("Get after Mutate: %v", err)
	}
	if got != "created" {
		t.Errorf("got %q, want %q", got, "created")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Delete with nonexistent bucket
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_Delete_NonExistentBucket(t *testing.T) {
	bs := testBolt(t)

	err := bs.Delete("no-such-bucket", "key")
	if err == nil {
		t.Fatal("expected error for nonexistent bucket")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — List with nonexistent bucket
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_List_NonExistentBucket(t *testing.T) {
	bs := testBolt(t)

	_, err := bs.List("no-such-bucket")
	if err == nil {
		t.Fatal("expected error for nonexistent bucket")
	}
}

func TestBolt_List_EmptyBucket(t *testing.T) {
	bs := testBolt(t)

	keys, err := bs.List("sessions")
	if err != nil {
		t.Fatalf("List empty sessions bucket: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetAPIKeyByPrefix with cancelled context
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_GetAPIKeyByPrefix_CancelledContext(t *testing.T) {
	bs := testBolt(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := bs.GetAPIKeyByPrefix(ctx, "prefix")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — GetWebhookSecret edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_GetWebhookSecret_NotFound(t *testing.T) {
	bs := testBolt(t)

	_, err := bs.GetWebhookSecret("nonexistent-webhook")
	if err == nil {
		t.Fatal("expected error for nonexistent webhook")
	}
}

func TestBolt_GetWebhookSecret_EmptyHash(t *testing.T) {
	bs := testBolt(t)

	// Insert a webhook record with no secret_hash
	emptyRec := map[string]string{"no": "hash"}
	data, _ := json.Marshal(emptyRec)
	if err := bs.Set("webhooks", "wh-nohash", json.RawMessage(data), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	_, err := bs.GetWebhookSecret("wh-nohash")
	if err == nil {
		t.Fatal("expected error for webhook with empty secret_hash")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — decodeAPIKeyRecord edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_DecodeAPIKeyRecord_InvalidJSON(t *testing.T) {
	_, err := decodeAPIKeyRecord([]byte(`{invalid json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestBolt_DecodeAPIKeyRecord_MissingFields(t *testing.T) {
	// Missing key_prefix and user_id should cause validation error
	rec := apiKeyKVRecord{
		ID:   "ak-test",
		Name: "Test Key",
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	_, err = decodeAPIKeyRecord(data)
	if err == nil {
		t.Fatal("expected error for record with missing key_prefix and user_id")
	}
}

func TestBolt_DecodeAPIKeyRecord_FallbackFields(t *testing.T) {
	// Use Hash, Prefix, CreatedBy fields instead of KeyHash, KeyPrefix, UserID
	rec := apiKeyKVRecord{
		ID:        "ak-fallback",
		Name:      "Fallback Key",
		Hash:      "hash-value",
		Prefix:    "dmk_pre",
		CreatedBy: "user_fallback",
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	key, err := decodeAPIKeyRecord(data)
	if err != nil {
		t.Fatalf("decodeAPIKeyRecord: %v", err)
	}
	if key.KeyHash != "hash-value" {
		t.Errorf("KeyHash = %q, want hash-value", key.KeyHash)
	}
	if key.KeyPrefix != "dmk_pre" {
		t.Errorf("KeyPrefix = %q, want dmk_pre", key.KeyPrefix)
	}
	if key.UserID != "user_fallback" {
		t.Errorf("UserID = %q, want user_fallback", key.UserID)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — Close with nil store / non-closable
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_Close_NilStore(t *testing.T) {
	var bs *BoltStore
	err := bs.Close() // should be safe
	if err != nil {
		t.Errorf("Close on nil store: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — TTL edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_TTL_ExpiredKey_GetReturnsNotFound(t *testing.T) {
	bs := testBolt(t)

	// 1-second TTL
	if err := bs.Set("sessions", "quick-expire", "data", 1); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Wait for expiry
	time.Sleep(1500 * time.Millisecond)

	var got string
	err := bs.Get("sessions", "quick-expire", &got)
	if err == nil {
		t.Error("expected error for expired key")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// users.go — GetUsersByIDs edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_GetUsersByIDs_NilAndEmpty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	users, err := db.GetUsersByIDs(ctx, nil, "tenant")
	if err != nil {
		t.Fatalf("GetUsersByIDs(nil): %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}

	users, err = db.GetUsersByIDs(ctx, []string{}, "tenant")
	if err != nil {
		t.Fatalf("GetUsersByIDs([]): %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestSQLite_GetUsersByIDs_WithIDs(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := db.CreateTenantWithDefaults(ctx, "Users Tenant", "users-tenant")

	userID, err := db.CreateUserWithMembership(ctx, "user1@example.com", "$2a$12$hash", "User One", "active", tenantID, "role_admin")
	if err != nil {
		t.Fatalf("CreateUserWithMembership: %v", err)
	}

	// GetUsersByIDs queries users with a tenant_id column that does not
	// exist in the users table — see TestSQLite_GetUsersByIDs_Exec in
	// db_coverage_remaining_test.go for the known-bug assertion.
	// This test exercises the code path (query building + execution) and
	// verifies it fails with the expected SQL error.
	_, err = db.GetUsersByIDs(ctx, []string{userID}, tenantID)
	if err == nil {
		t.Fatal("expected error — users table has no tenant_id column")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// apps.go — GetAppsByIDs edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_GetAppsByIDs_NilAndEmpty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	apps, err := db.GetAppsByIDs(ctx, nil)
	if err != nil {
		t.Fatalf("GetAppsByIDs(nil): %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("expected 0 apps, got %d", len(apps))
	}

	apps, err = db.GetAppsByIDs(ctx, []string{})
	if err != nil {
		t.Fatalf("GetAppsByIDs([]): %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("expected 0 apps, got %d", len(apps))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// deployments.go — GetLatestDeploymentsByAppIDs edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_GetLatestDeploymentsByAppIDs_NilAndEmpty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	result, err := db.GetLatestDeploymentsByAppIDs(ctx, nil)
	if err != nil {
		t.Fatalf("GetLatestDeploymentsByAppIDs(nil): %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}

	result, err = db.GetLatestDeploymentsByAppIDs(ctx, []string{})
	if err != nil {
		t.Fatalf("GetLatestDeploymentsByAppIDs([]): %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSQLite_GetLatestDeploymentsByAppIDs_WithData(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "deploy-app-ids")

	dep := &core.Deployment{AppID: app.ID, Version: 1, Image: "img:1", Status: "running"}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	result, err := db.GetLatestDeploymentsByAppIDs(ctx, []string{app.ID})
	if err != nil {
		t.Fatalf("GetLatestDeploymentsByAppIDs: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	if result[app.ID] == nil {
		t.Fatal("expected deployment for app")
	}
	if result[app.ID].Version != 1 {
		t.Errorf("version = %d, want 1", result[app.ID].Version)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// deployments.go — AtomicNextDeployVersion
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_AtomicNextDeployVersion_Success(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "atomic-version-app")

	v, err := db.AtomicNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("AtomicNextDeployVersion: %v", err)
	}
	if v != 1 {
		t.Errorf("expected version 1, got %d", v)
	}

	// Create a deployment and get next version again
	dep := &core.Deployment{AppID: app.ID, Version: v, Image: "img:1", Status: "running"}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	v, err = db.AtomicNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("AtomicNextDeployVersion: %v", err)
	}
	if v != 2 {
		t.Errorf("expected version 2, got %d", v)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListDomainsByAppIDs — empty
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListDomainsByAppIDs_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	result, err := db.ListDomainsByAppIDs(ctx, nil, "tenant")
	if err != nil {
		t.Fatalf("ListDomainsByAppIDs(nil): %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}

	result, err = db.ListDomainsByAppIDs(ctx, []string{}, "tenant")
	if err != nil {
		t.Fatalf("ListDomainsByAppIDs([]): %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestSQLite_ListDomainsByAppIDs_WithData(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "domain-list-app")

	d1 := &core.Domain{AppID: app.ID, FQDN: "one.example.com", Type: "custom"}
	d2 := &core.Domain{AppID: app.ID, FQDN: "two.example.com", Type: "custom"}
	if err := db.CreateDomain(ctx, d1); err != nil {
		t.Fatalf("CreateDomain 1: %v", err)
	}
	if err := db.CreateDomain(ctx, d2); err != nil {
		t.Fatalf("CreateDomain 2: %v", err)
	}

	result, err := db.ListDomainsByAppIDs(ctx, []string{app.ID}, tenantID)
	if err != nil {
		t.Fatalf("ListDomainsByAppIDs: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 app in map, got %d", len(result))
	}
	if len(result[app.ID]) != 2 {
		t.Fatalf("expected 2 domains, got %d", len(result[app.ID]))
	}
}

func TestSQLite_ListDomainsByAppIDs_CrossTenant(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	result, err := db.ListDomainsByAppIDs(ctx, []string{"nonexistent-app"}, "tenant1")
	if err != nil {
		t.Fatalf("ListDomainsByAppIDs: %v", err)
	}
	if result == nil {
		t.Fatal("expected empty map, got nil")
	}
	if len(result) != 0 {
		t.Errorf("expected 0 domains, got %d", len(result))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — BatchSet edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_BatchSet_WithTTL(t *testing.T) {
	bs := testBolt(t)

	items := []core.BoltBatchItem{
		{Bucket: "sessions", Key: "ttl-key", Value: "will-expire", TTL: 1},
	}
	if err := bs.BatchSet(items); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}

	var got string
	if err := bs.Get("sessions", "ttl-key", &got); err != nil {
		t.Fatalf("Get before expiry: %v", err)
	}
	if got != "will-expire" {
		t.Errorf("got %q, want %q", got, "will-expire")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// secrets.go — edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_Secret_GetByScopeAndName_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetSecretByScopeAndName(ctx, "tenant", "NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

func TestSQLite_Secret_GetLatestVersion_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetLatestSecretVersion(ctx, "nonexistent-secret")
	if err == nil {
		t.Fatal("expected error for nonexistent secret version")
	}
}

func TestSQLite_Secret_DeleteSecret_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	err := db.DeleteSecret(ctx, "tenant-nonexistent", "secret-nonexistent")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// decodeTOTPBackupCodes edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestDecodeTOTPBackupCodes_EmptyString(t *testing.T) {
	result := decodeTOTPBackupCodes("")
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
}

func TestDecodeTOTPBackupCodes_BadJSON(t *testing.T) {
	result := decodeTOTPBackupCodes("{invalid}")
	if result != nil {
		t.Errorf("expected nil for invalid JSON, got %v", result)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// server.go — ListAllServers
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListAllServers_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	servers, err := db.ListAllServers(ctx)
	if err != nil {
		t.Fatalf("ListAllServers: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

func TestSQLite_ListServersByTenant_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	servers, err := db.ListServersByTenant(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ListServersByTenant: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers, got %d", len(servers))
	}
}

func TestSQLite_GetServer_Missing(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetServer(ctx, "nonexistent")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// invites.go — ListAllTenants
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListAllTenants_NoTenants(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// There should be no tenants in a fresh DB
	tenants, total, err := db.ListAllTenants(ctx, 10, 0)
	if err != nil {
		t.Fatalf("ListAllTenants: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(tenants) != 0 {
		t.Errorf("expected 0 tenants, got %d", len(tenants))
	}
}

func TestSQLite_ListInvitesByTenant_NoInvites(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	invites, err := db.ListInvitesByTenant(ctx, "nonexistent-tenant")
	if err != nil {
		t.Fatalf("ListInvitesByTenant: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("expected 0 invites, got %d", len(invites))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CreateTenantWithDefaults — duplicate slug error path
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_CreateTenantWithDefaults_CollidingSlug(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	slug := "dup-slug-" + core.GenerateID()[:8]
	_, err := db.CreateTenantWithDefaults(ctx, "First", slug)
	if err != nil {
		t.Fatalf("First CreateTenantWithDefaults: %v", err)
	}

	// Second with same slug should fail (UNIQUE constraint on slug)
	_, err = db.CreateTenantWithDefaults(ctx, "Second", slug)
	if err == nil {
		t.Fatal("expected error for duplicate slug")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListRoles — non built-in role
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_GetRole_Builtin(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	role, err := db.GetRole(ctx, "role_admin")
	if err != nil {
		t.Fatalf("GetRole: %v", err)
	}
	if !role.IsBuiltin {
		t.Error("expected Admin role to be built-in")
	}
	if role.Name != "Admin" {
		t.Errorf("name = %q, want Admin", role.Name)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListAuditLogs — empty
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListAuditLogs_NoEntries(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entries, total, err := db.ListAuditLogs(ctx, "nonexistent", 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListTeamMembers — empty
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListTeamMembers_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	members, err := db.ListTeamMembers(ctx, "nonexistent")
	if err != nil {
		t.Fatalf("ListTeamMembers: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("expected 0 members, got %d", len(members))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListRoles — empty tenant (no custom roles, only built-ins)
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListRoles_IncludesBuiltins(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, err := db.CreateTenantWithDefaults(ctx, "Role Tenant", "role-tenant")
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	roles, err := db.ListRoles(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListRoles: %v", err)
	}
	if len(roles) == 0 {
		t.Fatal("expected at least built-in roles")
	}
	hasBuiltin := false
	for _, r := range roles {
		if r.IsBuiltin {
			hasBuiltin = true
			break
		}
	}
	if !hasBuiltin {
		t.Error("expected at least one built-in role")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListProjectsByTenant — empty
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListProjectsByTenant_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "EmptyProj", Slug: "empty-proj", Status: "active", PlanID: "free"}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	projects, err := db.ListProjectsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListProjectsByTenant: %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListAppsByProject — empty
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListAppsByProject_NoApps(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)

	apps, err := db.ListAppsByProject(ctx, projID, tenantID)
	if err != nil {
		t.Fatalf("ListAppsByProject: %v", err)
	}
	if len(apps) != 0 {
		t.Errorf("expected 0 apps, got %d", len(apps))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListDomains — empty
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListDomainsByApp_ZeroDomains(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "no-domains-app")

	domains, err := db.ListDomainsByApp(ctx, app.ID, tenantID)
	if err != nil {
		t.Fatalf("ListDomainsByApp: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(domains))
	}
}

func TestSQLite_ListAllDomains_ZeroDomains(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	domains, err := db.ListAllDomains(ctx)
	if err != nil {
		t.Fatalf("ListAllDomains: %v", err)
	}
	if len(domains) != 0 {
		t.Errorf("expected 0 domains, got %d", len(domains))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// GetDomainByFQDN / GetDomain — not found
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_GetDomain_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetDomain(ctx, "nonexistent-domain")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListDeploymentsByStatus — empty
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListDeploymentsByStatus_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	deployments, err := db.ListDeploymentsByStatus(ctx, "running")
	if err != nil {
		t.Fatalf("ListDeploymentsByStatus: %v", err)
	}
	if len(deployments) != 0 {
		t.Errorf("expected 0 deployments, got %d", len(deployments))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListDeploymentsByApp — no deployments
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListDeploymentsByApp_ZeroDeploys(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "no-deploy-app")

	deployments, err := db.ListDeploymentsByApp(ctx, app.ID, 10)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp: %v", err)
	}
	if len(deployments) != 0 {
		t.Errorf("expected 0 deployments, got %d", len(deployments))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListUsageRecordsByTenant — empty
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListUsageRecordsByTenant_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	records, total, err := db.ListUsageRecordsByTenant(ctx, "nonexistent", 10, 0)
	if err != nil {
		t.Fatalf("ListUsageRecordsByTenant: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total 0, got %d", total)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

// ═════════════════}
// ═══════════════════════════════════════════════════════════════════════════════
// NewSQLiteKVStoreFromDB — nil db (initSchema nil db check)
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_NewSQLiteKVStoreFromDB_NilDB(t *testing.T) {
	_, err := NewSQLiteKVStoreFromDB(nil)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListDeploymentsByStatus — with data
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListDeploymentsByStatus_WithData(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "status-deploy-app")

	dep := &core.Deployment{AppID: app.ID, Version: 1, Image: "img:1", Status: "running"}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	deployments, err := db.ListDeploymentsByStatus(ctx, "running")
	if err != nil {
		t.Fatalf("ListDeploymentsByStatus: %v", err)
	}
	if len(deployments) != 1 {
		t.Fatalf("expected 1 deployment, got %d", len(deployments))
	}
	if deployments[0].AppID != app.ID {
		t.Errorf("app_id = %q, want %q", deployments[0].AppID, app.ID)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// UpdateDeployment — success path
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_UpdateDeployment_PersistsValues(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "update-deploy-app")

	dep := &core.Deployment{AppID: app.ID, Version: 1, Image: "img:1", Status: "deploying"}
	if err := db.CreateDeployment(ctx, dep); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	dep.Status = "completed"
	dep.ContainerID = "c123"
	if err := db.UpdateDeployment(ctx, dep); err != nil {
		t.Fatalf("UpdateDeployment: %v", err)
	}

	got, err := db.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if got.Status != "completed" {
		t.Errorf("status = %q, want completed", got.Status)
	}
	if got.ContainerID != "c123" {
		t.Errorf("container_id = %q, want c123", got.ContainerID)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListBackupsByTenant — with data
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListBackupsByTenant_WithData(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	backup := &core.Backup{
		TenantID:      tenantID,
		SourceType:    "database",
		SourceID:      "src-1",
		StorageTarget: "local",
		Status:        "pending",
		RetentionDays: 7,
	}
	if err := db.CreateBackup(ctx, backup); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	backups, total, err := db.ListBackupsByTenant(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListBackupsByTenant: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(backups) != 1 {
		t.Errorf("backups len = %d, want 1", len(backups))
	}
	if backups[0].SourceType != "database" {
		t.Errorf("source_type = %q, want database", backups[0].SourceType)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CreateAuditLog — success path
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_CreateAuditLog_AllFields(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entry := &core.AuditEntry{
		TenantID:     "tenant-1",
		UserID:       "user-1",
		Action:       "test.action",
		ResourceType: "test",
		ResourceID:   "res-1",
		DetailsJSON:  `{"key":"value"}`,
		IPAddress:    "127.0.0.1",
		UserAgent:    "test-agent",
	}
	if err := db.CreateAuditLog(ctx, entry); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}
}
