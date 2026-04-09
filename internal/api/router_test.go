package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// =====================================================
// MODULE IDENTITY
// =====================================================

func TestModule_ID(t *testing.T) {
	m := New()
	if got := m.ID(); got != "api" {
		t.Errorf("ID() = %q, want %q", got, "api")
	}
}

func TestModule_Name(t *testing.T) {
	m := New()
	if got := m.Name(); got != "REST API" {
		t.Errorf("Name() = %q, want %q", got, "REST API")
	}
}

func TestModule_Version(t *testing.T) {
	m := New()
	if got := m.Version(); got != "1.0.0" {
		t.Errorf("Version() = %q, want %q", got, "1.0.0")
	}
}

func TestModule_Dependencies(t *testing.T) {
	m := New()
	deps := m.Dependencies()

	if len(deps) < 2 {
		t.Fatalf("expected at least 2 dependencies, got %d", len(deps))
	}

	foundDB := false
	foundAuth := false
	for _, d := range deps {
		if d == "core.db" {
			foundDB = true
		}
		if d == "core.auth" {
			foundAuth = true
		}
	}
	if !foundDB {
		t.Error("expected dependency on 'core.db'")
	}
	if !foundAuth {
		t.Error("expected dependency on 'core.auth'")
	}
}

func TestModule_Routes(t *testing.T) {
	m := New()
	if m.Routes() != nil {
		t.Error("Routes() should return nil")
	}
}

func TestModule_Events(t *testing.T) {
	m := New()
	if m.Events() != nil {
		t.Error("Events() should return nil")
	}
}

// =====================================================
// MODULE HEALTH
// =====================================================

func TestModule_Health_NoServer(t *testing.T) {
	m := New()
	if got := m.Health(); got != core.HealthDown {
		t.Errorf("Health() without server = %v, want HealthDown", got)
	}
}

func TestModule_Health_WithServer(t *testing.T) {
	m := New()
	m.server = &http.Server{}
	if got := m.Health(); got != core.HealthOK {
		t.Errorf("Health() with server = %v, want HealthOK", got)
	}
}

// =====================================================
// MODULE CONSTRUCTOR
// =====================================================

func TestNew_ReturnsNonNil(t *testing.T) {
	m := New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestModule_ImplementsInterface(t *testing.T) {
	var _ core.Module = (*Module)(nil)
}

// =====================================================
// MODULE STOP
// =====================================================

func TestModule_Stop_NilServer(t *testing.T) {
	m := New()
	err := m.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop() with nil server returned error: %v", err)
	}
}

// =====================================================
// RESPOND FUNCTIONS — additional tests
// =====================================================

func TestRespondOK_ContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondOK(rr, "hello")

	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
}

func TestRespondOK_NilData(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondOK(rr, nil)

	var resp APIResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestRespondOK_ComplexData(t *testing.T) {
	rr := httptest.NewRecorder()
	data := map[string]any{
		"apps":   []string{"app1", "app2"},
		"count":  2,
		"nested": map[string]string{"key": "value"},
	}
	RespondOK(rr, data)

	var resp APIResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Data == nil {
		t.Error("data should not be nil")
	}
}

func TestRespondCreated(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondCreated(rr, map[string]string{"id": "new-123"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}

	var resp APIResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestRespondError_MultipleStatusCodes(t *testing.T) {
	tests := []struct {
		status  int
		code    string
		message string
	}{
		{http.StatusBadRequest, "bad_request", "invalid input"},
		{http.StatusUnauthorized, "unauthorized", "invalid token"},
		{http.StatusForbidden, "forbidden", "access denied"},
		{http.StatusNotFound, "not_found", "resource not found"},
		{http.StatusConflict, "conflict", "already exists"},
		{http.StatusInternalServerError, "internal_error", "unexpected error"},
		{http.StatusServiceUnavailable, "unavailable", "service down"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			rr := httptest.NewRecorder()
			RespondError(rr, tt.status, tt.code, tt.message)

			if rr.Code != tt.status {
				t.Errorf("status = %d, want %d", rr.Code, tt.status)
			}

			var resp APIResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			if resp.Success {
				t.Error("expected success=false")
			}
			if resp.Error == nil {
				t.Fatal("expected error object")
			}
			if resp.Error.Code != tt.code {
				t.Errorf("error code = %q, want %q", resp.Error.Code, tt.code)
			}
			if resp.Error.Message != tt.message {
				t.Errorf("error message = %q, want %q", resp.Error.Message, tt.message)
			}
		})
	}
}

func TestRespondFromError_AllCoreErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"not_found", core.ErrNotFound, http.StatusNotFound, "not_found"},
		{"already_exists", core.ErrAlreadyExists, http.StatusConflict, "already_exists"},
		{"unauthorized", core.ErrUnauthorized, http.StatusUnauthorized, "unauthorized"},
		{"forbidden", core.ErrForbidden, http.StatusForbidden, "forbidden"},
		{"quota_exceeded", core.ErrQuotaExceeded, http.StatusForbidden, "quota_exceeded"},
		{"invalid_input", core.ErrInvalidInput, http.StatusBadRequest, "invalid_input"},
		{"invalid_token", core.ErrInvalidToken, http.StatusUnauthorized, "invalid_token"},
		{"unknown", fmt.Errorf("some random error"), http.StatusInternalServerError, "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			RespondFromError(rr, tt.err)

			if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rr.Code, tt.wantStatus)
			}

			var resp APIResponse
			json.Unmarshal(rr.Body.Bytes(), &resp)
			if resp.Error == nil {
				t.Fatal("expected error object")
			}
			if resp.Error.Code != tt.wantCode {
				t.Errorf("error code = %q, want %q", resp.Error.Code, tt.wantCode)
			}
		})
	}
}

// =====================================================
// RESPOND PAGINATED — additional cases
// =====================================================

func TestRespondPaginated_FirstPage(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondPaginated(rr, []string{"a", "b", "c"}, 1, 3, 10)

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Meta == nil {
		t.Fatal("expected meta")
	}
	if resp.Meta.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Meta.Page)
	}
	if resp.Meta.PerPage != 3 {
		t.Errorf("per_page = %d, want 3", resp.Meta.PerPage)
	}
	if resp.Meta.Total != 10 {
		t.Errorf("total = %d, want 10", resp.Meta.Total)
	}
	if resp.Meta.TotalPages != 4 {
		t.Errorf("total_pages = %d, want 4 (ceil(10/3))", resp.Meta.TotalPages)
	}
}

func TestRespondPaginated_LastPage(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondPaginated(rr, []string{"z"}, 5, 5, 21)

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Meta.TotalPages != 5 {
		t.Errorf("total_pages = %d, want 5 (ceil(21/5))", resp.Meta.TotalPages)
	}
}

func TestRespondPaginated_SinglePage(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondPaginated(rr, []string{"only"}, 1, 10, 1)

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp.Meta.TotalPages != 1 {
		t.Errorf("total_pages = %d, want 1", resp.Meta.TotalPages)
	}
	if resp.Meta.Total != 1 {
		t.Errorf("total = %d, want 1", resp.Meta.Total)
	}
}

func TestRespondPaginated_EmptyData(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondPaginated(rr, []string{}, 1, 20, 0)

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if !resp.Success {
		t.Error("expected success=true even with empty data")
	}
	if resp.Meta.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Meta.Total)
	}
}

// =====================================================
// HELPER FUNCTIONS
// =====================================================

func TestWriteJSON(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, map[string]string{"hello": "world"})

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["hello"] != "world" {
		t.Errorf("hello = %q, want %q", body["hello"], "world")
	}
}

func TestWriteError(t *testing.T) {
	rr := httptest.NewRecorder()
	writeError(rr, http.StatusBadRequest, "something went wrong")

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["error"] != "something went wrong" {
		t.Errorf("error = %q, want %q", body["error"], "something went wrong")
	}
}

func TestParseJSON_ValidBody(t *testing.T) {
	body := strings.NewReader(`{"name":"test","value":42}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)

	var dest struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	err := parseJSON(req, &dest)
	if err != nil {
		t.Fatalf("parseJSON error: %v", err)
	}
	if dest.Name != "test" {
		t.Errorf("name = %q, want %q", dest.Name, "test")
	}
	if dest.Value != 42 {
		t.Errorf("value = %d, want 42", dest.Value)
	}
}

func TestParseJSON_InvalidJSON(t *testing.T) {
	body := strings.NewReader(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)

	var dest struct{ Name string }
	err := parseJSON(req, &dest)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseJSON_NilBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/test", nil)
	req.Body = nil

	var dest struct{ Name string }
	err := parseJSON(req, &dest)
	if err == nil {
		t.Fatal("expected error for nil body")
	}
}

func TestParseJSON_UnknownFields(t *testing.T) {
	body := strings.NewReader(`{"name":"test","unknown_field":"value"}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)

	var dest struct {
		Name string `json:"name"`
	}

	err := parseJSON(req, &dest)
	if err == nil {
		t.Fatal("expected error for unknown fields (DisallowUnknownFields)")
	}
}

// =====================================================
// PAGINATION PARSING
// =====================================================

func TestParsePagination_Defaults(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	p := parsePagination(req)

	if p.Page != 1 {
		t.Errorf("page = %d, want 1", p.Page)
	}
	if p.PerPage != 20 {
		t.Errorf("per_page = %d, want 20", p.PerPage)
	}
	if p.Offset != 0 {
		t.Errorf("offset = %d, want 0", p.Offset)
	}
}

func TestParsePagination_ValidParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?page=3&per_page=50", nil)
	p := parsePagination(req)

	if p.Page != 3 {
		t.Errorf("page = %d, want 3", p.Page)
	}
	if p.PerPage != 50 {
		t.Errorf("per_page = %d, want 50", p.PerPage)
	}
	if p.Offset != 100 { // (3-1)*50
		t.Errorf("offset = %d, want 100", p.Offset)
	}
}

func TestParsePagination_NegativePage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?page=-5", nil)
	p := parsePagination(req)

	if p.Page != 1 {
		t.Errorf("page = %d, want 1 (clamped)", p.Page)
	}
}

func TestParsePagination_PerPageTooLarge(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?per_page=500", nil)
	p := parsePagination(req)

	if p.PerPage != 20 {
		t.Errorf("per_page = %d, want 20 (clamped for >100)", p.PerPage)
	}
}

func TestParsePagination_PerPageZero(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?per_page=0", nil)
	p := parsePagination(req)

	if p.PerPage != 20 {
		t.Errorf("per_page = %d, want 20 (default for 0)", p.PerPage)
	}
}

func TestParsePagination_InvalidStrings(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?page=abc&per_page=xyz", nil)
	p := parsePagination(req)

	if p.Page != 1 {
		t.Errorf("page = %d, want 1 (default for invalid)", p.Page)
	}
	if p.PerPage != 20 {
		t.Errorf("per_page = %d, want 20 (default for invalid)", p.PerPage)
	}
}

func TestParsePagination_OffsetCalculation(t *testing.T) {
	tests := []struct {
		page    string
		perPage string
		want    int
	}{
		{"1", "10", 0},
		{"2", "10", 10},
		{"3", "10", 20},
		{"1", "20", 0},
		{"5", "20", 80},
		{"10", "100", 900},
	}

	for _, tt := range tests {
		t.Run("page_"+tt.page+"_per_"+tt.perPage, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test?page="+tt.page+"&per_page="+tt.perPage, nil)
			p := parsePagination(req)
			if p.Offset != tt.want {
				t.Errorf("offset = %d, want %d", p.Offset, tt.want)
			}
		})
	}
}

// =====================================================
// REAL IP EXTRACTION
// =====================================================

func TestRealIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Real-IP", "203.0.113.50")

	ip := realIP(req)
	if ip != "203.0.113.50" {
		t.Errorf("realIP = %q, want %q", ip, "203.0.113.50")
	}
}

func TestRealIP_XForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Forwarded-For", "198.51.100.178, 203.0.113.50")

	ip := realIP(req)
	if ip != "198.51.100.178, 203.0.113.50" {
		t.Errorf("realIP = %q, want %q", ip, "198.51.100.178, 203.0.113.50")
	}
}

func TestRealIP_RemoteAddr(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"

	ip := realIP(req)
	if ip != "192.168.1.100:12345" {
		t.Errorf("realIP = %q, want %q", ip, "192.168.1.100:12345")
	}
}

func TestRealIP_XRealIPPriority(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Real-IP", "10.0.0.1")
	req.Header.Set("X-Forwarded-For", "10.0.0.2")
	req.RemoteAddr = "10.0.0.3"

	ip := realIP(req)
	// X-Real-IP takes priority
	if ip != "10.0.0.1" {
		t.Errorf("realIP = %q, want %q (X-Real-IP should take priority)", ip, "10.0.0.1")
	}
}

// =====================================================
// SPA HANDLER
// =====================================================

func TestNewSPAHandler_ReturnsNonNil(t *testing.T) {
	handler := newSPAHandler()
	if handler == nil {
		t.Fatal("newSPAHandler returned nil")
	}
}

func TestSPAHandler_ServesContent(t *testing.T) {
	handler := newSPAHandler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	body := rr.Body.String()
	if len(body) == 0 {
		t.Error("body should not be empty")
	}
}

func TestSPAHandler_FallbackForUnknownPaths(t *testing.T) {
	handler := newSPAHandler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/some/client/route", nil)

	handler.ServeHTTP(rr, req)

	// SPA handler should serve content (either the SPA or a placeholder)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (SPA fallback)", rr.Code)
	}
}

// =====================================================
// PAGINATED RESPONSE STRUCT
// =====================================================

func TestPaginatedResponse_Fields(t *testing.T) {
	resp := PaginatedResponse{
		Data:       []string{"a", "b"},
		Total:      100,
		Page:       2,
		PerPage:    20,
		TotalPages: 5,
	}

	if resp.Total != 100 {
		t.Errorf("Total = %d, want 100", resp.Total)
	}
	if resp.Page != 2 {
		t.Errorf("Page = %d, want 2", resp.Page)
	}
	if resp.PerPage != 20 {
		t.Errorf("PerPage = %d, want 20", resp.PerPage)
	}
	if resp.TotalPages != 5 {
		t.Errorf("TotalPages = %d, want 5", resp.TotalPages)
	}
}

// =====================================================
// API RESPONSE STRUCT
// =====================================================

func TestAPIResponse_JSONSerialization(t *testing.T) {
	resp := APIResponse{
		Success: true,
		Data:    map[string]string{"key": "value"},
		Meta: &APIMeta{
			RequestID:  "req-123",
			Page:       1,
			PerPage:    20,
			Total:      100,
			TotalPages: 5,
		},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded APIResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if !decoded.Success {
		t.Error("expected success=true")
	}
	if decoded.Meta == nil {
		t.Fatal("expected meta")
	}
	if decoded.Meta.RequestID != "req-123" {
		t.Errorf("request_id = %q, want %q", decoded.Meta.RequestID, "req-123")
	}
}

func TestAPIError_JSONSerialization(t *testing.T) {
	apiErr := APIError{
		Code:    "validation_error",
		Message: "email is required",
		Details: map[string]string{"field": "email"},
	}

	data, err := json.Marshal(apiErr)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded APIError
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Code != "validation_error" {
		t.Errorf("code = %q, want %q", decoded.Code, "validation_error")
	}
	if decoded.Message != "email is required" {
		t.Errorf("message = %q, want %q", decoded.Message, "email is required")
	}
}

// =====================================================
// HEALTH ENDPOINT — smoke test via mux
// =====================================================

func TestHealthEndpoint_Responds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		RespondOK(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, r *http.Request) {
		RespondOK(w, map[string]string{"status": "ok"})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Test /health
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health status = %d, want 200", resp.StatusCode)
	}

	// Test /api/v1/health
	resp2, err := http.Get(srv.URL + "/api/v1/health")
	if err != nil {
		t.Fatalf("request error: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("/api/v1/health status = %d, want 200", resp2.StatusCode)
	}
}

// =====================================================
// RESPOND FROM ERROR — wrapped errors
// =====================================================

func TestRespondFromError_WrappedNotFound(t *testing.T) {
	rr := httptest.NewRecorder()
	err := fmt.Errorf("app lookup: %w", core.ErrNotFound)
	RespondFromError(rr, err)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Error.Code != "not_found" {
		t.Errorf("error code = %q, want %q", resp.Error.Code, "not_found")
	}
}

func TestRespondFromError_WrappedForbidden(t *testing.T) {
	rr := httptest.NewRecorder()
	err := fmt.Errorf("access check: %w", core.ErrForbidden)
	RespondFromError(rr, err)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}
}

func TestRespondFromError_WrappedQuotaExceeded(t *testing.T) {
	rr := httptest.NewRecorder()
	err := fmt.Errorf("create app: %w", core.ErrQuotaExceeded)
	RespondFromError(rr, err)

	if rr.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Error.Code != "quota_exceeded" {
		t.Errorf("error code = %q, want %q", resp.Error.Code, "quota_exceeded")
	}
}

// =====================================================
// WRITE JSON — additional cases
// =====================================================

func TestWriteJSON_StatusCodes(t *testing.T) {
	codes := []int{
		http.StatusOK,
		http.StatusCreated,
		http.StatusAccepted,
		http.StatusNoContent,
		http.StatusBadRequest,
		http.StatusNotFound,
		http.StatusInternalServerError,
	}

	for _, code := range codes {
		t.Run(fmt.Sprintf("status_%d", code), func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeJSON(rr, code, map[string]string{"status": "test"})

			if rr.Code != code {
				t.Errorf("status = %d, want %d", rr.Code, code)
			}
			if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
		})
	}
}

func TestWriteJSON_Array(t *testing.T) {
	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, []string{"a", "b", "c"})

	var arr []string
	json.Unmarshal(rr.Body.Bytes(), &arr)
	if len(arr) != 3 {
		t.Errorf("expected 3 elements, got %d", len(arr))
	}
}

func TestWriteJSON_NestedStruct(t *testing.T) {
	type inner struct {
		Value int `json:"value"`
	}
	type outer struct {
		Name  string `json:"name"`
		Inner inner  `json:"inner"`
	}

	rr := httptest.NewRecorder()
	writeJSON(rr, http.StatusOK, outer{Name: "test", Inner: inner{Value: 42}})

	var result outer
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Name != "test" {
		t.Errorf("Name = %q, want %q", result.Name, "test")
	}
	if result.Inner.Value != 42 {
		t.Errorf("Inner.Value = %d, want 42", result.Inner.Value)
	}
}

// =====================================================
// WRITE ERROR — message preserved
// =====================================================

func TestWriteError_MultipleMessages(t *testing.T) {
	messages := []string{
		"not found",
		"invalid email format",
		"authentication required",
		"rate limit exceeded",
		"",
	}

	for _, msg := range messages {
		t.Run(msg, func(t *testing.T) {
			rr := httptest.NewRecorder()
			writeError(rr, http.StatusBadRequest, msg)

			var body map[string]string
			json.Unmarshal(rr.Body.Bytes(), &body)
			if body["error"] != msg {
				t.Errorf("error = %q, want %q", body["error"], msg)
			}
		})
	}
}

// =====================================================
// PARSE JSON — edge cases
// =====================================================

func TestParseJSON_EmptyObject(t *testing.T) {
	body := strings.NewReader(`{}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)

	var dest struct {
		Name string `json:"name"`
	}
	err := parseJSON(req, &dest)
	if err != nil {
		t.Fatalf("parseJSON error: %v", err)
	}
	if dest.Name != "" {
		t.Errorf("name should be empty, got %q", dest.Name)
	}
}

func TestParseJSON_NestedObject(t *testing.T) {
	body := strings.NewReader(`{"name":"test","config":{"port":8080,"debug":true}}`)
	req := httptest.NewRequest(http.MethodPost, "/test", body)

	var dest struct {
		Name   string `json:"name"`
		Config struct {
			Port  int  `json:"port"`
			Debug bool `json:"debug"`
		} `json:"config"`
	}
	err := parseJSON(req, &dest)
	if err != nil {
		t.Fatalf("parseJSON error: %v", err)
	}
	if dest.Config.Port != 8080 {
		t.Errorf("port = %d, want 8080", dest.Config.Port)
	}
	if !dest.Config.Debug {
		t.Error("debug should be true")
	}
}

func TestParseJSON_EmptyBody(t *testing.T) {
	body := strings.NewReader("")
	req := httptest.NewRequest(http.MethodPost, "/test", body)

	var dest struct{ Name string }
	err := parseJSON(req, &dest)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

// =====================================================
// RESPOND CREATED — various data types
// =====================================================

func TestRespondCreated_WithID(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondCreated(rr, map[string]string{"id": "app-xyz-123", "status": "created"})

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestRespondCreated_NilData(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondCreated(rr, nil)

	if rr.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", rr.Code)
	}

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if !resp.Success {
		t.Error("expected success=true")
	}
}

// =====================================================
// RESPOND PAGINATED — total pages calculation
// =====================================================

func TestRespondPaginated_TotalPagesCalc(t *testing.T) {
	tests := []struct {
		total     int
		perPage   int
		wantPages int
	}{
		{0, 20, 0},
		{1, 20, 1},
		{20, 20, 1},
		{21, 20, 2},
		{100, 10, 10},
		{101, 10, 11},
		{99, 100, 1},
		{1, 1, 1},
		{5, 3, 2},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("total_%d_per_%d", tt.total, tt.perPage), func(t *testing.T) {
			rr := httptest.NewRecorder()
			RespondPaginated(rr, nil, 1, tt.perPage, tt.total)

			var resp APIResponse
			json.Unmarshal(rr.Body.Bytes(), &resp)
			if resp.Meta.TotalPages != tt.wantPages {
				t.Errorf("total_pages = %d, want %d", resp.Meta.TotalPages, tt.wantPages)
			}
		})
	}
}

// =====================================================
// RESPOND OK — list data
// =====================================================

func TestRespondOK_ListData(t *testing.T) {
	rr := httptest.NewRecorder()
	apps := []map[string]string{
		{"id": "app-1", "name": "Web App"},
		{"id": "app-2", "name": "API Server"},
	}
	RespondOK(rr, apps)

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if !resp.Success {
		t.Error("expected success=true")
	}
	if resp.Data == nil {
		t.Error("data should contain the list")
	}
}

// =====================================================
// SPA HANDLER — specific file paths
// =====================================================

func TestSPAHandler_FaviconRequest(t *testing.T) {
	handler := newSPAHandler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/favicon.svg", nil)

	handler.ServeHTTP(rr, req)

	// Should serve the favicon file (exists in static/)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for favicon", rr.Code)
	}
}

func TestSPAHandler_AssetsPath(t *testing.T) {
	handler := newSPAHandler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/assets/nonexistent.js", nil)

	handler.ServeHTTP(rr, req)

	// Non-existent asset should fall back to index.html (SPA behavior)
	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (SPA fallback)", rr.Code)
	}
}

func TestSPAHandler_DeepRoute(t *testing.T) {
	handler := newSPAHandler()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/apps/123/settings/general", nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 for deep SPA route", rr.Code)
	}
}

// =====================================================
// RESPOND FROM ERROR — all core error types
// =====================================================

func TestRespondFromError_InvalidInput(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondFromError(rr, core.ErrInvalidInput)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rr.Code)
	}
}

func TestRespondFromError_InvalidToken(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondFromError(rr, core.ErrInvalidToken)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

func TestRespondFromError_AlreadyExists(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondFromError(rr, core.ErrAlreadyExists)

	if rr.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rr.Code)
	}
}

func TestRespondFromError_Unauthorized(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondFromError(rr, core.ErrUnauthorized)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
}

// =====================================================
// MODULE STOP — with real server
// =====================================================

func TestModule_Stop_WithServer(t *testing.T) {
	m := New()
	m.logger = slog.Default()
	m.server = &http.Server{Addr: "127.0.0.1:0"}

	// Start the server so Shutdown actually does something
	go m.server.ListenAndServe()

	err := m.Stop(t.Context())
	if err != nil {
		t.Errorf("Stop returned error: %v", err)
	}
}

// =====================================================
// API RESPONSE — error response has no data
// =====================================================

func TestRespondError_NoData(t *testing.T) {
	rr := httptest.NewRecorder()
	RespondError(rr, http.StatusBadRequest, "bad_request", "invalid")

	var resp APIResponse
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Data != nil {
		t.Error("error response should not have data")
	}
	if resp.Meta != nil {
		t.Error("error response should not have meta")
	}
}

// =====================================================
// PAGINATION STRUCT
// =====================================================

func TestPagination_Fields(t *testing.T) {
	p := Pagination{Page: 3, PerPage: 25, Offset: 50}
	if p.Page != 3 {
		t.Errorf("Page = %d, want 3", p.Page)
	}
	if p.PerPage != 25 {
		t.Errorf("PerPage = %d, want 25", p.PerPage)
	}
	if p.Offset != 50 {
		t.Errorf("Offset = %d, want 50", p.Offset)
	}
}

// =====================================================
// PARSE PAGINATION — boundary values
// =====================================================

func TestParsePagination_ExactlyMaxPerPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?per_page=100", nil)
	p := parsePagination(req)
	if p.PerPage != 100 {
		t.Errorf("per_page = %d, want 100", p.PerPage)
	}
}

func TestParsePagination_PerPage101(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?per_page=101", nil)
	p := parsePagination(req)
	if p.PerPage != 20 {
		t.Errorf("per_page = %d, want 20 (default for > 100)", p.PerPage)
	}
}

func TestParsePagination_NegativePerPage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test?per_page=-1", nil)
	p := parsePagination(req)
	if p.PerPage != 20 {
		t.Errorf("per_page = %d, want 20 (default for negative)", p.PerPage)
	}
}

// =====================================================
// MOCK STORE — minimal implementation for router tests
// =====================================================

type testStore struct {
	core.Store // embed interface; unimplemented methods panic
}

func (s *testStore) Close() error                              { return nil }
func (s *testStore) Ping(_ context.Context) error              { return nil }
func (s *testStore) CountUsers(_ context.Context) (int, error) { return 1, nil }
func (s *testStore) CreateTenantWithDefaults(_ context.Context, _, _ string) (string, error) {
	return "tenant-test", nil
}
func (s *testStore) CreateUserWithMembership(_ context.Context, _, _, _, _, _, _ string) (string, error) {
	return "user-test", nil
}
func (s *testStore) CreateTenant(_ context.Context, _ *core.Tenant) error        { return nil }
func (s *testStore) GetTenant(_ context.Context, _ string) (*core.Tenant, error) { return nil, nil }
func (s *testStore) GetTenantBySlug(_ context.Context, _ string) (*core.Tenant, error) {
	return nil, nil
}
func (s *testStore) UpdateTenant(_ context.Context, _ *core.Tenant) error           { return nil }
func (s *testStore) DeleteTenant(_ context.Context, _ string) error                 { return nil }
func (s *testStore) CreateUser(_ context.Context, _ *core.User) error               { return nil }
func (s *testStore) GetUser(_ context.Context, _ string) (*core.User, error)        { return nil, nil }
func (s *testStore) GetUserByEmail(_ context.Context, _ string) (*core.User, error) { return nil, nil }
func (s *testStore) UpdateUser(_ context.Context, _ *core.User) error               { return nil }
func (s *testStore) UpdatePassword(_ context.Context, _, _ string) error            { return nil }
func (s *testStore) UpdateLastLogin(_ context.Context, _ string) error              { return nil }
func (s *testStore) CreateApp(_ context.Context, _ *core.Application) error         { return nil }
func (s *testStore) GetApp(_ context.Context, _ string) (*core.Application, error)  { return nil, nil }
func (s *testStore) UpdateApp(_ context.Context, _ *core.Application) error         { return nil }
func (s *testStore) ListAppsByTenant(_ context.Context, _ string, _, _ int) ([]core.Application, int, error) {
	return nil, 0, nil
}
func (s *testStore) ListAppsByProject(_ context.Context, _ string) ([]core.Application, error) {
	return nil, nil
}
func (s *testStore) UpdateAppStatus(_ context.Context, _, _ string) error         { return nil }
func (s *testStore) DeleteApp(_ context.Context, _ string) error                  { return nil }
func (s *testStore) CreateDeployment(_ context.Context, _ *core.Deployment) error { return nil }
func (s *testStore) GetLatestDeployment(_ context.Context, _ string) (*core.Deployment, error) {
	return nil, nil
}
func (s *testStore) ListDeploymentsByApp(_ context.Context, _ string, _ int) ([]core.Deployment, error) {
	return nil, nil
}
func (s *testStore) GetNextDeployVersion(_ context.Context, _ string) (int, error) { return 1, nil }
func (s *testStore) CreateDomain(_ context.Context, _ *core.Domain) error          { return nil }
func (s *testStore) GetDomainByFQDN(_ context.Context, _ string) (*core.Domain, error) {
	return nil, nil
}
func (s *testStore) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return nil, nil
}
func (s *testStore) DeleteDomain(_ context.Context, _ string) error                { return nil }
func (s *testStore) ListAllDomains(_ context.Context) ([]core.Domain, error)       { return nil, nil }
func (s *testStore) CreateProject(_ context.Context, _ *core.Project) error        { return nil }
func (s *testStore) GetProject(_ context.Context, _ string) (*core.Project, error) { return nil, nil }
func (s *testStore) ListProjectsByTenant(_ context.Context, _ string) ([]core.Project, error) {
	return nil, nil
}
func (s *testStore) DeleteProject(_ context.Context, _ string) error         { return nil }
func (s *testStore) GetRole(_ context.Context, _ string) (*core.Role, error) { return nil, nil }
func (s *testStore) GetUserMembership(_ context.Context, _ string) (*core.TeamMember, error) {
	return nil, nil
}
func (s *testStore) ListRoles(_ context.Context, _ string) ([]core.Role, error) { return nil, nil }
func (s *testStore) CreateAuditLog(_ context.Context, _ *core.AuditEntry) error { return nil }
func (s *testStore) ListAuditLogs(_ context.Context, _ string, _, _ int) ([]core.AuditEntry, int, error) {
	return nil, 0, nil
}

// testBoltStore is a minimal BoltStorer for router construction tests.
type testBoltStore struct{}

func (b *testBoltStore) Set(_, _ string, _ any, _ int64) error { return nil }
func (b *testBoltStore) BatchSet(_ []core.BoltBatchItem) error { return nil }
func (b *testBoltStore) Get(_, _ string, _ any) error          { return fmt.Errorf("key not found") }
func (b *testBoltStore) Delete(_, _ string) error              { return nil }
func (b *testBoltStore) List(_ string) ([]string, error)       { return nil, nil }
func (b *testBoltStore) Close() error                          { return nil }
func (b *testBoltStore) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, fmt.Errorf("not found")
}
func (b *testBoltStore) GetWebhookSecret(_ string) (string, error) {
	return "", fmt.Errorf("not found")
}

// testCoreSetup creates a minimal Core + auth.Module for router tests.
func testCoreSetup(t *testing.T) (*core.Core, *auth.Module) {
	t.Helper()
	store := &testStore{}
	registry := core.NewRegistry()
	events := core.NewEventBus(nil)

	c := &core.Core{
		Registry: registry,
		Events:   events,
		Logger:   slog.Default(),
		Store:    store,
		Build:    core.BuildInfo{Version: "0.1.0-test"},
		Config:   &core.Config{Server: core.ServerConfig{SecretKey: "test-secret-key-32chars-for-jwt!"}},
		Services: core.NewServices(),
		DB:       &core.Database{Bolt: &testBoltStore{}},
	}

	authMod := auth.New()
	if err := authMod.Init(context.Background(), c); err != nil {
		t.Fatalf("auth.Init: %v", err)
	}
	return c, authMod
}

// =====================================================
// NEW ROUTER — full construction test
// =====================================================

func TestNewRouter_CreatesNonNil(t *testing.T) {
	c, authMod := testCoreSetup(t)
	r := NewRouter(c, authMod, c.Store)
	if r == nil {
		t.Fatal("NewRouter returned nil")
	}
	if r.mux == nil {
		t.Error("mux should be initialized")
	}
	if r.core != c {
		t.Error("core reference should be set")
	}
}

func TestNewRouter_HandlerNotNil(t *testing.T) {
	c, authMod := testCoreSetup(t)
	r := NewRouter(c, authMod, c.Store)
	h := r.Handler()
	if h == nil {
		t.Fatal("Handler() returned nil")
	}
}

func TestNewRouter_HealthEndpointRegistered(t *testing.T) {
	c, authMod := testCoreSetup(t)
	r := NewRouter(c, authMod, c.Store)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("health status = %d, want 200", rr.Code)
	}

	var body map[string]any
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
}

func TestNewRouter_APIHealthEndpoint(t *testing.T) {
	c, authMod := testCoreSetup(t)
	r := NewRouter(c, authMod, c.Store)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	r.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("/api/v1/health status = %d, want 200", rr.Code)
	}
}

func TestNewRouter_SPAFallback(t *testing.T) {
	c, authMod := testCoreSetup(t)
	r := NewRouter(c, authMod, c.Store)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.mux.ServeHTTP(rr, req)

	// SPA handler should respond (either real UI or placeholder)
	if rr.Code != http.StatusOK {
		t.Errorf("/ status = %d, want 200", rr.Code)
	}
}

func TestNewRouter_ProtectedEndpointReturns401(t *testing.T) {
	c, authMod := testCoreSetup(t)
	r := NewRouter(c, authMod, c.Store)

	// Access a protected endpoint without auth
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	r.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("/api/v1/apps without auth: status = %d, want 401", rr.Code)
	}
}

func TestNewRouter_OpenAPIEndpoint(t *testing.T) {
	c, authMod := testCoreSetup(t)
	r := NewRouter(c, authMod, c.Store)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil)
	r.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("/api/v1/openapi.json status = %d, want 200", rr.Code)
	}
}

func TestNewRouter_HandlerWithMiddleware(t *testing.T) {
	c, authMod := testCoreSetup(t)
	r := NewRouter(c, authMod, c.Store)
	handler := r.Handler()

	// The handler has middleware (CORS, RequestID, etc.)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("health via Handler() = %d, want 200", rr.Code)
	}
}

// =====================================================
// HANDLE HEALTH — direct test via Router struct
// =====================================================

func TestRouter_HandleHealth_AllOK(t *testing.T) {
	registry := core.NewRegistry()
	c := &core.Core{
		Registry: registry,
		Build:    core.BuildInfo{Version: "0.1.0-test"},
	}

	r := &Router{core: c}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rr.Code)
	}

	var body map[string]any
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["version"] != "0.1.0-test" {
		t.Errorf("version = %v, want 0.1.0-test", body["version"])
	}
}

func TestRouter_HandleHealth_ContentType(t *testing.T) {
	registry := core.NewRegistry()
	c := &core.Core{
		Registry: registry,
		Build:    core.BuildInfo{Version: "1.0.0"},
	}

	r := &Router{core: c}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.handleHealth(rr, req)

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestRouter_HandleHealth_HasModulesField(t *testing.T) {
	registry := core.NewRegistry()
	c := &core.Core{
		Registry: registry,
		Build:    core.BuildInfo{Version: "0.1.0"},
	}

	r := &Router{core: c}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.handleHealth(rr, req)

	var body map[string]any
	json.Unmarshal(rr.Body.Bytes(), &body)

	modules, ok := body["modules"]
	if !ok {
		t.Fatal("response should contain 'modules' field")
	}
	// With empty registry, modules should be an empty map
	modulesMap, ok := modules.(map[string]any)
	if !ok {
		t.Fatal("modules should be a map")
	}
	if len(modulesMap) != 0 {
		t.Errorf("expected 0 modules, got %d", len(modulesMap))
	}
}
