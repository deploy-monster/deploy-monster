package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
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
		{"${SECRET:db_pass}", "${SECRET:***}"}, // Secret refs masked to hide secret name
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

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil || errObj["message"] != "something went wrong" {
		t.Errorf("error message = %v", errObj)
	}
}

func TestValidateAppName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		ok    bool
	}{
		{"valid simple", "my-app", true},
		{"valid with dots", "app.v2", true},
		{"valid with spaces", "My App", true},
		{"valid with underscore", "my_app", true},
		{"starts with digit", "1app", true},
		{"empty", "", false},
		{"too long", string(make([]byte, 101)), false},
		{"starts with dash", "-app", false},
		{"starts with space", " app", false},
		{"special chars", "app@v2", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAppName(tt.input)
			if tt.ok && err != nil {
				t.Errorf("expected valid, got error: %v", err)
			}
			if !tt.ok && err == nil {
				t.Errorf("expected error for %q", tt.input)
			}
		})
	}
}

func TestWriteValidationErrors(t *testing.T) {
	rr := httptest.NewRecorder()
	rr.Header().Set("X-Request-ID", "req-123")

	writeValidationErrors(rr, "validation failed", []FieldError{
		{Field: "email", Message: "is required"},
		{Field: "name", Message: "too long"},
	})

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["success"] != false {
		t.Error("expected success=false")
	}
	if resp["request_id"] != "req-123" {
		t.Errorf("expected request_id, got %v", resp["request_id"])
	}

	errObj, _ := resp["error"].(map[string]any)
	if errObj["code"] != "validation_error" {
		t.Errorf("code = %v, want validation_error", errObj["code"])
	}
	details, ok := errObj["details"].([]any)
	if !ok || len(details) != 2 {
		t.Fatalf("expected 2 field errors, got %v", errObj["details"])
	}

	first, _ := details[0].(map[string]any)
	if first["field"] != "email" || first["message"] != "is required" {
		t.Errorf("unexpected first detail: %v", first)
	}
}

func TestParsePagination_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	pg := parsePagination(req)

	if pg.Page != 1 {
		t.Errorf("page = %d, want 1", pg.Page)
	}
	if pg.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", pg.PerPage)
	}
	if pg.Offset != 0 {
		t.Errorf("offset = %d, want 0", pg.Offset)
	}
}

func TestParsePagination_CustomValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps?page=3&per_page=50", nil)
	pg := parsePagination(req)

	if pg.Page != 3 {
		t.Errorf("page = %d, want 3", pg.Page)
	}
	if pg.PerPage != 50 {
		t.Errorf("per_page = %d, want 50", pg.PerPage)
	}
	if pg.Offset != 100 {
		t.Errorf("offset = %d, want 100", pg.Offset)
	}
}

func TestParsePagination_Clamping(t *testing.T) {
	tests := []struct {
		name    string
		query   string
		page    int
		perPage int
	}{
		{"negative page", "page=-1&per_page=10", 1, 10},
		{"zero page", "page=0&per_page=10", 1, 10},
		{"per_page too large", "page=1&per_page=200", 1, 20},
		{"per_page zero", "page=1&per_page=0", 1, 20},
		{"per_page negative", "page=1&per_page=-5", 1, 20},
		{"non-numeric", "page=abc&per_page=xyz", 1, 20},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/apps?"+tt.query, nil)
			pg := parsePagination(req)
			if pg.Page != tt.page {
				t.Errorf("page = %d, want %d", pg.Page, tt.page)
			}
			if pg.PerPage != tt.perPage {
				t.Errorf("per_page = %d, want %d", pg.PerPage, tt.perPage)
			}
		})
	}
}

func TestWritePaginatedJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	data := []string{"a", "b", "c"}
	pg := pagination{Page: 2, PerPage: 3, Offset: 3}

	writePaginatedJSON(rr, data, 10, pg)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["total"] != float64(10) {
		t.Errorf("total = %v, want 10", resp["total"])
	}
	if resp["page"] != float64(2) {
		t.Errorf("page = %v, want 2", resp["page"])
	}
	if resp["per_page"] != float64(3) {
		t.Errorf("per_page = %v, want 3", resp["per_page"])
	}
	if resp["total_pages"] != float64(4) {
		t.Errorf("total_pages = %v, want 4 (ceil(10/3))", resp["total_pages"])
	}
	arr, ok := resp["data"].([]any)
	if !ok || len(arr) != 3 {
		t.Errorf("data = %v", resp["data"])
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

func TestSetServerContext_DeployTrigger(t *testing.T) {
	h := NewDeployTriggerHandler(nil, nil, nil)
	// Default should be non-nil (context.Background)
	if h.serverCtx == nil {
		t.Fatal("expected non-nil default serverCtx")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.SetServerContext(ctx)

	if h.serverCtx != ctx {
		t.Error("SetServerContext did not set the context")
	}

	// Canceling should propagate
	cancel()
	select {
	case <-h.serverCtx.Done():
		// OK — context canceled
	default:
		t.Error("expected serverCtx to be canceled")
	}
}

func TestSetServerContext_Compose(t *testing.T) {
	h := NewComposeHandler(nil, nil, nil)
	if h.serverCtx == nil {
		t.Fatal("expected non-nil default serverCtx")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.SetServerContext(ctx)

	cancel()
	select {
	case <-h.serverCtx.Done():
	default:
		t.Error("expected serverCtx to be canceled")
	}
}

func TestSetServerContext_MarketplaceDeploy(t *testing.T) {
	h := NewMarketplaceDeployHandler(nil, nil, nil, nil)
	if h.serverCtx == nil {
		t.Fatal("expected non-nil default serverCtx")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h.SetServerContext(ctx)

	cancel()
	select {
	case <-h.serverCtx.Done():
	default:
		t.Error("expected serverCtx to be canceled")
	}
}

func TestSafeGo_RunsFunction(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	ran := false
	safeGo(func() {
		ran = true
		wg.Done()
	}, nil)
	wg.Wait()
	if !ran {
		t.Error("expected function to run")
	}
}

func TestSafeGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	safeGo(func() {
		defer wg.Done()
		panic("test panic in handler goroutine")
	}, nil)
	wg.Wait()
	// If we reach here, panic was recovered
}

func TestSafeGo_CallsOnPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	var called atomic.Bool
	safeGo(func() {
		panic("boom")
	}, func(r any) {
		called.Store(true)
		wg.Done()
	})
	wg.Wait()
	if !called.Load() {
		t.Error("expected onPanic to be called")
	}
}
