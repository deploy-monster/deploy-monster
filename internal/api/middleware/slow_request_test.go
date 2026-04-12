package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRequestLogger_SlowRequestWarning(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow request test in short mode")
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a slow request (>5s threshold)
		time.Sleep(5100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "slow request") {
		t.Errorf("expected 'slow request' warning in log, got: %s", output)
	}
	if !strings.Contains(output, "WARN") {
		t.Errorf("expected WARN level for slow request, got: %s", output)
	}
}

func TestRequestLogger_FastRequest_InfoLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	output := buf.String()
	if strings.Contains(output, "slow request") {
		t.Error("fast request should not be logged as 'slow request'")
	}
	if !strings.Contains(output, "INFO") {
		t.Errorf("expected INFO level for fast request, got: %s", output)
	}
}
