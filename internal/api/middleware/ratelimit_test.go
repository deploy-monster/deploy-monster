package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// rlBoltStore is a BoltStorer that actually persists data in memory for rate limit tests.
// We can't use mockBoltStore (auth_test.go) because its Get/Set are no-ops.
type rlBoltStore struct {
	data map[string][]byte
}

func newRLBoltStore() *rlBoltStore {
	return &rlBoltStore{data: make(map[string][]byte)}
}

func (m *rlBoltStore) Get(bucket, key string, out any) error {
	k := bucket + ":" + key
	raw, ok := m.data[k]
	if !ok {
		return fmt.Errorf("key %q in bucket %q: %w", key, bucket, core.ErrBoltNotFound)
	}
	return json.Unmarshal(raw, out)
}

func (m *rlBoltStore) Set(bucket, key string, value any, _ int64) error {
	k := bucket + ":" + key
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[k] = raw
	return nil
}

func (m *rlBoltStore) BatchSet(_ []core.BoltBatchItem) error     { return nil }
func (m *rlBoltStore) Delete(_, _ string) error                  { return nil }
func (m *rlBoltStore) List(_ string) ([]string, error)           { return nil, nil }
func (m *rlBoltStore) Close() error                              { return nil }
func (m *rlBoltStore) GetWebhookSecret(_ string) (string, error) { return "", nil }
func (m *rlBoltStore) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, fmt.Errorf("not found")
}

var _ core.BoltStorer = (*rlBoltStore)(nil)

func TestAuthRateLimiter_AllowsWithinLimit(t *testing.T) {
	store := newRLBoltStore()
	rl := NewAuthRateLimiter(store, 5, time.Minute, "login")

	handler := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.Header.Set("X-Real-IP", "1.2.3.4")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}
}

func TestAuthRateLimiter_BlocksOverLimit(t *testing.T) {
	store := newRLBoltStore()
	rl := NewAuthRateLimiter(store, 3, time.Minute, "login")

	handler := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Use up the limit
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		req.Header.Set("X-Real-IP", "10.0.0.1")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// 4th request should be rejected
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.Header.Set("X-Real-IP", "10.0.0.1")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header should be set")
	}
	if rec.Header().Get("X-RateLimit-Limit") != "3" {
		t.Errorf("X-RateLimit-Limit = %q, want %q", rec.Header().Get("X-RateLimit-Limit"), "3")
	}
	if rec.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want %q", rec.Header().Get("X-RateLimit-Remaining"), "0")
	}
}

func TestAuthRateLimiter_DifferentIPs_Independent(t *testing.T) {
	store := newRLBoltStore()
	rl := NewAuthRateLimiter(store, 1, time.Minute, "login")

	handler := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First IP uses its limit
	req1 := httptest.NewRequest(http.MethodPost, "/", nil)
	req1.RemoteAddr = "1.1.1.1:1234"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Errorf("IP 1.1.1.1: expected 200, got %d", rec1.Code)
	}

	// Second IP should not be affected
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.RemoteAddr = "2.2.2.2:1234"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("IP 2.2.2.2: expected 200, got %d", rec2.Code)
	}
}

func TestAuthRateLimiter_NilBolt_PassesThrough(t *testing.T) {
	rl := NewAuthRateLimiter(nil, 1, time.Minute, "login")

	called := false
	handler := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if !called {
		t.Error("handler should be called when bolt is nil")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestAuthRateLimiter_DifferentPrefixes_Independent(t *testing.T) {
	store := newRLBoltStore()
	loginRL := NewAuthRateLimiter(store, 1, time.Minute, "login")
	registerRL := NewAuthRateLimiter(store, 1, time.Minute, "register")

	loginHandler := loginRL.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	registerHandler := registerRL.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Use up login limit
	req1 := httptest.NewRequest(http.MethodPost, "/", nil)
	req1.Header.Set("X-Real-IP", "5.5.5.5")
	rec1 := httptest.NewRecorder()
	loginHandler.ServeHTTP(rec1, req1)

	// Register should still work for same IP
	req2 := httptest.NewRequest(http.MethodPost, "/", nil)
	req2.Header.Set("X-Real-IP", "5.5.5.5")
	rec2 := httptest.NewRecorder()
	registerHandler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Errorf("register should not be affected by login limit, got %d", rec2.Code)
	}
}
