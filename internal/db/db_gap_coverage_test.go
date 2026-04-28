package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// apps.go — GetAppByName
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_GetAppByName_Found(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	_ = createApp(t, db, tenantID, projID, "findme")

	got, err := db.GetAppByName(ctx, tenantID, "findme")
	if err != nil {
		t.Fatalf("GetAppByName: %v", err)
	}
	if got == nil || got.Name != "findme" {
		t.Errorf("GetAppByName returned %+v", got)
	}
}

func TestSQLite_GetAppByName_NotFound(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	_, err := db.GetAppByName(ctx, tenantID, "does-not-exist")
	if err != core.ErrNotFound {
		t.Errorf("GetAppByName error = %v, want ErrNotFound", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// domains.go — DeleteDomainsByApp
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_DeleteDomainsByApp(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "delete-domains-app")

	// Create 3 domains for this app
	for i, fqdn := range []string{"a.example.com", "b.example.com", "c.example.com"} {
		dom := &core.Domain{
			AppID:       app.ID,
			FQDN:        fqdn,
			Type:        "auto",
			DNSProvider: "cf",
		}
		if err := db.CreateDomain(ctx, dom); err != nil {
			t.Fatalf("CreateDomain[%d]: %v", i, err)
		}
	}

	// Delete all
	n, err := db.DeleteDomainsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("DeleteDomainsByApp: %v", err)
	}
	if n != 3 {
		t.Errorf("deleted count = %d, want 3", n)
	}

	// Verify no domains remain
	doms, err := db.ListDomainsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListDomainsByApp: %v", err)
	}
	if len(doms) != 0 {
		t.Errorf("expected 0 domains after delete, got %d", len(doms))
	}
}

func TestSQLite_DeleteDomainsByApp_NoDomains(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	n, err := db.DeleteDomainsByApp(ctx, "nonexistent-app")
	if err != nil {
		t.Fatalf("DeleteDomainsByApp: %v", err)
	}
	if n != 0 {
		t.Errorf("deleted count = %d, want 0", n)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// backups.go — CreateBackup / ListBackupsByTenant / UpdateBackupStatus
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_Backup_CRUD(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	// Create a backup
	backup := &core.Backup{
		ID:            "bkp-test-1",
		TenantID:      tenantID,
		SourceType:    "database",
		SourceID:      "src-1",
		StorageTarget: "local",
		FilePath:      "/tmp/bkp.tar",
		SizeBytes:     0,
		Encryption:    "aes-256-gcm",
		Status:        "pending",
		Scheduled:     false,
		RetentionDays: 7,
	}
	if err := db.CreateBackup(ctx, backup); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// List
	backups, total, err := db.ListBackupsByTenant(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListBackupsByTenant: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(backups) != 1 {
		t.Errorf("backups len = %d, want 1", len(backups))
	}
	if backups[0].ID != "bkp-test-1" {
		t.Errorf("backup ID = %q, want bkp-test-1", backups[0].ID)
	}

	// Update status → completed
	if err := db.UpdateBackupStatus(ctx, backup.ID, "completed", 1024); err != nil {
		t.Fatalf("UpdateBackupStatus completed: %v", err)
	}

	backups, _, err = db.ListBackupsByTenant(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListBackupsByTenant: %v", err)
	}
	if backups[0].Status != "completed" {
		t.Errorf("status = %q, want completed", backups[0].Status)
	}
	if backups[0].SizeBytes != 1024 {
		t.Errorf("size = %d, want 1024", backups[0].SizeBytes)
	}
	if backups[0].CompletedAt == nil {
		t.Error("CompletedAt should be set after completion")
	}

	// Update status → failed (also sets CompletedAt)
	backup2 := &core.Backup{
		ID:            "bkp-test-2",
		TenantID:      tenantID,
		SourceType:    "volume",
		SourceID:      "src-2",
		StorageTarget: "s3",
		Status:        "pending",
	}
	if err := db.CreateBackup(ctx, backup2); err != nil {
		t.Fatalf("CreateBackup 2: %v", err)
	}
	if err := db.UpdateBackupStatus(ctx, backup2.ID, "failed", 0); err != nil {
		t.Fatalf("UpdateBackupStatus failed: %v", err)
	}

	// Update status → running (should NOT set completed_at)
	backup3 := &core.Backup{
		ID:            "bkp-test-3",
		TenantID:      tenantID,
		SourceType:    "config",
		SourceID:      "src-3",
		StorageTarget: "local",
		Status:        "pending",
	}
	if err := db.CreateBackup(ctx, backup3); err != nil {
		t.Fatalf("CreateBackup 3: %v", err)
	}
	if err := db.UpdateBackupStatus(ctx, backup3.ID, "running", 0); err != nil {
		t.Fatalf("UpdateBackupStatus running: %v", err)
	}
}

func TestSQLite_ListBackupsByTenant_DefaultLimit(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	// Create a backup
	backup := &core.Backup{
		ID:         "bkp-default",
		TenantID:   tenantID,
		SourceType: "full",
		SourceID:   "s1",
		Status:     "pending",
	}
	if err := db.CreateBackup(ctx, backup); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Zero limit should default to 20
	backups, total, err := db.ListBackupsByTenant(ctx, tenantID, 0, 0)
	if err != nil {
		t.Fatalf("ListBackupsByTenant: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(backups) != 1 {
		t.Errorf("backups len = %d, want 1", len(backups))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// billing.go — CreateUsageRecord / ListUsageRecordsByTenant
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_UsageRecord_CRUD(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, projID := setupTenantAndProject(t, db)
	app := createApp(t, db, tenantID, projID, "usage-app")

	now := time.Now().UTC().Truncate(time.Hour)

	for i := 0; i < 3; i++ {
		rec := &core.UsageRecord{
			TenantID:   tenantID,
			AppID:      app.ID,
			MetricType: "cpu",
			Value:      float64(i * 10),
			HourBucket: now.Add(time.Duration(-i) * time.Hour),
		}
		if err := db.CreateUsageRecord(ctx, rec); err != nil {
			t.Fatalf("CreateUsageRecord[%d]: %v", i, err)
		}
	}

	records, total, err := db.ListUsageRecordsByTenant(ctx, tenantID, 10, 0)
	if err != nil {
		t.Fatalf("ListUsageRecordsByTenant: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(records) != 3 {
		t.Errorf("records len = %d, want 3", len(records))
	}
}

func TestSQLite_ListUsageRecordsByTenant_DefaultLimit(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	rec := &core.UsageRecord{
		TenantID:   tenantID,
		MetricType: "memory",
		Value:      42.0,
		HourBucket: time.Now().UTC(),
	}
	if err := db.CreateUsageRecord(ctx, rec); err != nil {
		t.Fatalf("CreateUsageRecord: %v", err)
	}

	// Zero limit should default to 20
	records, total, err := db.ListUsageRecordsByTenant(ctx, tenantID, 0, 0)
	if err != nil {
		t.Fatalf("ListUsageRecordsByTenant: %v", err)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1", total)
	}
	if len(records) != 1 {
		t.Errorf("records len = %d, want 1", len(records))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// secrets.go — ListAllSecretVersions / UpdateSecretVersionValue
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_ListAllSecretVersions(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	// Create 2 secrets, each with 2 versions
	for i, name := range []string{"API_KEY", "DB_PASS"} {
		secret := &core.Secret{
			TenantID: tenantID,
			Name:     name,
			Type:     "env",
			Scope:    "tenant",
		}
		if err := db.CreateSecret(ctx, secret); err != nil {
			t.Fatalf("CreateSecret[%d]: %v", i, err)
		}
		for j := 1; j <= 2; j++ {
			ver := &core.SecretVersion{
				SecretID:  secret.ID,
				Version:   j,
				ValueEnc:  "enc-value",
				CreatedBy: "user-1",
			}
			if err := db.CreateSecretVersion(ctx, ver); err != nil {
				t.Fatalf("CreateSecretVersion[%d][%d]: %v", i, j, err)
			}
		}
	}

	versions, err := db.ListAllSecretVersions(ctx)
	if err != nil {
		t.Fatalf("ListAllSecretVersions: %v", err)
	}
	if len(versions) != 4 {
		t.Errorf("len versions = %d, want 4", len(versions))
	}
}

func TestSQLite_UpdateSecretVersionValue(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tenantID, _ := setupTenantAndProject(t, db)

	secret := &core.Secret{
		TenantID: tenantID,
		Name:     "ROTATABLE",
		Type:     "env",
		Scope:    "tenant",
	}
	if err := db.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	ver := &core.SecretVersion{
		SecretID:  secret.ID,
		Version:   1,
		ValueEnc:  "old-enc",
		CreatedBy: "user-1",
	}
	if err := db.CreateSecretVersion(ctx, ver); err != nil {
		t.Fatalf("CreateSecretVersion: %v", err)
	}

	if err := db.UpdateSecretVersionValue(ctx, ver.ID, "new-enc"); err != nil {
		t.Fatalf("UpdateSecretVersionValue: %v", err)
	}

	got, err := db.GetLatestSecretVersion(ctx, secret.ID)
	if err != nil {
		t.Fatalf("GetLatestSecretVersion: %v", err)
	}
	if got.ValueEnc != "new-enc" {
		t.Errorf("ValueEnc = %q, want new-enc", got.ValueEnc)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// sqlite.go — Checkpoint / SnapshotBackup
// ═══════════════════════════════════════════════════════════════════════════════

func TestSQLite_Checkpoint(t *testing.T) {
	db := testDB(t)

	// Do a write first to ensure WAL has content
	ctx := context.Background()
	tenant := &core.Tenant{Name: "Chk", Slug: "chk-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	if err := db.Checkpoint(); err != nil {
		t.Fatalf("Checkpoint: %v", err)
	}
}

func TestSQLite_SnapshotBackup(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Add some data
	tenant := &core.Tenant{Name: "Snap", Slug: "snap-" + core.GenerateID()[:8], Status: "active", PlanID: "free"}
	if err := db.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	destDir := t.TempDir()
	destPath := filepath.Join(destDir, "snapshot.db")

	if err := db.SnapshotBackup(ctx, destPath); err != nil {
		t.Fatalf("SnapshotBackup: %v", err)
	}

	// Verify we can open the snapshot
	restored, err := NewSQLite(destPath)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer restored.Close()

	got, err := restored.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant on restored: %v", err)
	}
	if got.Name != "Snap" {
		t.Errorf("restored tenant name = %q", got.Name)
	}
}

func TestSQLite_SnapshotBackup_InvalidPath(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	// Parent directory does not exist — VACUUM INTO should fail
	badPath := filepath.Join(t.TempDir(), "missing_subdir", "snap.db")
	err := db.SnapshotBackup(ctx, badPath)
	if err == nil {
		t.Fatal("expected error for invalid dest path")
	}
}

func TestSQLite_SnapshotBackup_ClosedDB(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	db.Close()

	err = db.SnapshotBackup(context.Background(), filepath.Join(t.TempDir(), "snap.db"))
	if err == nil {
		t.Fatal("expected error on closed DB")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// bolt.go — BatchSet
// ═══════════════════════════════════════════════════════════════════════════════

func TestBolt_BatchSet_Success(t *testing.T) {
	bs := testBolt(t)

	// Pick buckets that exist (see bolt.go initialization)
	items := []core.BoltBatchItem{
		{Bucket: "sessions", Key: "s1", Value: "session-1-data", TTL: 0},
		{Bucket: "sessions", Key: "s2", Value: map[string]string{"k": "v"}, TTL: 0},
		{Bucket: "buildcache", Key: "c1", Value: 42, TTL: 60},
	}

	if err := bs.BatchSet(items); err != nil {
		t.Fatalf("BatchSet: %v", err)
	}

	// Verify each item was written
	var s1 string
	if err := bs.Get("sessions", "s1", &s1); err != nil {
		t.Errorf("Get s1: %v", err)
	}
	if s1 != "session-1-data" {
		t.Errorf("s1 = %q", s1)
	}
}

func TestBolt_BatchSet_Empty(t *testing.T) {
	bs := testBolt(t)
	if err := bs.BatchSet(nil); err != nil {
		t.Errorf("BatchSet(nil) = %v, want nil", err)
	}
	if err := bs.BatchSet([]core.BoltBatchItem{}); err != nil {
		t.Errorf("BatchSet([]) = %v, want nil", err)
	}
}

func TestBolt_BatchSet_UnknownBucket(t *testing.T) {
	bs := testBolt(t)
	items := []core.BoltBatchItem{
		{Bucket: "nonexistent_bucket", Key: "k", Value: "v"},
	}
	err := bs.BatchSet(items)
	if err == nil {
		t.Error("BatchSet with unknown bucket should fail")
	}
}
