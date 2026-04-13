package middleware

import (
	"bytes"
	"log/slog"
	"net/http"

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

// IdempotencyMiddleware replays cached responses for duplicate requests
// identified by the Idempotency-Key header. Only applies to POST/PUT/PATCH methods.
// Keys are stored in BoltDB with a 24-hour TTL.
func IdempotencyMiddleware(bolt core.BoltStorer) func(http.Handler) http.Handler {
	var logger *slog.Logger
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

			// Check for cached response
			var cached idempotencyEntry
			if err := bolt.Get(idempotencyBucket, scopedKey, &cached); err == nil {
				// Replay cached response
				for k, v := range cached.Headers {
					w.Header().Set(k, v)
				}
				w.Header().Set("X-Idempotent-Replayed", "true")
				w.WriteHeader(cached.StatusCode)
				_, _ = w.Write(cached.Body)
				return
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
				if err := bolt.Set(idempotencyBucket, scopedKey, entry, idempotencyTTLSecs); err != nil && logger != nil {
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
