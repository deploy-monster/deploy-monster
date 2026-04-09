package db

import (
	"context"
	"database/sql"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateDeployment inserts a new deployment record.
func (s *SQLiteDB) CreateDeployment(ctx context.Context, d *core.Deployment) error {
	if d.ID == "" {
		d.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO deployments (id, app_id, version, image, container_id, status, build_log, commit_sha, commit_message, triggered_by, strategy, started_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			d.ID, d.AppID, d.Version, d.Image, d.ContainerID, d.Status, d.BuildLog,
			d.CommitSHA, d.CommitMessage, d.TriggeredBy, d.Strategy, d.StartedAt,
		)
		return err
	})
}

// GetLatestDeployment returns the most recent deployment for an app.
func (s *SQLiteDB) GetLatestDeployment(ctx context.Context, appID string) (*core.Deployment, error) {
	d := &core.Deployment{}
	err := s.QueryRowContext(ctx,
		`SELECT id, app_id, version, image, container_id, status, commit_sha, commit_message,
		        triggered_by, strategy, started_at, finished_at, created_at
		 FROM deployments WHERE app_id = ? ORDER BY version DESC LIMIT 1`, appID,
	).Scan(&d.ID, &d.AppID, &d.Version, &d.Image, &d.ContainerID, &d.Status,
		&d.CommitSHA, &d.CommitMessage, &d.TriggeredBy, &d.Strategy,
		&d.StartedAt, &d.FinishedAt, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return d, err
}

// ListDeploymentsByApp returns deployments for an app, newest first.
func (s *SQLiteDB) ListDeploymentsByApp(ctx context.Context, appID string, limit int) ([]core.Deployment, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT id, app_id, version, image, container_id, status, commit_sha, commit_message,
		        triggered_by, strategy, started_at, finished_at, created_at
		 FROM deployments WHERE app_id = ? ORDER BY version DESC LIMIT ?`,
		appID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []core.Deployment
	for rows.Next() {
		var d core.Deployment
		if err := rows.Scan(&d.ID, &d.AppID, &d.Version, &d.Image, &d.ContainerID, &d.Status,
			&d.CommitSHA, &d.CommitMessage, &d.TriggeredBy, &d.Strategy,
			&d.StartedAt, &d.FinishedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

// GetNextDeployVersion returns the next deployment version number for an app.
func (s *SQLiteDB) GetNextDeployVersion(ctx context.Context, appID string) (int, error) {
	var maxVersion sql.NullInt64
	err := s.QueryRowContext(ctx,
		`SELECT MAX(version) FROM deployments WHERE app_id = ?`, appID,
	).Scan(&maxVersion)
	if err != nil {
		return 1, err
	}
	if !maxVersion.Valid {
		return 1, nil
	}
	return int(maxVersion.Int64) + 1, nil
}
