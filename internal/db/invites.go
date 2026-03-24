package db

import (
	"context"
	"database/sql"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateInvite inserts a new team invitation.
func (s *SQLiteDB) CreateInvite(ctx context.Context, invite *core.Invitation) error {
	if invite.ID == "" {
		invite.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO invitations (id, tenant_id, email, role_id, invited_by, token_hash, expires_at, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			invite.ID, invite.TenantID, invite.Email, invite.RoleID,
			invite.InvitedBy, invite.TokenHash, invite.ExpiresAt, invite.Status,
		)
		return err
	})
}

// ListInvitesByTenant returns all invitations for a tenant.
func (s *SQLiteDB) ListInvitesByTenant(ctx context.Context, tenantID string) ([]core.Invitation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, email, role_id, COALESCE(invited_by,''), token_hash,
		        expires_at, accepted_at, status, created_at
		 FROM invitations WHERE tenant_id = ? ORDER BY created_at DESC`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []core.Invitation
	for rows.Next() {
		var inv core.Invitation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.RoleID,
			&inv.InvitedBy, &inv.TokenHash, &inv.ExpiresAt, &inv.AcceptedAt,
			&inv.Status, &inv.CreatedAt); err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}

// ListAllTenants returns all tenants with pagination (admin only).
func (s *SQLiteDB) ListAllTenants(ctx context.Context, limit, offset int) ([]core.Tenant, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, slug, avatar_url, plan_id, COALESCE(owner_id,''),
		        status, limits_json, metadata_json, created_at, updated_at
		 FROM tenants ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tenants []core.Tenant
	for rows.Next() {
		var t core.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.AvatarURL, &t.PlanID, &t.OwnerID,
			&t.Status, &t.LimitsJSON, &t.MetadataJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, t)
	}
	return tenants, total, rows.Err()
}
