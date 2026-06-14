package core

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLokiSinkSendsRecords(t *testing.T) {
	var (
		mu       sync.Mutex
		requests int
		auth     string
		payload  map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		requests++
		auth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Errorf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	sink := NewLokiSink(srv.URL, "user", "pass", time.Second, slog.Default())
	record := slog.NewRecord(time.Unix(10, 20), slog.LevelWarn, "hello loki", 0)
	record.AddAttrs(slog.String("module", "test"))
	sink.Handle(record)
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if !strings.HasPrefix(auth, "Basic ") {
		t.Fatalf("missing basic auth header: %q", auth)
	}
	streams, ok := payload["streams"].([]any)
	if !ok || len(streams) != 1 {
		t.Fatalf("unexpected loki payload: %#v", payload)
	}
}

func TestLokiSinkHandleAfterCloseDoesNotPanic(t *testing.T) {
	sink := NewLokiSink("http://127.0.0.1:1/loki/api/v1/push", "", "", time.Millisecond, slog.Default())
	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Handle after Close panicked: %v", r)
		}
	}()
	sink.Handle(slog.NewRecord(time.Now(), slog.LevelInfo, "after close", 0))
}

func TestLokiSinkConcurrentHandleAndCloseDoesNotPanic(t *testing.T) {
	sink := NewLokiSink("http://127.0.0.1:1/loki/api/v1/push", "", "", time.Millisecond, slog.Default())
	var wg sync.WaitGroup
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sink.Handle(slog.NewRecord(time.Now(), slog.LevelInfo, "during close", 0))
		}()
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	wg.Wait()
}

func TestMultiHandlerFansOutToSinks(t *testing.T) {
	primary := slog.NewTextHandler(&strings.Builder{}, nil)
	sink := &recordingLogSink{}
	h := &multiHandler{primary: primary, sinks: []LogSink{sink}}

	if !h.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatal("handler should be enabled for info")
	}
	if err := h.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "fanout", 0)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if sink.count != 1 {
		t.Fatalf("sink count = %d, want 1", sink.count)
	}
	if h.WithAttrs([]slog.Attr{slog.String("k", "v")}) == nil {
		t.Fatal("WithAttrs returned nil")
	}
	if h.WithGroup("group") == nil {
		t.Fatal("WithGroup returned nil")
	}
}

func TestSetupLoggerSelectsFormatsAndLevels(t *testing.T) {
	for _, tc := range []struct {
		name   string
		level  string
		format string
	}{
		{name: "debug-json", level: "debug", format: "json"},
		{name: "warn-loki", level: "warning", format: "loki"},
		{name: "error-text", level: "error", format: "text"},
		{name: "default-text", level: "unknown", format: "unknown"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := SetupLogger(tc.level, tc.format, "", "", "", 0)
			if logger == nil {
				t.Fatal("SetupLogger returned nil")
			}
		})
	}
}

type recordingLogSink struct {
	count int
}

func (s *recordingLogSink) Handle(slog.Record) { s.count++ }
func (s *recordingLogSink) Close() error       { return nil }
