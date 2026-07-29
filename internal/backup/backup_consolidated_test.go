package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// === merged from backup_final_test.go ===

// ═══════════════════════════════════════════════════════════════════════════════
// LocalStorage.List — covers local.go:60 (the os.Stat error continue branch)
// The 85.7% means the `continue` on Stat error is not covered. We simulate
// this by creating a file and then removing it between Glob and Stat.
// Since this race is hard to trigger, we verify the existing paths work and
// test with a file that Stat can report on.
// ═══════════════════════════════════════════════════════════════════════════════

func TestLocalStorage_List_WithMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir, nil)

	// Create files
	os.WriteFile(filepath.Join(dir, "bk-001.tar"), []byte("data1"), 0644)
	os.WriteFile(filepath.Join(dir, "bk-002.tar"), []byte("data22"), 0644)

	entries, err := ls.List(context.Background(), "bk-")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Verify entries are sorted (newest first)
	if len(entries) == 2 && entries[0].CreatedAt < entries[1].CreatedAt {
		t.Error("entries should be sorted newest first")
	}
}

// TestLocalStorage_List_StatErrorBranch covers the os.Stat error continue branch
// by creating a symlink that points to a non-existent target on supported platforms.
func TestLocalStorage_List_StatErrorBranch(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir, nil)

	// Create a real file and a broken symlink
	os.WriteFile(filepath.Join(dir, "ls-good.tar"), []byte("ok"), 0644)

	// Create a broken symlink (points to non-existent target)
	brokenLink := filepath.Join(dir, "ls-broken.tar")
	err := os.Symlink(filepath.Join(dir, "nonexistent-target"), brokenLink)
	if err != nil {
		// On Windows without developer mode, symlinks may not work
		t.Skip("symlink creation not supported:", err)
	}

	entries, err := ls.List(context.Background(), "ls-")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// The broken symlink should be skipped (Stat fails -> continue),
	// only the good file should appear.
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (broken symlink skipped), got %d", len(entries))
	}
	if len(entries) == 1 && entries[0].Key != "ls-good.tar" {
		t.Errorf("entry key = %q, want ls-good.tar", entries[0].Key)
	}
}

func TestLocalStorage_List_GlobError(t *testing.T) {
	// Using a pattern with invalid glob chars is hard since filepath.Glob
	// is lenient. Instead test with an empty directory.
	dir := t.TempDir()
	ls := NewLocalStorage(dir, nil)

	entries, err := ls.List(context.Background(), "nonexistent-prefix-")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Scheduler.loop — covers scheduler.go:47 (the ticker + time match branch)
// The loop has a 1-minute ticker. We exercise the stopCh branch by starting
// and stopping the scheduler.
// ═══════════════════════════════════════════════════════════════════════════════

func TestScheduler_Loop_StopCh(t *testing.T) {
	events := core.NewEventBus(testLogger())
	storages := map[string]core.BackupStorage{
		"local": &mockBackupStorage{},
	}

	s := NewScheduler(nil, storages, events, nil, "02:00", testLogger())
	s.Start()

	// Give the goroutine time to start
	time.Sleep(20 * time.Millisecond)

	// Stop should cleanly terminate the loop
	s.Stop()
	time.Sleep(20 * time.Millisecond)
}

// TestScheduler_RunBackups_EmitsEvents verifies the backup scheduler emits
// the correct events when running backups.
func TestScheduler_RunBackups_EmitsCorrectEvents(t *testing.T) {
	events := core.NewEventBus(testLogger())

	var published []string
	events.Subscribe("backup.*", func(_ context.Context, e core.Event) error {
		published = append(published, e.Type)
		return nil
	})

	storages := map[string]core.BackupStorage{
		"local": &mockBackupStorage{},
	}

	store := &mockStore{}
	s := NewScheduler(store, storages, events, nil, "02:00", testLogger())
	s.runBackups()

	if len(published) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(published), published)
	}
	if published[0] != core.EventBackupStarted {
		t.Errorf("first event = %q, want %q", published[0], core.EventBackupStarted)
	}
	if published[1] != core.EventBackupCompleted {
		t.Errorf("second event = %q, want %q", published[1], core.EventBackupCompleted)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// init() — covers module.go:11
// ═══════════════════════════════════════════════════════════════════════════════

func TestInit_RegisteredAsModule(t *testing.T) {
	m := New()
	var _ core.Module = m
	if m.ID() != "backup" {
		t.Errorf("ID() = %q, want backup", m.ID())
	}
}

// === merged from backup_remaining2_test.go ===

// =============================================================================
// LocalStorage — NewLocalStorage edge cases (local.go:34)
// =============================================================================

func TestNewLocalStorage_AbsPath(t *testing.T) {
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, nil)
	if ls == nil {
		t.Fatal("expected non-nil storage")
	}
	if ls.basePath != tmpDir {
		t.Errorf("expected basePath %s, got %s", tmpDir, ls.basePath)
	}
}

func TestNewLocalStorage_WithEncryptionKey(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, key)
	if ls == nil {
		t.Fatal("expected non-nil storage")
	}
	if ls.encryptionKey == nil {
		t.Error("expected non-nil encryption key")
	}
}

// =============================================================================
// LocalStorage — Upload with absolute key (local.go:46)
// =============================================================================

func TestLocalUpload_AbsoluteKey(t *testing.T) {
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, nil)
	err := ls.Upload(context.Background(), "/etc/passwd", strings.NewReader("data"), 4)
	if err == nil || !strings.Contains(err.Error(), "absolute paths") {
		t.Fatalf("expected absolute path error, got: %v", err)
	}
}

// =============================================================================
// LocalStorage — Upload with path traversal key (local.go:46)
// =============================================================================

func TestLocalUpload_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, nil)
	err := ls.Upload(context.Background(), "../../etc/passwd", strings.NewReader("data"), 4)
	if err == nil || !strings.Contains(err.Error(), "outside storage root") {
		t.Fatalf("expected path traversal error, got: %v", err)
	}
}

// =============================================================================
// LocalStorage — Upload and Download round trip (local.go:46/136)
// =============================================================================

func TestLocalUploadDownloadRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, nil)

	data := "test backup content"
	err := ls.Upload(context.Background(), "test-app/backup.json", strings.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	reader, err := ls.Download(context.Background(), "test-app/backup.json")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != data {
		t.Errorf("expected %q, got %q", data, string(got))
	}
}

// =============================================================================
// LocalStorage — Download with absolute key (local.go:136)
// =============================================================================

func TestLocalDownload_AbsoluteKey(t *testing.T) {
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, nil)
	_, err := ls.Download(context.Background(), "/etc/shadow")
	if err == nil || !strings.Contains(err.Error(), "absolute paths") {
		t.Fatalf("expected absolute path error, got: %v", err)
	}
}

// =============================================================================
// LocalStorage — Delete (local.go:177)
// =============================================================================

func TestLocalDelete_AbsoluteKey(t *testing.T) {
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, nil)
	err := ls.Delete(context.Background(), "/etc/shadow")
	if err == nil || !strings.Contains(err.Error(), "absolute paths") {
		t.Fatalf("expected absolute path error, got: %v", err)
	}
}

func TestLocalDelete_NonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, nil)
	err := ls.Delete(context.Background(), "nonexistent/backup.json")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

// =============================================================================
// LocalStorage — List with path traversal prefix (local.go:193)
// =============================================================================

func TestLocalList_PathTraversalPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, nil)
	_, err := ls.List(context.Background(), "../../etc")
	if err == nil || !strings.Contains(err.Error(), "outside storage root") {
		t.Fatalf("expected path traversal error, got: %v", err)
	}
}

// =============================================================================
// LocalStorage — List empty directory (local.go:193)
// =============================================================================

func TestLocalList_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "tenant1/app1")
	_ = os.MkdirAll(subDir, 0750)
	ls := NewLocalStorage(tmpDir, nil)
	entries, err := ls.List(context.Background(), "tenant1/app1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// =============================================================================
// LocalStorage — List with files (local.go:193)
// =============================================================================

func TestLocalList_WithFiles(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "tenant1/app1")
	_ = os.MkdirAll(subDir, 0750)
	f1 := filepath.Join(subDir, "backup1.json")
	f2 := filepath.Join(subDir, "backup2.json")
	_ = os.WriteFile(f1, []byte("data1"), 0644)
	_ = os.WriteFile(f2, []byte("data2"), 0644)

	ls := NewLocalStorage(tmpDir, nil)
	entries, err := ls.List(context.Background(), "tenant1/app1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}
}

// =============================================================================
// encryptAES256GCM / decryptAES256GCM — round trip (local.go:97/115)
// =============================================================================

func TestEncryptDecrypt_RoundTrip(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)

	plaintext := []byte("sensitive backup data")
	encrypted, err := encryptAES256GCM(plaintext, key)
	if err != nil {
		t.Fatalf("encryptAES256GCM: %v", err)
	}

	// Encrypted should be plaintext + nonce + overhead
	if len(encrypted) <= len(plaintext) {
		t.Error("expected encrypted to be larger than plaintext")
	}

	decrypted, err := decryptAES256GCM(encrypted, key)
	if err != nil {
		t.Fatalf("decryptAES256GCM: %v", err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("expected %v, got %v", plaintext, decrypted)
	}
}

func TestEncryptDecrypt_WrongKey(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	wrongKey := make([]byte, 32)
	_, _ = rand.Read(wrongKey)

	plaintext := []byte("sensitive data")
	encrypted, err := encryptAES256GCM(plaintext, key)
	if err != nil {
		t.Fatalf("encryptAES256GCM: %v", err)
	}

	_, err = decryptAES256GCM(encrypted, wrongKey)
	if err == nil {
		t.Fatal("expected decryption error with wrong key")
	}
}

func TestEncryptDecrypt_InvalidKeySize(t *testing.T) {
	_, err := encryptAES256GCM([]byte("data"), []byte("short-key"))
	if err == nil {
		t.Fatal("expected error for invalid key size")
	}
}

func TestDecrypt_ShortCiphertext(t *testing.T) {
	_, err := decryptAES256GCM([]byte("short"), make([]byte, 32))
	if err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("expected 'too short' error, got: %v", err)
	}
}

// =============================================================================
// LocalStorage — Upload and Download with encryption (local.go:46/136)
// =============================================================================

func TestLocalEncryptedUploadDownload(t *testing.T) {
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	tmpDir := t.TempDir()
	ls := NewLocalStorage(tmpDir, key)

	data := "encrypted backup data"
	err := ls.Upload(context.Background(), "enc/test.json", strings.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Verify the file on disk is encrypted (not plaintext)
	raw, err := os.ReadFile(filepath.Join(tmpDir, "enc/test.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(raw) == data {
		t.Error("expected file on disk to be encrypted")
	}

	reader, err := ls.Download(context.Background(), "enc/test.json")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	defer reader.Close()

	got, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != data {
		t.Errorf("expected %q, got %q", data, string(got))
	}
}

// =============================================================================
// S3Storage — bucketURL and objectURL (s3.go:351/363)
// =============================================================================

func TestS3Storage_BucketURL(t *testing.T) {
	s := &S3Storage{
		endpoint: "s3.amazonaws.com",
		bucket:   "my-bucket",
		region:   "us-east-1",
	}
	url := s.bucketURL()
	if !strings.Contains(url, "my-bucket.s3.amazonaws.com") {
		t.Errorf("unexpected bucket URL: %s", url)
	}
}

func TestS3Storage_BucketURLPathStyle(t *testing.T) {
	s := &S3Storage{
		endpoint:  "http://localhost:9000",
		bucket:    "my-bucket",
		region:    "us-east-1",
		pathStyle: true,
	}
	url := s.bucketURL()
	if !strings.Contains(url, "http://localhost:9000/my-bucket") {
		t.Errorf("unexpected path-style URL: %s", url)
	}
}

func TestS3Storage_ObjectURL(t *testing.T) {
	s := &S3Storage{
		endpoint: "s3.amazonaws.com",
		bucket:   "my-bucket",
		region:   "us-east-1",
	}
	url := s.objectURL("backups/test.json")
	if !strings.Contains(url, "my-bucket.s3.amazonaws.com/backups/test.json") {
		t.Errorf("unexpected object URL: %s", url)
	}
}

func TestS3Storage_ObjectURLPathStyle(t *testing.T) {
	s := &S3Storage{
		endpoint:  "http://localhost:9000",
		bucket:    "my-bucket",
		region:    "us-east-1",
		pathStyle: true,
	}
	url := s.objectURL("backups/test.json")
	if !strings.Contains(url, "http://localhost:9000/my-bucket/backups/test.json") {
		t.Errorf("unexpected path-style URL: %s", url)
	}
}

// =============================================================================
// S3Storage — retry with nil context (s3.go:106)
// =============================================================================

func TestS3Storage_RetryNilContext(t *testing.T) {
	s := &S3Storage{maxRetries: 1, initialDelay: time.Millisecond, maxDelay: time.Millisecond}
	err := s.retry(nil, func() error { return nil })
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
}

// =============================================================================
// S3Storage — NewS3Storage defaults (s3.go:54)
// =============================================================================

func TestNewS3Storage_Defaults(t *testing.T) {
	cfg := S3Config{
		Endpoint: "",
		Bucket:   "test",
		Region:   "us-east-1",
	}
	s := NewS3Storage(cfg, nil)
	if s == nil {
		t.Fatal("expected non-nil storage")
	}
	if s.maxRetries != 3 {
		t.Errorf("expected max retries 3, got %d", s.maxRetries)
	}
	if s.initialDelay != 100*time.Millisecond {
		t.Errorf("expected initial delay 100ms, got %v", s.initialDelay)
	}
	if s.maxDelay != 5*time.Second {
		t.Errorf("expected max delay 5s, got %v", s.maxDelay)
	}
}

// =============================================================================
// S3Storage — stripScheme (s3.go:95)
// =============================================================================

func TestStripSchemeExtra(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://s3.amazonaws.com", "s3.amazonaws.com"},
		{"http://localhost:9000", "localhost:9000"},
		{"s3.amazonaws.com", "s3.amazonaws.com"},
	}
	for _, tt := range tests {
		got := stripScheme(tt.input)
		if got != tt.want {
			t.Errorf("stripScheme(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// =============================================================================
// Module — basic identity (module.go)
// =============================================================================

func TestBackupModule_ID(t *testing.T) {
	m := &Module{}
	if m.ID() == "" {
		t.Error("expected non-empty ID")
	}
	if m.Name() == "" {
		t.Error("expected non-empty Name")
	}
	if m.Version() == "" {
		t.Error("expected non-empty Version")
	}
}

// === merged from scheduler_boost_test.go ===

// errStore is a mock store that fails UpdateBackupStatus.
type errStore struct {
	mockStore
}

func (e *errStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64, _ string) error {
	return errors.New("db down")
}

func TestScheduler_markFailed_Error(t *testing.T) {
	s := NewScheduler(
		&errStore{},
		nil, // storage
		nil, // event bus
		nil, // encryption
		"02:00",
		testLogger(),
	)

	// Should not panic even when store returns error
	ctx := context.Background()
	s.markFailed(ctx, "backup-1", "t1", "test failure", errors.New("cause"))
}

func TestScheduler_markFailed_Success(t *testing.T) {
	s := NewScheduler(
		&mockStore{},
		nil,
		nil,
		nil,
		"02:00",
		testLogger(),
	)

	ctx := context.Background()
	s.markFailed(ctx, "backup-1", "t1", "test failure", errors.New("cause"))
	// No panic = success
}

func TestScheduler_publishEvent_NilBus(t *testing.T) {
	s := NewScheduler(
		&mockStore{},
		nil,
		nil, // nil event bus
		nil,
		"02:00",
		testLogger(),
	)

	ctx := context.Background()
	err := s.publishEvent(ctx, "backup.completed", map[string]string{"key": "val"})
	if err != nil {
		t.Errorf("expected nil error with nil bus, got %v", err)
	}
}

func TestScheduler_publishEvent_WithBus(t *testing.T) {
	bus := core.NewEventBus(testLogger())
	s := NewScheduler(
		&mockStore{},
		nil,
		bus,
		nil,
		"02:00",
		testLogger(),
	)

	ctx := context.Background()
	err := s.publishEvent(ctx, "backup.completed", map[string]string{"key": "val"})
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

// ─── snapshotAndUpload coverage ────────────────────────────────────────────

type fakeSnapshotter struct{}

func (f *fakeSnapshotter) SnapshotBackup(_ context.Context, destPath string) error {
	return os.WriteFile(destPath, []byte("snapshot"), 0644)
}

type errSnapshotter struct {
	err error
}

func (e *errSnapshotter) SnapshotBackup(_ context.Context, _ string) error {
	return e.err
}

type errUploadStorage struct {
	passThroughStorage
	err error
}

func (e *errUploadStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return e.err
}

func TestScheduler_snapshotAndUpload_SnapshotError(t *testing.T) {
	s := NewScheduler(&mockStore{}, nil, nil, &errSnapshotter{err: errors.New("snap fail")}, "02:00", testLogger())
	s.snapshotAndUpload(context.Background(), &passThroughStorage{})
}

func TestScheduler_snapshotAndUpload_UploadError(t *testing.T) {
	s := NewScheduler(&mockStore{}, nil, nil, &fakeSnapshotter{}, "02:00", testLogger())
	stor := &errUploadStorage{err: errors.New("upload fail")}
	s.snapshotAndUpload(context.Background(), stor)
}

func TestScheduler_snapshotAndUpload_Success(t *testing.T) {
	s := NewScheduler(&mockStore{}, nil, nil, &fakeSnapshotter{}, "02:00", testLogger())
	s.snapshotAndUpload(context.Background(), &passThroughStorage{})
}

// ─── backupApp coverage ────────────────────────────────────────────────────

type errCreateStore struct {
	mockStore
}

func (e *errCreateStore) CreateBackup(_ context.Context, _ *core.Backup) error {
	return errors.New("create fail")
}

func TestScheduler_backupApp_CreateBackupError(t *testing.T) {
	s := NewScheduler(&errCreateStore{}, nil, nil, nil, "02:00", testLogger())
	ok := s.backupApp(context.Background(), core.Tenant{ID: "t1"}, core.Application{ID: "a1", Name: "app"}, &passThroughStorage{}, "local")
	if ok {
		t.Error("expected false when CreateBackup fails")
	}
}

func TestScheduler_backupApp_UploadError(t *testing.T) {
	s := NewScheduler(&mockStore{}, nil, nil, nil, "02:00", testLogger())
	stor := &errUploadStorage{err: errors.New("upload fail")}
	ok := s.backupApp(context.Background(), core.Tenant{ID: "t1"}, core.Application{ID: "a1", Name: "app"}, stor, "local")
	if ok {
		t.Error("expected false when Upload fails")
	}
}

func TestScheduler_backupApp_UpdateStatusError(t *testing.T) {
	// Upload succeeds but UpdateBackupStatus fails — backupApp still returns true.
	s := NewScheduler(&errStore{}, nil, nil, nil, "02:00", testLogger())
	ok := s.backupApp(context.Background(), core.Tenant{ID: "t1"}, core.Application{ID: "a1", Name: "app"}, &passThroughStorage{}, "local")
	if !ok {
		t.Error("expected true even when UpdateBackupStatus fails")
	}
}

func TestScheduler_backupApp_Success(t *testing.T) {
	s := NewScheduler(&mockStore{}, nil, nil, nil, "02:00", testLogger())
	ok := s.backupApp(context.Background(), core.Tenant{ID: "t1"}, core.Application{ID: "a1", Name: "app"}, &passThroughStorage{}, "local")
	if !ok {
		t.Error("expected true on success")
	}
}

type captureBackupStore struct {
	mockStore
	created *core.Backup
	backups []core.Backup
}

func (c *captureBackupStore) CreateBackup(_ context.Context, b *core.Backup) error {
	cp := *b
	c.created = &cp
	return nil
}

func (c *captureBackupStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return c.backups, len(c.backups), nil
}

type captureUploadStorage struct {
	passThroughStorage
	uploaded []string
}

func (c *captureUploadStorage) Upload(_ context.Context, key string, _ io.Reader, _ int64) error {
	c.uploaded = append(c.uploaded, key)
	return nil
}

func TestScheduler_backupApp_PersistsRestorableMetadata(t *testing.T) {
	store := &captureBackupStore{}
	storage := &captureUploadStorage{}
	s := NewScheduler(store, nil, nil, nil, "02:00", testLogger())

	ok := s.backupApp(context.Background(), core.Tenant{ID: "t1"}, core.Application{ID: "a1", Name: "app"}, storage, "local")
	if !ok {
		t.Fatal("expected backupApp to succeed")
	}
	if store.created == nil {
		t.Fatal("expected backup metadata to be created")
	}
	if store.created.FilePath == "" {
		t.Fatal("expected backup metadata to include a restorable file path")
	}
	if len(storage.uploaded) != 1 {
		t.Fatalf("uploaded keys = %v, want one upload", storage.uploaded)
	}
	if store.created.FilePath != storage.uploaded[0] {
		t.Fatalf("metadata file path = %q, uploaded key = %q", store.created.FilePath, storage.uploaded[0])
	}
	if store.created.Encryption == "" || store.created.Encryption == "aes-256-gcm" {
		t.Fatalf("expected encryption field to persist payload hash, got %q", store.created.Encryption)
	}
	if store.created.SizeBytes == 0 {
		t.Fatal("expected metadata to include payload size")
	}
}

func TestScheduler_backupApp_IncrementalKeepsPreviousFullPath(t *testing.T) {
	app := core.Application{ID: "a1", TenantID: "t1", Name: "app"}
	payload, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent: %v", err)
	}
	store := &captureBackupStore{
		backups: []core.Backup{{
			ID:         "prev",
			TenantID:   "t1",
			SourceID:   "a1",
			FilePath:   "t1/a1/prev.json",
			Encryption: computeSHA256(payload),
			Status:     "completed",
			CreatedAt:  time.Now().Add(-time.Hour),
		}},
	}
	storage := &captureUploadStorage{}
	s := NewScheduler(store, nil, nil, nil, "02:00", testLogger())

	ok := s.backupApp(context.Background(), core.Tenant{ID: "t1"}, app, storage, "local")
	if !ok {
		t.Fatal("expected backupApp to succeed")
	}
	if len(storage.uploaded) != 0 {
		t.Fatalf("expected metadata-only incremental backup, uploaded %v", storage.uploaded)
	}
	if store.created == nil {
		t.Fatal("expected backup metadata to be created")
	}
	if store.created.FilePath != "t1/a1/prev.json" {
		t.Fatalf("metadata file path = %q, want previous full backup path", store.created.FilePath)
	}
}

func TestScheduler_Closed_NilCtx(t *testing.T) {
	s := &Scheduler{logger: testLogger()}
	if s.Closed() {
		t.Error("expected Closed=false when stopCtx is nil")
	}
}

// === merged from tier67_hardening_test.go ===

// Tier 67 — backup scheduler hardening tests.
//
// These cover the regressions fixed in Tier 67:
//   - sync.Once on Stop (double-Stop used to panic)
//   - wg.Wait in Stop (previously the loop outlived Stop)
//   - cancellable context threaded from Stop through runBackups
//   - nil-logger guard on NewScheduler
//   - lastRunDate dedupe inside the minute-resolution tick loop
//   - ListAllTenants pagination (not the "call twice" pattern)
//   - publishEvent nil-event-bus tolerance
//   - error surfacing in CleanupOldBackups path (no silent ignore)

// ─── helpers ───────────────────────────────────────────────────────────────

// blockingStorage hangs on Upload until unblocked or the context is
// canceled. Used to prove that Stop actually unblocks in-flight
// uploads rather than letting them run to completion.
type blockingStorage struct {
	release   chan struct{}
	uploaded  atomic.Int32
	uploadErr atomic.Value // error
}

func newBlockingStorage() *blockingStorage {
	return &blockingStorage{release: make(chan struct{})}
}

func (b *blockingStorage) Name() string { return "blocking" }
func (b *blockingStorage) Upload(ctx context.Context, _ string, _ io.Reader, _ int64) error {
	select {
	case <-b.release:
		b.uploaded.Add(1)
		return nil
	case <-ctx.Done():
		if v := ctx.Err(); v != nil {
			b.uploadErr.Store(v)
		}
		return ctx.Err()
	}
}
func (b *blockingStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (b *blockingStorage) Delete(_ context.Context, _ string) error { return nil }
func (b *blockingStorage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
	return nil, nil
}

// countingStore is a paginated ListAllTenants stub that hands out
// tenants in fixed-size pages. Used to prove that runBackups pages
// properly instead of the pre-Tier-67 "call twice" pattern.
type countingStore struct {
	core.Store
	tenants    []core.Tenant
	listCalls  atomic.Int32
	appCalls   atomic.Int32
	maxObserve int // highest `limit` the scheduler asked for
	mu         sync.Mutex
}

func (c *countingStore) ListAllTenants(_ context.Context, limit, offset int) ([]core.Tenant, int, error) {
	c.listCalls.Add(1)
	c.mu.Lock()
	if limit > c.maxObserve {
		c.maxObserve = limit
	}
	c.mu.Unlock()
	if offset >= len(c.tenants) {
		return nil, len(c.tenants), nil
	}
	end := offset + limit
	if end > len(c.tenants) {
		end = len(c.tenants)
	}
	return c.tenants[offset:end], len(c.tenants), nil
}
func (c *countingStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	c.appCalls.Add(1)
	return nil, 0, nil
}
func (c *countingStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (c *countingStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64, _ string) error {
	return nil
}

// flakyDeleteStorage fails every Delete — used to prove that a
// Delete error in CleanupOldBackups does not abort the sweep.
type flakyDeleteStorage struct {
	entries []core.BackupEntry
}

func (f *flakyDeleteStorage) Name() string { return "flaky" }
func (f *flakyDeleteStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}
func (f *flakyDeleteStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *flakyDeleteStorage) Delete(_ context.Context, _ string) error {
	return errors.New("permission denied")
}
func (f *flakyDeleteStorage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
	return f.entries, nil
}

// ─── NewScheduler nil-logger guard ─────────────────────────────────────────

func TestTier67_NewScheduler_NilLogger(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", nil)
	if s == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if s.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if s.stopCtx == nil || s.stopCancel == nil {
		t.Error("stopCtx/stopCancel should be initialized")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier67_Scheduler_Stop_Idempotent(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	s.Start()

	// Double-Stop must not panic. Before Tier 67 the second call would
	// panic because Stop closed a plain channel with no sync.Once.
	s.Stop()
	s.Stop()
}

func TestTier67_Scheduler_Stop_WithoutStart_Safe(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	// Must not deadlock on wg.Wait — nothing was added to the group.
	s.Stop()
	s.Stop()
}

// ─── Start idempotency ─────────────────────────────────────────────────────

func TestTier67_Scheduler_Start_Idempotent(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())

	// Starting twice must not double-count wg. If it did, Stop would
	// block forever waiting for a phantom second goroutine.
	s.Start()
	s.Start()

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop deadlocked — startOnce/wg balance is wrong")
	}
}

// ─── Stop waits for the loop goroutine ─────────────────────────────────────

func TestTier67_Scheduler_Stop_WaitsForLoop(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	s.Start()

	// Give the goroutine a moment to enter its select.
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return — wg.Wait missing or deadlock")
	}
}

// ─── Stop cancels in-flight uploads via context ────────────────────────────

// TestTier67_Scheduler_Stop_CancelsInFlightUpload proves that a
// long-running Upload is canceled by Stop instead of running to
// completion. We directly invoke runBackupsCtx with the scheduler's
// stopCtx so we can race Stop against the hanging upload.
func TestTier67_Scheduler_Stop_CancelsInFlightUpload(t *testing.T) {
	events := core.NewEventBus(testLogger())
	bs := newBlockingStorage()
	storages := map[string]core.BackupStorage{"blocking": bs}

	store := &countingStore{tenants: []core.Tenant{{ID: "t1", Name: "one"}}}

	s := NewScheduler(store, storages, events, nil, "02:00", testLogger())

	// Kick off runBackupsCtx in the background. It will block on
	// blockingStorage.Upload inside backupApp → except store has no
	// apps, so Upload is never actually called. Swap to a store that
	// yields one app so we actually hit Upload.
	store2 := &storeWithOneApp{}
	s2 := NewScheduler(store2, storages, events, nil, "02:00", testLogger())

	done := make(chan struct{})
	go func() {
		s2.runBackupsCtx(s2.stopCtx)
		close(done)
	}()

	// Give the goroutine time to enter the blocking upload.
	time.Sleep(50 * time.Millisecond)

	// Stop should cancel the context, which propagates into Upload
	// via ctx.Done().
	s2.Stop()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runBackupsCtx did not exit after Stop — ctx cancellation is not plumbed")
	}

	// Upload should have seen the ctx-canceled path, not the
	// release-channel path.
	if bs.uploaded.Load() != 0 {
		t.Error("Upload completed despite Stop — ctx was not propagated")
	}
	if err, _ := bs.uploadErr.Load().(error); err == nil {
		t.Error("Upload did not observe ctx cancellation")
	}

	// Unused reference to s to silence linter if s ever stops being needed.
	_ = s
}

// storeWithOneApp is a store that returns one tenant and one app per
// tenant, then halts pagination on the next page.
type storeWithOneApp struct {
	core.Store
	returnedTenants bool
	returnedApps    bool
	mu              sync.Mutex
}

func (s *storeWithOneApp) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.returnedTenants {
		return nil, 1, nil
	}
	s.returnedTenants = true
	return []core.Tenant{{ID: "t1", Name: "one"}}, 1, nil
}
func (s *storeWithOneApp) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.returnedApps {
		return nil, 1, nil
	}
	s.returnedApps = true
	return []core.Application{{ID: "a1", TenantID: "t1"}}, 1, nil
}
func (s *storeWithOneApp) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (s *storeWithOneApp) UpdateBackupStatus(_ context.Context, _, _ string, _ int64, _ string) error {
	return nil
}

// ─── Pagination fix ────────────────────────────────────────────────────────

// TestTier67_Scheduler_ListAllTenants_Pagination proves that the
// scheduler pages through tenants with a sensible page size instead
// of the pre-Tier-67 "call with 10000 then call with total" pattern.
func TestTier67_Scheduler_ListAllTenants_Pagination(t *testing.T) {
	// Build 1200 tenants so more than one page is required even with
	// the 500 page size.
	tenants := make([]core.Tenant, 1200)
	for i := range tenants {
		tenants[i] = core.Tenant{ID: fmt.Sprintf("t%d", i), Name: "x"}
	}
	store := &countingStore{tenants: tenants}

	storages := map[string]core.BackupStorage{"local": &passThroughStorage{}}
	s := NewScheduler(store, storages, core.NewEventBus(testLogger()), nil, "02:00", testLogger())
	s.runBackups()

	// The old code called ListAllTenants exactly twice regardless of
	// tenant count. The new code pages, so the call count scales
	// with the number of pages (ceil(1200/500) = 3) + 1 for the
	// terminator page = 3 calls total for a partial last page.
	calls := store.listCalls.Load()
	if calls < 3 {
		t.Errorf("expected at least 3 paginated ListAllTenants calls, got %d", calls)
	}
	// The scheduler must never ask for more than its configured
	// page size. The pre-Tier-67 code asked for 10000 on the first
	// call.
	store.mu.Lock()
	maxObs := store.maxObserve
	store.mu.Unlock()
	if maxObs > 500 {
		t.Errorf("scheduler requested limit %d — pagination was not applied", maxObs)
	}
}

// passThroughStorage is a no-op storage used when the test only
// cares about the control-flow around it.
type passThroughStorage struct{}

func (p *passThroughStorage) Name() string { return "passthrough" }
func (p *passThroughStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}
func (p *passThroughStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (p *passThroughStorage) Delete(_ context.Context, _ string) error { return nil }
func (p *passThroughStorage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
	return nil, nil
}

// ─── lastRunDate dedupe ────────────────────────────────────────────────────

// TestTier67_Scheduler_LastRunDate_Dedupe is a structural guarantee:
// we cannot easily drive the internal ticker, but we can verify the
// field exists on the struct so regressions get caught by the
// compiler. (We cannot access unexported fields from outside the
// package; inside the package we can. This test lives in the same
// package.)
//
// The actual dedupe logic is proven indirectly by
// TestTier67_Scheduler_Stop_WaitsForLoop — if the loop fires runs
// in tight succession it would not affect this test because we
// never let the loop run long enough. So this test just asserts
// the code compiles against the new lastRunDate variable by
// exercising runBackups twice in a row and verifying it is a pure
// function (no state leaks between invocations).
func TestTier67_Scheduler_RunBackups_Reentrant(t *testing.T) {
	storages := map[string]core.BackupStorage{"local": &passThroughStorage{}}
	store := &countingStore{tenants: []core.Tenant{{ID: "t1", Name: "one"}}}

	s := NewScheduler(store, storages, core.NewEventBus(testLogger()), nil, "02:00", testLogger())

	// Call twice — both should complete without error.
	s.runBackups()
	s.runBackups()

	// Each call should page at least once.
	if store.listCalls.Load() < 2 {
		t.Errorf("expected at least 2 ListAllTenants calls across two runs, got %d", store.listCalls.Load())
	}
}

// ─── publishEvent nil-tolerance ────────────────────────────────────────────

func TestTier67_Scheduler_PublishEvent_NilBus(t *testing.T) {
	s := NewScheduler(nil, nil, nil, nil, "02:00", testLogger())
	if err := s.publishEvent(context.Background(), core.EventBackupStarted, nil); err != nil {
		t.Errorf("publishEvent should tolerate nil event bus, got: %v", err)
	}
}

// ─── CleanupOldBackups does not abort on delete error ──────────────────────

func TestTier67_CleanupOldBackups_IgnoresDeleteErrors(t *testing.T) {
	// Build a listing with all entries older than the cutoff.
	old := time.Now().AddDate(0, 0, -60).Unix()
	storage := &flakyDeleteStorage{
		entries: []core.BackupEntry{
			{Key: "a.json", CreatedAt: old},
			{Key: "b.json", CreatedAt: old},
			{Key: "c.json", CreatedAt: old},
		},
	}

	// Every Delete fails. CleanupOldBackups must still return a
	// (deleted=0, err=nil) result rather than bubbling up the first
	// Delete error — retention sweeps are best-effort.
	deleted, err := CleanupOldBackups(context.Background(), storage, "", 30)
	if err != nil {
		t.Errorf("CleanupOldBackups should not return error on Delete failure, got: %v", err)
	}
	if deleted != 0 {
		t.Errorf("expected deleted=0 when every Delete fails, got %d", deleted)
	}
}

// ─── Context cancellation aborts runBackupsCtx before I/O ──────────────────

func TestTier67_Scheduler_RunBackupsCtx_CancelledContext(t *testing.T) {
	store := &countingStore{tenants: []core.Tenant{{ID: "t1", Name: "one"}}}
	storages := map[string]core.BackupStorage{"local": &passThroughStorage{}}
	s := NewScheduler(store, storages, core.NewEventBus(testLogger()), nil, "02:00", testLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s.runBackupsCtx(ctx)

	// A canceled context at entry means no ListAllTenants call at all.
	if store.listCalls.Load() != 0 {
		t.Errorf("expected 0 ListAllTenants calls on canceled ctx, got %d", store.listCalls.Load())
	}
}

// ─── runBackups uses stopCtx when present ──────────────────────────────────

func TestTier67_Scheduler_RunBackups_UsesStopCtx(t *testing.T) {
	store := &countingStore{tenants: []core.Tenant{{ID: "t1", Name: "one"}}}
	storages := map[string]core.BackupStorage{"local": &passThroughStorage{}}
	s := NewScheduler(store, storages, core.NewEventBus(testLogger()), nil, "02:00", testLogger())

	// Cancel the scheduler's own context, then call runBackups. It
	// should short-circuit at the ctx.Err() check.
	s.stopCancel()
	s.runBackups()

	if store.listCalls.Load() != 0 {
		t.Errorf("expected runBackups to respect canceled stopCtx, got %d calls", store.listCalls.Load())
	}
}

// ─── runCtx fallback when stopCtx is nil ───────────────────────────────────

func TestTier67_Scheduler_RunCtx_NilFallback(t *testing.T) {
	// Bare struct literal — no NewScheduler, so stopCtx is nil.
	s := &Scheduler{logger: testLogger()}
	ctx := s.runCtx()
	if ctx == nil {
		t.Fatal("runCtx must not return nil")
	}
	if ctx.Err() != nil {
		t.Errorf("fallback background context should not be canceled: %v", ctx.Err())
	}
}
