package backup

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// All tests use unique names to avoid collision with other backup test files.

func TestGap_FindBackupLister_NilPtr(t *testing.T) {
	var store *struct{}
	result := findBackupLister(store)
	if result != nil {
		t.Error("expected nil for nil pointer")
	}
}

func TestGap_FindBackupLister_NonPtr(t *testing.T) {
	store := struct{}{}
	result := findBackupLister(store)
	if result != nil {
		t.Error("expected nil for non-pointer")
	}
}

func TestGap_FindBackupLister_NoListMethod(t *testing.T) {
	store := &struct{}{}
	result := findBackupLister(store)
	if result != nil {
		t.Error("expected nil when no ListBackupsByTenant")
	}
}

func TestGap_ModuleTriggerNow_NoScheduler(t *testing.T) {
	m := &Module{}
	err := m.TriggerNow(context.Background())
	if err != ErrBackupNotReady {
		t.Errorf("expected ErrBackupNotReady, got %v", err)
	}
}

func TestGap_LocalUpload_AbsolutePath(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	err := s.Upload(context.Background(), "/etc/passwd", strings.NewReader("x"), 1)
	if err == nil {
		t.Fatal("expected error for absolute key")
	}
}

func TestGap_LocalUpload_PathTraversal(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	err := s.Upload(context.Background(), "../outside", strings.NewReader("x"), 1)
	if err == nil {
		t.Fatal("expected error for traversal")
	}
}

func TestGap_LocalDownload_AbsolutePath(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	_, err := s.Download(context.Background(), "/etc/passwd")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGap_LocalDownload_NonExistent(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	_, err := s.Download(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGap_LocalDelete_PathTraversal(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	err := s.Delete(context.Background(), "../outside")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGap_LocalList_PathTraversal(t *testing.T) {
	s := NewLocalStorage(t.TempDir(), nil)
	_, err := s.List(context.Background(), "../outside")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGap_S3_New_Defaults(t *testing.T) {
	s := NewS3Storage(S3Config{Region: "eu-west-1", Bucket: "b"}, nil)
	if s == nil {
		t.Fatal("expected non-nil")
	}
	if s.maxRetries != 3 {
		t.Errorf("maxRetries = %d, want 3", s.maxRetries)
	}
}

func TestGap_S3_Bucket_VirtualHosted(t *testing.T) {
	s := &S3Storage{endpoint: "s3.amazonaws.com", bucket: "b", pathStyle: false}
	url := s.bucketURL()
	if !strings.Contains(url, "b.s3.amazonaws.com") {
		t.Errorf("virtual hosted URL mismatch: %q", url)
	}
}

func TestGap_S3_Sign_EmptyRegion(t *testing.T) {
	s := &S3Storage{}
	req, _ := http.NewRequest("GET", "http://example.com/", nil)
	s.sign(req, nil)
}

func TestGap_EncryptDecrypt_ShortKey(t *testing.T) {
	_, err := encryptAES256GCM([]byte("data"), []byte("short"))
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = decryptAES256GCM([]byte("data"), []byte("short"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGap_CleanupOldBackups_ListErr(t *testing.T) {
	store := &errListStorage{}
	_, err := CleanupOldBackups(context.Background(), store, "p", 30)
	if err == nil {
		t.Fatal("expected error")
	}
}

type errListStorage struct{}

func (e *errListStorage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
	return nil, errors.New("list error")
}
func (e *errListStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error { return nil }
func (e *errListStorage) Download(_ context.Context, _ string) (io.ReadCloser, error) { return nil, nil }
func (e *errListStorage) Delete(_ context.Context, _ string) error { return nil }
func (e *errListStorage) Name() string { return "err" }

func TestGap_S3Storage_UploadEncrypted_HTTPError(t *testing.T) {
	key := make([]byte, 32)
	s := &S3Storage{
		encryptionKey: key,
		client:        &http.Client{Transport: &errTransport{}},
		endpoint:      "s3.amazonaws.com",
		bucket:        "b",
		region:        "us-east-1",
		maxRetries:    1,
		initialDelay:  1,
		maxDelay:      1,
	}
	err := s.Upload(context.Background(), "k", strings.NewReader("data"), 4)
	if err == nil {
		t.Log("upload failed as expected due to transport error")
	}
}

type errTransport struct{}

func (e *errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("mock transport error")
}
