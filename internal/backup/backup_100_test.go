package backup

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Module.Init — invalid encryption key (base64 decode error)
// ---------------------------------------------------------------------------

func TestInit_InvalidEncryptionKey(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config:   &core.Config{},
		Services: core.NewServices(),
	}
	c.Config.Backup.EncryptionKey = "!!!invalid-base64!!!"
	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("expected error for invalid base64 encryption key")
	}
}

// ---------------------------------------------------------------------------
// Module.Init — with valid encryption key (decodes base64)
// ---------------------------------------------------------------------------

func TestInit_WithEncryptionKey(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Config:   &core.Config{},
		Services: core.NewServices(),
	}
	// "aGVsbG8=" is base64("hello") — valid base64, though key is too short
	c.Config.Backup.EncryptionKey = "aGVsbG8="
	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init with valid key: %v", err)
	}
}

// ---------------------------------------------------------------------------
// findBackupLister — store with ListBackupsByTenant returns valid lister
// ---------------------------------------------------------------------------

type testBackupLister struct{}

func (t *testBackupLister) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	return nil, 0, nil
}

func TestFindBackupLister_Success(t *testing.T) {
	result := findBackupLister(&testBackupLister{})
	if result == nil {
		t.Fatal("expected non-nil lister")
	}
	backups, total, err := result(context.Background(), "t1", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("backups = %v, want empty", backups)
	}
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
}

// ---------------------------------------------------------------------------
// findBackupLister — anonymous field walking (struct embedding)
// ---------------------------------------------------------------------------

type embeddedStore struct {
	*testBackupLister
}

func TestFindBackupLister_EmbeddedLister(t *testing.T) {
	result := findBackupLister(&embeddedStore{testBackupLister: &testBackupLister{}})
	if result == nil {
		t.Fatal("expected non-nil lister for embedded store")
	}
}

// ---------------------------------------------------------------------------
// findBackupLister — panic recovery when embedded lister panics
// ---------------------------------------------------------------------------

type panickingLister struct{}

func (p *panickingLister) ListBackupsByTenant(_ context.Context, _ string, _, _ int) ([]core.Backup, int, error) {
	panic("unexpected panic")
}

type storeWithPanic struct {
	*panickingLister
}

func TestFindBackupLister_PanicRecovery(t *testing.T) {
	result := findBackupLister(&storeWithPanic{panickingLister: &panickingLister{}})
	if result == nil {
		t.Fatal("expected non-nil lister")
	}
	_, _, err := result(context.Background(), "t1", 10, 0)
	if err == nil {
		t.Fatal("expected error from panic recovery")
	}
}

// ---------------------------------------------------------------------------
// TriggerNow with valid scheduler — success path
// ---------------------------------------------------------------------------

func TestTriggerNow_WithScheduler(t *testing.T) {
	sched := NewScheduler(nil, nil, nil, nil, "", nil)
	m := &Module{
		scheduler: sched,
	}
	err := m.TriggerNow(context.Background())
	if err != nil {
		t.Fatalf("TriggerNow: %v", err)
	}
}

// NOTE: The init() closure body (module.go:20) is only called by
// core.RegisterModule during module registration, not during unit tests.
