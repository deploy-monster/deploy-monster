package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── Build Cache ─────────────────────────────────────────────────────────────

func TestBuildCache_Stats_Success(t *testing.T) {
	runtime := &mockContainerRuntime{}
	handler := NewBuildCacheHandler(runtime)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()

	handler.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if _, ok := resp["layers"]; !ok {
		t.Error("expected 'layers' field in response")
	}
	if _, ok := resp["size_mb"]; !ok {
		t.Error("expected 'size_mb' field in response")
	}
	if _, ok := resp["reclaimable_mb"]; !ok {
		t.Error("expected 'reclaimable_mb' field in response")
	}
}

func TestBuildCache_Stats_NilRuntime(t *testing.T) {
	handler := NewBuildCacheHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()

	handler.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBuildCache_Clear_Success(t *testing.T) {
	runtime := &mockContainerRuntime{}
	handler := NewBuildCacheHandler(runtime)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()

	handler.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "cleared" {
		t.Errorf("expected status=cleared, got %q", resp["status"])
	}
	if resp["message"] != "build cache pruned" {
		t.Errorf("expected message='build cache pruned', got %q", resp["message"])
	}
}

func TestBuildCache_Clear_NilRuntime(t *testing.T) {
	handler := NewBuildCacheHandler(nil)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()

	handler.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
