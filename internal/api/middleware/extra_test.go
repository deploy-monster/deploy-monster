package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── RequestLogger tests ─────────────────────────────────────────────────────

func TestRequestLogger_LogsRequest(t *testing.T) {
	handler := RequestLogger(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequestLogger_CapturesStatus(t *testing.T) {
	handler := RequestLogger(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestRequestLogger_WithXRealIP(t *testing.T) {
	handler := RequestLogger(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("X-Real-IP", "192.168.1.100")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRequestLogger_WithXForwardedFor(t *testing.T) {
	handler := RequestLogger(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// ── realIP tests ────────────────────────────────────────────────────────────

func TestRealIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "1.2.3.4")

	ip := realIP(req)
	if ip != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %q", ip)
	}
}

func TestRealIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-For", "5.6.7.8")

	ip := realIP(req)
	if ip != "5.6.7.8" {
		t.Errorf("expected 5.6.7.8, got %q", ip)
	}
}

func TestRealIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	// httptest.NewRequest sets RemoteAddr to "192.0.2.1:1234"

	ip := realIP(req)
	if ip == "" {
		t.Error("expected non-empty IP from RemoteAddr")
	}
}

func TestRealIP_XRealIPTakesPrecedence(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Real-IP", "1.1.1.1")
	req.Header.Set("X-Forwarded-For", "2.2.2.2")

	ip := realIP(req)
	if ip != "1.1.1.1" {
		t.Errorf("X-Real-IP should take precedence, got %q", ip)
	}
}

// ── statusWriter tests ──────────────────────────────────────────────────────

func TestStatusWriter_CapturesStatusCode(t *testing.T) {
	rr := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rr, status: http.StatusOK}

	sw.WriteHeader(http.StatusCreated)

	if sw.status != http.StatusCreated {
		t.Errorf("expected captured status 201, got %d", sw.status)
	}
	if rr.Code != http.StatusCreated {
		t.Errorf("expected underlying recorder 201, got %d", rr.Code)
	}
}

func TestStatusWriter_DefaultStatus(t *testing.T) {
	rr := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: rr, status: http.StatusOK}

	// Without calling WriteHeader, status should remain at the default
	if sw.status != http.StatusOK {
		t.Errorf("expected default status 200, got %d", sw.status)
	}
}

// ── IsDraining test ─────────────────────────────────────────────────────────

func TestGracefulShutdown_IsDraining(t *testing.T) {
	gs := NewGracefulShutdown()

	if gs.IsDraining() {
		t.Error("should not be draining initially")
	}

	gs.StartDraining()

	if !gs.IsDraining() {
		t.Error("should be draining after StartDraining()")
	}
}

// ── parseAuditPath additional edge case ─────────────────────────────────────

func TestParseAuditPath_UnknownMethod(t *testing.T) {
	_, _, action := parseAuditPath("CUSTOM", "/api/v1/apps")
	if action != "CUSTOM" {
		t.Errorf("expected action 'CUSTOM', got %q", action)
	}
}

func TestParseAuditPath_PATCH(t *testing.T) {
	res, id, action := parseAuditPath(http.MethodPatch, "/api/v1/apps/abc123")
	if res != "apps" {
		t.Errorf("expected resource 'apps', got %q", res)
	}
	if id != "abc123" {
		t.Errorf("expected ID 'abc123', got %q", id)
	}
	if action != "update" {
		t.Errorf("expected action 'update', got %q", action)
	}
}
