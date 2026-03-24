package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Get Build Log ───────────────────────────────────────────────────────────

func TestBuildLog_Get_Success(t *testing.T) {
	store := newMockStore()
	store.latestDeployments["app12345"] = &core.Deployment{
		ID:       "dep1",
		AppID:    "app12345",
		Version:  5,
		Status:   "success",
		BuildLog: "Step 1/5: FROM golang:1.22\nStep 2/5: COPY . .\n",
	}

	handler := NewBuildLogHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app12345/builds/5/log", nil)
	req.SetPathValue("id", "app12345")
	req.SetPathValue("version", "5")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app12345" {
		t.Errorf("expected app_id=app12345, got %v", resp["app_id"])
	}
	if int(resp["version"].(float64)) != 5 {
		t.Errorf("expected version=5, got %v", resp["version"])
	}
	if resp["status"] != "success" {
		t.Errorf("expected status=success, got %v", resp["status"])
	}
	if resp["log"] == nil || resp["log"] == "" {
		t.Error("expected non-empty log content")
	}
}

func TestBuildLog_Get_NotFound(t *testing.T) {
	store := newMockStore()
	handler := NewBuildLogHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/nonexistent/builds/1/log", nil)
	req.SetPathValue("id", "nonexistent")
	req.SetPathValue("version", "1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "no deployment found")
}

// ─── Download Build Log ──────────────────────────────────────────────────────

func TestBuildLog_Download_WithLog(t *testing.T) {
	store := newMockStore()
	store.latestDeployments["app12345"] = &core.Deployment{
		ID:       "dep1",
		AppID:    "app12345",
		Version:  3,
		Status:   "success",
		BuildLog: "BUILD LOG CONTENT HERE\n",
	}

	handler := NewBuildLogHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app12345/builds/3/log/download", nil)
	req.SetPathValue("id", "app12345")
	req.SetPathValue("version", "3")
	rr := httptest.NewRecorder()

	handler.Download(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("expected Content-Type text/plain, got %q", ct)
	}

	cd := rr.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") {
		t.Errorf("expected Content-Disposition attachment, got %q", cd)
	}
	if !strings.Contains(cd, "app12345") {
		t.Errorf("expected filename to contain app ID, got %q", cd)
	}

	body := rr.Body.String()
	if body != "BUILD LOG CONTENT HERE\n" {
		t.Errorf("expected build log content, got %q", body)
	}
}

func TestBuildLog_Download_EmptyLog(t *testing.T) {
	store := newMockStore()
	store.latestDeployments["app12345"] = &core.Deployment{
		ID:       "dep1",
		AppID:    "app12345",
		Version:  1,
		Status:   "success",
		BuildLog: "",
	}

	handler := NewBuildLogHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app12345/builds/1/log/download", nil)
	req.SetPathValue("id", "app12345")
	req.SetPathValue("version", "1")
	rr := httptest.NewRecorder()

	handler.Download(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "No build log available") {
		t.Errorf("expected fallback message, got %q", body)
	}
}

func TestBuildLog_Download_NotFound(t *testing.T) {
	store := newMockStore()
	handler := NewBuildLogHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/nonexistent/builds/1/log/download", nil)
	req.SetPathValue("id", "nonexistent")
	req.SetPathValue("version", "1")
	rr := httptest.NewRecorder()

	handler.Download(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "no deployment found")
}
