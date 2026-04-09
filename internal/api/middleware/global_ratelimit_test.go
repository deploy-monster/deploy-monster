package middleware

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWriteErrorJSON_Format(t *testing.T) {
	rr := httptest.NewRecorder()
	rr.Header().Set("X-Request-ID", "req-abc")

	writeErrorJSON(rr, http.StatusUnauthorized, "invalid token")

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp["success"] != false {
		t.Error("expected success=false")
	}
	if resp["request_id"] != "req-abc" {
		t.Errorf("request_id = %v, want req-abc", resp["request_id"])
	}

	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field missing or not object: %v", resp["error"])
	}
	if errObj["code"] != "unauthorized" {
		t.Errorf("error.code = %v, want unauthorized", errObj["code"])
	}
	if errObj["message"] != "invalid token" {
		t.Errorf("error.message = %v, want 'invalid token'", errObj["message"])
	}
}

func TestWriteErrorJSON_NoRequestID(t *testing.T) {
	rr := httptest.NewRecorder()
	writeErrorJSON(rr, http.StatusForbidden, "forbidden")

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if _, exists := resp["request_id"]; exists {
		t.Error("request_id should not be present when header is empty")
	}
	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "forbidden" {
		t.Errorf("error.code = %v, want forbidden", errObj["code"])
	}
}

func TestWriteErrorJSON_UnknownStatus(t *testing.T) {
	rr := httptest.NewRecorder()
	writeErrorJSON(rr, http.StatusGone, "resource gone")

	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)

	errObj := resp["error"].(map[string]any)
	if errObj["code"] != "error" {
		t.Errorf("error.code = %v, want 'error' for unknown status", errObj["code"])
	}
}

func TestGlobalRateLimiter_AllowsUnderLimit(t *testing.T) {
	rl := NewGlobalRateLimiter(5, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want 200", i+1, rr.Code)
		}
	}
}

func TestGlobalRateLimiter_BlocksOverLimit(t *testing.T) {
	rl := NewGlobalRateLimiter(3, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use up all 3 requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.2.3.4:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// 4th request should be blocked
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "1.2.3.4:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want 429", rr.Code)
	}

	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header")
	}

	// Verify structured JSON error
	var resp map[string]any
	json.Unmarshal(rr.Body.Bytes(), &resp)
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured error, got %v", resp)
	}
	if errObj["code"] != "rate_limited" {
		t.Errorf("error.code = %v, want rate_limited", errObj["code"])
	}
}

func TestGlobalRateLimiter_DifferentIPsIndependent(t *testing.T) {
	rl := NewGlobalRateLimiter(2, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust IP-A
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "1.1.1.1:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// IP-B should still work
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "2.2.2.2:5678"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("different IP should not be rate limited, got %d", rr.Code)
	}
}

func TestGlobalRateLimiter_RateLimitHeaders(t *testing.T) {
	rl := NewGlobalRateLimiter(10, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "5.5.5.5:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("X-RateLimit-Limit") != "10" {
		t.Errorf("X-RateLimit-Limit = %q, want 10", rr.Header().Get("X-RateLimit-Limit"))
	}
	if rr.Header().Get("X-RateLimit-Remaining") != "9" {
		t.Errorf("X-RateLimit-Remaining = %q, want 9", rr.Header().Get("X-RateLimit-Remaining"))
	}
	if rr.Header().Get("X-RateLimit-Reset") == "" {
		t.Error("expected X-RateLimit-Reset header")
	}
}

func TestGlobalRateLimiter_WindowResets(t *testing.T) {
	rl := NewGlobalRateLimiter(2, 50*time.Millisecond)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Exhaust limit
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "3.3.3.3:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	// Should be blocked
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "3.3.3.3:1234"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("after window reset: status = %d, want 200", rr.Code)
	}
}

func TestGlobalRateLimiter_XForwardedFor(t *testing.T) {
	rl := NewGlobalRateLimiter(1, time.Minute)
	defer rl.Stop()

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request from forwarded IP
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.RemoteAddr = "127.0.0.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", rr.Code)
	}

	// Second request from same forwarded IP — should be blocked
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "10.0.0.1")
	req.RemoteAddr = "127.0.0.1:1234"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("same forwarded IP should be rate limited, got %d", rr.Code)
	}
}

func TestGlobalRateLimiter_Stats(t *testing.T) {
	rl := NewGlobalRateLimiter(10, time.Minute)
	defer rl.Stop()

	stats := rl.Stats()
	if stats.ActiveClients != 0 {
		t.Errorf("initial ActiveClients = %d, want 0", stats.ActiveClients)
	}
	if stats.Rate != 10 {
		t.Errorf("Rate = %d, want 10", stats.Rate)
	}

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Make requests from 3 different IPs
	for _, ip := range []string{"1.1.1.1:1", "2.2.2.2:2", "3.3.3.3:3"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = ip
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}

	stats = rl.Stats()
	if stats.ActiveClients != 3 {
		t.Errorf("ActiveClients = %d, want 3", stats.ActiveClients)
	}
}

func TestAPIMetrics_TracksBytesOut(t *testing.T) {
	m := NewAPIMetrics()
	handler := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("hello world")) // 11 bytes
	}))

	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if m.totalBytesOut.Load() != 11 {
		t.Errorf("totalBytesOut = %d, want 11", m.totalBytesOut.Load())
	}

	// Second request
	req = httptest.NewRequest("GET", "/api/v1/apps", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if m.totalBytesOut.Load() != 22 {
		t.Errorf("totalBytesOut = %d, want 22", m.totalBytesOut.Load())
	}
}

func TestRequestLogger_IncludesBytes(t *testing.T) {
	handler := RequestLogger(slog.Default())(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Just verify it doesn't panic and response is correct
	if rr.Body.String() != "test" {
		t.Errorf("body = %q, want 'test'", rr.Body.String())
	}
}
