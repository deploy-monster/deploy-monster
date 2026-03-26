package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// =============================================================================
// PersistentRateLimiter — with working BBolt mock (full path coverage)
// =============================================================================

// fullMockBoltStorer is a richer mock that actually stores/retrieves JSON data
// to exercise the Middleware's internal read-modify-write logic.
type fullMockBoltStorer struct {
	data   map[string]map[string][]byte
	setErr error
	getErr error
}

func newFullMockBoltStorer() *fullMockBoltStorer {
	return &fullMockBoltStorer{
		data: make(map[string]map[string][]byte),
	}
}

func (m *fullMockBoltStorer) Set(bucket, key string, value any, _ int64) error {
	if m.setErr != nil {
		return m.setErr
	}
	if m.data[bucket] == nil {
		m.data[bucket] = make(map[string][]byte)
	}
	b, _ := json.Marshal(value)
	m.data[bucket][key] = b
	return nil
}

func (m *fullMockBoltStorer) Get(bucket, key string, dest any) error {
	if m.getErr != nil {
		return m.getErr
	}
	if m.data[bucket] == nil {
		return fmt.Errorf("key not found")
	}
	raw, ok := m.data[bucket][key]
	if !ok {
		return fmt.Errorf("key not found")
	}
	return json.Unmarshal(raw, dest)
}

func (m *fullMockBoltStorer) Delete(bucket, key string) error {
	if m.data[bucket] != nil {
		delete(m.data[bucket], key)
	}
	return nil
}

func (m *fullMockBoltStorer) List(bucket string) ([]string, error) {
	if m.data[bucket] == nil {
		return nil, nil
	}
	keys := make([]string, 0, len(m.data[bucket]))
	for k := range m.data[bucket] {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *fullMockBoltStorer) Close() error { return nil }

func (m *fullMockBoltStorer) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	return nil, fmt.Errorf("not implemented")
}

func (m *fullMockBoltStorer) GetWebhookSecret(webhookID string) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func TestPersistentRateLimiter_WithBolt_RateLimitEnforced(t *testing.T) {
	bolt := newFullMockBoltStorer()
	rl := NewPersistentRateLimiter(bolt, 2, time.Minute)

	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	// First 2 requests should pass (new window + increment)
	for i := range 2 {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// 3rd request should be rate limited
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("request 3: expected 429, got %d", rr.Code)
	}
}

func TestPersistentRateLimiter_WithBolt_NewWindowAfterExpiry(t *testing.T) {
	bolt := newFullMockBoltStorer()
	// Seed an expired entry
	entry := rateLimitEntry{Count: 100, ResetAt: time.Now().Unix() - 10}
	bolt.Set("ratelimit", "rl:10.0.0.1", entry, 0)

	rl := NewPersistentRateLimiter(bolt, 5, time.Minute)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.1:1234"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	// Should pass — old window expired, new one starts
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 after window expiry, got %d", rr.Code)
	}
}

func TestPersistentRateLimiter_WithBolt_RateLimitHeaders(t *testing.T) {
	bolt := newFullMockBoltStorer()
	// Seed an entry at the limit
	entry := rateLimitEntry{Count: 3, ResetAt: time.Now().Unix() + 60}
	bolt.Set("ratelimit", "rl:10.0.0.2", entry, 0)

	rl := NewPersistentRateLimiter(bolt, 3, time.Minute)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.2:5678"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}
	if rr.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("expected X-RateLimit-Remaining: 0, got %q", rr.Header().Get("X-RateLimit-Remaining"))
	}
}

func TestPersistentRateLimiter_WithBolt_IncrementWithinWindow(t *testing.T) {
	bolt := newFullMockBoltStorer()
	// Seed an entry within the window with room
	entry := rateLimitEntry{Count: 1, ResetAt: time.Now().Unix() + 120}
	bolt.Set("ratelimit", "rl:10.0.0.3", entry, 0)

	rl := NewPersistentRateLimiter(bolt, 5, 2*time.Minute)
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.0.0.3:9999"

	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for within-limit request, got %d", rr.Code)
	}

	// Verify count was incremented
	var updated rateLimitEntry
	bolt.Get("ratelimit", "rl:10.0.0.3", &updated)
	if updated.Count != 2 {
		t.Errorf("expected count 2, got %d", updated.Count)
	}
}

// Keep the io import reference for the existing mockBoltStorer
var _ = io.EOF
