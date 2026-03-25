package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// ─── Deploy Schedule ─────────────────────────────────────────────────────────

func TestDeploySchedule_Schedule_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployScheduleHandler(store, events, newMockBoltStore())

	futureTime := time.Now().Add(2 * time.Hour).Format(time.RFC3339)
	body, _ := json.Marshal(map[string]string{
		"scheduled_at": futureTime,
		"image":        "myapp:v3",
		"branch":       "release/1.0",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy/schedule", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Schedule(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp ScheduledDeploy
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.ID == "" {
		t.Error("expected non-empty schedule ID")
	}
	if resp.AppID != "app1" {
		t.Errorf("expected app_id=app1, got %q", resp.AppID)
	}
	if resp.Image != "myapp:v3" {
		t.Errorf("expected image=myapp:v3, got %q", resp.Image)
	}
	if resp.Branch != "release/1.0" {
		t.Errorf("expected branch=release/1.0, got %q", resp.Branch)
	}
	if resp.Strategy != "recreate" {
		t.Errorf("expected strategy=recreate, got %q", resp.Strategy)
	}
	if resp.Status != "pending" {
		t.Errorf("expected status=pending, got %q", resp.Status)
	}
	if resp.ScheduledAt.IsZero() {
		t.Error("expected non-zero scheduled_at")
	}
}

func TestDeploySchedule_Schedule_InvalidJSON(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployScheduleHandler(store, events, newMockBoltStore())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy/schedule", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Schedule(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestDeploySchedule_Schedule_InvalidTimeFormat(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployScheduleHandler(store, events, newMockBoltStore())

	body, _ := json.Marshal(map[string]string{
		"scheduled_at": "not-a-date",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy/schedule", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Schedule(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "scheduled_at must be RFC3339 format")
}

func TestDeploySchedule_Schedule_PastTime(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployScheduleHandler(store, events, newMockBoltStore())

	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	body, _ := json.Marshal(map[string]string{
		"scheduled_at": pastTime,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy/schedule", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Schedule(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "scheduled_at must be in the future")
}

func TestDeploySchedule_ListScheduled_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployScheduleHandler(store, events, newMockBoltStore())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/deploy/scheduled", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.ListScheduled(rr, req)

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

func TestDeploySchedule_CancelScheduled_Success(t *testing.T) {
	store := newMockStore()
	events := testCore().Events
	handler := NewDeployScheduleHandler(store, events, newMockBoltStore())

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1/deploy/scheduled/sched1", nil)
	req.SetPathValue("id", "app1")
	req.SetPathValue("scheduleId", "sched1")
	rr := httptest.NewRecorder()

	handler.CancelScheduled(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}
