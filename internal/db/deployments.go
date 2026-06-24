package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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

// CreateDeploymentAtomicVersion allocates the next version for d.AppID and
// inserts the deployment row in a single statement, so concurrent deploys
// cannot allocate the same version. The version is computed and the row
// written atomically via INSERT ... SELECT MAX(version)+1 ... RETURNING, which
// SQLite executes under a single write lock. d.Version is set on return.
func (s *SQLiteDB) CreateDeploymentAtomicVersion(ctx context.Context, d *core.Deployment) error {
	if d.ID == "" {
		d.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		return tx.QueryRowContext(ctx,
			`INSERT INTO deployments
			   (id, app_id, version, image, container_id, status, build_log, commit_sha, commit_message, triggered_by, strategy, started_at)
			 SELECT ?, ?, COALESCE(MAX(version), 0) + 1, ?, ?, ?, ?, ?, ?, ?, ?, ?
			 FROM deployments WHERE app_id = ?
			 RETURNING version`,
			d.ID, d.AppID, d.Image, d.ContainerID, d.Status, d.BuildLog,
			d.CommitSHA, d.CommitMessage, d.TriggeredBy, d.Strategy, d.StartedAt,
			d.AppID,
		).Scan(&d.Version)
	})
}

// UpdateDeployment persists a mutation to an existing deployment row.
// Tier 100: pre-Tier-100 the deploy pipeline only mutated deployment.Status
// in memory after a container started, so every row in the deployments
// table was eternally "deploying" regardless of actual state. This method
// writes the mutable fields (status, container_id, build_log, finished_at)
// back to disk so the UI and the restart-storm reclaim sweep can both
// trust what they read.
func (s *SQLiteDB) UpdateDeployment(ctx context.Context, d *core.Deployment) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE deployments
			 SET status = ?, container_id = ?, build_log = ?, finished_at = ?
			 WHERE id = ?`,
			d.Status, d.ContainerID, d.BuildLog, d.FinishedAt, d.ID,
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

// ListDeploymentsByStatus returns every deployment in the given status,
// newest first. Used by deploy.Module.Start to reclaim in-flight deployments
// left behind by a crashed master (Phase 3.1.2 restart storm).
func (s *SQLiteDB) ListDeploymentsByStatus(ctx context.Context, status string) ([]core.Deployment, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT id, app_id, version, image, container_id, status, commit_sha, commit_message,
		        triggered_by, strategy, started_at, finished_at, created_at
		 FROM deployments WHERE status = ? ORDER BY created_at DESC LIMIT 10000`, status,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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

// AtomicNextDeployVersion atomically allocates the next deployment version using
// database-level locking. This prevents race conditions where concurrent requests
// could allocate the same version number.
//
// SECURITY FIX (RACE-002): SQLite's default BEGIN is DEFERRED — it acquires no
// lock until the first write, so two concurrent readers can both see MAX(version)
// before either writes, and both allocate the same version. BEGIN IMMEDIATE acquires
// an exclusive write lock *before* the SELECT runs, serializing all callers.
func (s *SQLiteDB) AtomicNextDeployVersion(ctx context.Context, appID string) (int, error) {
	var nextVersion int
	err := s.Tx(ctx, func(tx *sql.Tx) error {
		// Acquire exclusive lock immediately — blocks all other readers/writers.
		if _, err := tx.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
			return fmt.Errorf("begin immediate: %w", err)
		}
		var maxVersion sql.NullInt64
		err := tx.QueryRowContext(ctx,
			`SELECT MAX(version) FROM deployments WHERE app_id = ?`, appID,
		).Scan(&maxVersion)
		if err != nil {
			return err
		}
		if !maxVersion.Valid {
			nextVersion = 1
		} else {
			nextVersion = int(maxVersion.Int64) + 1
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return nextVersion, nil
}

// GetLatestDeploymentsByAppIDs retrieves the latest deployment for each app in a single query.
// Returns a map from appID to its latest deployment (nil if no deployment found for that app).
func (s *SQLiteDB) GetLatestDeploymentsByAppIDs(ctx context.Context, appIDs []string) (map[string]*core.Deployment, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	// Use a subquery to get the latest deployment per app in a single query.
	// This avoids the N+1 problem where we'd otherwise call GetLatestDeployment per app.
	placeholders := strings.Repeat("?,", len(appIDs))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(
		`SELECT d.id, d.app_id, d.version, d.image, d.container_id, d.status,
		        d.commit_sha, d.commit_message, d.triggered_by, d.strategy,
		        d.started_at, d.finished_at, d.created_at
		 FROM deployments d
		 INNER JOIN (
		     SELECT app_id, MAX(version) as max_version
		     FROM deployments
		     WHERE app_id IN (%s)
		     GROUP BY app_id
		 ) latest ON d.app_id = latest.app_id AND d.version = latest.max_version`,
		placeholders,
	)
	args := make([]any, len(appIDs))
	for i, id := range appIDs {
		args[i] = id
	}
	rows, err := s.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]*core.Deployment)
	for rows.Next() {
		var d core.Deployment
		if err := rows.Scan(&d.ID, &d.AppID, &d.Version, &d.Image, &d.ContainerID, &d.Status,
			&d.CommitSHA, &d.CommitMessage, &d.TriggeredBy, &d.Strategy,
			&d.StartedAt, &d.FinishedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		result[d.AppID] = &d
	}
	return result, rows.Err()
}
