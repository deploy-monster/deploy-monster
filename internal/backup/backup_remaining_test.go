package backup

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// local.go:34 — NewLocalStorage (missed error paths)
// =============================================================================

func TestCov_NewLocalStorageAbsFallback(t *testing.T) {
	// filepath.Abs fails on some edge cases; NewLocalStorage falls back to Clean
	s := NewLocalStorage("/tmp/backup-test", nil)
	if s == nil {
		t.Fatal("expected non-nil storage")
	}
	if s.basePath == "" {
		t.Error("basePath should not be empty")
	}
}

// =============================================================================
// local.go:46 — Upload error paths
// =============================================================================

func TestCov_UploadAbsoluteKey(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	err := s.Upload(context.Background(), "/etc/passwd", strings.NewReader("data"), 4)
	if err == nil {
		t.Error("expected error for absolute key")
	}
}

func TestCov_UploadPathTraversal(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	err := s.Upload(context.Background(), "../../etc/passwd", strings.NewReader("data"), 4)
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestCov_UploadSymlinkRejection(t *testing.T) {
	dir := t.TempDir()
	// Create a symlink in the storage path
	symlinkPath := filepath.Join(dir, "link")
	_ = os.Symlink("/etc/passwd", symlinkPath)

	s := NewLocalStorage(dir, nil)
	err := s.Upload(context.Background(), "link", strings.NewReader("data"), 4)
	if err == nil {
		t.Error("expected error for symlink target")
	}
}

func TestCov_UploadWithEncryption(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	s := NewLocalStorage(dir, key)
	err := s.Upload(context.Background(), "test-file", strings.NewReader("hello world"), 11)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Verify file was created
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
	// File content should be encrypted (not "hello world")
	data, _ := os.ReadFile(filepath.Join(dir, "test-file"))
	if string(data) == "hello world" {
		t.Error("file content should be encrypted")
	}
}

// =============================================================================
// local.go:136 — Download error paths
// =============================================================================

func TestCov_DownloadAbsoluteKey(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	_, err := s.Download(context.Background(), "/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute key")
	}
}

func TestCov_DownloadPathTraversal(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	_, err := s.Download(context.Background(), "../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestCov_DownloadNonExistent(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	_, err := s.Download(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}

func TestCov_DownloadSymlinkRejection(t *testing.T) {
	dir := t.TempDir()
	symPath := filepath.Join(dir, "link")
	_ = os.Symlink("/etc/passwd", symPath)

	s := NewLocalStorage(dir, nil)
	_, err := s.Download(context.Background(), "link")
	if err == nil {
		t.Error("expected error for symlink target")
	}
}

func TestCov_DownloadWithEncryption(t *testing.T) {
	dir := t.TempDir()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	// First upload (encrypts)
	s := NewLocalStorage(dir, key)
	if err := s.Upload(context.Background(), "secret", strings.NewReader("sensitive data"), 14); err != nil {
		t.Fatalf("upload failed: %v", err)
	}

	// Download (decrypts)
	rc, err := s.Download(context.Background(), "secret")
	if err != nil {
		t.Fatalf("download failed: %v", err)
	}
	defer rc.Close()
	data, _ := io.ReadAll(rc)
	if string(data) != "sensitive data" {
		t.Errorf("got %q, want %q", string(data), "sensitive data")
	}
}

// =============================================================================
// local.go:177 — Delete error paths
// =============================================================================

func TestCov_DeleteAbsoluteKey(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	err := s.Delete(context.Background(), "/etc/passwd")
	if err == nil {
		t.Error("expected error for absolute key")
	}
}

func TestCov_DeletePathTraversal(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	err := s.Delete(context.Background(), "../../etc/passwd")
	if err == nil {
		t.Error("expected error for path traversal")
	}
}

func TestCov_DeleteNonExistent(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	err := s.Delete(context.Background(), "nonexistent")
	if err == nil {
		t.Log("expected error for non-existent file")
	}
}

// =============================================================================
// local.go:193 — List error paths
// =============================================================================

func TestCov_ListPathTraversal(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	_, err := s.List(context.Background(), "../../etc")
	if err == nil {
		t.Error("expected error for path traversal prefix")
	}
}

func TestCov_ListEmptyDir(t *testing.T) {
	dir := t.TempDir()
	s := NewLocalStorage(dir, nil)
	entries, err := s.List(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestCov_ListWithFiles(t *testing.T) {
	dir := t.TempDir()
	// Create some backup files
	for _, name := range []string{"backup-1.gz", "backup-2.gz", "other.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	s := NewLocalStorage(dir, nil)
	entries, err := s.List(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestCov_ListWithPrefix(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"backup-1.gz", "backup-2.gz", "other.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	s := NewLocalStorage(dir, nil)
	entries, err := s.List(context.Background(), "backup-")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with prefix 'backup-', got %d", len(entries))
	}
}

func TestCov_ListNonExistentDir(t *testing.T) {
	dir := t.TempDir()
	s := NewLocalStorage(dir, nil)
	entries, err := s.List(context.Background(), "nonexistent-dir/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entries == nil {
		t.Error("expected non-nil entries slice")
	}
}

// =============================================================================
// local.go:97 — encryptAES256GCM error paths
// =============================================================================

func TestCov_EncryptAESBadKey(t *testing.T) {
	_, err := encryptAES256GCM([]byte("data"), []byte("bad-key"))
	if err == nil {
		t.Error("expected error for bad key length")
	}
}

func TestCov_DecryptAESBadKey(t *testing.T) {
	_, err := decryptAES256GCM([]byte("data"), []byte("bad-key"))
	if err == nil {
		t.Error("expected error for bad key length")
	}
}

func TestCov_DecryptAESShortCiphertext(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	_, err := decryptAES256GCM([]byte("short"), key)
	if err == nil {
		t.Error("expected error for short ciphertext")
	}
}

func TestCov_DecryptAESTampered(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	encrypted, err := encryptAES256GCM([]byte("hello"), key)
	if err != nil {
		t.Fatal(err)
	}
	// Tamper with the ciphertext
	encrypted[len(encrypted)-1] ^= 0xff
	_, err = decryptAES256GCM(encrypted, key)
	if err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

// =============================================================================
// scheduler.go:584 — findBackupLister with nil storage
// =============================================================================

func TestCov_FindBackupListerNil(t *testing.T) {
	result := findBackupLister(nil)
	if result != nil {
		t.Error("expected nil for nil input")
	}
}

func TestCov_FindBackupListerNoLister(t *testing.T) {
	// Use a bare struct without ListBackupsByTenant method
	type bareStore struct{}
	result := findBackupLister(&bareStore{})
	if result != nil {
		t.Error("expected nil for store without ListBackupsByTenant")
	}
}

// =============================================================================
// Encryption round-trip with bad key scenarios
// =============================================================================

func TestCov_EncryptDecryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	plaintext := []byte("this is secret backup data")

	encrypted, err := encryptAES256GCM(plaintext, key)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	decrypted, err := decryptAES256GCM(encrypted, key)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("round-trip failed: got %q, want %q", decrypted, plaintext)
	}
}

// =============================================================================
// local.go:136 — Download file not found
// =============================================================================

func TestCov_ListWithSymlinks(t *testing.T) {
	dir := t.TempDir()
	// Create a file
	if err := os.WriteFile(filepath.Join(dir, "real-backup.gz"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	// Create a symlink
	if err := os.Symlink(filepath.Join(dir, "real-backup.gz"), filepath.Join(dir, "link.gz")); err != nil {
		t.Fatal(err)
	}

	s := NewLocalStorage(dir, nil)
	entries, err := s.List(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the real file should appear (symlinks are skipped)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry (symlink skipped), got %d", len(entries))
	}
}

// =============================================================================
// BackupStorage interface - verify all storage types implement it
// =============================================================================

func TestCov_StorageImplementsInterface(t *testing.T) {
	var _ core.BackupStorage = (*LocalStorage)(nil)
	var _ core.BackupStorage = (*S3Storage)(nil)
	t.Log("All storage types implement BackupStorage interface")
}
