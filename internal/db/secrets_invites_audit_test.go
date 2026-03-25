package db

import (
	"context"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Secrets CRUD
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_CreateSecret(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	secret := &core.Secret{
		TenantID:       tenantID,
		Name:           "DB_PASSWORD",
		Type:           "env",
		Description:    "Production database password",
		Scope:          "tenant",
		CurrentVersion: 1,
	}

	if err := db.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	if secret.ID == "" {
		t.Error("expected auto-generated ID")
	}
}

func TestSQLite_CreateSecret_WithID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	secret := &core.Secret{
		ID:             "secret-custom-id",
		TenantID:       tenantID,
		Name:           "API_KEY",
		Type:           "env",
		Description:    "External API key",
		Scope:          "project",
		CurrentVersion: 1,
	}

	if err := db.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret with ID: %v", err)
	}
	if secret.ID != "secret-custom-id" {
		t.Errorf("expected ID 'secret-custom-id', got %q", secret.ID)
	}
}

func TestSQLite_CreateSecretVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	// Create parent secret first
	secret := &core.Secret{
		TenantID:       tenantID,
		Name:           "SECRET_FOR_VERSION",
		Type:           "env",
		Description:    "Test secret",
		Scope:          "global",
		CurrentVersion: 1,
	}
	if err := db.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	version := &core.SecretVersion{
		SecretID:  secret.ID,
		Version:   1,
		ValueEnc:  "base64-encrypted-value-here",
		CreatedBy: "user-123",
	}

	if err := db.CreateSecretVersion(ctx, version); err != nil {
		t.Fatalf("CreateSecretVersion: %v", err)
	}
	if version.ID == "" {
		t.Error("expected auto-generated version ID")
	}
}

func TestSQLite_CreateSecretVersion_WithID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	secret := &core.Secret{
		TenantID: tenantID, Name: "SEC_V_ID", Type: "env",
		Scope: "tenant", CurrentVersion: 1,
	}
	db.CreateSecret(ctx, secret)

	version := &core.SecretVersion{
		ID:        "ver-custom-id",
		SecretID:  secret.ID,
		Version:   1,
		ValueEnc:  "encrypted",
		CreatedBy: "admin",
	}

	if err := db.CreateSecretVersion(ctx, version); err != nil {
		t.Fatalf("CreateSecretVersion with ID: %v", err)
	}
	if version.ID != "ver-custom-id" {
		t.Errorf("expected ID 'ver-custom-id', got %q", version.ID)
	}
}

func TestSQLite_ListSecretsByTenant(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantA, _ := setupTenantAndProject(t, db)
	tenantB, _ := setupTenantAndProject(t, db)

	// Create secrets for tenant A
	for _, name := range []string{"SECRET_A1", "SECRET_A2", "SECRET_A3"} {
		s := &core.Secret{
			TenantID: tenantA, Name: name, Type: "env",
			Scope: "tenant", CurrentVersion: 1,
		}
		if err := db.CreateSecret(ctx, s); err != nil {
			t.Fatalf("CreateSecret(%s): %v", name, err)
		}
	}

	// Create secret for tenant B
	s := &core.Secret{
		TenantID: tenantB, Name: "SECRET_B1", Type: "env",
		Scope: "tenant", CurrentVersion: 1,
	}
	db.CreateSecret(ctx, s)

	// List tenant A secrets
	secretsA, err := db.ListSecretsByTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("ListSecretsByTenant(A): %v", err)
	}
	if len(secretsA) != 3 {
		t.Errorf("expected 3 secrets for tenant A, got %d", len(secretsA))
	}

	// List tenant B secrets
	secretsB, err := db.ListSecretsByTenant(ctx, tenantB)
	if err != nil {
		t.Fatalf("ListSecretsByTenant(B): %v", err)
	}
	if len(secretsB) != 1 {
		t.Errorf("expected 1 secret for tenant B, got %d", len(secretsB))
	}
	if secretsB[0].Name != "SECRET_B1" {
		t.Errorf("expected name 'SECRET_B1', got %q", secretsB[0].Name)
	}
}

func TestSQLite_ListSecretsByTenant_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	secrets, err := db.ListSecretsByTenant(ctx, "nonexistent-tenant")
	if err != nil {
		t.Fatalf("ListSecretsByTenant: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("expected 0 secrets, got %d", len(secrets))
	}
}

func TestSQLite_ListSecretsByTenant_FieldValues(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, projID := setupTenantAndProject(t, db)

	secret := &core.Secret{
		TenantID:       tenantID,
		ProjectID:      projID,
		AppID:          "app-xyz",
		Name:           "FULL_SECRET",
		Type:           "file",
		Description:    "A fully populated secret",
		Scope:          "app",
		CurrentVersion: 3,
	}
	db.CreateSecret(ctx, secret)

	secrets, err := db.ListSecretsByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListSecretsByTenant: %v", err)
	}
	if len(secrets) != 1 {
		t.Fatalf("expected 1 secret, got %d", len(secrets))
	}

	got := secrets[0]
	if got.ID != secret.ID {
		t.Errorf("ID = %q, want %q", got.ID, secret.ID)
	}
	if got.TenantID != tenantID {
		t.Errorf("TenantID = %q, want %q", got.TenantID, tenantID)
	}
	if got.ProjectID != projID {
		t.Errorf("ProjectID = %q, want %q", got.ProjectID, projID)
	}
	if got.Name != "FULL_SECRET" {
		t.Errorf("Name = %q, want 'FULL_SECRET'", got.Name)
	}
	if got.Type != "file" {
		t.Errorf("Type = %q, want 'file'", got.Type)
	}
	if got.Description != "A fully populated secret" {
		t.Errorf("Description = %q, want 'A fully populated secret'", got.Description)
	}
	if got.Scope != "app" {
		t.Errorf("Scope = %q, want 'app'", got.Scope)
	}
	if got.CurrentVersion != 3 {
		t.Errorf("CurrentVersion = %d, want 3", got.CurrentVersion)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Invitations CRUD
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_CreateInvite(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	invite := &core.Invitation{
		TenantID:  tenantID,
		Email:     "invited@example.com",
		RoleID:    "role_admin",
		InvitedBy: "user-owner",
		TokenHash: "hash-of-token-abc123",
		ExpiresAt: time.Now().Add(72 * time.Hour),
		Status:    "pending",
	}

	if err := db.CreateInvite(ctx, invite); err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if invite.ID == "" {
		t.Error("expected auto-generated ID")
	}
}

func TestSQLite_CreateInvite_WithID(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	invite := &core.Invitation{
		ID:        "invite-custom-id",
		TenantID:  tenantID,
		Email:     "custom@example.com",
		RoleID:    "role_viewer",
		InvitedBy: "user-admin",
		TokenHash: "hash-custom",
		ExpiresAt: time.Now().Add(24 * time.Hour),
		Status:    "pending",
	}

	if err := db.CreateInvite(ctx, invite); err != nil {
		t.Fatalf("CreateInvite with ID: %v", err)
	}
	if invite.ID != "invite-custom-id" {
		t.Errorf("expected ID 'invite-custom-id', got %q", invite.ID)
	}
}

func TestSQLite_ListInvitesByTenant(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantA, _ := setupTenantAndProject(t, db)
	tenantB, _ := setupTenantAndProject(t, db)

	// Create invites for tenant A
	for _, email := range []string{"a@test.com", "b@test.com"} {
		inv := &core.Invitation{
			TenantID: tenantA, Email: email, RoleID: "role_admin",
			TokenHash: "hash-" + email, ExpiresAt: time.Now().Add(24 * time.Hour), Status: "pending",
		}
		if err := db.CreateInvite(ctx, inv); err != nil {
			t.Fatalf("CreateInvite(%s): %v", email, err)
		}
	}

	// Create invite for tenant B
	inv := &core.Invitation{
		TenantID: tenantB, Email: "c@test.com", RoleID: "role_viewer",
		TokenHash: "hash-c", ExpiresAt: time.Now().Add(24 * time.Hour), Status: "pending",
	}
	db.CreateInvite(ctx, inv)

	// List tenant A
	invitesA, err := db.ListInvitesByTenant(ctx, tenantA)
	if err != nil {
		t.Fatalf("ListInvitesByTenant(A): %v", err)
	}
	if len(invitesA) != 2 {
		t.Errorf("expected 2 invites for tenant A, got %d", len(invitesA))
	}

	// List tenant B
	invitesB, err := db.ListInvitesByTenant(ctx, tenantB)
	if err != nil {
		t.Fatalf("ListInvitesByTenant(B): %v", err)
	}
	if len(invitesB) != 1 {
		t.Errorf("expected 1 invite for tenant B, got %d", len(invitesB))
	}
}

func TestSQLite_ListInvitesByTenant_Empty(t *testing.T) {
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

func TestSQLite_ListInvitesByTenant_FieldValues(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)
	expires := time.Now().Add(48 * time.Hour).Truncate(time.Second)

	invite := &core.Invitation{
		TenantID:  tenantID,
		Email:     "detail@test.com",
		RoleID:    "role_developer",
		InvitedBy: "owner-user",
		TokenHash: "hash-detail",
		ExpiresAt: expires,
		Status:    "accepted",
	}
	db.CreateInvite(ctx, invite)

	invites, err := db.ListInvitesByTenant(ctx, tenantID)
	if err != nil {
		t.Fatalf("ListInvitesByTenant: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("expected 1 invite, got %d", len(invites))
	}

	got := invites[0]
	if got.TenantID != tenantID {
		t.Errorf("TenantID = %q, want %q", got.TenantID, tenantID)
	}
	if got.Email != "detail@test.com" {
		t.Errorf("Email = %q, want 'detail@test.com'", got.Email)
	}
	if got.RoleID != "role_developer" {
		t.Errorf("RoleID = %q, want 'role_developer'", got.RoleID)
	}
	if got.InvitedBy != "owner-user" {
		t.Errorf("InvitedBy = %q, want 'owner-user'", got.InvitedBy)
	}
	if got.Status != "accepted" {
		t.Errorf("Status = %q, want 'accepted'", got.Status)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ListAllTenants (admin only)
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListAllTenants(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Create multiple tenants
	for i := 0; i < 5; i++ {
		tenant := &core.Tenant{
			Name: "AllTenant-" + core.GenerateID()[:6], Slug: "all-" + core.GenerateID()[:8],
			Status: "active", PlanID: "free",
		}
		if err := db.CreateTenant(ctx, tenant); err != nil {
			t.Fatalf("CreateTenant: %v", err)
		}
	}

	// List all with pagination
	tenants, total, err := db.ListAllTenants(ctx, 3, 0)
	if err != nil {
		t.Fatalf("ListAllTenants: %v", err)
	}
	if total < 5 {
		t.Errorf("expected total >= 5, got %d", total)
	}
	if len(tenants) != 3 {
		t.Errorf("expected 3 tenants (limit=3), got %d", len(tenants))
	}

	// Second page
	tenants2, total2, err := db.ListAllTenants(ctx, 3, 3)
	if err != nil {
		t.Fatalf("ListAllTenants page 2: %v", err)
	}
	if total2 != total {
		t.Errorf("total should be consistent: %d vs %d", total, total2)
	}
	if len(tenants2) < 2 {
		t.Errorf("expected at least 2 tenants on page 2, got %d", len(tenants2))
	}
}

func TestSQLite_ListAllTenants_FieldValues(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenant := &core.Tenant{
		Name: "FieldCheck", Slug: "fieldcheck-" + core.GenerateID()[:8],
		Status: "active", PlanID: "pro", AvatarURL: "https://example.com/avatar.png",
	}
	db.CreateTenant(ctx, tenant)

	tenants, _, err := db.ListAllTenants(ctx, 100, 0)
	if err != nil {
		t.Fatalf("ListAllTenants: %v", err)
	}

	// Find our tenant
	var found *core.Tenant
	for i := range tenants {
		if tenants[i].ID == tenant.ID {
			found = &tenants[i]
			break
		}
	}
	if found == nil {
		t.Fatal("expected to find the created tenant")
	}
	if found.Name != "FieldCheck" {
		t.Errorf("Name = %q, want 'FieldCheck'", found.Name)
	}
	if found.PlanID != "pro" {
		t.Errorf("PlanID = %q, want 'pro'", found.PlanID)
	}
	if found.Status != "active" {
		t.Errorf("Status = %q, want 'active'", found.Status)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Audit Log CRUD
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_CreateAuditLog(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	entry := &core.AuditEntry{
		TenantID:     tenantID,
		UserID:       "user-abc",
		Action:       "app.deploy",
		ResourceType: "application",
		ResourceID:   "app-123",
		DetailsJSON:  `{"version":5}`,
		IPAddress:    "192.168.1.100",
		UserAgent:    "Mozilla/5.0",
	}

	if err := db.CreateAuditLog(ctx, entry); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}
}

func TestSQLite_ListAuditLogs(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantA, _ := setupTenantAndProject(t, db)
	tenantB, _ := setupTenantAndProject(t, db)

	// Create audit entries for tenant A
	for _, action := range []string{"app.create", "app.deploy", "app.delete", "user.login", "settings.update"} {
		entry := &core.AuditEntry{
			TenantID:     tenantA,
			UserID:       "user-1",
			Action:       action,
			ResourceType: "application",
			ResourceID:   "app-" + action,
			IPAddress:    "10.0.0.1",
			UserAgent:    "TestAgent",
		}
		if err := db.CreateAuditLog(ctx, entry); err != nil {
			t.Fatalf("CreateAuditLog(%s): %v", action, err)
		}
	}

	// Create entry for tenant B
	db.CreateAuditLog(ctx, &core.AuditEntry{
		TenantID: tenantB, UserID: "user-2", Action: "app.create",
		ResourceType: "application", ResourceID: "app-b1",
		IPAddress: "10.0.0.2", UserAgent: "TestAgent",
	})

	// List tenant A with pagination
	entries, total, err := db.ListAuditLogs(ctx, tenantA, 3, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if total != 5 {
		t.Errorf("expected total 5, got %d", total)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries (limit=3), got %d", len(entries))
	}

	// Second page
	entries2, total2, err := db.ListAuditLogs(ctx, tenantA, 3, 3)
	if err != nil {
		t.Fatalf("ListAuditLogs page 2: %v", err)
	}
	if total2 != 5 {
		t.Errorf("expected total 5, got %d", total2)
	}
	if len(entries2) != 2 {
		t.Errorf("expected 2 entries on page 2, got %d", len(entries2))
	}

	// Tenant B should have 1 entry
	entriesB, totalB, err := db.ListAuditLogs(ctx, tenantB, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs(B): %v", err)
	}
	if totalB != 1 {
		t.Errorf("expected total 1 for tenant B, got %d", totalB)
	}
	if len(entriesB) != 1 {
		t.Errorf("expected 1 entry for tenant B, got %d", len(entriesB))
	}
}

func TestSQLite_ListAuditLogs_Empty(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	entries, total, err := db.ListAuditLogs(ctx, "nonexistent-tenant", 10, 0)
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

func TestSQLite_ListAuditLogs_FieldValues(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantID, _ := setupTenantAndProject(t, db)

	entry := &core.AuditEntry{
		TenantID:     tenantID,
		UserID:       "user-field-check",
		Action:       "domain.verify",
		ResourceType: "domain",
		ResourceID:   "dom-456",
		DetailsJSON:  `{"fqdn":"example.com","verified":true}`,
		IPAddress:    "203.0.113.50",
		UserAgent:    "DeployMonster/1.0",
	}
	db.CreateAuditLog(ctx, entry)

	entries, _, err := db.ListAuditLogs(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	got := entries[0]
	if got.TenantID != tenantID {
		t.Errorf("TenantID = %q, want %q", got.TenantID, tenantID)
	}
	if got.UserID != "user-field-check" {
		t.Errorf("UserID = %q, want 'user-field-check'", got.UserID)
	}
	if got.Action != "domain.verify" {
		t.Errorf("Action = %q, want 'domain.verify'", got.Action)
	}
	if got.ResourceType != "domain" {
		t.Errorf("ResourceType = %q, want 'domain'", got.ResourceType)
	}
	if got.ResourceID != "dom-456" {
		t.Errorf("ResourceID = %q, want 'dom-456'", got.ResourceID)
	}
	if got.IPAddress != "203.0.113.50" {
		t.Errorf("IPAddress = %q, want '203.0.113.50'", got.IPAddress)
	}
	if got.UserAgent != "DeployMonster/1.0" {
		t.Errorf("UserAgent = %q, want 'DeployMonster/1.0'", got.UserAgent)
	}
	if got.ID == 0 {
		t.Error("expected non-zero auto-increment ID")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// BoltStore.List coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_List_Empty(t *testing.T) {
	store := testBolt(t)

	keys, err := store.List("sessions")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestBolt_List_MultipleKeys(t *testing.T) {
	store := testBolt(t)

	store.Set("sessions", "key-a", "value-a", 0)
	store.Set("sessions", "key-b", "value-b", 0)
	store.Set("sessions", "key-c", "value-c", 0)

	keys, err := store.List("sessions")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(keys))
	}

	keySet := make(map[string]bool)
	for _, k := range keys {
		keySet[k] = true
	}
	for _, expected := range []string{"key-a", "key-b", "key-c"} {
		if !keySet[expected] {
			t.Errorf("expected key %q in list", expected)
		}
	}
}

func TestBolt_List_SkipsExpiredKeys(t *testing.T) {
	store := testBolt(t)

	store.Set("sessions", "active", "alive", 0)
	store.Set("sessions", "expired", "dead", 1) // 1 second TTL

	// Wait for expiry
	time.Sleep(1100 * time.Millisecond)

	keys, err := store.List("sessions")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key (expired should be skipped), got %d", len(keys))
	}
	if len(keys) > 0 && keys[0] != "active" {
		t.Errorf("expected key 'active', got %q", keys[0])
	}
}

func TestBolt_List_NonexistentBucket(t *testing.T) {
	store := testBolt(t)

	_, err := store.List("nonexistent_bucket")
	if err == nil {
		t.Error("expected error for nonexistent bucket")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// GetUserByEmail — not found edge case
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_GetUserByEmail_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	_, err := db.GetUserByEmail(ctx, "nobody@example.com")
	if err != core.ErrNotFound {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
