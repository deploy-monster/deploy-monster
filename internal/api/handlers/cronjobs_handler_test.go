package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── List Cron Jobs ──────────────────────────────────────────────────────────

func TestCronJobs_List_Success(t *testing.T) {
	store := newMockStore()
	handler := NewCronJobHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/cron", nil)
	req.SetPathValue("id", "app1")
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
		t.Errorf("expected empty data array, got %d items", len(data))
	}
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

// ─── Create Cron Job ─────────────────────────────────────────────────────────

func TestCronJobs_Create_Success(t *testing.T) {
	store := newMockStore()
	handler := NewCronJobHandler(store)

	body, _ := json.Marshal(CronJobConfig{
		Name:     "db-backup",
		Schedule: "0 2 * * *",
		Command:  "pg_dump -U postgres mydb > /backups/dump.sql",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/cron", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}

	job, ok := resp["job"].(map[string]any)
	if !ok {
		t.Fatal("expected job object in response")
	}
	if job["schedule"] != "0 2 * * *" {
		t.Errorf("expected schedule='0 2 * * *', got %v", job["schedule"])
	}
	if job["enabled"] != true {
		t.Errorf("expected enabled=true, got %v", job["enabled"])
	}
	if job["id"] == nil || job["id"] == "" {
		t.Error("expected non-empty job ID")
	}
}

func TestCronJobs_Create_MissingSchedule(t *testing.T) {
	store := newMockStore()
	handler := NewCronJobHandler(store)

	body, _ := json.Marshal(CronJobConfig{
		Command: "echo hello",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/cron", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "schedule and command are required")
}

func TestCronJobs_Create_MissingCommand(t *testing.T) {
	store := newMockStore()
	handler := NewCronJobHandler(store)

	body, _ := json.Marshal(CronJobConfig{
		Schedule: "0 * * * *",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/cron", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "schedule and command are required")
}

func TestCronJobs_Create_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewCronJobHandler(store)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/cron", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

// ─── Delete Cron Job ─────────────────────────────────────────────────────────

func TestCronJobs_Delete_Success(t *testing.T) {
	store := newMockStore()
	handler := NewCronJobHandler(store)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1/cron/job1", nil)
	req.SetPathValue("id", "app1")
	req.SetPathValue("jobId", "job1")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rr.Code)
	}
}
