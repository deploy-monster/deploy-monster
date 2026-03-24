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
	entries  []core.BackupEntry
	errList  error
	errDown  error
	fileData string // data returned by Download
}

func (m *mockBackupStorage) Name() string { return "mock" }
func (m *mockBackupStorage) Upload(_ context.Context, _ string, _ io.Reader, _ int64) error {
	return nil
}
func (m *mockBackupStorage) Download(_ context.Context, key string) (io.ReadCloser, error) {
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
func (m *mockBackupStorage) List(_ context.Context, _ string) ([]core.BackupEntry, error) {
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
}

func TestBackupList_NilStorage(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, nil, events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/backups", nil)
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
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "failed to list backups")
}

// ─── Create ──────────────────────────────────────────────────────────────────

func TestBackupCreate_Success(t *testing.T) {
	store := newMockStore()
	storage := &mockBackupStorage{}
	events := core.NewEventBus(slog.Default())
	handler := NewBackupHandler(store, storage, events)

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

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "queued" {
		t.Errorf("expected status 'queued', got %q", resp["status"])
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
