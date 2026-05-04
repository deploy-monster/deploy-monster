package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// nullIfEmpty converts an empty string to a typed nil so nullable FK
// columns get NULL instead of ""; "" is not a valid parent key and
// Postgres rejects it with a foreign_key_violation (Tier 101 fix).
// SQLite happened to tolerate the empty string due to looser FK
// semantics but we normalise on the write path to keep both backends
// behaviorally identical.
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// CreateSecret inserts a new secret metadata record.
func (s *SQLiteDB) CreateSecret(ctx context.Context, secret *core.Secret) error {
	if secret.ID == "" {
		secret.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO secrets ( id, tenant_id, project_id, app_id, name, type, description, scope, current_version )
             VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			secret.ID, nullIfEmpty(secret.TenantID), nullIfEmpty(secret.ProjectID), nullIfEmpty(secret.AppID),
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
	rows, err := s.QueryContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), COALESCE(project_id,''), COALESCE(app_id,''),
                name, type, description, scope, current_version, created_at, updated_at
         FROM secrets WHERE tenant_id = ? ORDER BY created_at DESC LIMIT 1000`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

// DeleteSecret deletes one tenant-owned secret. Secret versions are removed by
// the database's ON DELETE CASCADE constraint.
func (s *SQLiteDB) DeleteSecret(ctx context.Context, tenantID, secretID string) error {
	res, err := s.ExecContext(ctx,
		`DELETE FROM secrets WHERE id = ? AND tenant_id = ?`,
		secretID, tenantID,
	)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete secret rows affected: %w", err)
	}
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
}

// GetSecretByScopeAndName returns a secret by its scope and name.
func (s *SQLiteDB) GetSecretByScopeAndName(ctx context.Context, scope, name string) (*core.Secret, error) {
	var secret core.Secret
	err := s.QueryRowContext(ctx,
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

// ListAllSecretVersions returns every secret version row (for key rotation).
func (s *SQLiteDB) ListAllSecretVersions(ctx context.Context) ([]core.SecretVersion, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT id, secret_id, version, value_enc, created_by, created_at
         FROM secret_versions ORDER BY secret_id, version LIMIT 10000`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var versions []core.SecretVersion
	for rows.Next() {
		var v core.SecretVersion
		if err := rows.Scan(&v.ID, &v.SecretID, &v.Version, &v.ValueEnc, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// UpdateSecretVersionValue updates the encrypted value of a secret version (for key rotation).
func (s *SQLiteDB) UpdateSecretVersionValue(ctx context.Context, id, valueEnc string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE secret_versions SET value_enc = ? WHERE id = ?`, valueEnc, id,
		)
		return err
	})
}

// GetLatestSecretVersion returns the most recent version of a secret.
func (s *SQLiteDB) GetLatestSecretVersion(ctx context.Context, secretID string) (*core.SecretVersion, error) {
	var version core.SecretVersion
	err := s.QueryRowContext(ctx,
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
