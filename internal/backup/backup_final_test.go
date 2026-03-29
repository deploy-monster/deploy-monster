package backup

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// LocalStorage.List — covers local.go:60 (the os.Stat error continue branch)
// The 85.7% means the `continue` on Stat error is not covered. We simulate
// this by creating a file and then removing it between Glob and Stat.
// Since this race is hard to trigger, we verify the existing paths work and
// test with a file that Stat can report on.
// ═══════════════════════════════════════════════════════════════════════════════

func TestLocalStorage_List_WithMultipleFiles(t *testing.T) {
	dir := t.TempDir()
	ls := NewLocalStorage(dir)

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
	ls := NewLocalStorage(dir)

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
	ls := NewLocalStorage(dir)

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

	s := NewScheduler(nil, storages, events, "02:00", testLogger())
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
	s := NewScheduler(store, storages, events, "02:00", testLogger())
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
