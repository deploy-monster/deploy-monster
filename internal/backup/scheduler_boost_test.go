package backup

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

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
