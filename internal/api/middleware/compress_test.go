package middleware

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCompress_LargeJSON(t *testing.T) {
	// Generate a JSON response larger than 1KB
	body := `{"data":"` + strings.Repeat("x", 2000) + `"}`

	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))

	req := httptest.NewRequest("GET", "/api/v1/apps", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected Content-Encoding: gzip for large response")
	}
	if rr.Header().Get("Vary") != "Accept-Encoding" {
		t.Error("expected Vary: Accept-Encoding")
	}

	// Verify gzip-decompressed body matches original
	gr, err := gzip.NewReader(rr.Body)
	if err != nil {
		t.Fatalf("gzip decode error: %v", err)
	}
	defer gr.Close()
	decoded, _ := io.ReadAll(gr)
	if string(decoded) != body {
		t.Errorf("decompressed body mismatch: got %d bytes, want %d", len(decoded), len(body))
	}
}

func TestCompress_SmallResponse_NoCompression(t *testing.T) {
	body := `{"ok":true}`

	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not compress responses under 1KB")
	}
	if rr.Body.String() != body {
		t.Errorf("body mismatch: got %q", rr.Body.String())
	}
}

func TestCompress_NoAcceptEncoding(t *testing.T) {
	body := strings.Repeat("x", 2000)

	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(body))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	// No Accept-Encoding header
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not compress when client doesn't accept gzip")
	}
	if rr.Body.String() != body {
		t.Errorf("body mismatch: got %d bytes", rr.Body.Len())
	}
}

func TestCompress_WebSocketUpgrade_Skipped(t *testing.T) {
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strings.Repeat("x", 2000)))
	}))

	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Upgrade", "websocket")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not compress WebSocket upgrades")
	}
}

func TestCompress_SSE_Skipped(t *testing.T) {
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(strings.Repeat("data: event\n\n", 200)))
	}))

	req := httptest.NewRequest("GET", "/events", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("Accept", "text/event-stream")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not compress SSE streams")
	}
}

func TestCompress_BinaryContent_Skipped(t *testing.T) {
	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte(strings.Repeat("\x89PNG", 500)))
	}))

	req := httptest.NewRequest("GET", "/image.png", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Content-Encoding") == "gzip" {
		t.Error("should not compress binary content types like image/png")
	}
}

func TestCompress_ExplicitStatusCode(t *testing.T) {
	body := strings.Repeat("x", 2000)

	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(body))
	}))

	req := httptest.NewRequest("POST", "/api/v1/apps", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected gzip on large 201 response")
	}
}

func TestCompress_PreCompressed_Skipped(t *testing.T) {
	body := strings.Repeat("x", 2000)

	handler := Compress(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "br") // already brotli-compressed
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	// Should keep original encoding, not double-compress
	if rr.Header().Get("Content-Encoding") != "br" {
		t.Errorf("expected original br encoding preserved, got %q", rr.Header().Get("Content-Encoding"))
	}
}
