package apierr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCode(t *testing.T) {
	cases := map[int]string{
		http.StatusBadRequest:          "bad_request",
		http.StatusUnauthorized:        "unauthorized",
		http.StatusNotFound:            "not_found",
		http.StatusTooManyRequests:     "rate_limited",
		http.StatusInternalServerError: "internal_error",
		http.StatusTeapot:              "error", // unmapped → fallback
	}
	for status, want := range cases {
		if got := Code(status); got != want {
			t.Errorf("Code(%d) = %q, want %q", status, got, want)
		}
	}
}

func TestWrite_Envelope(t *testing.T) {
	rr := httptest.NewRecorder()
	Write(rr, http.StatusForbidden, "nope")

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		RequestID string `json:"request_id"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Success {
		t.Error("success must be false")
	}
	if resp.Error.Code != "forbidden" || resp.Error.Message != "nope" {
		t.Errorf("error = %+v", resp.Error)
	}
	if resp.RequestID != "" {
		t.Errorf("request_id should be empty when header unset, got %q", resp.RequestID)
	}
}

func TestWrite_IncludesRequestID(t *testing.T) {
	rr := httptest.NewRecorder()
	rr.Header().Set("X-Request-ID", "req-123")
	Write(rr, http.StatusBadRequest, "bad")

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["request_id"] != "req-123" {
		t.Errorf("request_id = %v, want req-123", resp["request_id"])
	}
}
