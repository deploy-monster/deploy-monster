package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/enterprise"
)

// ─── Get Branding ────────────────────────────────────────────────────────────

func TestBrandingGet_Success(t *testing.T) {
	handler := NewBrandingHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/branding", nil)
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var b enterprise.Branding
	if err := json.Unmarshal(rr.Body.Bytes(), &b); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if b.AppName != "DeployMonster" {
		t.Errorf("expected default app_name 'DeployMonster', got %q", b.AppName)
	}
	if b.PrimaryColor != "#10b981" {
		t.Errorf("expected primary_color '#10b981', got %q", b.PrimaryColor)
	}
}

func TestBrandingGet_ReturnsDefaults(t *testing.T) {
	handler := NewBrandingHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/branding", nil)
	rr := httptest.NewRecorder()

	handler.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var b enterprise.Branding
	json.Unmarshal(rr.Body.Bytes(), &b)

	if b.SupportEmail != "support@deploy.monster" {
		t.Errorf("expected default support_email, got %q", b.SupportEmail)
	}
	if b.Copyright != "DeployMonster by ECOSTACK TECHNOLOGY" {
		t.Errorf("expected default copyright, got %q", b.Copyright)
	}
}

// ─── Update Branding ─────────────────────────────────────────────────────────

func TestBrandingUpdate_Success(t *testing.T) {
	handler := NewBrandingHandler()

	body, _ := json.Marshal(enterprise.Branding{
		AppName:      "MyPaaS",
		PrimaryColor: "#ff0000",
		AccentColor:  "#00ff00",
		Copyright:    "MyCompany 2026",
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/branding", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var b enterprise.Branding
	json.Unmarshal(rr.Body.Bytes(), &b)

	if b.AppName != "MyPaaS" {
		t.Errorf("expected app_name 'MyPaaS', got %q", b.AppName)
	}
	if b.PrimaryColor != "#ff0000" {
		t.Errorf("expected primary_color '#ff0000', got %q", b.PrimaryColor)
	}

	// Verify persistence by reading again
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/branding", nil)
	getRR := httptest.NewRecorder()
	handler.Get(getRR, getReq)

	var stored enterprise.Branding
	json.Unmarshal(getRR.Body.Bytes(), &stored)

	if stored.AppName != "MyPaaS" {
		t.Errorf("branding not persisted: expected 'MyPaaS', got %q", stored.AppName)
	}
}

func TestBrandingUpdate_InvalidJSON(t *testing.T) {
	handler := NewBrandingHandler()

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/branding", bytes.NewReader([]byte("{")))
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestBrandingUpdate_EmptyBody(t *testing.T) {
	handler := NewBrandingHandler()

	body, _ := json.Marshal(enterprise.Branding{})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/branding", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Update(rr, req)

	// Even empty fields are valid — the handler accepts it.
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}
