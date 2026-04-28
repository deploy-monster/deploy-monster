package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/api/middleware"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

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
// EventWebhookHandler — hashSecret, checkSecret
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

func TestCheckSecret(t *testing.T) {
	secret := "my-webhook-secret"
	hash := hashSecret(secret)

	if !checkSecret(secret, hash) {
		t.Error("checkSecret should return true for matching secret")
	}
	if checkSecret("wrong-secret", hash) {
		t.Error("checkSecret should return false for wrong secret")
	}
}

// =============================================================================
// RedirectHandler — SetEvents
// =============================================================================

func TestRedirectHandler_SetEvents(t *testing.T) {
	h := NewRedirectHandler(newMockStore(), newMockBoltStore())
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
	h := NewGPUHandler(newMockStore(), &mockContainerRuntime{}, newMockBoltStore())
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
// DeployApprovalHandler — CreatePending
// =============================================================================

func TestDeployApprovalHandler_CreatePending(t *testing.T) {
	h := NewDeployApprovalHandler(newMockStore(), nil)
	req := &ApprovalRequest{
		ID:       "app-1",
		AppID:    "app-1",
		TenantID: "tenant-1",
		Status:   "pending",
		CreatedAt: time.Now(),
	}
	h.CreatePending(req)

	h.mu.RLock()
	_, ok := h.pending["app-1"]
	h.mu.RUnlock()
	if !ok {
		t.Error("expected pending request to be stored")
	}
}

// =============================================================================
// CronJobHandler — SetEvents
// =============================================================================

func TestCronJobHandler_SetEvents(t *testing.T) {
	h := NewCronJobHandler(newMockStore(), newMockBoltStore())
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
	h := NewBasicAuthHandler(newMockStore(), newMockBoltStore())
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
	h := NewAutoscaleHandler(newMockStore(), newMockBoltStore())
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
	bolt := newMockBoltStore()
	h := NewEventWebhookHandler(newMockStore(), nil, bolt)

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
	bolt := newMockBoltStore()
	bolt.Set("event_webhooks", "tenant:t1", eventWebhookList{
		Webhooks: []EventWebhookConfig{
			{ID: "wh-1", URL: "https://example.com/hook", Events: []string{"app.deployed"}, Active: true, TenantID: "t1", SecretHash: "hash123"},
		},
	}, 0)

	h := NewEventWebhookHandler(newMockStore(), nil, bolt)
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
	h := NewEventWebhookHandler(newMockStore(), nil, newMockBoltStore())
	req := httptest.NewRequest("GET", "/api/v1/webhooks/outbound", nil)
	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Create_SuccessBoost(t *testing.T) {
	bolt := newMockBoltStore()
	h := NewEventWebhookHandler(newMockStore(), nil, bolt)

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
	bolt := newMockBoltStore()
	h := NewEventWebhookHandler(newMockStore(), nil, bolt)

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
	bolt := newMockBoltStore()
	list := eventWebhookList{Webhooks: make([]EventWebhookConfig, 20)}
	for i := range list.Webhooks {
		list.Webhooks[i] = EventWebhookConfig{ID: "wh-" + string(rune('a'+i))}
	}
	bolt.Set("event_webhooks", "tenant:t1", list, 0)

	h := NewEventWebhookHandler(newMockStore(), nil, bolt)
	body := `{"url":"https://example.com/hook","events":["app.deployed"]}`
	req := httptest.NewRequest("POST", "/api/v1/webhooks/outbound", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "admin@test.com")
	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestEventWebhookHandler_Delete_SuccessBoost(t *testing.T) {
	bolt := newMockBoltStore()
	bolt.Set("event_webhooks", "tenant:t1", eventWebhookList{
		Webhooks: []EventWebhookConfig{{ID: "wh-1", URL: "https://x.com", Events: []string{"e1"}}},
	}, 0)

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewEventWebhookHandler(newMockStore(), events, bolt)

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
	h := NewEventWebhookHandler(newMockStore(), nil, newMockBoltStore())
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
	h.CreatePending(&ApprovalRequest{
		ID:       "apr-1",
		AppID:    "app-1",
		TenantID: "tenant-1",
		Status:   "pending",
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
	h.CreatePending(&ApprovalRequest{
		ID:       "apr-1",
		AppID:    "app-1",
		TenantID: "tenant-2",
		Status:   "pending",
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
	h.CreatePending(&ApprovalRequest{
		ID:       "apr-1",
		AppID:    "app-1",
		TenantID: "tenant-1",
		Status:   "pending",
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
	h.CreatePending(&ApprovalRequest{
		ID:       "apr-1",
		AppID:    "app-1",
		TenantID: "tenant-2",
		Status:   "pending",
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
	h.CreatePending(&ApprovalRequest{ID: "apr-1", AppID: "app-1", TenantID: "t1", Status: "pending", CreatedAt: time.Now()})
	h.CreatePending(&ApprovalRequest{ID: "apr-2", AppID: "app-1", TenantID: "t1", Status: "approved", CreatedAt: time.Now()})

	req := httptest.NewRequest("GET", "/api/v1/deploy/approvals", nil)
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
	bolt := newMockBoltStore()
	bolt.Set("gpu_config", "app-1", GPUConfig{Enabled: true, Driver: "nvidia", Capabilities: []string{"compute"}}, 0)

	h := NewGPUHandler(store, &mockContainerRuntime{}, bolt)
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

	h := NewGPUHandler(store, &mockContainerRuntime{}, newMockBoltStore())
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
	h := NewGPUHandler(store, &mockContainerRuntime{}, newMockBoltStore())
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

	h := NewAutoscaleHandler(store, newMockBoltStore())
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
	bolt := newMockBoltStore()
	bolt.Set("autoscale", "app-1", AutoscaleConfig{Enabled: true, MinReplicas: 2, MaxReplicas: 5, CPUTarget: 70}, 0)

	h := NewAutoscaleHandler(store, bolt)
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
	h := NewAutoscaleHandler(store, newMockBoltStore())
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

	h := NewAutoscaleHandler(store, newMockBoltStore())
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

	h := NewCronJobHandler(store, newMockBoltStore())
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
	h := NewCronJobHandler(store, newMockBoltStore())
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

	h := NewCronJobHandler(store, newMockBoltStore())
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
	bolt := newMockBoltStore()
	list := cronJobList{Jobs: make([]CronJobConfig, 50)}
	for i := range list.Jobs {
		list.Jobs[i] = CronJobConfig{ID: "job-" + string(rune('a'+i))}
	}
	bolt.Set("cronjobs", "app-1", list, 0)

	h := NewCronJobHandler(store, bolt)
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
	bolt := newMockBoltStore()
	bolt.Set("cronjobs", "app-1", cronJobList{Jobs: []CronJobConfig{{ID: "job-1", Name: "backup", Schedule: "0 2 * * *", Command: "/app/backup.sh"}}}, 0)

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewCronJobHandler(store, bolt)
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

	h := NewBasicAuthHandler(store, newMockBoltStore())
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
	bolt := newMockBoltStore()
	bolt.Set("basic_auth", "app-1", BasicAuthConfig{Enabled: true, Realm: "Private", Users: map[string]string{"admin": "hash"}}, 0)

	h := NewBasicAuthHandler(store, bolt)
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
	h := NewBasicAuthHandler(store, newMockBoltStore())
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

	h := NewBasicAuthHandler(store, newMockBoltStore())

	tests := []struct {
		name string
		body string
		want int
	}{
		{"realm too long", `{"enabled":true,"realm":"` + strings.Repeat("a", 101) + `"}`, http.StatusBadRequest},
		{"too many users", `{"enabled":true,"users":{` + func() string {
			var sb strings.Builder
			for i := 0; i < 51; i++ {
				if i > 0 { sb.WriteByte(',') }
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

	h := NewRedirectHandler(store, newMockBoltStore())
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
	h := NewRedirectHandler(store, newMockBoltStore())
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

	h := NewRedirectHandler(store, newMockBoltStore())

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

	h := NewRedirectHandler(store, newMockBoltStore())
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
	bolt := newMockBoltStore()
	list := redirectList{Rules: make([]RedirectRule, 200)}
	for i := range list.Rules {
		list.Rules[i] = RedirectRule{ID: "r-" + string(rune('a'+i))}
	}
	bolt.Set("redirects", "app-1", list, 0)

	h := NewRedirectHandler(store, bolt)
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

func TestRedirectHandler_Delete(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "tenant-1", Name: "test"})
	bolt := newMockBoltStore()
	bolt.Set("redirects", "app-1", redirectList{Rules: []RedirectRule{{ID: "r-1", Source: "/old", Destination: "/new"}}}, 0)

	events := core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewRedirectHandler(store, bolt)
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
	h := NewAnnouncementHandler(newMockBoltStore())
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
	h := NewAnnouncementHandler(newMockBoltStore())
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
	h := NewAnnouncementHandler(newMockBoltStore())
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
	bolt := newMockBoltStore()
	list := announcementList{Items: make([]Announcement, 100)}
	for i := range list.Items {
		list.Items[i] = Announcement{ID: core.GenerateID(), Title: "Ann " + string(rune(i)), Type: "info"}
	}
	bolt.Set("announcements", "all", list, 0)

	h := NewAnnouncementHandler(bolt)
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

	h := NewDeployTriggerHandler(store, nil, core.NewEventBus(nil))
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

	h := NewDeployTriggerHandler(store, nil, core.NewEventBus(nil))
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
