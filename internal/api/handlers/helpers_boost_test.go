package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequirePathParam_Success(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/apps/{id}", nil)
	req.SetPathValue("id", "app-123")
	w := httptest.NewRecorder()

	val, ok := requirePathParam(w, req, "id")
	if !ok {
		t.Error("expected ok=true")
	}
	if val != "app-123" {
		t.Errorf("val = %q, want app-123", val)
	}
}

func TestRequirePathParam_Missing(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/apps/{id}", nil)
	w := httptest.NewRecorder()

	val, ok := requirePathParam(w, req, "id")
	if ok {
		t.Error("expected ok=false")
	}
	if val != "" {
		t.Errorf("val = %q, want empty", val)
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", w.Code)
	}
}
