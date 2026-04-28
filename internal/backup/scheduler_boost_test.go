package backup

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// errStore is a mock store that fails UpdateBackupStatus.
type errStore struct {
	mockStore
}

func (e *errStore) UpdateBackupStatus(_ context.Context, _, _ string, _ int64) error {
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
	s.markFailed(ctx, "backup-1", "test failure", errors.New("cause"))
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
	s.markFailed(ctx, "backup-1", "test failure", errors.New("cause"))
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

func TestScheduler_Closed_NilCtx(t *testing.T) {
	s := &Scheduler{logger: testLogger()}
	if s.Closed() {
		t.Error("expected Closed=false when stopCtx is nil")
	}
}
