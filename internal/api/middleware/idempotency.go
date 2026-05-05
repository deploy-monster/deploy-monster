package middleware

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"sync"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const (
	idempotencyHeader  = "Idempotency-Key"
	idempotencyBucket  = "idempotency"
	idempotencyTTLSecs = 86400 // 24 hours
)

// idempotencyEntry is the cached response for an idempotency key.
type idempotencyEntry struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
}

// inFlight tracks requests currently being processed to prevent duplicate processing
var inFlight = make(map[string]bool)
var inFlightMu sync.Mutex

// IdempotencyMiddleware replays cached responses for duplicate requests
// identified by the Idempotency-Key header. Only applies to POST/PUT/PATCH methods.
// Keys are stored in BoltDB with a 24-hour TTL.
// SECURITY FIX (RACE-003): Added mutex locking to prevent race conditions.
func IdempotencyMiddleware(bolt core.BoltStorer) func(http.Handler) http.Handler {
	// Previously declared as `var logger *slog.Logger` and never
	// assigned, which made the cache-write Error log on line 102 dead
	// code. Default to slog.Default() so production sees write
	// failures.
	logger := slog.Default()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only intercept state-changing methods
			if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get(idempotencyHeader)
			if key == "" || bolt == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Scope key to method+path to prevent cross-endpoint collisions
			scopedKey := r.Method + ":" + r.URL.Path + ":" + key

			// SECURITY FIX (RACE-003): Lock to prevent race condition on read-modify-write
			inFlightMu.Lock()
			if inFlight[scopedKey] {
				inFlightMu.Unlock()
				writeErrorJSON(w, http.StatusConflict, "request with this idempotency key is already being processed")
				return
			}
			// Mark as in-flight
			inFlight[scopedKey] = true
			inFlightMu.Unlock()

			// Cleanup in-flight status when done
			defer func() {
				inFlightMu.Lock()
				delete(inFlight, scopedKey)
				inFlightMu.Unlock()
			}()

			// Check for cached response
			var cached idempotencyEntry
			err := bolt.Get(idempotencyBucket, scopedKey, &cached)
			if err == nil {
				// Replay cached response
				for k, v := range cached.Headers {
					w.Header().Set(k, v)
				}
				w.Header().Set("X-Idempotent-Replayed", "true")
				w.WriteHeader(cached.StatusCode)
				_, _ = w.Write(cached.Body)
				return
			}
			if !errors.Is(err, core.ErrBoltNotFound) {
				// A non-NotFound failure (corrupted entry, unmarshal
				// error) triggers a re-execute, same as a cache miss.
				// The request will still flow through and a fresh
				// response will be cached at the end — log it so a
				// corrupted cache surfaces in operator logs instead
				// of silently slipping past.
				logger.Warn("idempotency cache read failed; falling through to handler",
					"key", scopedKey, "error", err)
			}

			// Capture the response while writing through
			rec := &idempotencyRecorder{
				ResponseWriter: w,
				status:         http.StatusOK,
				body:           &bytes.Buffer{},
			}
			next.ServeHTTP(rec, r)

			// Only cache successful responses (2xx)
			if rec.status >= 200 && rec.status < 300 {
				headers := map[string]string{
					"Content-Type": rec.Header().Get("Content-Type"),
				}
				entry := idempotencyEntry{
					StatusCode: rec.status,
					Headers:    headers,
					Body:       rec.body.Bytes(),
				}
				if err := bolt.Set(idempotencyBucket, scopedKey, entry, idempotencyTTLSecs); err != nil {
					logger.Error("idempotency cache write failed", "key", scopedKey, "error", err)
				}
			}
		})
	}
}

// idempotencyRecorder captures the response while writing through to the client.
type idempotencyRecorder struct {
	http.ResponseWriter
	status      int
	body        *bytes.Buffer
	wroteHeader bool
}

func (r *idempotencyRecorder) WriteHeader(code int) {
	if !r.wroteHeader {
		r.status = code
		r.ResponseWriter.WriteHeader(code)
		r.wroteHeader = true
	}
}

func (r *idempotencyRecorder) Write(b []byte) (int, error) {
	if !r.wroteHeader {
		r.wroteHeader = true
	}
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
