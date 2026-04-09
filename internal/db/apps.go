package db

import (
	"context"
	"database/sql"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateApp inserts a new application.
func (s *SQLiteDB) CreateApp(ctx context.Context, a *core.Application) error {
	if a.ID == "" {
		a.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO applications (id, project_id, tenant_id, name, type, source_type, source_url, branch, dockerfile, build_pack, env_vars_enc, labels_json, replicas, status, server_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			a.ID, a.ProjectID, a.TenantID, a.Name, a.Type, a.SourceType, a.SourceURL, a.Branch,
			a.Dockerfile, a.BuildPack, a.EnvVarsEnc, a.LabelsJSON, a.Replicas, a.Status, a.ServerID,
		)
		return err
	})
}

// GetApp retrieves an application by ID.
func (s *SQLiteDB) GetApp(ctx context.Context, id string) (*core.Application, error) {
	a := &core.Application{}
	err := s.QueryRowContext(ctx,
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        dockerfile, build_pack, env_vars_enc, labels_json, replicas, status, COALESCE(server_id,''),
		        created_at, updated_at
		 FROM applications WHERE id = ?`, id,
	).Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType, &a.SourceURL, &a.Branch,
		&a.Dockerfile, &a.BuildPack, &a.EnvVarsEnc, &a.LabelsJSON, &a.Replicas, &a.Status, &a.ServerID,
		&a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return a, err
}

// ListAppsByTenant returns all applications for a tenant.
func (s *SQLiteDB) ListAppsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]core.Application, int, error) {
	var total int
	err := s.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM applications WHERE tenant_id = ?`, tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.QueryContext(ctx,
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        status, replicas, created_at, updated_at
		 FROM applications WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var apps []core.Application
	for rows.Next() {
		var a core.Application
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType,
			&a.SourceURL, &a.Branch, &a.Status, &a.Replicas, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		apps = append(apps, a)
	}
	return apps, total, rows.Err()
}

// ListAppsByProject returns all applications in a project.
func (s *SQLiteDB) ListAppsByProject(ctx context.Context, projectID string) ([]core.Application, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        status, replicas, created_at, updated_at
		 FROM applications WHERE project_id = ? ORDER BY name`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []core.Application
	for rows.Next() {
		var a core.Application
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType,
			&a.SourceURL, &a.Branch, &a.Status, &a.Replicas, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

// UpdateApp updates all mutable application fields.
func (s *SQLiteDB) UpdateApp(ctx context.Context, a *core.Application) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE applications SET name=?, source_url=?, branch=?, dockerfile=?,
			 env_vars_enc=?, labels_json=?, replicas=?, status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			a.Name, a.SourceURL, a.Branch, a.Dockerfile,
			a.EnvVarsEnc, a.LabelsJSON, a.Replicas, a.Status, a.ID,
		)
		return err
	})
}

// UpdateAppStatus updates an application's status.
func (s *SQLiteDB) UpdateAppStatus(ctx context.Context, id, status string) error {
	_, err := s.ExecContext(ctx,
		`UPDATE applications SET status=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		status, id,
	)
	return err
}

// DeleteApp removes an application by ID.
func (s *SQLiteDB) DeleteApp(ctx context.Context, id string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM applications WHERE id = ?`, id)
		return err
	})
}
