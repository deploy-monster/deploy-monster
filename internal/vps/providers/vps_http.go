package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// vpsHTTPRetryConfig controls the retry behavior of the shared VPS HTTP helper.
// It is a package variable so tests can tighten the delays.
var vpsHTTPRetryConfig = core.RetryConfig{
	MaxAttempts:  4,
	InitialDelay: 300 * time.Millisecond,
	MaxDelay:     4 * time.Second,
}

// vpsMaxRetryAfter caps the delay honored from a provider's Retry-After header
// so a malicious or broken upstream cannot make us sleep indefinitely.
const vpsMaxRetryAfter = 30 * time.Second

// vpsMaxPages caps pagination loops to protect against runaway next-page links.
const vpsMaxPages = 20

// vpsErrBodyLimit limits how much of a response body is included in error
// messages to keep logs readable if the upstream returns a huge HTML page.
const vpsErrBodyLimit = 256

// vpsDoRequest performs an authenticated HTTP request against a VPS provider
// API and returns the response body. It layers:
//
//   - a circuit breaker, so a flapping upstream fails fast;
//   - bounded exponential-backoff retry via core.Retry, so transient network
//     hiccups and 5xx responses are retried;
//   - HTTP 429 handling that honors the Retry-After header up to vpsMaxRetryAfter;
//   - terminal classification for non-429 4xx responses via core.ErrNoRetry.
//
// `payload` may be nil for requests without a body. It is re-wrapped on each
// retry attempt because http.Request bodies are single-use.
func vpsDoRequest(
	ctx context.Context,
	client *http.Client,
	cb *core.CircuitBreaker,
	providerName, method, url, token string,
	payload []byte,
) ([]byte, error) {
	var respBody []byte
	err := core.Retry(ctx, vpsHTTPRetryConfig, func() error {
		return cb.Execute(func() error {
			var body io.Reader
			if payload != nil {
				body = bytes.NewReader(payload)
			}
			req, err := http.NewRequestWithContext(ctx, method, url, body)
			if err != nil {
				return core.ErrNoRetry(fmt.Errorf("%s API: build request: %w", providerName, err))
			}
			req.Header.Set("Authorization", "Bearer "+token)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				// Context cancellation is terminal; transport errors are retryable.
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return core.ErrNoRetry(fmt.Errorf("%s API: %w", providerName, err))
				}
				return fmt.Errorf("%s API: %w", providerName, err)
			}
			defer resp.Body.Close()

			readBody, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return fmt.Errorf("%s API %s %s: read body: %w", providerName, method, url, readErr)
			}
			respBody = readBody

			switch {
			case resp.StatusCode < 400:
				return nil
			case resp.StatusCode == http.StatusTooManyRequests:
				// Respect Retry-After (bounded) then return a retryable error.
				if wait := parseRetryAfter(resp.Header.Get("Retry-After")); wait > 0 {
					if wait > vpsMaxRetryAfter {
						wait = vpsMaxRetryAfter
					}
					select {
					case <-time.After(wait):
					case <-ctx.Done():
						return core.ErrNoRetry(ctx.Err())
					}
				}
				return fmt.Errorf("%s API %s %s: HTTP 429: %s",
					providerName, method, url, truncateBody(readBody, vpsErrBodyLimit))
			case resp.StatusCode >= 500:
				return fmt.Errorf("%s API %s %s: HTTP %d: %s",
					providerName, method, url, resp.StatusCode, truncateBody(readBody, vpsErrBodyLimit))
			default: // 4xx (excluding 429)
				return core.ErrNoRetry(fmt.Errorf("%s API %s %s: HTTP %d: %s",
					providerName, method, url, resp.StatusCode, truncateBody(readBody, vpsErrBodyLimit)))
			}
		})
	})
	if err != nil {
		return nil, err
	}
	return respBody, nil
}

// parseRetryAfter parses a Retry-After header value. It supports both the
// delta-seconds form ("120") and the HTTP-date form (RFC 7231 §7.1.3).
// Returns 0 when the header is absent, invalid, or specifies a past time.
func parseRetryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		d := time.Until(t)
		if d < 0 {
			return 0
		}
		return d
	}
	return 0
}

// truncateBody returns at most n bytes of b as a string, appending an ellipsis
// marker when the body was longer than the limit.
func truncateBody(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}

// vpsMarshalPayload JSON-encodes the payload for a request, returning a nil
// byte slice (not an empty slice) when no payload was given.
func vpsMarshalPayload(payload any) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}
	return json.Marshal(payload)
}
