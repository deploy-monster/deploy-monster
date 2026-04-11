package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID_Generated(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id == "" {
			t.Error("expected request ID in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header in response")
	}
}

func TestRequestID_Passthrough(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := GetRequestID(r.Context())
		if id != "custom-123" {
			t.Errorf("expected custom-123, got %q", id)
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Request-ID", "custom-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-Request-ID") != "custom-123" {
		t.Error("should pass through existing request ID")
	}
}

func TestRequestID_TraceparentGenerated(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := GetTraceID(r.Context())
		if traceID == "" {
			t.Error("expected trace ID in context")
		}
		if len(traceID) != 32 {
			t.Errorf("trace ID should be 32 hex chars, got %d", len(traceID))
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	tp := rr.Header().Get("traceparent")
	if tp == "" {
		t.Fatal("expected traceparent header in response")
	}
	// Format: 00-{32hex}-{16hex}-01
	if len(tp) != 55 {
		t.Errorf("traceparent length = %d, want 55", len(tp))
	}
}

func TestRequestID_TraceparentPassthrough(t *testing.T) {
	incoming := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := GetTraceID(r.Context())
		if traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("expected trace ID from incoming header, got %q", traceID)
		}
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("traceparent", incoming)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	tp := rr.Header().Get("traceparent")
	if tp == "" {
		t.Fatal("expected traceparent in response")
	}
	// Should preserve the same trace ID (first 36 chars: "00-" + 32-char trace ID)
	if len(tp) < 36 || tp[3:35] != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("response traceparent should preserve trace ID, got %q", tp)
	}
}

func TestParseTraceparent_Valid(t *testing.T) {
	traceID, parentID := parseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	if traceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("traceID = %q", traceID)
	}
	if parentID != "00f067aa0ba902b7" {
		t.Errorf("parentID = %q", parentID)
	}
}

func TestParseTraceparent_Invalid(t *testing.T) {
	tests := []string{
		"",
		"invalid",
		"01-abc-def-00",                // wrong version
		"00-short-00f067aa0ba902b7-01", // trace too short
		"00-4bf92f3577b34da6a3ce929d0e0e4736-short-01",           // parent too short
		"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-x", // flags too short
	}
	for _, tc := range tests {
		traceID, _ := parseTraceparent(tc)
		if traceID != "" {
			t.Errorf("parseTraceparent(%q) should return empty, got %q", tc, traceID)
		}
	}
}

func TestAPIVersion(t *testing.T) {
	handler := APIVersion("1.0.0")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-DeployMonster-Version") != "1.0.0" {
		t.Error("expected version header")
	}
	if rr.Header().Get("X-API-Version") != "v1" {
		t.Error("expected API version header")
	}
}
