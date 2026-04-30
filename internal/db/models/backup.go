package models

import "time"

// Backup represents a backup of an app's data.
type Backup struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	TenantID    string    `json:"tenant_id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Type        string    `json:"type"` // full, incremental, database
	Status      string    `json:"status"` // pending, running, completed, failed
	SizeBytes   int64     `json:"size_bytes"`
	StorageType string    `json:"storage_type"` // local, s3, sftp
	StoragePath string    `json:"storage_path"` // bucket/key or path
	Compressed  bool      `json:"compressed"`
	Encrypted   bool      `json:"encrypted"`
	ErrorMsg    string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// BackupSchedule defines when backups run automatically.
type BackupSchedule struct {
	ID            string    `json:"id"`
	AppID         string    `json:"app_id"`
	TenantID      string    `json:"tenant_id"`
	CronExpression string   `json:"cron_expression"` // standard cron
	RetentionDays int       `json:"retention_days"`   // how long to keep
	BackupType    string    `json:"backup_type"`     // full, incremental, database
	StorageType   string    `json:"storage_type"`   // local, s3, sftp
	StorageConfig string    `json:"storage_config"`  // JSON: bucket, region, credentials
	Enabled       bool      `json:"enabled"`
	LastRunAt     *time.Time `json:"last_run_at,omitempty"`
	NextRunAt     *time.Time `json:"next_run_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// BackupTarget defines what to backup and how.
type BackupTarget struct {
	Type        string            `json:"type"` // volume, database, files
	SourcePath  string            `json:"source_path"`
	ExcludePatterns []string      `json:"exclude_patterns,omitempty"`
	Destination string            `json:"destination"`
}

// RestorePoint represents a point-in-time restore option.
type RestorePoint struct {
	BackupID   string    `json:"backup_id"`
	Timestamp  time.Time `json:"timestamp"`
	AppID      string    `json:"app_id"`
	Name       string    `json:"name"`
	SizeBytes  int64     `json:"size_bytes"`
	Status     string    `json:"status"` // available, restoring, failed
}