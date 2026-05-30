package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// idempBoltStore is a BoltStorer that persists data in memory for idempotency tests.
type idempBoltStore struct {
	mu   sync.Mutex
	data map[string]map[string][]byte
}

func newIdempBoltStore() *idempBoltStore {
	return &idempBoltStore{data: make(map[string]map[string][]byte)}
}

func (m *idempBoltStore) Set(bucket, key string, value any, _ int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[bucket] == nil {
		m.data[bucket] = make(map[string][]byte)
	}
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	m.data[bucket][key] = b
	return nil
}

func (m *idempBoltStore) Get(bucket, key string, dest any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data[bucket] == nil {
		return fmt.Errorf("bucket %q: %w", bucket, core.ErrKVNotFound)
	}
	b, ok := m.data[bucket][key]
	if !ok {
		return fmt.Errorf("key %q: %w", key, core.ErrKVNotFound)
	}
	return json.Unmarshal(b, dest)
}

func (m *idempBoltStore) BatchSet(_ []core.BoltBatchItem) error     { return nil }
func (m *idempBoltStore) Delete(_, _ string) error                  { return nil }
func (m *idempBoltStore) List(_ string) ([]string, error)           { return nil, nil }
func (m *idempBoltStore) Close() error                              { return nil }
func (m *idempBoltStore) GetWebhookSecret(_ string) (string, error) { return "", nil }
func (m *idempBoltStore) GetAPIKeyByPrefix(_ context.Context, _ string) (*models.APIKey, error) {
	return nil, errors.New("not found")
}

var _ core.BoltStorer = (*idempBoltStore)(nil)

func TestIdempotency_NoHeader_PassesThrough(t *testing.T) {
	bolt := newIdempBoltStore()
	callCount := 0
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"123"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestIdempotency_FirstRequest_CachesResponse(t *testing.T) {
	bolt := newIdempBoltStore()
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"abc"}`))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("Idempotency-Key", "key-1")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rr.Code)
	}

	// Verify cached in bolt.
	scopedKey := scopedIdempotencyKey(req, "key-1", nil)
	var entry idempotencyEntry
	if err := bolt.Get(idempotencyBucket, scopedKey, &entry); err != nil {
		t.Fatalf("expected cached entry, got error: %v", err)
	}
	if entry.StatusCode != http.StatusCreated {
		t.Errorf("cached status = %d, want 201", entry.StatusCode)
	}
}

func TestIdempotency_DuplicateRequest_ReplaysResponse(t *testing.T) {
	bolt := newIdempBoltStore()
	callCount := 0
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"id":"abc"}`))
	}))

	// First request
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("Idempotency-Key", "key-dup")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Second request — same key
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req2.Header.Set("Idempotency-Key", "key-dup")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if callCount != 1 {
		t.Errorf("expected handler called once, got %d", callCount)
	}
	if rr2.Code != http.StatusCreated {
		t.Errorf("replayed status = %d, want 201", rr2.Code)
	}
	if rr2.Header().Get("X-Idempotent-Replayed") != "true" {
		t.Error("expected X-Idempotent-Replayed header")
	}
	if rr2.Body.String() != `{"id":"abc"}` {
		t.Errorf("replayed body = %q", rr2.Body.String())
	}
}

func TestIdempotency_GET_SkipsMiddleware(t *testing.T) {
	bolt := newIdempBoltStore()
	callCount := 0
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.Header.Set("Idempotency-Key", "key-get")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestIdempotency_ErrorResponse_NotCached(t *testing.T) {
	bolt := newIdempBoltStore()
	callCount := 0
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad"}`))
	}))

	// First request — 400 error
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("Idempotency-Key", "key-err")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Second request — should NOT be replayed (error wasn't cached)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req2.Header.Set("Idempotency-Key", "key-err")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if callCount != 2 {
		t.Errorf("expected 2 calls (error not cached), got %d", callCount)
	}
}

func TestIdempotency_NilBolt_PassesThrough(t *testing.T) {
	callCount := 0
	handler := IdempotencyMiddleware(nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("Idempotency-Key", "key-nil")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}
}

func TestIdempotency_DifferentPaths_DifferentKeys(t *testing.T) {
	bolt := newIdempBoltStore()
	callCount := 0
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"path":"` + r.URL.Path + `"}`))
	}))

	// Request to /apps
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req1.Header.Set("Idempotency-Key", "same-key")
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	// Same key but different path
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/databases", nil)
	req2.Header.Set("Idempotency-Key", "same-key")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if callCount != 2 {
		t.Errorf("expected 2 calls (different paths), got %d", callCount)
	}
}

func TestIdempotency_AuthRoutesSkipCaching(t *testing.T) {
	bolt := newIdempBoltStore()
	callCount := 0
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"access_token":"token-%d"}`, callCount)))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"a@example.com","password":"one"}`))
	req.Header.Set("Idempotency-Key", "login-key")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"b@example.com","password":"two"}`))
	req2.Header.Set("Idempotency-Key", "login-key")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if callCount != 2 {
		t.Fatalf("expected auth route to run twice, got %d calls", callCount)
	}
	if rr2.Header().Get("X-Idempotent-Replayed") != "" {
		t.Fatal("auth route response must not be replayed")
	}
	if strings.Contains(rr2.Body.String(), "token-1") {
		t.Fatalf("second auth response replayed first token: %s", rr2.Body.String())
	}
}

func TestIdempotency_DifferentBodiesDoNotReplay(t *testing.T) {
	bolt := newIdempBoltStore()
	callCount := 0
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fmt.Sprintf(`{"call":%d,"body":%q}`, callCount, string(body))))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"one"}`))
	req.Header.Set("Idempotency-Key", "same-key")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"two"}`))
	req2.Header.Set("Idempotency-Key", "same-key")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if callCount != 2 {
		t.Fatalf("expected different request bodies to execute independently, got %d calls", callCount)
	}
	if rr2.Header().Get("X-Idempotent-Replayed") != "" {
		t.Fatal("different request body must not replay cached response")
	}
}

func TestIdempotency_DifferentAuthScopesDoNotReplay(t *testing.T) {
	bolt := newIdempBoltStore()
	callCount := 0
	handler := IdempotencyMiddleware(bolt)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fmt.Sprintf(`{"call":%d}`, callCount)))
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"same"}`))
	req.Header.Set("Idempotency-Key", "same-key")
	req.Header.Set("Authorization", "Bearer user-a")
	handler.ServeHTTP(httptest.NewRecorder(), req)

	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"same"}`))
	req2.Header.Set("Idempotency-Key", "same-key")
	req2.Header.Set("Authorization", "Bearer user-b")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	if callCount != 2 {
		t.Fatalf("expected different auth scopes to execute independently, got %d calls", callCount)
	}
	if rr2.Header().Get("X-Idempotent-Replayed") != "" {
		t.Fatal("different auth scope must not replay cached response")
	}

	req3 := httptest.NewRequest(http.MethodPost, "/api/v1/apps", strings.NewReader(`{"name":"same"}`))
	req3.Header.Set("Idempotency-Key", "same-key")
	req3.Header.Set("Authorization", "Bearer user-a")
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)

	if callCount != 2 {
		t.Fatalf("expected matching auth/body scope to replay, got %d calls", callCount)
	}
	if rr3.Header().Get("X-Idempotent-Replayed") != "true" {
		t.Fatal("expected replay for same auth scope and body")
	}
}

// idempCorruptedStore returns a non-NotFound error from Get to provoke
// the new sentinel-aware Warn branch in IdempotencyMiddleware.
type idempCorruptedStore struct{ idempBoltStore }

func (s *idempCorruptedStore) Get(_, _ string, _ any) error {
	return errors.New("idempotency: bolt unmarshal failed (corrupted entry)")
}

// idempNotFoundStore returns a wrapped NotFound — the expected first-
// request path. Mirrors the pattern in ratelimit_observability_test.
type idempNotFoundStore struct{ idempBoltStore }

func (s *idempNotFoundStore) Get(bucket, key string, _ any) error {
	return fmt.Errorf("key %q in bucket %q: %w", key, bucket, core.ErrKVNotFound)
}

func TestIdempotency_CorruptedReadEmitsWarn(t *testing.T) {
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })

	store := &idempCorruptedStore{idempBoltStore: idempBoltStore{data: make(map[string]map[string][]byte)}}
	called := 0
	handler := IdempotencyMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("Idempotency-Key", "key-corrupted")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if called != 1 {
		t.Fatalf("expected request to fall through to handler on corrupted cache, called=%d", called)
	}
	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	if !strings.Contains(buf.String(), "idempotency cache read failed") {
		t.Errorf("expected Warn log, got: %q", buf.String())
	}
}

func TestIdempotency_NotFoundDoesNotWarn(t *testing.T) {
	var buf bytes.Buffer
	original := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(original) })

	store := &idempNotFoundStore{idempBoltStore: idempBoltStore{data: make(map[string]map[string][]byte)}}
	handler := IdempotencyMiddleware(store)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps", nil)
	req.Header.Set("Idempotency-Key", "key-fresh")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if strings.Contains(buf.String(), "idempotency cache read failed") {
		t.Fatalf("NotFound path must not warn, got: %q", buf.String())
	}
}
