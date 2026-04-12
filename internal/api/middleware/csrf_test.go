package middleware

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCSRFProtect_SafeMethods_PassThrough(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			handler := CSRFProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(method, "/api/v1/apps", nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("%s: expected 200, got %d", method, rec.Code)
			}
		})
	}
}

func TestCSRFProtect_BearerToken_SkipsCSRF(t *testing.T) {
	handler := CSRFProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("Authorization", "Bearer some-jwt-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for Bearer auth, got %d", rec.Code)
	}
}

func TestCSRFProtect_APIKey_SkipsCSRF(t *testing.T) {
	handler := CSRFProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("X-API-Key", "dm_testkey123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for API key auth, got %d", rec.Code)
	}
}

func TestCSRFProtect_NoCookie_PassesThrough(t *testing.T) {
	handler := CSRFProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 when no CSRF cookie, got %d", rec.Code)
	}
}

func TestCSRFProtect_CookieWithoutHeader_Rejects(t *testing.T) {
	handler := CSRFProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.AddCookie(&http.Cookie{Name: "dm_csrf", Value: "valid-token"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 when CSRF cookie present but no header, got %d", rec.Code)
	}
}

func TestCSRFProtect_MismatchToken_Rejects(t *testing.T) {
	handler := CSRFProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.AddCookie(&http.Cookie{Name: "dm_csrf", Value: "token-a"})
	req.Header.Set("X-CSRF-Token", "token-b")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for mismatched CSRF tokens, got %d", rec.Code)
	}
}

func TestCSRFProtect_MatchingToken_Passes(t *testing.T) {
	handler := CSRFProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.AddCookie(&http.Cookie{Name: "dm_csrf", Value: "valid-token"})
	req.Header.Set("X-CSRF-Token", "valid-token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for matching CSRF tokens, got %d", rec.Code)
	}
}

func TestCSRFProtect_EmptyCookieValue_PassesThrough(t *testing.T) {
	handler := CSRFProtect(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.AddCookie(&http.Cookie{Name: "dm_csrf", Value: ""})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for empty CSRF cookie, got %d", rec.Code)
	}
}

func TestSetCSRFCookie_SetsCookie(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "https://example.test/api/v1/auth/login", nil)
	req.TLS = &tls.ConnectionState{}
	SetCSRFCookie(rec, req)

	cookies := rec.Result().Cookies()
	var found *http.Cookie
	for _, c := range cookies {
		if c.Name == "dm_csrf" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected dm_csrf cookie to be set")
	}
	if found.Value == "" {
		t.Error("CSRF cookie value should not be empty")
	}
	if len(found.Value) != 32 { // hex-encoded 16 bytes
		t.Errorf("expected 32-char hex token, got %d chars", len(found.Value))
	}
	if found.HttpOnly {
		t.Error("CSRF cookie should NOT be httpOnly (JS must read it)")
	}
	if !found.Secure {
		t.Error("CSRF cookie should be Secure when request is over TLS")
	}
	if found.MaxAge != 86400 {
		t.Errorf("expected MaxAge 86400, got %d", found.MaxAge)
	}
}

// TestSetCSRFCookie_PlainHTTPNotSecure guards the dev/CI path: when the
// backend listens on plain HTTP (e.g. Playwright E2E against
// http://localhost:8443), the Secure flag must be omitted so Chromium
// actually stores the cookie instead of silently dropping it.
func TestSetCSRFCookie_PlainHTTPNotSecure(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://localhost:8443/api/v1/auth/login", nil)
	SetCSRFCookie(rec, req)

	var found *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "dm_csrf" {
			found = c
			break
		}
	}
	if found == nil {
		t.Fatal("expected dm_csrf cookie to be set")
	}
	if found.Secure {
		t.Error("CSRF cookie must NOT be Secure over plain HTTP")
	}
}

func TestSetCSRFCookie_UniqueTokens(t *testing.T) {
	rec1 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "https://example.test/api/v1/auth/login", nil)
	req.TLS = &tls.ConnectionState{}
	SetCSRFCookie(rec1, req)
	rec2 := httptest.NewRecorder()
	SetCSRFCookie(rec2, req)

	var token1, token2 string
	for _, c := range rec1.Result().Cookies() {
		if c.Name == "dm_csrf" {
			token1 = c.Value
		}
	}
	for _, c := range rec2.Result().Cookies() {
		if c.Name == "dm_csrf" {
			token2 = c.Value
		}
	}
	if token1 == token2 {
		t.Error("two calls should produce different tokens")
	}
}
