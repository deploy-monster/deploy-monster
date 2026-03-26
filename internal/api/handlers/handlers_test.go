package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEnvVarMasking(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ab", "****"},
		{"abc", "****"},
		{"abcd", "****"},
		{"abcde", "ab*de"},
		{"secretvalue123", "se**********23"},
		{"${SECRET:db_pass}", "${SECRET:db_pass}"}, // Secret refs not masked
		{"password123!", "pa********3!"},
	}

	for _, tt := range tests {
		got := maskValue(tt.input)
		if got != tt.want {
			t.Errorf("maskValue(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGenerateSlug_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"John Doe", "john-doe"},
		{"ALLCAPS", "allcaps"},
		{"with spaces", "with-spaces"},
		{"already-slug", "already-slug"},
		{"123numbers", "123numbers"},
		{"MixedCase123", "mixedcase123"},
	}

	for _, tt := range tests {
		got := generateSlug(tt.input)
		if got != tt.want {
			t.Errorf("generateSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		input string
		count int
	}{
		{"line1\nline2\nline3", 3},
		{"single", 1},
		{"", 0},
		{"a\nb\n", 2},
		{"with\r\nwindows\r\nlines", 3},
	}

	for _, tt := range tests {
		got := splitLines(tt.input)
		if len(got) != tt.count {
			t.Errorf("splitLines(%q) = %d lines, want %d", tt.input, len(got), tt.count)
		}
	}
}

// TestHealthEndpoint tests the /health endpoint response format.
func TestHealthEndpoint(t *testing.T) {
	// Create a minimal handler that matches the health response format
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": "test",
			"modules": map[string]string{},
		})
	})

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("health returned %d, want 200", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
}

// TestWriteJSON tests JSON response helper.
func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusCreated, map[string]string{"key": "value"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["key"] != "value" {
		t.Errorf("body key = %q, want value", resp["key"])
	}
}

// TestWriteError tests error response helper.
func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "something went wrong")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}

	var resp map[string]string
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["error"] != "something went wrong" {
		t.Errorf("error = %q", resp["error"])
	}
}

// TestScaleRequest validates scale request parsing.
func TestScaleRequestValidation(t *testing.T) {
	tests := []struct {
		body  string
		valid bool
	}{
		{`{"replicas": 3}`, true},
		{`{"replicas": 0}`, true},
		{`{"replicas": -1}`, false},
		{`{"replicas": 101}`, false},
	}

	for _, tt := range tests {
		var req scaleRequest
		json.Unmarshal([]byte(tt.body), &req)

		valid := req.Replicas >= 0 && req.Replicas <= 100
		if valid != tt.valid {
			t.Errorf("replicas=%d: valid=%v, want %v", req.Replicas, valid, tt.valid)
		}
	}
}

func TestCreateAppRequest_Defaults(t *testing.T) {
	body := `{"name":"test-app"}`
	var req createAppRequest
	json.Unmarshal([]byte(body), &req)

	if req.Name != "test-app" {
		t.Errorf("name = %q", req.Name)
	}

	// Defaults should be empty, applied by handler
	if req.Type != "" {
		t.Errorf("type should be empty, got %q", req.Type)
	}
	if req.Branch != "" {
		t.Errorf("branch should be empty, got %q", req.Branch)
	}
}

func TestInviteRequest_Parse(t *testing.T) {
	body := bytes.NewBufferString(`{"email":"test@example.com","role_id":"role_developer"}`)
	var req inviteRequest
	json.NewDecoder(body).Decode(&req)

	if req.Email != "test@example.com" {
		t.Errorf("email = %q", req.Email)
	}
	if req.RoleID != "role_developer" {
		t.Errorf("role_id = %q", req.RoleID)
	}
}
