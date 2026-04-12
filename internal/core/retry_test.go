package core

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestRetry_SuccessOnFirst(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), DefaultRetryConfig(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestRetry_SuccessOnSecond(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := Retry(context.Background(), cfg, func() error {
		calls++
		if calls < 2 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetry_AllFail(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 3, InitialDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := Retry(context.Background(), cfg, func() error {
		calls++
		return errors.New("permanent")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestRetry_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cfg := RetryConfig{MaxAttempts: 10, InitialDelay: time.Second, MaxDelay: time.Second}
	calls := 0
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()
	err := Retry(ctx, cfg, func() error {
		calls++
		return errors.New("fail")
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestRetry_NilContext(t *testing.T) {
	calls := 0
	err := Retry(nil, DefaultRetryConfig(), func() error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestRetry_ZeroAttempts(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 0, InitialDelay: time.Millisecond, MaxDelay: time.Millisecond}
	calls := 0
	err := Retry(context.Background(), cfg, func() error {
		calls++
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (fallback), got %d", calls)
	}
}

func TestRetry_ErrNoRetry_StopsImmediately(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 5, InitialDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := Retry(context.Background(), cfg, func() error {
		calls++
		return ErrNoRetry(errors.New("client error 400"))
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", calls)
	}
	if err.Error() != "client error 400" {
		t.Fatalf("expected unwrapped error, got %q", err.Error())
	}
}

func TestRetry_ErrNoRetry_AfterTransient(t *testing.T) {
	cfg := RetryConfig{MaxAttempts: 5, InitialDelay: time.Millisecond, MaxDelay: 10 * time.Millisecond}
	calls := 0
	err := Retry(context.Background(), cfg, func() error {
		calls++
		if calls < 3 {
			return errors.New("transient 500")
		}
		return ErrNoRetry(errors.New("permanent 404"))
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls (2 transient + 1 no-retry), got %d", calls)
	}
}

func TestErrNoRetry_ErrorAndUnwrap(t *testing.T) {
	inner := errors.New("backend rejected request")
	wrapped := ErrNoRetry(inner)

	// Error() on the wrapper should delegate to the inner message.
	if wrapped.Error() != inner.Error() {
		t.Errorf("wrapped.Error() = %q, want %q", wrapped.Error(), inner.Error())
	}

	// errors.Is must walk the Unwrap chain to find the inner error.
	if !errors.Is(wrapped, inner) {
		t.Error("errors.Is could not find inner error through Unwrap")
	}

	// errors.As must also resolve through the wrapper.
	var target *noRetryError
	if !errors.As(wrapped, &target) {
		t.Error("errors.As did not match *noRetryError")
	}
	if target == nil || target.Unwrap() != inner {
		t.Errorf("Unwrap() did not return the inner error")
	}
}

func TestRetry_LoggerBranch(t *testing.T) {
	// Covers the cfg.Logger != nil branch inside Retry — without a
	// logger attached, the Warn-on-retry path stayed uncovered.
	cfg := RetryConfig{
		MaxAttempts:  2,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Millisecond,
		Logger:       slog.Default(),
	}
	calls := 0
	err := Retry(context.Background(), cfg, func() error {
		calls++
		if calls == 1 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 calls, got %d", calls)
	}
}

func TestRetry_BackoffCappedAtMaxDelay(t *testing.T) {
	// Covers the delay > cfg.MaxDelay cap branch. InitialDelay*2^1 = 2ms
	// exceeds MaxDelay=1ms so the second retry hits the clamp.
	cfg := RetryConfig{
		MaxAttempts:  3,
		InitialDelay: time.Millisecond,
		MaxDelay:     time.Millisecond,
	}
	calls := 0
	_ = Retry(context.Background(), cfg, func() error {
		calls++
		return errors.New("boom")
	})
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestSafeGo_NoPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	ran := false
	SafeGo(nil, "test", func() {
		ran = true
		wg.Done()
	})
	wg.Wait()
	if !ran {
		t.Error("expected goroutine to run")
	}
}

func TestSafeGo_RecoversPanic(t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)
	// This should not crash the test process
	SafeGo(nil, "test-panic", func() {
		defer wg.Done()
		panic("test panic")
	})
	wg.Wait()
	// If we get here, the panic was recovered
}
