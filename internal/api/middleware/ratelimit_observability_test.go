package middleware

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/deploy-monster/deploy-monster/internal/db/models"
)

// rlBoltStub returns a configurable error from Get and remembers Set
// calls so we can assert the fresh-window write after a corrupted-read
// reset.
type rlBoltStub struct {
	getErr   error
	setCalls int
}

func (s *rlBoltStub) Set(bucket, key string, value any, _ int64) error {
	s.setCalls++
	return nil
}
func (s *rlBoltStub) BatchSet(_ []core.BoltBatchItem) error { return nil }
func (s *rlBoltStub) Get(bucket, key string, dest any) error {
	return s.getErr
}
func (s *rlBoltStub) Delete(bucket, key string) error  { return nil }
func (s *rlBoltStub) List(bucket string) ([]string, error) {
	return nil, nil
}
func (s *rlBoltStub) Close() error { return nil }
func (s *rlBoltStub) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*models.APIKey, error) {
	return nil, nil
}
func (s *rlBoltStub) GetWebhookSecret(string) (string, error) { return "", nil }

var _ core.BoltStorer = (*rlBoltStub)(nil)

func TestAuthRateLimiter_New_DefaultsLogger(t *testing.T) {
	rl := NewAuthRateLimiter(&rlBoltStub{}, 5, time.Minute, "login")
	if rl.logger == nil {
		t.Fatal("NewAuthRateLimiter must default the logger; the previously-zero field was making bolt.Set error logs dead code")
	}
}

func TestAuthRateLimiter_CorruptedReadEmitsWarn(t *testing.T) {
	stub := &rlBoltStub{getErr: errors.New("bolt: corrupted entry")}
	rl := NewAuthRateLimiter(stub, 5, time.Minute, "login")

	var buf bytes.Buffer
	rl.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	wrapped := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected request to flow through after fresh-window reset, got status %d", rr.Code)
	}
	if !strings.Contains(buf.String(), "auth ratelimit read failed") {
		t.Fatalf("expected Warn on corrupted Get, got logs: %q", buf.String())
	}
	if stub.setCalls != 1 {
		t.Errorf("expected one Set call (fresh-window write), got %d", stub.setCalls)
	}
}

// ---------------------------------------------------------------------------
// TenantRateLimiter parallels
// ---------------------------------------------------------------------------

func TestTenantRateLimiter_New_DefaultsLogger(t *testing.T) {
	trl := NewTenantRateLimiter(&rlBoltStub{}, 5, time.Minute)
	if trl.logger == nil {
		t.Fatal("NewTenantRateLimiter must default the logger so the new Warn-on-corruption paths are not dead code")
	}
}

func TestTenantRateLimiter_CorruptedReadEmitsWarn(t *testing.T) {
	stub := &rlBoltStub{getErr: errors.New("bolt: corrupted entry")}
	trl := NewTenantRateLimiter(stub, 5, time.Minute)

	var buf bytes.Buffer
	trl.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	handler := trl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{TenantID: "t-corrupted", UserID: "u1"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected request to flow through after fresh-window reset, got status %d", rec.Code)
	}
	logs := buf.String()
	// Both the config read and the window read use the same stub error,
	// so two Warn lines are expected: one for the per-tenant config
	// fall-through, one for the window reset.
	if !strings.Contains(logs, "tenant ratelimit config read failed") {
		t.Errorf("expected config-read Warn, got: %q", logs)
	}
	if !strings.Contains(logs, "tenant ratelimit read failed") {
		t.Errorf("expected window-read Warn, got: %q", logs)
	}
}

func TestTenantRateLimiter_NotFoundDoesNotWarn(t *testing.T) {
	stub := &rlBoltStub{getErr: fmt.Errorf("key %q: %w", "trl:t-fresh", core.ErrBoltNotFound)}
	trl := NewTenantRateLimiter(stub, 5, time.Minute)

	var buf bytes.Buffer
	trl.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	handler := trl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	ctx := auth.ContextWithClaims(req.Context(), &auth.Claims{TenantID: "t-fresh", UserID: "u1"})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req.WithContext(ctx))

	if strings.Contains(buf.String(), "tenant ratelimit") {
		t.Fatalf("NotFound path must not warn, got: %q", buf.String())
	}
}

func TestAuthRateLimiter_NotFoundDoesNotWarn(t *testing.T) {
	// The expected first-request path: Get returns a wrapped
	// ErrBoltNotFound, the limiter opens a fresh window without
	// emitting a warning.
	stub := &rlBoltStub{getErr: fmt.Errorf("key %q: %w", "login:10.0.0.1", core.ErrBoltNotFound)}
	rl := NewAuthRateLimiter(stub, 5, time.Minute, "login")

	var buf bytes.Buffer
	rl.SetLogger(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	wrapped := rl.Wrap(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	rr := httptest.NewRecorder()
	wrapped(rr, req)

	if strings.Contains(buf.String(), "auth ratelimit read failed") {
		t.Fatalf("NotFound path must not warn, got: %q", buf.String())
	}
}
