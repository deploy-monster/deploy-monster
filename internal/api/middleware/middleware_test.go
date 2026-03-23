package middleware

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
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
	handler := CORS("*")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
