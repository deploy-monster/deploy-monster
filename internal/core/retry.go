package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// noRetryError wraps an error to signal that the retry loop should stop immediately.
type noRetryError struct{ err error }

func (e *noRetryError) Error() string { return e.err.Error() }
func (e *noRetryError) Unwrap() error { return e.err }

// ErrNoRetry wraps an error to prevent further retry attempts.
// Use this for non-transient failures (e.g., 4xx HTTP responses, validation errors).
func ErrNoRetry(err error) error { return &noRetryError{err: err} }

// RetryConfig configures exponential backoff retry behavior.
type RetryConfig struct {
	MaxAttempts  int           // total attempts (including first); 0 or 1 means no retry
	InitialDelay time.Duration // delay before first retry
	MaxDelay     time.Duration // cap on backoff growth
	Logger       *slog.Logger  // optional structured logger
}

// DefaultRetryConfig returns a sensible default: 3 attempts, 200ms → 5s.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 200 * time.Millisecond,
		MaxDelay:     5 * time.Second,
	}
}

// Retry executes op with exponential backoff. Returns nil on first success.
// Respects context cancellation between attempts.
func Retry(ctx context.Context, cfg RetryConfig, op func() error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	attempts := cfg.MaxAttempts
	if attempts <= 0 {
		attempts = 1
	}

	var lastErr error
	for i := 0; i < attempts; i++ {
		if err := op(); err == nil {
			return nil
		} else {
			// If the error signals no-retry, return immediately
			var nre *noRetryError
			if errors.As(err, &nre) {
				return nre.err // unwrap and return the original error
			}
			lastErr = err
		}

		// Don't sleep after last attempt
		if i == attempts-1 {
			break
		}

		delay := cfg.InitialDelay * time.Duration(1<<uint(i))
		if delay > cfg.MaxDelay {
			delay = cfg.MaxDelay
		}

		if cfg.Logger != nil {
			cfg.Logger.Warn("operation failed, retrying",
				"attempt", i+1,
				"max_attempts", attempts,
				"delay", delay,
				"error", lastErr,
			)
		}

		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return fmt.Errorf("operation failed after %d attempts: %w", attempts, lastErr)
}
