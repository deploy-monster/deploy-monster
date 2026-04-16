package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// Phase 4 coverage lift for internal/auth — refresh-token rotation edge
// cases and the support-surface that rotation relies on. Each test pins
// one of the previously-uncovered functions (AddPreviousKey,
// RevokeAccessToken, IsAccessTokenRevoked, purgeExpiredPreviousKeys
// compaction, ClaimsFromContext/ContextWithClaims) without relying on
// integration infrastructure.

// fakeBolt is a minimal in-memory stand-in for core.BoltStorer shaped to
// the exact interface the revocation helpers require. TTL is ignored
// because the tests don't wait on it; if they needed to, the fake would
// schedule a real delete.
type fakeBolt struct {
	mu   sync.Mutex
	data map[string]map[string][]byte // bucket → key → (opaque)
}

func newFakeBolt() *fakeBolt {
	return &fakeBolt{data: map[string]map[string][]byte{}}
}

func (f *fakeBolt) Set(bucket, key string, _ any, _ int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.data[bucket] == nil {
		f.data[bucket] = map[string][]byte{}
	}
	// Store a sentinel — tests only need presence, not the value shape.
	f.data[bucket][key] = []byte{1}
	return nil
}

func (f *fakeBolt) Get(bucket, key string, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.data[bucket][key]; ok {
		return nil
	}
	return errors.New("not found")
}

func TestNewJWTService_RejectsShortSecret(t *testing.T) {
	// The panic branch is deliberate: a process that boots with a weak
	// secret is a misconfiguration we refuse to run under, not a runtime
	// error to handle. Wrap in recover() so the test can assert the
	// panic happens and carries the explanatory message.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic on short secret, got none")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %T (%v)", r, r)
		}
		if !strings.Contains(msg, "at least 32") {
			t.Errorf("panic message missing length requirement: %q", msg)
		}
	}()
	_ = NewJWTService("too-short")
}

func TestJWTService_AddPreviousKey_EmptyIsNoOp(t *testing.T) {
	j := NewJWTService("primary-key-32-bytes-long-abcdefg")
	before := len(j.previousKeys)
	j.AddPreviousKey("")
	if len(j.previousKeys) != before {
		t.Errorf("empty AddPreviousKey mutated previousKeys: before=%d after=%d",
			before, len(j.previousKeys))
	}
}

func TestJWTService_AddPreviousKey_AcceptsOldToken(t *testing.T) {
	// Mint a token under the original key, rotate, register the old key
	// via AddPreviousKey, and confirm validation still passes. This is
	// the core rotation invariant: issued tokens outlive the rotation
	// event up to RotationGracePeriod.
	oldSecret := "old-primary-key-32-bytes-abcdefgh"
	newSecret := "new-primary-key-32-bytes-abcdefgh"

	oldSvc := NewJWTService(oldSecret)
	pair, err := oldSvc.GenerateTokenPair("u1", "t1", "role_admin", "a@b.com")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	newSvc := NewJWTService(newSecret)
	// Without AddPreviousKey the rotated service must reject the old
	// token — pins the negative case so the positive case below isn't a
	// false confirmation.
	if _, err := newSvc.ValidateAccessToken(pair.AccessToken); err == nil {
		t.Fatal("expected rejection before AddPreviousKey, got validation")
	}

	newSvc.AddPreviousKey(oldSecret)
	claims, err := newSvc.ValidateAccessToken(pair.AccessToken)
	if err != nil {
		t.Fatalf("expected validation under rotated service, got %v", err)
	}
	if claims.UserID != "u1" {
		t.Errorf("UserID not preserved through rotation: got %q", claims.UserID)
	}

	// Refresh tokens rotate the same way — validate the refresh side
	// too since the roadmap target is specifically refresh-token rotation.
	rc, err := newSvc.ValidateRefreshToken(pair.RefreshToken)
	if err != nil {
		t.Fatalf("refresh validate after rotation: %v", err)
	}
	if rc.UserID != "u1" {
		t.Errorf("refresh UserID wrong: %q", rc.UserID)
	}
}

func TestJWTService_PurgeExpiredPreviousKeys_CompactsMixed(t *testing.T) {
	// Build a service with three previous keys: [expired, fresh, expired].
	// purgeExpiredPreviousKeys must compact to just the middle entry,
	// exercising both the slide branch (validIdx != i) and the truncate
	// branch at the end. Pre-rotation tests didn't reach either.
	j := NewJWTService("primary-key-32-bytes-long-abcdefg")
	now := time.Now()
	j.previousKeys = [][]byte{[]byte("keep-1"), []byte("keep-2"), []byte("keep-3")}
	j.previousAdded = []time.Time{
		now.Add(-2 * RotationGracePeriod), // expired
		now,                               // fresh
		now.Add(-3 * RotationGracePeriod), // expired
	}

	j.purgeExpiredPreviousKeys()

	if len(j.previousKeys) != 1 {
		t.Fatalf("expected 1 key retained, got %d (%q)", len(j.previousKeys), j.previousKeys)
	}
	if string(j.previousKeys[0]) != "keep-2" {
		t.Errorf("wrong key retained: got %q want %q", j.previousKeys[0], "keep-2")
	}
	if len(j.previousAdded) != 1 {
		t.Errorf("timestamps slice not truncated: len=%d", len(j.previousAdded))
	}
}

func TestJWTService_PurgeExpiredPreviousKeys_AllExpired(t *testing.T) {
	// All-expired is the pure-truncate branch.
	j := NewJWTService("primary-key-32-bytes-long-abcdefg")
	j.previousKeys = [][]byte{[]byte("a"), []byte("b")}
	j.previousAdded = []time.Time{
		time.Now().Add(-2 * RotationGracePeriod),
		time.Now().Add(-2 * RotationGracePeriod),
	}
	j.purgeExpiredPreviousKeys()
	if len(j.previousKeys) != 0 {
		t.Errorf("expected all keys purged, got %d", len(j.previousKeys))
	}
}

func TestJWTService_RevokeAndCheck(t *testing.T) {
	j := NewJWTService("primary-key-32-bytes-long-abcdefg")
	bolt := newFakeBolt()

	// IsAccessTokenRevoked on an empty store returns false.
	if j.IsAccessTokenRevoked(bolt, "jti-1") {
		t.Error("expected !revoked on empty store")
	}

	// Nil storer returns false — the documented fail-open behavior for
	// deployments that haven't wired up a revocation bolt.
	if j.IsAccessTokenRevoked(nil, "jti-1") {
		t.Error("expected !revoked with nil storer (fail-open contract)")
	}

	// Revoke with a future expiry so the TTL branch persists.
	future := time.Now().Add(10 * time.Minute)
	if err := j.RevokeAccessToken(bolt, "jti-1", "u1", future); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if !j.IsAccessTokenRevoked(bolt, "jti-1") {
		t.Error("expected revoked=true after RevokeAccessToken")
	}

	// Revoking an already-expired token is a silent no-op — pins the
	// "remaining <= 0" branch that the coverage profile showed unhit.
	past := time.Now().Add(-10 * time.Minute)
	if err := j.RevokeAccessToken(bolt, "jti-2", "u1", past); err != nil {
		t.Errorf("expected nil err for expired-token revoke, got %v", err)
	}
	if j.IsAccessTokenRevoked(bolt, "jti-2") {
		t.Error("expected expired-token revoke to be a no-op")
	}
}

func TestClaimsContextRoundtrip(t *testing.T) {
	want := &Claims{UserID: "u1", TenantID: "t1", RoleID: "role_admin", Email: "a@b.com"}
	ctx := ContextWithClaims(context.Background(), want)

	got := ClaimsFromContext(ctx)
	if got == nil {
		t.Fatal("ClaimsFromContext returned nil after ContextWithClaims")
	}
	if got.UserID != want.UserID || got.TenantID != want.TenantID {
		t.Errorf("roundtrip mismatch: got %+v want %+v", got, want)
	}

	// Empty context — the nil-cast path that middleware relies on to
	// short-circuit unauthenticated requests.
	if c := ClaimsFromContext(context.Background()); c != nil {
		t.Errorf("expected nil from empty context, got %+v", c)
	}
}

func TestHashPassword_TooLong(t *testing.T) {
	// bcrypt rejects passwords > 72 bytes with ErrPasswordTooLong. The
	// error-path hasn't been exercised before; pinning it here so a
	// future "truncate in app code" change can't silently swallow the
	// error.
	long := strings.Repeat("x", 80)
	if _, err := HashPassword(long); err == nil {
		t.Error("expected error for >72-byte password, got nil")
	}
}

func TestHashAPIKey_TooLong(t *testing.T) {
	long := strings.Repeat("y", 100)
	if _, err := HashAPIKey(long); err == nil {
		t.Error("expected error for >72-byte key, got nil")
	}
}
