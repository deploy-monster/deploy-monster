package db

import (
	"context"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateUsageRecord inserts a new usage record.
func (s *SQLiteDB) CreateUsageRecord(ctx context.Context, record *core.UsageRecord) error {
	_, err := s.ExecContext(ctx,
		`INSERT INTO usage_records (tenant_id, app_id, metric_type, value, hour_bucket, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		record.TenantID, record.AppID, record.MetricType, record.Value, record.HourBucket.UTC(), time.Now().UTC())
	return err
}

// ListUsageRecordsByTenant lists usage records for a tenant with pagination.
func (s *SQLiteDB) ListUsageRecordsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]core.UsageRecord, int, error) {
	var total int
	if err := s.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM usage_records WHERE tenant_id = ?`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}

	if limit <= 0 {
		limit = 20
	}

	rows, err := s.QueryContext(ctx,
		`SELECT id, tenant_id, app_id, metric_type, value, hour_bucket, created_at
		 FROM usage_records
		 WHERE tenant_id = ?
		 ORDER BY hour_bucket DESC
		 LIMIT ? OFFSET ?`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []core.UsageRecord
	for rows.Next() {
		var r core.UsageRecord
		if err := rows.Scan(&r.ID, &r.TenantID, &r.AppID, &r.MetricType, &r.Value, &r.HourBucket, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		records = append(records, r)
	}
	return records, total, rows.Err()
}
