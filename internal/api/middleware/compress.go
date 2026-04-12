package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

const compressionMinSize = 1024 // only compress responses > 1KB

// Compress returns middleware that gzip-compresses responses when the client
// supports it and the response body is large enough to benefit.
func Compress(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip if client doesn't accept gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Skip WebSocket upgrades and SSE streams
		if r.Header.Get("Upgrade") != "" || r.Header.Get("Accept") == "text/event-stream" {
			next.ServeHTTP(w, r)
			return
		}

		cw := &compressWriter{
			ResponseWriter: w,
			request:        r,
		}
		defer cw.Close()

		next.ServeHTTP(cw, r)
	})
}

var gzipPool = sync.Pool{
	New: func() any {
		w, _ := gzip.NewWriterLevel(io.Discard, gzip.DefaultCompression)
		return w
	},
}

// compressWriter buffers small responses and only enables gzip for larger ones.
type compressWriter struct {
	http.ResponseWriter
	request    *http.Request
	gzWriter   *gzip.Writer
	buf        []byte
	statusCode int
	headerSent bool
}

func (cw *compressWriter) WriteHeader(code int) {
	cw.statusCode = code
	// Don't forward yet — we decide on compression when we see the body size
}

func (cw *compressWriter) Write(b []byte) (int, error) {
	if cw.headerSent {
		// Already decided — write to wherever we committed
		if cw.gzWriter != nil {
			return cw.gzWriter.Write(b)
		}
		return cw.ResponseWriter.Write(b)
	}

	cw.buf = append(cw.buf, b...)

	// If buffer exceeds threshold, commit to gzip
	if len(cw.buf) >= compressionMinSize {
		cw.commitGzip()
		return len(b), nil
	}

	return len(b), nil
}

func (cw *compressWriter) commitGzip() {
	cw.headerSent = true

	// Don't compress if Content-Encoding is already set (e.g. pre-compressed)
	if cw.Header().Get("Content-Encoding") != "" {
		cw.writeStatus()
		cw.ResponseWriter.Write(cw.buf)
		return
	}

	// Don't compress non-text content types that won't benefit
	ct := cw.Header().Get("Content-Type")
	if ct != "" && !compressible(ct) {
		cw.writeStatus()
		cw.ResponseWriter.Write(cw.buf)
		return
	}

	cw.Header().Set("Content-Encoding", "gzip")
	cw.Header().Set("Vary", "Accept-Encoding")
	cw.Header().Del("Content-Length") // length changes after compression

	cw.writeStatus()

	gz := gzipPool.Get().(*gzip.Writer)
	gz.Reset(cw.ResponseWriter)
	cw.gzWriter = gz
	gz.Write(cw.buf)
}

func (cw *compressWriter) commitPlain() {
	cw.headerSent = true
	cw.writeStatus()
	if len(cw.buf) > 0 {
		cw.ResponseWriter.Write(cw.buf)
	}
}

func (cw *compressWriter) writeStatus() {
	if cw.statusCode == 0 {
		cw.statusCode = http.StatusOK
	}
	cw.ResponseWriter.WriteHeader(cw.statusCode)
}

// Close flushes any buffered data.
func (cw *compressWriter) Close() {
	if !cw.headerSent {
		// Small response — send uncompressed
		cw.commitPlain()
		return
	}
	if cw.gzWriter != nil {
		cw.gzWriter.Close()
		gzipPool.Put(cw.gzWriter)
		cw.gzWriter = nil
	}
}

// compressible returns true for content types that benefit from compression.
func compressible(ct string) bool {
	ct = strings.ToLower(ct)
	return strings.HasPrefix(ct, "application/json") ||
		strings.HasPrefix(ct, "text/") ||
		strings.HasPrefix(ct, "application/javascript") ||
		strings.HasPrefix(ct, "application/xml") ||
		strings.HasPrefix(ct, "image/svg+xml")
}
