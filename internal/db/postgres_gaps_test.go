package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Phase 4 coverage lift for internal/db — fills the 17 zero-coverage
// PostgresDB methods plus the transaction-rollback paths in
// AtomicNextDeployVersion that the roadmap called out. Every test uses
// go-sqlmock (no live Postgres required). Error branches are exercised
// alongside success branches so a regression that forgets to return an
// error on a Query/Scan failure fails the test.

func TestPostgresDB_DB_ReturnsHandle(t *testing.T) {
	pg, _ := newMockPostgres(t)
	if pg.DB() == nil {
		t.Fatal("DB() returned nil")
	}
}

// ----- GetAppByName -----

func TestPostgresDB_GetAppByName_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM applications WHERE tenant_id = \\$1 AND name = \\$2").
		WithArgs("t1", "myapp").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "tenant_id", "name", "type", "source_type", "source_url", "branch",
			"dockerfile", "build_pack", "env_vars_enc", "labels_json", "replicas", "status", "server_id",
			"created_at", "updated_at",
		}).AddRow("a1", "p1", "t1", "myapp", "web", "git", "https://x", "main",
			"Dockerfile", "", "", "{}", 1, "running", "", now, now))
	app, err := pg.GetAppByName(context.Background(), "t1", "myapp")
	if err != nil || app == nil || app.Name != "myapp" {
		t.Fatalf("GetAppByName: err=%v app=%+v", err, app)
	}
}

func TestPostgresDB_GetAppByName_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM applications").
		WithArgs("t1", "nope").
		WillReturnError(sql.ErrNoRows)
	_, err := pg.GetAppByName(context.Background(), "t1", "nope")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ----- UpdateDeployment -----

func TestPostgresDB_UpdateDeployment_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	d := &core.Deployment{ID: "d1", Status: "success", ContainerID: "c1", BuildLog: "log", FinishedAt: &now}
	mock.ExpectExec("UPDATE deployments SET status").
		WithArgs("success", "c1", "log", &now, "d1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.UpdateDeployment(context.Background(), d); err != nil {
		t.Fatalf("UpdateDeployment: %v", err)
	}
}

func TestPostgresDB_UpdateDeployment_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE deployments SET status").
		WillReturnError(errors.New("update failed"))
	if err := pg.UpdateDeployment(context.Background(), &core.Deployment{ID: "d1"}); err == nil {
		t.Error("expected error")
	}
}

// ----- ListDeploymentsByStatus -----

func TestPostgresDB_ListDeploymentsByStatus_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE status = \\$1").
		WithArgs("running").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "app_id", "version", "image", "container_id", "status", "commit_sha",
			"commit_message", "triggered_by", "strategy", "started_at", "finished_at", "created_at",
		}).AddRow("d1", "a1", 1, "img", "cid", "running", "", "", "", "", now, nil, now))
	out, err := pg.ListDeploymentsByStatus(context.Background(), "running")
	if err != nil || len(out) != 1 || out[0].ID != "d1" {
		t.Fatalf("ListDeploymentsByStatus: err=%v out=%+v", err, out)
	}
}

func TestPostgresDB_ListDeploymentsByStatus_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM deployments WHERE status = \\$1").
		WillReturnError(errors.New("query failed"))
	if _, err := pg.ListDeploymentsByStatus(context.Background(), "x"); err == nil {
		t.Error("expected error")
	}
}

// ----- AtomicNextDeployVersion (transaction rollback paths) -----
//
// This is the one method the roadmap explicitly flagged as
// transaction-rollback-undertested. Each branch must be covered because
// a silent failure here yields duplicate deployment versions under
// concurrent writes — RACE-002 was a real incident.

func TestPostgresDB_AtomicNextDeployVersion_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT MAX\\(version\\) FROM deployments").
		WithArgs("app-1").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(5))
	mock.ExpectCommit()
	v, err := pg.AtomicNextDeployVersion(context.Background(), "app-1")
	if err != nil || v != 6 {
		t.Fatalf("AtomicNextDeployVersion: v=%d err=%v", v, err)
	}
}

func TestPostgresDB_AtomicNextDeployVersion_FirstDeploy(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT MAX\\(version\\) FROM deployments").
		WithArgs("app-new").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(nil)) // NULL → .Valid=false
	mock.ExpectCommit()
	v, err := pg.AtomicNextDeployVersion(context.Background(), "app-new")
	if err != nil || v != 1 {
		t.Fatalf("expected v=1 on null max, got v=%d err=%v", v, err)
	}
}

func TestPostgresDB_AtomicNextDeployVersion_BeginTxError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
	if _, err := pg.AtomicNextDeployVersion(context.Background(), "app-1"); err == nil {
		t.Error("expected error on BeginTx failure")
	}
}

func TestPostgresDB_AtomicNextDeployVersion_LockError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnError(errors.New("lock failed"))
	mock.ExpectRollback()
	if _, err := pg.AtomicNextDeployVersion(context.Background(), "app-1"); err == nil {
		t.Error("expected error on lock failure — deferred Rollback should trigger")
	}
}

func TestPostgresDB_AtomicNextDeployVersion_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT MAX\\(version\\)").WillReturnError(errors.New("scan failed"))
	mock.ExpectRollback()
	if _, err := pg.AtomicNextDeployVersion(context.Background(), "app-1"); err == nil {
		t.Error("expected error on scan failure — deferred Rollback should trigger")
	}
}

func TestPostgresDB_AtomicNextDeployVersion_CommitError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT MAX\\(version\\)").
		WillReturnRows(sqlmock.NewRows([]string{"max"}).AddRow(0))
	mock.ExpectCommit().WillReturnError(errors.New("commit failed"))
	if _, err := pg.AtomicNextDeployVersion(context.Background(), "app-1"); err == nil {
		t.Error("expected error on commit failure")
	}
}

func TestHashAppIDToLockID_Stable(t *testing.T) {
	// Pure function — verify stability (same input ⇒ same output) and
	// positivity (lock IDs must fit in signed int64, the Postgres
	// pg_advisory_lock signature).
	a := hashAppIDToLockID("app-1")
	b := hashAppIDToLockID("app-1")
	if a != b {
		t.Errorf("hash not stable: %d vs %d", a, b)
	}
	if hashAppIDToLockID("") != 0 {
		t.Errorf("empty-string hash not 0")
	}
	// Positivity invariant — the 0x7FFF... mask guarantees this.
	c := hashAppIDToLockID("very-long-app-id-that-would-overflow-int64-if-mask-were-missing")
	if c < 0 {
		t.Errorf("lock ID went negative: %d", c)
	}
}

// ----- GetDomain -----

func TestPostgresDB_GetDomain_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM domains WHERE id = \\$1").
		WithArgs("d1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "app_id", "fqdn", "type", "dns_provider", "dns_synced", "verified", "created_at"}).
			AddRow("d1", "a1", "example.com", "a", "cf", true, true, time.Now()))
	out, err := pg.GetDomain(context.Background(), "d1")
	if err != nil || out.FQDN != "example.com" {
		t.Fatalf("GetDomain: err=%v out=%+v", err, out)
	}
}

func TestPostgresDB_GetDomain_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM domains").WillReturnError(sql.ErrNoRows)
	if _, err := pg.GetDomain(context.Background(), "nope"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

// ----- DeleteDomainsByApp -----

func TestPostgresDB_DeleteDomainsByApp_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM domains WHERE app_id = \\$1").
		WithArgs("a1").
		WillReturnResult(sqlmock.NewResult(0, 3))
	n, err := pg.DeleteDomainsByApp(context.Background(), "a1")
	if err != nil || n != 3 {
		t.Errorf("DeleteDomainsByApp: n=%d err=%v", n, err)
	}
}

func TestPostgresDB_DeleteDomainsByApp_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM domains").WillReturnError(errors.New("delete failed"))
	if _, err := pg.DeleteDomainsByApp(context.Background(), "a1"); err == nil {
		t.Error("expected error")
	}
}

// ----- Secret lookup methods -----

func TestPostgresDB_GetSecretByScopeAndName_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM secrets WHERE scope = \\$1 AND name = \\$2").
		WithArgs("global", "db-pass").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "project_id", "app_id", "name", "type",
			"description", "scope", "current_version", "created_at", "updated_at",
		}).AddRow("s1", "", "", "", "db-pass", "password", "", "global", 1, now, now))
	out, err := pg.GetSecretByScopeAndName(context.Background(), "global", "db-pass")
	if err != nil || out.ID != "s1" {
		t.Fatalf("GetSecretByScopeAndName: err=%v out=%+v", err, out)
	}
}

func TestPostgresDB_GetSecretByScopeAndName_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM secrets").WillReturnError(sql.ErrNoRows)
	if _, err := pg.GetSecretByScopeAndName(context.Background(), "global", "x"); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_GetLatestSecretVersion_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM secret_versions WHERE secret_id = \\$1").
		WithArgs("s1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "secret_id", "version", "value_enc", "created_by", "created_at"}).
			AddRow("v1", "s1", 1, "enc", "u1", now))
	out, err := pg.GetLatestSecretVersion(context.Background(), "s1")
	if err != nil || out.Version != 1 {
		t.Fatalf("GetLatestSecretVersion: err=%v out=%+v", err, out)
	}
}

func TestPostgresDB_GetLatestSecretVersion_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM secret_versions").WillReturnError(sql.ErrNoRows)
	if _, err := pg.GetLatestSecretVersion(context.Background(), "s1"); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_ListAllSecretVersions_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM secret_versions ORDER BY secret_id").
		WillReturnRows(sqlmock.NewRows([]string{"id", "secret_id", "version", "value_enc", "created_by", "created_at"}).
			AddRow("v1", "s1", 1, "enc1", "u1", now).
			AddRow("v2", "s1", 2, "enc2", "u1", now))
	out, err := pg.ListAllSecretVersions(context.Background())
	if err != nil || len(out) != 2 {
		t.Fatalf("ListAllSecretVersions: err=%v len=%d", err, len(out))
	}
}

func TestPostgresDB_ListAllSecretVersions_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM secret_versions").WillReturnError(errors.New("query failed"))
	if _, err := pg.ListAllSecretVersions(context.Background()); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_UpdateSecretVersionValue_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE secret_versions SET value_enc = \\$1 WHERE id = \\$2").
		WithArgs("new-enc", "v1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.UpdateSecretVersionValue(context.Background(), "v1", "new-enc"); err != nil {
		t.Errorf("UpdateSecretVersionValue: %v", err)
	}
}

// ----- Usage records -----

func TestPostgresDB_CreateUsageRecord_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	rec := &core.UsageRecord{TenantID: "t1", AppID: "a1", MetricType: "cpu", Value: 1.5, HourBucket: time.Now()}
	mock.ExpectExec("INSERT INTO usage_records").
		WithArgs(rec.TenantID, rec.AppID, rec.MetricType, rec.Value, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := pg.CreateUsageRecord(context.Background(), rec); err != nil {
		t.Errorf("CreateUsageRecord: %v", err)
	}
}

func TestPostgresDB_ListUsageRecordsByTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_records WHERE tenant_id").
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .+ FROM usage_records").
		WithArgs("t1", 20, 0).
		WillReturnRows(sqlmock.NewRows([]string{"id", "tenant_id", "app_id", "metric_type", "value", "hour_bucket", "created_at"}).
			AddRow(int64(1), "t1", "a1", "cpu", 1.5, now, now))
	out, total, err := pg.ListUsageRecordsByTenant(context.Background(), "t1", 0, 0)
	if err != nil || total != 1 || len(out) != 1 {
		t.Fatalf("ListUsageRecordsByTenant: err=%v total=%d len=%d", err, total, len(out))
	}
}

func TestPostgresDB_ListUsageRecordsByTenant_CountError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_records").WillReturnError(errors.New("count failed"))
	if _, _, err := pg.ListUsageRecordsByTenant(context.Background(), "t1", 20, 0); err == nil {
		t.Error("expected error")
	}
}

// ----- Backups -----

func TestPostgresDB_CreateBackup_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	b := &core.Backup{ID: "b1", TenantID: "t1", SourceType: "app", SourceID: "a1", StorageTarget: "s3", FilePath: "/x", Status: "pending"}
	mock.ExpectExec("INSERT INTO backups").
		WithArgs(b.ID, b.TenantID, b.SourceType, b.SourceID, b.StorageTarget,
			b.FilePath, b.SizeBytes, b.Encryption, b.Status, b.Scheduled,
			b.RetentionDays, nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := pg.CreateBackup(context.Background(), b); err != nil {
		t.Errorf("CreateBackup: %v", err)
	}
}

func TestPostgresDB_ListBackupsByTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM backups WHERE tenant_id").
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .+ FROM backups").
		WithArgs("t1", 5, 0).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "source_type", "source_id", "storage_target", "file_path",
			"size_bytes", "encryption", "status", "scheduled", "retention_days",
			"started_at", "completed_at", "created_at",
		}).AddRow("b1", "t1", "app", "a1", "s3", "/x", 0, "none", "completed", false, 30, &now, &now, now))
	out, total, err := pg.ListBackupsByTenant(context.Background(), "t1", 5, 0)
	if err != nil || total != 1 || len(out) != 1 {
		t.Fatalf("ListBackupsByTenant: err=%v total=%d len=%d", err, total, len(out))
	}
}

func TestPostgresDB_ListBackupsByTenant_CountError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM backups").WillReturnError(errors.New("count failed"))
	if _, _, err := pg.ListBackupsByTenant(context.Background(), "t1", 0, 0); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_UpdateBackupStatus_Completed(t *testing.T) {
	pg, mock := newMockPostgres(t)
	// status=completed sets completed_at to time.Now().UTC()
	mock.ExpectExec("UPDATE backups SET status = \\$1, size_bytes = \\$2, completed_at = \\$3 WHERE id = \\$4").
		WithArgs("completed", int64(1024), sqlmock.AnyArg(), "b1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.UpdateBackupStatus(context.Background(), "b1", "completed", 1024); err != nil {
		t.Errorf("UpdateBackupStatus: %v", err)
	}
}

func TestPostgresDB_UpdateBackupStatus_Running(t *testing.T) {
	pg, mock := newMockPostgres(t)
	// status != completed/failed leaves completed_at as nil
	mock.ExpectExec("UPDATE backups SET status").
		WithArgs("running", int64(0), nil, "b1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.UpdateBackupStatus(context.Background(), "b1", "running", 0); err != nil {
		t.Errorf("UpdateBackupStatus: %v", err)
	}
}
