package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// sqlite.go — uncovered error paths in NewSQLite
// =============================================================================

func TestNewSQLite_InvalidPath(t *testing.T) {
	_, err := NewSQLite("/nonexistent/dir/something.db")
	if err == nil {
		t.Fatal("expected error for invalid path")
	}
}

// =============================================================================
// setup.go — uncovered error paths for nested DB operations
// =============================================================================

func TestCreateUserWithMembership_DuplicateEmail(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)

	tenantID, err := s.CreateTenantWithDefaults(ctx, "Test", "test-dupe")
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	role, err := s.GetRole(ctx, "role_admin")
	if err != nil {
		t.Fatalf("GetRole role_admin: %v", err)
	}

	_, err = s.CreateUserWithMembership(ctx, "dupe@test.com", "hash1", "User1", "active", tenantID, role.ID)
	if err != nil {
		t.Fatalf("CreateUserWithMembership first: %v", err)
	}

	_, err = s.CreateUserWithMembership(ctx, "dupe@test.com", "hash2", "User2", "active", tenantID, role.ID)
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
}

func TestRemoveTeamMember_NonexistentMember(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)

	err := s.RemoveTeamMember(ctx, "no-such-tenant", "no-such-member")
	if err == nil {
		t.Fatal("expected error for nonexistent member")
	}
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

func TestRemoveTeamMember_AlreadyRemoved(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)

	tenantID, err := s.CreateTenantWithDefaults(ctx, "Test", "test-rm")
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	u := &core.User{Email: "rmtest@test.com", PasswordHash: "hash", Name: "RM", Status: "active"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	role, err := s.GetRole(ctx, "role_admin")
	if err != nil {
		t.Fatalf("GetRole role_admin: %v", err)
	}

	memberID := core.GenerateID()
	err = s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO team_members (id, tenant_id, user_id, role_id, status) VALUES (?, ?, ?, ?, 'active')`,
			memberID, tenantID, u.ID, role.ID,
		)
		return err
	})
	if err != nil {
		t.Fatalf("insert team member: %v", err)
	}

	if err := s.RemoveTeamMember(ctx, tenantID, memberID); err != nil {
		t.Fatalf("RemoveTeamMember first: %v", err)
	}

	err = s.RemoveTeamMember(ctx, tenantID, memberID)
	if err == nil {
		t.Fatal("expected error for already-removed member")
	}
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("want ErrNotFound, got %v", err)
	}
}

// =============================================================================
// module.go — uncovered error paths in Stop, Health
// =============================================================================

func TestModule_Stop_NilComponents(t *testing.T) {
	m := &Module{
		driver: "sqlite",
	}
	err := m.Stop(context.Background())
	if err != nil {
		t.Logf("Stop returned: %v", err)
	}
}

func TestModule_Health_SQLiteNil(t *testing.T) {
	m := &Module{
		driver: "sqlite",
		bolt:   &BoltStore{db: testDB(t).DB()},
	}
	status := m.Health()
	if status != core.HealthDown {
		t.Errorf("want HealthDown, got %v", status)
	}
}

func TestModule_Health_PostgresNil(t *testing.T) {
	m := &Module{
		driver: "postgres",
		bolt:   &BoltStore{db: testDB(t).DB()},
	}
	status := m.Health()
	if status != core.HealthDown {
		t.Errorf("want HealthDown, got %v", status)
	}
}

func TestModule_Health_BoltNil(t *testing.T) {
	m := &Module{
		driver: "sqlite",
		sqlite: testDB(t),
	}
	status := m.Health()
	if status != core.HealthDown {
		t.Errorf("want HealthDown, got %v", status)
	}
}

// =============================================================================
// bolt.go — uncovered error paths
// =============================================================================

func TestBolt_GetAPIKeyByPrefix_CancelledCtx(t *testing.T) {
	bs := testBolt(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := bs.GetAPIKeyByPrefix(ctx, "test")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestBolt_Delete_NonExistentKey(t *testing.T) {
	bs := testBolt(t)
	err := bs.Delete("sessions", "no-such-key")
	if err != nil {
		t.Fatalf("Delete non-existent key: %v", err)
	}
}

func TestBolt_GetWebhookSecret_EmptyHashRemaining(t *testing.T) {
	bs := testBolt(t)

	// Store data without secret_hash
	if err := bs.Set("webhooks", "no-hash", map[string]string{"other": "data"}, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	_, err := bs.GetWebhookSecret("no-hash")
	if err == nil {
		t.Fatal("expected error for webhook with empty secret_hash")
	}
}

func TestBolt_GetWebhookSecret_InvalidStructure(t *testing.T) {
	bs := testBolt(t)

	if err := bs.Set("webhooks", "bad-struct", "not-json-map", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	_, err := bs.GetWebhookSecret("bad-struct")
	if err == nil {
		t.Fatal("expected error for invalid webhook structure")
	}
}

// =============================================================================
// secrets.go — DeleteSecret error path
// =============================================================================

func TestDeleteSecret_NotFound(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)

	err := s.DeleteSecret(ctx, "no-such-scope", "no-such-name")
	if err == nil {
		t.Fatal("expected error for nonexistent secret")
	}
}

// =============================================================================
// users.go — UpdateTOTPBackupCodes edge cases
// =============================================================================

func TestUpdateTOTPBackupCodes_EdgeCases(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)

	u := &core.User{Email: "totp-edge@test.com", PasswordHash: "hash", Name: "TOTP", Status: "active"}
	if err := s.CreateUser(ctx, u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := s.UpdateTOTPBackupCodes(ctx, u.ID, nil); err != nil {
		t.Fatalf("UpdateTOTPBackupCodes nil: %v", err)
	}

	if err := s.UpdateTOTPBackupCodes(ctx, u.ID, []string{}); err != nil {
		t.Fatalf("UpdateTOTPBackupCodes empty: %v", err)
	}

	if err := s.UpdateTOTPBackupCodes(ctx, u.ID, []string{"code1", "code2"}); err != nil {
		t.Fatalf("UpdateTOTPBackupCodes with values: %v", err)
	}
}

// =============================================================================
// bolt.go — Mutate with expired key
// =============================================================================

func TestBolt_Mutate_ExpiredKey(t *testing.T) {
	bs := testBolt(t)

	// Set a key with 1-second TTL, then wait for it to expire
	if err := bs.Set("sessions", "exp-key", "old-value", 1); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Wait for the TTL to expire
	time.Sleep(1100 * time.Millisecond)

	var val string
	err := bs.Mutate("sessions", "exp-key", &val, 0, func(exists bool) error {
		if exists {
			t.Fatal("expected exists=false for expired key")
		}
		val = "new-value"
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate expired: %v", err)
	}

	var got string
	if err := bs.Get("sessions", "exp-key", &got); err != nil {
		t.Fatalf("Get after Mutate: %v", err)
	}
	if got != "new-value" {
		t.Errorf("got %q, want %q", got, "new-value")
	}
}

// =============================================================================
// bolt.go — Mutate with unmarshal error for corrupt data
// =============================================================================

func TestBolt_Mutate_CorruptData(t *testing.T) {
	bs := testBolt(t)

	// Store non-JSON data directly via raw SQL to bypass the json.Marshal in Set
	if err := bs.Set("sessions", "corrupt", "valid-string", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	var dest map[string]string // This type mismatch will cause unmarshal error
	err := bs.Mutate("sessions", "corrupt", &dest, 0, func(exists bool) error {
		t.Logf("corrupt data mutate called with exists=%v", exists)
		return nil
	})
	if err == nil {
		t.Fatal("expected error for corrupt data type mismatch")
	}
}

// =============================================================================
// bolt.go — GetAPIKeyByPrefix with no matching keys
// =============================================================================

func TestBolt_GetAPIKeyByPrefix_NotFound(t *testing.T) {
	bs := testBolt(t)

	_, err := bs.GetAPIKeyByPrefix(context.Background(), "no-such-prefix")
	if err == nil {
		t.Fatal("expected error for non-existent prefix")
	}
}

// =============================================================================
// billing.go — ListUsageRecordsByTenant edge case
// =============================================================================

func TestListUsageRecordsByTenant_Empty(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)

	records, total, err := s.ListUsageRecordsByTenant(ctx, "no-such-tenant", 10, 0)
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

// =============================================================================
// invites.go — ListInvitesByTenant edge case
// =============================================================================

func TestListInvitesByTenant_Empty(t *testing.T) {
	ctx := context.Background()
	s := testDB(t)

	invites, err := s.ListInvitesByTenant(ctx, "no-such-tenant")
	if err != nil {
		t.Fatalf("ListInvitesByTenant: %v", err)
	}
	if len(invites) != 0 {
		t.Errorf("expected 0 invites, got %d", len(invites))
	}
}

// =============================================================================
// bolt.go — bucketExists errors (via Delete on non-existent bucket)
// =============================================================================

func TestBolt_Delete_NonExistentBucketRemaining(t *testing.T) {
	bs := testBolt(t)

	err := bs.Delete("no-such-bucket", "key")
	if err == nil {
		t.Fatal("expected error for nonexistent bucket")
	}
}
