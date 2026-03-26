package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateSecret inserts a new secret metadata record.
func (s *SQLiteDB) CreateSecret(ctx context.Context, secret *core.Secret) error {
	if secret.ID == "" {
		secret.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO secrets ( id, tenant_id, project_id, app_id, name, type, description, scope, current_version )
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			secret.ID, secret.TenantID, secret.ProjectID, secret.AppID,
			secret.Name, secret.Type, secret.Description, secret.Scope, secret.CurrentVersion,
		)
		return err
	})
}

// CreateSecretVersion inserts a new encrypted secret version.
func (s *SQLiteDB) CreateSecretVersion(ctx context.Context, version *core.SecretVersion) error {
	if version.ID == "" {
		version.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO secret_versions (id, secret_id, version, value_enc, created_by)
             VALUES (?, ?, ?, ?, ?)`,
			version.ID, version.SecretID, version.Version, version.ValueEnc, version.CreatedBy,
		)
		return err
	})
}

// ListSecretsByTenant returns all secret metadata for a tenant (no values).
func (s *SQLiteDB) ListSecretsByTenant(ctx context.Context, tenantID string) ([]core.Secret, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), COALESCE(project_id,''), COALESCE(app_id,''),
                name, type, description, scope, current_version, created_at, updated_at
         FROM secrets WHERE tenant_id = ? ORDER BY created_at DESC`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []core.Secret
	for rows.Next() {
		var s core.Secret
		if err := rows.Scan(&s.ID, &s.TenantID, &s.ProjectID, &s.AppID,
			&s.Name, &s.Type, &s.Description, &s.Scope, &s.CurrentVersion,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

// GetSecretByScopeAndName returns a secret by its scope and name.
func (s *SQLiteDB) GetSecretByScopeAndName(ctx context.Context, scope, name string) (*core.Secret, error) {
	var secret core.Secret
	err := s.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), COALESCE(project_id,''), COALESCE(app_id,''),
                name, type, description, scope, current_version, created_at, updated_at
         FROM secrets WHERE scope = ? AND name = ?`, scope, name,
	).Scan(&secret.ID, &secret.TenantID, &secret.ProjectID, &secret.AppID,
		&secret.Name, &secret.Type, &secret.Description, &secret.Scope, &secret.CurrentVersion,
		&secret.CreatedAt, &secret.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("secret %s/%s not found", scope, name)
	}
	if err != nil {
		return nil, err
	}
	return &secret, nil
}

// GetLatestSecretVersion returns the most recent version of a secret.
func (s *SQLiteDB) GetLatestSecretVersion(ctx context.Context, secretID string) (*core.SecretVersion, error) {
	var version core.SecretVersion
	err := s.db.QueryRowContext(ctx,
		`SELECT id, secret_id, version, value_enc, created_by, created_at
         FROM secret_versions WHERE secret_id = ? ORDER BY version DESC LIMIT 1`, secretID,
	).Scan(&version.ID, &version.SecretID, &version.Version, &version.ValueEnc,
		&version.CreatedBy, &version.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no versions found for secret %s", secretID)
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}
