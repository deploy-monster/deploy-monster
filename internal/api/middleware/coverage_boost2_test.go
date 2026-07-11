package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// =============================================================================
// statusWriter — Flush and Unwrap (previously 0.0%)
// =============================================================================

func TestStatusWriter_Flush(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	// Should not panic when underlying writer is a flusher (httptest.ResponseRecorder is)
	sw.Flush()
}

func TestStatusWriter_Unwrap(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
	uw := sw.Unwrap()
	if uw != w {
		t.Error("Unwrap() should return the underlying ResponseWriter")
	}
}

// =============================================================================
// RealIPNoXFF (previously 0.0%)
// =============================================================================

func TestRealIPNoXFF(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.168.1.1:12345"
	ip := RealIPNoXFF(r)
	if ip == "" {
		t.Error("RealIPNoXFF() returned empty string")
	}
}

// =============================================================================
// validateIP — empty string and invalid IP
// =============================================================================

func TestValidateIP_Empty(t *testing.T) {
	if got := validateIP(""); got != "" {
		t.Errorf("validateIP('') = %q, want ''", got)
	}
}

func TestValidateIP_Invalid(t *testing.T) {
	if got := validateIP("not-an-ip"); got != "" {
		t.Errorf("validateIP('not-an-ip') = %q, want ''", got)
	}
}

// =============================================================================
// validateIPPermissive — empty string
// =============================================================================

func TestValidateIPPermissive_Empty(t *testing.T) {
	if got := validateIPPermissive(""); got != "" {
		t.Errorf("validateIPPermissive('') = %q, want ''", got)
	}
}

func TestValidateIPPermissive_Invalid(t *testing.T) {
	if got := validateIPPermissive("bad"); got != "" {
		t.Errorf("validateIPPermissive('bad') = %q, want ''", got)
	}
}

// =============================================================================
// stripPort — with IPv4 and IPv6 addresses
// =============================================================================

func TestStripPort_IPv4(t *testing.T) {
	if got := stripPort("1.2.3.4:8080"); got != "1.2.3.4" {
		t.Errorf("stripPort = %q, want '1.2.3.4'", got)
	}
}

func TestStripPort_NoPort(t *testing.T) {
	if got := stripPort("1.2.3.4"); got != "1.2.3.4" {
		t.Errorf("stripPort = %q, want '1.2.3.4'", got)
	}
}

// =============================================================================
// idempotencyAuthScope — covers authorization header and anonymous paths
// =============================================================================

func TestIdempotencyAuthScope_Authorization(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Set("Authorization", "Bearer token123")
	scope := idempotencyAuthScope(r)
	if scope != "authorization:Bearer token123" {
		t.Errorf("scope = %q, want 'authorization:Bearer token123'", scope)
	}
}

func TestIdempotencyAuthScope_Anonymous(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	scope := idempotencyAuthScope(r)
	if scope != "anonymous" {
		t.Errorf("scope = %q, want 'anonymous'", scope)
	}
}

// =============================================================================
// idempotencyRecorder Write — covers the wroteHeader path
// =============================================================================

func TestIdempotencyRecorder_Write(t *testing.T) {
	w := httptest.NewRecorder()
	ir := &idempotencyRecorder{
		ResponseWriter: w,
		status:         http.StatusOK,
		body:           &bytes.Buffer{},
	}
	n, err := ir.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != 5 {
		t.Errorf("Write returned %d, want 5", n)
	}
	if !ir.wroteHeader {
		t.Error("wroteHeader should be true after Write")
	}
}

// =============================================================================
// groupPath — metric path grouping
// =============================================================================

func TestGroupPath_ShortPath(t *testing.T) {
	got := groupPath("/api/v1/apps")
	if got != "/api/v1/apps" {
		t.Errorf("groupPath = %q, want '/api/v1/apps'", got)
	}
}

func TestGroupPath_LongID(t *testing.T) {
	got := groupPath("/api/v1/apps/abcdef12345678901234567890")
	if got != "/api/v1/apps/{id}" {
		t.Errorf("groupPath = %q, want '/api/v1/apps/{id}'", got)
	}
}

// =============================================================================
// GlobalRateLimiter — cleanup goroutine recover path
// =============================================================================

func TestGlobalRateLimiter_Stop_Idempotent(t *testing.T) {
	rl := NewGlobalRateLimiterWithLogger(10, time.Minute, nil)
	rl.Stop()
	// Second stop must not panic
	rl.Stop()
}

// =============================================================================
// TenantRateLimiter — log() method with nil logger
// =============================================================================

func TestTenantRateLimiter_Log_NilLogger(t *testing.T) {
	trl := &TenantRateLimiter{}
	l := trl.log()
	if l == nil {
		t.Fatal("log() returned nil")
	}
}

func TestTenantRateLimiter_Log_WithLogger(t *testing.T) {
	trl := &TenantRateLimiter{logger: slog.Default()}
	l := trl.log()
	if l == nil {
		t.Fatal("log() returned nil")
	}
}

// =============================================================================
// IPAllowlist Middleware — unparseable IP
// =============================================================================

func TestIPAllowlist_Middleware_UnparseableIP(t *testing.T) {
	al := NewIPAllowlist([]string{"10.0.0.0/8"}, slog.Default())
	handler := al.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	// Set a RemoteAddr that produces an unparseable IP
	r.RemoteAddr = "not-an-ip"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", w.Code)
	}
}

func TestIPAllowlist_Middleware_NoCIDRs(t *testing.T) {
	al := NewIPAllowlist(nil, slog.Default())
	handler := al.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// =============================================================================
// AuthRateLimiter SetLogger — nil logger
// =============================================================================

func TestAuthRateLimiter_SetLogger_Nil(t *testing.T) {
	rl := NewAuthRateLimiter(nil, 10, time.Minute, "test")
	logger := rl.logger
	rl.SetLogger(nil)
	if rl.logger != logger {
		t.Error("SetLogger(nil) should not change the logger")
	}
}

// =============================================================================
// safeClientIP — X-Real-IP and X-Forwarded-For
// =============================================================================

func TestSafeClientIP_XRealIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Real-IP", "10.0.0.1")
	r.RemoteAddr = "192.168.1.1:12345"
	ip := safeClientIP(r, false)
	// X-Real-IP with permissive validation should return it
	if ip != "10.0.0.1" {
		t.Errorf("safeClientIP = %q, want '10.0.0.1'", ip)
	}
}

func TestSafeClientIP_XForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	r.RemoteAddr = "192.168.1.1:12345"
	ip := safeClientIP(r, true)
	// First XFF IP should be returned when trustXFF is true and it's a public IP
	if ip == "" {
		t.Error("safeClientIP with XFF returned empty")
	}
}

// =============================================================================
// parseCORSOrigins — empty string
// =============================================================================

func TestParseCORSOrigins_Empty(t *testing.T) {
	origins := parseCORSOrigins("")
	if origins != nil {
		t.Errorf("parseCORSOrigins('') = %v, want nil", origins)
	}
}

// =============================================================================
// csrfCookieValue — covers both cookie names
// =============================================================================

func TestCSRFCookieValue_DevCookie(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: csrfDevCookieName, Value: "dev-token"})
	val := csrfCookieValue(r)
	if val != "dev-token" {
		t.Errorf("csrfCookieValue = %q, want 'dev-token'", val)
	}
}

func TestCSRFCookieValue_NoCookie(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	val := csrfCookieValue(r)
	if val != "" {
		t.Errorf("csrfCookieValue = %q, want ''", val)
	}
}

// =============================================================================
// idempotencyRecorder WriteHeader — prevents double write
// =============================================================================

func TestIdempotencyRecorder_WriteHeader_Double(t *testing.T) {
	w := httptest.NewRecorder()
	ir := &idempotencyRecorder{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
	ir.WriteHeader(http.StatusOK)
	ir.WriteHeader(http.StatusInternalServerError) // second write should be ignored
	if ir.status != http.StatusOK {
		t.Errorf("status after double WriteHeader = %d, want 200", ir.status)
	}
}

// =============================================================================
// global_ratelimit — Stop with nil stopCh and idle cleanup
// =============================================================================

func TestGlobalRateLimiter_Stop_NilStopCh(t *testing.T) {
	rl := &GlobalRateLimiter{
		rate:    10,
		window:  time.Minute,
		clients: make(map[string]*rateLimitWindow),
		logger:  slog.Default(),
	}
	// stopCh is nil — Stop should handle it gracefully
	rl.Stop()
}

// =============================================================================
// CORS — origin not in allowlist
// =============================================================================

func TestCORS_NotAllowedOrigin(t *testing.T) {
	mw := CORS("https://allowed.com", false)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	// No Allow-Origin header should be set
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("Allow-Origin should not be set for non-allowed origin")
	}
}
