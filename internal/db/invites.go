package db

import (
	"context"
	"database/sql"
	"errors"
	"time"

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

// GetInviteByTokenHash returns the invitation carrying tokenHash, or
// core.ErrNotFound when no pending-or-accepted invitation matches.
func (s *SQLiteDB) GetInviteByTokenHash(ctx context.Context, tokenHash string) (*core.Invitation, error) {
	inv := &core.Invitation{}
	err := s.QueryRowContext(ctx,
		`SELECT id, tenant_id, email, role_id, COALESCE(invited_by,''), token_hash,
		        expires_at, accepted_at, status, created_at
		 FROM invitations WHERE token_hash = ? LIMIT 1`, tokenHash,
	).Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.RoleID,
		&inv.InvitedBy, &inv.TokenHash, &inv.ExpiresAt, &inv.AcceptedAt,
		&inv.Status, &inv.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return inv, nil
}

// AcceptInvite atomically transitions a pending invitation to accepted.
// It fails with core.ErrInvalidToken when the invitation is not pending
// (already used, or concurrently redeemed by another request).
func (s *SQLiteDB) AcceptInvite(ctx context.Context, id string) error {
	res, err := s.ExecContext(ctx,
		`UPDATE invitations SET status = 'accepted', accepted_at = ? WHERE id = ? AND status = 'pending'`,
		time.Now().UTC(), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return core.ErrInvalidToken
	}
	return nil
}

// ListInvitesByTenant returns all invitations for a tenant.
func (s *SQLiteDB) ListInvitesByTenant(ctx context.Context, tenantID string) ([]core.Invitation, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT id, tenant_id, email, role_id, COALESCE(invited_by,''), token_hash,
		        expires_at, accepted_at, status, created_at
		 FROM invitations WHERE tenant_id = ? ORDER BY created_at DESC LIMIT 1000`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
	if err := s.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.QueryContext(ctx,
		`SELECT id, name, slug, avatar_url, plan_id, COALESCE(owner_id,''),
		        status, limits_json, metadata_json, created_at, updated_at
		 FROM tenants ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

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
