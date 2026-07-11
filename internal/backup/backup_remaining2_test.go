package backup

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

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
