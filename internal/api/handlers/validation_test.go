package handlers

import (
	"fmt"
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

// ─── Sprint 3 follow-up batch: the 7 items cataloged in
// security-report/input-validation-audit.md § Follow-up. Each test
// pins a rule that closes a validation gap flagged by the audit.

func TestRedirectHandler_Create_UnknownStatusCode(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewRedirectHandler(store, newMockBoltStore())

	body := `{"source":"/old","destination":"/new","type":"redirect","status_code":418}`
	req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-redirect status_code, got %d", rr.Code)
	}
}

func TestRedirectHandler_Create_ValidStatusCodes(t *testing.T) {
	for _, code := range []int{301, 302, 307, 308} {
		t.Run(fmt.Sprintf("code_%d", code), func(t *testing.T) {
			store := newMockStore()
			store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
			h := NewRedirectHandler(store, newMockBoltStore())
			body := fmt.Sprintf(`{"source":"/old","destination":"/new","type":"redirect","status_code":%d}`, code)
			req := httptest.NewRequest("POST", "/api/v1/apps/app-1/redirects", strings.NewReader(body))
			req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
			req.SetPathValue("id", "app-1")
			rr := httptest.NewRecorder()
			h.Create(rr, req)
			if rr.Code != http.StatusCreated {
				t.Errorf("expected 201 for status_code=%d, got %d: %s", code, rr.Code, rr.Body.String())
			}
		})
	}
}

// ResponseHeaders: the header-splitting class — if header names can
// contain CRLF or non-token characters, the reverse proxy will emit a
// new header line downstream. Same defect class as sticky_sessions.
func TestResponseHeadersHandler_Update_HeaderNameInjection(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewResponseHeadersHandler(store, newMockBoltStore())

	body := `{"custom":{"X-Evil\r\nSet-Cookie":"sid=attacker"}}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for CRLF in header name, got %d", rr.Code)
	}
}

func TestResponseHeadersHandler_Update_HeaderValueCRLF(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewResponseHeadersHandler(store, newMockBoltStore())

	body := `{"custom":{"X-Evil":"value\r\nSet-Cookie: sid=a"}}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/response-headers", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for CRLF in header value, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update_KeyTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewLabelsHandler(store)

	body := `{"` + strings.Repeat("k", 64) + `":"v"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/labels", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for >63-char label key, got %d", rr.Code)
	}
}

func TestLabelsHandler_Update_ValueTooLong(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewLabelsHandler(store)

	body := `{"k":"` + strings.Repeat("v", 254) + `"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/labels", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for >253-char label value, got %d", rr.Code)
	}
}

func TestDNSRecordHandler_Create_ValueTooLong(t *testing.T) {
	services := core.NewServices()
	services.RegisterDNSProvider("cloudflare", &mockDNS{})
	h := NewDNSRecordHandler(services)

	body := `{"name":"test.example.com","value":"` + strings.Repeat("x", 2049) + `","type":"TXT"}`
	req := httptest.NewRequest("POST", "/api/v1/dns/records?provider=cloudflare", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for >2048-char DNS value, got %d", rr.Code)
	}
}

func TestLogRetentionHandler_Update_MaxSizeTooLarge(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewLogRetentionHandler(store, newMockBoltStore())

	body := `{"max_size_mb":10241,"max_files":5,"driver":"json-file"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for max_size_mb > 10 GB, got %d", rr.Code)
	}
}

func TestLogRetentionHandler_Update_UnknownDriver(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewLogRetentionHandler(store, newMockBoltStore())

	body := `{"max_size_mb":50,"max_files":5,"driver":"splunk"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/log-retention", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown log driver, got %d", rr.Code)
	}
}

func TestErrorPageHandler_Update_PageTooLarge(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewErrorPageHandler(store, newMockBoltStore())

	// 1 MB + 1 byte.
	body := `{"page_502":"` + strings.Repeat("a", (1<<20)+1) + `"}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/error-pages", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for error page > 1 MB, got %d", rr.Code)
	}
}

func TestEnvVarHandler_Update_TooManyVars(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{ID: "app-1", TenantID: "t1", Name: "test"})
	h := NewEnvVarHandler(store)

	parts := make([]string, 501)
	for i := range parts {
		parts[i] = fmt.Sprintf(`{"key":"K%d","value":"v"}`, i)
	}
	body := `{"vars":[` + strings.Join(parts, ",") + `]}`
	req := httptest.NewRequest("PUT", "/api/v1/apps/app-1/env", strings.NewReader(body))
	req = withClaims(req, "u1", "t1", "role_admin", "a@b.com")
	req.SetPathValue("id", "app-1")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for >500 env vars, got %d", rr.Code)
	}
}
