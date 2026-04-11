package core

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestSetupLogger_JSONFormat(t *testing.T) {
	logger := SetupLogger("debug", "json")
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_LevelParsing(t *testing.T) {
	cases := []struct {
		name  string
		level string
	}{
		{"debug lowercase", "debug"},
		{"warn alias", "warn"},
		{"warning alias", "warning"},
		{"error", "error"},
		{"unknown falls back to info", "nope"},
		{"mixed case", "DEBUG"},
		{"empty falls back to info", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SetupLogger(tc.level, "text"); got == nil {
				t.Error("expected non-nil logger")
			}
		})
	}
}

func TestLogWriter_WritesNonEmptyLines(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)

	w := NewLogWriter(logger, slog.LevelInfo, "build: ")
	n, err := w.Write([]byte("hello world"))
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if n != len("hello world") {
		t.Errorf("Write returned %d, want %d", n, len("hello world"))
	}

	// Decode the single JSON line and verify the prefix + message landed.
	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("expected logger output, got empty buffer")
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		t.Fatalf("log line is not valid JSON: %v", err)
	}
	msg, _ := rec["msg"].(string)
	if msg != "build: hello world" {
		t.Errorf("msg = %q, want %q", msg, "build: hello world")
	}
}

func TestLogWriter_SkipsEmptyAndWhitespaceOnly(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	w := NewLogWriter(logger, slog.LevelInfo, "")

	// Whitespace-only should return len(p) but emit no log line.
	for _, input := range [][]byte{{}, []byte(" "), []byte("\n"), []byte("  \t \n")} {
		buf.Reset()
		n, err := w.Write(input)
		if err != nil {
			t.Fatalf("Write(%q) returned error: %v", input, err)
		}
		if n != len(input) {
			t.Errorf("Write(%q) n=%d, want %d", input, n, len(input))
		}
		if buf.Len() != 0 {
			t.Errorf("Write(%q) should have emitted nothing, got %q", input, buf.String())
		}
	}
}

func TestLogWriter_TrimsSurroundingWhitespace(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	w := NewLogWriter(logger, slog.LevelWarn, "")
	_, _ = w.Write([]byte("\n  warning message  \n"))

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("log line is not valid JSON: %v", err)
	}
	if msg, _ := rec["msg"].(string); msg != "warning message" {
		t.Errorf("msg = %q, want trimmed %q", msg, "warning message")
	}
}
