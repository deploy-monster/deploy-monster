package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Mock BackupStorage ──────────────────────────────────────────────────────

type mockBackupStorage struct {
	entries         []core.BackupEntry
	errList         error
	errDown         error
	fileData        string // data returned by Download
	lastListPrefix  string
	lastDownloadKey string
}

func (m *mockBackupStorage) Name() string { return "mock" }
func (m *mockBackupStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}
func (m *mockBackupStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
	m.lastDownloadKey = key
	if m.errDown != nil {
		return nil, m.errDown
	}
	data := m.fileData
	if data == "" {
		data = "backup-data-for-" + key
	}
	return io.NopCloser(strings.NewReader(data)), nil
}
func (m *mockBackupStorage) Delete(_ context.Context, _ string) error { return nil }
func (m *mockBackupStorage) List(_ context.Context, prefix string) ([]core.BackupEntry, error) {
	m.lastListPrefix = prefix
	if m.errList != nil {
		return nil, m.errList
	}
	return m.entries, nil
}

// ─── List ────────────────────────────────────────────────────────────────────

func TestBackupList_Success(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{
		entries: []core.BackupEntry{
			{Key: "backup-001.tar.gz", Size: 1024, CreatedAt: 1700000000},
			{Key: "backup-002.tar.gz", Size: 2048, CreatedAt: 1700001000},
		},
	}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 2 {
		t.Errorf("expected 2 backups, got %d", len(data))
	}

	total, _ := resp["total"].(float64)
	if int(total) != 2 {
		t.Errorf("expected total 2, got %d", int(total))
	}
	if storage.lastListPrefix != "tenant1/" {
		t.Errorf("list prefix = %q, want tenant1/", storage.lastListPrefix)
	}
}

func TestBackupList_NilStorage(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	total, _ := resp["total"].(float64)
	if int(total) != 0 {
		t.Errorf("expected total 0 when storage is nil, got %d", int(total))
	}
}

func TestBackupList_StorageError(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{
		errList: io.ErrUnexpectedEOF,
	}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to list backups")
}

// ─── Create ──────────────────────────────────────────────────────────────────

type fakeBackupTrigger struct{ called bool }

func (f *fakeBackupTrigger) TriggerNow(_ context.Context) error {
	f.called = true
	return nil
}

func TestBackupCreate_Success(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)
	trigger := &fakeBackupTrigger{}
	handler.SetTrigger(trigger)

	body, _ := json.Marshal(map[string]string{
		"source_type": "volume",
		"source_id":   "vol-123",
		"storage":     "local",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	if !trigger.called {
		t.Error("expected TriggerNow to be invoked")
	}
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "running" {
		t.Errorf("expected status 'running', got %q", resp["status"])
	}
}

func TestBackupCreate_NoTrigger(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	body, _ := json.Marshal(map[string]string{"source_type": "volume", "source_id": "vol-1"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 when no trigger wired, got %d", rr.Code)
	}
}

func TestBackupCreate_NoClaims(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	body, _ := json.Marshal(map[string]string{
		"source_type": "volume",
		"source_id":   "vol-123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "unauthorized")
}

func TestBackupCreate_InvalidJSON(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups", bytes.NewReader([]byte("not json")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestBackupRestore_Success(t *testing.T) {
	store := newMockStore()
	// Provide valid JSON that matches core.Application
	appJSON := `{"id":"app-orig-001","tenant_id":"tenant1","name":"my-app","type":"docker","status":"running","replicas":1}`
	storage := &mockBackupStorage{fileData: appJSON}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/tenant1/app1/backup-001.tar.gz/restore", nil)
	req.SetPathValue("key", "tenant1/app1/backup-001.tar.gz")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restore(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "restored" {
		t.Errorf("expected status 'restored', got %q", resp["status"])
	}
	if storage.lastDownloadKey != "tenant1/app1/backup-001.tar.gz" {
		t.Errorf("download key = %q, want tenant1/app1/backup-001.tar.gz", storage.lastDownloadKey)
	}
}

func TestBackupRestore_CrossTenantKey(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{fileData: `{"id":"app-orig-001","tenant_id":"tenant2","name":"my-app","type":"docker","status":"running","replicas":1}`}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backups/tenant2/app1/backup-001.tar.gz/restore", nil)
	req.SetPathValue("key", "tenant2/app1/backup-001.tar.gz")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Restore(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	if storage.lastDownloadKey != "" {
		t.Fatalf("cross-tenant restore reached storage with key %q", storage.lastDownloadKey)
	}
}
