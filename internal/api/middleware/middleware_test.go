package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestRecovery(t *testing.T) {
	handler := Recovery(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	// Should not panic
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 after panic, got %d", rr.Code)
	}
}

func TestCORS(t *testing.T) {
	handler := CORS("*", false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Regular request
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}

	// OPTIONS preflight
	req2 := httptest.NewRequest("OPTIONS", "/", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if rr2.Code != http.StatusNoContent {
		t.Errorf("OPTIONS should return 204, got %d", rr2.Code)
	}

	if rr2.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("missing allowed methods header")
	}
	allowedHeaders := rr2.Header().Get("Access-Control-Allow-Headers")
	for _, header := range []string{"X-CSRF-Token", "Idempotency-Key"} {
		if !strings.Contains(allowedHeaders, header) {
			t.Fatalf("allowed headers %q missing %s", allowedHeaders, header)
		}
	}
}

func TestChain(t *testing.T) {
	var order []string

	m1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m1-before")
			next.ServeHTTP(w, r)
			order = append(order, "m1-after")
		})
	}

	m2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "m2-before")
			next.ServeHTTP(w, r)
			order = append(order, "m2-after")
		})
	}

	handler := Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "handler")
	}), m1, m2)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// m1 wraps m2 wraps handler
	expected := []string{"m1-before", "m2-before", "handler", "m2-after", "m1-after"}
	if len(order) != len(expected) {
		t.Fatalf("order length = %d, want %d", len(order), len(expected))
	}
	for i, v := range expected {
		if order[i] != v {
			t.Errorf("order[%d] = %q, want %q", i, order[i], v)
		}
	}
}

func TestAuditPath(t *testing.T) {
	tests := []struct {
		method       string
		path         string
		wantResource string
		wantAction   string
	}{
		{"POST", "/api/v1/apps", "apps", "create"},
		{"DELETE", "/api/v1/apps/abc123", "apps", "delete"},
		{"PATCH", "/api/v1/apps/abc123", "apps", "update"},
		{"POST", "/api/v1/apps/abc123/restart", "apps", "restart"},
	}

	for _, tt := range tests {
		res, _, action := parseAuditPath(tt.method, tt.path)
		if res != tt.wantResource {
			t.Errorf("%s %s: resource = %q, want %q", tt.method, tt.path, res, tt.wantResource)
		}
		if action != tt.wantAction {
			t.Errorf("%s %s: action = %q, want %q", tt.method, tt.path, action, tt.wantAction)
		}
	}
}

func TestAPIVersion(t *testing.T) {
	handler := APIVersion("1.2.3")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("code = %d, want 202", rr.Code)
	}
	if rr.Header().Get("X-DeployMonster-Version") != "1.2.3" {
		t.Fatalf("missing deploymonster version header")
	}
	if rr.Header().Get("X-API-Version") != "v1" {
		t.Fatalf("missing API version header")
	}
}

func TestIPAllowlist(t *testing.T) {
	allowed := NewIPAllowlist([]string{" 127.0.0.0/8 ", "", "bad-cidr"}, slog.Default())
	handler := allowed.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("allowed code = %d, want 204", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("blocked code = %d, want 403", rr.Code)
	}

	open := NewIPAllowlist(nil, nil).Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "not-an-ip"
	rr = httptest.NewRecorder()
	open.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("empty allowlist should pass, got %d", rr.Code)
	}
}

func TestTracingNoTracerAndParentID(t *testing.T) {
	ctx := WithParentID(context.Background(), "parent")
	if got, _ := ctx.Value(parentIDKey{}).(string); got != "parent" {
		t.Fatalf("parent id = %q", got)
	}

	startCtx, end := StartSpan(ctx, &core.Core{})
	if startCtx != ctx {
		t.Fatal("StartSpan without tracer should return original context")
	}
	end()

	var seen bool
	handler := Tracing(&core.Core{})(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		seen = true
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if !seen {
		t.Fatal("tracing middleware did not invoke next handler")
	}
}
