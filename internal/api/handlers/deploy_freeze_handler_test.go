package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ─── Deploy Freeze ───────────────────────────────────────────────────────────

func TestDeployFreeze_Get_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployFreezeHandler(store, events, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/deploy/freeze", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatal("expected data array in response")
	}
	if len(data) != 0 {
		t.Errorf("expected empty data, got %d items", len(data))
	}
	if resp["frozen"] != false {
		t.Errorf("expected frozen=false, got %v", resp["frozen"])
	}
}

func TestDeployFreeze_Create_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployFreezeHandler(store, events, newMockBoltStore())

	startsAt := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	endsAt := time.Now().Add(24 * time.Hour).Format(time.RFC3339)
	body, _ := json.Marshal(map[string]string{
		"reason":    "holiday freeze",
		"starts_at": startsAt,
		"ends_at":   endsAt,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/freeze", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp FreezeWindow
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.ID == "" {
		t.Error("expected non-empty freeze ID")
	}
	if resp.Reason != "holiday freeze" {
		t.Errorf("expected reason='holiday freeze', got %q", resp.Reason)
	}
	if !resp.Active {
		t.Error("expected active=true")
	}
	if resp.StartsAt.IsZero() {
		t.Error("expected non-zero starts_at")
	}
	if resp.EndsAt.IsZero() {
		t.Error("expected non-zero ends_at")
	}
}

func TestDeployFreeze_Create_DefaultTimes(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployFreezeHandler(store, events, newMockBoltStore())

	body, _ := json.Marshal(map[string]string{
		"reason": "emergency freeze",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/freeze", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp FreezeWindow
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.StartsAt.IsZero() {
		t.Error("expected starts_at to default to now")
	}
	if resp.EndsAt.IsZero() {
		t.Error("expected ends_at to default to starts_at + 24h")
	}
	// Verify ends_at is approximately 24h after starts_at.
	diff := resp.EndsAt.Sub(resp.StartsAt)
	if diff < 23*time.Hour || diff > 25*time.Hour {
		t.Errorf("expected ends_at ~24h after starts_at, got %v", diff)
	}
}

func TestDeployFreeze_Create_InvalidJSON(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployFreezeHandler(store, events, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/deploy/freeze", bytes.NewReader([]byte("{")))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestDeployFreeze_Delete_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployFreezeHandler(store, events, newMockBoltStore())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/deploy/freeze/freeze1", nil)
	req.SetPathValue("id", "freeze1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}
