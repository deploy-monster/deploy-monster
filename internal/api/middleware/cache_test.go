package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCacheControl_SetsMaxAge(t *testing.T) {
	handler := CacheControl(3600)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Cache-Control")
	want := "public, max-age=3600"
	if got != want {
		t.Errorf("Cache-Control = %q, want %q", got, want)
	}
}

func TestCacheControl_ZeroMeansNoCache(t *testing.T) {
	handler := CacheControl(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	got := rec.Header().Get("Cache-Control")
	if got != "no-cache" {
		t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
	}
}

func TestCacheControl_NegativeMeansNoCache(t *testing.T) {
	handler := CacheControl(-1)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("expected no-cache for negative maxAge")
	}
}

func TestETag_SetsETagHeader(t *testing.T) {
	body := []byte(`{"status":"ok"}`)
	handler := ETag(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag header should be set")
	}

	// Verify ETag format and correctness
	hash := sha256.Sum256(body)
	expected := fmt.Sprintf(`"%x"`, hash[:8])
	if etag != expected {
		t.Errorf("ETag = %q, want %q", etag, expected)
	}
}

func TestETag_IfNoneMatch_Returns304(t *testing.T) {
	body := []byte(`{"status":"ok"}`)
	hash := sha256.Sum256(body)
	etag := fmt.Sprintf(`"%x"`, hash[:8])

	handler := ETag(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotModified {
		t.Errorf("expected 304, got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Error("body should be empty for 304")
	}
}

func TestETag_IfNoneMatch_Mismatch_Returns200(t *testing.T) {
	handler := ETag(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":"new"}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", `"stale-etag"`)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 for mismatched ETag, got %d", rec.Code)
	}
}

func TestETag_ErrorResponse_NoETag(t *testing.T) {
	handler := ETag(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"fail"}`))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	etag := rec.Header().Get("ETag")
	if etag != "" {
		t.Errorf("ETag should not be set for 500 response, got %q", etag)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}
