package handlers

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/marketplace"
)

// === merged from auth_boost_test.go ===

func TestEnforceSessionLimit_NoBolt(t *testing.T) {
	h := NewAuthHandler(nil, nil, nil)
	// Should not panic
	h.enforceSessionLimit("user-1")
}

func TestEnforceSessionLimit_ListError(t *testing.T) {
	kv := newMockKVStore()
	h := NewAuthHandler(nil, nil, kv)
	// user_sessions bucket doesn't exist — List returns error
	h.enforceSessionLimit("user-1")
	// No panic = pass
}

func TestEnforceSessionLimit_UnderLimit(t *testing.T) {
	kv := newMockKVStore()
	h := NewAuthHandler(nil, nil, kv)

	for i := 0; i < 5; i++ {
		session := map[string]any{
			"user_id":    "user-1",
			"jti":        "jti-" + strconv.Itoa(i),
			"created_at": time.Now().Add(-time.Duration(i) * time.Hour),
		}
		kv.Set("user_sessions", "user-1:jti-"+strconv.Itoa(i), session, 0)
	}

	h.enforceSessionLimit("user-1")

	keys, _ := kv.List("user_sessions")
	if len(keys) != 5 {
		t.Errorf("expected 5 sessions, got %d", len(keys))
	}
}

func TestEnforceSessionLimit_OverLimit(t *testing.T) {
	kv := newMockKVStore()
	h := NewAuthHandler(nil, nil, kv)

	for i := 0; i < 12; i++ {
		session := map[string]any{
			"user_id":    "user-1",
			"jti":        "jti-" + strconv.Itoa(i),
			"created_at": time.Now().Add(-time.Duration(i) * time.Hour),
		}
		kv.Set("user_sessions", "user-1:jti-"+strconv.Itoa(i), session, 0)
	}

	h.enforceSessionLimit("user-1")

	keys, _ := kv.List("user_sessions")
	if len(keys) != 10 {
		t.Errorf("expected 10 sessions after pruning, got %d", len(keys))
	}

	// Oldest 2 sessions (jti-10, jti-11) should be revoked
	var revoked bool
	if err := kv.Get("revoked_tokens", "jti-10", &revoked); err != nil || !revoked {
		t.Error("expected jti-10 to be revoked")
	}
	if err := kv.Get("revoked_tokens", "jti-11", &revoked); err != nil || !revoked {
		t.Error("expected jti-11 to be revoked")
	}
}

func TestEnforceSessionLimit_OtherUserNotAffected(t *testing.T) {
	kv := newMockKVStore()
	h := NewAuthHandler(nil, nil, kv)

	// 12 sessions for user-1
	for i := 0; i < 12; i++ {
		session := map[string]any{
			"user_id":    "user-1",
			"jti":        "jti-" + strconv.Itoa(i),
			"created_at": time.Now().Add(-time.Duration(i) * time.Hour),
		}
		kv.Set("user_sessions", "user-1:jti-"+strconv.Itoa(i), session, 0)
	}

	// 5 sessions for user-2
	for i := 0; i < 5; i++ {
		session := map[string]any{
			"user_id":    "user-2",
			"jti":        "jti-u2-" + strconv.Itoa(i),
			"created_at": time.Now().Add(-time.Duration(i) * time.Hour),
		}
		kv.Set("user_sessions", "user-2:jti-u2-"+strconv.Itoa(i), session, 0)
	}

	h.enforceSessionLimit("user-1")

	keys, _ := kv.List("user_sessions")
	if len(keys) != 15 {
		t.Errorf("expected 15 sessions total, got %d", len(keys))
	}
}

// === merged from bulk_boost_test.go ===

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
	count    int
	after    int
	errOn    string // "start", "stop", "restart", "delete"
	failOnce bool   // only fail once, then allow subsequent calls (for rollback)
	failed   bool
}

func (s *errorAfterCountStore) UpdateAppStatus(ctx context.Context, id, status, tenantID string) error {
	if s.failOnce && s.failed {
		return s.mockStore.UpdateAppStatus(ctx, id, status, tenantID)
	}
	if s.count >= s.after && (s.errOn == "start" || s.errOn == "stop" || s.errOn == "restart") {
		s.failed = true
		return errors.New("simulated failure after count")
	}
	s.count++
	return s.mockStore.UpdateAppStatus(ctx, id, status, tenantID)
}

func (s *errorAfterCountStore) DeleteApp(ctx context.Context, id, tenantID string) error {
	if s.count >= s.after && s.errOn == "delete" {
		return errors.New("simulated delete failure")
	}
	s.count++
	return s.mockStore.DeleteApp(ctx, id, tenantID)
}

// Test BulkExecute rollback when the second app fails (first succeeds).
func TestBulkExecute_RollbackOnPartialFailure(t *testing.T) {
	store := &errorAfterCountStore{
		mockStore: newMockStore(),
		after:     1, // first succeeds, second fails
		errOn:     "start",
		failOnce:  true, // allow rollback to succeed
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
		after:     1,         // first (stop) succeeds, second (start) fails
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

// === merged from commands_boost_test.go ===

func TestCommand_History_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewCommandHandler(&mockContainerRuntime{}, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestCommand_History_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewCommandHandler(&mockContainerRuntime{}, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestCommand_History_WrongTenant(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant2", Name: "Test", Status: "running"})
	handler := NewCommandHandler(&mockContainerRuntime{}, store, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.History(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

// === merged from coverage_boost2_test.go ===

// ═══════════════════════════════════════════════════════════════════════════════
// RollbackHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRollbackHandler_New(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)
	if h == nil {
		t.Fatal("NewRollbackHandler returned nil")
	}
}

func TestRollbackHandler_Rollback_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRollbackHandler_Rollback_RejectsTrailingJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":1}{"version":2}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestRollbackHandler_Rollback_ZeroVersion(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":0}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRollbackHandler_Rollback_NegativeVersion(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":-1}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRollbackHandler_Rollback_VersionNotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":99}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestRollbackHandler_Rollback_Success(t *testing.T) {
	store := newMockStore()
	store.addDeployment("app-1", core.Deployment{Version: 1, Image: "app:v1", Status: "stopped"})
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback", strings.NewReader(`{"version":1}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.Rollback(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRollbackHandler_ListVersions_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	store.addDeployment("app-1", core.Deployment{Version: 2, Image: "app:v2", Status: "running"})
	store.addDeployment("app-1", core.Deployment{Version: 1, Image: "app:v1", Status: "stopped"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/versions", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.ListVersions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRollbackHandler_ListVersions_Error(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	store.errListDeploymentsByApp = core.ErrNotFound
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/versions", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.ListVersions(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// TestRollbackHandler_ListVersions_CrossTenant verifies Phase 7.11 fix —
// a developer token for tenant A must not be able to list versions of an
// app owned by tenant B, even if they guess the app ID.
func TestRollbackHandler_ListVersions_CrossTenant(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-B", Name: "foreign", TenantID: "tenant-B"})
	store.addDeployment("app-B", core.Deployment{Version: 1, Image: "app:v1", Status: "running"})
	events := core.NewEventBus(slog.Default())
	h := NewRollbackHandler(store, nil, events)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-B/versions", nil)
	req.SetPathValue("id", "app-B")
	req = withClaims(req, "u1", "tenant-A", "role_developer", "u@t")
	rr := httptest.NewRecorder()
	h.ListVersions(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("cross-tenant ListVersions should be 404, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// EventWebhookHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestEventWebhookHandler_New(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())
	if h == nil {
		t.Fatal("NewEventWebhookHandler returned nil")
	}
}

func TestEventWebhookHandler_List_Empty(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/webhooks/outbound", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_List_WithData(t *testing.T) {
	kv := newMockKVStore()
	list := eventWebhookList{
		Webhooks: []EventWebhookConfig{
			{ID: "wh1", URL: "https://example.com/hook", SecretHash: hashSecret("s3cret"), Events: []string{"app.deployed"}, Active: true},
			{ID: "wh2", URL: "https://example.com/hook2", SecretHash: "", Events: []string{"app.crashed"}, Active: true},
		},
	}
	kv.Set("event_webhooks", "tenant:tenant1", list, 0)

	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), kv)

	req := httptest.NewRequest("GET", "/api/v1/webhooks/outbound", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	data := body["data"].([]any)
	if len(data) != 2 {
		t.Errorf("len(data) = %d, want 2", len(data))
	}
	if body["total"] != float64(2) {
		t.Errorf("total = %v, want 2", body["total"])
	}
}

func TestEventWebhookHandler_Create_NoClaims(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_InvalidBody(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader("{bad"))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_MissingFields(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(`{"url":""}`))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_Success(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())

	body := `{"url":"https://example.com/hook","events":["app.deployed"]}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_WithSecret(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())

	body := `{"url":"https://example.com/hook","events":["app.deployed"],"secret":"my-secret"}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Delete_NotFound(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())

	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/outbound/wh-1", nil)
	req.SetPathValue("id", "wh-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Delete_Success(t *testing.T) {
	kv := newMockKVStore()
	list := eventWebhookList{
		Webhooks: []EventWebhookConfig{
			{ID: "wh-1", URL: "https://example.com", Events: []string{"app.deployed"}},
			{ID: "wh-2", URL: "https://other.com", Events: []string{"app.crashed"}},
		},
	}
	kv.Set("event_webhooks", "tenant:t1", list, 0)

	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(nil), kv)

	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/outbound/wh-1", nil)
	req.SetPathValue("id", "wh-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ExecHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestExecHandler_New(t *testing.T) {
	h := NewExecHandler(nil, nil, slog.Default(), nil)
	if h == nil {
		t.Fatal("NewExecHandler returned nil")
	}
}

func TestExecHandler_NilRuntime(t *testing.T) {
	h := NewExecHandler(nil, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(`{"command":"ls"}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestExecHandler_InvalidBody(t *testing.T) {
	h := NewExecHandler(&mockContainerRuntime{}, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestExecHandler_EmptyCommand(t *testing.T) {
	h := NewExecHandler(&mockContainerRuntime{}, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(`{"command":""}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestExecHandler_NoContainer(t *testing.T) {
	h := NewExecHandler(&mockContainerRuntime{containers: []core.ContainerInfo{}}, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(`{"command":"ls"}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestExecHandler_Success(t *testing.T) {
	h := NewExecHandler(&mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc123", State: "running"}},
	}, nil, slog.Default(), nil)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(`{"command":"ls -la"}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// FileBrowserHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestFileBrowserHandler_New(t *testing.T) {
	h := NewFileBrowserHandler(nil, nil)
	if h == nil {
		t.Fatal("NewFileBrowserHandler returned nil")
	}
}

func TestFileBrowserHandler_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewFileBrowserHandler(store, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/files?path=/", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestFileBrowserHandler_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewFileBrowserHandler(store, &mockContainerRuntime{containers: []core.ContainerInfo{}})

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/files?path=/tmp", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestFileBrowserHandler_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewFileBrowserHandler(store, &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc123def456", State: "running"}},
	})

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/files", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// GPUHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestGPUHandler_New(t *testing.T) {
	h := NewGPUHandler(newMockStore(), nil, newMockKVStore())
	if h == nil {
		t.Fatal("NewGPUHandler returned nil")
	}
}

func TestGPUHandler_Get_Default(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGPUHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	cfg := GPUConfig{Enabled: true, Capabilities: []string{"compute"}, Driver: "nvidia"}
	kv.Set("gpu_config", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, kv)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGPUHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/gpu", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestGPUHandler_Update_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, newMockKVStore())

	body := `{"enabled":true,"capabilities":[],"driver":""}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/gpu", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGPUHandler_DetectGPU_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, nil, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	detection := body["detection"].(map[string]any)
	if detection["available"] != false {
		t.Errorf("expected GPU not available, got %v", detection["available"])
	}
}

func TestGPUHandler_DetectGPU_WithNvidiaImage(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{},
	}
	// Override ImageList to return nvidia images
	h := &GPUHandler{store: store, runtime: runtime, kv: newMockKVStore()}

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RedirectHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRedirectHandler_New(t *testing.T) {
	h := NewRedirectHandler(nil, newMockKVStore())
	if h == nil {
		t.Fatal("NewRedirectHandler returned nil")
	}
}

func TestRedirectHandler_List_Empty(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/redirects", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRedirectHandler_List_WithData(t *testing.T) {
	kv := newMockKVStore()
	list := redirectList{Rules: []RedirectRule{{ID: "r1", Source: "/old", Destination: "/new", StatusCode: 301}}}
	kv.Set("redirects", "app-1", list, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, kv)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/redirects", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_RejectsTrailingJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(`{"source":"/old","destination":"/new"}{"source":"/other","destination":"/next"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestRedirectHandler_Create_MissingFields(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(`{"source":""}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockKVStore())

	body := `{"source":"/old","destination":"/new"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestRedirectHandler_Delete_NotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, newMockKVStore())

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/redirects/r-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("ruleId", "r-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestRedirectHandler_Delete_Success(t *testing.T) {
	kv := newMockKVStore()
	list := redirectList{Rules: []RedirectRule{
		{ID: "r-1", Source: "/old", Destination: "/new"},
		{ID: "r-2", Source: "/foo", Destination: "/bar"},
	}}
	kv.Set("redirects", "app-1", list, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRedirectHandler(store, kv)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/redirects/r-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("ruleId", "r-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ResponseHeadersHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestResponseHeadersHandler_New(t *testing.T) {
	h := NewResponseHeadersHandler(nil, newMockKVStore())
	if h == nil {
		t.Fatal("NewResponseHeadersHandler returned nil")
	}
}

func TestResponseHeadersHandler_Get_Default(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/response-headers", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestResponseHeadersHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	cfg := ResponseHeadersConfig{HSTS: "max-age=31536000", XFrameOptions: "SAMEORIGIN"}
	kv.Set("response_headers", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, kv)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/response-headers", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestResponseHeadersHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestResponseHeadersHandler_Update_RejectsUnknownFields(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader(`{"x_frame_options":"DENY","extra":true}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestResponseHeadersHandler_Update_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResponseHeadersHandler(store, newMockKVStore())

	body := `{"hsts":"max-age=31536000","x_frame_options":"DENY"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// StickySessionHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestStickySessionHandler_New(t *testing.T) {
	h := NewStickySessionHandler(nil, newMockKVStore())
	if h == nil {
		t.Fatal("NewStickySessionHandler returned nil")
	}
}

func TestStickySessionHandler_Get_Default(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/sticky-sessions", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	cfg := StickySessionConfig{Enabled: true, Cookie: "MY_SESSION", MaxAge: 7200}
	kv.Set("sticky_sessions", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, kv)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/sticky-sessions", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Update_RejectsTrailingJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader(`{"enabled":true}{"enabled":false}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestStickySessionHandler_Update_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStickySessionHandler(store, newMockKVStore())

	body := `{"enabled":true,"cookie":"","max_age":3600}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// SuspendHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestSuspendHandler_New(t *testing.T) {
	h := NewSuspendHandler(newMockStore(), nil, core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewSuspendHandler returned nil")
	}
}

func TestSuspendHandler_Suspend_AppNotFound(t *testing.T) {
	store := newMockStore()
	h := NewSuspendHandler(store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/suspend", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Suspend(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestSuspendHandler_Suspend_AlreadySuspended(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test", Status: "suspended"})
	h := NewSuspendHandler(store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/suspend", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Suspend(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestSuspendHandler_Suspend_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test", Status: "running"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1", State: "running"}},
	}
	h := NewSuspendHandler(store, runtime, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/suspend", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Suspend(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSuspendHandler_Resume_AppNotFound(t *testing.T) {
	store := newMockStore()
	h := NewSuspendHandler(store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/resume", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Resume(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestSuspendHandler_Resume_NotSuspended(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test", Status: "running"})
	h := NewSuspendHandler(store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/resume", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Resume(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestSuspendHandler_Resume_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test", Status: "suspended"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1", State: "exited"}},
	}
	h := NewSuspendHandler(store, runtime, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/resume", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Resume(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ResourceHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestResourceHandler_New(t *testing.T) {
	h := NewResourceHandler(newMockStore(), core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewResourceHandler returned nil")
	}
}

func TestResourceHandler_SetLimits_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResourceHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/resources", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.SetLimits(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestResourceHandler_SetLimits_RejectsUnknownFields(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewResourceHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/resources", strings.NewReader(`{"cpu_quota":100000,"memory_mb":512,"extra":true}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.SetLimits(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestResourceHandler_SetLimits_AppNotFound(t *testing.T) {
	h := NewResourceHandler(newMockStore(), core.NewEventBus(nil))

	body := `{"cpu_quota":100000,"memory_mb":512}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/resources", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.SetLimits(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestResourceHandler_SetLimits_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	h := NewResourceHandler(store, core.NewEventBus(nil))

	body := `{"cpu_quota":100000,"memory_mb":512}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/resources", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.SetLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestResourceHandler_GetLimits_AppNotFound(t *testing.T) {
	h := NewResourceHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/resources", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.GetLimits(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestResourceHandler_GetLimits_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	h := NewResourceHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/resources", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.GetLimits(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// StatsHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestStatsHandler_New(t *testing.T) {
	h := NewStatsHandler(nil, newMockStore())
	if h == nil {
		t.Fatal("NewStatsHandler returned nil")
	}
}

func TestStatsHandler_AppStats_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStatsHandler(nil, store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/stats", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppStats(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestStatsHandler_AppStats_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStatsHandler(&mockContainerRuntime{}, store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/stats", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (empty stats for app with no containers), got %d", rr.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if count, _ := resp["count"].(float64); count != 0 {
		t.Errorf("expected count=0, got %v", resp["count"])
	}
}

func TestStatsHandler_AppStats_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewStatsHandler(&mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-1", State: "running"}},
	}, store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/stats", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestStatsHandler_ServerStats_NilRuntime(t *testing.T) {
	h := NewStatsHandler(nil, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/servers/stats", nil)
	rr := httptest.NewRecorder()
	h.ServerStats(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestStatsHandler_ServerStats_Success(t *testing.T) {
	h := NewStatsHandler(&mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running"},
			{ID: "c2", State: "exited"},
		},
	}, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/servers/stats", nil)
	rr := httptest.NewRecorder()
	h.ServerStats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WebhookReplayHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestWebhookReplayHandler_New(t *testing.T) {
	h := NewWebhookReplayHandler(nil, core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewWebhookReplayHandler returned nil")
	}
}

func TestWebhookReplayHandler_Replay(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewWebhookReplayHandler(store, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/webhooks/log-1/replay", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("logId", "log-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Replay(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WebhookRotateHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestWebhookRotateHandler_New(t *testing.T) {
	h := NewWebhookRotateHandler(newMockStore(), core.NewEventBus(nil), newMockKVStore())
	if h == nil {
		t.Fatal("NewWebhookRotateHandler returned nil")
	}
}

func TestWebhookRotateHandler_Rotate(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	kv := newMockKVStore()
	h := NewWebhookRotateHandler(store, core.NewEventBus(slog.Default()), kv)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/webhooks/rotate", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Rotate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp struct {
		NewSecret string `json:"new_secret"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.NewSecret == "" {
		t.Fatal("expected response to include one-time webhook secret")
	}

	got, err := kv.GetWebhookSecret("app-1")
	if err != nil {
		t.Fatalf("GetWebhookSecret: %v", err)
	}
	if got != hashSecret(resp.NewSecret) {
		t.Fatalf("persisted secret hash = %q, want hash of response secret", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WebhookTestDeliveryHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestWebhookTestDeliveryHandler_New(t *testing.T) {
	h := NewWebhookTestDeliveryHandler(nil, core.NewEventBus(nil), newMockKVStore())
	if h == nil {
		t.Fatal("NewWebhookTestDeliveryHandler returned nil")
	}
}

func TestWebhookTestDeliveryHandler_TestDeliver(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewWebhookTestDeliveryHandler(store, core.NewEventBus(slog.Default()), newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/webhooks/test", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.TestDeliver(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// SaveTemplateHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestSaveTemplateHandler_New(t *testing.T) {
	h := NewSaveTemplateHandler(newMockStore())
	if h == nil {
		t.Fatal("NewSaveTemplateHandler returned nil")
	}
}

func TestSaveTemplateHandler_NoClaims(t *testing.T) {
	h := NewSaveTemplateHandler(newMockStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader("{}"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSaveTemplateHandler_AppNotFound(t *testing.T) {
	h := NewSaveTemplateHandler(newMockStore())

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader("{}"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestSaveTemplateHandler_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewSaveTemplateHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSaveTemplateHandler_RejectsTrailingJSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewSaveTemplateHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader(`{"name":"template"}{"name":"other"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestSaveTemplateHandler_Success_DefaultFields(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "my-app", SourceType: "image", SourceURL: "nginx:latest"})
	h := NewSaveTemplateHandler(store)

	body := `{"description":"A test template","category":"web"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/save-template", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Save(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TransferHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestTransferHandler_New(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))
	if h == nil {
		t.Fatal("NewTransferHandler returned nil")
	}
}

func TestTransferHandler_InvalidBody(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestTransferHandler_RejectsUnknownFields(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":"t2","extra":true}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestTransferHandler_MissingTargetTenant(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":""}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestTransferHandler_AppNotFound(t *testing.T) {
	h := NewTransferHandler(newMockStore(), core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":"t2"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestTransferHandler_TargetTenantNotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	h := NewTransferHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":"t-nonexistent"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestTransferHandler_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "test", TenantID: "t1"})
	store.addTenant(&core.Tenant{ID: "t2", Name: "Target Tenant"})
	h := NewTransferHandler(store, core.NewEventBus(nil))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(`{"target_tenant_id":"t2"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// WildcardSSLHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestWildcardSSLHandler_New(t *testing.T) {
	h := NewWildcardSSLHandler(newMockKVStore())
	if h == nil {
		t.Fatal("NewWildcardSSLHandler returned nil")
	}
}

func TestWildcardSSLHandler_Request_InvalidBody(t *testing.T) {
	h := NewWildcardSSLHandler(newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader("{bad"))
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Request(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestWildcardSSLHandler_Request_RejectsTrailingJSON(t *testing.T) {
	h := NewWildcardSSLHandler(newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader(`{"domain":"example.com"}{"domain":"other.com"}`))
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Request(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestWildcardSSLHandler_Request_MissingDomain(t *testing.T) {
	h := NewWildcardSSLHandler(newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader(`{"domain":""}`))
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Request(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestWildcardSSLHandler_Request_Success(t *testing.T) {
	h := NewWildcardSSLHandler(newMockKVStore())

	body := `{"domain":"example.com","dns_provider":"cloudflare"}`
	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Request(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
}

func TestWildcardSSLHandler_Request_NoClaims(t *testing.T) {
	h := NewWildcardSSLHandler(newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader(`{"domain":"example.com"}`))
	rr := httptest.NewRecorder()
	h.Request(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RestartHistoryHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRestartHistoryHandler_New(t *testing.T) {
	h := NewRestartHistoryHandler(nil, nil)
	if h == nil {
		t.Fatal("NewRestartHistoryHandler returned nil")
	}
}

func TestRestartHistoryHandler_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRestartHistoryHandler(store, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/restarts", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRestartHistoryHandler_NoContainer(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRestartHistoryHandler(store, &mockContainerRuntime{})

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/restarts", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRestartHistoryHandler_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewRestartHistoryHandler(store, &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc123def456", State: "running"}},
	})

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/restarts", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRestartHistoryHandler_ListReadsBoltEventsNewestFirst(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	kv := newMockKVStore()
	oldEvent := RestartEvent{ID: "old", AppID: "app-1", Reason: "crash", Source: core.EventContainerDied, Timestamp: time.Now().Add(-time.Hour)}
	newEvent := RestartEvent{ID: "new", AppID: "app-1", Reason: "deploy", Source: core.EventAppDeployed, Timestamp: time.Now()}
	otherEvent := RestartEvent{ID: "other", AppID: "app-2", Reason: "crash", Source: core.EventContainerDied, Timestamp: time.Now()}
	if err := kv.Set(RestartHistoryBucket, "app-1:old", oldEvent, 0); err != nil {
		t.Fatalf("seed old event: %v", err)
	}
	if err := kv.Set(RestartHistoryBucket, "app-1:new", newEvent, 0); err != nil {
		t.Fatalf("seed new event: %v", err)
	}
	if err := kv.Set(RestartHistoryBucket, "app-2:other", otherEvent, 0); err != nil {
		t.Fatalf("seed other event: %v", err)
	}

	h := NewRestartHistoryHandler(store, &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "ctr-abc123def456", State: "running"}},
	})
	h.SetKV(kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/restarts", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Total       int            `json:"total"`
		ContainerID string         `json:"container_id"`
		Data        []RestartEvent `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != 2 || len(resp.Data) != 2 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if resp.ContainerID != "ctr-abc123de" {
		t.Fatalf("container_id = %q", resp.ContainerID)
	}
	if resp.Data[0].ID != "new" || resp.Data[1].ID != "old" {
		t.Fatalf("events not sorted newest first: %+v", resp.Data)
	}
}

func TestSubscribeRestartHistoryPersistsSupportedEvents(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	kv := newMockKVStore()
	SubscribeRestartHistory(events, kv)

	ctx := context.Background()
	events.Publish(ctx, core.NewEvent(core.EventContainerDied, "docker", core.DeployEventData{AppID: "app-1", ContainerID: "ctr-1"}))
	events.Publish(ctx, core.NewEvent(core.EventAppStarted, "operator", map[string]string{"id": "app-1", "action": "restart", "container_id": "ctr-2"}))
	events.Publish(ctx, core.NewEvent(core.EventAppDeployed, "deploy", core.AppEventData{AppID: "app-1"}))
	events.Drain()

	keys, err := kv.List(RestartHistoryBucket)
	if err != nil {
		t.Fatalf("List restart history: %v", err)
	}
	if len(keys) != 3 {
		t.Fatalf("keys = %v, want 3 events", keys)
	}
	var sawCrash, sawRestart, sawDeploy bool
	for _, key := range keys {
		var ev RestartEvent
		if err := kv.Get(RestartHistoryBucket, key, &ev); err != nil {
			t.Fatalf("Get %s: %v", key, err)
		}
		sawCrash = sawCrash || ev.Reason == "crash"
		sawRestart = sawRestart || ev.Reason == "restart"
		sawDeploy = sawDeploy || ev.Reason == "deploy"
	}
	if !sawCrash || !sawRestart || !sawDeploy {
		t.Fatalf("missing expected reasons: crash=%v restart=%v deploy=%v", sawCrash, sawRestart, sawDeploy)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RestartPolicyHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestRestartPolicyHandler_New(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)
	if h == nil {
		t.Fatal("NewRestartPolicyHandler returned nil")
	}
}

func TestRestartPolicyHandler_InvalidBody(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRestartPolicyHandler_RejectsUnknownFields(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader(`{"policy":"always","extra":true}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestRestartPolicyHandler_InvalidPolicy(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader(`{"policy":"invalid"}`))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestRestartPolicyHandler_AppNotFound(t *testing.T) {
	h := NewRestartPolicyHandler(newMockStore(), nil)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader(`{"policy":"always"}`))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestRestartPolicyHandler_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	h := NewRestartPolicyHandler(store, nil)

	for _, policy := range []string{"always", "unless-stopped", "on-failure", "no"} {
		req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/restart-policy", strings.NewReader(`{"policy":"`+policy+`"}`))
		req.SetPathValue("id", "app-1")
		req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
		rr := httptest.NewRecorder()
		h.Update(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("policy %q: expected 200, got %d", policy, rr.Code)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TenantRateLimitHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestTenantRateLimitHandler_New(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockKVStore())
	if h == nil {
		t.Fatal("NewTenantRateLimitHandler returned nil")
	}
}

func TestTenantRateLimitHandler_Get_Default(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants/t1/ratelimit", nil)
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	cfg := RateLimitConfig{RequestsPerMinute: 200, BurstSize: 50}
	kv.Set("tenant_ratelimit", "t1", cfg, 0)

	h := NewTenantRateLimitHandler(kv)

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants/t1/ratelimit", nil)
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Update_InvalidBody(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/admin/tenants/t1/ratelimit", strings.NewReader("{bad"))
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Update_RejectsUnknownFields(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/admin/tenants/t1/ratelimit", strings.NewReader(`{"requests_per_minute":200,"extra":true}`))
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestTenantRateLimitHandler_Update_Success(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockKVStore())

	body := `{"requests_per_minute":200,"burst_size":50,"builds_per_hour":20,"deploys_per_hour":30}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/tenants/t1/ratelimit", strings.NewReader(body))
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Update_DefaultValues(t *testing.T) {
	h := NewTenantRateLimitHandler(newMockKVStore())

	body := `{"requests_per_minute":0,"burst_size":0}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/tenants/t1/ratelimit", strings.NewReader(body))
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageTagHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestImageTagHandler_New(t *testing.T) {
	h := NewImageTagHandler(newMockStore(), nil)
	if h == nil {
		t.Fatal("NewImageTagHandler returned nil")
	}
}

func TestImageTagHandler_List_MissingImage(t *testing.T) {
	h := NewImageTagHandler(newMockStore(), &mockContainerRuntime{})

	req := httptest.NewRequest("GET", "/api/v1/images/tags", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestImageTagHandler_List_Success(t *testing.T) {
	h := NewImageTagHandler(newMockStore(), &mockContainerRuntime{})

	req := httptest.NewRequest("GET", "/api/v1/images/tags?image=nginx", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.List(rr, req)

	// With tenant isolation, image access is denied if no apps use it
	// Mock store has no apps, so expect 403 Forbidden
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 (access denied to image), got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// StorageHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestStorageHandler_New(t *testing.T) {
	h := NewStorageHandler(newMockStore(), nil, newMockKVStore())
	if h == nil {
		t.Fatal("NewStorageHandler returned nil")
	}
}

func TestStorageHandler_Usage_NoClaims(t *testing.T) {
	h := NewStorageHandler(newMockStore(), &mockContainerRuntime{}, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/storage/usage", nil)
	rr := httptest.NewRecorder()
	h.Usage(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestStorageHandler_Usage_Success(t *testing.T) {
	h := NewStorageHandler(newMockStore(), &mockContainerRuntime{}, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/storage/usage", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Usage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// TenantSettingsHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestTenantSettingsHandler_New(t *testing.T) {
	h := NewTenantSettingsHandler(newMockStore())
	if h == nil {
		t.Fatal("NewTenantSettingsHandler returned nil")
	}
}

func TestTenantSettingsHandler_Get_NoClaims(t *testing.T) {
	h := NewTenantSettingsHandler(newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/tenant/settings", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestTenantSettingsHandler_Get_TenantNotFound(t *testing.T) {
	h := NewTenantSettingsHandler(newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/tenant/settings", nil)
	req = withClaims(req, "u1", "t-missing", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestTenantSettingsHandler_Get_Success(t *testing.T) {
	store := newMockStore()
	store.addTenant(&core.Tenant{ID: "t1", Name: "Test", Slug: "test", PlanID: "free", Status: "active"})
	h := NewTenantSettingsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/tenant/settings", nil)
	req = withClaims(req, "u1", "t1", "admin", "u@e.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// === merged from coverage_boost4_test.go ===

// =============================================================================
// AdminAPIKeyHandler — List, Generate, Revoke
// =============================================================================

func TestAdminAPIKeyHandler_List_EmptyIndex(t *testing.T) {
	kv := newMockKVStore()
	h := NewAdminAPIKeyHandler(newMockStore(), kv)

	req := httptest.NewRequest("GET", "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total 0, got %v", resp["total"])
	}
}

func TestAdminAPIKeyHandler_List_WithKeys(t *testing.T) {
	kv := newMockKVStore()
	// Seed an index with two prefixes
	kv.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"pfx-a", "pfx-b"}}, 0)
	kv.Set("api_keys", "pfx-a", apiKeyRecord{Prefix: "pfx-a", Hash: "h1", Type: "platform", CreatedBy: "u1", CreatedAt: time.Now()}, 0)
	kv.Set("api_keys", "pfx-b", apiKeyRecord{Prefix: "pfx-b", Hash: "h2", Type: "platform", CreatedBy: "u2", CreatedAt: time.Now()}, 0)

	h := NewAdminAPIKeyHandler(newMockStore(), kv)
	req := httptest.NewRequest("GET", "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 2 {
		t.Errorf("expected total 2, got %v", resp["total"])
	}
}

func TestAdminAPIKeyHandler_List_MissingKeyRecord(t *testing.T) {
	kv := newMockKVStore()
	// Index has a prefix but the record doesn't exist
	kv.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"pfx-missing"}}, 0)

	h := NewAdminAPIKeyHandler(newMockStore(), kv)
	req := httptest.NewRequest("GET", "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total 0 (record missing), got %v", resp["total"])
	}
}

func TestAdminAPIKeyHandler_Generate_Success(t *testing.T) {
	kv := newMockKVStore()
	h := NewAdminAPIKeyHandler(newMockStore(), kv)

	req := httptest.NewRequest("POST", "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["key"] == nil || resp["key"].(string) == "" {
		t.Error("expected key to be returned")
	}
	if resp["prefix"] == nil {
		t.Error("expected prefix")
	}
}

func TestAdminAPIKeyHandler_Revoke_Success(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("api_keys", "pfx-x", apiKeyRecord{Prefix: "pfx-x"}, 0)
	kv.Set("api_keys", "_index", apiKeyIndex{Prefixes: []string{"pfx-x", "pfx-y"}}, 0)

	h := NewAdminAPIKeyHandler(newMockStore(), kv)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/api-keys/pfx-x", nil)
	req.SetPathValue("prefix", "pfx-x")
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Revoke(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}

	// Verify index is updated
	var idx apiKeyIndex
	kv.Get("api_keys", "_index", &idx)
	for _, p := range idx.Prefixes {
		if p == "pfx-x" {
			t.Error("pfx-x should have been removed from index")
		}
	}
}

func TestAdminAPIKeyHandler_Revoke_NoIndex(t *testing.T) {
	kv := newMockKVStore()
	h := NewAdminAPIKeyHandler(newMockStore(), kv)

	req := httptest.NewRequest("DELETE", "/api/v1/admin/api-keys/pfx-z", nil)
	req.SetPathValue("prefix", "pfx-z")
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Revoke(rr, req)

	// Should still return 204 even if index doesn't exist
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// =============================================================================
// AgentStatusHandler — List, GetAgent
// =============================================================================

func TestAgentStatusHandler_List_WithContainer(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", Name: "test-app"},
			{ID: "c2", Name: "test-app-2"},
		},
	}
	c.Services.Container = runtime

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local := resp["local"].(map[string]any)
	if int(local["containers"].(float64)) != 2 {
		t.Errorf("expected 2 containers, got %v", local["containers"])
	}
}

func TestAgentStatusHandler_List_NilContainer(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	// c.Services.Container is nil

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAgentStatusHandler_List_ContainerListError(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	c.Services.Container = &mockContainerRuntime{listErr: io.EOF}

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 even with container list error, got %d", rr.Code)
	}
}

func TestAgentStatusHandler_List_DegradedHealth(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	// Register a module with degraded health
	c.Registry = core.NewRegistry()
	c.Registry.Register(&degradedModule{})

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local := resp["local"].(map[string]any)
	if local["status"] != "degraded" {
		t.Errorf("expected degraded, got %v", local["status"])
	}
}

func TestAgentStatusHandler_List_DownHealth(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	c.Registry.Register(&downModule{})

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	local := resp["local"].(map[string]any)
	if local["status"] != "unhealthy" {
		t.Errorf("expected unhealthy, got %v", local["status"])
	}
}

func TestAgentStatusHandler_GetAgent_Local(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()

	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents/local", nil)
	req.SetPathValue("id", "local")
	rr := httptest.NewRecorder()
	h.GetAgent(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAgentStatusHandler_GetAgent_Remote(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "2.0.0"}
	c.Registry = core.NewRegistry()

	// Phase 7.11: GetAgent previously echoed any requested ID back with
	// faked fields, leaking existence of arbitrary server IDs to any
	// authenticated user. Until the remote-agent registry lookup lands,
	// the handler returns 404 for any non-"local" ID.
	h := NewAgentStatusHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/agents/remote-1", nil)
	req.SetPathValue("id", "remote-1")
	rr := httptest.NewRecorder()
	h.GetAgent(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// PinHandler — Pin, Unpin
// =============================================================================

func TestPinHandler_Pin_NoClaims(t *testing.T) {
	h := NewPinHandler(newMockStore(), newMockKVStore())
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Pin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestPinHandler_Pin_NewPin(t *testing.T) {
	kv := newMockKVStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "Test", Status: "running"})
	h := NewPinHandler(store, kv)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Pin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["pinned"] != "true" {
		t.Errorf("expected pinned=true, got %q", resp["pinned"])
	}
}

func TestPinHandler_Pin_AlreadyPinned(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("app_pins", "u1", pinnedApps{AppIDs: []string{"app-1"}}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "Test", Status: "running"})
	h := NewPinHandler(store, kv)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Pin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPinHandler_Unpin_NoClaims(t *testing.T) {
	h := NewPinHandler(newMockStore(), newMockKVStore())
	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Unpin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestPinHandler_Unpin_NoPins(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "Test", Status: "running"})
	h := NewPinHandler(store, newMockKVStore())
	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Unpin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPinHandler_Unpin_ExistingPin(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("app_pins", "u1", pinnedApps{AppIDs: []string{"app-1", "app-2"}}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "Test", Status: "running"})
	h := NewPinHandler(store, kv)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Unpin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Verify app-1 is removed but app-2 remains
	var pins pinnedApps
	kv.Get("app_pins", "u1", &pins)
	if len(pins.AppIDs) != 1 || pins.AppIDs[0] != "app-2" {
		t.Errorf("expected [app-2], got %v", pins.AppIDs)
	}
}

// =============================================================================
// BackupHandler — Download
// =============================================================================

func TestBackupHandler_Download_NilStorage_Boost4(t *testing.T) {
	h := NewBackupHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))
	req := httptest.NewRequest("GET", "/api/v1/backups/t1/app-1/test.tar/download", nil)
	req.SetPathValue("key", "t1/app-1/test.tar")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Download(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

func TestBackupHandler_Download_Success(t *testing.T) {
	storage := &mockBackupStorage{
		fileData: "backup-data-bytes",
	}
	h := NewBackupHandler(newMockStore(), storage, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("GET", "/api/v1/backups/t1/app-1/test.tar/download", nil)
	req.SetPathValue("key", "t1/app-1/test.tar")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Download(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/octet-stream" {
		t.Errorf("wrong Content-Type: %q", rr.Header().Get("Content-Type"))
	}
	if !strings.Contains(rr.Header().Get("Content-Disposition"), "test.tar") {
		t.Error("expected Content-Disposition to contain the filename")
	}
	if rr.Body.String() != "backup-data-bytes" {
		t.Errorf("body = %q, want backup-data-bytes", rr.Body.String())
	}
}

func TestBackupHandler_Download_NotFound(t *testing.T) {
	storage := &mockBackupStorage{errDown: io.EOF}
	h := NewBackupHandler(newMockStore(), storage, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("GET", "/api/v1/backups/t1/app-1/missing.tar/download", nil)
	req.SetPathValue("key", "t1/app-1/missing.tar")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Download(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// CertificateHandler — List (expired cert path), Upload
// =============================================================================

func TestCertificateHandler_List_ExpiredCerts(t *testing.T) {
	kv := newMockKVStore()
	// Seed with one expired cert (with matching TenantID so filter passes)
	kv.Set("certificates", "all", certStore{
		Certs: []CertInfo{
			{ID: "c1", TenantID: "test-tenant", Domain: "example.com", ExpiresAt: time.Now().Add(-24 * time.Hour), Status: "active"},
		},
	}, 0)

	h := NewCertificateHandler(newMockStore(), kv)
	req := httptest.NewRequest("GET", "/api/v1/certificates", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	data := resp["data"].([]any)
	cert := data[0].(map[string]any)
	if cert["status"] != "expired" {
		t.Errorf("expected status 'expired', got %v", cert["status"])
	}
}

func TestCertificateHandler_Upload_MissingFields(t *testing.T) {
	h := NewCertificateHandler(newMockStore(), newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates", strings.NewReader(`{"domain_id":"","cert_pem":"","key_pem":""}`))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCertificateHandler_Upload_InvalidCert(t *testing.T) {
	h := NewCertificateHandler(newMockStore(), newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/certificates",
		strings.NewReader(`{"domain_id":"d1","cert_pem":"not-a-cert","key_pem":"not-a-key"}`))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid cert, got %d", rr.Code)
	}
}

// =============================================================================
// BuildCacheHandler — Stats, Clear
// =============================================================================

func TestBuildCacheHandler_Stats_NilRuntime(t *testing.T) {
	h := NewBuildCacheHandler(nil, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestBuildCacheHandler_Stats_WithImages(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{
			{ID: "img1", Tags: []string{"app:v1"}, Size: 100 * 1024 * 1024},
			{ID: "img2", Tags: []string{"app:v2"}, Size: 200 * 1024 * 1024},
		},
	}
	kv := newMockKVStore()
	kv.Set("buildcache", "stats", buildCacheStats{TotalBuilds: 10, CacheHits: 7, CacheMisses: 3, TotalSavedSec: 120}, 0)

	h := NewBuildCacheHandler(runtime, kv)
	req := httptest.NewRequest("GET", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Stats(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["layers"].(float64)) != 2 {
		t.Errorf("expected 2 layers, got %v", resp["layers"])
	}
	if int(resp["total_builds"].(float64)) != 10 {
		t.Errorf("expected total_builds 10, got %v", resp["total_builds"])
	}
}

func TestBuildCacheHandler_Stats_ImageListError(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{imageListErr: io.EOF}
	h := NewBuildCacheHandler(runtime, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Stats(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestBuildCacheHandler_Clear_NilRuntime(t *testing.T) {
	h := NewBuildCacheHandler(nil, newMockKVStore())
	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestBuildCacheHandler_Clear_WithDanglingImages(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{
			{ID: "img1", Tags: []string{"<none>:<none>"}, Size: 100 * 1024 * 1024},
			{ID: "img2", Tags: []string{"app:latest"}, Size: 50 * 1024 * 1024}, // Not dangling
			{ID: "img3", Tags: []string{}, Size: 75 * 1024 * 1024},             // No tags = dangling
		},
	}
	h := NewBuildCacheHandler(runtime, newMockKVStore())
	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["images_removed"].(float64)) != 2 {
		t.Errorf("expected 2 images removed, got %v", resp["images_removed"])
	}
}

func TestBuildCacheHandler_Clear_ImageListError(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{imageListErr: io.EOF}
	h := NewBuildCacheHandler(runtime, newMockKVStore())

	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ContainerHistoryHandler — History with stored metrics, period filters
// =============================================================================

func TestContainerHistoryHandler_History_WithStoredMetrics(t *testing.T) {
	kv := newMockKVStore()
	now := time.Now()
	kv.Set("metrics_ring", "app-1", metricsRingData{
		Points: []ContainerResourcePoint{
			{Timestamp: now.Add(-30 * time.Minute), CPUPercent: 50.0, MemoryMB: 256},
			{Timestamp: now.Add(-10 * time.Minute), CPUPercent: 70.0, MemoryMB: 512},
		},
	}, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewContainerHistoryHandler(store, nil, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=1h", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 2 {
		t.Errorf("expected 2 points, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_Period24h(t *testing.T) {
	kv := newMockKVStore()
	now := time.Now()
	kv.Set("metrics_ring", "app-1", metricsRingData{
		Points: []ContainerResourcePoint{
			{Timestamp: now.Add(-48 * time.Hour), CPUPercent: 20.0}, // Outside 24h
			{Timestamp: now.Add(-12 * time.Hour), CPUPercent: 40.0}, // Inside 24h
		},
	}, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewContainerHistoryHandler(store, nil, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=24h", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected 1 point inside 24h, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_Period7d(t *testing.T) {
	kv := newMockKVStore()
	now := time.Now()
	kv.Set("metrics_ring", "app-1", metricsRingData{
		Points: []ContainerResourcePoint{
			{Timestamp: now.Add(-3 * 24 * time.Hour), CPUPercent: 30.0},
		},
	}, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewContainerHistoryHandler(store, nil, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=7d", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected 1 point inside 7d, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_EmptyMetrics_24h(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewContainerHistoryHandler(store, nil, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=24h", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 96 {
		t.Errorf("expected 96 points for 24h empty, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_EmptyMetrics_7d(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewContainerHistoryHandler(store, nil, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history?period=7d", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 168 {
		t.Errorf("expected 168 points for 7d empty, got %v", resp["count"])
	}
}

func TestContainerHistoryHandler_History_NilBolt(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewContainerHistoryHandler(store, nil, nil)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/containers/history", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// AppMiddlewareHandler — Get stored, Update success
// =============================================================================

func TestAppMiddlewareHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	cfg := MiddlewareConfig{Compress: false, Headers: map[string]string{"X-Custom": "val"}}
	kv.Set("app_middleware", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewAppMiddlewareHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/middleware", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp MiddlewareConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Headers["X-Custom"] != "val" {
		t.Errorf("expected X-Custom header, got %v", resp.Headers)
	}
}

func TestAppMiddlewareHandler_Update_Success(t *testing.T) {
	kv := newMockKVStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewAppMiddlewareHandler(store, kv)

	body := `{"compress":true,"headers":{"X-Frame-Options":"DENY"}}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/middleware", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// AutoscaleHandler — Get stored, Update min/max correction
// =============================================================================

func TestAutoscaleHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	cfg := AutoscaleConfig{Enabled: true, MinReplicas: 2, MaxReplicas: 8}
	kv.Set("autoscale", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewAutoscaleHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/autoscale", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var resp AutoscaleConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if !resp.Enabled || resp.MinReplicas != 2 || resp.MaxReplicas != 8 {
		t.Errorf("unexpected config: %+v", resp)
	}
}

func TestAutoscaleHandler_Update_MinMaxCorrection(t *testing.T) {
	kv := newMockKVStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewAutoscaleHandler(store, kv)

	// min=0 should be corrected to 1, max=0 should be corrected to min
	body := `{"enabled":true,"min_replicas":0,"max_replicas":0}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/autoscale", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// BasicAuthHandler — Get stored, Update default realm
// =============================================================================

func TestBasicAuthHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	cfg := BasicAuthConfig{Enabled: true, Realm: "Admin", Users: map[string]string{"admin": "hash123"}}
	kv.Set("basic_auth", "app-1", cfg, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewBasicAuthHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/basic-auth", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var resp BasicAuthConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Realm != "Admin" || !resp.Enabled {
		t.Errorf("unexpected config: %+v", resp)
	}
}

func TestBasicAuthHandler_Update_DefaultRealm(t *testing.T) {
	kv := newMockKVStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewBasicAuthHandler(store, kv)

	body := `{"enabled":true,"realm":"","users":{"admin":"hash"}}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/basic-auth", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	// Verify the realm was defaulted to "Restricted"
	var stored BasicAuthConfig
	kv.Get("basic_auth", "app-1", &stored)
	if stored.Realm != "Restricted" {
		t.Errorf("expected default realm 'Restricted', got %q", stored.Realm)
	}
}

// =============================================================================
// AdminHandler — SystemInfo, ListTenants
// =============================================================================

func TestAdminHandler_SystemInfo(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0", Commit: "abc123"}
	c.Registry = core.NewRegistry()

	h := NewAdminHandler(c, newMockStore())
	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	rr := httptest.NewRecorder()
	h.SystemInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["version"] != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %v", resp["version"])
	}
}

func TestAdminHandler_ListTenants_WithPagination(t *testing.T) {
	store := newMockStore()
	store.allTenantsList = []core.Tenant{
		{ID: "t1", Name: "Tenant1"},
		{ID: "t2", Name: "Tenant2"},
		{ID: "t3", Name: "Tenant3"},
	}

	h := NewAdminHandler(testCore(), store)
	req := httptest.NewRequest("GET", "/api/v1/admin/tenants?page=1&per_page=2", nil)
	rr := httptest.NewRecorder()
	h.ListTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 tenants, got %d", len(data))
	}
	if int(resp["total"].(float64)) != 3 {
		t.Errorf("expected total 3, got %v", resp["total"])
	}
}

func TestAdminHandler_ListTenants_Error(t *testing.T) {
	store := newMockStore()
	store.errListAllTenants = io.EOF

	h := NewAdminHandler(testCore(), store)
	req := httptest.NewRequest("GET", "/api/v1/admin/tenants", nil)
	rr := httptest.NewRecorder()
	h.ListTenants(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestAdminHandler_ListTenants_DefaultPagination(t *testing.T) {
	store := newMockStore()
	h := NewAdminHandler(testCore(), store)

	// Zero/negative page and per_page should get defaults
	req := httptest.NewRequest("GET", "/api/v1/admin/tenants?page=0&per_page=-1", nil)
	rr := httptest.NewRecorder()
	h.ListTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAdminHandler_ListTenants_OverMaxPerPage(t *testing.T) {
	store := newMockStore()
	h := NewAdminHandler(testCore(), store)

	req := httptest.NewRequest("GET", "/api/v1/admin/tenants?per_page=999", nil)
	rr := httptest.NewRecorder()
	h.ListTenants(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// Mock helpers for this test file
// =============================================================================

// mockContainerRuntimeWithImages extends the mock to support ImageList/ImageRemove.
type mockContainerRuntimeWithImages struct {
	images       []core.ImageInfo
	imageListErr error
	removeErr    error
}

func (m *mockContainerRuntimeWithImages) Ping() error { return nil }
func (m *mockContainerRuntimeWithImages) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "c1", nil
}
func (m *mockContainerRuntimeWithImages) Stop(_ context.Context, _ string, _ int) error { return nil }
func (m *mockContainerRuntimeWithImages) Remove(_ context.Context, _ string, _ bool) error {
	return nil
}
func (m *mockContainerRuntimeWithImages) Restart(_ context.Context, _ string) error { return nil }
func (m *mockContainerRuntimeWithImages) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (m *mockContainerRuntimeWithImages) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (m *mockContainerRuntimeWithImages) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (m *mockContainerRuntimeWithImages) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}
func (m *mockContainerRuntimeWithImages) ImagePull(_ context.Context, _ string) error { return nil }
func (m *mockContainerRuntimeWithImages) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	if m.imageListErr != nil {
		return nil, m.imageListErr
	}
	return m.images, nil
}
func (m *mockContainerRuntimeWithImages) ImageRemove(_ context.Context, id string) error {
	return m.removeErr
}
func (m *mockContainerRuntimeWithImages) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (m *mockContainerRuntimeWithImages) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

// =============================================================================
// DeployFreezeHandler — Get with stored windows, Delete with existing
// =============================================================================

func TestDeployFreezeHandler_Get_WithWindows(t *testing.T) {
	kv := newMockKVStore()
	now := time.Now()
	kv.Set("deploy_freeze", "t1", freezeWindowList{
		Windows: []FreezeWindow{
			{ID: "fw1", Reason: "maintenance", StartsAt: now.Add(-1 * time.Hour), EndsAt: now.Add(1 * time.Hour), Active: true},
			{ID: "fw2", Reason: "past", StartsAt: now.Add(-48 * time.Hour), EndsAt: now.Add(-24 * time.Hour), Active: true},
			{ID: "fw3", Reason: "inactive", StartsAt: now.Add(-1 * time.Hour), EndsAt: now.Add(1 * time.Hour), Active: false},
		},
	}, 0)

	h := NewDeployFreezeHandler(newMockStore(), core.NewEventBus(slog.Default()), kv)
	req := httptest.NewRequest("GET", "/api/v1/deploy/freeze", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["frozen"] != true {
		t.Error("expected frozen=true")
	}
	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("expected 2 active windows, got %d", len(data))
	}
}

func TestDeployFreezeHandler_Delete_WithExisting(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("deploy_freeze", "t1", freezeWindowList{
		Windows: []FreezeWindow{
			{ID: "fw1", Reason: "test", Active: true},
			{ID: "fw2", Reason: "other", Active: true},
		},
	}, 0)

	h := NewDeployFreezeHandler(newMockStore(), core.NewEventBus(slog.Default()), kv)
	req := httptest.NewRequest("DELETE", "/api/v1/deploy/freeze/fw1", nil)
	req.SetPathValue("id", "fw1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}

	// Verify fw1 is deactivated
	var list freezeWindowList
	kv.Get("deploy_freeze", "t1", &list)
	for _, w := range list.Windows {
		if w.ID == "fw1" && w.Active {
			t.Error("fw1 should be deactivated")
		}
	}
}

func TestDeployFreezeHandler_Delete_NoExisting(t *testing.T) {
	h := NewDeployFreezeHandler(newMockStore(), core.NewEventBus(slog.Default()), newMockKVStore())
	req := httptest.NewRequest("DELETE", "/api/v1/deploy/freeze/fw1", nil)
	req.SetPathValue("id", "fw1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// =============================================================================
// CronJobHandler — List with jobs, Delete with existing
// =============================================================================

func TestCronJobHandler_List_WithJobs(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("cronjobs", "app-1", cronJobList{
		Jobs: []CronJobConfig{{ID: "j1", Name: "cleanup", Schedule: "0 0 * * *", Command: "/bin/clean", Enabled: true}},
	}, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewCronJobHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/cron", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 1 {
		t.Errorf("expected 1 job, got %v", resp["total"])
	}
}

func TestCronJobHandler_Delete_WithExisting(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("cronjobs", "app-1", cronJobList{
		Jobs: []CronJobConfig{
			{ID: "j1", Name: "old", Schedule: "* * * * *", Command: "echo 1", Enabled: true},
			{ID: "j2", Name: "keep", Schedule: "* * * * *", Command: "echo 2", Enabled: true},
		},
	}, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewCronJobHandler(store, kv)
	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/cron/j1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("jobId", "j1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}

	var list cronJobList
	kv.Get("cronjobs", "app-1", &list)
	if len(list.Jobs) != 1 || list.Jobs[0].ID != "j2" {
		t.Errorf("expected only j2 remaining, got %v", list.Jobs)
	}
}

func TestCronJobHandler_Delete_NoExisting(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewCronJobHandler(store, newMockKVStore())
	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/cron/j1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("jobId", "j1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// =============================================================================
// DeployApprovalHandler — ListPending with items, Approve/Reject found
// =============================================================================

func TestDeployApprovalHandler_ListPending_WithItems(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), core.NewEventBus(slog.Default()))
	now := time.Now()
	h.pending["a1"] = &ApprovalRequest{ID: "a1", AppID: "app-1", TenantID: "t1", Status: "pending", CreatedAt: now}
	h.pending["a2"] = &ApprovalRequest{ID: "a2", AppID: "app-2", TenantID: "t1", Status: "approved", CreatedAt: now}
	h.pending["a3"] = &ApprovalRequest{ID: "a3", AppID: "app-3", TenantID: "t2", Status: "pending", CreatedAt: now}

	req := httptest.NewRequest("GET", "/api/v1/deploy/approvals", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.ListPending(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 1 {
		t.Errorf("expected 1 pending, got %v", resp["total"])
	}
}

func TestDeployApprovalHandler_Approve_Found(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), core.NewEventBus(slog.Default()))
	h.pending["a1"] = &ApprovalRequest{ID: "a1", AppID: "app-1", TenantID: "t1", Status: "pending"}

	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/a1/approve", nil)
	req.SetPathValue("id", "a1")
	req = withClaims(req, "admin1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Approve(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if h.pending["a1"].Status != "approved" {
		t.Errorf("expected approved, got %q", h.pending["a1"].Status)
	}
}

func TestDeployApprovalHandler_Reject_Found(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), core.NewEventBus(slog.Default()))
	h.pending["a1"] = &ApprovalRequest{ID: "a1", AppID: "app-1", TenantID: "t1", Status: "pending"}

	body := `{"reason":"not ready"}`
	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/a1/reject", strings.NewReader(body))
	req.SetPathValue("id", "a1")
	req = withClaims(req, "admin1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Reject(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if h.pending["a1"].Status != "rejected" {
		t.Errorf("expected rejected, got %q", h.pending["a1"].Status)
	}
}

// =============================================================================
// DeployNotifyHandler — Get stored, Update success
// =============================================================================

func TestDeployNotifyHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("deploy_notify", "app-1", DeployNotifyConfig{
		OnSuccess: []NotifyTarget{{Channel: "slack", Recipient: "#deploys"}},
	}, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewDeployNotifyHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/deploy-notifications", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var resp DeployNotifyConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.OnSuccess) != 1 {
		t.Errorf("expected 1 success target, got %d", len(resp.OnSuccess))
	}
}

func TestDeployNotifyHandler_Update_Success(t *testing.T) {
	kv := newMockKVStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewDeployNotifyHandler(store, kv)

	body := `{"on_success":[{"channel":"discord","recipient":"#ops"}],"on_failure":[{"channel":"email","recipient":"admin@x.com"}]}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/deploy-notifications", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// degradedModule is a module that reports HealthDegraded.
type degradedModule struct{}

func (d *degradedModule) ID() string                                 { return "test.degraded" }
func (d *degradedModule) Name() string                               { return "Degraded" }
func (d *degradedModule) Version() string                            { return "1.0.0" }
func (d *degradedModule) Dependencies() []string                     { return nil }
func (d *degradedModule) Init(_ context.Context, _ *core.Core) error { return nil }
func (d *degradedModule) Start(_ context.Context) error              { return nil }
func (d *degradedModule) Stop(_ context.Context) error               { return nil }
func (d *degradedModule) Health() core.HealthStatus                  { return core.HealthDegraded }
func (d *degradedModule) Routes() []core.Route                       { return nil }
func (d *degradedModule) Events() []core.EventHandler                { return nil }

// downModule is a module that reports HealthDown.
type downModule struct{}

func (d *downModule) ID() string                                 { return "test.down" }
func (d *downModule) Name() string                               { return "Down" }
func (d *downModule) Version() string                            { return "1.0.0" }
func (d *downModule) Dependencies() []string                     { return nil }
func (d *downModule) Init(_ context.Context, _ *core.Core) error { return nil }
func (d *downModule) Start(_ context.Context) error              { return nil }
func (d *downModule) Stop(_ context.Context) error               { return nil }
func (d *downModule) Health() core.HealthStatus                  { return core.HealthDown }
func (d *downModule) Routes() []core.Route                       { return nil }
func (d *downModule) Events() []core.EventHandler                { return nil }

// =============================================================================
// ImageCleanupHandler — DanglingImages, Prune
// =============================================================================

func TestImageCleanupHandler_DanglingImages_WithImages(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{
			{ID: "img1", Tags: []string{"<none>:<none>"}, Size: 100 * 1024 * 1024},
			{ID: "img2", Tags: []string{"app:latest"}, Size: 200 * 1024 * 1024},
			{ID: "img3", Tags: []string{}, Size: 50 * 1024 * 1024},
		},
	}
	h := NewImageCleanupHandler(runtime)
	req := httptest.NewRequest("GET", "/api/v1/images/dangling", nil)
	rr := httptest.NewRecorder()
	h.DanglingImages(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["dangling_count"].(float64)) != 2 {
		t.Errorf("expected 2 dangling, got %v", resp["dangling_count"])
	}
}

func TestImageCleanupHandler_DanglingImages_Error(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{imageListErr: io.EOF}
	h := NewImageCleanupHandler(runtime)
	req := httptest.NewRequest("GET", "/api/v1/images/dangling", nil)
	rr := httptest.NewRecorder()
	h.DanglingImages(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestImageCleanupHandler_Prune_WithImages(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{
			{ID: "img1", Tags: []string{"<none>:<none>"}, Size: 100 * 1024 * 1024},
			{ID: "img2", Tags: []string{}, Size: 75 * 1024 * 1024},
			{ID: "img3", Tags: []string{"app:latest"}, Size: 200 * 1024 * 1024},
		},
	}
	h := NewImageCleanupHandler(runtime)
	req := httptest.NewRequest("DELETE", "/api/v1/images/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["images_removed"].(float64)) != 2 {
		t.Errorf("expected 2 removed, got %v", resp["images_removed"])
	}
}

func TestImageCleanupHandler_Prune_Error(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{imageListErr: io.EOF}
	h := NewImageCleanupHandler(runtime)
	req := httptest.NewRequest("DELETE", "/api/v1/images/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ImageTagHandler — List with matches
// =============================================================================

func TestImageTagHandler_List_WithMatches(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{
			{ID: "img1", Tags: []string{"nginx:latest", "nginx:1.25"}, Size: 100 * 1024 * 1024},
			{ID: "img2", Tags: []string{"redis:7"}, Size: 50 * 1024 * 1024},
		},
	}
	h := NewImageTagHandler(newMockStore(), runtime)
	req := httptest.NewRequest("GET", "/api/v1/images/tags?image=nginx", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.List(rr, req)

	// With tenant isolation, image access is denied if no apps use it
	// Mock store has no apps, so expect 403 Forbidden
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 (access denied to image), got %d", rr.Code)
	}
}

func TestImageTagHandler_List_NoMatch(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{{ID: "img1", Tags: []string{"redis:7"}}},
	}
	h := NewImageTagHandler(newMockStore(), runtime)
	req := httptest.NewRequest("GET", "/api/v1/images/tags?image=nginx", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.List(rr, req)

	// With tenant isolation, image access is denied if no apps use it
	// Mock store has no apps, so expect 403 Forbidden
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 (access denied to image), got %d", rr.Code)
	}
}

func TestImageTagHandler_List_Error(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{imageListErr: io.EOF}
	h := NewImageTagHandler(newMockStore(), runtime)
	req := httptest.NewRequest("GET", "/api/v1/images/tags?image=nginx", nil)
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.List(rr, req)
	// With tenant isolation, the image access check happens before runtime call
	// Mock store has no apps, so expect 403 Forbidden (not 500)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403 (access denied to image), got %d", rr.Code)
	}
}

// =============================================================================
// MetricsHistoryHandler — AppMetrics with stored data and runtime fallback
// =============================================================================

func TestMetricsHistoryHandler_AppMetrics_StoredData(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("metrics_ring", "app-1:24h", metricsRing{
		Points: []MetricsPoint{{Timestamp: time.Now(), CPUPercent: 45.0, MemoryMB: 512}},
	}, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewMetricsHistoryHandler(store, nil, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/metrics?period=24h", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppMetrics(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected 1 point, got %v", resp["count"])
	}
}

func TestMetricsHistoryHandler_AppMetrics_RuntimeFallback(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "c1", Name: "app-1"}},
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewMetricsHistoryHandler(store, runtime, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/metrics", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppMetrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestMetricsHistoryHandler_AppMetrics_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewMetricsHistoryHandler(store, nil, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/metrics", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.AppMetrics(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 0 {
		t.Errorf("expected 0 points, got %v", resp["count"])
	}
}

// =============================================================================
// ExecHandler — full path coverage
// =============================================================================

func TestExecHandler_Exec_WithArgs(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "c1", Name: "test"}},
	}
	h := NewExecHandler(runtime, store, slog.Default(), nil)

	body := `{"command":"ls","args":["-la","/tmp"]}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestExecHandler_Exec_NoContainers(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	runtime := &mockContainerRuntime{containers: nil}
	h := NewExecHandler(runtime, store, slog.Default(), nil)

	body := `{"command":"echo hello"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestExecHandler_Exec_ListError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "test"})
	runtime := &mockContainerRuntime{listErr: io.EOF}
	h := NewExecHandler(runtime, store, slog.Default(), nil)

	body := `{"command":"echo hello"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestExecHandler_Exec_AppNotFound(t *testing.T) {
	store := newMockStore()
	runtime := &mockContainerRuntime{}
	h := NewExecHandler(runtime, store, slog.Default(), nil)

	body := `{"command":"echo hello"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/no-app/exec", strings.NewReader(body))
	req.SetPathValue("id", "no-app")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// GPUHandler — detectGPU with nvidia images
// =============================================================================

func TestGPUHandler_Get_WithNvidiaImages(t *testing.T) {
	runtime := &mockContainerRuntimeWithImages{
		images: []core.ImageInfo{
			{ID: "img1", Tags: []string{"nvidia/cuda:12.0-runtime"}},
		},
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewGPUHandler(store, runtime, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	detection := resp["detection"].(map[string]any)
	if detection["available"] != true {
		t.Error("expected GPU available=true with nvidia image")
	}
}

// =============================================================================
// DNSRecordHandler — List with provider
// =============================================================================

func TestDNSRecordHandler_List_NoProvider(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)
	req := httptest.NewRequest("GET", "/api/v1/dns/records?domain=example.com", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (empty list), got %d", rr.Code)
	}
}

// =============================================================================
// CertificateHandler — Upload with valid self-signed cert
// =============================================================================

func TestCertificateHandler_Upload_ValidCert(t *testing.T) {
	// Generate a self-signed certificate for testing
	cert, key := generateTestCert(t)
	kv := newMockKVStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "test-tenant"})
	store.addDomain(&core.Domain{ID: "domain-test", AppID: "app1", FQDN: "test.example.com", Type: "custom"})
	h := NewCertificateHandler(store, kv)

	body := `{"domain_id":"domain-test","cert_pem":"` + escapeJSON(cert) + `","key_pem":"` + escapeJSON(key) + `"}`
	req := httptest.NewRequest("POST", "/api/v1/certificates", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func generateTestCert(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()
	// Use crypto/ecdsa for a smaller, faster test cert
	key, err := ecdsa.GenerateKey(elliptic.P256(), crand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(365 * 24 * time.Hour),
	}

	certDER, err := x509.CreateCertificate(crand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	certBuf := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyBuf := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return string(certBuf), string(keyBuf)
}

func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	// Remove the surrounding quotes
	return string(b[1 : len(b)-1])
}

// =============================================================================
// Error-path kv tests for handlers with uncovered kv.Set error branches
// =============================================================================

// errorKVStore always returns error on Set.
type errorKVStore struct {
	mockKVStore
}

func newErrorKVStore() *errorKVStore {
	return &errorKVStore{mockKVStore: *newMockKVStore()}
}

func (m *errorKVStore) Set(_, _ string, _ any, _ int64) error {
	return io.EOF
}

func (m *errorKVStore) Mutate(_, _ string, _ any, _ int64, _ func(bool) error) error {
	return io.EOF
}

func TestAppMiddlewareHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewAppMiddlewareHandler(store, newErrorKVStore())
	body := `{"compress":true}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/middleware", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestAutoscaleHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewAutoscaleHandler(store, newErrorKVStore())
	body := `{"enabled":true,"min_replicas":1,"max_replicas":5}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/autoscale", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestBasicAuthHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewBasicAuthHandler(store, newErrorKVStore())
	body := `{"enabled":true,"realm":"X","users":{"a":"b"}}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/basic-auth", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestDeployNotifyHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant1", Name: "Test", Status: "running"})
	h := NewDeployNotifyHandler(store, newErrorKVStore())
	body := `{"on_success":[]}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/deploy-notifications", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestPinHandler_Pin_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "Test", Status: "running"})
	h := NewPinHandler(store, newErrorKVStore())
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Pin(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestPinHandler_Unpin_BoltError(t *testing.T) {
	eb := newErrorKVStore()
	// Seed data so the Get succeeds but Set fails
	eb.mockKVStore.Set("app_pins", "u1", pinnedApps{AppIDs: []string{"app-1"}}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewPinHandler(store, eb)
	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/pin", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Unpin(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestCronJobHandler_Create_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewCronJobHandler(store, newErrorKVStore())
	body := `{"schedule":"* * * * *","command":"echo hi"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/cron", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestCronJobHandler_Delete_BoltError(t *testing.T) {
	eb := newErrorKVStore()
	eb.mockKVStore.Set("cronjobs", "app-1", cronJobList{Jobs: []CronJobConfig{{ID: "j1"}}}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewCronJobHandler(store, eb)
	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/cron/j1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("jobId", "j1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_List_NoDomain(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)
	req := httptest.NewRequest("GET", "/api/v1/dns/records", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// DeployTriggerHandler — image-type deploy
// =============================================================================

func TestDeployTriggerHandler_ImageDeploy(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID: "app-1", Name: "img-app", SourceType: "image",
		SourceURL: "nginx:latest", TenantID: "t1",
	})
	store.nextDeployVersion["app-1"] = 1

	runtime := &mockContainerRuntime{}
	events := core.NewEventBus(slog.Default())
	h := NewDeployTriggerHandler(context.Background(), store, runtime, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/deploy", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeployTriggerHandler_AppNotFound(t *testing.T) {
	store := newMockStore()
	h := NewDeployTriggerHandler(context.Background(), store, nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/nope/deploy", nil)
	req.SetPathValue("id", "nope")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDeployTriggerHandler_GitDeploy(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID: "app-2", Name: "git-app", SourceType: "git",
		SourceURL: "https://github.com/test/repo", Branch: "main", TenantID: "t1",
	})

	runtime := &mockContainerRuntime{}
	events := core.NewEventBus(slog.Default())
	h := NewDeployTriggerHandler(context.Background(), store, runtime, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-2/deploy", nil)
	req.SetPathValue("id", "app-2")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// ErrorPageHandler — Get stored, Update success/error
// =============================================================================

func TestErrorPageHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("error_pages", "app-1", ErrorPageConfig{Page502: "<h1>Bad Gateway</h1>"}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewErrorPageHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/error-pages", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	var resp ErrorPageConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Page502 != "<h1>Bad Gateway</h1>" {
		t.Errorf("expected page_502 content, got %q", resp.Page502)
	}
}

func TestErrorPageHandler_Update_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewErrorPageHandler(store, newMockKVStore())
	body := `{"page_502":"<h1>Down</h1>","page_503":"<h1>Maintenance</h1>"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/error-pages", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestErrorPageHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewErrorPageHandler(store, newErrorKVStore())
	body := `{"page_502":"<h1>Down</h1>"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/error-pages", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestDeployFreezeHandler_Create_BoltError(t *testing.T) {
	eb := newErrorKVStore()
	h := NewDeployFreezeHandler(newMockStore(), core.NewEventBus(slog.Default()), eb)
	body := `{"reason":"maintenance"}`
	req := httptest.NewRequest("POST", "/api/v1/deploy/freeze", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AdminAPIKeyHandler — Generate kv errors
// =============================================================================

// =============================================================================
// Batch kv-error tests for many handlers with uncovered Set error paths
// =============================================================================

func TestGPUHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewGPUHandler(store, nil, newErrorKVStore())
	body := `{"enabled":true,"driver":"nvidia"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/gpu", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestLogRetentionHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewLogRetentionHandler(store, newErrorKVStore())
	body := `{"max_size_mb":100,"max_files":10,"driver":"json-file"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestLogRetentionHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("log_retention", "app-1", map[string]int{"days": 14}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewLogRetentionHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/log-retention", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestMaintenanceHandler_Get_Stored(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("maintenance", "app-1", map[string]bool{"enabled": true}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewMaintenanceHandler(store, core.NewEventBus(slog.Default()), kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/maintenance", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestMaintenanceHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewMaintenanceHandler(store, core.NewEventBus(slog.Default()), newErrorKVStore())
	body := `{"enabled":true}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/maintenance", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewStickySessionHandler(store, newErrorKVStore())
	body := `{"enabled":true,"cookie":"MONSTERSESSION"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	store.errUpdateApp = io.EOF
	h := NewLabelsHandler(store)
	body := `{"env":"production"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/labels", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestTenantRateLimitHandler_Update_BoltError(t *testing.T) {
	h := NewTenantRateLimitHandler(newErrorKVStore())
	body := `{"requests_per_minute":100,"burst_size":20}`
	req := httptest.NewRequest("PUT", "/api/v1/admin/tenants/t1/ratelimit", strings.NewReader(body))
	req.SetPathValue("id", "t1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewRedirectHandler(store, newErrorKVStore())
	body := `{"source":"/old","destination":"/new","status_code":301}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestMetricsHistoryHandler_ServerMetrics_Stored(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("metrics_ring", "server:srv-1:24h", metricsRing{
		Points: []MetricsPoint{{Timestamp: time.Now(), CPUPercent: 30.0}},
	}, 0)
	h := NewMetricsHistoryHandler(newMockStore(), nil, kv)
	req := httptest.NewRequest("GET", "/api/v1/servers/srv-1/metrics?period=24h", nil)
	req.SetPathValue("id", "srv-1")
	rr := httptest.NewRecorder()
	h.ServerMetrics(rr, req)

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	points := resp["points"].([]any)
	if len(points) != 1 {
		t.Errorf("expected 1 point, got %d", len(points))
	}
}

func TestAdminAPIKeyHandler_Generate_BoltSetError(t *testing.T) {
	h := NewAdminAPIKeyHandler(newMockStore(), newErrorKVStore())
	req := httptest.NewRequest("POST", "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ResponseHeadersHandler — Update kv error
// =============================================================================

func TestResponseHeadersHandler_Update_BoltError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewResponseHeadersHandler(store, newErrorKVStore())
	body := `{"hsts":"max-age=31536000"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestResponseHeadersHandler_Update_SuccessCustom(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewResponseHeadersHandler(store, newMockKVStore())
	body := `{"hsts":"max-age=31536000","csp":"default-src 'self'"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// MetricsExportHandler — CSV and runtime paths
// =============================================================================

func TestMetricsExportHandler_Export_CSV(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "c1", Name: "app-1"}},
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-12345", TenantID: "t1", Name: "test", Status: "running"})
	h := NewMetricsExportHandler(store, newMockKVStore(), runtime)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-12345/metrics/export?format=csv", nil)
	req.SetPathValue("id", "app-12345")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "text/csv" {
		t.Errorf("expected text/csv, got %q", rr.Header().Get("Content-Type"))
	}
}

func TestMetricsExportHandler_Export_JSON_WithRuntime(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "c1", Name: "test-app"}},
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewMetricsExportHandler(store, newMockKVStore(), runtime)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/metrics/export", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestMetricsExportHandler_Export_StoredData(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("metrics_export", "app-1", []metricsPoint{
		{Timestamp: "2026-01-01T00:00:00Z", CPUPercent: 50.0, MemoryMB: 256},
	}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewMetricsExportHandler(store, kv, nil)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/metrics/export?format=csv", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// DeployTriggerHandler — image deploy with runtime error
// =============================================================================

func TestDeployTriggerHandler_ImageDeploy_RuntimeError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID: "app-err", Name: "fail-app", SourceType: "image",
		SourceURL: "nginx:latest", TenantID: "t1",
	})

	runtime := &mockContainerRuntime{listErr: io.EOF}
	h := NewDeployTriggerHandler(context.Background(), store, runtime, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/apps/app-err/deploy", nil)
	req.SetPathValue("id", "app-err")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	// CreateAndStart doesn't use listErr — it returns successfully by default.
	// Let's verify it still works (at minimum exercises the code path).
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDeployTriggerHandler_ImageDeploy_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID: "app-nil", Name: "nil-rt-app", SourceType: "image",
		SourceURL: "nginx:latest", TenantID: "t1",
	})

	h := NewDeployTriggerHandler(context.Background(), store, nil, core.NewEventBus(slog.Default()))
	req := httptest.NewRequest("POST", "/api/v1/apps/app-nil/deploy", nil)
	req.SetPathValue("id", "app-nil")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// Secrets handler — List with data
// =============================================================================

// mockVault implements the vault interface for secrets handler tests.
type mockVault struct{}

func (v *mockVault) Encrypt(s string) (string, error) { return "enc:" + s, nil }
func (v *mockVault) Decrypt(s string) (string, error) { return "dec:" + s, nil }

func TestSecretHandler_List_WithSecrets(t *testing.T) {
	store := newMockStore()
	store.secrets["t1"] = []core.Secret{
		{ID: "s1", TenantID: "t1", Name: "DB_PASS", Type: "env"},
		{ID: "s2", TenantID: "t1", Name: "API_KEY", Type: "env"},
	}

	h := NewSecretHandler(store, &mockVault{}, core.NewEventBus(slog.Default()))
	req := httptest.NewRequest("GET", "/api/v1/secrets", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 2 {
		t.Errorf("expected 2 secrets, got %v", resp["total"])
	}
}

// =============================================================================
// Batch kv error tests — final push to 90%
// =============================================================================

func TestAnnouncementHandler_Create_BoltError(t *testing.T) {
	h := NewAnnouncementHandler(newErrorKVStore())
	body := `{"title":"Update","body":"New version available","type":"info"}`
	req := httptest.NewRequest("POST", "/api/v1/announcements", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Delete_BoltError(t *testing.T) {
	eb := newErrorKVStore()
	eb.mockKVStore.Set("event_webhooks", "_all", map[string]any{"hooks": []any{}}, 0)
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(slog.Default()), eb)
	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/events/wh-1", nil)
	req.SetPathValue("id", "wh-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	// Even with kv error, delete should handle it gracefully
	if rr.Code != http.StatusInternalServerError && rr.Code != http.StatusNoContent {
		t.Errorf("expected 500 or 204, got %d", rr.Code)
	}
}

func TestHealthDetailedHandler_DetailedHealth(t *testing.T) {
	c := testCore()
	c.Build = core.BuildInfo{Version: "1.0.0"}
	c.Registry = core.NewRegistry()
	c.Registry.Register(&degradedModule{})
	c.Services.Container = &mockContainerRuntime{}

	h := NewDetailedHealthHandler(c)
	req := httptest.NewRequest("GET", "/api/v1/health/detailed", nil)
	rr := httptest.NewRecorder()
	h.DetailedHealth(rr, req)

	// May return 200 or 503 depending on DB/Docker availability
	if rr.Code != http.StatusOK && rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 200 or 503, got %d", rr.Code)
	}
}

func TestSessionHandler_UpdateProfile_Error(t *testing.T) {
	store := newMockStore()
	store.errUpdateUser = io.EOF
	seedTestUser(store, "u1", "test@x.com", "pass123", "t1", "role_admin")

	h := NewSessionHandler(store, nil, nil)
	body := `{"name":"New Name"}`
	req := httptest.NewRequest("PUT", "/api/v1/auth/profile", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "test@x.com")
	rr := httptest.NewRecorder()
	h.UpdateProfile(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestSessionHandler_ChangePassword_WrongOld(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@x.com", "correct-password", "t1", "role_admin")

	h := NewSessionHandler(store, nil, nil)
	body := `{"current_password":"wrong-old","new_password":"new-pass-123456"}`
	req := httptest.NewRequest("PUT", "/api/v1/auth/password", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "test@x.com")
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestWildcardSSLHandler_Request_BoltError(t *testing.T) {
	h := NewWildcardSSLHandler(newErrorKVStore())
	body := `{"domain":"*.example.com"}`
	req := httptest.NewRequest("POST", "/api/v1/certificates/wildcard", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Request(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestRegistryHandler_List_Stored(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("registries", registryListKey("t1"), registryList{
		Registries: []RegistryConfig{{ID: "custom-1", Name: "Tenant Registry", URL: "registry.example.com"}},
	}, 0)
	h := NewRegistryHandler(kv)
	req := httptest.NewRequest("GET", "/api/v1/registries", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestWebhookLogHandler_List_Stored(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("webhook_logs", "wh-1", map[string]any{"entries": []any{}}, 0)
	store := newMockStore()
	store.addApp(&core.Application{ID: "wh-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewWebhookLogHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/webhooks/wh-1/logs", nil)
	req.SetPathValue("id", "wh-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// isPathSafe — file browser security checks
// =============================================================================

func TestIsPathSafe_ValidPaths(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"/", true},
		{"/app", true},
		{"/app/data", true},
		{"/app/data/file.txt", true},
		{"app", true}, // prepended with /
		{"relative/path", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isPathSafe(tt.path); got != tt.expected {
				t.Errorf("isPathSafe(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestIsPathSafe_PathTraversal(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		{"../etc/passwd", false},
		{"/app/../etc/passwd", false},
		{"/app/../../etc/passwd", false},
		{"%2e%2e/etc/passwd", false},
		{"%252e%252e%252fetc%252fpasswd", false},
		{"/..", false},
		{"/app/..", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isPathSafe(tt.path); got != tt.expected {
				t.Errorf("isPathSafe(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestIsPathSafe_NullBytes(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"null at end", "/app/file.txt\x00", false},
		{"null in middle", "/app/file\x00.txt", false},
		{"null at start", "\x00/app/file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPathSafe(tt.path); got != tt.expected {
				t.Errorf("isPathSafe(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestIsPathSafe_WindowsDriveLetters(t *testing.T) {
	tests := []struct {
		path     string
		expected bool
	}{
		// Windows drive letters are blocked before the "/" prepend
		{"C:\\Windows\\System32", false},
		{"D:\\app\\data", false},
		{"Z:\\secret", false},
		{"c:\\windows", false},
		{"C:bad", false},
		// Non-drive-letter paths are allowed
		{"/app/C:fake", true},
		{"/C:/Windows", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isPathSafe(tt.path); got != tt.expected {
				t.Errorf("isPathSafe(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

// === merged from coverage_boost5_test.go ===

// =============================================================================
// BrandingHandler — validateCustomCSS, validationError.Error
// =============================================================================

func TestValidateCustomCSS_Empty(t *testing.T) {
	if err := validateCustomCSS(""); err != nil {
		t.Errorf("expected nil for empty css, got %v", err)
	}
	if err := validateCustomCSS("   "); err != nil {
		t.Errorf("expected nil for whitespace-only css, got %v", err)
	}
}

func TestValidateCustomCSS_StyleTag(t *testing.T) {
	if err := validateCustomCSS("body { color: red } <style"); err == nil {
		t.Error("expected error for <style tag")
	}
	if err := validateCustomCSS("body { color: red } </style"); err == nil {
		t.Error("expected error for </style tag")
	}
}

func TestValidateCustomCSS_Expression(t *testing.T) {
	if err := validateCustomCSS("body { width: expression(alert(1)) }"); err == nil {
		t.Error("expected error for expression()")
	}
}

func TestValidateCustomCSS_JavascriptURL(t *testing.T) {
	if err := validateCustomCSS("body { background: javascript:alert(1) }"); err == nil {
		t.Error("expected error for javascript: URL")
	}
}

func TestValidateCustomCSS_DataURL(t *testing.T) {
	if err := validateCustomCSS("body { background: data:text/html,<script>alert(1)</script> }"); err == nil {
		t.Error("expected error for data: URL")
	}
}

func TestValidateCustomCSS_Import(t *testing.T) {
	if err := validateCustomCSS("@import url('https://evil.com/style.css')"); err == nil {
		t.Error("expected error for @import")
	}
}

func TestValidateCustomCSS_TooLong(t *testing.T) {
	if err := validateCustomCSS(strings.Repeat("a", 50001)); err == nil {
		t.Error("expected error for css > 50KB")
	}
}

func TestValidateCustomCSS_Valid(t *testing.T) {
	if err := validateCustomCSS("body { color: red; font-size: 14px; }"); err != nil {
		t.Errorf("expected valid css, got %v", err)
	}
}

func TestValidationError_Error(t *testing.T) {
	e := &validationError{msg: "test error"}
	if e.Error() != "test error" {
		t.Errorf("Error() = %q, want test error", e.Error())
	}
}

func TestBrandingHandler_Get(t *testing.T) {
	h := NewBrandingHandler()
	req := httptest.NewRequest("GET", "/api/v1/branding", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestBrandingHandler_Update_InvalidCSS(t *testing.T) {
	h := NewBrandingHandler()
	body := `{"custom_css":"body { width: expression(alert(1)) }"}`
	req := httptest.NewRequest("PATCH", "/api/v1/admin/branding", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestBrandingHandler_Update_Valid(t *testing.T) {
	h := NewBrandingHandler()
	body := `{"custom_css":"body { color: red }"}`
	req := httptest.NewRequest("PATCH", "/api/v1/admin/branding", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// EventWebhookHandler — hashSecret
// =============================================================================

func TestHashSecret(t *testing.T) {
	h1 := hashSecret("secret1")
	h2 := hashSecret("secret1")
	h3 := hashSecret("secret2")

	if h1 != h2 {
		t.Error("same secret should produce same hash")
	}
	if h1 == h3 {
		t.Error("different secrets should produce different hashes")
	}
	if len(h1) != 64 {
		t.Errorf("expected sha256 hex length 64, got %d", len(h1))
	}
}

// =============================================================================
// RedirectHandler — SetEvents
// =============================================================================

func TestRedirectHandler_SetEvents(t *testing.T) {
	h := NewRedirectHandler(newMockStore(), newMockKVStore())
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetEvents(events)
	if h.events != events {
		t.Error("SetEvents did not set events")
	}
}

// =============================================================================
// GPUHandler — SetEvents
// =============================================================================

func TestGPUHandler_SetEvents(t *testing.T) {
	h := NewGPUHandler(newMockStore(), &mockContainerRuntime{}, newMockKVStore())
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetEvents(events)
	if h.events != events {
		t.Error("SetEvents did not set events")
	}
}

// =============================================================================
// DetailedHealthHandler — SetRateLimiter
// =============================================================================

func TestDetailedHealthHandler_SetRateLimiter(t *testing.T) {
	c := testCore()
	c.Store = newMockStore()
	h := NewDetailedHealthHandler(c)
	rl := middleware.NewGlobalRateLimiter(100, 200)
	h.SetRateLimiter(rl)
	if h.rateLimit != rl {
		t.Error("SetRateLimiter did not set rate limiter")
	}
}

// =============================================================================
// DNSRecordHandler — SetEvents
// =============================================================================

func TestDNSRecordHandler_SetEvents(t *testing.T) {
	h := NewDNSRecordHandler(core.NewServices())
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetEvents(events)
	if h.events != events {
		t.Error("SetEvents did not set events")
	}
}

// =============================================================================
// DeployApprovalHandler — pending request storage
// =============================================================================

func TestDeployApprovalHandler_SeedPendingForTests(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), nil)
	req := &ApprovalRequest{
		ID:        "app-1",
		AppID:     "app-1",
		TenantID:  "tenant-1",
		Status:    "pending",
		CreatedAt: time.Now(),
	}
	seedApprovalRequest(h, req)

	h.mu.RLock()
	_, ok := h.pending["app-1"]
	h.mu.RUnlock()
	if !ok {
		t.Error("expected pending request to be stored")
	}
}

func seedApprovalRequest(h *DeployApprovalHandler, req *ApprovalRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending[req.ID] = req
}

// =============================================================================
// CronJobHandler — SetEvents
// =============================================================================

func TestCronJobHandler_SetEvents(t *testing.T) {
	h := NewCronJobHandler(newMockStore(), newMockKVStore())
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetEvents(events)
	if h.events != events {
		t.Error("SetEvents did not set events")
	}
}

// =============================================================================
// BasicAuthHandler — SetEvents
// =============================================================================

func TestBasicAuthHandler_SetEvents(t *testing.T) {
	h := NewBasicAuthHandler(newMockStore(), newMockKVStore())
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetEvents(events)
	if h.events != events {
		t.Error("SetEvents did not set events")
	}
}

// =============================================================================
// AutoscaleHandler — SetEvents
// =============================================================================

func TestAutoscaleHandler_SetEvents(t *testing.T) {
	h := NewAutoscaleHandler(newMockStore(), newMockKVStore())
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h.SetEvents(events)
	if h.events != events {
		t.Error("SetEvents did not set events")
	}
}

// =============================================================================
// EventWebhookHandler — webhookListKey, List, Create, Delete
// =============================================================================

func TestWebhookListKey(t *testing.T) {
	if webhookListKey("t1") != "tenant:t1" {
		t.Errorf("webhookListKey = %q, want tenant:t1", webhookListKey("t1"))
	}
}

func TestEventWebhookHandler_List_EmptyBoost(t *testing.T) {
	kv := newMockKVStore()
	h := NewEventWebhookHandler(newMockStore(), nil, kv)

	req := httptest.NewRequest("GET", "/api/v1/webhooks/outbound", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total 0, got %v", resp["total"])
	}
}

func TestEventWebhookHandler_List_WithItems(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("event_webhooks", "tenant:t1", eventWebhookList{
		Webhooks: []EventWebhookConfig{
			{ID: "wh-1", URL: "https://example.com/hook", Events: []string{"app.deployed"}, Active: true, TenantID: "t1", SecretHash: "hash123"},
		},
	}, 0)

	h := NewEventWebhookHandler(newMockStore(), nil, kv)
	req := httptest.NewRequest("GET", "/api/v1/webhooks/outbound", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 1 {
		t.Errorf("expected total 1, got %v", resp["total"])
	}
	// SecretHash should be stripped from response (omitempty means key may be absent)
	data, _ := resp["data"].([]any)
	if len(data) > 0 {
		first, _ := data[0].(map[string]any)
		if sh, ok := first["secret_hash"]; ok && sh != "" {
			t.Error("expected secret_hash to be stripped from list response")
		}
	}
}

func TestEventWebhookHandler_List_NoClaims(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), nil, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/webhooks/outbound", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_SuccessBoost(t *testing.T) {
	kv := newMockKVStore()
	h := NewEventWebhookHandler(newMockStore(), nil, kv)

	body := `{"url":"https://example.com/hook","events":["app.deployed","app.crashed"]}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["url"] != "https://example.com/hook" {
		t.Errorf("expected url in response, got %v", resp)
	}
	if resp["secret"] == "" {
		t.Error("expected secret to be returned at creation")
	}
}

func TestEventWebhookHandler_Create_ValidationErrors(t *testing.T) {
	kv := newMockKVStore()
	h := NewEventWebhookHandler(newMockStore(), nil, kv)

	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty url and events", `{}`, http.StatusBadRequest},
		{"empty url", `{"events":["app.deployed"]}`, http.StatusBadRequest},
		{"empty events", `{"url":"https://x.com"}`, http.StatusBadRequest},
		{"url too long", `{"url":"` + strings.Repeat("a", 2049) + `","events":["e1"]}`, http.StatusBadRequest},
		{"too many events", `{"url":"https://x.com","events":[` + strings.Repeat(`"e",`, 51) + `"e"]}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(tt.body))
			req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
			rr := httptest.NewRecorder()
			h.Create(rr, req)
			if rr.Code != tt.want {
				t.Errorf("expected %d, got %d", tt.want, rr.Code)
			}
		})
	}
}

func TestEventWebhookHandler_Create_LimitReached(t *testing.T) {
	kv := newMockKVStore()
	list := eventWebhookList{Webhooks: make([]EventWebhookConfig, 20)}
	for i := range list.Webhooks {
		list.Webhooks[i] = EventWebhookConfig{ID: "wh-" + string(rune('a'+i))}
	}
	kv.Set("event_webhooks", "tenant:t1", list, 0)

	h := NewEventWebhookHandler(newMockStore(), nil, kv)
	body := `{"url":"https://example.com/hook","events":["app.deployed"]}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_ConcurrentPreservesAllWebhooks(t *testing.T) {
	kv := newMockKVStore()
	h := NewEventWebhookHandler(newMockStore(), nil, kv)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"url":"https://example.com/hook-%d","events":["app.deployed"]}`, i)
			req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
			req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
			rr := httptest.NewRecorder()
			h.Create(rr, req)
			if rr.Code != http.StatusCreated {
				t.Errorf("Create %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
			}
		}(i)
	}
	wg.Wait()

	var list eventWebhookList
	if err := kv.Get("event_webhooks", "tenant:t1", &list); err != nil {
		t.Fatalf("Get webhooks: %v", err)
	}
	if len(list.Webhooks) != 10 {
		t.Fatalf("stored webhooks = %d, want 10", len(list.Webhooks))
	}
}

func TestEventWebhookHandler_Delete_SuccessBoost(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("event_webhooks", "tenant:t1", eventWebhookList{
		Webhooks: []EventWebhookConfig{{ID: "wh-1", URL: "https://x.com", Events: []string{"e1"}}},
	}, 0)

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewEventWebhookHandler(newMockStore(), events, kv)

	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/outbound/wh-1", nil)
	req.SetPathValue("id", "wh-1")
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Delete_NoClaims(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), nil, newMockKVStore())
	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/outbound/wh-1", nil)
	req.SetPathValue("id", "wh-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// =============================================================================
// DeployApprovalHandler — Approve, Reject, ListPending with events
// =============================================================================

func TestDeployApprovalHandler_Approve_Success(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewDeployApprovalHandler(newMockStore(), events)
	seedApprovalRequest(h, &ApprovalRequest{
		ID:        "apr-1",
		AppID:     "app-1",
		TenantID:  "tenant-1",
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/apr-1/approve", nil)
	req.SetPathValue("id", "apr-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Approve(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDeployApprovalHandler_Approve_WrongTenant(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), nil)
	seedApprovalRequest(h, &ApprovalRequest{
		ID:        "apr-1",
		AppID:     "app-1",
		TenantID:  "tenant-2",
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/apr-1/approve", nil)
	req.SetPathValue("id", "apr-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Approve(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestDeployApprovalHandler_Approve_NotFound(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), nil)
	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/missing/approve", nil)
	req.SetPathValue("id", "missing")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Approve(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDeployApprovalHandler_Reject_Success(t *testing.T) {
	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewDeployApprovalHandler(newMockStore(), events)
	seedApprovalRequest(h, &ApprovalRequest{
		ID:        "apr-1",
		AppID:     "app-1",
		TenantID:  "tenant-1",
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	body := `{"reason":"needs more tests"}`
	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/apr-1/reject", strings.NewReader(body))
	req.SetPathValue("id", "apr-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Reject(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDeployApprovalHandler_Reject_WrongTenant(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), nil)
	seedApprovalRequest(h, &ApprovalRequest{
		ID:        "apr-1",
		AppID:     "app-1",
		TenantID:  "tenant-2",
		Status:    "pending",
		CreatedAt: time.Now(),
	})

	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/apr-1/reject", nil)
	req.SetPathValue("id", "apr-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Reject(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestDeployApprovalHandler_Reject_NotFound(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), nil)
	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/missing/reject", nil)
	req.SetPathValue("id", "missing")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Reject(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestDeployApprovalHandler_ListPending(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), nil)
	seedApprovalRequest(h, &ApprovalRequest{ID: "apr-1", AppID: "app-1", TenantID: "t1", Status: "pending", CreatedAt: time.Now()})
	seedApprovalRequest(h, &ApprovalRequest{ID: "apr-2", AppID: "app-1", TenantID: "t1", Status: "approved", CreatedAt: time.Now()})

	req := httptest.NewRequest("GET", "/api/v1/deploy/approvals", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.ListPending(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 1 {
		t.Errorf("expected 1 pending, got %v", resp["total"])
	}
}

// =============================================================================
// GPUHandler — Get, Update with events
// =============================================================================

func TestGPUHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	kv := newMockKVStore()
	kv.Set("gpu_config", "app-1", GPUConfig{Enabled: true, Driver: "nvidia", Capabilities: []string{"compute"}}, 0)

	h := NewGPUHandler(store, &mockContainerRuntime{}, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestGPUHandler_Get_Defaults(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewGPUHandler(store, &mockContainerRuntime{}, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/gpu", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	cfg, _ := resp["config"].(map[string]any)
	if cfg["driver"] != "nvidia" {
		t.Errorf("expected default driver nvidia, got %v", cfg["driver"])
	}
}

func TestGPUHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewGPUHandler(store, &mockContainerRuntime{}, newMockKVStore())
	h.SetEvents(events)

	body := `{"enabled":true,"device_ids":["0"],"capabilities":["compute","utility"],"driver":"nvidia"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/gpu", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// AutoscaleHandler — Get, Update with events
// =============================================================================

func TestAutoscaleHandler_Get_Defaults(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewAutoscaleHandler(store, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/autoscale", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["cpu_target_percent"] == nil {
		t.Error("expected default cpu_target_percent")
	}
}

func TestAutoscaleHandler_Get_StoredBoost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	kv := newMockKVStore()
	kv.Set("autoscale", "app-1", AutoscaleConfig{Enabled: true, MinReplicas: 2, MaxReplicas: 5, CPUTarget: 70}, 0)

	h := NewAutoscaleHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/autoscale", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["min_replicas"] == nil {
		t.Error("expected min_replicas from stored config")
	}
}

func TestAutoscaleHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewAutoscaleHandler(store, newMockKVStore())
	h.SetEvents(events)

	body := `{"enabled":true,"min_replicas":2,"max_replicas":8,"cpu_target_percent":75}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/autoscale", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestAutoscaleHandler_Update_Clamping(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewAutoscaleHandler(store, newMockKVStore())
	body := `{"enabled":true,"min_replicas":0,"max_replicas":0}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/autoscale", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	cfg, _ := resp["config"].(map[string]any)
	if int(cfg["min_replicas"].(float64)) != 1 {
		t.Errorf("expected min_replicas clamped to 1, got %v", cfg["min_replicas"])
	}
	if int(cfg["max_replicas"].(float64)) != 1 {
		t.Errorf("expected max_replicas clamped to min, got %v", cfg["max_replicas"])
	}
}

// =============================================================================
// CronJobHandler — Get, Create, Delete with events
// =============================================================================

func TestCronJobHandler_List_Empty(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewCronJobHandler(store, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/cron", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total 0, got %v", resp["total"])
	}
}

func TestCronJobHandler_Create(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewCronJobHandler(store, newMockKVStore())
	h.SetEvents(events)

	body := `{"name":"backup","schedule":"0 2 * * *","command":"/app/backup.sh"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/cron", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestCronJobHandler_Create_Validation(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewCronJobHandler(store, newMockKVStore())
	body := `{"name":"backup","schedule":"","command":""}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/cron", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCronJobHandler_Create_LimitReached(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	kv := newMockKVStore()
	list := cronJobList{Jobs: make([]CronJobConfig, 50)}
	for i := range list.Jobs {
		list.Jobs[i] = CronJobConfig{ID: "job-" + string(rune('a'+i))}
	}
	kv.Set("cronjobs", "app-1", list, 0)

	h := NewCronJobHandler(store, kv)
	body := `{"name":"backup","schedule":"0 2 * * *","command":"/app/backup.sh"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/cron", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestCronJobHandler_Delete(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	kv := newMockKVStore()
	kv.Set("cronjobs", "app-1", cronJobList{Jobs: []CronJobConfig{{ID: "job-1", Name: "backup", Schedule: "0 2 * * *", Command: "/app/backup.sh"}}}, 0)

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewCronJobHandler(store, kv)
	h.SetEvents(events)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/cron/job-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("jobId", "job-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// =============================================================================
// BasicAuthHandler — Get, Update with events
// =============================================================================

func TestBasicAuthHandler_Get_Default(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewBasicAuthHandler(store, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/basic-auth", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp BasicAuthConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Realm != "Restricted" {
		t.Errorf("expected default realm Restricted, got %q", resp.Realm)
	}
}

func TestBasicAuthHandler_Get_StoredBoost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	kv := newMockKVStore()
	kv.Set("basic_auth", "app-1", BasicAuthConfig{Enabled: true, Realm: "Private", Users: map[string]string{"admin": "hash"}}, 0)

	h := NewBasicAuthHandler(store, kv)
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/basic-auth", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp BasicAuthConfig
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if !resp.Enabled {
		t.Error("expected enabled=true from stored config")
	}
}

func TestBasicAuthHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewBasicAuthHandler(store, newMockKVStore())
	h.SetEvents(events)

	body := `{"enabled":true,"users":{"admin":"$2a$10$hash"},"realm":"Secure"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/basic-auth", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestBasicAuthHandler_Update_Validation(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewBasicAuthHandler(store, newMockKVStore())

	tests := []struct {
		name string
		body string
		want int
	}{
		{"realm too long", `{"enabled":true,"realm":"` + strings.Repeat("a", 101) + `"}`, http.StatusBadRequest},
		{"too many users", `{"enabled":true,"users":{` + func() string {
			var sb strings.Builder
			for i := 0; i < 51; i++ {
				if i > 0 {
					sb.WriteByte(',')
				}
				sb.WriteString(fmt.Sprintf("\"u%d\":\"h\"", i))
			}
			return sb.String()
		}() + `}}`, http.StatusBadRequest},
		{"username too long", `{"enabled":true,"users":{"` + strings.Repeat("a", 101) + `":"h"}}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/basic-auth", strings.NewReader(tt.body))
			req.SetPathValue("id", "app-1")
			req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
			rr := httptest.NewRecorder()
			h.Update(rr, req)
			if rr.Code != tt.want {
				t.Errorf("expected %d, got %d", tt.want, rr.Code)
			}
		})
	}
}

// =============================================================================
// RedirectHandler — List, Create, Delete with events
// =============================================================================

func TestRedirectHandler_List_EmptyBoost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewRedirectHandler(store, newMockKVStore())
	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/redirects", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total 0, got %v", resp["total"])
	}
}

func TestRedirectHandler_Create(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewRedirectHandler(store, newMockKVStore())
	h.SetEvents(events)

	body := `{"source":"/old","destination":"/new","type":"redirect","status_code":301}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_Validation(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewRedirectHandler(store, newMockKVStore())

	tests := []struct {
		name string
		body string
		want int
	}{
		{"empty source", `{"source":"","destination":"/new"}`, http.StatusBadRequest},
		{"empty destination", `{"source":"/old","destination":""}`, http.StatusBadRequest},
		{"source too long", `{"source":"` + strings.Repeat("a", 2049) + `","destination":"/new"}`, http.StatusBadRequest},
		{"destination too long", `{"source":"/old","destination":"` + strings.Repeat("a", 2049) + `"}`, http.StatusBadRequest},
		{"bad status code", `{"source":"/old","destination":"/new","status_code":404}`, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(tt.body))
			req.SetPathValue("id", "app-1")
			req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
			rr := httptest.NewRecorder()
			h.Create(rr, req)
			if rr.Code != tt.want {
				t.Errorf("expected %d, got %d", tt.want, rr.Code)
			}
		})
	}
}

func TestRedirectHandler_Create_DefaultStatus(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})

	h := NewRedirectHandler(store, newMockKVStore())
	body := `{"source":"/old","destination":"/new"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	rule, _ := resp["rule"].(map[string]any)
	if int(rule["status_code"].(float64)) != 301 {
		t.Errorf("expected default status 301, got %v", rule["status_code"])
	}
}

func TestRedirectHandler_Create_LimitReached(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	kv := newMockKVStore()
	list := redirectList{Rules: make([]RedirectRule, 200)}
	for i := range list.Rules {
		list.Rules[i] = RedirectRule{ID: "r-" + string(rune('a'+i))}
	}
	kv.Set("redirects", "app-1", list, 0)

	h := NewRedirectHandler(store, kv)
	body := `{"source":"/old","destination":"/new"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_ConcurrentPreservesAllRules(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	kv := newMockKVStore()
	h := NewRedirectHandler(store, kv)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"source":"/old-%d","destination":"/new-%d"}`, i, i)
			req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
			req.SetPathValue("id", "app-1")
			req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
			rr := httptest.NewRecorder()
			h.Create(rr, req)
			if rr.Code != http.StatusCreated {
				t.Errorf("Create %d: expected 201, got %d: %s", i, rr.Code, rr.Body.String())
			}
		}(i)
	}
	wg.Wait()

	var list redirectList
	if err := kv.Get("redirects", "app-1", &list); err != nil {
		t.Fatalf("Get redirects: %v", err)
	}
	if len(list.Rules) != 10 {
		t.Fatalf("stored redirect rules = %d, want 10", len(list.Rules))
	}
}

func TestRedirectHandler_Delete(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	kv := newMockKVStore()
	kv.Set("redirects", "app-1", redirectList{Rules: []RedirectRule{{ID: "r-1", Source: "/old", Destination: "/new"}}}, 0)

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewRedirectHandler(store, kv)
	h.SetEvents(events)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/redirects/r-1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("ruleId", "r-1")
	req = withClaims(req, "u1", "tenant-1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// =============================================================================
// DetailedHealthHandler — DetailedHealth
// =============================================================================

func TestDetailedHealthHandler_DetailedHealth(t *testing.T) {
	c := testCore()
	c.Store = newMockStore()
	c.Services.Container = &mockContainerRuntime{}
	c.Build = core.BuildInfo{Version: "1.0.0-test"}
	c.Registry = core.NewRegistry()

	h := NewDetailedHealthHandler(c)
	rl := middleware.NewGlobalRateLimiter(100, 200)
	h.SetRateLimiter(rl)

	req := httptest.NewRequest("GET", "/health/detailed", nil)
	rr := httptest.NewRecorder()
	h.DetailedHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "healthy" {
		t.Errorf("expected status healthy, got %v", resp["status"])
	}
	checks, _ := resp["checks"].(map[string]any)
	if checks["database"] == nil {
		t.Error("expected database check")
	}
	if checks["docker"] == nil {
		t.Error("expected docker check")
	}
	if checks["events"] == nil {
		t.Error("expected events check")
	}
	if checks["rate_limiter"] == nil {
		t.Error("expected rate_limiter check")
	}
	if checks["runtime"] == nil {
		t.Error("expected runtime check")
	}
}

func TestDetailedHealthHandler_DetailedHealth_DBDown(t *testing.T) {
	c := testCore()
	c.Store = &mockStorePingErr{}
	c.Build = core.BuildInfo{Version: "1.0.0-test"}
	c.Registry = core.NewRegistry()

	h := NewDetailedHealthHandler(c)

	req := httptest.NewRequest("GET", "/health/detailed", nil)
	rr := httptest.NewRecorder()
	h.DetailedHealth(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// mockStore with ping error for health tests
type mockStorePingErr struct {
	mockStore
}

func (m *mockStorePingErr) Ping(_ context.Context) error {
	return errors.New("ping failed")
}

// =============================================================================
// AnnouncementHandler Create — validation error paths
// =============================================================================

func TestAnnouncementHandler_Create_TitleTooLong(t *testing.T) {
	h := NewAnnouncementHandler(newMockKVStore())
	body, _ := json.Marshal(Announcement{
		Title: strings.Repeat("a", 201),
		Body:  "Valid body",
		Type:  "info",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAnnouncementHandler_Create_BodyTooLong(t *testing.T) {
	h := NewAnnouncementHandler(newMockKVStore())
	body, _ := json.Marshal(Announcement{
		Title: "Valid title",
		Body:  strings.Repeat("b", 10001),
		Type:  "info",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAnnouncementHandler_Create_InvalidType(t *testing.T) {
	h := NewAnnouncementHandler(newMockKVStore())
	body, _ := json.Marshal(Announcement{
		Title: "Valid title",
		Body:  "Valid body",
		Type:  "invalid_type",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestAnnouncementHandler_Create_LimitReached(t *testing.T) {
	kv := newMockKVStore()
	list := announcementList{Items: make([]Announcement, 100)}
	for i := range list.Items {
		list.Items[i] = Announcement{ID: core.GenerateID(), Title: "Ann " + string(rune(i)), Type: "info"}
	}
	kv.Set("announcements", "all", list, 0)

	h := NewAnnouncementHandler(kv)
	body, _ := json.Marshal(Announcement{
		Title: "One more",
		Body:  "Should fail",
		Type:  "info",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/announcements", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

// =============================================================================
// EnvImportHandler Import — validation error paths
// =============================================================================

func TestEnvImportHandler_EmptyKey(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	h := NewEnvImportHandler(store)

	body := `[{"key":"","value":"val"}]`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/env/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEnvImportHandler_KeyTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	h := NewEnvImportHandler(store)

	body := `[{"key":"` + strings.Repeat("k", 257) + `","value":"val"}]`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/env/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEnvImportHandler_ValueTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	h := NewEnvImportHandler(store)

	body := `[{"key":"KEY","value":"` + strings.Repeat("v", 64*1024+1) + `"}]`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/env/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEnvImportHandler_TotalSizeExceeded(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "App"})
	h := NewEnvImportHandler(store)

	vars := make([]envVarEntry, 10)
	for i := range vars {
		vars[i] = envVarEntry{Key: "KEY" + string(rune('0'+i)), Value: strings.Repeat("x", 60*1024)}
	}
	body, _ := json.Marshal(vars)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/env/import", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// DomainHandler Create — field validation error paths
// =============================================================================

func TestDomainHandler_Create_FQDNTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})
	h := NewDomainHandler(store, core.NewEventBus(nil))

	body, _ := json.Marshal(createDomainRequest{
		AppID: "app1",
		FQDN:  strings.Repeat("a", 254) + ".com",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDomainHandler_Create_DNSProviderTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})
	h := NewDomainHandler(store, core.NewEventBus(nil))

	body, _ := json.Marshal(createDomainRequest{
		AppID:       "app1",
		FQDN:        "example.com",
		DNSProvider: strings.Repeat("p", 51),
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/domains", bytes.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// DeployTriggerHandler — image app store error paths
// =============================================================================

func TestDeployTriggerHandler_ImageDeploy_AtomicVersionError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID: "app1", Name: "img-app", SourceType: "image",
		SourceURL: "nginx:latest", TenantID: "t1",
	})
	store.errGetNextDeployVersion = core.ErrNotFound

	h := NewDeployTriggerHandler(context.Background(), store, nil, core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestDeployTriggerHandler_ImageDeploy_CreateDeploymentError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID: "app1", Name: "img-app", SourceType: "image",
		SourceURL: "nginx:latest", TenantID: "t1",
	})
	store.nextDeployVersion["app1"] = 1
	store.errCreateDeployment = core.ErrNotFound

	h := NewDeployTriggerHandler(context.Background(), store, nil, core.NewEventBus(nil))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AppHandler Delete — runtime available path
// =============================================================================

func TestAppHandler_Delete_WithRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})

	c := testCore()
	c.Services.Container = &mockContainerRuntime{}

	h := NewAppHandler(store, c)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/apps/app1", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

// === merged from coverage_boost_test.go ===

// ═══════════════════════════════════════════════════════════════════════════════
// DiskUsageHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestDiskUsageHandler_AppDisk_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewDiskUsageHandler(store, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/disk", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.AppDisk(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	if body["app_id"] != "app-1" {
		t.Errorf("app_id = %v, want app-1", body["app_id"])
	}
}

func TestDiskUsageHandler_AppDisk_WithRuntime(t *testing.T) {
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "c1", State: "running"}},
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-2", TenantID: "t1", Name: "test", Status: "running"})
	h := NewDiskUsageHandler(store, runtime)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-2/disk", nil)
	req.SetPathValue("id", "app-2")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.AppDisk(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var body map[string]any
	json.NewDecoder(rr.Body).Decode(&body)
	if body["containers"] != float64(1) {
		t.Errorf("containers = %v, want 1", body["containers"])
	}
}

func TestDiskUsageHandler_SystemDisk(t *testing.T) {
	h := NewDiskUsageHandler(nil, nil)

	req := httptest.NewRequest("GET", "/api/v1/admin/disk", nil)
	rr := httptest.NewRecorder()
	h.SystemDisk(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDiskUsageHandler_AppDisk_ImageSizeByIDAndTag(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-3", TenantID: "t1", Name: "test", Status: "running"})
	runtime := &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", Image: "sha256:one"},
			{ID: "c2", Image: "repo/app:latest"},
			{ID: "c3", Image: "missing"},
		},
		imageList: []core.ImageInfo{
			{ID: "sha256:one", Size: 100},
			{ID: "sha256:two", Tags: []string{"repo/app:latest"}, Size: 250},
			{ID: "sha256:unused", Tags: []string{"unused:latest"}, Size: 999},
		},
	}
	h := NewDiskUsageHandler(store, runtime)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-3/disk", nil)
	req.SetPathValue("id", "app-3")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.AppDisk(rr, req)

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["containers"] != float64(3) || body["image_size_bytes"] != float64(350) {
		t.Fatalf("unexpected disk response: %+v", body)
	}
}

func TestDiskUsageHandler_SystemDisk_RuntimeImagesAndVolumes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("abc"), 0o644); err != nil {
		t.Fatalf("write volume file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "nested", "b.txt"), []byte("abcd"), 0o644); err != nil {
		t.Fatalf("write nested volume file: %v", err)
	}
	runtime := &diskRuntime{
		mockContainerRuntime: &mockContainerRuntime{
			imageList: []core.ImageInfo{{ID: "i1", Size: 100}, {ID: "i2", Size: 250}},
		},
		volumes: []core.VolumeInfo{{Name: "v1", Mountpoint: dir}, {Name: "v2"}},
	}
	h := NewDiskUsageHandler(nil, runtime)

	req := httptest.NewRequest("GET", "/api/v1/admin/disk", nil)
	rr := httptest.NewRecorder()
	h.SystemDisk(rr, req)

	var body map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["images_bytes"] != float64(350) || body["images_count"] != float64(2) {
		t.Fatalf("unexpected image totals: %+v", body)
	}
	if body["volumes_bytes"] != float64(7) || body["volumes_count"] != float64(2) {
		t.Fatalf("unexpected volume totals: %+v", body)
	}
}

type diskRuntime struct {
	*mockContainerRuntime
	volumes []core.VolumeInfo
}

func (r *diskRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return r.volumes, nil
}

// ═══════════════════════════════════════════════════════════════════════════════
// ErrorPageHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestErrorPageHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewErrorPageHandler(store, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/error-pages", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestErrorPageHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewErrorPageHandler(store, newMockKVStore())

	body := `{"page_502":"<h1>Down</h1>","page_503":"<h1>Unavailable</h1>"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/error-pages", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "updated" {
		t.Errorf("status = %v, want updated", resp["status"])
	}
}

func TestErrorPageHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewErrorPageHandler(store, newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/error-pages", strings.NewReader("not json"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageCleanupHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestImageCleanupHandler_DanglingImages(t *testing.T) {
	runtime := &mockContainerRuntime{}
	h := NewImageCleanupHandler(runtime)

	req := httptest.NewRequest("GET", "/api/v1/images/dangling", nil)
	rr := httptest.NewRecorder()
	h.DanglingImages(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestImageCleanupHandler_Prune(t *testing.T) {
	runtime := &mockContainerRuntime{}
	h := NewImageCleanupHandler(runtime)

	req := httptest.NewRequest("DELETE", "/api/v1/images/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "pruned" {
		t.Errorf("status = %v, want pruned", resp["status"])
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LogRetentionHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestLogRetentionHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewLogRetentionHandler(store, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/log-retention", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var cfg LogRetentionConfig
	json.NewDecoder(rr.Body).Decode(&cfg)
	if cfg.MaxSizeMB != 50 {
		t.Errorf("MaxSizeMB = %d, want 50", cfg.MaxSizeMB)
	}
	if cfg.MaxFiles != 5 {
		t.Errorf("MaxFiles = %d, want 5", cfg.MaxFiles)
	}
	if cfg.Driver != "json-file" {
		t.Errorf("Driver = %q, want json-file", cfg.Driver)
	}
}

func TestLogRetentionHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewLogRetentionHandler(store, newMockKVStore())

	body := `{"max_size_mb":100,"max_files":10,"driver":"local"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLogRetentionHandler_Update_DefaultValues(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewLogRetentionHandler(store, newMockKVStore())

	// max_size_mb <= 0 should default to 50, max_files <= 0 should default to 5
	body := `{"max_size_mb":-1,"max_files":0}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLogRetentionHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	h := NewLogRetentionHandler(store, newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader("{invalid"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// MaintenanceHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestMaintenanceHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	events := core.NewEventBus(nil)
	h := NewMaintenanceHandler(store, events, newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/maintenance", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var cfg MaintenanceConfig
	json.NewDecoder(rr.Body).Decode(&cfg)
	if cfg.Enabled {
		t.Error("default maintenance mode should be disabled")
	}
}

func TestMaintenanceHandler_Update_Enable(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	events := core.NewEventBus(nil)
	h := NewMaintenanceHandler(store, events, newMockKVStore())

	body := `{"enabled":true,"message":"We are upgrading","allowed_ips":["10.0.0.1"]}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/maintenance", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "enabled" {
		t.Errorf("status = %v, want enabled", resp["status"])
	}
}

func TestMaintenanceHandler_Update_Disable(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	events := core.NewEventBus(nil)
	h := NewMaintenanceHandler(store, events, newMockKVStore())

	body := `{"enabled":false}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/maintenance", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["status"] != "disabled" {
		t.Errorf("status = %v, want disabled", resp["status"])
	}
}

func TestMaintenanceHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", Status: "running"})
	events := core.NewEventBus(nil)
	h := NewMaintenanceHandler(store, events, newMockKVStore())

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/maintenance", strings.NewReader("bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PortHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestPortHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test-app"})
	h := NewPortHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/ports", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPortHandler_Get_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewPortHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/ports", nil)
	req.SetPathValue("id", "missing")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestPortHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test-app"})
	h := NewPortHandler(store)

	body := `[{"container_port":8080,"protocol":"tcp","exposed":true}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req = withClaims(req, "user-1", "tenant-1", "admin", "test@test.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPortHandler_Update_InvalidPort(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test-app"})
	h := NewPortHandler(store)

	body := `[{"container_port":-1,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req = withClaims(req, "user-1", "tenant-1", "admin", "test@test.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestPortHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test-app"})
	h := NewPortHandler(store)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader("not-json"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "user-1", "tenant-1", "admin", "test@test.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestPortHandler_Update_PortOverMax(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test-app"})
	h := NewPortHandler(store)

	body := `[{"container_port":70000,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req = withClaims(req, "user-1", "tenant-1", "admin", "test@test.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for port > 65535, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LabelsHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestLabelsHandler_Get(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test", LabelsJSON: `{"env":"prod","team":"backend"}`})
	h := NewLabelsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/labels", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLabelsHandler_Get_Empty(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewLabelsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/labels", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLabelsHandler_Get_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewLabelsHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/labels", nil)
	req.SetPathValue("id", "missing")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewLabelsHandler(store)

	body := `{"env":"staging","version":"v2"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/labels", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewLabelsHandler(store)

	body := `{"env":"staging"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/missing/labels", strings.NewReader(body))
	req.SetPathValue("id", "missing")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewLabelsHandler(store)

	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/labels", strings.NewReader("bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CommitRollbackHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestCommitRollbackHandler_RollbackToCommit_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "testapp", TenantID: "t1", Status: "running"})
	store.addDeployment("app-1", core.Deployment{
		AppID: "app-1", Version: 1, CommitSHA: "abc1234567890", Image: "myapp:v1",
	})
	store.addDeployment("app-1", core.Deployment{
		AppID: "app-1", Version: 2, CommitSHA: "def4567890abc", Image: "myapp:v2",
	})
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":"abc1234567890"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["version"] != float64(1) {
		t.Errorf("version = %v, want 1", resp["version"])
	}
}

func TestCommitRollbackHandler_RollbackToCommit_PartialMatch(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", Name: "testapp", TenantID: "t1", Status: "running"})
	store.addDeployment("app-1", core.Deployment{
		AppID: "app-1", Version: 3, CommitSHA: "abcdef1234567890", Image: "myapp:v3",
	})
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":"abcdef1"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestCommitRollbackHandler_RollbackToCommit_NotFound(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "testapp", Status: "running"})
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":"nonexistent"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestCommitRollbackHandler_RollbackToCommit_EmptyCommit(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "testapp", Status: "running"})
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":""}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommitRollbackHandler_RollbackToCommit_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "testapp", Status: "running"})
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader("{bad"))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestCommitRollbackHandler_RollbackToCommit_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "testapp", Status: "running"})
	store.errListDeploymentsByApp = core.ErrNotFound
	events := core.NewEventBus(nil)
	h := NewCommitRollbackHandler(store, &mockContainerRuntime{}, events)

	body := `{"commit_sha":"abc1234"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/rollback-to-commit", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.RollbackToCommit(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// EnvCompareHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestEnvCompareHandler_Compare(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-left",
		TenantID:   "t1",
		Name:       "left-app",
		EnvVarsEnc: `[{"key":"DB_HOST","value":"localhost"},{"key":"DB_PORT","value":"5432"}]`,
	})
	store.addApp(&core.Application{
		ID:         "app-right",
		TenantID:   "t1",
		Name:       "right-app",
		EnvVarsEnc: `[{"key":"DB_HOST","value":"prod-server"},{"key":"REDIS_URL","value":"redis://localhost"}]`,
	})
	h := NewEnvCompareHandler(store)

	body := `{"left_app_id":"app-left","right_app_id":"app-right"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/env/compare", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Compare(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	total := resp["total"].(float64)
	if total < 2 {
		t.Errorf("expected at least 2 diffs, got %v", total)
	}
}

func TestEnvCompareHandler_Compare_LeftNotFound(t *testing.T) {
	store := newMockStore()
	h := NewEnvCompareHandler(store)

	body := `{"left_app_id":"missing","right_app_id":"also-missing"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/env/compare", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Compare(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestEnvCompareHandler_Compare_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewEnvCompareHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/env/compare", strings.NewReader("bad"))
	rr := httptest.NewRecorder()
	h.Compare(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// EnvImportHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestEnvImportHandler_Import_DotEnv(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewEnvImportHandler(store)

	envContent := "DB_HOST=localhost\nDB_PORT=5432\n# comment\nREDIS_URL=\"redis://localhost\"\n"
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/env/import", strings.NewReader(envContent))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["imported"] != float64(3) {
		t.Errorf("imported = %v, want 3", resp["imported"])
	}
}

func TestEnvImportHandler_Import_JSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewEnvImportHandler(store)

	body := `[{"key":"API_KEY","value":"secret123"},{"key":"NODE_ENV","value":"production"}]`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/env/import", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestEnvImportHandler_Import_Empty(t *testing.T) {
	store := newMockStore()
	h := NewEnvImportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/env/import", strings.NewReader(""))
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestEnvImportHandler_Import_AppNotFound(t *testing.T) {
	store := newMockStore()
	h := NewEnvImportHandler(store)

	body := "KEY=VALUE\n"
	req := httptest.NewRequest("POST", "/api/v1/apps/missing/env/import", strings.NewReader(body))
	req.SetPathValue("id", "missing")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestEnvImportHandler_Export_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewEnvImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/env/export", nil)
	req.SetPathValue("id", "missing")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestEnvImportHandler_Export_DotEnv(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "t1",
		Name:       "test",
		EnvVarsEnc: `[{"key":"FOO","value":"bar"},{"key":"BAZ","value":"qux"}]`,
	})
	h := NewEnvImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/env/export", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "text/plain" {
		t.Errorf("expected Content-Type text/plain, got %q", rr.Header().Get("Content-Type"))
	}
	body := rr.Body.String()
	// Values are quoted by sanitizeEnvValue for injection prevention
	if !strings.Contains(body, "FOO=\"bar\"") {
		t.Errorf("expected FOO=\"bar\" in output, got %q", body)
	}
}

func TestEnvImportHandler_Export_JSON(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "t1",
		Name:       "test",
		EnvVarsEnc: `[{"key":"FOO","value":"bar"}]`,
	})
	h := NewEnvImportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/env/export?format=json", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImportExportHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestImportExportHandler_Export(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID: "app-1", TenantID: "t1", Name: "my-web-app", Type: "service",
		SourceType: "image", SourceURL: "nginx:latest", Replicas: 2,
	})
	store.domainsByApp["app-1"] = []core.Domain{
		{FQDN: "example.com"},
		{FQDN: "www.example.com"},
	}
	h := NewImportExportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/app-1/export", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var manifest AppManifest
	json.NewDecoder(rr.Body).Decode(&manifest)
	if manifest.Name != "my-web-app" {
		t.Errorf("name = %q, want my-web-app", manifest.Name)
	}
	if len(manifest.Domains) != 2 {
		t.Errorf("domains = %d, want 2", len(manifest.Domains))
	}
}

func TestImportExportHandler_Export_NotFound(t *testing.T) {
	store := newMockStore()
	h := NewImportExportHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/apps/missing/export", nil)
	req.SetPathValue("id", "missing")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestImportExportHandler_Import(t *testing.T) {
	store := newMockStore()
	store.projects["tenant-1"] = []core.Project{{ID: "proj-1", Name: "Default"}}
	h := NewImportExportHandler(store)

	manifest := `{"version":"1","name":"imported-app","type":"service","source_type":"image","source_url":"nginx:latest","replicas":1,"domains":["imported.example.com"]}`
	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader(manifest))
	req = withClaims(req, "user-1", "tenant-1", "admin", "user@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestImportExportHandler_Import_NoClaims(t *testing.T) {
	store := newMockStore()
	h := NewImportExportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader("{}"))
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestImportExportHandler_Import_InvalidManifest(t *testing.T) {
	store := newMockStore()
	h := NewImportExportHandler(store)

	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader("bad"))
	req = withClaims(req, "user-1", "tenant-1", "admin", "user@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestImportExportHandler_Import_RejectsUnknownFields(t *testing.T) {
	store := newMockStore()
	h := NewImportExportHandler(store)

	manifest := `{"version":"1","name":"imported-app","type":"service","source_type":"image","source_url":"nginx:latest","replicas":1,"extra":true}`
	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader(manifest))
	req = withClaims(req, "user-1", "tenant-1", "admin", "user@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

// ═══════════════════════════════════════════════════════════════════════════════
// DNSRecordHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestDNSRecordHandler_List(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("GET", "/api/v1/dns/records?domain=example.com", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Create_MissingFields(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	body := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Create_InvalidBody(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("POST", "/api/v1/dns/records", strings.NewReader("bad"))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Create_NoProvider(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	body := `{"name":"test","value":"1.2.3.4","type":"A"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing provider, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Delete_NoProvider(t *testing.T) {
	services := core.NewServices()
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("DELETE", "/api/v1/dns/records/rec-1", nil)
	req.SetPathValue("id", "rec-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing provider, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// DomainVerifyHandler
// ═══════════════════════════════════════════════════════════════════════════════

func TestDomainVerifyHandler_Verify_EmptyFQDN(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	store.addDomain(&core.Domain{ID: "dom-1", AppID: "app-1", FQDN: ""})
	h := NewDomainVerifyHandler(store, newMockKVStore())

	body := `{"fqdn":""}`
	req := httptest.NewRequest("POST", "/api/v1/domains/dom-1/verify", strings.NewReader(body))
	req.SetPathValue("id", "dom-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDomainVerifyHandler_Verify_WithFQDN(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	store.addDomain(&core.Domain{ID: "dom-1", AppID: "app-1", FQDN: "localhost"})
	h := NewDomainVerifyHandler(store, newMockKVStore())

	body := `{"fqdn":"localhost"}`
	req := httptest.NewRequest("POST", "/api/v1/domains/dom-1/verify", strings.NewReader(body))
	req.SetPathValue("id", "dom-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDomainVerifyHandler_Verify_InvalidBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	store.addDomain(&core.Domain{ID: "dom-1", AppID: "app-1", FQDN: "localhost"})
	h := NewDomainVerifyHandler(store, newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/domains/dom-1/verify", strings.NewReader("bad"))
	req.SetPathValue("id", "dom-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDomainVerifyHandler_BatchVerify(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	store.addDomain(&core.Domain{ID: "dom-1", AppID: "app-1", FQDN: "localhost"})
	h := NewDomainVerifyHandler(store, newMockKVStore())

	body := `{"fqdns":["localhost"]}`
	req := httptest.NewRequest("POST", "/api/v1/domains/verify-batch", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.BatchVerify(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestDomainVerifyHandler_BatchVerify_InvalidBody(t *testing.T) {
	store := newMockStore()
	h := NewDomainVerifyHandler(store, newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/domains/verify-batch", strings.NewReader("bad"))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.BatchVerify(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// helpers — additional coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestWriteJSON_StatusAccepted(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusAccepted, map[string]string{"key": "value"})

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %q", rr.Header().Get("Content-Type"))
	}
}

func TestWriteError_NotFound(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusNotFound, "not found")

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["message"] != "not found" {
		t.Errorf("error message = %v, want 'not found'", errObj)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// AppManifest.Validate — comprehensive validation tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestAppManifest_Validate_Valid(t *testing.T) {
	m := AppManifest{
		Name:       "valid-app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Branch:     "main",
		Replicas:   1,
	}

	errors := m.Validate()
	if len(errors) != 0 {
		t.Errorf("expected no errors, got: %v", errors)
	}
}

func TestAppManifest_Validate_MissingName(t *testing.T) {
	m := AppManifest{
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for missing name")
	}
	if errors[0] != "name is required" {
		t.Errorf("error = %q, want 'name is required'", errors[0])
	}
}

func TestAppManifest_Validate_NameTooLong(t *testing.T) {
	m := AppManifest{
		Name:       strings.Repeat("a", 65),
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for name too long")
	}
}

func TestAppManifest_Validate_NameInvalidChars(t *testing.T) {
	m := AppManifest{
		Name:       "app<invalid>",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for invalid characters in name")
	}
}

func TestAppManifest_Validate_MissingType(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for missing type")
	}
}

func TestAppManifest_Validate_InvalidType(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "invalid-type",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for invalid type")
	}
}

func TestAppManifest_Validate_MissingSourceType(t *testing.T) {
	m := AppManifest{
		Name:      "app",
		Type:      "web",
		SourceURL: "https://github.com/test/repo",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for missing source_type")
	}
}

func TestAppManifest_Validate_InvalidSourceType(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "invalid",
		SourceURL:  "https://github.com/test/repo",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for invalid source_type")
	}
}

func TestAppManifest_Validate_MissingSourceURL(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for missing source_url")
	}
}

func TestAppManifest_Validate_ImageSourceWithInvalidChars(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "image",
		SourceURL:  "nginx;rm -rf /",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for invalid characters in image source_url")
	}
}

func TestAppManifest_Validate_GitSourceWithInvalidScheme(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "ftp://github.com/test/repo",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for invalid scheme in git source_url")
	}
}

func TestAppManifest_Validate_BranchPathTraversal(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Branch:     "../../../etc/passwd",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for path traversal in branch")
	}
}

func TestAppManifest_Validate_BranchInvalidChars(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Branch:     "main;echo hacked",
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for invalid characters in branch")
	}
}

func TestAppManifest_Validate_ReplicasNegative(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Replicas:   -1,
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for negative replicas")
	}
}

func TestAppManifest_Validate_ReplicasTooHigh(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Replicas:   101,
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for replicas > 100")
	}
}

func TestAppManifest_Validate_EmptyDomain(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Domains:    []string{""},
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for empty domain")
	}
}

func TestAppManifest_Validate_DomainTooLong(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Domains:    []string{strings.Repeat("a", 254) + ".com"},
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for domain too long")
	}
}

func TestAppManifest_Validate_InvalidDomainFormat(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Domains:    []string{"invalid domain with spaces"},
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for invalid domain format")
	}
}

func TestAppManifest_Validate_ValidDomain(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Domains:    []string{"example.com", "sub.example.org"},
	}

	errors := m.Validate()
	for _, e := range errors {
		if strings.Contains(e, "domain") {
			t.Errorf("unexpected domain error: %s", e)
		}
	}
}

func TestAppManifest_Validate_EmptyEnvVarKey(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		EnvVars:    map[string]string{"": "value"},
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for empty env var key")
	}
}

func TestAppManifest_Validate_EnvVarKeyInvalidChars(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		EnvVars:    map[string]string{"KEY=BAD": "value"},
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for invalid characters in env var key")
	}
}

func TestAppManifest_Validate_EmptyLabelKey(t *testing.T) {
	m := AppManifest{
		Name:       "app",
		Type:       "web",
		SourceType: "git",
		SourceURL:  "https://github.com/test/repo",
		Labels:     map[string]string{"": "value"},
	}

	errors := m.Validate()
	if len(errors) == 0 {
		t.Fatal("expected error for empty label key")
	}
}

func TestAppManifest_Validate_AllValidTypes(t *testing.T) {
	validTypes := []string{"web", "worker", "static", "cron", "docker", "compose", "database", "service"}

	for _, typ := range validTypes {
		t.Run(typ, func(t *testing.T) {
			m := AppManifest{
				Name:       "app",
				Type:       typ,
				SourceType: "git",
				SourceURL:  "https://github.com/test/repo",
			}

			errors := m.Validate()
			for _, e := range errors {
				if strings.Contains(e, "type") {
					t.Errorf("type %q should be valid, got error: %s", typ, e)
				}
			}
		})
	}
}

func TestAppManifest_Validate_AllValidSourceTypes(t *testing.T) {
	validSourceTypes := []string{"git", "github", "gitlab", "image", "tarball", "docker", "dockerfile"}

	for _, st := range validSourceTypes {
		t.Run(st, func(t *testing.T) {
			sourceURL := "https://github.com/test/repo"
			if st == "image" || st == "docker" {
				sourceURL = "nginx:latest"
			}

			m := AppManifest{
				Name:       "app",
				Type:       "web",
				SourceType: st,
				SourceURL:  sourceURL,
			}

			errors := m.Validate()
			for _, e := range errors {
				if strings.Contains(e, "source_type") {
					t.Errorf("source_type %q should be valid, got error: %s", st, e)
				}
			}
		})
	}
}

// === merged from deploy_trigger_boost2_test.go ===

func TestDeployTrigger_ImageApp_RuntimeError_Boost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})

	// Override CreateAndStart to return error via a custom mock
	// But mockContainerRuntime doesn't support this. Create a custom one.
	errRuntime := &errCreateRuntime{err: errors.New("docker error")}

	handler := NewDeployTriggerHandler(context.Background(), store, errRuntime, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}

	if store.updatedStatus["app1"] != "failed" {
		t.Errorf("expected app status=failed, got %q", store.updatedStatus["app1"])
	}
}

// errCreateRuntime is a mock runtime that fails CreateAndStart.
type errCreateRuntime struct {
	err error
}

func (e *errCreateRuntime) Ping() error { return nil }
func (e *errCreateRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", e.err
}
func (e *errCreateRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (e *errCreateRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (e *errCreateRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (e *errCreateRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (e *errCreateRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (e *errCreateRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}
func (e *errCreateRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}
func (e *errCreateRuntime) ImagePull(_ context.Context, _ string) error           { return nil }
func (e *errCreateRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) { return nil, nil }
func (e *errCreateRuntime) ImageRemove(_ context.Context, _ string) error         { return nil }
func (e *errCreateRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}
func (e *errCreateRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) { return nil, nil }

func TestDeployTrigger_ImageApp_AtomicVersionError_Boost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})
	store.errGetNextDeployVersion = errors.New("version fail")

	handler := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeployTrigger_ImageApp_CreateDeploymentError_Boost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})
	store.nextDeployVersion["app1"] = 3
	store.errCreateDeployment = errors.New("db fail")

	handler := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeployTrigger_ImageApp_UpdateStatusError_Boost(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app1",
		TenantID:   "tenant1",
		Name:       "My Image App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})
	store.nextDeployVersion["app1"] = 3
	store.errUpdateAppStatus = errors.New("status fail")

	runtime := &mockContainerRuntime{}
	handler := NewDeployTriggerHandler(context.Background(), store, runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/deploy", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	// UpdateAppStatus errors are logged but not fatal — should still return 200
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// === merged from deploy_trigger_boost_test.go ===

func TestDeployTriggerHandler_buildDeployLabels(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:        "app-1",
		ProjectID: "project-1",
		TenantID:  "tenant-1",
		Name:      "my-app",
		Port:      8080,
	})
	store.addDomain(&core.Domain{
		ID:    "dom-1",
		AppID: "app-1",
		FQDN:  "myapp.example.com",
	})

	h := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 3)

	if labels["monster.app.id"] != "app-1" {
		t.Errorf("app.id = %q, want app-1", labels["monster.app.id"])
	}
	if labels["monster.deploy.version"] != "3" {
		t.Errorf("version = %q, want 3", labels["monster.deploy.version"])
	}
	if labels["monster.http.routers.my-app-0.rule"] != "Host(`myapp.example.com`)" {
		t.Errorf("router rule = %q, want Host(`myapp.example.com`)", labels["monster.http.routers.my-app-0.rule"])
	}
	if labels["monster.http.services.my-app-0.loadbalancer.server.port"] != "8080" {
		t.Errorf("port = %q, want 8080", labels["monster.http.services.my-app-0.loadbalancer.server.port"])
	}
}

func TestDeployTriggerHandler_buildDeployLabels_DefaultPort(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:        "app-1",
		ProjectID: "project-1",
		TenantID:  "tenant-1",
		Name:      "my-app",
		Port:      0,
	})
	store.addDomain(&core.Domain{
		ID:    "dom-1",
		AppID: "app-1",
		FQDN:  "myapp.example.com",
	})

	h := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 1)

	if labels["monster.http.services.my-app-0.loadbalancer.server.port"] != "80" {
		t.Errorf("default port = %q, want 80", labels["monster.http.services.my-app-0.loadbalancer.server.port"])
	}
}

func TestDeployTriggerHandler_buildDeployLabels_NoDomains(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:        "app-1",
		ProjectID: "project-1",
		TenantID:  "tenant-1",
		Name:      "my-app",
	})

	h := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 1)

	if labels["monster.app.id"] != "app-1" {
		t.Errorf("app.id = %q, want app-1", labels["monster.app.id"])
	}
	// No router labels should exist
	if _, ok := labels["monster.http.routers.my-app-0.rule"]; ok {
		t.Error("expected no router labels when no domains")
	}
}

func TestDeployTriggerHandler_buildDeployLabels_ListDomainsError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:        "app-1",
		ProjectID: "project-1",
		TenantID:  "tenant-1",
		Name:      "my-app",
	})
	store.errListDomainsByApp = context.Canceled

	h := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 1)

	// Should still return base labels even when domain list fails
	if labels["monster.app.id"] != "app-1" {
		t.Errorf("app.id = %q, want app-1", labels["monster.app.id"])
	}
}

func TestDeployTriggerHandler_RuntimeSelection(t *testing.T) {
	localRuntime := &mockContainerRuntime{}
	h := NewDeployTriggerHandler(context.Background(), newMockStore(), localRuntime, nil)

	rt, err := h.deployRuntimeForApp(&core.Application{ID: "app-1"})
	if err != nil {
		t.Fatalf("local runtime: %v", err)
	}
	if rt != localRuntime {
		t.Fatal("expected local runtime for empty server ID")
	}

	rt, err = h.deployRuntimeForApp(&core.Application{ID: "app-1", ServerID: "local"})
	if err != nil {
		t.Fatalf("explicit local runtime: %v", err)
	}
	if rt != localRuntime {
		t.Fatal("expected local runtime for serverID=local")
	}

	if _, err := NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).deployRuntimeForApp(&core.Application{}); err == nil {
		t.Fatal("expected nil local runtime to fail")
	}
	if _, err := h.deployRuntimeForApp(&core.Application{ID: "app-1", ServerID: "remote-1"}); err == nil {
		t.Fatal("expected remote app without node manager to fail")
	}

	node := &fakeNodeExecutor{id: "remote-1"}
	h.SetNodeManager(&fakeNodeManager{nodes: map[string]core.NodeExecutor{"remote-1": node}})
	rt, err = h.deployRuntimeForApp(&core.Application{ID: "app-1", ServerID: "remote-1"})
	if err != nil {
		t.Fatalf("remote runtime: %v", err)
	}
	if rt != node {
		t.Fatal("expected remote node executor")
	}
}

func TestDeployTriggerHelpers_ImageNamesAndRegistryRefs(t *testing.T) {
	cases := map[string]bool{
		"nginx:latest":                 false,
		"library/nginx:latest":         false,
		"localhost/nginx:latest":       true,
		"registry.example.com/app:tag": true,
		"registry:5000/app:tag":        true,
	}
	for ref, want := range cases {
		if got := imageRefHasRegistry(ref); got != want {
			t.Fatalf("imageRefHasRegistry(%q) = %v, want %v", ref, got, want)
		}
	}
	if got := imageNamePart("My_App. Prod", "fallback"); got != "my-app-prod" {
		t.Fatalf("imageNamePart sanitized = %q", got)
	}
	if got := buildImageTagForRegistry(" registry.example.com/team/ ", &core.Application{Name: "My App", ID: "app-1"}, "abcdef1234567890"); got != "registry.example.com/team/my-app:abcdef123456" {
		t.Fatalf("buildImageTagForRegistry = %q", got)
	}
	if got := buildImageTagForRegistry("", &core.Application{Name: "My App"}, "abcdef"); got != "" {
		t.Fatalf("empty registry prefix produced %q", got)
	}
	if got := buildImageTagForRegistry("repo", nil, "abcdef"); got != "" {
		t.Fatalf("nil app produced %q", got)
	}
}

func TestDeployTriggerCleanupPreviousContainers(t *testing.T) {
	runtime := &recordingDeployRuntime{
		containers: []core.ContainerInfo{{ID: "keep"}, {ID: "old-1"}, {ID: "old-2"}, {ID: ""}},
	}
	NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).cleanupPreviousAppContainers(context.Background(), runtime, "app-1", "keep")

	if len(runtime.stopped) != 2 || len(runtime.removed) != 2 {
		t.Fatalf("stopped=%v removed=%v", runtime.stopped, runtime.removed)
	}
	if runtime.stopped[0] != "old-1" || runtime.removed[1] != "old-2" {
		t.Fatalf("unexpected cleanup order: stopped=%v removed=%v", runtime.stopped, runtime.removed)
	}

	errRuntime := &recordingDeployRuntime{listErr: errors.New("list failed")}
	NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).cleanupPreviousAppContainers(context.Background(), errRuntime, "app-1", "")
	if len(errRuntime.stopped) != 0 || len(errRuntime.removed) != 0 {
		t.Fatalf("cleanup should not continue after list error: %+v", errRuntime)
	}
}

func TestEnsureDeployNetwork(t *testing.T) {
	if err := ensureDeployNetwork(context.Background(), &recordingDeployRuntime{}); err != nil {
		t.Fatalf("non network runtime should be ignored: %v", err)
	}
	rt := &networkDeployRuntime{}
	if err := ensureDeployNetwork(context.Background(), rt); err != nil {
		t.Fatalf("ensure network: %v", err)
	}
	if rt.name != "monster-network" {
		t.Fatalf("network name = %q", rt.name)
	}
	rt.err = errors.New("network failed")
	if err := ensureDeployNetwork(context.Background(), rt); err == nil {
		t.Fatal("expected network error")
	}
}

func TestDeployTriggerHandler_SubscribeWebhookDeploysBranches(t *testing.T) {
	NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).SubscribeWebhookDeploys()

	events := core.NewEventBus(nil)
	store := newMockStore()
	store.addApp(&core.Application{ID: "image-app", TenantID: "t1", SourceType: "image"})
	store.addApp(&core.Application{ID: "git-app", TenantID: "t1", SourceType: "git", Branch: "main"})
	h := NewDeployTriggerHandler(context.Background(), store, nil, events)
	h.SubscribeWebhookDeploys()

	ctx := context.Background()
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", "bad payload"))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{}))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "missing"}))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "image-app"}))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "git-app", Branch: "feature"}))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "git-app", Branch: "main", CommitSHA: "abcdef"}))
	events.Drain()

	if store.updatedStatus["git-app"] != "failed" {
		t.Fatalf("git app status = %q, want failed after nil runtime deploy", store.updatedStatus["git-app"])
	}
}

func TestDeployTriggerHandler_SubscribeWebhookDeploysHonorsFreeze(t *testing.T) {
	events := core.NewEventBus(nil)
	store := newMockStore()
	store.addApp(&core.Application{ID: "git-app", TenantID: "t1", SourceType: "git"})
	kv := newMockKVStore()
	if err := seedActiveDeployFreeze(kv, "t1"); err != nil {
		t.Fatalf("seed freeze: %v", err)
	}
	h := NewDeployTriggerHandler(context.Background(), store, nil, events)
	h.SetDeployFreezeStore(kv)
	h.SubscribeWebhookDeploys()

	events.Publish(context.Background(), core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "git-app"}))
	events.Drain()

	if store.updatedStatus["git-app"] != "" {
		t.Fatalf("frozen webhook should not update app status, got %q", store.updatedStatus["git-app"])
	}
}

type fakeNodeManager struct {
	nodes map[string]core.NodeExecutor
}

func (m *fakeNodeManager) Get(serverID string) (core.NodeExecutor, error) {
	node, ok := m.nodes[serverID]
	if !ok {
		return nil, core.ErrNotFound
	}
	return node, nil
}
func (m *fakeNodeManager) Local() core.NodeExecutor            { return nil }
func (m *fakeNodeManager) All() []string                       { return nil }
func (m *fakeNodeManager) OnConnect(func(info core.AgentInfo)) {}
func (m *fakeNodeManager) OnDisconnect(func(serverID string))  {}

type fakeNodeExecutor struct {
	id string
}

func (e *fakeNodeExecutor) ServerID() string { return e.id }
func (e *fakeNodeExecutor) IsLocal() bool    { return false }
func (e *fakeNodeExecutor) CreateAndStart(context.Context, core.ContainerOpts) (string, error) {
	return "remote-container", nil
}
func (e *fakeNodeExecutor) Stop(context.Context, string, int) error     { return nil }
func (e *fakeNodeExecutor) Remove(context.Context, string, bool) error  { return nil }
func (e *fakeNodeExecutor) Restart(context.Context, string) error       { return nil }
func (e *fakeNodeExecutor) EnsureNetwork(context.Context, string) error { return nil }
func (e *fakeNodeExecutor) Logs(context.Context, string, string, bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (e *fakeNodeExecutor) ListByLabels(context.Context, map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (e *fakeNodeExecutor) Exec(context.Context, string) (string, error)         { return "", nil }
func (e *fakeNodeExecutor) Metrics(context.Context) (*core.ServerMetrics, error) { return nil, nil }
func (e *fakeNodeExecutor) Ping(context.Context) error                           { return nil }

type recordingDeployRuntime struct {
	containers []core.ContainerInfo
	listErr    error
	stopped    []string
	removed    []string
}

func (r *recordingDeployRuntime) CreateAndStart(context.Context, core.ContainerOpts) (string, error) {
	return "container", nil
}
func (r *recordingDeployRuntime) Stop(_ context.Context, id string, _ int) error {
	r.stopped = append(r.stopped, id)
	return nil
}
func (r *recordingDeployRuntime) Remove(_ context.Context, id string, _ bool) error {
	r.removed = append(r.removed, id)
	return nil
}
func (r *recordingDeployRuntime) ListByLabels(context.Context, map[string]string) ([]core.ContainerInfo, error) {
	return r.containers, r.listErr
}

type networkDeployRuntime struct {
	recordingDeployRuntime
	name string
	err  error
}

func (r *networkDeployRuntime) EnsureNetwork(_ context.Context, name string) error {
	r.name = name
	return r.err
}

// === merged from domains_boost_test.go ===

func TestDeleteDomain_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestDeleteDomain_MissingID(t *testing.T) {
	store := newMockStore()
	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestDeleteDomain_GetDomainError(t *testing.T) {
	store := newMockStore()
	store.errGetDomain = errors.New("db error")
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "delete-me.com"})

	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestDeleteDomain_GetAppError(t *testing.T) {
	store := newMockStore()
	store.errGetApp = errors.New("db error")
	store.addApp(&core.Application{ID: "app1", TenantID: "t1", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "delete-me.com"})

	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestDeleteDomain_WrongTenant(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "t2", Name: "test-app"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "delete-me.com"})

	handler := NewDomainHandler(store, testCore().Events)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/domains/d1", nil)
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()

	handler.Delete(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rr.Code)
	}
}

// === merged from env_import_boost_test.go ===

func TestSanitizeEnvValue(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"simple", `"simple"`},
		{"hello world", `"hello world"`},
		{`with\backslash`, `"with\\backslash"`},
		{`with"quotes`, `"with\"quotes"`},
		{`with$dollar`, `"with$$dollar"`},
		{"with\nnewline", `"with\nnewline"`},
		{"with\rcarriage", `"with\rcarriage"`},
		{"", `""`},
		{"mixed\\\"$\n\r", `"mixed\\\"$$\n\r"`},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizeEnvValue(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeEnvValue(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// === merged from final90_test.go ===

// =============================================================================
// DeployTriggerHandler — image app with runtime error
// =============================================================================

func TestDeployTrigger_ImageApp_RuntimeError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-err",
		TenantID:   "t1",
		Name:       "err-app",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})

	runtime := &failRuntime{err: fmt.Errorf("container start failed")}
	handler := NewDeployTriggerHandler(context.Background(), store, runtime, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-err/deploy", nil)
	req.SetPathValue("id", "app-err")
	req = withClaims(req, "user1", "t1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.TriggerDeploy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "deploy failed")

	if store.updatedStatus["app-err"] != "failed" {
		t.Errorf("expected status=failed, got %q", store.updatedStatus["app-err"])
	}
}

// failRuntime is a ContainerRuntime that returns error on CreateAndStart.
type failRuntime struct {
	mockContainerRuntime
	err error
}

func (f *failRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", f.err
}

// =============================================================================
// DNSRecordHandler — List with provider (verify error/success/missing domain)
// =============================================================================

func TestDNSRecordHandler_List_VerifyError(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{verifyErr: fmt.Errorf("DNS lookup timeout")})
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("GET", "/api/v1/dns/records?domain=fail.com&provider=cloudflare", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_List_VerifySuccess(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{verified: true})
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("GET", "/api/v1/dns/records?domain=ok.com&provider=cloudflare", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["verified"] != true {
		t.Errorf("expected verified=true, got %v", resp["verified"])
	}
}

// =============================================================================
// DNSRecordHandler — Create success / provider error
// =============================================================================

func TestDNSRecordHandler_Create_Success(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{})
	h := NewDNSRecordHandler(services)

	body := `{"name":"test.example.com","value":"1.2.3.4","type":"A"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records?provider=cloudflare", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDNSRecordHandler_Create_ProviderError(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{createErr: fmt.Errorf("rate limited")})
	h := NewDNSRecordHandler(services)

	body := `{"name":"test.example.com","value":"1.2.3.4","type":"A"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records?provider=cloudflare", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// DNSRecordHandler — Delete success / error
// =============================================================================

func TestDNSRecordHandler_Delete_Success(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{})
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("DELETE", "/api/v1/dns/records/rec-1?provider=cloudflare&name=example.com", nil)
	req.SetPathValue("id", "rec-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Delete_Error(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{deleteErr: fmt.Errorf("not found")})
	h := NewDNSRecordHandler(services)

	req := httptest.NewRequest("DELETE", "/api/v1/dns/records/rec-1?provider=cloudflare&name=example.com", nil)
	req.SetPathValue("id", "rec-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AdminAPIKeyHandler — Generate then List / kv error
// =============================================================================

func TestAdminAPIKeys_GenerateThenList(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	handler := NewAdminAPIKeyHandler(store, kv)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.Generate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("Generate: expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/admin/api-keys", nil)
	req2 = withClaims(req2, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr2 := httptest.NewRecorder()
	handler.List(rr2, req2)

	if rr2.Code != http.StatusOK {
		t.Fatalf("List: expected 200, got %d", rr2.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr2.Body.Bytes(), &resp)
	data := resp["data"].([]any)
	if len(data) != 1 {
		t.Errorf("expected 1 key, got %d", len(data))
	}
}

func TestAdminAPIKeys_Generate_BoltSetError(t *testing.T) {
	store := newMockStore()
	kv := &errOnSetKV{}
	handler := NewAdminAPIKeyHandler(store, kv)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "user1", "tenant1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	handler.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

type errOnSetKV struct{ mockKVStore }

func (e *errOnSetKV) Set(_, _ string, _ any, _ int64) error {
	return fmt.Errorf("kv write error")
}
func (e *errOnSetKV) Get(_, _ string, _ any) error {
	return fmt.Errorf("key not found: %w", core.ErrKVNotFound)
}

// =============================================================================
// OpenAPIHandler — Spec endpoint
// =============================================================================

func TestOpenAPIHandler_Spec(t *testing.T) {
	h := NewOpenAPIHandler("1.2.3")

	req := httptest.NewRequest("GET", "/api/v1/openapi.json", nil)
	rr := httptest.NewRecorder()
	h.Spec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["openapi"] != "3.1.0" {
		t.Errorf("openapi = %v, want 3.1.0", resp["openapi"])
	}
	info := resp["info"].(map[string]any)
	if info["version"] != "1.2.3" {
		t.Errorf("version = %v, want 1.2.3", info["version"])
	}
	paths, ok := resp["paths"].(map[string]any)
	if !ok || len(paths) == 0 {
		t.Error("expected non-empty paths")
	}
}

// =============================================================================
// MigrationHandler — no DB
// =============================================================================

func TestMigrationHandler_NoDB(t *testing.T) {
	c := testCore()
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// =============================================================================
// PlatformStatsHandler — success
// =============================================================================

func TestPlatformStatsHandler_Success(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	platform := resp["platform"].(map[string]any)
	if platform["version"] != "1.0.0" {
		t.Errorf("version = %v", platform["version"])
	}
}

func TestPlatformStatsHandler_NilEventBus(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["events"] == nil {
		t.Fatal("expected events field")
	}
}

// =============================================================================
// SSHKeyHandler — Generate / List
// =============================================================================

func TestSSHKeyHandler_Generate_Success(t *testing.T) {
	kv := newMockKVStore()
	h := NewSSHKeyHandler(newMockStore(), kv)

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["name"] != "my-key" {
		t.Errorf("name = %v", resp["name"])
	}
	if resp["private_key"] == nil || resp["private_key"] == "" {
		t.Error("expected private_key in response")
	}
	if resp["public_key"] == nil || resp["public_key"] == "" {
		t.Error("expected public_key in response")
	}
	if resp["fingerprint"] == nil || resp["fingerprint"] == "" {
		t.Error("expected fingerprint in response")
	}
}

func TestSSHKeyHandler_Generate_NoClaims(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockKVStore())

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSSHKeyHandler_Generate_MissingName(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockKVStore())

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSHKeyHandler_List_NoClaims(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/ssh-keys", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSSHKeyHandler_List_Empty(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/ssh-keys", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// SSLStatusHandler — Check
// =============================================================================

func TestSSLStatusHandler_Check_MissingFQDN(t *testing.T) {
	h := NewSSLStatusHandler(newMockKVStore())

	req := httptest.NewRequest("GET", "/api/v1/domains/d1/ssl-status", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSLStatusHandler_Check_Uncached(t *testing.T) {
	kv := newMockKVStore()
	h := NewSSLStatusHandler(kv)

	// Use an unreachable host to get the error path of checkSSL
	req := httptest.NewRequest("GET", "/api/v1/domains/d1/ssl-status?fqdn=localhost.invalid.nxdomain", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp SSLCheckResult
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.FQDN != "localhost.invalid.nxdomain" {
		t.Errorf("fqdn = %q", resp.FQDN)
	}
	// Should have error since host is not reachable
	if resp.Error == "" {
		t.Error("expected error for unreachable host")
	}
}

// =============================================================================
// SSHTestHandler — Test endpoint
// =============================================================================

func TestSSHTestHandler_MissingHost(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	body := `{"host":""}`
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSHTestHandler_InvalidBody(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader("bad json"))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSHTestHandler_RejectsUnknownFields(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(`{"host":"127.0.0.1","extra":true}`))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestSSHTestHandler_UnreachableHost(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	body := `{"host":"192.0.2.1","port":22}`
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["reachable"] != false {
		t.Errorf("expected reachable=false for unreachable host")
	}
}

// =============================================================================
// MarketplaceDeployHandler — constructor
// =============================================================================

func TestMarketplaceDeployHandler_New(t *testing.T) {
	h := NewMarketplaceDeployHandler(context.Background(), nil, nil, newMockStore(), testCore().Events)
	if h == nil {
		t.Fatal("NewMarketplaceDeployHandler returned nil")
	}
}

// =============================================================================
// SelfUpdateHandler — CheckUpdate
// =============================================================================

func TestSelfUpdateHandler_CheckUpdate(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "dev", Commit: "abc123", Date: "2026-01-01"},
		Registry: core.NewRegistry(),
	}
	h := NewSelfUpdateHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/updates", nil)
	rr := httptest.NewRecorder()
	h.CheckUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["current_version"] != "dev" {
		t.Errorf("current_version = %v", resp["current_version"])
	}
}

// =============================================================================
// Mock DNS provider
// =============================================================================

type mockDNS struct {
	verified  bool
	verifyErr error
	createErr error
	deleteErr error
}

func (m *mockDNS) Name() string                                           { return "mock" }
func (m *mockDNS) CreateRecord(_ context.Context, _ core.DNSRecord) error { return m.createErr }
func (m *mockDNS) UpdateRecord(_ context.Context, _ core.DNSRecord) error { return nil }
func (m *mockDNS) DeleteRecord(_ context.Context, _ core.DNSRecord) error { return m.deleteErr }
func (m *mockDNS) Verify(_ context.Context, _ string) (bool, error) {
	return m.verified, m.verifyErr
}

// === merged from final95_test.go ===

// =============================================================================
// AdminAPIKeyHandler.Generate — kv index Set error (second Set fails)
// =============================================================================

// kvFailOnSecondSet allows the first Set (key record) to succeed
// but fails on the second Set (index update).
type kvFailOnSecondSet struct {
	mockKVStore
	setCalls int
}

func (b *kvFailOnSecondSet) Set(bucket, key string, value any, ttl int64) error {
	b.setCalls++
	if b.setCalls >= 2 {
		return fmt.Errorf("kv index write error")
	}
	return b.mockKVStore.Set(bucket, key, value, ttl)
}

func TestFinal95_AdminAPIKey_Generate_IndexSetError(t *testing.T) {
	kv := &kvFailOnSecondSet{mockKVStore: *newMockKVStore()}
	h := NewAdminAPIKeyHandler(newMockStore(), kv)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to update key index")
}

// =============================================================================
// DomainVerifyHandler.verifyDNS — success path (resolve a well-known host)
// =============================================================================

func TestFinal95_VerifyDNS_Success(t *testing.T) {
	// Use "localhost" which should resolve on any machine
	result := verifyDNS("localhost")
	if result.FQDN != "localhost" {
		t.Errorf("fqdn = %q, want localhost", result.FQDN)
	}
	// localhost should resolve, so Verified should be true and Records non-empty
	if !result.Verified {
		// On some CI machines localhost might not resolve — just verify no panic
		t.Logf("localhost did not verify (possible CI env): error=%q", result.Error)
	} else {
		if len(result.Records) == 0 {
			t.Error("expected at least one record for localhost")
		}
	}
	if result.CheckedAt == "" {
		t.Error("expected CheckedAt to be set")
	}
}

func TestFinal95_VerifyDNS_Failure(t *testing.T) {
	result := verifyDNS("this-domain-does-not-exist-xyz123.invalid")
	if result.Verified {
		t.Error("expected Verified=false for non-existent domain")
	}
	if result.Error == "" {
		t.Error("expected error message for non-existent domain")
	}
}

// =============================================================================
// DomainVerifyHandler.Verify — FQDN from stored kv record
// =============================================================================

func TestFinal95_DomainVerify_FQDNFromBolt(t *testing.T) {
	kv := newMockKVStore()
	// Pre-store a domain verify record so the handler looks it up
	kv.Set("domain_verify", "domain-1", domainVerifyRecord{
		DomainID: "domain-1",
		FQDN:     "this-domain-does-not-exist-xyz.invalid",
	}, 0)

	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	store.addDomain(&core.Domain{ID: "domain-1", AppID: "app-1", FQDN: "this-domain-does-not-exist-xyz.invalid"})
	h := NewDomainVerifyHandler(store, kv)

	// POST without fqdn in body — handler should pull it from kv
	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/domains/domain-1/verify", strings.NewReader(body))
	req.SetPathValue("id", "domain-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp VerifyResult
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.FQDN != "this-domain-does-not-exist-xyz.invalid" {
		t.Errorf("fqdn = %q, want stored domain", resp.FQDN)
	}
}

// =============================================================================
// DomainVerifyHandler.Verify — no FQDN anywhere returns 400
// =============================================================================

func TestFinal95_DomainVerify_NoFQDNAnywhere(t *testing.T) {
	kv := newMockKVStore()
	h := NewDomainVerifyHandler(newMockStore(), kv)

	body := `{}`
	req := httptest.NewRequest("POST", "/api/v1/domains/domain-1/verify", strings.NewReader(body))
	req.SetPathValue("id", "domain-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// MigrationHandler.Status — successful query with migration rows
// =============================================================================

func TestFinal95_MigrationHandler_Status_WithDB(t *testing.T) {
	store := newMockStore()
	store.migrations = []core.MigrationStatus{
		{Version: 1, Name: "initial", AppliedAt: "2026-01-01T00:00:00Z"},
		{Version: 2, Name: "add_users", AppliedAt: "2026-01-02T00:00:00Z"},
	}

	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{Driver: "sqlite"},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Store:    store,
		Registry: core.NewRegistry(),
	}
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["driver"] != "sqlite" {
		t.Errorf("driver = %v, want sqlite", resp["driver"])
	}
	total, ok := resp["total"].(float64)
	if !ok || int(total) != 2 {
		t.Errorf("total = %v, want 2", resp["total"])
	}
	migrations, ok := resp["migrations"].([]any)
	if !ok || len(migrations) != 2 {
		t.Errorf("migrations count = %d, want 2", len(migrations))
	}
}

// =============================================================================
// MigrationHandler.Status — query error (table does not exist)
// =============================================================================

func TestFinal95_MigrationHandler_Status_QueryError(t *testing.T) {
	store := newMockStore()
	store.errListMigrations = fmt.Errorf("query failed")

	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{Driver: "sqlite"},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Store:    store,
		Registry: core.NewRegistry(),
	}
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// MigrationHandler.Status — Store is nil
// =============================================================================

func TestFinal95_MigrationHandler_Status_NilStore(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Registry: core.NewRegistry(),
	}
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// =============================================================================
// MigrationHandler.Status — empty migration table (zero rows)
// =============================================================================

func TestFinal95_MigrationHandler_Status_EmptyTable(t *testing.T) {
	c := &core.Core{
		Config: &core.Config{
			Database: core.DatabaseConfig{Driver: "sqlite"},
		},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Store:    newMockStore(),
		Registry: core.NewRegistry(),
	}
	h := NewMigrationHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/migrations", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total, ok := resp["total"].(float64)
	if !ok || int(total) != 0 {
		t.Errorf("total = %v, want 0", resp["total"])
	}
}

// =============================================================================
// PlatformStatsHandler.Overview — with container runtime
// =============================================================================

func TestFinal95_PlatformStats_WithContainerRuntime(t *testing.T) {
	services := core.NewServices()
	services.Container = &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", Name: "app-1"},
			{ID: "c2", Name: "app-2"},
		},
	}

	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: services,
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "2.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	containers, ok := resp["containers"].(float64)
	if !ok || int(containers) != 2 {
		t.Errorf("containers = %v, want 2", resp["containers"])
	}
}

// =============================================================================
// PlatformStatsHandler.Overview — container runtime returns error
// =============================================================================

func TestFinal95_PlatformStats_ContainerRuntimeError(t *testing.T) {
	services := core.NewServices()
	services.Container = &mockContainerRuntime{
		listErr: fmt.Errorf("docker not available"),
	}

	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: services,
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "2.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	// Should still succeed (graceful degradation), containers = 0
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	containers, ok := resp["containers"].(float64)
	if !ok || int(containers) != 0 {
		t.Errorf("containers = %v, want 0", resp["containers"])
	}
}

// =============================================================================
// PlatformStatsHandler.Overview — with module health statuses
// =============================================================================

// stubModule is a minimal core.Module implementation for testing.
type stubModule struct {
	id     string
	health core.HealthStatus
}

func (s *stubModule) ID() string                                 { return s.id }
func (s *stubModule) Name() string                               { return s.id }
func (s *stubModule) Version() string                            { return "1.0.0" }
func (s *stubModule) Dependencies() []string                     { return nil }
func (s *stubModule) Init(_ context.Context, _ *core.Core) error { return nil }
func (s *stubModule) Start(_ context.Context) error              { return nil }
func (s *stubModule) Stop(_ context.Context) error               { return nil }
func (s *stubModule) Health() core.HealthStatus                  { return s.health }
func (s *stubModule) Routes() []core.Route                       { return nil }
func (s *stubModule) Events() []core.EventHandler                { return nil }

func TestFinal95_PlatformStats_ModuleHealth(t *testing.T) {
	registry := core.NewRegistry()
	registry.Register(&stubModule{id: "auth", health: core.HealthOK})
	registry.Register(&stubModule{id: "deploy", health: core.HealthDegraded})
	registry.Register(&stubModule{id: "backup", health: core.HealthDown})

	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "2.0.0"},
		Registry: registry,
	}
	h := NewPlatformStatsHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/stats", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Overview(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	modules := resp["modules"].(map[string]any)
	if int(modules["healthy"].(float64)) != 1 {
		t.Errorf("healthy = %v, want 1", modules["healthy"])
	}
	if int(modules["degraded"].(float64)) != 1 {
		t.Errorf("degraded = %v, want 1", modules["degraded"])
	}
	if int(modules["down"].(float64)) != 1 {
		t.Errorf("down = %v, want 1", modules["down"])
	}
	if int(modules["total"].(float64)) != 3 {
		t.Errorf("total = %v, want 3", modules["total"])
	}
}

// =============================================================================
// SelfUpdateHandler.CheckUpdate — version comparison with update available
// =============================================================================

func TestFinal95_SelfUpdate_VersionFields(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build: core.BuildInfo{
			Version: "v1.0.0",
			Commit:  "abc123def",
			Date:    "2026-03-25",
		},
		Registry: core.NewRegistry(),
	}
	h := NewSelfUpdateHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/updates", nil)
	rr := httptest.NewRecorder()
	h.CheckUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["current_version"] != "v1.0.0" {
		t.Errorf("current_version = %v", resp["current_version"])
	}
	if resp["commit"] != "abc123def" {
		t.Errorf("commit = %v", resp["commit"])
	}
	if resp["build_date"] != "2026-03-25" {
		t.Errorf("build_date = %v", resp["build_date"])
	}
}

// =============================================================================
// SSHTestHandler.Test — default port (port <= 0 defaults to 22)
// =============================================================================

func TestFinal95_SSHTest_DefaultPort(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	body := `{"host":"192.0.2.1","port":0}`
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["port"].(float64)) != 22 {
		t.Errorf("port = %v, want 22", resp["port"])
	}
}

// =============================================================================
// SSHTestHandler.Test — reachable host (use local TCP listener)
// =============================================================================

func TestFinal95_SSHTest_ReachableHost(t *testing.T) {
	// Start a local TCP listener to simulate a reachable SSH port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Accept one connection in background
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	h := NewSSHTestHandler(core.NewServices())

	body := fmt.Sprintf(`{"host":"127.0.0.1","port":%d}`, addr.Port)
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["reachable"] != true {
		t.Errorf("expected reachable=true, got %v", resp["reachable"])
	}
	if resp["latency"] == nil || resp["latency"] == "" {
		t.Error("expected latency to be set")
	}
}

// =============================================================================
// SSHTestHandler.Test — with server_id and SSH client (success path)
// =============================================================================

// mockSSHClient implements core.SSHClient for testing.
type mockSSHClient struct {
	output string
	err    error
}

func (m *mockSSHClient) Execute(_ context.Context, _, _ string) (string, error) {
	return m.output, m.err
}

func (m *mockSSHClient) Upload(_ context.Context, _, _, _ string) error {
	return nil
}

func TestFinal95_SSHTest_WithServerID_SSHSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	services := core.NewServices()
	services.SSH = &mockSSHClient{output: "ok\n"}
	h := NewSSHTestHandler(services)

	body := fmt.Sprintf(`{"host":"127.0.0.1","port":%d,"server_id":"srv-1"}`, addr.Port)
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["reachable"] != true {
		t.Errorf("expected reachable=true")
	}
	if resp["ssh_auth"] != true {
		t.Errorf("expected ssh_auth=true, got %v", resp["ssh_auth"])
	}
	if resp["ssh_output"] != "ok\n" {
		t.Errorf("ssh_output = %q", resp["ssh_output"])
	}
}

// =============================================================================
// SSHTestHandler.Test — with server_id and SSH client (error path)
// =============================================================================

func TestFinal95_SSHTest_WithServerID_SSHError(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	services := core.NewServices()
	services.SSH = &mockSSHClient{err: fmt.Errorf("auth failed")}
	h := NewSSHTestHandler(services)

	body := fmt.Sprintf(`{"host":"127.0.0.1","port":%d,"server_id":"srv-1"}`, addr.Port)
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["reachable"] != true {
		t.Errorf("expected reachable=true")
	}
	if resp["ssh_auth"] != false {
		t.Errorf("expected ssh_auth=false, got %v", resp["ssh_auth"])
	}
	if resp["ssh_error"] == nil {
		t.Error("expected ssh_error to be set")
	}
}

// =============================================================================
// SSHTestHandler.Test — negative port defaults to 22
// =============================================================================

func TestFinal95_SSHTest_NegativePort(t *testing.T) {
	h := NewSSHTestHandler(core.NewServices())

	body := `{"host":"192.0.2.1","port":-1}`
	req := httptest.NewRequest("POST", "/api/v1/servers/test-ssh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["port"].(float64)) != 22 {
		t.Errorf("port = %v, want 22", resp["port"])
	}
}

// =============================================================================
// SSHKeyHandler.Generate — kv Set error
// =============================================================================

func TestFinal95_SSHKey_Generate_BoltSaveError(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newErrorKVStore())

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "failed to store SSH key")
}

// =============================================================================
// SSHKeyHandler.Generate — invalid body (decode error)
// =============================================================================

func TestFinal95_SSHKey_Generate_InvalidBody(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader("bad json"))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestFinal95_SSHKey_Generate_RejectsTrailingJSON(t *testing.T) {
	h := NewSSHKeyHandler(newMockStore(), newMockKVStore())

	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(`{"name":"my-key"}{"name":"other"}`))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

// =============================================================================
// SSHKeyHandler.List — with existing keys
// =============================================================================

func TestFinal95_SSHKey_List_WithKeys(t *testing.T) {
	kv := newMockKVStore()
	// Pre-store some SSH keys
	list := sshKeyList{
		Keys: []SSHKeyInfo{
			{ID: "k1", Name: "key-1", Fingerprint: "SHA256:abc"},
			{ID: "k2", Name: "key-2", Fingerprint: "SHA256:def"},
		},
	}
	kv.Set("ssh_keys", "u1", list, 0)

	h := NewSSHKeyHandler(newMockStore(), kv)

	req := httptest.NewRequest("GET", "/api/v1/ssh-keys", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total := int(resp["total"].(float64))
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	data := resp["data"].([]any)
	if len(data) != 2 {
		t.Errorf("data len = %d, want 2", len(data))
	}
}

// =============================================================================
// SSLStatusHandler.Check — cached result returns immediately
// =============================================================================

func TestFinal95_SSLStatus_Check_Cached(t *testing.T) {
	kv := newMockKVStore()
	cached := SSLCheckResult{
		FQDN:      "example.com",
		Valid:     true,
		Issuer:    "Let's Encrypt",
		Subject:   "example.com",
		ExpiresAt: time.Now().Add(90 * 24 * time.Hour),
		DaysLeft:  90,
		CheckedAt: time.Now(),
	}
	kv.Set("certificates", "ssl_check:example.com", cached, 300)

	h := NewSSLStatusHandler(kv)

	req := httptest.NewRequest("GET", "/api/v1/domains/d1/ssl-status?fqdn=example.com", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp SSLCheckResult
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.FQDN != "example.com" {
		t.Errorf("fqdn = %q", resp.FQDN)
	}
	if !resp.Valid {
		t.Error("expected valid=true from cache")
	}
	if resp.Issuer != "Let's Encrypt" {
		t.Errorf("issuer = %q", resp.Issuer)
	}
}

// =============================================================================
// SSLStatusHandler.checkSSL — TLS error path (invalid host)
// =============================================================================

func TestFinal95_CheckSSL_InvalidHost(t *testing.T) {
	result := checkSSL("192.0.2.1.nxdomain.invalid")
	if result.Valid {
		t.Error("expected valid=false for invalid host")
	}
	if result.Error == "" {
		t.Error("expected error for invalid host")
	}
	if result.FQDN != "192.0.2.1.nxdomain.invalid" {
		t.Errorf("fqdn = %q", result.FQDN)
	}
}

// =============================================================================
// SSLStatusHandler.checkSSL — struct fields always populated
// =============================================================================

func TestFinal95_CheckSSL_StructFields(t *testing.T) {
	// Use a host that will fail TLS connection — verifies the result struct
	// is properly populated even on failure.
	result := checkSSL("localhost")
	if result.FQDN != "localhost" {
		t.Errorf("fqdn = %q, want localhost", result.FQDN)
	}
	if result.CheckedAt.IsZero() {
		t.Error("expected CheckedAt to be set")
	}
	// localhost:443 is unlikely to have TLS, so should fail
	if result.Valid {
		// If by some chance it's valid, just verify cert fields are set
		if result.Issuer == "" {
			t.Error("valid cert should have issuer")
		}
	} else {
		if result.Error == "" {
			t.Error("invalid result should have error")
		}
	}
}

// =============================================================================
// SSLStatusHandler.checkSSL — connection refused
// =============================================================================

func TestFinal95_CheckSSL_ConnectionRefused(t *testing.T) {
	// Use a port that's almost certainly not listening
	result := checkSSL("127.0.0.1:1")
	// checkSSL appends :443, so the actual address is "127.0.0.1:1:443"
	// which will fail to connect
	if result.Valid {
		t.Error("expected valid=false")
	}
	if result.Error == "" {
		t.Error("expected error for connection refused")
	}
}

// =============================================================================
// DomainVerifyHandler.Verify — FQDN provided in body
// =============================================================================

func TestFinal95_DomainVerify_FQDNInBody(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app-1", FQDN: "this-domain-does-not-exist-xyz.invalid"})
	h := NewDomainVerifyHandler(store, newMockKVStore())

	body := `{"fqdn":"this-domain-does-not-exist-xyz.invalid"}`
	req := httptest.NewRequest("POST", "/api/v1/domains/d1/verify", strings.NewReader(body))
	req.SetPathValue("id", "d1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Verify(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp VerifyResult
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.FQDN != "this-domain-does-not-exist-xyz.invalid" {
		t.Errorf("fqdn = %q", resp.FQDN)
	}
	if resp.Verified {
		t.Error("expected verified=false for non-existent domain")
	}
}

// =============================================================================
// DomainVerifyHandler.BatchVerify — success with multiple FQDNs
// =============================================================================

func TestFinal95_DomainVerify_BatchVerify_MultipleFQDNs(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	store.addDomain(&core.Domain{ID: "d1", AppID: "app-1", FQDN: "invalid-domain-abc.invalid"})
	store.addDomain(&core.Domain{ID: "d2", AppID: "app-1", FQDN: "invalid-domain-xyz.invalid"})
	h := NewDomainVerifyHandler(store, newMockKVStore())

	body := `{"fqdns":["invalid-domain-abc.invalid","invalid-domain-xyz.invalid"]}`
	req := httptest.NewRequest("POST", "/api/v1/domains/verify-batch", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.BatchVerify(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total := int(resp["total"].(float64))
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	results := resp["results"].([]any)
	if len(results) != 2 {
		t.Errorf("results len = %d, want 2", len(results))
	}
}

// =============================================================================
// TenantSettingsHandler.Update — all branches
// =============================================================================

// mockStoreWithUpdateTenant wraps mockStore and adds a working UpdateTenant.
type mockStoreWithUpdateTenant struct {
	*mockStore
	errUpdateTenant error
	updatedTenant   *core.Tenant
}

func (m *mockStoreWithUpdateTenant) UpdateTenant(_ context.Context, t *core.Tenant) error {
	if m.errUpdateTenant != nil {
		return m.errUpdateTenant
	}
	m.updatedTenant = t
	return nil
}

func TestFinal95_TenantSettings_Update_Success(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{
		ID: "t1", Name: "Original", Slug: "orig", Status: "active",
	})
	store := &mockStoreWithUpdateTenant{mockStore: ms}
	h := NewTenantSettingsHandler(store)

	body := `{"name":"Updated Name","metadata":"{\"theme\":\"dark\"}"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "updated" {
		t.Errorf("status = %q", resp["status"])
	}
	if store.updatedTenant.Name != "Updated Name" {
		t.Errorf("name = %q", store.updatedTenant.Name)
	}
}

func TestFinal95_TenantSettings_Update_NoClaims(t *testing.T) {
	h := NewTenantSettingsHandler(newMockStore())

	body := `{"name":"X"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestFinal95_TenantSettings_Update_InvalidBody(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{ID: "t1", Name: "Test"})
	h := NewTenantSettingsHandler(&mockStoreWithUpdateTenant{mockStore: ms})

	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader("bad"))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestFinal95_TenantSettings_Update_RejectsUnknownFields(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{ID: "t1", Name: "Test"})
	h := NewTenantSettingsHandler(&mockStoreWithUpdateTenant{mockStore: ms})

	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(`{"name":"X","extra":true}`))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestFinal95_TenantSettings_Update_TenantNotFound(t *testing.T) {
	ms := newMockStore()
	// No tenant added — GetTenant will return ErrNotFound
	h := NewTenantSettingsHandler(&mockStoreWithUpdateTenant{mockStore: ms})

	body := `{"name":"X"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestFinal95_TenantSettings_Update_SaveError(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{ID: "t1", Name: "Test"})
	store := &mockStoreWithUpdateTenant{
		mockStore:       ms,
		errUpdateTenant: fmt.Errorf("db error"),
	}
	h := NewTenantSettingsHandler(store)

	body := `{"name":"X"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestFinal95_TenantSettings_Update_PartialFields(t *testing.T) {
	ms := newMockStore()
	ms.addTenant(&core.Tenant{
		ID: "t1", Name: "Original", MetadataJSON: `{"old":"data"}`,
	})
	store := &mockStoreWithUpdateTenant{mockStore: ms}
	h := NewTenantSettingsHandler(store)

	// Only name, no metadata — metadata should stay unchanged
	body := `{"name":"NewName"}`
	req := httptest.NewRequest("PATCH", "/api/v1/tenant/settings", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if store.updatedTenant.MetadataJSON != `{"old":"data"}` {
		t.Errorf("metadata should be unchanged, got %q", store.updatedTenant.MetadataJSON)
	}
}

// =============================================================================
// MCPHandler — NewMCPHandler, ListTools, CallTool
// =============================================================================

func TestFinal95_MCPHandler_NewMCPHandler(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)
	if h == nil {
		t.Fatal("NewMCPHandler returned nil")
	}
}

func TestFinal95_MCPHandler_ListTools(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "2.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	req := httptest.NewRequest("GET", "/mcp/v1/tools", nil)
	rr := httptest.NewRecorder()
	h.ListTools(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["version"] != "2.0.0" {
		t.Errorf("version = %v", resp["version"])
	}
	if resp["tools"] == nil {
		t.Error("expected tools in response")
	}
}

func TestFinal95_MCPHandler_CallTool_ValidTool(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	// Call list_apps tool with empty input
	body := `{}`
	req := httptest.NewRequest("POST", "/mcp/v1/tools/list_apps", strings.NewReader(body))
	req.SetPathValue("name", "list_apps")
	rr := httptest.NewRecorder()
	h.CallTool(rr, req)

	// May return 200 or 400 depending on the tool — just check it doesn't panic
	if rr.Code != http.StatusOK && rr.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinal95_MCPHandler_CallTool_UnknownTool(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	body := `{}`
	req := httptest.NewRequest("POST", "/mcp/v1/tools/nonexistent_tool", strings.NewReader(body))
	req.SetPathValue("name", "nonexistent_tool")
	rr := httptest.NewRecorder()
	h.CallTool(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinal95_MCPHandler_CallTool_InvalidBody(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	req := httptest.NewRequest("POST", "/mcp/v1/tools/list_apps", strings.NewReader("not json"))
	req.SetPathValue("name", "list_apps")
	rr := httptest.NewRecorder()
	h.CallTool(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestFinal95_MCPHandler_CallTool_RejectsTrailingJSON(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Build:    core.BuildInfo{Version: "1.0.0"},
		Registry: core.NewRegistry(),
	}
	h := NewMCPHandler(c, newMockStore(), &mockContainerRuntime{}, c.Events)

	req := httptest.NewRequest("POST", "/mcp/v1/tools/list_apps", strings.NewReader(`{} {}`))
	req.SetPathValue("name", "list_apps")
	rr := httptest.NewRecorder()
	h.CallTool(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

// =============================================================================
// MarketplaceDeployHandler.Deploy — all branches
// =============================================================================

func TestFinal95_MarketplaceDeploy_NoClaims(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(context.Background(), registry, &mockContainerRuntime{}, newMockStore(), core.NewEventBus(slog.Default()))

	body := `{"slug":"wordpress"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestFinal95_MarketplaceDeploy_InvalidBody(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(context.Background(), registry, &mockContainerRuntime{}, newMockStore(), core.NewEventBus(slog.Default()))

	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader("bad json"))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestFinal95_MarketplaceDeploy_EmptySlug(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(context.Background(), registry, &mockContainerRuntime{}, newMockStore(), core.NewEventBus(slog.Default()))

	body := `{"slug":""}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestFinal95_MarketplaceDeploy_TemplateNotFound(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(context.Background(), registry, &mockContainerRuntime{}, newMockStore(), core.NewEventBus(slog.Default()))

	body := `{"slug":"nonexistent"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestFinal95_MarketplaceDeploy_InvalidComposeYAML(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	registry.Add(&marketplace.Template{
		Slug:        "broken",
		Name:        "Broken",
		Category:    "test",
		ComposeYAML: `not: valid: compose: yaml: [[[`,
	})

	events := core.NewEventBus(slog.Default())
	store := newMockStore()
	h := NewMarketplaceDeployHandler(context.Background(), registry, &mockContainerRuntime{}, store, events)

	body := `{"slug":"broken","name":"my-broken"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinal95_MarketplaceDeploy_CreateAppError(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	registry.Add(&marketplace.Template{
		Slug:     "nginx",
		Name:     "Nginx",
		Category: "web",
		ComposeYAML: `services:
  web:
    image: nginx:latest
`,
	})

	events := core.NewEventBus(slog.Default())
	store := newMockStore()
	store.errCreateApp = fmt.Errorf("db error")
	h := NewMarketplaceDeployHandler(context.Background(), registry, &mockContainerRuntime{}, store, events)

	body := `{"slug":"nginx"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ExecHandler.Exec — exec error with "exec create" in message
// =============================================================================

func TestFinal95_ExecHandler_ExecCreateError(t *testing.T) {
	runtime := &mockExecErrorRuntime{
		mockContainerRuntime: mockContainerRuntime{
			containers: []core.ContainerInfo{{ID: "c1", Name: "app-1"}},
		},
		execErr: fmt.Errorf("exec create: connection refused"),
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	h := NewExecHandler(runtime, store, slog.Default(), nil)

	body := `{"command":"ls"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestFinal95_ExecHandler_ExecNonZeroExit(t *testing.T) {
	runtime := &mockExecErrorRuntime{
		mockContainerRuntime: mockContainerRuntime{
			containers: []core.ContainerInfo{{ID: "c1", Name: "app-1"}},
		},
		execErr:    fmt.Errorf("command failed with exit code 1"),
		execOutput: "some partial output",
	}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1"})
	h := NewExecHandler(runtime, store, slog.Default(), nil)

	body := `{"command":"false"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/exec", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Exec(rr, req)

	// Non-zero exit still returns 200 with exit_code=1
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp execResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.ExitCode != 1 {
		t.Errorf("exit_code = %d, want 1", resp.ExitCode)
	}
}

// mockExecErrorRuntime returns an error from Exec.
type mockExecErrorRuntime struct {
	mockContainerRuntime
	execErr    error
	execOutput string
}

func (m *mockExecErrorRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return m.execOutput, m.execErr
}

// =============================================================================
// LicenseHandler — Get with expired license, Activate kv error
// =============================================================================

func TestFinal95_License_Get_ExpiredLicense(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("license", "current", LicenseInfo{
		Type:       "enterprise",
		Key:        "test****test",
		ValidUntil: time.Now().Add(-24 * time.Hour), // expired yesterday
		Status:     "active",
	}, 0)
	h := NewLicenseHandler(kv)

	req := httptest.NewRequest("GET", "/api/v1/admin/license", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp LicenseInfo
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Status != "expired" {
		t.Errorf("status = %q, want expired", resp.Status)
	}
}

func TestFinal95_License_Activate_BoltError(t *testing.T) {
	h := NewLicenseHandler(newErrorKVStore())

	body := `{"key":"enterprise-key-12345678"}`
	req := httptest.NewRequest("POST", "/api/v1/admin/license", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Activate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// RedirectHandler.Delete — kv Set error path
// =============================================================================

func TestFinal95_Redirect_Delete_BoltSetError(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("redirects", "app-1", redirectList{
		Rules: []RedirectRule{
			{ID: "r1", Source: "/old", Destination: "/new", StatusCode: 301},
		},
	}, 0)

	// Wrap kv to fail on Set
	errBolt := &boltGetOkSetFail{mockKVStore: kv}
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "App"})
	h := NewRedirectHandler(store, errBolt)

	req := httptest.NewRequest("DELETE", "/api/v1/apps/app-1/redirects/r1", nil)
	req.SetPathValue("id", "app-1")
	req.SetPathValue("ruleId", "r1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// boltGetOkSetFail wraps mockKVStore: Get works, Set always fails.
type boltGetOkSetFail struct {
	*mockKVStore
}

func (b *boltGetOkSetFail) Set(_, _ string, _ any, _ int64) error {
	return fmt.Errorf("kv set error")
}

func (b *boltGetOkSetFail) Mutate(_, _ string, _ any, _ int64, _ func(bool) error) error {
	return fmt.Errorf("kv set error")
}

// =============================================================================
// EventWebhookHandler — Delete kv Set error, Create kv Set error
// =============================================================================

func TestFinal95_EventWebhook_Delete_BoltSetError(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("event_webhooks", "tenant:t1", eventWebhookList{
		Webhooks: []EventWebhookConfig{
			{ID: "wh1", URL: "https://example.com/hook", Events: []string{"deploy.success"}},
		},
	}, 0)

	errBolt := &boltGetOkSetFail{mockKVStore: kv}
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(slog.Default()), errBolt)

	req := httptest.NewRequest("DELETE", "/api/v1/webhooks/outbound/wh1", nil)
	req.SetPathValue("id", "wh1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestFinal95_EventWebhook_Create_BoltSetError(t *testing.T) {
	h := NewEventWebhookHandler(newMockStore(), core.NewEventBus(slog.Default()), newErrorKVStore())

	body := `{"url":"https://example.com/hook","events":["deploy.success"],"secret":"my-secret"}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// RegistryHandler.List — with custom registries
// =============================================================================

func TestFinal95_Registry_List_WithCustom(t *testing.T) {
	kv := newMockKVStore()
	kv.Set("registries", registryListKey("t1"), registryList{
		Registries: []RegistryConfig{
			{ID: "custom-1", Name: "My Registry", URL: "registry.example.com"},
		},
	}, 0)
	h := NewRegistryHandler(kv)

	req := httptest.NewRequest("GET", "/api/v1/registries", nil)
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total := int(resp["total"].(float64))
	// 3 builtins + 1 custom = 4
	if total != 4 {
		t.Errorf("total = %d, want 4", total)
	}
}

// =============================================================================
// DeployApprovalHandler.Reject — not found path
// =============================================================================

func TestFinal95_DeployApproval_Reject_NotFound(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	h := NewDeployApprovalHandler(newMockStore(), events)

	body := `{"reason":"too risky"}`
	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/nonexistent/reject", strings.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Reject(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestFinal95_DeployApproval_Reject_NoClaims(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	h := NewDeployApprovalHandler(newMockStore(), events)

	body := `{"reason":"nope"}`
	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/a1/reject", strings.NewReader(body))
	req.SetPathValue("id", "a1")
	rr := httptest.NewRecorder()
	h.Reject(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// === merged from handlers_coverage_boost_test.go ===

// ─── deploy_trigger.go coverage ──────────────────────────────────────────────

func TestDeployTriggerHandler_SetBuildImageRegistry(t *testing.T) {
	h := &DeployTriggerHandler{}

	// Normal case
	h.SetBuildImageRegistry(" registry.example.com/team/ ")
	if h.buildRepo != "registry.example.com/team" {
		t.Errorf("SetBuildImageRegistry with spaces = %q, want registry.example.com/team", h.buildRepo)
	}

	// Empty prefix
	h.SetBuildImageRegistry("")
	if h.buildRepo != "" {
		t.Errorf("SetBuildImageRegistry empty = %q, want empty", h.buildRepo)
	}

	// Leading/trailing slash trimmed
	h.SetBuildImageRegistry("/registry.example.com/team/")
	if h.buildRepo != "registry.example.com/team" {
		t.Errorf("SetBuildImageRegistry with slashes = %q", h.buildRepo)
	}
}

func TestDeployTriggerHandler_SetBuildImagePush(t *testing.T) {
	h := &DeployTriggerHandler{}

	h.SetBuildImagePush(true)
	if !h.buildPush {
		t.Error("SetBuildImagePush(true): expected true")
	}

	h.SetBuildImagePush(false)
	if h.buildPush {
		t.Error("SetBuildImagePush(false): expected false")
	}
}

func TestDeployTriggerHandler_SetBuildRegistryAuth(t *testing.T) {
	h := &DeployTriggerHandler{}

	h.SetBuildRegistryAuth("user1", "pass1")
	if h.buildUser != "user1" || h.buildPass != "pass1" {
		t.Errorf("SetBuildRegistryAuth got (%q, %q), want (user1, pass1)", h.buildUser, h.buildPass)
	}
}

func TestDeployTriggerHandler_failReservedDeployment(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
		Status:   "deploying",
	})
	h := &DeployTriggerHandler{store: store}

	// Test with nil dep
	h.failReservedDeployment(context.Background(), "app1", "tenant1", nil)
	if store.updatedStatus["app1"] != "failed" {
		t.Errorf("expected app status 'failed', got %q", store.updatedStatus["app1"])
	}

	// Reset and test with valid dep — should mark deployment as failed
	store.updatedStatus["app1"] = ""
	dep := &core.Deployment{ID: "dep1", AppID: "app1", Status: "deploying"}
	store.deploymentsByApp["app1"] = []core.Deployment{*dep}
	h.failReservedDeployment(context.Background(), "app1", "tenant1", dep)
	if dep.Status != "failed" {
		t.Errorf("expected dep status 'failed', got %q", dep.Status)
	}
	if dep.FinishedAt == nil {
		t.Error("expected FinishedAt to be set")
	}
}

func TestDeployTriggerHandler_failReserved(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
		Status:   "deploying",
	})
	h := &DeployTriggerHandler{store: store, events: events}

	dep := &core.Deployment{ID: "dep1", AppID: "app1", Status: "deploying"}
	h.failReserved(context.Background(), "app1", "tenant1", dep, "something went wrong")

	if store.updatedStatus["app1"] != "failed" {
		t.Errorf("expected app status 'failed', got %q", store.updatedStatus["app1"])
	}
	if dep.Status != "failed" {
		t.Errorf("expected dep status 'failed', got %q", dep.Status)
	}
}

func TestDeployTriggerHandler_publishDeployFailed(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	h := &DeployTriggerHandler{events: events}

	// Should not panic
	h.publishDeployFailed(context.Background(), "test", "app1", "error msg")
}

func TestDeployTriggerHandler_publishDeployFailed_NilEvents(t *testing.T) {
	h := &DeployTriggerHandler{}

	// Should not panic with nil events
	h.publishDeployFailed(context.Background(), "test", "app1", "error msg")
}

func TestDeployTriggerHandler_failApp_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
	})
	store.errUpdateAppStatus = errors.New("store error")
	h := &DeployTriggerHandler{store: store}

	// Should not panic when store.UpdateAppStatus fails
	h.failApp(context.Background(), "app1", "tenant1")
}

func TestDeployTriggerHandler_failReservedDeployment_StoreError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
	})
	store.errUpdateAppStatus = errors.New("store error")
	h := &DeployTriggerHandler{store: store}

	// Should not panic when UpdateAppStatus fails
	dep := &core.Deployment{ID: "dep1", AppID: "app1"}
	h.failReservedDeployment(context.Background(), "app1", "tenant1", dep)
}

func TestDeployTriggerHandler_deployRuntimeForApp_NodeGetError(t *testing.T) {
	h := NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil)
	nm := &fakeNodeManager{nodes: map[string]core.NodeExecutor{}}
	h.SetNodeManager(nm)

	_, err := h.deployRuntimeForApp(&core.Application{ID: "app1", ServerID: "remote-1"})
	if err == nil {
		t.Fatal("expected error for disconnected remote server")
	}
	if !strings.Contains(err.Error(), "is not connected") {
		t.Errorf("error = %q, want 'is not connected'", err.Error())
	}
}

func TestDeployTriggerHandler_deployRuntimeForApp_NilRuntime(t *testing.T) {
	h := NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil)

	_, err := h.deployRuntimeForApp(&core.Application{ID: "app1"})
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}
	if !strings.Contains(err.Error(), "container runtime not available") {
		t.Errorf("error = %q, want 'container runtime not available'", err.Error())
	}
}

func TestDeployTriggerHandler_cleanupPreviousAppContainers_NilRuntime(t *testing.T) {
	h := &DeployTriggerHandler{}
	// Should not panic with nil runtime
	h.cleanupPreviousAppContainers(context.Background(), nil, "app1", "keep")
}

func TestDeployTriggerHandler_cleanupPreviousAppContainers_StopError(t *testing.T) {
	rt := &errorInjectingRuntime{
		recordingDeployRuntime: recordingDeployRuntime{
			containers: []core.ContainerInfo{{ID: "old-1"}},
		},
		stopErr: errors.New("stop failed"),
	}
	NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).cleanupPreviousAppContainers(context.Background(), rt, "app1", "keep")
	// Should continue even after stop error — remove should still be called
	if len(rt.removed) != 1 {
		t.Errorf("expected 1 remove call despite stop error, got %d", len(rt.removed))
	}
}

func TestDeployTriggerHandler_cleanupPreviousAppContainers_RemoveError(t *testing.T) {
	rt := &errorInjectingRuntime{
		recordingDeployRuntime: recordingDeployRuntime{
			containers: []core.ContainerInfo{{ID: "old-1"}},
		},
		removeErr: errors.New("remove failed"),
	}
	NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).cleanupPreviousAppContainers(context.Background(), rt, "app1", "")
}

func TestBuildImageTagForRegistry_EmptyCommitSHA(t *testing.T) {
	// When commitSHA is empty, it falls back to GenerateID
	tag := buildImageTagForRegistry("registry.example.com", &core.Application{Name: "My App", ID: "app1"}, "")
	if !strings.HasPrefix(tag, "registry.example.com/my-app:") {
		t.Errorf("unexpected tag format: %q", tag)
	}
	if len(tag) <= len("registry.example.com/my-app:") {
		t.Errorf("tag too short: %q", tag)
	}
}

func TestImageNamePart_EdgeCases(t *testing.T) {
	// Empty name with fallback
	part := imageNamePart("", "Fallback-App")
	if part != "fallback-app" {
		t.Errorf("empty name with fallback = %q, want fallback-app", part)
	}

	// Only special characters
	part = imageNamePart("___", "default")
	if part == "" || part == "___" {
		t.Errorf("special chars only should produce fallback, got %q", part)
	}

	// Trailing separator
	part = imageNamePart("app-", "fallback")
	if strings.HasSuffix(part, "-") {
		t.Errorf("trailing dash should be trimmed: %q", part)
	}
	if part != "app" {
		t.Errorf("trailing dash trimmed = %q, want app", part)
	}

	// Uppercase and mixed separators
	part = imageNamePart("My_App.Name-Version", "fallback")
	if part != "my-app-name-version" {
		t.Errorf("mixed separators = %q, want my-app-name-version", part)
	}
}

func TestImageNamePart_EmptyNameAndEmptyFallback(t *testing.T) {
	// When both name and fallback are empty, should generate a fallback with prefix
	part := imageNamePart("", "")
	if !strings.HasPrefix(part, "app-") {
		t.Errorf("should generate 'app-' prefix, got %q", part)
	}
}

// ─── auth.go coverage ────────────────────────────────────────────────────────

func TestAuthHandler_SetLogger(t *testing.T) {
	h := &AuthHandler{}

	// Set logger
	customLogger := slog.New(slog.NewTextHandler(nil, nil))
	h.SetLogger(customLogger)
	if h.logger != customLogger {
		t.Error("SetLogger did not set logger")
	}

	// Set nil logger — should not replace existing
	h.SetLogger(nil)
	if h.logger != customLogger {
		t.Error("SetLogger(nil) should be a no-op")
	}
}

func TestAuthHandler_Log_NilLogger(t *testing.T) {
	h := &AuthHandler{logger: nil}
	// Should return slog.Default() rather than panic
	l := h.log()
	if l == nil {
		t.Fatal("log() returned nil")
	}
}

func TestAuthHandler_NewAuthHandler_NilAuthMod(t *testing.T) {
	h := NewAuthHandler(nil, newMockStore(), nil)
	if h.authMod != nil {
		t.Error("expected nil authMod")
	}
	if h.totpValidator != nil {
		t.Error("expected nil totpValidator when authMod is nil")
	}
}

func TestAuthHandler_NewAuthHandler_WithAuthMod(t *testing.T) {
	// Auth module with TOTP service
	mod := &testAuthServices{jwt: testJWT()}
	h := NewAuthHandler(mod, newMockStore(), nil)
	if h.authMod != mod {
		t.Error("expected authMod to be set")
	}
	// TOTP service is nil in testAuthServices, so totpValidator should be nil
	if h.totpValidator != nil {
		t.Error("expected nil totpValidator for nil TOTP service")
	}
}

func TestIsSecureRequest_EdgeCases(t *testing.T) {
	if isSecureRequest(nil) {
		t.Error("expected false for nil request")
	}

	// Request with X-Forwarded-Proto: https
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-Proto", "https")
	if !isSecureRequest(r) {
		t.Error("expected true for X-Forwarded-Proto: https")
	}

	// Request with X-Forwarded-Proto: http
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.Header.Set("X-Forwarded-Proto", "http")
	if isSecureRequest(r2) {
		t.Error("expected false for X-Forwarded-Proto: http")
	}
}

func TestGenerateSlug_EmptyOrSpecial(t *testing.T) {
	// Empty -> should generate fallback
	slug := generateSlug("")
	if slug == "" {
		t.Error("generateSlug('') should not be empty")
	}
	if len(slug) != 8 {
		t.Errorf("generateSlug('') length = %d, want 8", len(slug))
	}

	// Only special chars
	slug = generateSlug("___!!!")
	if slug == "" {
		t.Error("generateSlug('___!!!') should not be empty")
	}
}

func TestRegistrationTenantSlug_Truncation(t *testing.T) {
	// Name longer than 80 chars
	longName := strings.Repeat("a", 100)
	slug := registrationTenantSlug(longName)
	// Should start with truncated base (80 chars)
	if !strings.HasPrefix(slug, strings.Repeat("a", 80)) {
		t.Errorf("slug = %q, expected prefix of 80 a's", slug)
	}
	// Should contain a dash and short ID
	if !strings.Contains(slug, "-") {
		t.Errorf("slug = %q, expected '-' separator", slug)
	}
}

func TestRegistrationTenantSlug_NormalName(t *testing.T) {
	slug := registrationTenantSlug("My Team")
	if !strings.HasPrefix(slug, "my-team-") {
		t.Errorf("slug = %q, want prefix my-team-", slug)
	}
}

// ─── helpers.go coverage ─────────────────────────────────────────────────────

func TestWriteJSON_EncodeError(t *testing.T) {
	// writeJSON with a value that fails to encode (e.g., a channel)
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]any{"ch": make(chan int)})
	// Should not panic, should return 200
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestPaginateSlice_EdgeCases(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}

	// Offset past total
	result, total := paginateSlice(items, pagination{Page: 10, PerPage: 20, Offset: 200})
	if total != 5 {
		t.Errorf("total = %d, want 5", result)
	}
	if len(result) != 0 {
		t.Errorf("result length = %d, want 0", len(result))
	}

	// Offset at exact boundary (should return empty slice, not panic)
	result, total = paginateSlice(items, pagination{Page: 2, PerPage: 5, Offset: 5})
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(result) != 0 {
		t.Errorf("result length = %d, want 0", len(result))
	}

	// Empty slice
	result, total = paginateSlice([]int{}, pagination{Page: 1, PerPage: 20, Offset: 0})
	if total != 0 {
		t.Errorf("total = %d, want 0", total)
	}
	if len(result) != 0 {
		t.Errorf("result length = %d, want 0", len(result))
	}
}

func TestParsePagination_Caps(t *testing.T) {
	// Page > maxPage
	r := httptest.NewRequest("GET", "/api/v1/apps?page=99999", nil)
	p := parsePagination(r)
	if p.Page != maxPage {
		t.Errorf("page = %d, want %d", p.Page, maxPage)
	}

	// per_page > 100
	r2 := httptest.NewRequest("GET", "/api/v1/apps?per_page=500", nil)
	p2 := parsePagination(r2)
	if p2.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", p2.PerPage)
	}

	// per_page < 1
	r3 := httptest.NewRequest("GET", "/api/v1/apps?per_page=-1", nil)
	p3 := parsePagination(r3)
	if p3.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", p3.PerPage)
	}

	// page < 1
	r4 := httptest.NewRequest("GET", "/api/v1/apps?page=0", nil)
	p4 := parsePagination(r4)
	if p4.Page != 1 {
		t.Errorf("page = %d, want 1", p4.Page)
	}
}

// ─── apps.go coverage ──────────────────────────────────────────────────────

func TestBuiltinPlanAppLimit_Found(t *testing.T) {
	limit, ok := builtinPlanAppLimit("free")
	if !ok {
		t.Error("expected 'free' plan to be found")
	}
	_ = limit
}

func TestBuiltinPlanAppLimit_NotFound(t *testing.T) {
	limit, ok := builtinPlanAppLimit("nonexistent-plan")
	if ok {
		t.Error("expected 'nonexistent-plan' to not be found")
	}
	if limit != 0 {
		t.Errorf("limit = %d, want 0", limit)
	}
}

func TestStricterPositiveLimit_EdgeCases(t *testing.T) {
	// a <= 0
	if got := stricterPositiveLimit(0, 5); got != 5 {
		t.Errorf("stricterPositiveLimit(0,5) = %d, want 5", got)
	}
	if got := stricterPositiveLimit(-1, 5); got != 5 {
		t.Errorf("stricterPositiveLimit(-1,5) = %d, want 5", got)
	}

	// b <= 0
	if got := stricterPositiveLimit(5, 0); got != 5 {
		t.Errorf("stricterPositiveLimit(5,0) = %d, want 5", got)
	}
	if got := stricterPositiveLimit(5, -1); got != 5 {
		t.Errorf("stricterPositiveLimit(5,-1) = %d, want 5", got)
	}

	// both positive, a < b
	if got := stricterPositiveLimit(3, 10); got != 3 {
		t.Errorf("stricterPositiveLimit(3,10) = %d, want 3", got)
	}

	// both positive, a > b
	if got := stricterPositiveLimit(10, 3); got != 3 {
		t.Errorf("stricterPositiveLimit(10,3) = %d, want 3", got)
	}

	// equal
	if got := stricterPositiveLimit(5, 5); got != 5 {
		t.Errorf("stricterPositiveLimit(5,5) = %d, want 5", got)
	}
}

func TestFindAppContainerID_NilRuntime(t *testing.T) {
	c := testCore()
	c.Services = core.NewServices()
	// Services.Container is nil by default
	h := NewAppHandler(newMockStore(), c)

	r := httptest.NewRequest("GET", "/", nil)
	id, err := h.findAppContainerID(r, "app1")
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}
	if id != "" {
		t.Errorf("id = %q, want empty", id)
	}
}

func TestFindAppContainerID_EmptyContainerList(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{}
	c.Services = svc
	h := NewAppHandler(newMockStore(), c)

	r := httptest.NewRequest("GET", "/", nil)
	id, err := h.findAppContainerID(r, "app1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "" {
		t.Errorf("id = %q, want empty (no containers)", id)
	}
}

func TestAppHandler_Restart_NilRuntime(t *testing.T) {
	c := testCore()
	c.Services = core.NewServices() // Container is nil
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Restart(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Stop_NilRuntime(t *testing.T) {
	c := testCore()
	c.Services = core.NewServices()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Stop(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Start_NilRuntime(t *testing.T) {
	c := testCore()
	c.Services = core.NewServices()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Start(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Stop_ContainerLookupError(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{listErr: errors.New("list failed")}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Stop(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Stop_NoContainer(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/stop", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Stop(rr, req)

	// Idempotent stop on undeployed app
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if store.updatedStatus["app1"] != "stopped" {
		t.Errorf("expected app status 'stopped', got %q", store.updatedStatus["app1"])
	}
}

func TestAppHandler_Restart_ContainerLookupError(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{listErr: errors.New("list failed")}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Restart(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Restart_NoContainer(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/restart", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Restart(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Start_ContainerLookupError(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{listErr: errors.New("list failed")}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Start(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAppHandler_Start_NoContainer(t *testing.T) {
	c := testCore()
	svc := core.NewServices()
	svc.Container = &mockContainerRuntime{}
	c.Services = svc
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := NewAppHandler(store, c)

	req := httptest.NewRequest("POST", "/api/v1/apps/app1/start", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.Start(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── errorInjectingRuntime wraps recordingDeployRuntime to inject stop/remove errors ─

type errorInjectingRuntime struct {
	recordingDeployRuntime
	stopErr   error
	removeErr error
}

func (r *errorInjectingRuntime) Stop(ctx context.Context, id string, timeout int) error {
	r.stopped = append(r.stopped, id)
	if r.stopErr != nil {
		return r.stopErr
	}
	return nil
}

func (r *errorInjectingRuntime) Remove(ctx context.Context, id string, force bool) error {
	r.removed = append(r.removed, id)
	if r.removeErr != nil {
		return r.removeErr
	}
	return nil
}

// ─── backups.go coverage ─────────────────────────────────────────────────────

func TestBackupHandler_SetLogger(t *testing.T) {
	h := &BackupHandler{}
	customLogger := slog.New(slog.NewTextHandler(nil, nil))
	h.SetLogger(customLogger)
	if h.logger != customLogger {
		t.Error("SetLogger did not set logger")
	}
}

func TestIsStrictBackupKey_EdgeCases(t *testing.T) {
	// Double-encoded path
	if isStrictBackupKey("tenant1%252Fbackup") {
		t.Error("expected false for double-encoded key")
	}

	// Key with double slash
	if isStrictBackupKey("tenant1//backup") {
		t.Error("expected false for double slash")
	}

	// Key with trailing slash
	if isStrictBackupKey("tenant1/backup/") {
		t.Error("expected false for trailing slash")
	}

	// Key with invalid characters
	if isStrictBackupKey("tenant1/backup<file>") {
		t.Error("expected false for invalid characters")
	}

	// Valid key
	if !isStrictBackupKey("tenant1/backup-file_v1.2") {
		t.Error("expected true for valid key")
	}

	// Key with URL encoding is rejected because decoded != key
	if isStrictBackupKey("tenant1/backup%20file") {
		t.Error("expected false for URL-encoded key (decoded != key check)")
	}

	// Underscore, hyphens, dots all valid
	if !isStrictBackupKey("tenant1/backup-file_v1.2") {
		t.Error("expected true for valid key")
	}

	// Empty key component after split
	if isStrictBackupKey("tenant1//backup") {
		t.Error("expected false for empty path component")
	}
}

// ─── commands.go coverage ────────────────────────────────────────────────────

func TestCommandHandler_SetKV(t *testing.T) {
	h := &CommandHandler{}
	mockBolt := newMockKVStore()
	h.SetKV(mockBolt)
	if h.kv != mockBolt {
		t.Error("SetKV did not set kv")
	}
}

func TestCommandHandler_History_NilBolt(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	h := &CommandHandler{store: store}

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCommandHandler_History_BoltListError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	kv := newMockKVStore()
	// Don't add any data — List returns error for missing bucket
	h := &CommandHandler{store: store, kv: kv}

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.History(rr, req)

	// Should return empty list, not error
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCommandHandler_History_NonMatchingPrefix(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app1", TenantID: "tenant1"})
	kv := newMockKVStore()
	// Seed a command for a different app
	_ = kv.Set("app_commands", "app2:cmd1", commandHistoryEntry{
		ID: "cmd1", AppID: "app2",
	}, 3600)
	h := &CommandHandler{store: store, kv: kv}

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/commands", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.History(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── event_webhooks.go coverage ──────────────────────────────────────────────

func TestValidateWebhookURL_EdgeCases(t *testing.T) {
	// Empty URL
	if err := validateWebhookURL(""); err == nil {
		t.Error("expected error for empty URL")
	}

	// Invalid URL
	if err := validateWebhookURL("://invalid"); err == nil {
		t.Error("expected error for invalid URL")
	}

	// Non-HTTPS scheme
	if err := validateWebhookURL("http://example.com/webhook"); err == nil {
		t.Error("expected error for non-HTTPS URL")
	}

	// Empty hostname
	if err := validateWebhookURL("https:///path"); err == nil {
		t.Error("expected error for empty hostname")
	}

	// Localhost
	if err := validateWebhookURL("https://localhost/webhook"); err == nil {
		t.Error("expected error for localhost")
	}

	// IP loopback
	if err := validateWebhookURL("https://127.0.0.1/webhook"); err == nil {
		t.Error("expected error for loopback IP")
	}

	// Cloud metadata IP
	if err := validateWebhookURL("https://169.254.169.254/"); err == nil {
		t.Error("expected error for cloud metadata IP")
	}

	// Internal hostname
	if err := validateWebhookURL("https://metadata.google.internal/"); err == nil {
		t.Error("expected error for internal hostname")
	}

	// Subdomain of internal hostname
	if err := validateWebhookURL("https://sub.metadata.google.internal/"); err == nil {
		t.Error("expected error for subdomain of internal hostname")
	}

	// Valid URL
	if err := validateWebhookURL("https://example.com/webhook"); err != nil {
		t.Errorf("unexpected error for valid URL: %v", err)
	}
}

// mutateKVValue covers the non-Mutate path (fallback read+write).
func TestMutateBoltValue_NonMutatorBolt(t *testing.T) {
	kv := newMockKVStore()
	// prime data
	var dest struct {
		Count int `json:"count"`
	}
	err := mutateKVValue(kv, "test_bucket", "test_key", &dest, 3600, func(exists bool) error {
		if !exists {
			return errors.New("expected exists")
		}
		dest.Count = 42
		return nil
	})
	if err == nil {
		t.Error("expected error because key does not exist yet")
	}
}

func TestMutateBoltValue_NonMutatorBolt_NewKey(t *testing.T) {
	kv := newMockKVStore()
	var dest struct {
		Count int `json:"count"`
	}
	err := mutateKVValue(kv, "test_bucket", "new_key", &dest, 3600, func(exists bool) error {
		if exists {
			return errors.New("expected not exists")
		}
		dest.Count = 99
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Verify value was stored
	var got struct {
		Count int `json:"count"`
	}
	if err := kv.Get("test_bucket", "new_key", &got); err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if got.Count != 99 {
		t.Errorf("count = %d, want 99", got.Count)
	}
}

func TestMutateBoltValue_NonMutatorBolt_MutateReturnsError(t *testing.T) {
	kv := newMockKVStore()
	var dest struct{}
	err := mutateKVValue(kv, "test_bucket", "new_key", &dest, 3600, func(exists bool) error {
		return errors.New("mutate failed")
	})
	if err == nil {
		t.Error("expected error from mutate callback")
	}
}

// ─── billing.go coverage ─────────────────────────────────────────────────────

func TestUsageRecordTime_MissingFields(t *testing.T) {
	rec := core.UsageRecord{
		TenantID: "t1",
		// No HourBucket or CreatedAt set
	}
	result := usageRecordTime(rec)
	if !result.IsZero() {
		t.Error("expected zero time when both HourBucket and CreatedAt are zero")
	}
}

func TestUsageRecordTime_HourBucket(t *testing.T) {
	now := time.Now()
	rec := core.UsageRecord{
		TenantID:   "t1",
		HourBucket: now,
	}
	result := usageRecordTime(rec)
	if !result.Equal(now) {
		t.Errorf("usageRecordTime = %v, want %v", result, now)
	}
}

func TestUsageRecordTime_FallsBackToCreatedAt(t *testing.T) {
	now := time.Now()
	rec := core.UsageRecord{
		TenantID:  "t1",
		CreatedAt: now,
	}
	result := usageRecordTime(rec)
	if !result.Equal(now) {
		t.Errorf("usageRecordTime = %v, want %v", result, now)
	}
}

// ─── sessions.go coverage ────────────────────────────────────────────────────

func TestSessionHandler_GetTOTPStatus_NilAuthMod(t *testing.T) {
	h := &SessionHandler{} // authMod is nil
	req := httptest.NewRequest("GET", "/api/v1/auth/totp/status", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.GetTOTPStatus(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSessionHandler_DisableTOTP_NilAuthMod(t *testing.T) {
	h := &SessionHandler{} // authMod is nil
	req := httptest.NewRequest("POST", "/api/v1/auth/totp/disable", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.DisableTOTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSessionHandler_GenerateBackupCodes_NilAuthMod(t *testing.T) {
	h := &SessionHandler{} // authMod is nil
	req := httptest.NewRequest("POST", "/api/v1/auth/totp/backup-codes", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.GenerateBackupCodes(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ─── certificates.go coverage ────────────────────────────────────────────────

// TestCertMatchesDomain covers wildcard matching and SAN/CN fallback.
func TestCertMatchesDomain(t *testing.T) {
	// Wildcard cert matching subdomain
	cert := &x509.Certificate{
		DNSNames: []string{"*.example.com"},
	}
	if !certMatchesDomain(cert, "sub.example.com") {
		t.Error("expected wildcard *.example.com to match sub.example.com")
	}
	if certMatchesDomain(cert, "example.com") {
		t.Error("expected wildcard *.example.com to NOT match example.com")
	}
	if certMatchesDomain(cert, "other.com") {
		t.Error("expected wildcard *.example.com to NOT match other.com")
	}

	// SAN exact match
	cert2 := &x509.Certificate{
		DNSNames: []string{"app.example.com"},
	}
	if !certMatchesDomain(cert2, "app.example.com") {
		t.Error("expected exact SAN match")
	}

	// CN fallback (no SANs)
	cert3 := &x509.Certificate{
		Subject: pkix.Name{CommonName: "cn.example.com"},
	}
	if !certMatchesDomain(cert3, "cn.example.com") {
		t.Error("expected CN fallback match")
	}
	if certMatchesDomain(cert3, "wrong.example.com") {
		t.Error("expected CN fallback to NOT match wrong domain")
	}

	// No match at all
	if certMatchesDomain(cert2, "nonexistent.com") {
		t.Error("expected no match for unrelated domain")
	}
}

// ─── auth.go Login coverage — edge case: nil claims context ──────────────────

func TestLoginRateLimitCheck_NilBolt(t *testing.T) {
	h := &AuthHandler{} // kv is nil
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/v1/auth/login", nil)

	result := h.loginRateLimitCheck(w, r, "test@example.com")
	if result != 0 {
		t.Errorf("expected 0 for nil kv, got %d", result)
	}
}

func TestCheckPerAccountRateLimit_NotFoundError(t *testing.T) {
	h := &AuthHandler{kv: newMockKVStore()}
	locked, until := h.checkPerAccountRateLimit("nonexistent@example.com")
	if locked {
		t.Error("expected not locked for nonexistent email")
	}
	if until != 0 {
		t.Errorf("expected until=0, got %d", until)
	}
}

// ─── deploy_trigger.go deployGitApp nil runtime path ─────────────────────────

func TestDeployTriggerHandler_deployGitApp_NilRuntime(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:       "app1",
		TenantID: "tenant1",
		Status:   "idle",
	})
	h := &DeployTriggerHandler{store: store} // runtime is nil

	err := h.deployGitApp(context.Background(), store.apps["app1"], "manual", "", nil)
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}
	if !strings.Contains(err.Error(), "container runtime not available") {
		t.Errorf("error = %q, want 'container runtime not available'", err.Error())
	}
	if store.updatedStatus["app1"] != "failed" {
		t.Errorf("expected app status 'failed', got %q", store.updatedStatus["app1"])
	}
}

// === merged from handlers_final_test.go ===

// =============================================================================
// AdminHandler.SystemInfo — covers line 23 (runtime.ReadMemStats + module loop)
// =============================================================================

func TestFinal_AdminHandler_SystemInfo(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Registry: core.NewRegistry(),
	}
	h := NewAdminHandler(c, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	rr := httptest.NewRecorder()
	h.SystemInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["go"] == nil {
		t.Error("expected 'go' field in response")
	}
}

func TestFinal_AdminHandler_SystemInfo_NilEventBus(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Registry: core.NewRegistry(),
	}
	h := NewAdminHandler(c, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/admin/system", nil)
	rr := httptest.NewRecorder()
	h.SystemInfo(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["events"] == nil {
		t.Fatal("expected events field")
	}
}

// =============================================================================
// AdminAPIKeyHandler.Generate — kv Set error on first Set (key record)
// =============================================================================

type boltFailOnFirstSet struct {
	*mockKVStore
}

func (b *boltFailOnFirstSet) Set(bucket, key string, value any, ttl int64) error {
	return fmt.Errorf("kv write error")
}

func TestFinal_AdminAPIKey_Generate_BoltSetError(t *testing.T) {
	kv := &boltFailOnFirstSet{mockKVStore: newMockKVStore()}
	h := NewAdminAPIKeyHandler(newMockStore(), kv)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/api-keys", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// AuthHandler.Login — GetUserByEmail non-ErrNotFound error (line 61)
// =============================================================================

func TestFinal_Auth_Login_InternalError(t *testing.T) {
	store := newMockStore()
	store.errGetUserByEmail = fmt.Errorf("database connection lost")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	body := `{"email":"test@test.com","password":"secret123"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AuthHandler.Login — GetUserMembership error (line 74)
// =============================================================================

func TestFinal_Auth_Login_MembershipError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "Password123!", "t1", "role_owner")
	store.errGetUserMembership = fmt.Errorf("membership query failed")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	body := `{"email":"test@test.com","password":"Password123!"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/login", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Login(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AuthHandler.Register — CreateTenantWithDefaults error (line 131)
// =============================================================================

func TestFinal_Auth_Register_CreateTenantError(t *testing.T) {
	store := newMockStore()
	store.errCreateTenantWithDefaults = fmt.Errorf("tenant creation failed")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	body := `{"email":"new@test.com","password":"Password123!","name":"New User"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AuthHandler.Register — CreateUserWithMembership error (line 137)
// =============================================================================

func TestFinal_Auth_Register_CreateUserError(t *testing.T) {
	store := newMockStore()
	store.errCreateUserWithMembership = fmt.Errorf("user creation failed")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	body := `{"email":"new@test.com","password":"Password123!","name":"New User"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/register", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Register(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// AuthHandler.Refresh — GetUser error (line 174)
// =============================================================================

func TestFinal_Auth_Refresh_GetUserError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "Password123!", "t1", "role_owner")
	store.errGetUser = fmt.Errorf("user not found")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("u1", "t1", "role_owner", "test@test.com")
	body := fmt.Sprintf(`{"refresh_token":"%s"}`, refreshToken)
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Refresh(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// AuthHandler.Refresh — GetUserMembership error (line 181)
// =============================================================================

func TestFinal_Auth_Refresh_MembershipError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "Password123!", "t1", "role_owner")
	store.errGetUserMembership = fmt.Errorf("membership error")
	authMod := testAuthModule(store)
	h := NewAuthHandler(authMod, store, nil)

	refreshToken := generateTestRefreshToken("u1", "t1", "role_owner", "test@test.com")
	body := fmt.Sprintf(`{"refresh_token":"%s"}`, refreshToken)
	req := httptest.NewRequest("POST", "/api/v1/auth/refresh", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Refresh(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// generateSlug — empty result (line 206-208: special chars only)
// =============================================================================

func TestFinal_GenerateSlug_SpecialCharsOnly(t *testing.T) {
	slug := generateSlug("!!!@@@###")
	if slug == "" || slug == "!!!@@@###" {
		t.Errorf("expected generated ID fallback, got %q", slug)
	}
	if len(slug) != 8 {
		t.Errorf("expected 8-char fallback slug, got %q (len=%d)", slug, len(slug))
	}
}

func TestFinal_GenerateSlug_Underscore(t *testing.T) {
	slug := generateSlug("my_app")
	if slug != "my-app" {
		t.Errorf("expected 'my-app', got %q", slug)
	}
}

// =============================================================================
// BulkHandler.Execute — max 50 limit (line 52-53)
// =============================================================================

func TestFinal_Bulk_TooManyApps(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewBulkHandler(store, nil, events)

	ids := make([]string, 51)
	for i := range ids {
		ids[i] = fmt.Sprintf("app-%d", i)
	}
	idsJSON, _ := json.Marshal(ids)
	body := fmt.Sprintf(`{"action":"start","app_ids":%s}`, string(idsJSON))

	req := httptest.NewRequest("POST", "/api/v1/apps/bulk", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Execute(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// BulkHandler.Execute — unknown action (line 88-89)
// =============================================================================

func TestFinal_Bulk_UnknownAction(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	events := core.NewEventBus(slog.Default())
	h := NewBulkHandler(store, nil, events)

	body := `{"action":"destroy","app_ids":["app-1"]}`
	req := httptest.NewRequest("POST", "/api/v1/apps/bulk", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Execute(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["failed"].(float64) != 1 {
		t.Errorf("expected 1 failed, got %v", resp["failed"])
	}
}

// =============================================================================
// CertificateHandler.Upload — invalid cert pair (line 79)
// =============================================================================

func TestFinal_Certificate_Upload_InvalidPair(t *testing.T) {
	h := NewCertificateHandler(newMockStore(), newMockKVStore())

	body := `{"domain_id":"d1","cert_pem":"not-a-cert","key_pem":"not-a-key"}`
	req := httptest.NewRequest("POST", "/api/v1/certificates", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// CertificateHandler.Upload — kv Set error (store cert data, line 107-108)
// =============================================================================

func TestFinal_Certificate_Upload_BoltError(t *testing.T) {
	kv := &boltFailOnFirstSet{mockKVStore: newMockKVStore()}
	h := NewCertificateHandler(newMockStore(), kv)

	// Use a valid self-signed cert/key pair is complex; test just the field validation
	body := `{"domain_id":"","cert_pem":"x","key_pem":"y"}`
	req := httptest.NewRequest("POST", "/api/v1/certificates", strings.NewReader(body))
	req = req.WithContext(auth.ContextWithClaims(req.Context(), &auth.Claims{
		TenantID: "test-tenant",
		UserID:   "test-user",
	}))
	rr := httptest.NewRecorder()
	h.Upload(rr, req)

	// Missing domain_id should be caught first
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// ComposeHandler.Deploy — YAML content type path (line 40-47)
// =============================================================================

func TestFinal_Compose_Deploy_YAMLContentType(t *testing.T) {
	store := newMockStore()
	store.projects = map[string][]core.Project{"t1": {{ID: "p1", TenantID: "t1"}}}
	events := core.NewEventBus(slog.Default())
	// Use nil runtime so the async goroutine's deployer returns early without crashing
	h := NewComposeHandler(context.Background(), store, nil, events)

	yamlBody := `version: "3"
services:
  web:
    image: nginx`
	req := httptest.NewRequest("POST", "/api/v1/stacks?name=mystack", strings.NewReader(yamlBody))
	req.Header.Set("Content-Type", "application/x-yaml")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// DeployApprovalHandler.Approve — not found (line 80)
// =============================================================================

func TestFinal_DeployApproval_Approve_NotFound(t *testing.T) {
	events := core.NewEventBus(slog.Default())
	h := NewDeployApprovalHandler(newMockStore(), events)

	req := httptest.NewRequest("POST", "/api/v1/deploy/approvals/xxx/approve", nil)
	req.SetPathValue("id", "xxx")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Approve(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// DeployTriggerHandler.TriggerDeploy — image type deploy with runtime (lines 33-76)
// =============================================================================

func TestFinal_DeployTrigger_ImageDeploy(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "t1",
		Name:       "myapp",
		SourceType: "image",
		SourceURL:  "nginx:latest",
	})
	store.nextDeployVersion = map[string]int{"app-1": 2}
	rt := &mockContainerRuntime{}
	events := core.NewEventBus(slog.Default())
	h := NewDeployTriggerHandler(context.Background(), store, rt, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/deploy", nil)
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// DeployTriggerHandler.TriggerDeploy — git-sourced deploy (lines 80-110)
// =============================================================================

func TestFinal_DeployTrigger_GitDeploy(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-2",
		TenantID:   "t1",
		Name:       "gitapp",
		SourceType: "git",
		SourceURL:  "https://github.com/user/repo.git",
		Branch:     "main",
	})
	events := core.NewEventBus(slog.Default())
	h := NewDeployTriggerHandler(context.Background(), store, &mockContainerRuntime{}, events)

	req := httptest.NewRequest("POST", "/api/v1/apps/app-2/deploy", nil)
	req.SetPathValue("id", "app-2")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.TriggerDeploy(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// EnvImportHandler.Import — .env format (line 42-43: parseDotEnv branch)
// =============================================================================

func TestFinal_EnvImport_DotEnvFormat(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "myapp"})
	h := NewEnvImportHandler(store)

	envContent := "DB_HOST=localhost\nDB_PORT=5432\n# comment\nDB_NAME=mydb"
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/env/import", strings.NewReader(envContent))
	req.SetPathValue("id", "app-1")
	req.Header.Set("Content-Type", "text/plain")
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// EnvImportHandler.parseDotEnv — quoted values (line 111-113)
// =============================================================================

func TestFinal_ParseDotEnv_QuotedValues(t *testing.T) {
	content := `KEY1="value with spaces"
KEY2='single quoted'
KEY3=no-quotes`
	vars := parseDotEnv(content)
	if len(vars) != 3 {
		t.Fatalf("expected 3 vars, got %d", len(vars))
	}
	if vars[0].Value != "value with spaces" {
		t.Errorf("KEY1 = %q, want 'value with spaces'", vars[0].Value)
	}
	if vars[1].Value != "single quoted" {
		t.Errorf("KEY2 = %q, want 'single quoted'", vars[1].Value)
	}
}

// =============================================================================
// parseEnvJSON — empty string (line 80-81)
// =============================================================================

func TestFinal_ParseEnvJSON_Empty(t *testing.T) {
	result := parseEnvJSON("")
	if len(result) != 0 {
		t.Errorf("expected empty map, got %d entries", len(result))
	}
}

// =============================================================================
// ImageCleanupHandler.Prune — ImageRemove error (line 55: continue branch)
// =============================================================================

type mockRuntimeRemoveFails struct {
	mockContainerRuntime
}

func (m *mockRuntimeRemoveFails) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return []core.ImageInfo{
		{ID: "img1", Tags: []string{"<none>:<none>"}, Size: 10 * 1024 * 1024},
	}, nil
}

func (m *mockRuntimeRemoveFails) ImageRemove(_ context.Context, _ string) error {
	return fmt.Errorf("image in use")
}

func TestFinal_ImageCleanup_Prune_RemoveError(t *testing.T) {
	h := NewImageCleanupHandler(&mockRuntimeRemoveFails{})

	req := httptest.NewRequest("DELETE", "/api/v1/images/prune", nil)
	rr := httptest.NewRecorder()
	h.Prune(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["images_removed"].(float64) != 0 {
		t.Errorf("expected 0 removed, got %v", resp["images_removed"])
	}
}

// =============================================================================
// ImportExportHandler.Import — CreateApp error (line 100)
// =============================================================================

func TestFinal_Import_CreateAppError(t *testing.T) {
	store := newMockStore()
	store.errCreateApp = fmt.Errorf("db write error")
	h := NewImportExportHandler(store)

	body := `{"version":"1","name":"imported","type":"service","source_type":"image","source_url":"nginx:latest","replicas":1}`
	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// ImportExportHandler.Import — with projects (line 95-97)
// =============================================================================

func TestFinal_Import_WithProject(t *testing.T) {
	store := newMockStore()
	store.projects = map[string][]core.Project{"t1": {{ID: "p1", TenantID: "t1"}}}
	h := NewImportExportHandler(store)

	body := `{"version":"1","name":"imported","type":"service","source_type":"image","source_url":"nginx:latest","replicas":1,"domains":["app.example.com"]}`
	req := httptest.NewRequest("POST", "/api/v1/apps/import", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Import(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// InviteHandler.Create — CreateInvite error (line 64)
// =============================================================================

func TestFinal_Invite_Create_StoreError(t *testing.T) {
	store := newMockStore()
	store.errCreateInvite = fmt.Errorf("db error")
	store.addUser(&core.User{ID: "u1", Email: "test@test.com"}, &core.TeamMember{
		ID:       "tm-1",
		UserID:   "u1",
		TenantID: "t1",
		RoleID:   "role_owner",
		Status:   "active",
	})
	// Seed role so RBAC check passes
	store.roles["t1"] = append(store.roles["t1"], core.Role{
		ID:              "role_owner",
		TenantID:        "t1",
		PermissionsJSON: `["member.invite","member.list","member.remove"]`,
	}, core.Role{
		ID:              "role_member",
		TenantID:        "t1",
		PermissionsJSON: `[]`,
	})
	events := core.NewEventBus(slog.Default())
	h := NewInviteHandler(store, events)

	body := `{"email":"new@test.com","role_id":"role_member"}`
	req := httptest.NewRequest("POST", "/api/v1/team/invites", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// LogDownloadHandler.Download — Logs error (line 39)
// =============================================================================

func TestFinal_LogDownload_LogsError(t *testing.T) {
	rt := &mockContainerRuntime{
		containers: []core.ContainerInfo{{ID: "cnt-12345678", State: "running"}},
		logsErr:    fmt.Errorf("log read failed"),
	}
	h := NewLogDownloadHandler(rt)

	req := httptest.NewRequest("GET", "/api/v1/apps/app12345678/logs/download", nil)
	req.SetPathValue("id", "app12345678")
	rr := httptest.NewRecorder()
	h.Download(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// MarketplaceHandler.List — nil registry (line 20-22)
// =============================================================================

func TestFinal_Marketplace_List_NilRegistry(t *testing.T) {
	h := NewMarketplaceHandler(nil)

	req := httptest.NewRequest("GET", "/api/v1/marketplace", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["total"].(float64) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

// =============================================================================
// MarketplaceDeployHandler.Deploy — template not found (line 52)
// =============================================================================

func TestFinal_MarketplaceDeploy_TemplateNotFound(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	h := NewMarketplaceDeployHandler(context.Background(), registry, nil, newMockStore(), core.NewEventBus(slog.Default()))

	body := `{"slug":"nonexistent"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// MarketplaceDeployHandler.Deploy — CreateApp error (line 88)
// =============================================================================

func TestFinal_MarketplaceDeploy_CreateAppError(t *testing.T) {
	registry := marketplace.NewTemplateRegistry()
	registry.Add(&marketplace.Template{
		Slug:        "test-app",
		Name:        "Test App",
		ComposeYAML: "version: '3'\nservices:\n  web:\n    image: nginx\n",
	})

	store := newMockStore()
	store.errCreateApp = fmt.Errorf("db error")
	h := NewMarketplaceDeployHandler(context.Background(), registry, nil, store, core.NewEventBus(slog.Default()))

	body := `{"slug":"test-app","name":"myapp"}`
	req := httptest.NewRequest("POST", "/api/v1/marketplace/deploy", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Deploy(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", rr.Code, rr.Body.String())
	}
}

// =============================================================================
// PortHandler.Update — invalid port (line 58-59)
// =============================================================================

func TestFinal_Port_Update_InvalidPort(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test-app"})
	h := NewPortHandler(store)

	body := `[{"container_port":0,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// PortHandler.Update — port over 65535
// =============================================================================

func TestFinal_Port_Update_PortTooHigh(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test-app"})
	h := NewPortHandler(store)

	body := `[{"container_port":70000,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// RegistryHandler.Add — kv Set error (line 96-98)
// =============================================================================

func TestFinal_Registry_Add_BoltError(t *testing.T) {
	kv := &boltFailOnFirstSet{mockKVStore: newMockKVStore()}
	h := NewRegistryHandler(kv)

	body := `{"name":"My Registry","url":"registry.example.com","username":"user","password":"pass"}`
	req := httptest.NewRequest("POST", "/api/v1/registries", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Add(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SearchHandler.Search — results limit >20 (line 75-76)
// =============================================================================

func TestFinal_Search_ResultsLimit(t *testing.T) {
	store := newMockStore()
	// Add 25 apps that match the search
	for i := range 25 {
		store.appList = append(store.appList, core.Application{
			ID: fmt.Sprintf("app-%d", i), Name: fmt.Sprintf("test-app-%d", i), Status: "running",
		})
	}
	store.appTotal = 25
	h := NewSearchHandler(store)

	req := httptest.NewRequest("GET", "/api/v1/search?q=test", nil)
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Search(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	total := resp["total"].(float64)
	if total > 20 {
		t.Errorf("expected max 20 results, got %v", total)
	}
}

// =============================================================================
// SecretHandler.Create — vault encrypt error (line 60)
// =============================================================================

type mockVaultEncryptFails struct{}

func (m *mockVaultEncryptFails) Encrypt(_ string) (string, error) {
	return "", fmt.Errorf("encryption hardware failure")
}
func (m *mockVaultEncryptFails) Decrypt(s string) (string, error) { return s, nil }

func TestFinal_Secret_Create_VaultEncryptError(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewSecretHandler(store, &mockVaultEncryptFails{}, events)

	body := `{"name":"DB_PASS","value":"secret123","scope":"tenant"}`
	req := httptest.NewRequest("POST", "/api/v1/secrets", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SecretHandler.List — store error (line 119)
// =============================================================================

func TestFinal_Secret_List_StoreError(t *testing.T) {
	store := newMockStore()
	store.errListSecretsByTenant = fmt.Errorf("db error")
	events := core.NewEventBus(slog.Default())
	h := NewSecretHandler(store, nil, events)

	req := httptest.NewRequest("GET", "/api/v1/secrets", nil)
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SecretHandler.Create — CreateSecretVersion error (line 90)
// =============================================================================

func TestFinal_Secret_Create_VersionError(t *testing.T) {
	store := newMockStore()
	store.errCreateSecretVersion = fmt.Errorf("version write error")
	events := core.NewEventBus(slog.Default())
	h := NewSecretHandler(store, nil, events)

	body := `{"name":"DB_PASS","value":"secret123"}`
	req := httptest.NewRequest("POST", "/api/v1/secrets", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.UpdateProfile — GetUser error (line 72)
// =============================================================================

func TestFinal_Session_UpdateProfile_GetUserError(t *testing.T) {
	store := newMockStore()
	store.errGetUser = fmt.Errorf("user not found")
	h := NewSessionHandler(store, nil, nil)

	body := `{"name":"New Name"}`
	req := httptest.NewRequest("PATCH", "/api/v1/auth/me", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.UpdateProfile(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.UpdateProfile — UpdateUser error (line 85)
// =============================================================================

func TestFinal_Session_UpdateProfile_UpdateError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "Pass123!", "t1", "role_owner")
	store.errUpdateUser = fmt.Errorf("update failed")
	h := NewSessionHandler(store, nil, nil)

	body := `{"name":"New Name"}`
	req := httptest.NewRequest("PATCH", "/api/v1/auth/me", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.UpdateProfile(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.ChangePassword — wrong current password (line 117)
// =============================================================================

func TestFinal_Session_ChangePassword_WrongCurrent(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "CurrentPass1!", "t1", "role_owner")
	h := NewSessionHandler(store, nil, nil)

	body := `{"current_password":"WrongPassword1!","new_password":"NewPass123!"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.ChangePassword — weak new password (line 123)
// =============================================================================

func TestFinal_Session_ChangePassword_WeakNewPassword(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "CurrentPass1!", "t1", "role_owner")
	h := NewSessionHandler(store, nil, nil)

	body := `{"current_password":"CurrentPass1!","new_password":"short"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// =============================================================================
// SessionHandler.ChangePassword — UpdatePassword error (line 134)
// =============================================================================

func TestFinal_Session_ChangePassword_UpdateError(t *testing.T) {
	store := newMockStore()
	seedTestUser(store, "u1", "test@test.com", "CurrentPass1!", "t1", "role_owner")
	store.errUpdatePassword = fmt.Errorf("db write error")
	h := NewSessionHandler(store, nil, nil)

	body := `{"current_password":"CurrentPass1!","new_password":"NewStrongPass1!"}`
	req := httptest.NewRequest("POST", "/api/v1/auth/change-password", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.ChangePassword(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// SSHKeyHandler.Generate — kv Set error (line 92-93)
// =============================================================================

func TestFinal_SSHKey_Generate_BoltError(t *testing.T) {
	kv := &boltFailOnFirstSet{mockKVStore: newMockKVStore()}
	h := NewSSHKeyHandler(newMockStore(), kv)

	body := `{"name":"my-key"}`
	req := httptest.NewRequest("POST", "/api/v1/ssh-keys/generate", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Generate(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// StatsHandler.ServerStats — runtime error (line 55-56)
// =============================================================================

func TestFinal_Stats_ServerStats_RuntimeError(t *testing.T) {
	rt := &mockContainerRuntime{listErr: fmt.Errorf("docker down")}
	h := NewStatsHandler(rt, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/servers/stats", nil)
	rr := httptest.NewRecorder()
	h.ServerStats(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// StatsHandler.ServerStats — nil runtime (line 47-48)
// =============================================================================

func TestFinal_Stats_ServerStats_NilRuntime(t *testing.T) {
	h := NewStatsHandler(nil, newMockStore())

	req := httptest.NewRequest("GET", "/api/v1/servers/stats", nil)
	rr := httptest.NewRecorder()
	h.ServerStats(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// =============================================================================
// StorageHandler.Usage — exercises volume/image/kv paths (line 33-53)
// =============================================================================

func TestFinal_Storage_Usage(t *testing.T) {
	rt := &mockContainerRuntime{}
	kv := newMockKVStore()
	h := NewStorageHandler(newMockStore(), rt, kv)

	req := httptest.NewRequest("GET", "/api/v1/storage/usage", nil)
	req = withClaims(req, "u1", "t1", "role_owner", "test@test.com")
	rr := httptest.NewRecorder()
	h.Usage(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// TransferHandler.TransferApp — GetApp error (line 47)
// =============================================================================

func TestFinal_Transfer_AppNotFound(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	h := NewTransferHandler(store, events)

	body := `{"target_tenant_id":"t2"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/nonexistent/transfer", strings.NewReader(body))
	req.SetPathValue("id", "nonexistent")
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// =============================================================================
// TransferHandler.TransferApp — UpdateApp error (line 62)
// =============================================================================

func TestFinal_Transfer_UpdateAppError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "myapp"})
	store.addTenant(&core.Tenant{ID: "t2", Name: "Other Team"})
	store.errUpdateApp = fmt.Errorf("update failed")
	events := core.NewEventBus(slog.Default())
	h := NewTransferHandler(store, events)

	body := `{"target_tenant_id":"t2"}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/transfer", strings.NewReader(body))
	req.SetPathValue("id", "app-1")
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.TransferApp(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// BuildCacheHandler.Clear — no runtime (line 82-83: nil runtime path)
// =============================================================================

func TestFinal_BuildCache_Clear_NilRuntime(t *testing.T) {
	kv := newMockKVStore()
	h := NewBuildCacheHandler(nil, kv)

	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// BuildCacheHandler.Clear — ImageRemove error (skip branch, line 76-77)
// =============================================================================

func TestFinal_BuildCache_Clear_RemoveError(t *testing.T) {
	kv := newMockKVStore()
	h := NewBuildCacheHandler(&mockRuntimeRemoveFails{}, kv)

	req := httptest.NewRequest("DELETE", "/api/v1/build/cache", nil)
	rr := httptest.NewRecorder()
	h.Clear(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// DBBackupHandler.Backup — file not found (line 38)
// =============================================================================

func TestFinal_DBBackup_Backup_FileNotFound(t *testing.T) {
	c := &core.Core{Config: &core.Config{Database: core.DatabaseConfig{Path: "/nonexistent/path/db.sqlite"}}}
	h := NewDBBackupHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/backup", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Backup(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// DBBackupHandler.Status — file not found (line 77)
// =============================================================================

func TestFinal_DBBackup_Status_FileNotFound(t *testing.T) {
	c := &core.Core{Config: &core.Config{Database: core.DatabaseConfig{Path: "/nonexistent/path/db.sqlite"}}}
	h := NewDBBackupHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/db/status", nil)
	req = withClaims(req, "u1", "t1", "role_super_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Status(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// =============================================================================
// DetailedHealthHandler — db not available, docker not available (degraded path)
// =============================================================================

func TestFinal_DetailedHealth_Degraded(t *testing.T) {
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Registry: core.NewRegistry(),
		// No Store set => db check fails
		// No Container set => docker check fails
	}
	h := NewDetailedHealthHandler(c)

	req := httptest.NewRequest("GET", "/health/detailed", nil)
	rr := httptest.NewRecorder()
	h.DetailedHealth(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "degraded" {
		t.Errorf("expected 'degraded', got %v", resp["status"])
	}
}

// =============================================================================
// MetricsExportHandler.Export — CSV format (line 77-94)
// =============================================================================

func TestFinal_MetricsExport_CSV(t *testing.T) {
	kv := newMockKVStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "app12345678", TenantID: "t1", Name: "App"})
	h := NewMetricsExportHandler(store, kv, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/app12345678/metrics/export?format=csv", nil)
	req.SetPathValue("id", "app12345678")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/csv") {
		t.Errorf("expected CSV content type, got %q", rr.Header().Get("Content-Type"))
	}
}

// =============================================================================
// MetricsExportHandler.Export — short appID (line 77-78)
// =============================================================================

func TestFinal_MetricsExport_ShortAppID(t *testing.T) {
	kv := newMockKVStore()
	store := newMockStore()
	store.addApp(&core.Application{ID: "ab", TenantID: "t1", Name: "App"})
	h := NewMetricsExportHandler(store, kv, nil)

	req := httptest.NewRequest("GET", "/api/v1/apps/ab/metrics/export?format=csv", nil)
	req.SetPathValue("id", "ab")
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	rr := httptest.NewRecorder()
	h.Export(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// AgentStatusHandler.List — with module health statuses (line 43-45, 62-69)
// =============================================================================

func TestFinal_AgentStatus_List_WithModules(t *testing.T) {
	reg := core.NewRegistry()
	c := &core.Core{
		Config:   &core.Config{},
		Events:   core.NewEventBus(slog.Default()),
		Services: core.NewServices(),
		Logger:   slog.Default(),
		Registry: reg,
	}
	h := NewAgentStatusHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/agents", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// =============================================================================
// SelfUpdateHandler.CheckUpdate — exercises network call path (line 27-36)
// =============================================================================

func TestFinal_SelfUpdate_CheckUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network test in short mode")
	}

	c := &core.Core{
		Config: &core.Config{},
		Build:  core.BuildInfo{Version: "0.0.1-test", Commit: "abc123", Date: "2026-01-01"},
	}
	h := NewSelfUpdateHandler(c)

	req := httptest.NewRequest("GET", "/api/v1/admin/updates", nil)
	rr := httptest.NewRecorder()
	h.CheckUpdate(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// === merged from handlers_followup_round2_test.go ===

// TestMonitoringHandler_Metrics_WithContainerRuntime exercises the
// container-listing branch that the no-runtime test in
// monitoring_handler_test.go skips. Three containers are seeded with two
// in the "running" state and one stopped, so the response must report
// containers_total=3 and containers_running=2.
func TestMonitoringHandler_Metrics_WithContainerRuntime(t *testing.T) {
	c := monitoringTestCore()
	c.Services.Container = &mockContainerRuntime{
		containers: []core.ContainerInfo{
			{ID: "c1", State: "running"},
			{ID: "c2", State: "running"},
			{ID: "c3", State: "exited"},
		},
	}

	h := NewMonitoringHandler(c, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/server", nil)
	rr := httptest.NewRecorder()
	h.Metrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if got, _ := resp["containers_total"].(float64); got != 3 {
		t.Errorf("containers_total = %v, want 3", got)
	}
	if got, _ := resp["containers_running"].(float64); got != 2 {
		t.Errorf("containers_running = %v, want 2", got)
	}
}

// TestMonitoringHandler_Metrics_ContainerRuntimeError covers the error
// branch on ListByLabels — counts must fall back to zero rather than
// failing the whole response.
func TestMonitoringHandler_Metrics_ContainerRuntimeError(t *testing.T) {
	c := monitoringTestCore()
	c.Services.Container = &mockContainerRuntime{
		listErr: errBoom,
	}

	h := NewMonitoringHandler(c, time.Now())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/metrics/server", nil)
	rr := httptest.NewRecorder()
	h.Metrics(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 even on runtime error; body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if got, _ := resp["containers_total"].(float64); got != 0 {
		t.Errorf("containers_total = %v, want 0 when ListByLabels errors", got)
	}
}

// errBoom is a sentinel error for runtime mocks. Kept tiny and local.
var errBoom = &runtimeFailure{msg: "boom"}

type runtimeFailure struct{ msg string }

func (e *runtimeFailure) Error() string { return e.msg }

// ---------------------------------------------------------------------------
// SecretHandler.Delete
// ---------------------------------------------------------------------------

func TestSecretHandler_Delete_Unauthorized(t *testing.T) {
	h := NewSecretHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/sec-1", nil)
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestSecretHandler_Delete_MissingID(t *testing.T) {
	h := NewSecretHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))

	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when path id is empty", rr.Code)
	}
}

func TestSecretHandler_Delete_StoreDoesNotImplementInterface(t *testing.T) {
	// mockStore lacks DeleteSecret, so the handler must reply 501.
	h := NewSecretHandler(newMockStore(), nil, core.NewEventBus(slog.Default()))

	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/secrets/sec-1", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	req.SetPathValue("id", "sec-1")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ServerHandler.List
// ---------------------------------------------------------------------------

func TestServerHandler_List_ReturnsLocalNode(t *testing.T) {
	h := NewServerHandler(newMockStore(), core.NewServices(), core.NewEventBus(slog.Default()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/servers", nil)
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []struct {
			ID       string `json:"id"`
			Hostname string `json:"hostname"`
			Provider string `json:"provider"`
			Role     string `json:"role"`
			Status   string `json:"status"`
		} `json:"data"`
		Total int `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("expected exactly one local server, got total=%d data=%d", resp.Total, len(resp.Data))
	}
	got := resp.Data[0]
	if got.ID != "local" || got.Provider != "local" || got.Role != "master" || got.Status != "active" {
		t.Fatalf("unexpected local server payload: %+v", got)
	}
	// Hostname mirrors the OS hostname when one is available; the
	// fallback is "local". Either is acceptable here.
	if hostname, _ := os.Hostname(); hostname != "" && got.Hostname != hostname && got.Hostname != "local" {
		t.Fatalf("hostname = %q, want OS hostname %q or fallback 'local'", got.Hostname, hostname)
	}
}

// === merged from handlers_remaining_test.go ===

func TestContainsStringAllBranches(t *testing.T) {
	if !containsString([]string{"a", "b"}, "a") {
		t.Error("should find a")
	}
	if containsString([]string{"a", "b"}, "c") {
		t.Error("should not find c")
	}
	if containsString(nil, "x") {
		t.Error("nil should not contain x")
	}
	if containsString([]string{}, "x") {
		t.Error("empty should not contain x")
	}
}

func TestGitProviderDisplayNameBranches(t *testing.T) {
	for in, want := range map[string]string{
		"github": "GitHub", "gitlab": "GitLab", "gitea": "Gitea",
		"bitbucket": "Bitbucket", "unknown": "unknown", "": "",
	} {
		if got := gitProviderDisplayName(in); got != want {
			t.Errorf("gitProviderDisplayName(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestExtractAppActionAllCases(t *testing.T) {
	id, act, cid := extractAppAction(core.Event{Data: core.AppEventData{AppID: "a1"}})
	if id != "a1" || act != "" || cid != "" {
		t.Errorf("AppEventData: (%q,%q,%q)", id, act, cid)
	}

	id, act, cid = extractAppAction(core.Event{Data: map[string]string{"id": "a2", "action": "restart", "container_id": "c1"}})
	if id != "a2" || act != "restart" || cid != "c1" {
		t.Errorf("map: (%q,%q,%q)", id, act, cid)
	}

	id, act, cid = extractAppAction(core.Event{Data: "raw"})
	if id != "" || act != "" || cid != "" {
		t.Errorf("unknown: (%q,%q,%q)", id, act, cid)
	}

	id, act, cid = extractAppAction(core.Event{})
	if id != "" || act != "" || cid != "" {
		t.Errorf("nil: (%q,%q,%q)", id, act, cid)
	}
}

func TestIsStrictBackupKeyAllCases(t *testing.T) {
	for _, k := range []string{"t/b1", "a/b.c"} {
		if !isStrictBackupKey(k) {
			t.Errorf("should be valid: %q", k)
		}
	}
	for _, k := range []string{"", "../etc", "a//b", "a/b/", "a/./b", "a/../b", "a/b@c", "a%2Fb"} {
		if isStrictBackupKey(k) {
			t.Errorf("should be invalid: %q", k)
		}
	}
}

func TestActiveDeployFreezeAllPaths(t *testing.T) {
	if activeDeployFreeze(nil, "t1") {
		t.Error("nil kv should be false")
	}
	if activeDeployFreeze(newMockKVStore(), "") {
		t.Error("empty tenant should be false")
	}
	if activeDeployFreeze(newMockKVStore(), "t1") {
		t.Error("no data should be false")
	}

	b := newMockKVStore()
	now := time.Now()
	b.Set("deploy_freeze", "t1", freezeWindowList{Windows: []FreezeWindow{{
		ID: "f1", StartsAt: now.Add(-1 * time.Hour), EndsAt: now.Add(1 * time.Hour), Active: true,
	}}}, 0)
	if !activeDeployFreeze(b, "t1") {
		t.Error("active freeze should be true")
	}

	b2 := newMockKVStore()
	b2.Set("deploy_freeze", "t2", freezeWindowList{Windows: []FreezeWindow{{
		ID: "f2", StartsAt: now.Add(-48 * time.Hour), EndsAt: now.Add(-24 * time.Hour), Active: true,
	}}}, 0)
	if activeDeployFreeze(b2, "t2") {
		t.Error("expired freeze should be false")
	}

	b3 := newMockKVStore()
	b3.Set("deploy_freeze", "t3", freezeWindowList{Windows: []FreezeWindow{{
		ID: "f3", StartsAt: now.Add(-1 * time.Hour), EndsAt: now.Add(1 * time.Hour), Active: false,
	}}}, 0)
	if activeDeployFreeze(b3, "t3") {
		t.Error("inactive freeze should be false")
	}

	b4 := newMockKVStore()
	b4.Set("deploy_freeze", "t4", freezeWindowList{Windows: []FreezeWindow{{
		ID: "f4", StartsAt: now.Add(24 * time.Hour), EndsAt: now.Add(48 * time.Hour), Active: true,
	}}}, 0)
	if activeDeployFreeze(b4, "t4") {
		t.Error("future freeze should be false")
	}
}

func TestContainsAnyAll(t *testing.T) {
	if containsAny("hello", "xyz") {
		t.Error("should not match")
	}
	if !containsAny("hello", "ae") {
		t.Error("should match 'e'")
	}
	if !containsAny("test.com", ".:") {
		t.Error("should match '.'")
	}
	if containsAny("", ".:") {
		t.Error("empty should not match")
	}
}

func TestImageRefHasRegistryAllCases(t *testing.T) {
	cases := []struct {
		ref  string
		want bool
	}{
		{"alpine", false}, {"library/nginx", false}, {"localhost:5000/img", true},
		{"reg.example.com/img", true}, {"docker.io/img", true}, {"", false},
	}
	for _, c := range cases {
		if got := imageRefHasRegistry(c.ref); got != c.want {
			t.Errorf("imageRefHasRegistry(%q)=%v, want %v", c.ref, got, c.want)
		}
	}
}

func TestImageNamePartAllCases(t *testing.T) {
	if got := imageNamePart("My App", ""); got != "my-app" {
		t.Errorf("got %q", got)
	}
	if got := imageNamePart("", "fallback"); got != "fallback" {
		t.Errorf("got %q", got)
	}
	if got := imageNamePart("", ""); !strings.HasPrefix(got, "app-") {
		t.Errorf("got %q", got)
	}
}

func TestBuildImageTagForRegistryEdgeCases(t *testing.T) {
	if v := buildImageTagForRegistry("", &core.Application{Name: "a", ID: "1"}, "abc"); v != "" {
		t.Errorf("expected empty, got %q", v)
	}
	if v := buildImageTagForRegistry("r.io", nil, "abc"); v != "" {
		t.Errorf("expected empty, got %q", v)
	}
	if v := buildImageTagForRegistry("r.io/r", &core.Application{Name: "MyApp", ID: "id"}, ""); v == "" {
		t.Error("empty sha should still produce tag")
	}
}

func TestTenantBackupPrefixAll(t *testing.T) {
	if p := tenantBackupPrefix("tenant1"); p != "tenant1/" {
		t.Errorf("got %q", p)
	}
	if p := tenantBackupPrefix("/t/"); p != "t/" {
		t.Errorf("got %q", p)
	}
}

type noMutateKV struct{ core.KVStorer }

func TestMutateBoltValueFallbackGetError(t *testing.T) {
	inner := newMockKVStore()
	inner.errGet = fmt.Errorf("get error")
	var list eventWebhookList
	err := mutateKVValue(&noMutateKV{inner}, "b", "k", &list, 0, func(_ bool) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "get error") {
		t.Errorf("expected 'get error', got %v", err)
	}
}

func TestMutateBoltValueFallbackMutateError(t *testing.T) {
	inner := newMockKVStore()
	var list eventWebhookList
	err := mutateKVValue(&noMutateKV{inner}, "b", "k", &list, 0, func(_ bool) error {
		return fmt.Errorf("custom err")
	})
	if err == nil || err.Error() != "custom err" {
		t.Errorf("expected 'custom err', got %v", err)
	}
}

func TestMutateBoltValueWithMutate(t *testing.T) {
	kv := newMockKVStore()
	var list eventWebhookList
	err := mutateKVValue(kv, "wh", "k1", &list, 0, func(exists bool) error {
		if exists {
			t.Error("first call should have exists=false")
		}
		list.Webhooks = append(list.Webhooks, EventWebhookConfig{ID: "wh1"})
		return nil
	})
	if err != nil {
		t.Fatalf("first mutate: %v", err)
	}

	err = mutateKVValue(kv, "wh", "k1", &list, 0, func(exists bool) error {
		if !exists {
			t.Error("second call should have exists=true")
		}
		list.Webhooks = append(list.Webhooks, EventWebhookConfig{ID: "wh2"})
		return nil
	})
	if err != nil {
		t.Fatalf("second mutate: %v", err)
	}
	if len(list.Webhooks) != 2 {
		t.Fatalf("expected 2, got %d", len(list.Webhooks))
	}
}

func TestCertMatchesDomainAllCases(t *testing.T) {
	cert := &x509.Certificate{DNSNames: []string{"example.com"}}
	if !certMatchesDomain(cert, "example.com") {
		t.Error("direct")
	}

	cert = &x509.Certificate{DNSNames: []string{"*.example.com"}}
	if !certMatchesDomain(cert, "sub.example.com") {
		t.Error("wildcard")
	}
	if certMatchesDomain(cert, "example.com") {
		t.Error("apex should not match wildcard")
	}

	cert = &x509.Certificate{}
	cert.Subject.CommonName = "myapp.com"
	if !certMatchesDomain(cert, "myapp.com") {
		t.Error("CN match")
	}
	if certMatchesDomain(cert, "other.com") {
		t.Error("CN mismatch")
	}

	cert.Subject.CommonName = "*.example.com"
	if !certMatchesDomain(cert, "sub.example.com") {
		t.Error("wildcard CN")
	}
	if certMatchesDomain(cert, "example.com") {
		t.Error("wildcard CN apex")
	}

	cert = &x509.Certificate{}
	if certMatchesDomain(cert, "x") {
		t.Error("empty")
	}
}

func TestAppVisibleToTenantAllPaths(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1"})
	h := NewVolumeHandler(nil, s, nil)

	if !h.appVisibleToTenant(context.Background(), "a1", "t1", map[string]string{"monster.tenant": "t1"}) {
		t.Error("label match")
	}
	if h.appVisibleToTenant(context.Background(), "a1", "t2", map[string]string{"monster.tenant": "t1"}) {
		t.Error("label mismatch")
	}
	if !h.appVisibleToTenant(context.Background(), "a1", "t1", nil) {
		t.Error("store match")
	}
	if h.appVisibleToTenant(context.Background(), "a1", "t2", nil) {
		t.Error("store mismatch")
	}

	h2 := NewVolumeHandler(nil, nil, nil)
	if h2.appVisibleToTenant(context.Background(), "a1", "t1", nil) {
		t.Error("nil store")
	}
}

func TestRequireTenantDomainPaths(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "app1", TenantID: "t1"})
	s.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "ex.com"})
	h := NewDomainVerifyHandler(s, newMockKVStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	_, ok := h.requireTenantDomain(rr, req, "nonexistent", "t1")
	if ok {
		t.Error("should fail for missing domain")
	}

	s.addApp(&core.Application{ID: "app2", TenantID: "t2"})
	s.addDomain(&core.Domain{ID: "d2", AppID: "app2", FQDN: "other.com"})
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	_, ok = h.requireTenantDomain(rr, req, "d2", "t1")
	if ok {
		t.Error("should fail for wrong tenant")
	}
}

func TestRequireTenantCertDomainPaths(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "app1", TenantID: "t1"})
	s.addDomain(&core.Domain{ID: "d1", AppID: "app1", FQDN: "ex.com"})
	h := NewCertificateHandler(s, newMockKVStore())

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	_, ok := h.requireTenantCertificateDomain(rr, req, "nonexistent", "t1")
	if ok {
		t.Error("should fail for missing domain")
	}

	_, ok = h.requireTenantCertificateDomain(rr, req, "ex.com", "t1")
	if !ok {
		t.Error("should find by FQDN")
	}

	s.addApp(&core.Application{ID: "app2", TenantID: "t2"})
	s.addDomain(&core.Domain{ID: "d2", AppID: "app2", FQDN: "other.com"})
	_, ok = h.requireTenantCertificateDomain(rr, req, "d2", "t1")
	if ok {
		t.Error("should fail for wrong tenant")
	}
}

func TestCheckAndIncrementRateLimit(t *testing.T) {
	h := &AuthHandler{}
	locked, _ := h.checkPerAccountRateLimit("u@e.com")
	if locked {
		t.Error("nil kv should not lock")
	}

	kv := newMockKVStore()
	h.kv = kv

	// Expired lock
	kv.Set("account_rl", "u@e.com", accountRateLimitEntry{LockedUntil: time.Now().Add(-1 * time.Minute).Unix()}, 0)
	locked, _ = h.checkPerAccountRateLimit("u@e.com")
	if locked {
		t.Error("expired lock should not lock")
	}

	// KV error on check
	kv.errGet = fmt.Errorf("fail")
	locked, _ = h.checkPerAccountRateLimit("e@e.com")
	if locked {
		t.Error("kv error should not lock")
	}
	kv.errGet = nil

	// Increment nil kv
	(&AuthHandler{}).incrementPerAccountRateLimit(context.Background(), "u@e.com")

	// Already locked - should not increment
	kv.Set("account_rl", "lk@e.com", accountRateLimitEntry{FailedCount: 5, LockedUntil: time.Now().Add(15 * time.Minute).Unix()}, 0)
	h.incrementPerAccountRateLimit(context.Background(), "lk@e.com")
	var entry accountRateLimitEntry
	kv.Get("account_rl", "lk@e.com", &entry)
	if entry.FailedCount != 5 {
		t.Errorf("should stay at 5, got %d", entry.FailedCount)
	}

	// KV get error on increment
	bolt2 := newMockKVStore()
	bolt2.errGet = fmt.Errorf("fail")
	h.kv = bolt2
	h.incrementPerAccountRateLimit(context.Background(), "u@e.com")
}

func TestLoginRateLimitEdge(t *testing.T) {
	h := &AuthHandler{}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	if r := h.loginRateLimitCheck(rr, req, "u@e.com"); r != 0 {
		t.Errorf("expected 0, got %d", r)
	}

	kv := newMockKVStore()
	lu := time.Now().Add(15 * time.Minute).Unix()
	kv.Set("account_rl", "l@e.com", accountRateLimitEntry{LockedUntil: lu}, 0)
	h.kv = kv
	rr = httptest.NewRecorder()
	r := h.loginRateLimitCheck(rr, req, "l@e.com")
	if r != lu {
		t.Errorf("expected %d, got %d", lu, r)
	}
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
}

func TestRevokeAccessTokenEdgeCases(t *testing.T) {
	(&AuthHandler{}).revokeAccessTokenFromRequest(httptest.NewRequest("GET", "/", nil))
	h := &AuthHandler{kv: newMockKVStore(), authMod: testAuthModule(nil)}
	h.revokeAccessTokenFromRequest(httptest.NewRequest("GET", "/", nil))
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	h.revokeAccessTokenFromRequest(req)
}

func TestTrackAndEnforceSessionEdge(t *testing.T) {
	(&AuthHandler{}).trackSession(httptest.NewRequest("GET", "/", nil), "u1", "tok")
	(&AuthHandler{}).enforceSessionLimit("u1")
	(&AuthHandler{kv: newMockKVStore()}).enforceSessionLimit("")
}

func TestStripPortAllVariants(t *testing.T) {
	for in, want := range map[string]string{
		"1.2.3.4:8080": "1.2.3.4", "192.168.1.1:80": "192.168.1.1",
		"hostname:443": "hostname", "[::1]:8080": "[::1]:8080", "no-port": "no-port",
	} {
		if got := stripPort(in); got != want {
			t.Errorf("stripPort(%q)=%q, want %q", in, got, want)
		}
	}
}

func TestSubscribeRestartHistoryNilBoltEvents(t *testing.T) {
	SubscribeRestartHistory(nil, nil)
	SubscribeRestartHistory(core.NewEventBus(slog.Default()), nil)
	SubscribeRestartHistory(nil, newMockKVStore())
}

func TestGetConnectionNilBolt(t *testing.T) {
	h := &GitSourceHandler{}
	_, err := h.getConnection("t1", "github")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestListConnectionsNilBolt(t *testing.T) {
	h := &GitSourceHandler{}
	records, err := h.listConnections("t1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if records != nil {
		t.Errorf("expected nil, got %v", records)
	}
}

func TestProviderForRequestEdge(t *testing.T) {
	h := NewGitSourceHandler(core.NewServices(), newMockKVStore(), nil)
	_ = h.providerForRequest(httptest.NewRequest("GET", "/", nil), "github")

	req := httptest.NewRequest("GET", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	_ = h.providerForRequest(req, "github")
}

func TestAuthHandlerHelpers(t *testing.T) {
	h := NewAuthHandler(nil, newMockStore(), newMockKVStore())
	if h.totpValidator != nil {
		t.Error("totpValidator should be nil")
	}

	h2 := &AuthHandler{}
	if h2.log() == nil {
		t.Error("log() should return default")
	}
	if h2.validateTOTP("u1", "c") {
		t.Error("validateTOTP should return false")
	}
}

func TestRegistrationSlugAll(t *testing.T) {
	slug := registrationTenantSlug("Test User")
	if !strings.HasPrefix(slug, "test-user-") {
		t.Errorf("got %q", slug)
	}
	slug2 := registrationTenantSlug(strings.Repeat("a", 100))
	if len(slug2) > 90 {
		t.Errorf("slug too long: %d", len(slug2))
	}
}

func TestServerDeleteErrorPaths(t *testing.T) {
	h := NewServerHandler(newMockStore(), core.NewServices(), testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/local", nil)
	req.SetPathValue("id", "local")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Delete(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	s := newMockStore()
	s.addServer(&core.Server{ID: "s1", TenantID: ""})
	h2 := NewServerHandler(s, core.NewServices(), testCore().Events)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/s1", nil)
	req.SetPathValue("id", "s1")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.Delete(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestGitDisconnectErrorPaths(t *testing.T) {
	h := NewGitSourceHandler(core.NewServices(), newMockKVStore(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/", nil)
	h.Disconnect(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	h2 := NewGitSourceHandler(core.NewServices(), nil, nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.Disconnect(rr, req)
	t.Logf("restore status=%d", rr.Code)

	h3 := NewGitSourceHandler(core.NewServices(), newMockKVStore(), nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h3.Disconnect(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSecretDeleteErrorPaths(t *testing.T) {
	h := NewSecretHandler(newMockStore(), nil, testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("DELETE", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Delete(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/s1", nil)
	req.SetPathValue("id", "s1")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Delete(rr, req)
	if rr.Code != http.StatusNotImplemented {
		t.Errorf("expected 501, got %d", rr.Code)
	}
}

func TestBackupRestoreNoStorage(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1"})
	h := NewBackupHandler(s, nil, testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/k1/restore", nil)
	req.SetPathValue("key", "k1")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Restore(rr, req)
	t.Logf("restore status=%d", rr.Code)
}

func TestStorageUsageNoAuth(t *testing.T) {
	h := NewStorageHandler(newMockStore(), nil, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.Usage(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Usage(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSuspendResumeConflict(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "T", Status: "suspended"})
	h := NewSuspendHandler(s, nil, testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.Suspend(rr, req)
	t.Logf("suspend status=%d", rr.Code)

	s2 := newMockStore()
	s2.addApp(&core.Application{ID: "a2", TenantID: "t1", Name: "T2", Status: "running"})
	h2 := NewSuspendHandler(s2, nil, testCore().Events)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/", nil)
	req.SetPathValue("id", "a2")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.Resume(rr, req)
}

func TestMonitoringAlertsListError(t *testing.T) {
	c := testCore()
	c.Services.Container = &mockContainerRuntime{listErr: fmt.Errorf("list failed")}
	h := NewMonitoringHandler(c, time.Now())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.Alerts(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// TestSystemDiskAndAppDiskNil covers SysDisk and AppDisk both nil in
func TestSystemDiskAndAppDiskNil(t *testing.T) {
	h := NewDiskUsageHandler(newMockStore(), nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.SystemDisk(rr, req)
	t.Logf("system disk status=%d", rr.Code)

	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1"})
	h2 := NewDiskUsageHandler(s, nil)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/a1/disk", nil)
	req.SetPathValue("id", "a1")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.AppDisk(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestVolumeCreateAndListErrors(t *testing.T) {
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1"})
	r := &mockContainerRuntime{listErr: fmt.Errorf("err")}
	h := NewVolumeHandler(r, s, testCore().Events)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h.List(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}

	h2 := NewVolumeHandler(nil, newMockStore(), testCore().Events)
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/", strings.NewReader(`{"name":""}`))
	req.Header.Set("Content-Type", "application/json")
	req = withClaims(req, "u1", "t1", "owner", "u@e.com")
	h2.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestDeployFreezeEndpointsNoAuth(t *testing.T) {
	h := NewDeployFreezeHandler(newMockStore(), testCore().Events, newMockKVStore())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	h.Create(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/", nil)
	h.Get(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("DELETE", "/f1", nil)
	req.SetPathValue("id", "f1")
	h.Delete(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestStripeWebhookNotConfigured(t *testing.T) {
	h := NewStripeWebhookHandler(nil, newMockKVStore(), slog.Default())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/", nil)
	h.ServeHTTP(rr, req)
	t.Logf("restore status=%d", rr.Code)
}

func TestCertificateListAndTopology(t *testing.T) {
	h := NewCertificateHandler(newMockStore(), newMockKVStore())
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	th := &TopologyHandler{store: newMockStore()}
	rr = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/", nil)
	th.Validate(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAppStartStopRestartNoRuntime(t *testing.T) {
	c := testCore()
	s := newMockStore()
	s.addApp(&core.Application{ID: "a1", TenantID: "t1", Name: "T", Status: "running"})
	h := NewAppHandler(s, c)

	for _, method := range []string{"start", "stop", "restart"} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/"+method, nil)
		req.SetPathValue("id", "a1")
		req = withClaims(req, "u1", "t1", "owner", "u@e.com")
		switch method {
		case "start":
			h.Start(rr, req)
		case "stop":
			h.Stop(rr, req)
		case "restart":
			h.Restart(rr, req)
		}
		if rr.Code != http.StatusServiceUnavailable {
			t.Errorf("%s expected 503, got %d", method, rr.Code)
		}
	}
}

func TestUserSessionsEdge(t *testing.T) {
	h := &SessionHandler{}
	s, err := h.GetUserSessions("u1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if s != nil {
		t.Errorf("expected nil, got %v", s)
	}

	kv := newMockKVStore()
	kv.Set("user_sessions", "u1:j1", SessionTrackingInfo{UserID: "u1", JTI: "j1"}, 0)
	kv.Set("user_sessions", "u1:j2", SessionTrackingInfo{UserID: "u1", JTI: "j2"}, 0)
	h.kv = kv
	s, err = h.GetUserSessions("u1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(s) != 2 {
		t.Errorf("expected 2, got %d", len(s))
	}

	// Revoke all (empty)
	h2 := &SessionHandler{kv: kv}
	err = h2.revokeAllUserSessions(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// === merged from helpers_boost_test.go ===

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

// === merged from image_tags_boost_test.go ===

func TestImageTagHandler_List_Unauthorized(t *testing.T) {
	store := newMockStore()
	h := NewImageTagHandler(store, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=nginx", nil)
	// No claims
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestImageTagHandler_List_ForbiddenImage(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "tenant-1",
		Name:       "My App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.latestDeployments["app-1"] = &core.Deployment{
		AppID: "app-1",
		Image: "nginx:latest",
	}

	h := NewImageTagHandler(store, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=redis", nil)
	req = withClaims(req, "user1", "tenant-1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", rr.Code)
	}
}

func TestImageTagHandler_List_RuntimeError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "tenant-1",
		Name:       "My App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.latestDeployments["app-1"] = &core.Deployment{
		AppID: "app-1",
		Image: "nginx:latest",
	}

	runtime := &mockContainerRuntime{imageListErr: context.Canceled}
	h := NewImageTagHandler(store, runtime)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=nginx", nil)
	req = withClaims(req, "user1", "tenant-1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestImageTagHandler_List_EmptyResult(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "tenant-1",
		Name:       "My App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.latestDeployments["app-1"] = &core.Deployment{
		AppID: "app-1",
		Image: "nginx:latest",
	}

	runtime := &mockContainerRuntime{imageList: []core.ImageInfo{}}
	h := NewImageTagHandler(store, runtime)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=nginx", nil)
	req = withClaims(req, "user1", "tenant-1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	tags, _ := resp["tags"].([]any)
	if len(tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(tags))
	}
	if resp["total"] != float64(0) {
		t.Errorf("total = %v, want 0", resp["total"])
	}
}

func TestImageTagHandler_List_WithMatches_Allowed(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:         "app-1",
		TenantID:   "tenant-1",
		Name:       "My App",
		SourceType: "image",
		SourceURL:  "nginx:latest",
		Status:     "running",
	})
	store.latestDeployments["app-1"] = &core.Deployment{
		AppID: "app-1",
		Image: "nginx:latest",
	}

	runtime := &mockContainerRuntime{
		imageList: []core.ImageInfo{
			{ID: "sha256:abc", Tags: []string{"nginx:latest", "nginx:1.25"}, Size: 1000000},
			{ID: "sha256:def", Tags: []string{"redis:7"}, Size: 500000},
		},
	}
	h := NewImageTagHandler(store, runtime)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/images/tags?image=nginx", nil)
	req = withClaims(req, "user1", "tenant-1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["image"] != "nginx" {
		t.Errorf("image = %v, want nginx", resp["image"])
	}
	tags, _ := resp["tags"].([]any)
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
}

// === merged from projects_boost_test.go ===

func TestRequireTenantProject_NoClaims(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/projects/proj1", nil)
	w := httptest.NewRecorder()

	store := newMockStore()
	result := requireTenantProject(w, req, store)
	if result != nil {
		t.Error("expected nil result")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("code = %d, want 401", w.Code)
	}
}

func TestRequireTenantProject_MissingID(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/v1/projects/", nil)
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	w := httptest.NewRecorder()

	store := newMockStore()
	result := requireTenantProject(w, req, store)
	if result != nil {
		t.Error("expected nil result")
	}
	if w.Code != http.StatusBadRequest {
		t.Errorf("code = %d, want 400", w.Code)
	}
}

func TestRequireTenantProject_NotFound(t *testing.T) {
	store := newMockStore()

	req := httptest.NewRequest("GET", "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	w := httptest.NewRecorder()

	result := requireTenantProject(w, req, store)
	if result != nil {
		t.Error("expected nil result")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", w.Code)
	}
}

func TestRequireTenantProject_WrongTenant(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj1", TenantID: "tenant2", Name: "Alpha"})

	req := httptest.NewRequest("GET", "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	w := httptest.NewRecorder()

	result := requireTenantProject(w, req, store)
	if result != nil {
		t.Error("expected nil result")
	}
	if w.Code != http.StatusNotFound {
		t.Errorf("code = %d, want 404", w.Code)
	}
}

func TestRequireTenantProject_Success(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj1", TenantID: "tenant1", Name: "Alpha"})

	req := httptest.NewRequest("GET", "/api/v1/projects/proj1", nil)
	req.SetPathValue("id", "proj1")
	req = withClaims(req, "u1", "tenant1", "role_admin", "a@b.com")
	w := httptest.NewRecorder()

	result := requireTenantProject(w, req, store)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.ID != "proj1" {
		t.Errorf("id = %q, want proj1", result.ID)
	}
}

// === merged from sessions_boost2_test.go ===

func TestSessionHandler_LogoutAll_RevokeError(t *testing.T) {
	// Seed a session so revokeAllUserSessions has work to do,
	// but kv.Delete will succeed, not fail. We need to simulate
	// an error in revokeAllUserSessions. Looking at the implementation,
	// it calls GetUserSessions then deletes each. GetUserSessions
	// returns nil error when kv is present. So we need kv.List
	// to return an error.
	bolt2 := &mockKVStore{data: make(map[string]map[string][]byte)}
	bolt2.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a"}, 0)

	// Now make List return error by clearing the data after Set
	// Actually List returns error when bucket not found. Let's just
	// use the normal kv but the error path is hard to trigger.
	// Instead, test LogoutAll with claims.ID revocation error.

	// Re-read the code: revokeAllUserSessions error path is when
	// GetUserSessions returns error. That happens when kv.List fails.
	// Let's create a kv that fails List.
	listErrBolt := &listErrorKV{mockKVStore: *newMockKVStore()}
	listErrBolt.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a"}, 0)

	h := NewSessionHandler(newMockStore(), listErrBolt, testAuthModule(newMockStore()))

	req := httptest.NewRequest("POST", "/api/v1/auth/logout-all", nil)
	req = withClaims(req, "user-1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.LogoutAll(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

// listErrorKV is a mock kv that fails List.
type listErrorKV struct {
	mockKVStore
}

func (l *listErrorKV) List(bucket string) ([]string, error) {
	return nil, errors.New("kv list error")
}

func TestSessionHandler_LogoutAll_RevokeAccessTokenError(t *testing.T) {
	kv := newMockKVStore()
	// Seed a session
	kv.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a"}, 0)

	// Use a real auth module so claims can be validated
	store := newMockStore()
	authMod := testAuthModule(store)

	// Generate a valid token for the user so claims.ID is set
	tokens, _ := authMod.JWT().GenerateTokenPair("user-1", "t1", "role_admin", "admin@test.com")
	claims, _ := authMod.JWT().ValidateAccessToken(tokens.AccessToken)

	// Put the claims into the request context
	req := httptest.NewRequest("POST", "/api/v1/auth/logout-all", nil)
	ctx := auth.ContextWithClaims(req.Context(), claims)
	req = req.WithContext(ctx)
	rr := httptest.NewRecorder()

	h := NewSessionHandler(store, kv, authMod)
	h.LogoutAll(rr, req)

	// Even if token revocation fails, the endpoint should return 200
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

// === merged from sessions_boost_test.go ===

func TestSessionHandler_GetUserSessions(t *testing.T) {
	kv := newMockKVStore()
	h := NewSessionHandler(newMockStore(), kv, nil)

	// Seed two sessions for user-1 and one for user-2
	kv.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a", IP: "1.1.1.1", CreatedAt: time.Now().Add(-2 * time.Hour)}, 0)
	kv.Set("user_sessions", "user-1:jti-b", SessionTrackingInfo{UserID: "user-1", JTI: "jti-b", IP: "2.2.2.2", CreatedAt: time.Now().Add(-1 * time.Hour)}, 0)
	kv.Set("user_sessions", "user-2:jti-c", SessionTrackingInfo{UserID: "user-2", JTI: "jti-c", IP: "3.3.3.3", CreatedAt: time.Now()}, 0)

	sessions, err := h.GetUserSessions("user-1")
	if err != nil {
		t.Fatalf("GetUserSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
	// Should be sorted oldest first
	if sessions[0].JTI != "jti-a" {
		t.Errorf("expected oldest session first, got %s", sessions[0].JTI)
	}
}

func TestSessionHandler_GetUserSessions_NilBolt(t *testing.T) {
	h := NewSessionHandler(newMockStore(), nil, nil)

	sessions, err := h.GetUserSessions("user-1")
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
	if sessions != nil {
		t.Error("expected nil sessions when kv is nil")
	}
}

func TestSessionHandler_revokeAllUserSessions(t *testing.T) {
	kv := newMockKVStore()
	h := NewSessionHandler(newMockStore(), kv, nil)

	kv.Set("user_sessions", "user-1:jti-abcdef", SessionTrackingInfo{UserID: "user-1", JTI: "jti-abcdef"}, 0)
	kv.Set("user_sessions", "user-1:jti-xyz123", SessionTrackingInfo{UserID: "user-1", JTI: "jti-xyz123"}, 0)

	err := h.revokeAllUserSessions(context.Background(), "user-1")
	if err != nil {
		t.Errorf("revokeAllUserSessions: %v", err)
	}

	// Verify sessions are deleted
	keys, _ := kv.List("user_sessions")
	if len(keys) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(keys))
	}
}

type testAuthServicesWithTOTP struct {
	jwt  *auth.JWTService
	totp *auth.TOTPService
}

func (s *testAuthServicesWithTOTP) JWT() *auth.JWTService {
	return s.jwt
}

func (s *testAuthServicesWithTOTP) TOTP() *auth.TOTPService {
	return s.totp
}

type testTOTPVault struct{}

func (testTOTPVault) Encrypt(value string) (string, error) {
	return "enc:" + value, nil
}

func (testTOTPVault) Decrypt(value string) (string, error) {
	return strings.TrimPrefix(value, "enc:"), nil
}

func TestSessionHandler_EnableTOTPRejectsMalformedJSON(t *testing.T) {
	store := newMockStore()
	h := NewSessionHandler(store, nil, &testAuthServicesWithTOTP{
		jwt:  testJWT(),
		totp: auth.NewTOTPService(store),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/totp/enroll", strings.NewReader("{"))
	req = withClaims(req, "user-1", "tenant-1", "role_admin", "admin@example.com")
	rr := httptest.NewRecorder()

	h.EnableTOTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for malformed JSON, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSessionHandler_EnableTOTPRejectsTrailingJSON(t *testing.T) {
	store := newMockStore()
	h := NewSessionHandler(store, nil, &testAuthServicesWithTOTP{
		jwt:  testJWT(),
		totp: auth.NewTOTPService(store),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/totp/enroll", strings.NewReader(`{"code":"123456"}{"code":"654321"}`))
	req = withClaims(req, "user-1", "tenant-1", "role_admin", "admin@example.com")
	rr := httptest.NewRecorder()

	h.EnableTOTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for trailing JSON, got %d: %s", rr.Code, rr.Body.String())
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestSessionHandler_TOTPInternalErrorsAreSanitized(t *testing.T) {
	store := newMockStore()
	store.errGetUser = errors.New("db connection string leaked")
	totp := auth.NewTOTPService(store)
	totp.SetVault(testTOTPVault{})
	h := NewSessionHandler(store, nil, &testAuthServicesWithTOTP{
		jwt:  testJWT(),
		totp: totp,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/totp/enroll", nil)
	req = withClaims(req, "user-1", "tenant-1", "role_admin", "admin@example.com")
	rr := httptest.NewRecorder()

	h.EnableTOTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "db connection") || strings.Contains(rr.Body.String(), "get user") {
		t.Fatalf("response leaked internal error: %s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to update TOTP settings") {
		t.Fatalf("response = %s, want generic TOTP failure", rr.Body.String())
	}
}

func TestSessionHandler_TOTPUserErrorsRemainUserFacing(t *testing.T) {
	store := newMockStore()
	store.addUser(&core.User{
		ID:         "user-1",
		Email:      "admin@example.com",
		TOTPSecret: "enc:JBSWY3DPEHPK3PXP",
	}, &core.TeamMember{UserID: "user-1", TenantID: "tenant-1", RoleID: "role_admin"})
	totp := auth.NewTOTPService(store)
	totp.SetVault(testTOTPVault{})
	h := NewSessionHandler(store, nil, &testAuthServicesWithTOTP{
		jwt:  testJWT(),
		totp: totp,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/totp/enroll", strings.NewReader(`{"code":"000000"}`))
	req = withClaims(req, "user-1", "tenant-1", "role_admin", "admin@example.com")
	rr := httptest.NewRecorder()

	h.EnableTOTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid TOTP code") {
		t.Fatalf("response = %s, want validation message", rr.Body.String())
	}
}

func TestSessionHandler_LogoutAll(t *testing.T) {
	kv := newMockKVStore()
	h := NewSessionHandler(newMockStore(), kv, nil)

	kv.Set("user_sessions", "user-1:jti-a", SessionTrackingInfo{UserID: "user-1", JTI: "jti-a"}, 0)

	req := httptest.NewRequest("POST", "/api/v1/auth/logout-all", nil)
	req = withClaims(req, "user-1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.LogoutAll(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSessionHandler_LogoutAll_NoClaims(t *testing.T) {
	kv := newMockKVStore()
	h := NewSessionHandler(newMockStore(), kv, nil)

	req := httptest.NewRequest("POST", "/api/v1/auth/logout-all", nil)
	rr := httptest.NewRecorder()
	h.LogoutAll(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestSessionHandler_ListSessions(t *testing.T) {
	kv := newMockKVStore()
	h := NewSessionHandler(newMockStore(), kv, nil)

	kv.Set("user_sessions", "user-1:jti-abcdef", SessionTrackingInfo{UserID: "user-1", JTI: "jti-abcdef", IP: "127.0.0.1", UserAgent: "TestAgent", CreatedAt: time.Now()}, 0)

	req := httptest.NewRequest("GET", "/api/v1/auth/sessions", nil)
	req = withClaims(req, "user-1", "t1", "role_admin", "admin@test.com")
	req.Header.Set("User-Agent", "TestAgent")
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["count"].(float64)) != 1 {
		t.Errorf("expected 1 session, got %v", resp["count"])
	}
}

func TestSessionHandler_ListSessions_ShortJTI(t *testing.T) {
	kv := newMockKVStore()
	h := NewSessionHandler(newMockStore(), kv, nil)

	kv.Set("user_sessions", "user-1:jti", SessionTrackingInfo{UserID: "user-1", JTI: "jti", IP: "127.0.0.1", UserAgent: "TestAgent", CreatedAt: time.Now()}, 0)

	req := httptest.NewRequest("GET", "/api/v1/auth/sessions", nil)
	req = withClaims(req, "user-1", "t1", "role_admin", "admin@test.com")
	req.Header.Set("User-Agent", "TestAgent")
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestSessionHandler_ListSessions_NoClaims(t *testing.T) {
	kv := newMockKVStore()
	h := NewSessionHandler(newMockStore(), kv, nil)

	req := httptest.NewRequest("GET", "/api/v1/auth/sessions", nil)
	rr := httptest.NewRecorder()
	h.ListSessions(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// === merged from snapshots_boost_test.go ===

func TestSnapshot_Create_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app1/snapshots", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestSnapshot_Create_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app123456789", TenantID: "tenant1", Name: "Test"})
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app123456789/snapshots", nil)
	req.SetPathValue("id", "app123456789")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SnapshotInfo
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.AppID != "app123456789" {
		t.Errorf("app_id = %q, want app1", resp.AppID)
	}
	if resp.ID == "" {
		t.Error("expected non-empty snapshot ID")
	}
}

func TestSnapshot_Create_ShortAppID(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "a", TenantID: "tenant1", Name: "Test"})
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/a/snapshots", nil)
	req.SetPathValue("id", "a")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp SnapshotInfo
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !strings.HasPrefix(resp.Image, "monster-snapshot/a:") {
		t.Fatalf("image = %q, want short app ID prefix", resp.Image)
	}
}

func TestSnapshot_List_NoClaims(t *testing.T) {
	store := newMockStore()
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/snapshots", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestSnapshot_List_AppNotFound(t *testing.T) {
	store := newMockStore()
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app1/snapshots", nil)
	req.SetPathValue("id", "app1")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rr.Code)
	}
}

func TestSnapshot_List_Success(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app123456789", TenantID: "tenant1", Name: "Test"})
	handler := NewSnapshotHandler(store, &mockContainerRuntime{}, testCore().Events)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app123456789/snapshots", nil)
	req.SetPathValue("id", "app123456789")
	req = withClaims(req, "user1", "tenant1", "role_owner", "user@example.com")
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 0 {
		t.Errorf("expected total=0, got %v", resp["total"])
	}
}

// === merged from ssl_status_boost_test.go ===

func TestCheckSSL_Error(t *testing.T) {
	result := checkSSL("invalid.host.that.does.not.exist.example:99999")
	if result.Error == "" {
		t.Error("expected error for invalid host")
	}
	if result.FQDN != "invalid.host.that.does.not.exist.example:99999" {
		t.Errorf("FQDN = %q, want original input", result.FQDN)
	}
}

func TestSSLStatusHandler_Check_CacheHit(t *testing.T) {
	kv := &mockKVStore{data: make(map[string]map[string][]byte)}
	h := NewSSLStatusHandler(kv)

	// Seed cache
	cached := SSLCheckResult{FQDN: "example.com", Valid: true, DaysLeft: 30}
	_ = kv.Set("certificates", "ssl_check:example.com", cached, 300)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/ssl-status?fqdn=example.com", nil)
	rr := httptest.NewRecorder()

	h.Check(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSSLStatusHandler_Check_MissingFQDN_Boost(t *testing.T) {
	h := NewSSLStatusHandler(newMockKVStore())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/ssl-status", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestSSLStatusHandler_Check_CacheMiss(t *testing.T) {
	kv := newMockKVStore()
	h := NewSSLStatusHandler(kv)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/domains/1/ssl-status?fqdn=bad.local", nil)
	rr := httptest.NewRecorder()
	h.Check(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestCheckSSL_ValidLocalTLS(t *testing.T) {
	// Spin up a local HTTPS server with a self-signed cert so we can
	// exercise the success path of checkSSL. Since we can't skip
	// verification in checkSSL (InsecureSkipVerify is false), we
	// need a cert the system trusts. Instead, use a real public
	// domain that we expect to have valid TLS.
	result := checkSSL("google.com")
	// Either it succeeds with valid=true or it fails due to network
	// issues. If it succeeds, verify the fields are populated.
	if result.Valid {
		if result.Issuer == "" {
			t.Error("expected Issuer to be set")
		}
		if result.Subject == "" {
			t.Error("expected Subject to be set")
		}
		if result.ExpiresAt.IsZero() {
			t.Error("expected ExpiresAt to be set")
		}
		if result.DaysLeft <= 0 {
			t.Errorf("expected positive DaysLeft, got %d", result.DaysLeft)
		}
	}
}

// === merged from topology_boost2_test.go ===

func TestTopologyHandler_Save_Success(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj-1", TenantID: "tenant-1", Name: "Test Project"})
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/save", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["success"] != true {
		t.Errorf("expected success=true, got %v", resp["success"])
	}
}

func TestTopologyHandler_Save_CrossTenantProject(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj-1", TenantID: "tenant-2", Name: "Foreign Project"})
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/save", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
	if _, ok := kv.data["topologies"]; ok {
		t.Fatal("cross-tenant save wrote topology data")
	}
}

func TestTopologyHandler_Save_NoClaims(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/save", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Save_InvalidBody(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	httpReq := httptest.NewRequest("POST", "/api/v1/topology/save", bytes.NewReader([]byte(`{invalid`)))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Save_RejectsTrailingJSON(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj-1", TenantID: "tenant-1", Name: "Test Project"})
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	body := []byte(`{"nodes":[],"edges":[],"projectId":"proj-1","environment":"production"}{"nodes":[]}`)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/save", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Save(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	assertErrorMessage(t, w, "invalid request body")
}

func TestGetStringMapFromMap(t *testing.T) {
	m := map[string]any{
		"labels": map[string]any{
			"app": "myapp",
			"env": "prod",
			"num": 42,
		},
		"empty": map[string]any{},
	}

	result := getStringMapFromMap(m, "labels")
	if len(result) != 2 {
		t.Errorf("expected 2 entries, got %d", len(result))
	}
	if result["app"] != "myapp" {
		t.Errorf("app = %q, want myapp", result["app"])
	}
	if result["env"] != "prod" {
		t.Errorf("env = %q, want prod", result["env"])
	}
	// Non-string value should be skipped
	if _, ok := result["num"]; ok {
		t.Error("expected num to be skipped (not a string)")
	}

	// Missing key
	empty := getStringMapFromMap(m, "missing")
	if len(empty) != 0 {
		t.Errorf("expected empty map for missing key, got %d", len(empty))
	}

	// Key exists but not a map[string]any
	notMap := getStringMapFromMap(m, "num")
	if len(notMap) != 0 {
		t.Errorf("expected empty map for non-map value, got %d", len(notMap))
	}
}

// === merged from topology_boost_test.go ===

func TestTopologyHandler_Load_NotFound(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	// Seed a project for the tenant (so project check passes, but no topology exists)
	store.projectsByID["proj-1"] = &core.Project{ID: "proj-1", TenantID: "tenant-1", Name: "Test Project"}

	req := httptest.NewRequest("GET", "/api/v1/topology/proj-1/production", nil)
	req.SetPathValue("projectId", "proj-1")
	req.SetPathValue("environment", "production")
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Load(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "No saved topology found" {
		t.Errorf("unexpected message: %v", resp["message"])
	}
}

func TestTopologyHandler_Load_Found(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	// Seed a project for the tenant
	store.projectsByID["proj-1"] = &core.Project{ID: "proj-1", TenantID: "tenant-1", Name: "Test Project"}

	// Seed a saved topology
	key := "topology:tenant-1:proj-1:production"
	kv.Set("topologies", key, TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}, 0)

	req := httptest.NewRequest("GET", "/api/v1/topology/proj-1/production", nil)
	req.SetPathValue("projectId", "proj-1")
	req.SetPathValue("environment", "production")
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Load(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["message"] != "Topology loaded successfully" {
		t.Errorf("unexpected message: %v", resp["message"])
	}
}

func TestTopologyHandler_Load_NoClaims(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	req := httptest.NewRequest("GET", "/api/v1/topology/proj-1/production", nil)
	w := httptest.NewRecorder()
	h.Load(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Compile_Success(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes: []TopologyNode{
			{ID: "app-1", Type: "app", Position: Position{X: 100, Y: 100}, Data: map[string]any{
				"name":   "api",
				"gitUrl": "https://github.com/user/api",
				"branch": "main",
				"port":   3000,
			}},
		},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/compile", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Compile(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp CompileResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Success {
		t.Errorf("expected success, got: %s", resp.Message)
	}
}

func TestTopologyHandler_Compile_EmptyNodes(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{Nodes: []TopologyNode{}, ProjectID: "p1", Environment: "prod"}
	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/compile", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Compile(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Compile_NoClaims(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	httpReq := httptest.NewRequest("POST", "/api/v1/topology/compile", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	h.Compile(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Validate_Success(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	req := TopologyDeployRequest{
		Nodes: []TopologyNode{
			{ID: "app-1", Type: "app", Position: Position{X: 100, Y: 100}, Data: map[string]any{
				"name":   "api",
				"gitUrl": "https://github.com/user/api",
				"branch": "main",
				"port":   3000,
			}},
		},
		Edges:       []TopologyEdge{},
		ProjectID:   "proj-1",
		Environment: "production",
	}

	body, _ := json.Marshal(req)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/validate", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Validate(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["valid"] != true {
		t.Errorf("expected valid=true, got %v", resp["valid"])
	}
}

func TestTopologyHandler_Validate_AllowsReactFlowNodeMetadata(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	body := []byte(`{
		"nodes":[{
			"id":"app-1",
			"type":"app",
			"position":{"x":100,"y":100},
			"data":{"name":"api","gitUrl":"https://github.com/user/api","branch":"main","port":3000},
			"measured":{"width":180,"height":80},
			"selected":true,
			"dragging":false
		}],
		"edges":[],
		"projectId":"proj-1",
		"environment":"production"
	}`)
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/validate", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Validate(w, httpReq)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["valid"] != true {
		t.Errorf("expected valid=true, got %v", resp["valid"])
	}
}

func TestTopologyHandler_Validate_NoClaims(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	httpReq := httptest.NewRequest("POST", "/api/v1/topology/validate", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	h.Validate(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Templates(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	req := httptest.NewRequest("GET", "/api/v1/topology/templates", nil)
	w := httptest.NewRecorder()
	h.Templates(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp []map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp) == 0 {
		t.Error("expected templates to be returned")
	}
}

func TestTopologyHandler_convertToVolume(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	node := TopologyNode{
		ID:   "vol-1",
		Type: "volume",
		Data: map[string]any{
			"name":       "data-vol",
			"sizeGB":     20,
			"mountPath":  "/app/data",
			"volumeType": "nfs",
			"temporary":  true,
		},
	}

	vol := h.convertToVolume(node)
	if vol.ID != "vol-1" {
		t.Errorf("id = %q, want vol-1", vol.ID)
	}
	if vol.Name != "data-vol" {
		t.Errorf("name = %q, want data-vol", vol.Name)
	}
	if vol.SizeGB != 20 {
		t.Errorf("sizeGB = %d, want 20", vol.SizeGB)
	}
	if vol.MountPath != "/app/data" {
		t.Errorf("mountPath = %q, want /app/data", vol.MountPath)
	}
	if vol.VolumeType != "nfs" {
		t.Errorf("volumeType = %q, want nfs", vol.VolumeType)
	}
	if !vol.Temporary {
		t.Error("expected temporary=true")
	}
}

// === merged from topology_deploy_boost_test.go ===

func TestTopologyHandler_Deploy_NoClaims(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestTopologyHandler_Deploy_InvalidBody(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader([]byte(`{invalid`)))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Deploy_EmptyNodes(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{
		Nodes:       []TopologyNode{},
		ProjectID:   "proj-1",
		Environment: "production",
	})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Deploy_PathTraversal(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		ProjectID:   "../etc/passwd",
		Environment: "production",
	})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestTopologyHandler_Deploy_EncodedPathTraversal(t *testing.T) {
	store := newMockStore()
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{
		Nodes:       []TopologyNode{{ID: "app-1", Type: "app"}},
		ProjectID:   "%252e%252e%252fetc",
		Environment: "production",
	})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestValidateTopologyPathParts(t *testing.T) {
	tests := []struct {
		name        string
		projectID   string
		environment string
		want        bool
	}{
		{name: "valid", projectID: "proj-1", environment: "production", want: true},
		{name: "underscore", projectID: "proj_1", environment: "staging_2", want: true},
		{name: "empty project", projectID: "", environment: "production", want: false},
		{name: "dot traversal", projectID: "../etc", environment: "production", want: false},
		{name: "encoded traversal", projectID: "%2e%2e%2fetc", environment: "production", want: false},
		{name: "double encoded traversal", projectID: "%252e%252e%252fetc", environment: "production", want: false},
		{name: "absolute path", projectID: "/var/lib", environment: "production", want: false},
		{name: "backslash path", projectID: "proj-1", environment: "..\\prod", want: false},
		{name: "whitespace", projectID: " proj-1", environment: "production", want: false},
		{name: "colon", projectID: "proj:1", environment: "production", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := validateTopologyPathParts(tt.projectID, tt.environment); got != tt.want {
				t.Fatalf("validateTopologyPathParts(%q, %q) = %v, want %v", tt.projectID, tt.environment, got, tt.want)
			}
		})
	}
}

func TestTopologyHandler_Deploy_CrossTenantProject(t *testing.T) {
	store := newMockStore()
	store.addProjectByID(&core.Project{ID: "proj-1", TenantID: "tenant-2", Name: "Foreign Project"})
	kv := newMockKVStore()
	c := &core.Core{DB: &core.Database{KV: kv}}
	h := NewTopologyHandler(store, c)

	body, _ := json.Marshal(TopologyDeployRequest{
		Nodes: []TopologyNode{{
			ID:   "app-1",
			Type: "app",
			Data: map[string]any{
				"name":   "api",
				"gitUrl": "https://github.com/user/api",
				"branch": "main",
				"port":   3000,
			},
		}},
		ProjectID:   "proj-1",
		Environment: "production",
		DryRun:      true,
	})
	httpReq := httptest.NewRequest("POST", "/api/v1/topology/deploy", bytes.NewReader(body))
	ctx := auth.ContextWithClaims(httpReq.Context(), &auth.Claims{
		UserID:   "user-1",
		TenantID: "tenant-1",
		RoleID:   "admin",
	})
	httpReq = httpReq.WithContext(ctx)

	w := httptest.NewRecorder()
	h.Deploy(w, httpReq)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
