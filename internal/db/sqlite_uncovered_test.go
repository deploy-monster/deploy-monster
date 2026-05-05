package db

import (
	"context"
	"errors"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestSQLite_UpdateTOTPEnabled covers the SQLite-side persistence the
// auth TOTP flow relies on — the function had 0% coverage even though
// the auth-side TOTPService tests exercise the full path through a
// fake store.
func TestSQLite_UpdateTOTPEnabled(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	user := &core.User{
		Email:        "totp@example.com",
		PasswordHash: "$2a$12$fakehashhere",
		Name:         "TOTP User",
		Status:       "active",
	}
	if err := db.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if err := db.UpdateTOTPEnabled(ctx, user.ID, true, "ciphertext-secret"); err != nil {
		t.Fatalf("UpdateTOTPEnabled enable: %v", err)
	}

	got, err := db.GetUserByEmail(ctx, "totp@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if !got.TOTPEnabled {
		t.Fatal("TOTPEnabled was not persisted as true")
	}
	if got.TOTPSecret != "ciphertext-secret" {
		t.Fatalf("TOTPSecret = %q, want ciphertext-secret", got.TOTPSecret)
	}

	if err := db.UpdateTOTPEnabled(ctx, user.ID, false, ""); err != nil {
		t.Fatalf("UpdateTOTPEnabled disable: %v", err)
	}
	got, err = db.GetUserByEmail(ctx, "totp@example.com")
	if err != nil {
		t.Fatalf("GetUserByEmail after disable: %v", err)
	}
	if got.TOTPEnabled {
		t.Fatal("TOTPEnabled was not cleared")
	}
	if got.TOTPSecret != "" {
		t.Fatalf("TOTPSecret = %q, want empty", got.TOTPSecret)
	}
}

// TestSQLite_TeamMember_ListAndRemove walks the two TeamMember helpers
// that had 0% coverage. Seeding goes through CreateUserWithMembership
// so we exercise the same insert path the runtime uses.
func TestSQLite_TeamMember_ListAndRemove(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, err := db.CreateTenantWithDefaults(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	roleID := core.GenerateID()
	if _, err := db.DB().ExecContext(ctx,
		`INSERT INTO roles (id, tenant_id, name, description, permissions_json, is_builtin)
		 VALUES (?, ?, 'member', '', '[]', 0)`, roleID, tenantID); err != nil {
		t.Fatalf("seed role: %v", err)
	}

	uid1, err := db.CreateUserWithMembership(ctx, "a@example.com", "h", "Alice", "active", tenantID, roleID)
	if err != nil {
		t.Fatalf("CreateUserWithMembership a: %v", err)
	}
	uid2, err := db.CreateUserWithMembership(ctx, "b@example.com", "h", "Bob", "active", tenantID, roleID)
	if err != nil {
		t.Fatalf("CreateUserWithMembership b: %v", err)
	}

	members, err := db.ListTeamMembers(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListTeamMembers: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("ListTeamMembers returned %d members, want 2", len(members))
	}
	seen := map[string]bool{}
	for _, m := range members {
		seen[m.UserID] = true
		if m.Status != "active" {
			t.Fatalf("expected active member, got status=%q", m.Status)
		}
	}
	if !seen[uid1] || !seen[uid2] {
		t.Fatalf("ListTeamMembers missing expected users: %+v", seen)
	}

	memberToRemove := members[0]
	if err := db.RemoveTeamMember(ctx, tenantID, memberToRemove.ID); err != nil {
		t.Fatalf("RemoveTeamMember: %v", err)
	}

	left, err := db.ListTeamMembers(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListTeamMembers after remove: %v", err)
	}
	if len(left) != 1 {
		t.Fatalf("after RemoveTeamMember active count = %d, want 1", len(left))
	}
	if left[0].ID == memberToRemove.ID {
		t.Fatal("removed member is still listed as active")
	}

	// Removing the same member again must report ErrNotFound (the
	// status='active' guard in the UPDATE means the row is no longer
	// matched).
	if err := db.RemoveTeamMember(ctx, tenantID, memberToRemove.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("re-RemoveTeamMember err=%v, want ErrNotFound", err)
	}
}

// TestSQLite_DeleteSecret covers the SQLite DeleteSecret implementation —
// success path and the "not found" path that returns ErrNotFound.
func TestSQLite_DeleteSecret(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, err := db.CreateTenantWithDefaults(ctx, "Acme", "acme")
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	secret := &core.Secret{
		TenantID:       tenantID,
		Name:           "DATABASE_URL",
		Type:           "string",
		Description:    "test secret",
		Scope:          "tenant",
		CurrentVersion: 0,
	}
	if err := db.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	if err := db.DeleteSecret(ctx, tenantID, secret.ID); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	listed, err := db.ListSecretsByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListSecretsByTenant: %v", err)
	}
	for _, s := range listed {
		if s.ID == secret.ID {
			t.Fatal("DeleteSecret did not remove the row")
		}
	}

	// Re-deleting must surface ErrNotFound.
	if err := db.DeleteSecret(ctx, tenantID, secret.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("re-DeleteSecret err=%v, want ErrNotFound", err)
	}

	// Deleting from a different tenant must also be ErrNotFound rather
	// than a silent success — the WHERE clause pairs id and tenant_id.
	other := &core.Secret{TenantID: tenantID, Name: "API_KEY", Type: "string", Scope: "tenant"}
	if err := db.CreateSecret(ctx, other); err != nil {
		t.Fatalf("CreateSecret other: %v", err)
	}
	if err := db.DeleteSecret(ctx, "wrong-tenant", other.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("cross-tenant DeleteSecret err=%v, want ErrNotFound", err)
	}
}
