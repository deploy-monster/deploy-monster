package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// LogSink is an optional destination for structured log entries.
// Implementations can forward logs to Loki, Datadog, Splunk, etc.
type LogSink interface {
	// Handle emits a log record. Implementations must be safe for
	// concurrent use and non-blocking (or timeout internally).
	Handle(r slog.Record)
	// Close releases any resources held by the sink.
	Close() error
}

// LokiSink forwards logs to a Loki endpoint using the HTTP push API.
type LokiSink struct {
	url      string
	username string
	password string
	timeout  time.Duration
	client   *http.Client
	wg       sync.WaitGroup
	ch       chan slog.Record
	logger   *slog.Logger
	stopOnce sync.Once
	stopCh   chan struct{}
}

// NewLokiSink creates a Loki log sink. The url should be the full Loki
// push API endpoint, e.g. http://loki:3100/loki/api/v1/push.
// A nil logger is tolerated and replaced with the default slog logger.
func NewLokiSink(url, username, password string, timeout time.Duration, logger *slog.Logger) *LokiSink {
	if logger == nil {
		logger = slog.Default()
	}
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ch := make(chan slog.Record, 1024)
	ls := &LokiSink{
		url:      url,
		username: username,
		password: password,
		timeout:  timeout,
		client:   &http.Client{Timeout: timeout},
		ch:       ch,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
	ls.wg.Add(1)
	go ls.loop()
	return ls
}

func (ls *LokiSink) loop() {
	defer ls.wg.Done()
	for {
		select {
		case r, ok := <-ls.ch:
			if !ok {
				ls.flush()
				return
			}
			ls.send(r)
		case <-ls.stopCh:
			close(ls.ch)
		}
	}
}

func (ls *LokiSink) Handle(r slog.Record) {
	select {
	case ls.ch <- r:
	default:
		// Drop if channel is full — don't block the caller.
		// In a healthy deployment the channel should never fill.
	}
}

func (ls *LokiSink) Close() error {
	ls.stopOnce.Do(func() { close(ls.stopCh) })
	ls.wg.Wait()
	return nil
}

func (ls *LokiSink) send(r slog.Record) {
	line := make(map[string]any)
	line["time"] = r.Time.Format(time.RFC3339Nano)
	line["level"] = r.Level.String()
	line["msg"] = r.Message
	attrs := make(map[string]any)
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	if len(attrs) > 0 {
		line["attrs"] = attrs
	}

	lineBytes, _ := json.Marshal(line)
	ts := fmt.Sprintf("%d", r.Time.UnixNano())
	payload := map[string]any{
		"streams": []map[string]any{
			{
				"stream": map[string]string{
					"job":      "deploymonster",
					"instance": "default",
				},
				"values": [][2]string{{ts, string(lineBytes)}},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest(http.MethodPost, ls.url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if ls.username != "" && ls.password != "" {
		req.SetBasicAuth(ls.username, ls.password)
	}
	resp, err := ls.client.Do(req)
	if err != nil {
		ls.logger.Warn("loki sink delivery failed", "error", err)
		return
	}
	resp.Body.Close()
}

func (ls *LokiSink) flush() {
	for {
		select {
		case r, ok := <-ls.ch:
			if !ok {
				return
			}
			ls.send(r)
		default:
			return
		}
	}
}

// multiHandler is a slog.Handler that fans out to one primary handler
// (stdout/stderr) and zero or more LogSink backends.
type multiHandler struct {
	primary slog.Handler
	sinks   []LogSink
	mu      sync.Mutex
}

func (mh *multiHandler) Enabled(_ context.Context, l slog.Level) bool {
	return mh.primary.Enabled(context.Background(), l)
}

func (mh *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	mh.mu.Lock()
	defer mh.mu.Unlock()
	if err := mh.primary.Handle(ctx, r); err != nil {
		return err
	}
	for _, s := range mh.sinks {
		s.Handle(r)
	}
	return nil
}

func (mh *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &multiHandler{primary: mh.primary.WithAttrs(attrs), sinks: mh.sinks}
}

func (mh *multiHandler) WithGroup(name string) slog.Handler {
	return &multiHandler{primary: mh.primary.WithGroup(name), sinks: mh.sinks}
}

// SetupLogger configures structured JSON logging for production
// or text logging for development. When lokiURL is set, logs are also
// forwarded to Loki via an async background sink.
func SetupLogger(level, format, lokiURL, lokiUsername, lokiPassword string, lokiTimeout time.Duration) *slog.Logger {
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

	var primaryHandler slog.Handler
	switch strings.ToLower(format) {
	case "json":
		primaryHandler = slog.NewJSONHandler(os.Stdout, opts)
	case "loki":
		primaryHandler = slog.NewJSONHandler(os.Stdout, opts)
	default:
		primaryHandler = slog.NewTextHandler(os.Stdout, opts)
	}

	var handler slog.Handler
	if lokiURL != "" {
		sink := NewLokiSink(lokiURL, lokiUsername, lokiPassword, lokiTimeout, nil)
		handler = &multiHandler{
			primary: primaryHandler,
			sinks:   []LogSink{sink},
		}
	} else {
		handler = primaryHandler
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
