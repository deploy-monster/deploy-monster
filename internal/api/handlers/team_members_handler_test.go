package handlers

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// mockStore in this package deliberately does NOT implement
// teamMemberStore (it has neither ListTeamMembers nor RemoveTeamMember),
// so TeamHandler.ListMembers takes its single-user fallback branch and
// TeamHandler.RemoveMember returns 501. Those branches were the
// previously-uncovered paths in this package.

func TestTeamHandler_ListMembers_Unauthorized(t *testing.T) {
	h := NewTeamHandler(newMockStore(), core.NewEventBus(slog.Default()))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/team/members", nil)
	rr := httptest.NewRecorder()
	h.ListMembers(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", rr.Code, rr.Body.String())
	}
}

func TestTeamHandler_ListMembers_FallbackPathReturnsCallerOnly(t *testing.T) {
	store := newMockStore()
	user := &core.User{ID: "user-1", Email: "alice@example.com", Name: "Alice"}
	store.addUser(user, &core.TeamMember{
		ID:        "tm-1",
		TenantID:  "tenant-1",
		UserID:    "user-1",
		RoleID:    "role-admin",
		Status:    "active",
		CreatedAt: time.Now(),
	})

	h := NewTeamHandler(store, core.NewEventBus(slog.Default()))
	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/v1/team/members", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	rr := httptest.NewRecorder()
	h.ListMembers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data  []TeamMemberView `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response not JSON: %v", err)
	}
	if resp.Total != 1 || len(resp.Data) != 1 {
		t.Fatalf("expected single-member fallback, got total=%d data=%d", resp.Total, len(resp.Data))
	}
	got := resp.Data[0]
	if got.ID != "tm-1" || got.Email != "alice@example.com" || got.Role != "role-admin" {
		t.Fatalf("unexpected fallback view: %+v", got)
	}
}

func TestTeamHandler_RemoveMember_Unauthorized(t *testing.T) {
	h := NewTeamHandler(newMockStore(), core.NewEventBus(slog.Default()))
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/team/members/tm-1", nil)
	rr := httptest.NewRecorder()
	h.RemoveMember(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
}

func TestTeamHandler_RemoveMember_MissingPathID(t *testing.T) {
	h := NewTeamHandler(newMockStore(), core.NewEventBus(slog.Default()))
	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/team/members/", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	rr := httptest.NewRecorder()
	h.RemoveMember(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when path id is empty", rr.Code)
	}
}

func TestTeamHandler_RemoveMember_StoreDoesNotImplementInterface(t *testing.T) {
	// mockStore lacks ListTeamMembers / RemoveTeamMember, so the handler
	// must respond 501 rather than dereferencing a nil method.
	h := NewTeamHandler(newMockStore(), core.NewEventBus(slog.Default()))
	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/team/members/tm-1", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	req.SetPathValue("id", "tm-1")
	rr := httptest.NewRecorder()
	h.RemoveMember(rr, req)

	if rr.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", rr.Code, rr.Body.String())
	}
}
