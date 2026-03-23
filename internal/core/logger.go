package core

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

// SetupLogger configures structured JSON logging for production
// or text logging for development.
func SetupLogger(level, format string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level:     lvl,
		AddSource: lvl == slog.LevelDebug,
	}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		handler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// NewLogWriter creates an io.Writer that writes to slog at the given level.
// Useful for streaming build logs through the structured logger.
type LogWriter struct {
	logger *slog.Logger
	level  slog.Level
	prefix string
}

func NewLogWriter(logger *slog.Logger, level slog.Level, prefix string) io.Writer {
	return &LogWriter{logger: logger, level: level, prefix: prefix}
}

func (w *LogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg == "" {
		return len(p), nil
	}
	w.logger.Log(context.Background(), w.level, w.prefix+msg)
	return len(p), nil
}
