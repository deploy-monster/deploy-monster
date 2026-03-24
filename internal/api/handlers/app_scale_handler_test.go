package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ─── Scale App ───────────────────────────────────────────────────────────────

func TestScale_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		Name:     "Web App",
		Replicas: 1,
		Status:   "running",
	})

	events := testCore().Events
	handler := NewScaleHandler(store, events)

	body, _ := json.Marshal(scaleRequest{Replicas: 5})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/scale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Scale(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["app_id"] != "app1" {
		t.Errorf("expected app_id=app1, got %v", resp["app_id"])
	}
	if int(resp["old_replicas"].(float64)) != 1 {
		t.Errorf("expected old_replicas=1, got %v", resp["old_replicas"])
	}
	if int(resp["new_replicas"].(float64)) != 5 {
		t.Errorf("expected new_replicas=5, got %v", resp["new_replicas"])
	}

	// Verify the app was updated in the store.
	if store.updatedApp == nil {
		t.Fatal("expected app to be updated in store")
	}
	if store.updatedApp.Replicas != 5 {
		t.Errorf("expected stored replicas=5, got %d", store.updatedApp.Replicas)
	}
}

func TestScale_ScaleToZero(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		Name:     "Idle App",
		Replicas: 3,
		Status:   "running",
	})

	handler := NewScaleHandler(store, testCore().Events)

	body, _ := json.Marshal(scaleRequest{Replicas: 0})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/scale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Scale(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if int(resp["new_replicas"].(float64)) != 0 {
		t.Errorf("expected new_replicas=0, got %v", resp["new_replicas"])
	}
}

func TestScale_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewScaleHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/scale", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Scale(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestScale_NegativeReplicas(t *testing.T) {
	store := newMockStore()
	handler := NewScaleHandler(store, testCore().Events)

	body, _ := json.Marshal(scaleRequest{Replicas: -1})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/scale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Scale(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "replicas must be between 0 and 100")
}

func TestScale_TooManyReplicas(t *testing.T) {
	store := newMockStore()
	handler := NewScaleHandler(store, testCore().Events)

	body, _ := json.Marshal(scaleRequest{Replicas: 101})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/scale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Scale(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "replicas must be between 0 and 100")
}

func TestScale_MaxReplicas(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "Big App", Replicas: 1, Status: "running"})

	handler := NewScaleHandler(store, testCore().Events)

	body, _ := json.Marshal(scaleRequest{Replicas: 100})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/scale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Scale(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestScale_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewScaleHandler(store, testCore().Events)

	body, _ := json.Marshal(scaleRequest{Replicas: 3})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/nonexistent/scale", bytes.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	rr := httptest.NewRecorder()

	handler.Scale(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "app not found")
}

func TestScale_UpdateError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", Name: "Fail App", Replicas: 1, Status: "running"})
	store.errUpdateApp = errors.New("db error")

	handler := NewScaleHandler(store, testCore().Events)

	body, _ := json.Marshal(scaleRequest{Replicas: 5})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/scale", bytes.NewReader(body))
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Scale(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "scale failed")
}
