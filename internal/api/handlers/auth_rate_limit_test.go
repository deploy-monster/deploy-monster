package handlers

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// validateTOTP
// ---------------------------------------------------------------------------

func TestAuthHandler_ValidateTOTP_NilValidator_FailsClosed(t *testing.T) {
	h := &AuthHandler{} // totpValidator left nil — TOTP not configured
	if h.validateTOTP("u1", "123456") {
		t.Fatal("validateTOTP must fail closed when no validator is wired")
	}
}

func TestAuthHandler_ValidateTOTP_DelegatesToValidator(t *testing.T) {
	calls := 0
	h := &AuthHandler{
		totpValidator: func(userID, code string) bool {
			calls++
			return userID == "u1" && code == "good"
		},
	}

	if !h.validateTOTP("u1", "good") {
		t.Fatal("validateTOTP must return whatever the validator says (true)")
	}
	if h.validateTOTP("u1", "bad") {
		t.Fatal("validateTOTP must return whatever the validator says (false)")
	}
	if calls != 2 {
		t.Fatalf("validator was invoked %d times, want 2", calls)
	}
}

// ---------------------------------------------------------------------------
// checkPerAccountRateLimit
// ---------------------------------------------------------------------------

func TestAuthHandler_CheckPerAccountRateLimit_NilBolt(t *testing.T) {
	h := &AuthHandler{}
	if locked, until := h.checkPerAccountRateLimit("a@example.com"); locked || until != 0 {
		t.Fatalf("nil bolt must report not-locked, got locked=%v until=%d", locked, until)
	}
}

func TestAuthHandler_CheckPerAccountRateLimit_NoEntry(t *testing.T) {
	h := &AuthHandler{bolt: newMockBoltStore()}
	if locked, until := h.checkPerAccountRateLimit("a@example.com"); locked || until != 0 {
		t.Fatalf("absent entry must report not-locked, got locked=%v until=%d", locked, until)
	}
}

func TestAuthHandler_CheckPerAccountRateLimit_LockedThenExpired(t *testing.T) {
	bolt := newMockBoltStore()
	h := &AuthHandler{bolt: bolt}

	// Seed an entry that is locked into the future.
	future := time.Now().Add(2 * time.Minute).Unix()
	if err := bolt.Set("account_rl", "a@example.com", accountRateLimitEntry{FailedCount: 5, LockedUntil: future}, 0); err != nil {
		t.Fatalf("seed: %v", err)
	}
	locked, until := h.checkPerAccountRateLimit("a@example.com")
	if !locked || until != future {
		t.Fatalf("locked entry: locked=%v until=%d, want true %d", locked, until, future)
	}

	// Update the seeded entry to an expired LockedUntil — should report
	// not-locked.
	past := time.Now().Add(-2 * time.Minute).Unix()
	if err := bolt.Set("account_rl", "a@example.com", accountRateLimitEntry{FailedCount: 5, LockedUntil: past}, 0); err != nil {
		t.Fatalf("seed expired: %v", err)
	}
	if locked, until := h.checkPerAccountRateLimit("a@example.com"); locked || until != 0 {
		t.Fatalf("expired lock must report not-locked, got locked=%v until=%d", locked, until)
	}
}

// ---------------------------------------------------------------------------
// incrementPerAccountRateLimit
// ---------------------------------------------------------------------------

func TestAuthHandler_IncrementPerAccountRateLimit_NilBoltIsNoop(t *testing.T) {
	h := &AuthHandler{}
	// Must not panic and must produce no observable side effect.
	h.incrementPerAccountRateLimit(context.Background(), "a@example.com")
}

func TestAuthHandler_IncrementPerAccountRateLimit_NoEntryIsNoop(t *testing.T) {
	// The current implementation treats a missing entry the same as
	// an "already locked or unknown error" Get failure — it returns
	// without writing anything. Pin that behavior so a future change
	// to seed-on-miss has to also update this test.
	bolt := newMockBoltStore()
	h := &AuthHandler{bolt: bolt}

	h.incrementPerAccountRateLimit(context.Background(), "a@example.com")

	var entry accountRateLimitEntry
	if err := bolt.Get("account_rl", "a@example.com", &entry); err == nil {
		t.Fatalf("expected key still absent after first increment, got entry=%+v", entry)
	}
}

func TestAuthHandler_IncrementPerAccountRateLimit_BelowThreshold(t *testing.T) {
	bolt := newMockBoltStore()
	h := &AuthHandler{bolt: bolt}

	// Seed a non-locked entry; the increment path runs only when a
	// prior Get succeeds and LockedUntil == 0.
	if err := bolt.Set("account_rl", "a@example.com", accountRateLimitEntry{FailedCount: 2}, 0); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h.incrementPerAccountRateLimit(context.Background(), "a@example.com")

	var entry accountRateLimitEntry
	if err := bolt.Get("account_rl", "a@example.com", &entry); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.FailedCount != 3 {
		t.Fatalf("FailedCount = %d, want 3", entry.FailedCount)
	}
	if entry.LockedUntil != 0 {
		t.Fatalf("LockedUntil should still be 0 below threshold, got %d", entry.LockedUntil)
	}
}

func TestAuthHandler_IncrementPerAccountRateLimit_LocksAtThreshold(t *testing.T) {
	bolt := newMockBoltStore()
	h := &AuthHandler{bolt: bolt}

	// Seed an entry that's one increment away from the lockout
	// threshold. The next call must push the count up and set
	// LockedUntil into the future.
	if err := bolt.Set("account_rl", "a@example.com",
		accountRateLimitEntry{FailedCount: maxFailedAttempts - 1}, 0); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h.incrementPerAccountRateLimit(context.Background(), "a@example.com")

	var entry accountRateLimitEntry
	if err := bolt.Get("account_rl", "a@example.com", &entry); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.FailedCount != maxFailedAttempts {
		t.Fatalf("FailedCount = %d, want %d", entry.FailedCount, maxFailedAttempts)
	}
	if entry.LockedUntil == 0 {
		t.Fatal("LockedUntil must be set once threshold is reached")
	}
	if entry.LockedUntil < time.Now().Unix() {
		t.Fatalf("LockedUntil = %d is in the past", entry.LockedUntil)
	}
}

func TestAuthHandler_IncrementPerAccountRateLimit_AlreadyLockedIsNoop(t *testing.T) {
	bolt := newMockBoltStore()
	h := &AuthHandler{bolt: bolt}

	// Seed with an already-locked entry.
	already := accountRateLimitEntry{
		FailedCount: 5,
		LockedUntil: time.Now().Add(10 * time.Minute).Unix(),
	}
	if err := bolt.Set("account_rl", "a@example.com", already, 0); err != nil {
		t.Fatalf("seed: %v", err)
	}

	h.incrementPerAccountRateLimit(context.Background(), "a@example.com")

	var got accountRateLimitEntry
	if err := bolt.Get("account_rl", "a@example.com", &got); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.FailedCount != already.FailedCount || got.LockedUntil != already.LockedUntil {
		t.Fatalf("already-locked entry was mutated: got %+v, want %+v", got, already)
	}
}

// ---------------------------------------------------------------------------
// loginRateLimitCheck
// ---------------------------------------------------------------------------

func TestAuthHandler_LoginRateLimitCheck_NilBolt(t *testing.T) {
	h := &AuthHandler{}
	rr := httptest.NewRecorder()
	if got := h.loginRateLimitCheck(rr, httptest.NewRequest("POST", "/api/v1/auth/login", nil), "a@example.com"); got != 0 {
		t.Fatalf("loginRateLimitCheck returned %d with nil bolt, want 0", got)
	}
	if rr.Code != 200 { // ResponseRecorder defaults to 200 when no header is written
		t.Fatalf("nil-bolt path must not write to the response, got status=%d", rr.Code)
	}
}

func TestAuthHandler_LoginRateLimitCheck_NotLocked(t *testing.T) {
	h := &AuthHandler{bolt: newMockBoltStore()}
	rr := httptest.NewRecorder()
	if got := h.loginRateLimitCheck(rr, httptest.NewRequest("POST", "/api/v1/auth/login", nil), "a@example.com"); got != 0 {
		t.Fatalf("loginRateLimitCheck returned %d for clean account, want 0", got)
	}
	if rr.Code != 200 {
		t.Fatalf("not-locked path must not write to the response, got status=%d", rr.Code)
	}
}

func TestAuthHandler_LoginRateLimitCheck_Locked(t *testing.T) {
	bolt := newMockBoltStore()
	h := &AuthHandler{bolt: bolt}

	until := time.Now().Add(5 * time.Minute).Unix()
	if err := bolt.Set("account_rl", "a@example.com", accountRateLimitEntry{
		FailedCount: 5,
		LockedUntil: until,
	}, 0); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rr := httptest.NewRecorder()
	got := h.loginRateLimitCheck(rr, httptest.NewRequest("POST", "/api/v1/auth/login", nil), "a@example.com")
	if got != until {
		t.Fatalf("loginRateLimitCheck returned %d, want %d", got, until)
	}
	if rr.Code != 429 {
		t.Fatalf("status = %d, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Error("Retry-After header missing on lockout")
	}
	if rr.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("X-RateLimit-Limit header missing on lockout")
	}
	if rr.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Errorf("X-RateLimit-Remaining = %q, want 0", rr.Header().Get("X-RateLimit-Remaining"))
	}

	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response body: %v", err)
	}
}
