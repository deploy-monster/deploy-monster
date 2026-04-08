package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"
)

// CacheControl wraps a handler with Cache-Control and ETag headers.
// maxAge is in seconds; 0 means no-cache.
func CacheControl(maxAge int) func(http.Handler) http.Handler {
	directive := fmt.Sprintf("public, max-age=%d", maxAge)
	if maxAge <= 0 {
		directive = "no-cache"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", directive)
			next.ServeHTTP(w, r)
		})
	}
}

// ETag wraps a handler to compute and set an ETag header based on the response body.
// If the client sends a matching If-None-Match, responds with 304 Not Modified.
func ETag(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		rec := &responseRecorder{ResponseWriter: w, body: make([]byte, 0, 1024)}
		next.ServeHTTP(rec, r)

		// Only ETag successful responses
		if rec.status == 0 || (rec.status >= 200 && rec.status < 300) {
			hash := sha256.Sum256(rec.body)
			etag := fmt.Sprintf(`"%x"`, hash[:8])
			w.Header().Set("ETag", etag)

			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}

		if rec.status != 0 {
			w.WriteHeader(rec.status)
		}
		w.Write(rec.body)
	}
}

type responseRecorder struct {
	http.ResponseWriter
	status int
	body   []byte
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return len(b), nil
}
