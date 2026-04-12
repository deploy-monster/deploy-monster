package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestRespondOK(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondOK(rr, map[string]string{"key": "value"})

	if rr.Code != 200 {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestRespondError(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondError(rr, 404, "not_found", "resource not found")

	if rr.Code != 404 {
		t.Errorf("expected 404, got %d", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Success {
		t.Error("expected success=false")
	}
	if resp.Error == nil {
		t.Fatal("expected error object")
	}
	if resp.Error.Code != "not_found" {
		t.Errorf("expected code 'not_found', got %q", resp.Error.Code)
	}
}

func TestRespondPaginated(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondPaginated(rr, []string{"a", "b"}, 1, 10, 25)

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Meta == nil {
		t.Fatal("expected meta object")
	}
	if resp.Meta.Total != 25 {
		t.Errorf("expected total 25, got %d", resp.Meta.Total)
	}
	if resp.Meta.TotalPages != 3 {
		t.Errorf("expected 3 total pages, got %d", resp.Meta.TotalPages)
	}
}
