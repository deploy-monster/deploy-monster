package auth

import (
	"context"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

func TestClaimsFromContext_WithClaims(t *testing.T) {
	ctx := context.Background()
	original := &Claims{
		UserID:   "user-42",
		TenantID: "tenant-7",
		RoleID:   "role_admin",
		Email:    "admin@example.com",
	}

	ctx = ContextWithClaims(ctx, original)
	got := ClaimsFromContext(ctx)

	if got == nil {
		t.Fatal("expected non-nil claims from context")
	}
	if got.UserID != original.UserID {
		t.Errorf("UserID = %q, want %q", got.UserID, original.UserID)
	}
	if got.TenantID != original.TenantID {
		t.Errorf("TenantID = %q, want %q", got.TenantID, original.TenantID)
	}
	if got.RoleID != original.RoleID {
		t.Errorf("RoleID = %q, want %q", got.RoleID, original.RoleID)
	}
	if got.Email != original.Email {
		t.Errorf("Email = %q, want %q", got.Email, original.Email)
	}
	// Verify it is the same pointer
	if got != original {
		t.Error("expected same pointer returned from context")
	}
}

func TestClaimsFromContext_NilContext(t *testing.T) {
	// Context with no claims set should return nil
	ctx := context.Background()
	got := ClaimsFromContext(ctx)

	if got != nil {
		t.Errorf("expected nil claims from empty context, got %+v", got)
	}
}

func TestClaimsFromContext_WrongType(t *testing.T) {
	// If someone stored a non-Claims value under the same key mechanism,
	// the type assertion should safely return nil.
	ctx := context.WithValue(context.Background(), claimsKey, "not-a-claims-struct")
	got := ClaimsFromContext(ctx)

	if got != nil {
		t.Errorf("expected nil for wrong type in context, got %+v", got)
	}
}

func TestContextWithClaims(t *testing.T) {
	parent := context.Background()
	claims := &Claims{
		UserID:   "u1",
		TenantID: "t1",
		RoleID:   "r1",
		Email:    "e@e.com",
	}

	child := ContextWithClaims(parent, claims)

	// Child context should carry the claims
	got := ClaimsFromContext(child)
	if got == nil {
		t.Fatal("expected claims in child context")
	}
	if got.UserID != "u1" {
		t.Errorf("UserID = %q, want %q", got.UserID, "u1")
	}

	// Parent context should NOT have the claims
	parentGot := ClaimsFromContext(parent)
	if parentGot != nil {
		t.Error("parent context should not have claims")
	}
}

func TestContextWithClaims_NilClaims(t *testing.T) {
	ctx := ContextWithClaims(context.Background(), nil)
	got := ClaimsFromContext(ctx)

	if got != nil {
		t.Errorf("expected nil claims when nil was stored, got %+v", got)
	}
}

func TestGetRoleCached_Expired(t *testing.T) {
	roleID := "role_expired_test"

	// Manually insert an expired entry into the cache
	cache.mu.Lock()
	cache.roles[roleID] = &cachedRole{
		role: &models.Role{
			ID:              roleID,
			Name:            "Expired Role",
			PermissionsJSON: `["app.view"]`,
		},
		expiresAt: time.Now().Add(-1 * time.Second), // Already expired
	}
	cache.mu.Unlock()

	// GetRoleCached should return nil for expired entries
	got := GetRoleCached(roleID)
	if got != nil {
		t.Errorf("expected nil for expired cache entry, got %+v", got)
	}
}

func TestGetRoleCached_NotFound(t *testing.T) {
	got := GetRoleCached("role_does_not_exist_at_all")
	if got != nil {
		t.Errorf("expected nil for nonexistent cache entry, got %+v", got)
	}
}

func TestSetRoleInCache_ThenGet(t *testing.T) {
	role := &models.Role{
		ID:              "role_cache_roundtrip",
		Name:            "Cache Test Role",
		PermissionsJSON: `["app.view","app.deploy"]`,
	}

	SetRoleInCache(role)
	got := GetRoleCached("role_cache_roundtrip")

	if got == nil {
		t.Fatal("expected non-nil role from cache")
	}
	if got.ID != role.ID {
		t.Errorf("ID = %q, want %q", got.ID, role.ID)
	}
	if got.Name != role.Name {
		t.Errorf("Name = %q, want %q", got.Name, role.Name)
	}
}

func TestSetRoleInCache_Overwrite(t *testing.T) {
	roleID := "role_overwrite_test"

	SetRoleInCache(&models.Role{
		ID:              roleID,
		Name:            "Version 1",
		PermissionsJSON: `["app.view"]`,
	})

	SetRoleInCache(&models.Role{
		ID:              roleID,
		Name:            "Version 2",
		PermissionsJSON: `["app.view","app.deploy"]`,
	})

	got := GetRoleCached(roleID)
	if got == nil {
		t.Fatal("expected non-nil role from cache")
	}
	if got.Name != "Version 2" {
		t.Errorf("Name = %q, want %q (should be overwritten)", got.Name, "Version 2")
	}
}

func TestHasPermission_ExpiredCacheRole(t *testing.T) {
	roleID := "role_has_perm_expired"

	cache.mu.Lock()
	cache.roles[roleID] = &cachedRole{
		role: &models.Role{
			ID:              roleID,
			PermissionsJSON: `["app.view"]`,
		},
		expiresAt: time.Now().Add(-1 * time.Minute),
	}
	cache.mu.Unlock()

	// Should return false because the cache entry is expired
	if HasPermission(roleID, "app.view") {
		t.Error("expected false for permission check with expired cache entry")
	}
}
