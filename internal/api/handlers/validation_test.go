package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Regression tests for the Sprint 3 input-validation audit. Each case
// pins a rule that must not regress: an attacker (or a careless admin)
// who POSTs an out-of-bounds value should get a 400, not have the
// server quietly store it and trip over it later.

func TestPortHandler_Update_HostPortOutOfRange(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewPortHandler(store)

	body := `[{"container_port":8080,"host_port":70000,"protocol":"tcp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for out-of-range host port, got %d", rr.Code)
	}
}

func TestPortHandler_Update_UnknownProtocol(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewPortHandler(store)

	body := `[{"container_port":8080,"protocol":"sctp"}]`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown protocol, got %d", rr.Code)
	}
}

func TestPortHandler_Update_TooManyMappings(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewPortHandler(store)

	// Build a 101-entry array — one over the cap.
	parts := make([]string, 101)
	for i := range parts {
		parts[i] = `{"container_port":8080,"protocol":"tcp"}`
	}
	body := "[" + strings.Join(parts, ",") + "]"
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/ports", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for >100 port mappings, got %d", rr.Code)
	}
}

func TestHealthCheckHandler_Update_IntervalTooLarge(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewHealthCheckHandler(store)

	body := `{"type":"http","path":"/h","interval":99999}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/healthcheck", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized interval, got %d", rr.Code)
	}
}

func TestHealthCheckHandler_Update_RetriesTooLarge(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewHealthCheckHandler(store)

	body := `{"type":"http","retries":1000}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/healthcheck", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized retries, got %d", rr.Code)
	}
}

func TestHealthCheckHandler_Update_PathTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewHealthCheckHandler(store)

	body := `{"type":"http","path":"` + strings.Repeat("a", 2049) + `"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/healthcheck", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized path, got %d", rr.Code)
	}
}

func TestHealthCheckHandler_Update_PortOutOfRange(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewHealthCheckHandler(store)

	body := `{"type":"tcp","port":70000}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/healthcheck", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for out-of-range port, got %d", rr.Code)
	}
}

// The cookie-name rule is the one rule with a real exploit if it
// regresses: a value like `foo; Path=/; Set-Cookie: sid=attacker`
// would split the Set-Cookie header when the reverse proxy later
// emits the affinity cookie. The regex rejects anything outside the
// RFC 6265 token set.
func TestStickySessionHandler_Update_CookieHeaderInjection(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewStickySessionHandler(store, newMockBoltStore())

	body := `{"enabled":true,"cookie":"foo; Path=/","max_age":3600}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for cookie name with non-token chars, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Update_InvalidSameSite(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewStickySessionHandler(store, newMockBoltStore())

	body := `{"enabled":true,"cookie":"AFFINITY","max_age":3600,"same_site":"banana"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid same_site, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Update_MaxAgeNegative(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewStickySessionHandler(store, newMockBoltStore())

	body := `{"enabled":true,"cookie":"AFFINITY","max_age":-1}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative max_age, got %d", rr.Code)
	}
}

func TestStickySessionHandler_Update_MaxAgeTooLarge(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewStickySessionHandler(store, newMockBoltStore())

	body := `{"enabled":true,"cookie":"AFFINITY","max_age":99999999999}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/sticky-sessions", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for oversized max_age, got %d", rr.Code)
	}
}
