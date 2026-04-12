package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── List Announcements ─────────────────────────────────────────────────────

func TestAnnouncements_List_Empty(t *testing.T) {
	handler := NewAnnouncementHandler(newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/announcements", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

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
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

func TestAnnouncements_List_AfterCreate(t *testing.T) {
	handler := NewAnnouncementHandler(newMockBoltStore())

	// Create an announcement first.
	body, _ := json.Marshal(Announcement{
		Title: "Scheduled Maintenance",
		Body:  "Platform will be down for maintenance.",
		Type:  "maintenance",
	})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", bytes.NewReader(body))
	createReq = withClaims(createReq, "user1", "tenant1", "role_super_admin", "admin@test.com")
	createRR := httptest.NewRecorder()
	handler.Create(createRR, createReq)

	if createRR.Code != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d: %s", createRR.Code, createRR.Body.String())
	}

	// Now list.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/announcements", nil)
	listRR := httptest.NewRecorder()
	handler.List(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("list: expected 200, got %d", listRR.Code)
	}

	var resp map[string]any
	json.Unmarshal(listRR.Body.Bytes(), &resp)

	if int(resp["total"].(float64)) != 1 {
		t.Errorf("expected total=1, got %v", resp["total"])
	}

	data := resp["data"].([]any)
	ann := data[0].(map[string]any)
	if ann["title"] != "Scheduled Maintenance" {
		t.Errorf("expected title=Scheduled Maintenance, got %v", ann["title"])
	}
	if ann["active"] != true {
		t.Errorf("expected active=true, got %v", ann["active"])
	}
}

// ─── Create Announcement ────────────────────────────────────────────────────

func TestAnnouncements_Create_Success(t *testing.T) {
	handler := NewAnnouncementHandler(newMockBoltStore())

	body, _ := json.Marshal(Announcement{
		Title: "New Feature",
		Body:  "We launched a new feature!",
		Type:  "info",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var ann Announcement
	json.Unmarshal(rr.Body.Bytes(), &ann)

	if ann.Title != "New Feature" {
		t.Errorf("expected title=New Feature, got %q", ann.Title)
	}
	if ann.ID == "" {
		t.Error("expected non-empty ID")
	}
	if !ann.Active {
		t.Error("expected active=true")
	}
	if ann.CreatedAt.IsZero() {
		t.Error("expected non-zero created_at")
	}
}

func TestAnnouncements_Create_InvalidJSON(t *testing.T) {
	handler := NewAnnouncementHandler(newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", bytes.NewReader([]byte("{")))
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

// ─── Dismiss Announcement ───────────────────────────────────────────────────

func TestAnnouncements_Dismiss_Success(t *testing.T) {
	handler := NewAnnouncementHandler(newMockBoltStore())

	// Create one first.
	body, _ := json.Marshal(Announcement{Title: "To Dismiss", Type: "info"})
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", bytes.NewReader(body))
	createReq = withClaims(createReq, "user1", "tenant1", "role_super_admin", "admin@test.com")
	createRR := httptest.NewRecorder()
	handler.Create(createRR, createReq)

	var created Announcement
	json.Unmarshal(createRR.Body.Bytes(), &created)

	// Dismiss it.
	dismissReq := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/announcements/"+created.ID, nil)
	dismissReq.SetPathValue("id", created.ID)
	dismissRR := httptest.NewRecorder()

	handler.Dismiss(dismissRR, dismissReq)

	if dismissRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", dismissRR.Code)
	}

	// Verify it no longer shows in active list.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/announcements", nil)
	listRR := httptest.NewRecorder()
	handler.List(listRR, listReq)

	var resp map[string]any
	json.Unmarshal(listRR.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total=0 after dismiss, got %v", resp["total"])
	}
}

func TestAnnouncements_Dismiss_NonexistentID(t *testing.T) {
	handler := NewAnnouncementHandler(newMockBoltStore())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/announcements/nonexistent", nil)
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Dismiss(rr, req)

	// Handler returns 204 even for nonexistent IDs (idempotent).
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}
