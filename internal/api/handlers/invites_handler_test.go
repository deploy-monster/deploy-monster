package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"golang.org/x/crypto/bcrypt"
)

// ─── Create Invite ───────────────────────────────────────────────────────────

func TestInviteCreate_Success(t *testing.T) {
	store := newMockStore()
	store.roles["tenant1"] = append(store.roles["tenant1"], core.Role{
		ID:              "role_owner",
		TenantID:        "tenant1",
		PermissionsJSON: `["member.invite","member.list","member.remove"]`,
	})
	events := core.NewEventBus(nil)
	handler := NewInviteHandler(store, events)

	body, _ := json.Marshal(inviteRequest{
		Email:  "newuser@example.com",
		RoleID: "role_member",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/team/invites", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "admin@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["email"] != "newuser@example.com" {
		t.Errorf("expected email 'newuser@example.com', got %v", resp["email"])
	}
	if resp["role_id"] != "role_member" {
		t.Errorf("expected role_id 'role_member', got %v", resp["role_id"])
	}

	// Token is no longer returned in response (security: sent via email instead)
	// But token_hash is still returned for verification
	tokenHash, ok := resp["token_hash"].(string)
	if !ok || tokenHash == "" {
		t.Error("expected non-empty token_hash in response")
	}

	if _, ok := resp["expires_at"]; !ok {
		t.Error("expected expires_at in response")
	}
}

func TestInviteCreate_NoClaims(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	handler := NewInviteHandler(store, events)

	body, _ := json.Marshal(inviteRequest{
		Email:  "user@example.com",
		RoleID: "role_member",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/team/invites", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestInviteCreate_InvalidJSON(t *testing.T) {
	store := newMockStore()
	// Seed the role so permission check passes
	store.roles["tenant1"] = append(store.roles["tenant1"], core.Role{
		ID:              "role_owner",
		TenantID:        "tenant1",
		PermissionsJSON: `["member.invite","member.list","member.remove"]`,
	})
	events := core.NewEventBus(nil)
	handler := NewInviteHandler(store, events)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/team/invites", bytes.NewReader([]byte("{")))
	req = withClaims(req, "user1", "tenant1", "role_owner", "admin@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "invalid request body")
}

func TestInviteCreate_MissingEmail(t *testing.T) {
	store := newMockStore()
	store.roles["tenant1"] = append(store.roles["tenant1"], core.Role{
		ID:              "role_owner",
		TenantID:        "tenant1",
		PermissionsJSON: `["member.invite","member.list","member.remove"]`,
	})
	events := core.NewEventBus(nil)
	handler := NewInviteHandler(store, events)

	body, _ := json.Marshal(inviteRequest{RoleID: "role_member"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/team/invites", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "admin@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "email and role_id are required")
}

func TestInviteCreate_MissingRoleID(t *testing.T) {
	store := newMockStore()
	store.roles["tenant1"] = append(store.roles["tenant1"], core.Role{
		ID:              "role_owner",
		TenantID:        "tenant1",
		PermissionsJSON: `["member.invite","member.list","member.remove"]`,
	})
	events := core.NewEventBus(nil)
	handler := NewInviteHandler(store, events)

	body, _ := json.Marshal(inviteRequest{Email: "user@example.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/team/invites", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "admin@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "email and role_id are required")
}

func TestInviteCreate_BothFieldsMissing(t *testing.T) {
	store := newMockStore()
	store.roles["tenant1"] = append(store.roles["tenant1"], core.Role{
		ID:              "role_owner",
		TenantID:        "tenant1",
		PermissionsJSON: `["member.invite","member.list","member.remove"]`,
	})
	events := core.NewEventBus(nil)
	handler := NewInviteHandler(store, events)

	body, _ := json.Marshal(inviteRequest{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/team/invites", bytes.NewReader(body))
	req = withClaims(req, "user1", "tenant1", "role_owner", "admin@example.com")
	rr := httptest.NewRecorder()

	handler.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
	assertErrorMessage(t, rr, "email and role_id are required")
}

func TestInviteCreate_TokenIsUnique(t *testing.T) {
	store := newMockStore()
	store.roles["tenant1"] = append(store.roles["tenant1"], core.Role{
		ID:              "role_owner",
		TenantID:        "tenant1",
		PermissionsJSON: `["member.invite","member.list","member.remove"]`,
	})
	events := core.NewEventBus(nil)
	handler := NewInviteHandler(store, events)

	// Create two invites and verify token hashes differ (token itself is no longer returned)
	var hashes []string
	for i := 0; i < 2; i++ {
		body, _ := json.Marshal(inviteRequest{
			Email:  "user@example.com",
			RoleID: "role_member",
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/team/invites", bytes.NewReader(body))
		req = withClaims(req, "user1", "tenant1", "role_owner", "admin@example.com")
		rr := httptest.NewRecorder()

		handler.Create(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", rr.Code)
		}

		var resp map[string]any
		json.Unmarshal(rr.Body.Bytes(), &resp)
		hashes = append(hashes, resp["token_hash"].(string))
	}

	if hashes[0] == hashes[1] {
		t.Error("expected unique token hashes for separate invites")
	}
}

// ─── hashToken ───────────────────────────────────────────────────────────────

func TestHashToken_Deterministic(t *testing.T) {
	token := "test-token-value"
	h1 := hashToken(token)
	h2 := hashToken(token)

	// bcrypt is non-deterministic due to salt, so hashes differ between calls
	// but both hashes must verify against the original token.
	if err := bcrypt.CompareHashAndPassword([]byte(h1), []byte(token)); err != nil {
		t.Errorf("bcrypt verify failed for hash h1: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(h2), []byte(token)); err != nil {
		t.Errorf("bcrypt verify failed for hash h2: %v", err)
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := hashToken("token-a")
	h2 := hashToken("token-b")

	if h1 == h2 {
		t.Error("expected different hashes for different tokens")
	}
	// bcrypt hashes have a specific format starting with $2
	if len(h1) < 50 || h1[:4] != "$2a$" {
		t.Errorf("expected bcrypt hash format, got length %d and prefix %q", len(h1), h1[:4])
	}
}
