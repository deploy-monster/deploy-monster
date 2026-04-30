package ws

import (
	"bufio"
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// LogStreamer handles WebSocket connections for real-time log streaming.
// Since we want minimum dependencies, we use Server-Sent Events (SSE) instead
// of WebSocket as a simpler transport that requires no external library.
type LogStreamer struct {
	runtime core.ContainerRuntime
	logger  *slog.Logger
}

// NewLogStreamer creates a new log streamer.
func NewLogStreamer(runtime core.ContainerRuntime, logger *slog.Logger) *LogStreamer {
	return &LogStreamer{runtime: runtime, logger: logger}
}

// StreamLogs handles GET /api/v1/apps/{id}/logs/stream
// Uses Server-Sent Events for real-time log delivery.
func (ls *LogStreamer) StreamLogs(w http.ResponseWriter, r *http.Request) {
	appID := r.PathValue("id")
	tail := r.URL.Query().Get("tail")
	if tail == "" {
		tail = "100"
	}

	if ls.runtime == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "container runtime not available"})
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return // response already started; cannot send error status
	}

	// Find container for this app
	containers, err := ls.runtime.ListByLabels(r.Context(), map[string]string{
		"monster.app.id": appID,
	})
	if err != nil || len(containers) == 0 {
		_, _ = w.Write([]byte("event: error\ndata: no container found\n\n"))
		flusher.Flush()
		return
	}

	containerID := containers[0].ID

	// Stream logs
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	logReader, err := ls.runtime.Logs(ctx, containerID, tail, true)
	if err != nil {
		_, _ = w.Write([]byte("event: error\ndata: " + err.Error() + "\n\n"))
		flusher.Flush()
		return
	}
	defer func() { _ = logReader.Close() }()

	scanner := bufio.NewScanner(logReader)
	for scanner.Scan() {
		line := scanner.Text()
		_, _ = w.Write([]byte("data: " + line + "\n\n"))
		flusher.Flush()
	}
}

// EventStreamer streams system events to the client.
type EventStreamer struct {
	events *core.EventBus
	logger *slog.Logger
}

// NewEventStreamer creates a new event streamer.
func NewEventStreamer(events *core.EventBus, logger *slog.Logger) *EventStreamer {
	return &EventStreamer{events: events, logger: logger}
}

// StreamEvents handles GET /api/v1/events/stream
// Streams all system events matching optional type filter.
func (es *EventStreamer) StreamEvents(w http.ResponseWriter, r *http.Request) {
	typeFilter := r.URL.Query().Get("type")
	if typeFilter == "" {
		typeFilter = "*"
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return // response already started; cannot send error status
	}

	// Channel to receive events
	ch := make(chan core.Event, 100)

	// Single timer reused for keepalive pings — avoids allocating a new timer
	// on every iteration (anti-pattern with time.After in loops).
	timer := time.NewTimer(30 * time.Second)
	defer timer.Stop()

	// Subscribe to events and get subscription ID for cleanup
	subID := es.events.SubscribeAsync(typeFilter, func(_ context.Context, event core.Event) error {
		select {
		case ch <- event:
		default:
			// Drop if buffer full
		}
		return nil
	})

	// Reset helper to restart the keepalive timer after each ping or event.
	resetTimer := func() {
		if !timer.Stop() {
			<-timer.C // Drain expired channel if stopped
		}
		timer.Reset(30 * time.Second)
	}

	// Stream events until client disconnects
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			// Clean up subscription to prevent goroutine leak
			es.events.Unsubscribe(subID)
			return
		case event := <-ch:
			data := event.DebugString()
			_, _ = w.Write([]byte("event: " + event.Type + "\ndata: " + data + "\n\n"))
			flusher.Flush()
			resetTimer()
		case <-timer.C:
			// Keepalive ping — reset and continue
			_, _ = w.Write([]byte(": keepalive\n\n"))
			flusher.Flush()
			resetTimer()
		}
	}
}
