package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── DB Backup ───────────────────────────────────────────────────────────────

func TestDBBackup_Backup_Success(t *testing.T) {
	// Create a temporary file to act as the database.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("sqlite-data-here"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Driver: "sqlite",
				Path:   dbPath,
			},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
	}
	handler := NewDBBackupHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/backup", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Backup(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/octet-stream" {
		t.Errorf("expected Content-Type application/octet-stream, got %q", ct)
	}

	cd := rr.Header().Get("Content-Disposition")
	if cd == "" {
		t.Error("expected Content-Disposition header")
	}

	if rr.Body.String() != "sqlite-data-here" {
		t.Errorf("expected body 'sqlite-data-here', got %q", rr.Body.String())
	}
}

func TestDBBackup_Backup_NoClaims(t *testing.T) {
	c := testCore()
	handler := NewDBBackupHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/backup", nil)
	rr := httptest.NewRecorder()

	handler.Backup(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "super admin required")
}

func TestDBBackup_Backup_NonAdmin(t *testing.T) {
	c := testCore()
	handler := NewDBBackupHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/backup", nil)
	req = withClaims(req, "user1", "tenant1", "role_member", "user@test.com")
	rr := httptest.NewRecorder()

	handler.Backup(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "super admin required")
}

func TestDBBackup_Backup_FileNotAccessible(t *testing.T) {
	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Path: "/nonexistent/path/db.sqlite",
			},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
	}
	handler := NewDBBackupHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/backup", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Backup(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "database file not accessible")
}

func TestDBBackup_Status_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	if err := os.WriteFile(dbPath, []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}

	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Driver: "sqlite",
				Path:   dbPath,
			},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
	}
	handler := NewDBBackupHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/status", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Status(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["driver"] != "sqlite" {
		t.Errorf("expected driver=sqlite, got %v", resp["driver"])
	}
	if resp["path"] != dbPath {
		t.Errorf("expected path=%s, got %v", dbPath, resp["path"])
	}
	if _, ok := resp["size_mb"]; !ok {
		t.Error("expected size_mb field")
	}
	if _, ok := resp["modified"]; !ok {
		t.Error("expected modified field")
	}
}

func TestDBBackup_Status_NoClaims(t *testing.T) {
	c := testCore()
	handler := NewDBBackupHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/status", nil)
	rr := httptest.NewRecorder()

	handler.Status(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "super admin required")
}

func TestDBBackup_Status_NonAdmin(t *testing.T) {
	c := testCore()
	handler := NewDBBackupHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/status", nil)
	req = withClaims(req, "user1", "tenant1", "role_developer", "dev@test.com")
	rr := httptest.NewRecorder()

	handler.Status(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "super admin required")
}

func TestDBBackup_Status_DBNotAccessible(t *testing.T) {
	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{
				Path: "/nonexistent/path/db.sqlite",
			},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
	}
	handler := NewDBBackupHandler(c)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/db/status", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Status(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "database not accessible")
}
