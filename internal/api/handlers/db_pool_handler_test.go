package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─── DB Pool ─────────────────────────────────────────────────────────────────

func TestDBPool_Get_Success(t *testing.T) {
	store := newMockStore()
	handler := NewDBPoolHandler(store)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases/db1/pool", nil)
	req.SetPathValue("id", "db1")
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp PoolConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.MaxConnections != 20 {
		t.Errorf("expected max_connections=20, got %d", resp.MaxConnections)
	}
	if resp.MinConnections != 2 {
		t.Errorf("expected min_connections=2, got %d", resp.MinConnections)
	}
	if resp.IdleTimeout != 300 {
		t.Errorf("expected idle_timeout_sec=300, got %d", resp.IdleTimeout)
	}
	if resp.MaxLifetime != 3600 {
		t.Errorf("expected max_lifetime_sec=3600, got %d", resp.MaxLifetime)
	}
}

func TestDBPool_Update_Success(t *testing.T) {
	store := newMockStore()
	handler := NewDBPoolHandler(store)

	body, _ := json.Marshal(PoolConfig{
		MaxConnections: 50,
		MinConnections: 5,
		IdleTimeout:    600,
		MaxLifetime:    7200,
	})
	req := httptest.NewRequest(http.MethodPut, "/api/v1/databases/db1/pool", bytes.NewReader(body))
	req.SetPathValue("id", "db1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["db_id"] != "db1" {
		t.Errorf("expected db_id=db1, got %v", resp["db_id"])
	}
	if resp["status"] != "updated" {
		t.Errorf("expected status=updated, got %v", resp["status"])
	}

	cfg, ok := resp["config"].(map[string]any)
	if !ok {
		t.Fatal("expected config object in response")
	}
	if int(cfg["max_connections"].(float64)) != 50 {
		t.Errorf("expected max_connections=50, got %v", cfg["max_connections"])
	}
	if int(cfg["min_connections"].(float64)) != 5 {
		t.Errorf("expected min_connections=5, got %v", cfg["min_connections"])
	}
}

func TestDBPool_Update_InvalidJSON(t *testing.T) {
	store := newMockStore()
	handler := NewDBPoolHandler(store)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/databases/db1/pool", bytes.NewReader([]byte("{")))
	req.SetPathValue("id", "db1")
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}
