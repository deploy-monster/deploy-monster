package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestSanitizeError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"sql error", errors.New("database sql connection failed"), "operation failed"},
		{"connection refused", errors.New("connection refused"), "operation failed"},
		{"timeout", errors.New("request timeout"), "operation failed"},
		{"deadline exceeded", errors.New("context deadline exceeded"), "operation failed"},
		{"no such file", errors.New("open /tmp/x: no such file or directory"), "operation failed"},
		{"permission denied", errors.New("permission denied"), "operation failed"},
		{"internal", errors.New("internal server error"), "internal error"},
		{"panic", errors.New("panic: runtime error"), "internal error"},
		{"generic", errors.New("something went wrong"), "operation failed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeError(tc.err)
			if got != tc.want {
				t.Errorf("sanitizeError(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// ─── toLower ─────────────────────────────────────────────────────────────────

func TestToLower(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"HELLO", "hello"},
		{"HeLLo", "hello"},
		{"", ""},
		{"ABC", "abc"},
		{"abc", "abc"},
		{"Mixed", "mixed"},
		{"ALREADY_LOWER", "already_lower"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := toLower(tc.input)
			if result != tc.expected {
				t.Errorf("toLower(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestToLower_FullAlphabet(t *testing.T) {
	upper := "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	expected := "abcdefghijklmnopqrstuvwxyz"
	result := toLower(upper)
	if result != expected {
		t.Errorf("toLower(%q) = %q, want %q", upper, result, expected)
	}
}

// ─── toUpper ─────────────────────────────────────────────────────────────────

func TestToUpper(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "HELLO"},
		{"HELLO", "HELLO"},
		{"HeLLo", "HELLO"},
		{"", ""},
		{"abc", "ABC"},
		{"ABC", "ABC"},
		{"Mixed", "MIXED"},
		{"already_upper", "ALREADY_UPPER"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result := toUpper(tc.input)
			if result != tc.expected {
				t.Errorf("toUpper(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

// ─── containsCaseInsensitive ─────────────────────────────────────────────────

func TestContainsCaseInsensitive(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "HELLO", true},
		{"hello world", "WORLD", true},
		{"hello world", "hello", true},
		{"hello world", "world", true},
		{"hello world", "xyz", false},
		{"", "", true},
		{"hello", "", true},
		{"", "x", false},
	}

	for _, tc := range tests {
		t.Run(tc.s+"_"+tc.substr, func(t *testing.T) {
			result := containsCaseInsensitive(tc.s, tc.substr)
			if result != tc.expected {
				t.Errorf("containsCaseInsensitive(%q, %q) = %v, want %v", tc.s, tc.substr, result, tc.expected)
			}
		})
	}
}

// ─── strContain ──────────────────────────────────────────────────────────────

func TestStrContain(t *testing.T) {
	tests := []struct {
		s        string
		substr   string
		expected bool
	}{
		{"hello world", "hello", true},
		{"hello world", "world", true},
		{"hello world", "xyz", false},
		{"hello", "hell", true},
		{"hello", "ello", true},
		{"hello", "hello", true},
		{"", "", true},
		{"hello", "", true},
		{"", "x", false},
		{"ab", "abcd", false},
	}

	for _, tc := range tests {
		t.Run(tc.s+"_"+tc.substr, func(t *testing.T) {
			result := strContain(tc.s, tc.substr)
			if result != tc.expected {
				t.Errorf("strContain(%q, %q) = %v, want %v", tc.s, tc.substr, result, tc.expected)
			}
		})
	}
}

// ─── Bulk Execute rollback ───────────────────────────────────────────────────

type errorAfterCountStore struct {
	*mockStore
	count int
	after int
	errOn string // "start", "stop", "restart", "delete"
}

func (s *errorAfterCountStore) UpdateAppStatus(ctx context.Context, id, status string) error {
	if s.count >= s.after && (s.errOn == "start" || s.errOn == "stop" || s.errOn == "restart") {
		return errors.New("simulated failure after count")
	}
	s.count++
	return s.mockStore.UpdateAppStatus(ctx, id, status)
}

func (s *errorAfterCountStore) DeleteApp(ctx context.Context, id string) error {
	if s.count >= s.after && s.errOn == "delete" {
		return errors.New("simulated delete failure")
	}
	s.count++
	return s.mockStore.DeleteApp(ctx, id)
}

// Test BulkExecute rollback when the second app fails (first succeeds).
func TestBulkExecute_RollbackOnPartialFailure(t *testing.T) {
	store := &errorAfterCountStore{
		mockStore: newMockStore(),
		after:     1, // first succeeds, second fails
		errOn:     "start",
	}

	// Add apps to the store with initial status "original"
	store.apps["app1"] = &core.Application{ID: "app1", TenantID: "tenant1", Status: "original"}
	store.apps["app2"] = &core.Application{ID: "app2", TenantID: "tenant1", Status: "original"}

	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "start",
		AppIDs: []string{"app1", "app2"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	rolledBack, ok := resp["rolled_back"].(bool)
	if !ok || !rolledBack {
		t.Error("expected rolled_back=true")
	}

	// First app should have been rolled back to original status
	if store.updatedStatus["app1"] != "original" {
		t.Errorf("expected app1 status 'original' after rollback, got %q", store.updatedStatus["app1"])
	}
}

// Test BulkExecute where the FIRST app immediately fails (no rollback needed).
func TestBulkExecute_FirstAppFailsNoRollback(t *testing.T) {
	store := &errorAfterCountStore{
		mockStore: newMockStore(),
		after:     0, // fails immediately
		errOn:     "start",
	}

	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "start",
		AppIDs: []string{"app1", "app2"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	rolledBack, ok := resp["rolled_back"].(bool)
	if ok && rolledBack {
		t.Error("expected rolled_back=false when first app fails (nothing to rollback)")
	}

	// No apps should have been updated
	for id := range store.updatedStatus {
		t.Errorf("app %s was updated but should not have been", id)
	}
}

// Test BulkExecute restart with start failure (rollback to original status).
func TestBulkExecute_RestartStartFailsRollback(t *testing.T) {
	store := &errorAfterCountStore{
		mockStore: newMockStore(),
		after:     1, // first (stop) succeeds, second (start) fails
		errOn:     "restart", // the errorOn applies to the start phase
	}

	events := core.NewEventBus(nil)
	handler := NewBulkHandler(store, nil, events)

	body, _ := json.Marshal(bulkRequest{
		Action: "restart",
		AppIDs: []string{"app1"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/bulk", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	failed := int(resp["failed"].(float64))
	if failed != 1 {
		t.Errorf("expected failed=1, got %d", failed)
	}
}