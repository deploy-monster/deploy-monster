package build

import (
	"context"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Default ring buffer size — enough for a typical Docker build output
// (a few hundred lines) without letting runaway builds bloat memory.
const defaultLogLines = 1000

// LogStore keeps a bounded ring buffer of build log lines per app, and
// optionally emits a core.EventBuildLog event per line so SSE / WebSocket
// endpoints can stream the build output live.
//
// Pre-Tier-Phase-1 the pipeline piped build output to io.Discard and
// operators had no way to see why a build failed. LogStore replaces
// that sink with something queryable.
type LogStore struct {
	mu     sync.RWMutex
	bufs   map[string]*logRing // app ID -> ring
	events *core.EventBus      // optional; nil = no event emission
	lines  int                 // ring capacity per app
}

// NewLogStore creates a log store. Pass events = nil to suppress event
// emission (useful in tests). linesPerApp <= 0 uses the default.
func NewLogStore(events *core.EventBus, linesPerApp int) *LogStore {
	if linesPerApp <= 0 {
		linesPerApp = defaultLogLines
	}
	return &LogStore{
		bufs:   make(map[string]*logRing),
		events: events,
		lines:  linesPerApp,
	}
}

// Writer returns an io.Writer that appends newline-delimited lines to
// the ring buffer for the given app ID and (if events != nil) publishes
// a build.log event per line. The returned writer is safe for
// concurrent use from a single goroutine — the underlying docker build
// output reader calls Write from one goroutine.
func (s *LogStore) Writer(appID string) io.Writer {
	return &logWriter{store: s, appID: appID}
}

// Lines returns a snapshot of the last N buffered log lines for appID.
// Returns nil if no lines have been captured.
func (s *LogStore) Lines(appID string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ring, ok := s.bufs[appID]
	if !ok {
		return nil
	}
	return ring.snapshot()
}

// Reset clears the ring buffer for appID. Called by the pipeline at
// the start of each new build so logs from the previous run don't
// bleed into the current one.
func (s *LogStore) Reset(appID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.bufs, appID)
}

func (s *LogStore) append(appID, line string) {
	s.mu.Lock()
	ring, ok := s.bufs[appID]
	if !ok {
		ring = newLogRing(s.lines)
		s.bufs[appID] = ring
	}
	ring.push(line)
	events := s.events
	s.mu.Unlock()

	if events != nil {
		_ = events.Publish(context.Background(), core.NewEvent(
			core.EventBuildLog, "build",
			core.BuildLogEventData{
				AppID:     appID,
				Line:      line,
				Timestamp: time.Now().Unix(),
			},
		))
	}
}

// logWriter splits arbitrary byte chunks into newline-terminated lines
// and forwards them to the store. Partial final lines are held in a
// buffer until the next Write completes them.
type logWriter struct {
	store   *LogStore
	appID   string
	pending strings.Builder
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.pending.Write(p)
	s := w.pending.String()
	w.pending.Reset()

	for {
		idx := strings.IndexByte(s, '\n')
		if idx < 0 {
			break
		}
		line := strings.TrimRight(s[:idx], "\r")
		if line != "" {
			w.store.append(w.appID, line)
		}
		s = s[idx+1:]
	}
	w.pending.WriteString(s)
	return len(p), nil
}

// logRing is a bounded FIFO of log lines. When full, pushing a new
// line drops the oldest one.
type logRing struct {
	buf  []string
	head int
	size int
	cap  int
}

func newLogRing(capacity int) *logRing {
	return &logRing{buf: make([]string, capacity), cap: capacity}
}

func (r *logRing) push(line string) {
	if r.size < r.cap {
		r.buf[(r.head+r.size)%r.cap] = line
		r.size++
		return
	}
	r.buf[r.head] = line
	r.head = (r.head + 1) % r.cap
}

func (r *logRing) snapshot() []string {
	out := make([]string, r.size)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(r.head+i)%r.cap]
	}
	return out
}
