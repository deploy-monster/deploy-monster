package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Public mode (allowedOrigins == "*"): wildcard Allow-Origin, no
// Allow-Credentials. This matches the fetch-spec rule against combining
// wildcard with credentials.
func TestCORS_WildcardOrigin(t *testing.T) {
	handler := CORS("*", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("expected wildcard origin, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("wildcard mode must not emit Allow-Credentials, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods header")
	}
	if got := rr.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("expected Access-Control-Allow-Headers header")
	}
	if got := rr.Header().Get("Access-Control-Expose-Headers"); got == "" {
		t.Error("expected Access-Control-Expose-Headers header")
	}
	if got := rr.Header().Get("Access-Control-Max-Age"); got != "86400" {
		t.Errorf("expected max-age 86400, got %q", got)
	}
}

// Allowlist mode: request Origin matches a configured origin → echo it
// back and permit credentials.
func TestCORS_SpecificAllowedOrigin(t *testing.T) {
	handler := CORS("https://app.deploy.monster,https://admin.deploy.monster", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Origin", "https://app.deploy.monster")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://app.deploy.monster" {
		t.Errorf("expected echoed origin, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("allowlist mode must emit Allow-Credentials=true, got %q", got)
	}
}

// Allowlist mode: request Origin NOT in the list → no Allow-Origin header
// is emitted, which the browser interprets as CORS denial.
func TestCORS_DisallowedOrigin(t *testing.T) {
	handler := CORS("https://app.deploy.monster", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("disallowed origin must not receive Allow-Origin, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("disallowed origin must not receive Allow-Credentials, got %q", got)
	}
}

// Allowlist mode without an Origin header (e.g. same-origin request or
// a non-browser client): no Allow-Origin header. This also prevents the
// CORS-002 class of empty-origin bypass.
func TestCORS_NoOriginHeader(t *testing.T) {
	handler := CORS("https://app.deploy.monster", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	// No Origin header set.
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("missing Origin must not yield Allow-Origin, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("expected Access-Control-Allow-Methods even without Origin")
	}
}

// Empty CORSOrigins config → public mode fallback (wildcard, no creds).
// This preserves backward compatibility for self-hosted users who never
// configure CORSOrigins.
func TestCORS_EmptyConfigFallsToPublic(t *testing.T) {
	handler := CORS("", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("empty config must default to wildcard, got %q", got)
	}
	if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Errorf("public-mode fallback must not emit Allow-Credentials, got %q", got)
	}
}

// OPTIONS preflight short-circuits to 204 without invoking the handler.
func TestCORS_PreflightOptions(t *testing.T) {
	handlerCalled := false
	handler := CORS("*", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/v1/apps", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("OPTIONS should return 204, got %d", rr.Code)
	}
	if handlerCalled {
		t.Error("handler should not be called for OPTIONS preflight")
	}
}

func TestCORS_NonPreflightPassesThrough(t *testing.T) {
	handlerCalled := false
	handler := CORS("*", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if !handlerCalled {
		t.Error("handler should be called for non-OPTIONS request")
	}
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// Regression: verify we never emit wildcard+credentials combo, which
// browsers reject and is flagged as CORS-001 in the security report.
func TestCORS_NeverWildcardWithCredentials(t *testing.T) {
	cases := []struct {
		name    string
		origins string
		origin  string
	}{
		{"public_mode_with_origin", "*", "https://example.com"},
		{"public_mode_no_origin", "*", ""},
		{"empty_config", "", "https://example.com"},
		{"allowlist_unknown_origin", "https://app.deploy.monster", "https://evil.com"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := CORS(tc.origins, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			ao := rr.Header().Get("Access-Control-Allow-Origin")
			ac := rr.Header().Get("Access-Control-Allow-Credentials")
			if ao == "*" && ac != "" {
				t.Errorf("wildcard + credentials is forbidden: got ao=%q ac=%q", ao, ac)
			}
		})
	}
}
