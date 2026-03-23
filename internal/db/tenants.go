package db

import (
	"context"
	"database/sql"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateTenant inserts a new tenant.
func (s *SQLiteDB) CreateTenant(ctx context.Context, t *core.Tenant) error {
	if t.ID == "" {
		t.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tenants (id, name, slug, avatar_url, plan_id, owner_id, status, limits_json, metadata_json)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			t.ID, t.Name, t.Slug, t.AvatarURL, t.PlanID, t.OwnerID, t.Status, t.LimitsJSON, t.MetadataJSON,
		)
		return err
	})
}

// GetTenant retrieves a tenant by ID.
func (s *SQLiteDB) GetTenant(ctx context.Context, id string) (*core.Tenant, error) {
	t := &core.Tenant{}
	var ignore string // placeholder for reseller_id column
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, avatar_url, plan_id, COALESCE(owner_id,''), COALESCE(reseller_id,''),
		        status, limits_json, metadata_json, created_at, updated_at
		 FROM tenants WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.AvatarURL, &t.PlanID, &t.OwnerID, &ignore,
		&t.Status, &t.LimitsJSON, &t.MetadataJSON, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return t, err
}

// GetTenantBySlug retrieves a tenant by slug.
func (s *SQLiteDB) GetTenantBySlug(ctx context.Context, slug string) (*core.Tenant, error) {
	t := &core.Tenant{}
	var ignore string // placeholder for reseller_id column
	err := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, avatar_url, plan_id, COALESCE(owner_id,''), COALESCE(reseller_id,''),
		        status, limits_json, metadata_json, created_at, updated_at
		 FROM tenants WHERE slug = ?`, slug,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.AvatarURL, &t.PlanID, &t.OwnerID, &ignore,
		&t.Status, &t.LimitsJSON, &t.MetadataJSON, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return t, err
}

// UpdateTenant updates tenant fields.
func (s *SQLiteDB) UpdateTenant(ctx context.Context, t *core.Tenant) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE tenants SET name=?, slug=?, avatar_url=?, plan_id=?, status=?,
			 limits_json=?, metadata_json=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			t.Name, t.Slug, t.AvatarURL, t.PlanID, t.Status, t.LimitsJSON, t.MetadataJSON, t.ID,
		)
		return err
	})
}

// DeleteTenant removes a tenant by ID.
func (s *SQLiteDB) DeleteTenant(ctx context.Context, id string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM tenants WHERE id = ?`, id)
		return err
	})
}
