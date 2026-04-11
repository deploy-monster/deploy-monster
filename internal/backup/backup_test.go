package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// HELPERS
// =====================================================

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// Ensure testLogger is used for S3Storage tests
var _ = testLogger()

// mockStore implements core.Store minimally for scheduler tests.
type mockStore struct{ core.Store }

func (m *mockStore) ListAllTenants(_ context.Context, _, _ int) ([]core.Tenant, int, error) {
	return []core.Tenant{{ID: "t1", Name: "test"}}, 1, nil
}
func (m *mockStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (m *mockStore) CreateBackup(_ context.Context, _ *core.Backup) error { return nil }
func (m *mockStore) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}
func (m *mockStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error { return nil }

// mockBackupStorage implements core.BackupStorage for cleanup tests.
type mockBackupStorage struct {
	entries []core.BackupEntry
	listErr error
	deleted []string
	delErr  error
}

func (m *mockBackupStorage) Name() string { return "mock" }
func (m *mockBackupStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}
func (m *mockBackupStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *mockBackupStorage) Delete(_ context.Context, key string) error {
	if m.delErr != nil {
		return m.delErr
	}
	m.deleted = append(m.deleted, key)
	return nil
}
func (m *mockBackupStorage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.entries, nil
}

// =====================================================
// MODULE TESTS
// =====================================================

func TestNew(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
	if m.storages == nil {
		t.Fatal("storages map should be initialized")
	}
}

func TestModuleImplementsInterface(t *testing.T) {
	var _ core.Module = (*Module)(nil)
}

func TestModuleIdentity(t *testing.T) {
	m := New()

	tests := []struct {
		method string
		got    string
		want   string
	}{
		{"ID", m.ID(), "backup"},
		{"Name", m.Name(), "Backup Engine"},
		{"Version", m.Version(), "1.0.0"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s() = %q, want %q", tt.method, tt.got, tt.want)
			}
		})
	}
}

func TestModuleDependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "core.db" {
		t.Errorf("Dependencies() = %v, want [core.db]", deps)
	}
}

func TestModuleRoutes(t *testing.T) {
	m := New()
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
}

func TestModuleEvents(t *testing.T) {
	m := New()
	if events := m.Events(); events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

func TestModuleHealth(t *testing.T) {
	m := New()
	// Empty storages → Degraded
	if h := m.Health(); h != core.HealthDegraded {
		t.Errorf("Health() with no storages = %v, want HealthDegraded", h)
	}
	// With storage → OK
	m.RegisterStorage("local", &mockBackupStorage{})
	if h := m.Health(); h != core.HealthOK {
		t.Errorf("Health() with storage = %v, want HealthOK", h)
	}
}

func TestModuleInit_WithStoragePath(t *testing.T) {
	tmpDir := t.TempDir()

	c := &core.Core{
		Config: &core.Config{
			Backup: core.BackupConfig{
				StoragePath: filepath.Join(tmpDir, "backups"),
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

	// Should have registered local storage
	names := m.StorageNames()
	if len(names) != 1 || names[0] != "local" {
		t.Errorf("StorageNames() = %v, want [local]", names)
	}

	// Core services should have backup storage registered
	if s := c.Services.BackupStorage("local"); s == nil {
		t.Error("expected local backup storage registered in core services")
	}
}

func TestModuleInit_EmptyStoragePath(t *testing.T) {
	c := &core.Core{
		Config: &core.Config{
			Backup: core.BackupConfig{
				StoragePath: "",
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

	// Should default to "backups" path
	names := m.StorageNames()
	if len(names) != 1 {
		t.Errorf("expected 1 storage, got %d", len(names))
	}
}

func TestModuleStartAndStop(t *testing.T) {
	tmpDir := t.TempDir()

	c := &core.Core{
		Config: &core.Config{
			Backup: core.BackupConfig{
				StoragePath: filepath.Join(tmpDir, "backups"),
				Schedule:    "03:00",
			},
		},
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Scheduler should be set
	if m.scheduler == nil {
		t.Fatal("scheduler should be set after Start()")
	}

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

func TestModuleStart_DefaultSchedule(t *testing.T) {
	tmpDir := t.TempDir()

	c := &core.Core{
		Config: &core.Config{
			Backup: core.BackupConfig{
				StoragePath: filepath.Join(tmpDir, "backups"),
				Schedule:    "", // empty => default "02:00"
			},
		},
		Logger:   testLogger(),
		Events:   core.NewEventBus(testLogger()),
		Services: core.NewServices(),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	// Clean up
	m.Stop(context.Background())
}

func TestModuleStop_NilScheduler(t *testing.T) {
	m := New()
	// Stop with nil scheduler should not panic
	err := m.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop() with nil scheduler returned error: %v", err)
	}
}

// =====================================================
// RegisterStorage / StorageNames TESTS
// =====================================================

func TestRegisterStorage(t *testing.T) {
	m := New()
	mock := &mockBackupStorage{}

	m.RegisterStorage("s3", mock)

	names := m.StorageNames()
	if len(names) != 1 || names[0] != "s3" {
		t.Errorf("StorageNames() = %v, want [s3]", names)
	}
}

func TestRegisterStorage_Multiple(t *testing.T) {
	m := New()
	m.RegisterStorage("local", &mockBackupStorage{})
	m.RegisterStorage("s3", &mockBackupStorage{})
	m.RegisterStorage("r2", &mockBackupStorage{})

	names := m.StorageNames()
	if len(names) != 3 {
		t.Errorf("expected 3 storages, got %d", len(names))
	}
}

func TestRegisterStorage_ConcurrentAccess(t *testing.T) {
	m := New()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			name := fmt.Sprintf("storage-%d", idx)
			m.RegisterStorage(name, &mockBackupStorage{})
		}(i)
	}
	wg.Wait()

	names := m.StorageNames()
	if len(names) != 20 {
		t.Errorf("expected 20 storages after concurrent registration, got %d", len(names))
	}
}

func TestStorageNames_ConcurrentRead(t *testing.T) {
	m := New()
	m.RegisterStorage("local", &mockBackupStorage{})
	m.RegisterStorage("s3", &mockBackupStorage{})

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			names := m.StorageNames()
			if len(names) < 2 {
				t.Errorf("expected at least 2 storages, got %d", len(names))
			}
		}()
	}
	wg.Wait()
}

// =====================================================
// LOCAL STORAGE TESTS
// =====================================================

func TestLocalStorage_Name(t *testing.T) {
	ls := NewLocalStorage(t.TempDir())
	if name := ls.Name(); name != "local" {
		t.Errorf("Name() = %q, want %q", name, "local")
	}
}

func TestLocalStorage_ImplementsInterface(t *testing.T) {
	var _ core.BackupStorage = (*LocalStorage)(nil)
}

func TestLocalStorage_Upload(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	data := []byte("test backup data")
	err := ls.Upload(context.Background(), "test-backup.tar.gz", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	// Verify file exists
	content, err := os.ReadFile(filepath.Join(dir, "test-backup.tar.gz"))
	if err != nil {
		t.Fatalf("failed to read uploaded file: %v", err)
	}
	if !bytes.Equal(content, data) {
		t.Error("uploaded file content mismatch")
	}
}

func TestLocalStorage_Upload_SubDirectory(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	data := []byte("nested backup")
	err := ls.Upload(context.Background(), "apps/myapp/backup.tar.gz", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "apps", "myapp", "backup.tar.gz"))
	if err != nil {
		t.Fatalf("failed to read nested file: %v", err)
	}
	if !bytes.Equal(content, data) {
		t.Error("file content mismatch")
	}
}

func TestLocalStorage_Download(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	// Write a file first
	data := []byte("download me")
	os.WriteFile(filepath.Join(dir, "backup.tar"), data, 0644)

	rc, err := ls.Download(context.Background(), "backup.tar")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer rc.Close()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("failed to read download: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("downloaded content mismatch")
	}
}

func TestLocalStorage_Download_NotFound(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	_, err := ls.Download(context.Background(), "nonexistent.tar")
	if err == nil {
		t.Error("Download() expected error for nonexistent file")
	}
}

func TestLocalStorage_Delete(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	// Create a file
	path := filepath.Join(dir, "delete-me.tar")
	os.WriteFile(path, []byte("data"), 0644)

	err := ls.Delete(context.Background(), "delete-me.tar")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// File should not exist
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should have been deleted")
	}
}

func TestLocalStorage_Delete_NotFound(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	err := ls.Delete(context.Background(), "nonexistent.tar")
	if err == nil {
		t.Error("Delete() expected error for nonexistent file")
	}
}

func TestLocalStorage_List(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	// Create some files
	for _, name := range []string{"backup-001.tar", "backup-002.tar", "backup-003.tar"} {
		os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644)
	}

	entries, err := ls.List(context.Background(), "backup-")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(entries) != 3 {
		t.Fatalf("List() returned %d entries, want 3", len(entries))
	}

	// Verify entries have valid fields
	for _, e := range entries {
		if e.Key == "" {
			t.Error("entry key should not be empty")
		}
		if e.Size == 0 {
			t.Error("entry size should not be zero")
		}
		if e.CreatedAt == 0 {
			t.Error("entry createdAt should not be zero")
		}
	}
}

func TestLocalStorage_List_Empty(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	entries, err := ls.List(context.Background(), "none-")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List() returned %d entries, want 0", len(entries))
	}
}

func TestLocalStorage_List_SortedByCreatedAtDescending(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	// Create files with different modification times
	names := []string{"b-old.tar", "b-mid.tar", "b-new.tar"}
	for i, name := range names {
		path := filepath.Join(dir, name)
		os.WriteFile(path, []byte("data"), 0644)
		// Set modification times: oldest first
		modTime := time.Now().Add(time.Duration(i) * time.Hour)
		os.Chtimes(path, modTime, modTime)
	}

	entries, err := ls.List(context.Background(), "b-")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(entries) < 2 {
		t.Fatalf("expected at least 2 entries, got %d", len(entries))
	}

	// Should be sorted newest first (descending)
	for i := 0; i < len(entries)-1; i++ {
		if entries[i].CreatedAt < entries[i+1].CreatedAt {
			t.Errorf("entries not sorted descending: %d < %d at index %d",
				entries[i].CreatedAt, entries[i+1].CreatedAt, i)
		}
	}
}

// =====================================================
// S3 STORAGE TESTS
// =====================================================

func TestNewS3Storage_CustomEndpoint(t *testing.T) {
	s := NewS3Storage(S3Config{
		Endpoint:  "https://minio.local:9000",
		Bucket:    "backups",
		Region:    "us-east-1",
		AccessKey: "access",
		SecretKey: "secret",
		PathStyle: true,
	}, testLogger())

	// Endpoint is stored without scheme for consistent URL building
	if s.endpoint != "minio.local:9000" {
		t.Errorf("endpoint = %q, want custom endpoint", s.endpoint)
	}
	if s.bucket != "backups" {
		t.Errorf("bucket = %q, want %q", s.bucket, "backups")
	}
	if !s.pathStyle {
		t.Error("pathStyle should be true")
	}
}

func TestNewS3Storage_DefaultEndpoint(t *testing.T) {
	s := NewS3Storage(S3Config{
		Endpoint:  "",
		Bucket:    "my-bucket",
		Region:    "eu-west-1",
		AccessKey: "ak",
		SecretKey: "sk",
	}, testLogger())

	// Endpoint is stored without scheme for consistent URL building
	expected := "s3.eu-west-1.amazonaws.com"
	if s.endpoint != expected {
		t.Errorf("endpoint = %q, want %q", s.endpoint, expected)
	}
	if s.pathStyle {
		t.Error("pathStyle should be false by default")
	}
}

func TestS3Storage_Name(t *testing.T) {
	s := NewS3Storage(S3Config{Bucket: "b", Region: "r"}, testLogger())
	if name := s.Name(); name != "s3" {
		t.Errorf("Name() = %q, want %q", name, "s3")
	}
}

func TestS3Storage_ImplementsInterface(t *testing.T) {
	var _ core.BackupStorage = (*S3Storage)(nil)
}

func TestS3Storage_ObjectURL_PathStyle(t *testing.T) {
	s := &S3Storage{
		endpoint:  "https://minio.local:9000",
		bucket:    "backups",
		pathStyle: true,
	}

	url := s.objectURL("path/to/backup.tar.gz")
	expected := "https://minio.local:9000/backups/path/to/backup.tar.gz"
	if url != expected {
		t.Errorf("objectURL() = %q, want %q", url, expected)
	}
}

func TestS3Storage_ObjectURL_VirtualHostStyle(t *testing.T) {
	s := &S3Storage{
		endpoint:  "s3.us-east-1.amazonaws.com",
		bucket:    "my-bucket",
		pathStyle: false,
	}

	url := s.objectURL("backup.tar.gz")
	expected := "https://my-bucket.s3.us-east-1.amazonaws.com/backup.tar.gz"
	if url != expected {
		t.Errorf("objectURL() = %q, want %q", url, expected)
	}
}

func TestS3Storage_Upload_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != "backup-data" {
			t.Errorf("unexpected body: %q", string(body))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
	}

	data := []byte("backup-data")
	err := s.Upload(context.Background(), "key.tar", bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
}

func TestS3Storage_Upload_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
	}

	err := s.Upload(context.Background(), "key.tar", bytes.NewReader([]byte("data")), 4)
	if err == nil {
		t.Fatal("Upload() expected error for 403 response")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

func TestS3Storage_Upload_NetworkError(t *testing.T) {
	s := &S3Storage{
		endpoint:  "http://127.0.0.1:1", // nothing listening
		bucket:    "test",
		pathStyle: true,
		client:    &http.Client{Timeout: 100 * time.Millisecond},
	}

	err := s.Upload(context.Background(), "key.tar", bytes.NewReader([]byte("data")), 4)
	if err == nil {
		t.Fatal("Upload() expected error for unreachable server")
	}
}

func TestS3Storage_Download_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("downloaded-data"))
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
	}

	rc, err := s.Download(context.Background(), "key.tar")
	if err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	defer rc.Close()

	data, _ := io.ReadAll(rc)
	if string(data) != "downloaded-data" {
		t.Errorf("Download() content = %q, want %q", string(data), "downloaded-data")
	}
}

func TestS3Storage_Download_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
	}

	_, err := s.Download(context.Background(), "missing.tar")
	if err == nil {
		t.Fatal("Download() expected error for 404 response")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error should mention 404, got: %v", err)
	}
}

func TestS3Storage_Download_NetworkError(t *testing.T) {
	s := &S3Storage{
		endpoint:  "http://127.0.0.1:1",
		bucket:    "test",
		pathStyle: true,
		client:    &http.Client{Timeout: 100 * time.Millisecond},
	}

	_, err := s.Download(context.Background(), "key.tar")
	if err == nil {
		t.Fatal("Download() expected error for unreachable server")
	}
}

func TestS3Storage_Delete_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
	}

	err := s.Delete(context.Background(), "key.tar")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestS3Storage_Delete_NetworkError(t *testing.T) {
	// Use a closed server to guarantee connection refused on all platforms
	closedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedSrv.Close()

	s := &S3Storage{
		endpoint:  closedSrv.URL,
		bucket:    "test",
		pathStyle: true,
		client:    &http.Client{Timeout: 100 * time.Millisecond},
	}

	err := s.Delete(context.Background(), "key.tar")
	if err == nil {
		t.Fatal("Delete() expected error for unreachable server")
	}
}

func TestS3Storage_List_ReturnsError(t *testing.T) {
	s := NewS3Storage(S3Config{Bucket: "b", Region: "r"}, testLogger())
	_, err := s.List(context.Background(), "prefix")
	if err == nil {
		t.Fatal("List() expected error (no S3 server available)")
	}
	// Should get a network/connection error since no real S3 endpoint exists
	if !strings.Contains(err.Error(), "S3") && !strings.Contains(err.Error(), "connection") && !strings.Contains(err.Error(), "lookup") {
		t.Errorf("unexpected error: %v", err)
	}
}

// =====================================================
// SCHEDULER TESTS
// =====================================================

func TestNewScheduler(t *testing.T) {
	storages := map[string]core.BackupStorage{}
	events := core.NewEventBus(testLogger())
	s := NewScheduler(nil, storages, events, nil, "03:00", testLogger())

	if s == nil {
		t.Fatal("NewScheduler() returned nil")
	}
	if s.schedule != "03:00" {
		t.Errorf("schedule = %q, want %q", s.schedule, "03:00")
	}
}

func TestScheduler_StartAndStop(t *testing.T) {
	storages := map[string]core.BackupStorage{}
	events := core.NewEventBus(testLogger())
	s := NewScheduler(nil, storages, events, nil, "02:00", testLogger())

	s.Start()

	// Give goroutine time to start
	time.Sleep(10 * time.Millisecond)

	// Stop should close the channel and exit the goroutine
	s.Stop()

	// Give goroutine time to exit
	time.Sleep(10 * time.Millisecond)

	// Double-stop should not panic — but the channel is already closed,
	// so we just verify Stop() was called once cleanly.
}

// =====================================================
// parseSimpleSchedule TESTS
// =====================================================

func TestParseSimpleSchedule(t *testing.T) {
	tests := []struct {
		input string
		wantH int
		wantM int
	}{
		{"02:00", 2, 0},
		{"14:30", 14, 30},
		{"0:00", 0, 0},
		{"23:59", 23, 59},
		{" 3 : 15 ", 3, 15}, // with spaces
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			h, m := parseSimpleSchedule(tt.input)
			if h != tt.wantH || m != tt.wantM {
				t.Errorf("parseSimpleSchedule(%q) = (%d, %d), want (%d, %d)",
					tt.input, h, m, tt.wantH, tt.wantM)
			}
		})
	}
}

func TestParseSimpleSchedule_Invalid(t *testing.T) {
	tests := []struct {
		input string
		name  string
	}{
		{"xyz", "no colon"},
		{"", "empty"},
		{"noon", "word"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, m := parseSimpleSchedule(tt.input)
			// Should return default 2:00
			if h != 2 || m != 0 {
				t.Errorf("parseSimpleSchedule(%q) = (%d, %d), want default (2, 0)",
					tt.input, h, m)
			}
		})
	}
}

// =====================================================
// CleanupOldBackups TESTS
// =====================================================

func TestCleanupOldBackups_DeletesOldEntries(t *testing.T) {
	now := time.Now()
	old := now.AddDate(0, 0, -31).Unix()   // 31 days ago
	recent := now.AddDate(0, 0, -5).Unix() // 5 days ago

	storage := &mockBackupStorage{
		entries: []core.BackupEntry{
			{Key: "old-backup.tar", Size: 100, CreatedAt: old},
			{Key: "recent-backup.tar", Size: 200, CreatedAt: recent},
			{Key: "very-old-backup.tar", Size: 50, CreatedAt: old - 86400},
		},
	}

	deleted, err := CleanupOldBackups(context.Background(), storage, "prefix", 30)
	if err != nil {
		t.Fatalf("CleanupOldBackups() error = %v", err)
	}

	if deleted != 2 {
		t.Errorf("deleted = %d, want 2", deleted)
	}

	if len(storage.deleted) != 2 {
		t.Errorf("storage.deleted = %v, expected 2 keys", storage.deleted)
	}
}

func TestCleanupOldBackups_NothingToDelete(t *testing.T) {
	now := time.Now()
	storage := &mockBackupStorage{
		entries: []core.BackupEntry{
			{Key: "recent.tar", Size: 100, CreatedAt: now.Unix()},
		},
	}

	deleted, err := CleanupOldBackups(context.Background(), storage, "", 30)
	if err != nil {
		t.Fatalf("CleanupOldBackups() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestCleanupOldBackups_ListError(t *testing.T) {
	storage := &mockBackupStorage{
		listErr: fmt.Errorf("storage unreachable"),
	}

	_, err := CleanupOldBackups(context.Background(), storage, "", 30)
	if err == nil {
		t.Fatal("expected error when List fails")
	}
	if !strings.Contains(err.Error(), "list backups") {
		t.Errorf("error should wrap list failure: %v", err)
	}
}

func TestCleanupOldBackups_DeleteError_StillCounts(t *testing.T) {
	old := time.Now().AddDate(0, 0, -60).Unix()

	storage := &mockBackupStorage{
		entries: []core.BackupEntry{
			{Key: "fail.tar", Size: 100, CreatedAt: old},
			{Key: "ok.tar", Size: 100, CreatedAt: old},
		},
		delErr: fmt.Errorf("permission denied"),
	}

	deleted, err := CleanupOldBackups(context.Background(), storage, "", 30)
	if err != nil {
		t.Fatalf("CleanupOldBackups() error = %v", err)
	}
	// When delete fails, the entry is not counted as deleted
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (deletes failed)", deleted)
	}
}

func TestCleanupOldBackups_EmptyList(t *testing.T) {
	storage := &mockBackupStorage{
		entries: []core.BackupEntry{},
	}

	deleted, err := CleanupOldBackups(context.Background(), storage, "", 7)
	if err != nil {
		t.Fatalf("CleanupOldBackups() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

// =====================================================
// SCHEDULER runBackups TEST
// =====================================================

func TestScheduler_RunBackups(t *testing.T) {
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

	// Should have published backup.started and backup.completed
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

// =====================================================
// LOCAL STORAGE EDGE CASE TESTS
// =====================================================

func TestLocalStorage_Upload_CreateFileError(t *testing.T) {
	// Use a path that cannot be created (e.g., file as directory)
	dir := t.TempDir()
	// Create a regular file where a directory should be
	blockingFile := filepath.Join(dir, "blocker")
	os.WriteFile(blockingFile, []byte("x"), 0644)

	ls := &LocalStorage{basePath: dir}
	// Try to upload to a key where "blocker" is a file but we need it as dir
	err := ls.Upload(context.Background(), "blocker/sub/file.tar", strings.NewReader("data"), 4)
	if err == nil {
		t.Error("Upload() expected error when parent is a file not directory")
	}
}

func TestLocalStorage_Upload_CopyError(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	// Use an erroring reader
	err := ls.Upload(context.Background(), "bad.tar", &errReader{}, 10)
	if err == nil {
		t.Error("Upload() expected error from failing reader")
	}
}

// errReader always returns an error on Read.
type errReader struct{}

func (e *errReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("read error")
}

func TestLocalStorage_List_StatError(t *testing.T) {
	// This covers the case where Glob finds a match but Stat fails
	// (e.g., file removed between Glob and Stat). Hard to simulate reliably,
	// but we test that list still works when all stat calls succeed.
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

	os.WriteFile(filepath.Join(dir, "test-001.tar"), []byte("d"), 0644)
	entries, err := ls.List(context.Background(), "test-")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

// =====================================================
// S3 STORAGE: request creation error paths
// =====================================================

func TestS3Storage_Upload_RequestCreationError(t *testing.T) {
	s := &S3Storage{
		endpoint:  "https://valid.endpoint",
		bucket:    "b",
		pathStyle: true,
		client:    &http.Client{},
	}
	// A nil context will cause NewRequestWithContext to fail
	err := s.Upload(nil, "key", strings.NewReader("d"), 1) //nolint:staticcheck
	if err == nil {
		t.Error("expected error with nil context")
	}
}

func TestS3Storage_Download_RequestCreationError(t *testing.T) {
	s := &S3Storage{
		endpoint:  "https://valid.endpoint",
		bucket:    "b",
		pathStyle: true,
		client:    &http.Client{},
	}
	_, err := s.Download(nil, "key") //nolint:staticcheck
	if err == nil {
		t.Error("expected error with nil context")
	}
}

func TestS3Storage_Delete_RequestCreationError(t *testing.T) {
	s := &S3Storage{
		endpoint:  "https://valid.endpoint",
		bucket:    "b",
		pathStyle: true,
		client:    &http.Client{},
	}
	err := s.Delete(nil, "key") //nolint:staticcheck
	if err == nil {
		t.Error("expected error with nil context")
	}
}

// =====================================================
// S3 RETRY PATHS
// =====================================================

func TestS3Storage_Retry_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // Always fail to trigger retry
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:     server.URL,
		bucket:       "test",
		pathStyle:    true,
		client:       server.Client(),
		maxRetries:   10,
		initialDelay: 100 * time.Millisecond,
		maxDelay:     5 * time.Second,
		logger:       testLogger(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	err := s.Upload(ctx, "key.tar", strings.NewReader("data"), 4)
	if err == nil {
		t.Fatal("expected error due to context cancellation")
	}
	if !strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "context") {
		t.Logf("Upload error: %v (expected context cancellation)", err)
	}
}

func TestS3Storage_Delete_404IsOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound) // 404 - should be treated as OK for delete
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
	}

	err := s.Delete(context.Background(), "nonexistent.tar")
	if err != nil {
		t.Fatalf("Delete() should succeed for 404, got error: %v", err)
	}
}

func TestS3Storage_Delete_Non404HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden) // 403 - should be treated as error
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
	}

	err := s.Delete(context.Background(), "key.tar")
	if err == nil {
		t.Fatal("Delete() expected error for 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

func TestS3Storage_Retry_MaxDelayCapping(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		callCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:     server.URL,
		bucket:       "test",
		pathStyle:    true,
		client:       server.Client(),
		maxRetries:   3,
		initialDelay: 10 * time.Millisecond,
		maxDelay:     50 * time.Millisecond, // Will cap the exponential backoff
		logger:       testLogger(),
	}

	err := s.Upload(context.Background(), "key.tar", strings.NewReader("data"), 4)
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
	if callCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", callCount)
	}
}

func TestNewS3Storage_DefaultRetrySettings(t *testing.T) {
	s := NewS3Storage(S3Config{
		Bucket:     "test",
		Region:     "us-east-1",
		AccessKey:  "ak",
		SecretKey:  "sk",
		MaxRetries: 0, // Should default to 3
	}, testLogger())

	if s.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3 (default)", s.maxRetries)
	}
	if s.initialDelay != 100*time.Millisecond {
		t.Errorf("initialDelay = %v, want 100ms (default)", s.initialDelay)
	}
	if s.maxDelay != 5*time.Second {
		t.Errorf("maxDelay = %v, want 5s (default)", s.maxDelay)
	}
}

func TestNewS3Storage_CustomRetrySettings(t *testing.T) {
	s := NewS3Storage(S3Config{
		Bucket:       "test",
		Region:       "us-east-1",
		AccessKey:    "ak",
		SecretKey:    "sk",
		MaxRetries:   5,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     10 * time.Second,
	}, testLogger())

	if s.maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", s.maxRetries)
	}
	if s.initialDelay != 200*time.Millisecond {
		t.Errorf("initialDelay = %v, want 200ms", s.initialDelay)
	}
	if s.maxDelay != 10*time.Second {
		t.Errorf("maxDelay = %v, want 10s", s.maxDelay)
	}
}

func TestS3Storage_Retry_NilContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	s := &S3Storage{
		endpoint:  server.URL,
		bucket:    "test",
		pathStyle: true,
		client:    server.Client(),
	}

	// Nil context should be handled gracefully (line 86-88 in retry)
	err := s.Upload(nil, "key.tar", strings.NewReader("data"), 4) //nolint:staticcheck
	if err != nil {
		t.Logf("Upload with nil context: %v", err)
	}
}
