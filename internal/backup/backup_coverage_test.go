package backup

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// S3 List() — covers s3.go:224 (39.3% → ~90%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestS3Storage_List_Success(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>test-bucket</Name>
  <Prefix>backups/</Prefix>
  <KeyCount>2</KeyCount>
  <Contents>
    <Key>backups/app1.tar.gz</Key>
    <LastModified>2025-01-15T10:30:00Z</LastModified>
    <Size>1024</Size>
    <ETag>"abc123"</ETag>
  </Contents>
  <Contents>
    <Key>backups/app2.tar.gz</Key>
    <LastModified>2025-01-14T08:00:00Z</LastModified>
    <Size>2048</Size>
    <ETag>"def456"</ETag>
  </Contents>
</ListBucketResult>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		// Verify list-type=2 query param
		if r.URL.Query().Get("list-type") != "2" {
			t.Errorf("expected list-type=2, got %q", r.URL.Query().Get("list-type"))
		}
		if r.URL.Query().Get("prefix") != "backups/" {
			t.Errorf("expected prefix=backups/, got %q", r.URL.Query().Get("prefix"))
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(xmlBody))
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test-bucket",
		pathStyle: true,
		client:    server.Client(),
		logger:    testLogger(),
	}

	entries, err := s.List(context.Background(), "backups/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Key != "backups/app1.tar.gz" {
		t.Errorf("entries[0].Key = %q, want backups/app1.tar.gz", entries[0].Key)
	}
	if entries[0].Size != 1024 {
		t.Errorf("entries[0].Size = %d, want 1024", entries[0].Size)
	}
	expected, _ := time.Parse(time.RFC3339, "2025-01-15T10:30:00Z")
	if entries[0].CreatedAt != expected.Unix() {
		t.Errorf("entries[0].CreatedAt = %d, want %d", entries[0].CreatedAt, expected.Unix())
	}

	if entries[1].Key != "backups/app2.tar.gz" {
		t.Errorf("entries[1].Key = %q, want backups/app2.tar.gz", entries[1].Key)
	}
	if entries[1].Size != 2048 {
		t.Errorf("entries[1].Size = %d, want 2048", entries[1].Size)
	}
}

func TestS3Storage_List_EmptyPrefix(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Name>bucket</Name>
  <KeyCount>1</KeyCount>
  <Contents>
    <Key>file.tar</Key>
    <LastModified>2025-03-01T00:00:00Z</LastModified>
    <Size>512</Size>
  </Contents>
</ListBucketResult>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// With empty prefix, should not have prefix param
		if r.URL.Query().Get("prefix") != "" {
			t.Errorf("expected no prefix param, got %q", r.URL.Query().Get("prefix"))
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(xmlBody))
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "bucket",
		pathStyle: true,
		client:    server.Client(),
		logger:    testLogger(),
	}

	entries, err := s.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Key != "file.tar" {
		t.Errorf("entry key = %q, want file.tar", entries[0].Key)
	}
}

func TestS3Storage_List_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:     server.URL,
		bucket:       "test",
		pathStyle:    true,
		client:       server.Client(),
		maxRetries:   1,
		initialDelay: time.Millisecond,
		maxDelay:     time.Millisecond,
	}

	_, err := s.List(context.Background(), "prefix-")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("error should mention 500, got: %v", err)
	}
}

func TestS3Storage_List_InvalidXML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-xml"))
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:     server.URL,
		bucket:       "test",
		pathStyle:    true,
		client:       server.Client(),
		maxRetries:   1,
		initialDelay: time.Millisecond,
		maxDelay:     time.Millisecond,
	}

	_, err := s.List(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
	if !strings.Contains(err.Error(), "parse list response") {
		t.Errorf("error should mention parse, got: %v", err)
	}
}

func TestS3Storage_List_EmptyResult(t *testing.T) {
	xmlBody := `<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult>
  <Name>bucket</Name>
  <KeyCount>0</KeyCount>
</ListBucketResult>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(xmlBody))
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "bucket",
		pathStyle: true,
		client:    server.Client(),
		logger:    testLogger(),
	}

	entries, err := s.List(context.Background(), "nonexistent/")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// S3 bucketURL() — covers s3.go:296 (71.4% → 100%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestS3Storage_BucketURL_PathStyle_HTTP(t *testing.T) {
	s := &S3Storage{
		endpoint:  "http://minio.local:9000",
		bucket:    "backups",
		pathStyle: true,
	}
	got := s.bucketURL()
	want := "http://minio.local:9000/backups"
	if got != want {
		t.Errorf("bucketURL() = %q, want %q", got, want)
	}
}

func TestS3Storage_BucketURL_PathStyle_HTTPS(t *testing.T) {
	s := &S3Storage{
		endpoint:  "https://minio.local:9000",
		bucket:    "backups",
		pathStyle: true,
	}
	got := s.bucketURL()
	want := "https://minio.local:9000/backups"
	if got != want {
		t.Errorf("bucketURL() = %q, want %q", got, want)
	}
}

func TestS3Storage_BucketURL_VirtualHost(t *testing.T) {
	s := &S3Storage{
		endpoint:  "s3.us-east-1.amazonaws.com",
		bucket:    "my-bucket",
		pathStyle: false,
	}
	got := s.bucketURL()
	want := "https://my-bucket.s3.us-east-1.amazonaws.com"
	if got != want {
		t.Errorf("bucketURL() = %q, want %q", got, want)
	}
}

func TestS3Storage_BucketURL_VirtualHost_HTTP(t *testing.T) {
	s := &S3Storage{
		endpoint:  "http://s3.local:4566",
		bucket:    "dev",
		pathStyle: false,
	}
	got := s.bucketURL()
	want := "http://dev.s3.local:4566"
	if got != want {
		t.Errorf("bucketURL() = %q, want %q", got, want)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module Init with S3 — covers module.go:52-64 (73.3% → ~95%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestModuleInit_WithS3Config(t *testing.T) {
	tmpDir := t.TempDir()

	c := &core.Core{
		Config: &core.Config{
			Backup: core.BackupConfig{
				StoragePath: filepath.Join(tmpDir, "backups"),
				S3: core.BackupS3Config{
					Bucket:    "my-backup-bucket",
					Region:    "us-east-1",
					Endpoint:  "https://s3.amazonaws.com",
					AccessKey: "AKIAIOSFODNN7EXAMPLE",
					SecretKey: "secret",
					PathStyle: false,
				},
			},
		},
		Logger:   testLogger(),
		Services: core.NewServices(),
	}

	m := New()
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	// Should have both local and s3
	names := m.StorageNames()
	if len(names) != 2 {
		t.Fatalf("expected 2 storages, got %d: %v", len(names), names)
	}

	foundLocal, foundS3 := false, false
	for _, n := range names {
		if n == "local" {
			foundLocal = true
		}
		if n == "s3" {
			foundS3 = true
		}
	}
	if !foundLocal {
		t.Error("expected local storage")
	}
	if !foundS3 {
		t.Error("expected s3 storage")
	}

	// Both should be registered in core services
	if s := c.Services.BackupStorage("local"); s == nil {
		t.Error("local storage not registered in core services")
	}
	if s := c.Services.BackupStorage("s3"); s == nil {
		t.Error("s3 storage not registered in core services")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Scheduler runBackups with snapshotter — covers scheduler.go:97-139
// ═══════════════════════════════════════════════════════════════════════════════

// goodStorage is a happy-path core.BackupStorage that records every Upload
// but never returns an error. Used by scheduler tests that care about the
// control flow, not the storage semantics.
type goodStorage struct {
	uploaded []string
}

func (g *goodStorage) Name() string { return "good" }
func (g *goodStorage) Upload(_ context.Context, key string, _ io.Reader, _ int64) error {
	g.uploaded = append(g.uploaded, key)
	return nil
}
func (g *goodStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (g *goodStorage) Delete(_ context.Context, _ string) error { return nil }
func (g *goodStorage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
	return nil, nil
}

// mockStoreWithApps returns one tenant with one app. Used by scheduler
// tests that need to exercise the full backup-an-app path.
type mockStoreWithApps struct{ core.Store }

func (m *mockStoreWithApps) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return []core.Tenant{{ID: "t1", Name: "test"}}, 1, nil
}
func (m *mockStoreWithApps) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return []core.Application{{ID: "a1", TenantID: "t1", Name: "app1"}}, 1, nil
}
func (m *mockStoreWithApps) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (m *mockStoreWithApps) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
func (m *mockStoreWithApps) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error {
	return nil
}

// mockSnapshotter implements core.DBSnapshotter for testing.
type mockSnapshotter struct {
	err      error
	callPath string
}

func (m *mockSnapshotter) SnapshotBackup(_ context.Context, destPath string) error {
	m.callPath = destPath
	if m.err != nil {
		return m.err
	}
	// Write a small file to simulate a real snapshot
	return os.WriteFile(destPath, []byte("snapshot-data"), 0644)
}

func TestScheduler_RunBackups_WithSnapshotter(t *testing.T) {
	events := core.NewEventBus(testLogger())

	var published []string
	events.Subscribe("backup.*", func(_ context.Context, e core.Event) error {
		published = append(published, e.Type)
		return nil
	})
	events.Subscribe("alert.*", func(_ context.Context, _ core.Event) error {
		return nil
	})

	storage := &goodStorage{}
	storages := map[string]core.BackupStorage{
		"local": storage,
	}

	store := &mockStoreWithApps{}
	snap := &mockSnapshotter{}

	s := NewScheduler(store, storages, events, snap, "02:00", testLogger())
	s.runBackups()

	// Snapshotter should have been called
	if snap.callPath == "" {
		t.Error("snapshotter was not called")
	}

	// Backup events should fire
	if len(published) < 2 {
		t.Fatalf("expected at least 2 backup events, got %d: %v", len(published), published)
	}
}

func TestScheduler_RunBackups_SnapshotterError(t *testing.T) {
	events := core.NewEventBus(testLogger())
	events.Subscribe("backup.*", func(_ context.Context, _ core.Event) error {
		return nil
	})

	storage := &goodStorage{}
	storages := map[string]core.BackupStorage{
		"local": storage,
	}

	store := &mockStoreWithApps{}
	snap := &mockSnapshotter{err: fmt.Errorf("disk full")}

	s := NewScheduler(store, storages, events, snap, "02:00", testLogger())
	// Should not panic even if snapshotter fails
	s.runBackups()

	// Verify snapshotter was called (and failed gracefully)
	if snap.callPath == "" {
		t.Error("snapshotter was not called")
	}
}

func TestScheduler_RunBackups_NoStorage(t *testing.T) {
	events := core.NewEventBus(testLogger())
	events.Subscribe("backup.*", func(_ context.Context, _ core.Event) error {
		return nil
	})

	storages := map[string]core.BackupStorage{} // empty
	store := &mockStore{}

	s := NewScheduler(store, storages, events, nil, "02:00", testLogger())
	// Should return early without panic
	s.runBackups()
}

func TestScheduler_RunBackups_NilStorageValue(t *testing.T) {
	events := core.NewEventBus(testLogger())
	events.Subscribe("backup.*", func(_ context.Context, _ core.Event) error {
		return nil
	})

	storages := map[string]core.BackupStorage{
		"local": nil, // nil storage value
	}
	store := &mockStore{}

	s := NewScheduler(store, storages, events, nil, "02:00", testLogger())
	// Should handle nil storage gracefully
	s.runBackups()
}

// ═══════════════════════════════════════════════════════════════════════════════
// S3 retry — covers context cancellation branch (s3.go:101)
// ═══════════════════════════════════════════════════════════════════════════════

func TestS3Storage_Retry_ContextCancelled(t *testing.T) {
	s := &S3Storage{
		maxRetries:   3,
		initialDelay: time.Millisecond,
		maxDelay:     10 * time.Millisecond,
		logger:       testLogger(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	calls := 0
	err := s.retry(ctx, func() error {
		calls++
		return fmt.Errorf("transient error")
	})

	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	// Should have tried once, then context is done
	if calls < 1 {
		t.Errorf("expected at least 1 call, got %d", calls)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// S3 Delete with 404 — covers s3.go:213 (404 not treated as error)
// ═══════════════════════════════════════════════════════════════════════════════

func TestS3Storage_Delete_404_NotError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
		logger:    testLogger(),
	}

	err := s.Delete(context.Background(), "nonexistent.tar")
	if err != nil {
		t.Errorf("Delete() should not error on 404, got: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// S3 Download HTTP error — covers s3.go:189 (error branch for >= 400)
// ═══════════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════════
// Scheduler runBackups with ListAppsByTenant error
// ═══════════════════════════════════════════════════════════════════════════════

type mockStoreListAppsError struct{ core.Store }

func (m *mockStoreListAppsError) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return []core.Tenant{{ID: "t1", Name: "test"}}, 1, nil
}
func (m *mockStoreListAppsError) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, fmt.Errorf("database error")
}
func (m *mockStoreListAppsError) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (m *mockStoreListAppsError) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
func (m *mockStoreListAppsError) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error {
	return nil
}

func TestScheduler_RunBackups_ListAppsError(t *testing.T) {
	events := core.NewEventBus(testLogger())
	events.Subscribe("backup.*", func(_ context.Context, _ core.Event) error {
		return nil
	})

	storages := map[string]core.BackupStorage{
		"local": &mockBackupStorage{},
	}
	store := &mockStoreListAppsError{}

	s := NewScheduler(store, storages, events, nil, "02:00", testLogger())
	// Should not panic
	s.runBackups()
}

// ═══════════════════════════════════════════════════════════════════════════════
// Scheduler runBackups with ListAllTenants error
// ═══════════════════════════════════════════════════════════════════════════════

type mockStoreListTenantsError struct{ core.Store }

func (m *mockStoreListTenantsError) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return nil, 0, fmt.Errorf("database unavailable")
}
func (m *mockStoreListTenantsError) CreateBackup(_ context.Context, _ *core.Backup) error {
	return nil
}
func (m *mockStoreListTenantsError) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
func (m *mockStoreListTenantsError) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error {
	return nil
}

func TestScheduler_RunBackups_ListTenantsError(t *testing.T) {
	events := core.NewEventBus(testLogger())
	events.Subscribe("backup.*", func(_ context.Context, _ core.Event) error {
		return nil
	})

	storages := map[string]core.BackupStorage{
		"local": &mockBackupStorage{},
	}
	store := &mockStoreListTenantsError{}

	s := NewScheduler(store, storages, events, nil, "02:00", testLogger())
	// Should not panic, should return early
	s.runBackups()
}

// ═══════════════════════════════════════════════════════════════════════════════
// Scheduler runBackups — upload failure path
// ═══════════════════════════════════════════════════════════════════════════════

// failUploadStorage fails on Upload but succeeds elsewhere.
type failUploadStorage struct {
	uploaded map[string]string
}

func (f *failUploadStorage) Name() string { return "fail-upload" }
func (f *failUploadStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return fmt.Errorf("storage full")
}
func (f *failUploadStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (f *failUploadStorage) Delete(_ context.Context, _ string) error { return nil }
func (f *failUploadStorage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
	return nil, nil
}

func TestScheduler_RunBackups_UploadFailure(t *testing.T) {
	events := core.NewEventBus(testLogger())
	events.Subscribe("backup.*", func(_ context.Context, _ core.Event) error {
		return nil
	})

	storages := map[string]core.BackupStorage{
		"local": &failUploadStorage{},
	}
	store := &mockStoreWithApps{}

	s := NewScheduler(store, storages, events, nil, "02:00", testLogger())
	// Should not panic — upload failure is logged and counted
	s.runBackups()
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module Start with DB snapshotter — covers module.go:76-78
// ═══════════════════════════════════════════════════════════════════════════════

func TestModuleStart_WithDBSnapshotter(t *testing.T) {
	tmpDir := t.TempDir()

	c := &core.Core{
		Config: &core.Config{
			Backup: core.BackupConfig{
				StoragePath: filepath.Join(tmpDir, "backups"),
				Schedule:    "04:00",
			},
		},
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
		DB: &core.Database{
			Snapshotter: &mockSnapshotter{},
		},
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if m.scheduler == nil {
		t.Fatal("scheduler should be set")
	}

	m.Stop(context.Background())
}

// ═══════════════════════════════════════════════════════════════════════════════
// S3 List network error — covers retry path in List
// ═══════════════════════════════════════════════════════════════════════════════

func TestS3Storage_List_NetworkError(t *testing.T) {
	s := &S3Storage{
		endpoint:     "http://127.0.0.1:1",
		bucket:       "test",
		pathStyle:    true,
		client:       &http.Client{Timeout: 100 * time.Millisecond},
		maxRetries:   1,
		initialDelay: time.Millisecond,
		maxDelay:     time.Millisecond,
	}

	_, err := s.List(context.Background(), "prefix-")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// S3 Upload with logger=nil — covers s3.go:168 nil logger path
// ═══════════════════════════════════════════════════════════════════════════════

func TestS3Storage_Upload_NilLogger(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
		logger:    nil, // nil logger
	}

	err := s.Upload(context.Background(), "key.tar", strings.NewReader("data"), 4)
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// stripScheme edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestStripScheme(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://example.com", "example.com"},
		{"http://example.com", "example.com"},
		{"example.com", "example.com"},
		{"ftp://example.com", "ftp://example.com"}, // Only strips http(s)
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := stripScheme(tt.input); got != tt.want {
				t.Errorf("stripScheme(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
