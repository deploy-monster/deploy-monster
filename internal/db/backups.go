package db

import (
	"context"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateBackup inserts a new backup record.
func (s *SQLiteDB) CreateBackup(ctx context.Context, backup *core.Backup) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO backups (id, tenant_id, source_type, source_id, storage_target, file_path, size_bytes, encryption, status, scheduled, retention_days, started_at, completed_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		backup.ID, backup.TenantID, backup.SourceType, backup.SourceID, backup.StorageTarget,
		backup.FilePath, backup.SizeBytes, backup.Encryption, backup.Status, backup.Scheduled,
		backup.RetentionDays, nil, nil, time.Now().UTC())
	return err
}

// ListBackupsByTenant lists backups for a tenant with pagination.
func (s *SQLiteDB) ListBackupsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]core.Backup, int, error) {
	var total int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM backups WHERE tenant_id = ?`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 20
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, tenant_id, source_type, source_id, storage_target, file_path, size_bytes, encryption, status, scheduled, retention_days, started_at, completed_at, created_at
		 FROM backups
		 WHERE tenant_id = ?
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var backups []core.Backup
	for rows.Next() {
		var b core.Backup
		var startedAt, completedAt *time.Time
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.SourceType, &b.SourceID, &b.StorageTarget,
			&b.FilePath, &b.SizeBytes, &b.Encryption, &b.Status, &b.Scheduled,
			&b.RetentionDays, &startedAt, &completedAt, &b.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		b.StartedAt = startedAt
		b.CompletedAt = completedAt
		backups = append(backups, b)
	}
	return backups, total, rows.Err()
}

// UpdateBackupStatus updates a backup's status and size.
func (s *SQLiteDB) UpdateBackupStatus(ctx context.Context, id, status string, sizeBytes int64) error {
	var completedAt *time.Time
	if status == "completed" || status == "failed" {
		now := time.Now().UTC()
		completedAt = &now
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE backups SET status = ?, size_bytes = ?, completed_at = ? WHERE id = ?`,
		status, sizeBytes, completedAt, id)
	return err
}
