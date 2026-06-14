package handlers

import (
	"context"
	"encoding/json"
	"errors"
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

func TestTeamHandler_ListMembers_TeamStoreSuccess(t *testing.T) {
	store := &teamStoreMock{
		mockStore: newMockStore(),
		members: []core.TeamMember{
			{ID: "tm-1", TenantID: "tenant-1", UserID: "user-1", RoleID: "role-admin", CreatedAt: time.Now()},
			{ID: "tm-2", TenantID: "tenant-1", UserID: "missing-user", RoleID: "role-dev", CreatedAt: time.Now()},
		},
		users: []core.User{{ID: "user-1", Email: "alice@example.com", Name: "Alice", AvatarURL: "avatar.png"}},
	}
	h := NewTeamHandler(store, core.NewEventBus(slog.Default()))

	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/v1/team/members", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	rr := httptest.NewRecorder()
	h.ListMembers(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data  []TeamMemberView `json:"data"`
		Total int              `json:"total"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Total != 1 || resp.Data[0].Email != "alice@example.com" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestTeamHandler_ListMembers_TeamStoreErrorsAndEmpty(t *testing.T) {
	req := withClaims(httptest.NewRequest(http.MethodGet, "/api/v1/team/members", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")

	h := NewTeamHandler(&teamStoreMock{mockStore: newMockStore()}, core.NewEventBus(slog.Default()))
	rr := httptest.NewRecorder()
	h.ListMembers(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("empty members status = %d", rr.Code)
	}

	h = NewTeamHandler(&teamStoreMock{mockStore: newMockStore(), listErr: errors.New("list failed")}, core.NewEventBus(slog.Default()))
	rr = httptest.NewRecorder()
	h.ListMembers(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("list error status = %d", rr.Code)
	}

	h = NewTeamHandler(&teamStoreMock{
		mockStore: newMockStore(),
		members:   []core.TeamMember{{ID: "tm-1", TenantID: "tenant-1", UserID: "user-1"}},
		usersErr:  errors.New("users failed"),
	}, core.NewEventBus(slog.Default()))
	rr = httptest.NewRecorder()
	h.ListMembers(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("users error status = %d", rr.Code)
	}
}

func TestTeamHandler_RemoveMember_TeamStoreBranches(t *testing.T) {
	req := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/team/members/tm-2", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	req.SetPathValue("id", "tm-2")

	store := &teamStoreMock{
		mockStore: newMockStore(),
		members: []core.TeamMember{
			{ID: "tm-1", TenantID: "tenant-1", UserID: "user-1"},
			{ID: "tm-2", TenantID: "tenant-1", UserID: "user-2"},
		},
	}
	h := NewTeamHandler(store, core.NewEventBus(slog.Default()))
	rr := httptest.NewRecorder()
	h.RemoveMember(rr, req)
	if rr.Code != http.StatusOK || store.removedID != "tm-2" {
		t.Fatalf("remove success status=%d removed=%q body=%s", rr.Code, store.removedID, rr.Body.String())
	}

	reqSelf := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/team/members/tm-1", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	reqSelf.SetPathValue("id", "tm-1")
	rr = httptest.NewRecorder()
	h.RemoveMember(rr, reqSelf)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("self remove status = %d", rr.Code)
	}

	reqMissing := withClaims(httptest.NewRequest(http.MethodDelete, "/api/v1/team/members/missing", nil),
		"user-1", "tenant-1", "role-admin", "alice@example.com")
	reqMissing.SetPathValue("id", "missing")
	rr = httptest.NewRecorder()
	h.RemoveMember(rr, reqMissing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing member status = %d", rr.Code)
	}

	h = NewTeamHandler(&teamStoreMock{mockStore: newMockStore(), listErr: errors.New("list failed")}, core.NewEventBus(slog.Default()))
	rr = httptest.NewRecorder()
	h.RemoveMember(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("list error status = %d", rr.Code)
	}

	h = NewTeamHandler(&teamStoreMock{
		mockStore: newMockStore(),
		members:   []core.TeamMember{{ID: "tm-2", TenantID: "tenant-1", UserID: "user-2"}},
		removeErr: core.ErrNotFound,
	}, core.NewEventBus(slog.Default()))
	rr = httptest.NewRecorder()
	h.RemoveMember(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("remove not found status = %d", rr.Code)
	}

	h = NewTeamHandler(&teamStoreMock{
		mockStore: newMockStore(),
		members:   []core.TeamMember{{ID: "tm-2", TenantID: "tenant-1", UserID: "user-2"}},
		removeErr: errors.New("remove failed"),
	}, core.NewEventBus(slog.Default()))
	rr = httptest.NewRecorder()
	h.RemoveMember(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("remove error status = %d", rr.Code)
	}
}

type teamStoreMock struct {
	*mockStore
	members   []core.TeamMember
	users     []core.User
	listErr   error
	usersErr  error
	removeErr error
	removedID string
}

func (s *teamStoreMock) ListTeamMembers(context.Context, string) ([]core.TeamMember, error) {
	return s.members, s.listErr
}

func (s *teamStoreMock) RemoveTeamMember(_ context.Context, _ string, memberID string) error {
	s.removedID = memberID
	return s.removeErr
}

func (s *teamStoreMock) GetUsersByIDs(context.Context, []string, string) ([]core.User, error) {
	return s.users, s.usersErr
}
