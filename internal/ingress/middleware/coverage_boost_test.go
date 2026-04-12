package middleware

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// ═══════════════════════════════════════════════════════════════════════════════
// Compress — response body integrity
// ═══════════════════════════════════════════════════════════════════════════════

func TestCompress_ResponseBody(t *testing.T) {
	largeBody := strings.Repeat("Hello, World! This is a test. ", 100)

	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeBody))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected Content-Encoding: gzip")
	}

	// Verify Content-Length is removed
	if rr.Header().Get("Content-Length") != "" {
		t.Error("Content-Length should be removed for gzip responses")
	}

	// Decompress and verify
	reader, err := gzip.NewReader(rr.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read decompressed: %v", err)
	}
	if string(decompressed) != largeBody {
		t.Errorf("decompressed body mismatch: got %d bytes, want %d bytes", len(decompressed), len(largeBody))
	}
}

func TestCompress_NoGzipAccept(t *testing.T) {
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("plain response"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	// No Accept-Encoding header
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Body.String() != "plain response" {
		t.Errorf("body = %q, want 'plain response'", rr.Body.String())
	}
	if rr.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not set gzip encoding when not accepted")
	}
}

func TestCompress_OtherEncoding(t *testing.T) {
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("plain"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "br") // brotli only, no gzip
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Body.String() != "plain" {
		t.Errorf("body = %q, want 'plain'", rr.Body.String())
	}
}

func TestCompress_MultipleAcceptEncodings(t *testing.T) {
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Encoding", "deflate, gzip, br")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Error("should apply gzip when it appears in Accept-Encoding")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RateLimiter — window reset
// ═══════════════════════════════════════════════════════════════════════════════

func TestRateLimiter_WindowReset(t *testing.T) {
	// Use a very short window so we can test reset
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     2,
		window:   50 * time.Millisecond,
	}

	// First 2 requests pass
	if !rl.allow("1.1.1.1") {
		t.Error("request 1 should be allowed")
	}
	if !rl.allow("1.1.1.1") {
		t.Error("request 2 should be allowed")
	}
	// 3rd request is blocked
	if rl.allow("1.1.1.1") {
		t.Error("request 3 should be blocked")
	}

	// Wait for window to reset
	time.Sleep(60 * time.Millisecond)

	// Should be allowed again after window reset
	if !rl.allow("1.1.1.1") {
		t.Error("request after window reset should be allowed")
	}
}

func TestRateLimiter_Cleanup(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     10,
		window:   10 * time.Millisecond,
	}

	rl.allow("old-ip")
	time.Sleep(30 * time.Millisecond) // Wait for 2x window

	rl.cleanup()

	rl.mu.Lock()
	remaining := len(rl.visitors)
	rl.mu.Unlock()

	if remaining != 0 {
		t.Errorf("expected 0 visitors after cleanup, got %d", remaining)
	}
}

func TestRateLimiter_Cleanup_KeepsFreshEntries(t *testing.T) {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     10,
		window:   1 * time.Hour,
	}

	rl.allow("fresh-ip")
	rl.cleanup()

	rl.mu.Lock()
	remaining := len(rl.visitors)
	rl.mu.Unlock()

	if remaining != 1 {
		t.Errorf("expected 1 visitor (fresh), got %d", remaining)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// extractIP — edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestExtractIP_XForwardedFor_SingleIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	req.RemoteAddr = "10.0.0.1:1234"

	got := extractIP(req)
	if got != "203.0.113.50" {
		t.Errorf("expected '203.0.113.50', got %q", got)
	}
}

func TestExtractIP_IPv6(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "[::1]:12345"

	got := extractIP(req)
	if got != "::1" {
		t.Errorf("expected '::1', got %q", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PersistentRateLimiter — with mock bolt store
// ═══════════════════════════════════════════════════════════════════════════════

// mockBoltStorer implements core.BoltStorer for testing.
type mockBoltStorer struct {
	data   map[string]map[string][]byte
	setErr error
	getErr error
}

func newMockBoltStorer() *mockBoltStorer {
	return &mockBoltStorer{
		data: make(map[string]map[string][]byte),
	}
}

func (m *mockBoltStorer) Set(bucket, key string, value any, _ int64) error {
	if m.setErr != nil {
		return m.setErr
	}
	if m.data[bucket] == nil {
		m.data[bucket] = make(map[string][]byte)
	}
	// Store as-is (mock)
	m.data[bucket][key] = nil
	return nil
}

func (m *mockBoltStorer) BatchSet(items []core.BoltBatchItem) error {
	for _, item := range items {
		if err := m.Set(item.Bucket, item.Key, item.Value, item.TTL); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockBoltStorer) Get(bucket, key string, dest any) error {
	if m.getErr != nil {
		return m.getErr
	}
	if m.data[bucket] == nil {
		return io.EOF
	}
	if _, ok := m.data[bucket][key]; !ok {
		return io.EOF
	}
	return nil
}

func (m *mockBoltStorer) Delete(bucket, key string) error {
	return nil
}

func (m *mockBoltStorer) List(bucket string) ([]string, error) {
	if m.data[bucket] == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(m.data[bucket]))
	for k := range m.data[bucket] {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockBoltStorer) Close() error {
	return nil
}

func (m *mockBoltStorer) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *mockBoltStorer) GetWebhookSecret(webhookID string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func TestPersistentRateLimiter_WithBolt_AllowsRequests(t *testing.T) {
	bolt := newMockBoltStorer()
	// Always return error on Get so each request starts a new window
	bolt.getErr = io.EOF

	rl := NewPersistentRateLimiter(bolt, 5, time.Minute)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestPersistentRateLimiter_Config(t *testing.T) {
	rl := NewPersistentRateLimiter(nil, 100, 5*time.Minute)

	if rl.rate != 100 {
		t.Errorf("rate = %d, want 100", rl.rate)
	}
	if rl.window != 5*time.Minute {
		t.Errorf("window = %v, want 5m", rl.window)
	}
	if rl.bucket != "ratelimit" {
		t.Errorf("bucket = %q, want ratelimit", rl.bucket)
	}
}
