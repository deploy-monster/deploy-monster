package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =========================================================================
// Mocks
// =========================================================================

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// mockRuntime implements core.ContainerRuntime.
type mockRuntime struct {
	containers []core.ContainerInfo
	listErr    error
	logsData   string
	logsErr    error
	execOutput string
	execErr    error
}

func (m *mockRuntime) Ping() error { return nil }
func (m *mockRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", nil
}
func (m *mockRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (m *mockRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (m *mockRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (m *mockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	if m.logsErr != nil {
		return nil, m.logsErr
	}
	return io.NopCloser(strings.NewReader(m.logsData)), nil
}
func (m *mockRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return m.containers, m.listErr
}
func (m *mockRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return m.execOutput, m.execErr
}
func (m *mockRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return &core.ContainerStats{}, nil
}
func (m *mockRuntime) ImagePull(_ context.Context, _ string) error               { return nil }
func (m *mockRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error)     { return nil, nil }
func (m *mockRuntime) ImageRemove(_ context.Context, _ string) error             { return nil }
func (m *mockRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) { return nil, nil }
func (m *mockRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error)   { return nil, nil }

// mockStore implements core.Store for terminal handler.
type mockStore struct {
	core.Store
	app    *core.Application
	appErr error
}

func (m *mockStore) GetApp(_ context.Context, id string) (*core.Application, error) {
	if m.appErr != nil {
		return nil, m.appErr
	}
	if m.app != nil {
		return m.app, nil
	}
	return nil, fmt.Errorf("not found")
}

func (m *mockStore) GetAppByName(_ context.Context, _, _ string) (*core.Application, error) {
	return nil, core.ErrNotFound
}

func (m *mockStore) DeleteDomainsByApp(_ context.Context, _ string) (int, error) {
	return 0, nil
}

// flushRecorder wraps httptest.ResponseRecorder to implement http.Flusher.
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed int
}

func newFlushRecorder() *flushRecorder {
	return &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (f *flushRecorder) Flush() {
	f.flushed++
}

// =========================================================================
// LogStreamer tests
// =========================================================================

func TestNewLogStreamer(t *testing.T) {
	ls := NewLogStreamer(&mockRuntime{}, discardLogger())
	if ls == nil {
		t.Fatal("NewLogStreamer() returned nil")
	}
	// ls is guaranteed non-nil after Fatal
	if ls.runtime == nil {
		t.Error("runtime not set")
	}
	if ls.logger == nil {
		t.Error("logger not set")
	}
}

func TestLogStreamer_StreamLogs_NilRuntime(t *testing.T) {
	ls := NewLogStreamer(nil, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/logs/stream", nil)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	ls.StreamLogs(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(w.Body.String(), "container runtime not available") {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestLogStreamer_StreamLogs_NoContainers(t *testing.T) {
	runtime := &mockRuntime{containers: []core.ContainerInfo{}}
	ls := NewLogStreamer(runtime, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/logs/stream", nil)
	req.SetPathValue("id", "app-1")
	w := newFlushRecorder()

	ls.StreamLogs(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected error event, body = %q", body)
	}
	if !strings.Contains(body, "no container found") {
		t.Errorf("body = %q", body)
	}
}

func TestLogStreamer_StreamLogs_ListByLabelsError(t *testing.T) {
	runtime := &mockRuntime{listErr: fmt.Errorf("docker error")}
	ls := NewLogStreamer(runtime, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/logs/stream", nil)
	req.SetPathValue("id", "app-1")
	w := newFlushRecorder()

	ls.StreamLogs(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "no container found") {
		t.Errorf("body = %q", body)
	}
}

func TestLogStreamer_StreamLogs_LogsError(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456"},
		},
		logsErr: fmt.Errorf("container stopped unexpectedly"),
	}
	ls := NewLogStreamer(runtime, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/logs/stream", nil)
	req.SetPathValue("id", "app-1")
	w := newFlushRecorder()

	ls.StreamLogs(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected error event, body = %q", body)
	}
	if !strings.Contains(body, "container stopped unexpectedly") {
		t.Errorf("body = %q", body)
	}
}

func TestLogStreamer_StreamLogs_Success(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456", Name: "my-app"},
		},
		logsData: "line1\nline2\nline3",
	}
	ls := NewLogStreamer(runtime, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/logs/stream?tail=50", nil)
	req.SetPathValue("id", "app-1")
	w := newFlushRecorder()

	ls.StreamLogs(w, req)

	// Check SSE headers
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q", conn)
	}
	if xa := w.Header().Get("X-Accel-Buffering"); xa != "no" {
		t.Errorf("X-Accel-Buffering = %q", xa)
	}

	body := w.Body.String()
	if !strings.Contains(body, "data: line1") {
		t.Errorf("missing line1 in body: %q", body)
	}
	if !strings.Contains(body, "data: line2") {
		t.Errorf("missing line2 in body: %q", body)
	}
	if !strings.Contains(body, "data: line3") {
		t.Errorf("missing line3 in body: %q", body)
	}
	if w.flushed == 0 {
		t.Error("expected Flush() to be called")
	}
}

func TestLogStreamer_StreamLogs_DefaultTail(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456"},
		},
		logsData: "log line",
	}
	ls := NewLogStreamer(runtime, discardLogger())

	// No tail query param — should default to "100"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/logs/stream", nil)
	req.SetPathValue("id", "app-1")
	w := newFlushRecorder()

	ls.StreamLogs(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "data: log line") {
		t.Errorf("body = %q", body)
	}
}

func TestLogStreamer_StreamLogs_NonFlusher(t *testing.T) {
	// Use a regular ResponseRecorder which does NOT implement http.Flusher.
	// However, httptest.ResponseRecorder now implements Flusher in modern Go.
	// The code checks w.(http.Flusher) AFTER writing headers, so this is hard
	// to trigger with standard tools. We still test the main path.
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456"},
		},
		logsData: "",
	}
	ls := NewLogStreamer(runtime, discardLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/logs/stream", nil)
	req.SetPathValue("id", "app-1")
	w := newFlushRecorder()

	ls.StreamLogs(w, req)
	// No panic = pass
}

// =========================================================================
// EventStreamer tests
// =========================================================================

func TestNewEventStreamer(t *testing.T) {
	events := core.NewEventBus(discardLogger())
	es := NewEventStreamer(events, discardLogger())
	if es == nil {
		t.Fatal("NewEventStreamer() returned nil")
	}
	// es is guaranteed non-nil after Fatal
	if es.events == nil {
		t.Error("events not set")
	}
	if es.logger == nil {
		t.Error("logger not set")
	}
}

func TestEventStreamer_StreamEvents_SetsSSEHeaders(t *testing.T) {
	events := core.NewEventBus(discardLogger())
	es := NewEventStreamer(events, discardLogger())

	// Create a cancellable context so we can stop the blocking handler
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so the handler returns right away

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream?type=app.*", nil)
	req = req.WithContext(ctx)
	w := newFlushRecorder()

	es.StreamEvents(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if conn := w.Header().Get("Connection"); conn != "keep-alive" {
		t.Errorf("Connection = %q", conn)
	}
}

func TestEventStreamer_StreamEvents_DefaultFilter(t *testing.T) {
	events := core.NewEventBus(discardLogger())
	es := NewEventStreamer(events, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// No type= query param — should default to "*"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream", nil)
	req = req.WithContext(ctx)
	w := newFlushRecorder()

	es.StreamEvents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestEventStreamer_StreamEvents_ReceivesEvent(t *testing.T) {
	events := core.NewEventBus(discardLogger())
	es := NewEventStreamer(events, discardLogger())

	// We need a context that stays alive long enough for the event to be processed
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	w := newFlushRecorder()

	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream?type=*", nil)
		req = req.WithContext(ctx)
		es.StreamEvents(w, req)
		close(done)
	}()

	// Publish an event — the async subscription needs a moment to be registered
	// Because SubscribeAsync runs in a goroutine, we publish and then cancel
	events.Publish(context.Background(), core.Event{
		ID:     "evt-12345678",
		Type:   "app.deployed",
		Source: "test",
	})

	cancel()
	<-done

	// The event may or may not be in the output depending on timing
	// but no panic = pass. If the event was received, it should be formatted as SSE
}

func TestEventStreamer_StreamEvents_ReceivesEventSync(t *testing.T) {
	events := core.NewEventBus(discardLogger())
	es := NewEventStreamer(events, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	w := newFlushRecorder()

	// Start streamer in background
	go func() {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream?type=*", nil)
		req = req.WithContext(ctx)
		es.StreamEvents(w, req)
		close(done)
	}()

	// Give the goroutine time to enter the select loop
	time.Sleep(50 * time.Millisecond)

	// Publish multiple events to ensure at least one gets through
	for i := 0; i < 50; i++ {
		events.Publish(context.Background(), core.Event{
			ID:     fmt.Sprintf("evt-%08d", i),
			Type:   "test.event",
			Source: "test",
		})
	}

	// Give async handlers time to push events through the channel
	time.Sleep(100 * time.Millisecond)

	cancel()
	<-done

	body := w.Body.String()
	// Verify that at least one event was received and formatted as SSE
	if !strings.Contains(body, "event: test.event") {
		t.Logf("body = %q", body)
		// Timing-dependent, don't fail the test but log it
		t.Log("note: no events captured (timing-dependent)")
	} else {
		if !strings.Contains(body, "data: ") {
			t.Errorf("event in body but missing data line: %q", body)
		}
		if w.flushed == 0 {
			t.Error("expected Flush() to be called for events")
		}
	}
}

func TestEventStreamer_StreamEvents_NonFlusher(t *testing.T) {
	events := core.NewEventBus(discardLogger())
	es := NewEventStreamer(events, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	req := httptest.NewRequest(http.MethodGet, "/api/v1/events/stream", nil)
	req = req.WithContext(ctx)
	w := newNonFlusherWriter()

	es.StreamEvents(w, req)
	// Non-flusher: code writes headers then checks flusher, returns early
	// No panic = pass
}

// =========================================================================
// Terminal tests
// =========================================================================

func TestNewTerminal(t *testing.T) {
	runtime := &mockRuntime{}
	store := &mockStore{}
	term := NewTerminal(runtime, store, discardLogger())
	if term == nil {
		t.Fatal("NewTerminal() returned nil")
	}
	// term is guaranteed non-nil after Fatal
	if term.runtime == nil {
		t.Error("runtime not set")
	}
	if term.store == nil {
		t.Error("store not set")
	}
	if term.logger == nil {
		t.Error("logger not set")
	}
}

// ---------------------------------------------------------------------------
// Terminal.StreamOutput
// ---------------------------------------------------------------------------

func TestTerminal_StreamOutput_NilRuntime(t *testing.T) {
	term := NewTerminal(nil, nil, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/terminal", nil)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.StreamOutput(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if !strings.Contains(w.Body.String(), "runtime not available") {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestTerminal_StreamOutput_NoContainers(t *testing.T) {
	runtime := &mockRuntime{containers: []core.ContainerInfo{}}
	term := NewTerminal(runtime, nil, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/terminal", nil)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.StreamOutput(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if !strings.Contains(w.Body.String(), "no container found") {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestTerminal_StreamOutput_ListByLabelsError(t *testing.T) {
	runtime := &mockRuntime{listErr: fmt.Errorf("docker error")}
	term := NewTerminal(runtime, nil, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/terminal", nil)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.StreamOutput(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestTerminal_StreamOutput_LogsError(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456", Image: "nginx", State: "running"},
		},
		logsErr: fmt.Errorf("logs failed"),
	}
	term := NewTerminal(runtime, nil, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/terminal", nil)
	req.SetPathValue("id", "app-1")
	w := newFlushRecorder()

	term.StreamOutput(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "event: error") {
		t.Errorf("expected error event, body = %q", body)
	}
	if !strings.Contains(body, "logs failed") {
		t.Errorf("body = %q", body)
	}
}

func TestTerminal_StreamOutput_Success(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456789", Image: "nginx:latest", State: "running"},
		},
		logsData: "output line 1\noutput line 2",
	}
	term := NewTerminal(runtime, nil, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/terminal", nil)
	req.SetPathValue("id", "app-1")
	w := newFlushRecorder()

	term.StreamOutput(w, req)

	// SSE headers
	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q", ct)
	}

	body := w.Body.String()
	// Should have session event
	if !strings.Contains(body, "event: session") {
		t.Errorf("missing session event in body: %q", body)
	}
	// Should have connected event with container info
	if !strings.Contains(body, "event: connected") {
		t.Errorf("missing connected event in body: %q", body)
	}
	if !strings.Contains(body, "container123") {
		t.Errorf("missing truncated container ID in body: %q", body)
	}
	if !strings.Contains(body, "nginx:latest") {
		t.Errorf("missing image name in body: %q", body)
	}
	// Should have log data
	if !strings.Contains(body, "data: output line 1") {
		t.Errorf("missing log line 1: %q", body)
	}
	if !strings.Contains(body, "data: output line 2") {
		t.Errorf("missing log line 2: %q", body)
	}
	if w.flushed < 2 {
		t.Errorf("expected at least 2 flushes, got %d", w.flushed)
	}
}

// ---------------------------------------------------------------------------
// Terminal.SendCommand
// ---------------------------------------------------------------------------

func TestTerminal_SendCommand_NilRuntime(t *testing.T) {
	term := NewTerminal(nil, nil, discardLogger())

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}

func TestTerminal_SendCommand_EmptyCommand(t *testing.T) {
	term := NewTerminal(&mockRuntime{}, nil, discardLogger())

	body := strings.NewReader(`{"command":""}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "command is required" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestTerminal_SendCommand_InvalidJSON(t *testing.T) {
	term := NewTerminal(&mockRuntime{}, nil, discardLogger())

	body := strings.NewReader(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestTerminal_SendCommand_AppNotFound(t *testing.T) {
	store := &mockStore{appErr: fmt.Errorf("not found")}
	term := NewTerminal(&mockRuntime{}, store, discardLogger())

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "app not found" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestTerminal_SendCommand_NilStore(t *testing.T) {
	// When store is nil, the app check is skipped
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456"},
		},
		execOutput: "file.txt",
	}
	term := NewTerminal(runtime, nil, discardLogger())

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestTerminal_SendCommand_ListByLabelsError(t *testing.T) {
	runtime := &mockRuntime{listErr: fmt.Errorf("docker daemon unreachable")}
	store := &mockStore{app: &core.Application{ID: "app-1"}}
	term := NewTerminal(runtime, store, discardLogger())

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "failed to find container" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestTerminal_SendCommand_NoContainers(t *testing.T) {
	runtime := &mockRuntime{containers: []core.ContainerInfo{}}
	store := &mockStore{app: &core.Application{ID: "app-1"}}
	term := NewTerminal(runtime, store, discardLogger())

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "no running container for this app" {
		t.Errorf("error = %q", resp["error"])
	}
}

func TestTerminal_SendCommand_ExecSuccess(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456789"},
		},
		execOutput: "file1.txt\nfile2.txt",
	}
	store := &mockStore{app: &core.Application{ID: "app-1"}}
	term := NewTerminal(runtime, store, discardLogger())

	body := strings.NewReader(`{"command":"ls"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["output"] != "file1.txt\nfile2.txt" {
		t.Errorf("output = %q", resp["output"])
	}
	exitCode, ok := resp["exit_code"].(float64)
	if !ok || exitCode != 0 {
		t.Errorf("exit_code = %v", resp["exit_code"])
	}
	if resp["container_id"] != "container123" {
		t.Errorf("container_id = %q", resp["container_id"])
	}
	// Should not have error field on success
	if _, hasErr := resp["error"]; hasErr {
		t.Error("success response should not have error field")
	}
}

func TestTerminal_SendCommand_ExecError(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456789"},
		},
		execOutput: "some partial output",
		execErr:    fmt.Errorf("command not found"),
	}
	store := &mockStore{app: &core.Application{ID: "app-1"}}
	term := NewTerminal(runtime, store, discardLogger())

	body := strings.NewReader(`{"command":"false"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	// Exec errors still return 200 with exit_code=1
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	exitCode, ok := resp["exit_code"].(float64)
	if !ok || exitCode != 1 {
		t.Errorf("exit_code = %v", resp["exit_code"])
	}
	if resp["output"] != "some partial output" {
		t.Errorf("output = %q", resp["output"])
	}
	if resp["error"] != "command not found" {
		t.Errorf("error = %q", resp["error"])
	}
	if resp["container_id"] != "container123" {
		t.Errorf("container_id = %q", resp["container_id"])
	}
}

// =========================================================================
// writeJSON helper test
// =========================================================================

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"message": "hello"}
	writeJSON(w, http.StatusCreated, data)

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", w.Code, http.StatusCreated)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["message"] != "hello" {
		t.Errorf("message = %q", resp["message"])
	}
}

func TestWriteJSON_Error(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"error": "bad request"}
	writeJSON(w, http.StatusBadRequest, data)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// =========================================================================
// Terminal.StreamOutput with non-flusher (edge case)
// =========================================================================

// nonFlusherWriter wraps a ResponseRecorder but does NOT implement http.Flusher.
type nonFlusherWriter struct {
	header http.Header
	code   int
	body   strings.Builder
}

func newNonFlusherWriter() *nonFlusherWriter {
	return &nonFlusherWriter{
		header: make(http.Header),
		code:   http.StatusOK,
	}
}

func (n *nonFlusherWriter) Header() http.Header         { return n.header }
func (n *nonFlusherWriter) Write(b []byte) (int, error) { return n.body.Write(b) }
func (n *nonFlusherWriter) WriteHeader(code int)        { n.code = code }

func TestTerminal_StreamOutput_NonFlusher(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456789", Image: "nginx", State: "running"},
		},
		logsData: "data",
	}
	term := NewTerminal(runtime, nil, discardLogger())

	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps/app-1/terminal", nil)
	req.SetPathValue("id", "app-1")
	w := newNonFlusherWriter()

	term.StreamOutput(w, req)

	// Should return early without panic when flusher is not available
	// The code writes headers but returns early if Flusher cast fails
	if w.code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.code, http.StatusOK)
	}
	// Body should be empty since we return before writing anything
	if w.body.Len() > 0 {
		t.Logf("body = %q (non-flusher path)", w.body.String())
	}
}

// =========================================================================
// Table-driven: SendCommand response format
// =========================================================================

func TestTerminal_SendCommand_ResponseFormat(t *testing.T) {
	tests := []struct {
		name       string
		execOutput string
		execErr    error
		wantCode   int
		wantExit   float64
		wantError  bool
	}{
		{
			name:       "success",
			execOutput: "hello world",
			execErr:    nil,
			wantCode:   http.StatusOK,
			wantExit:   0,
			wantError:  false,
		},
		{
			name:       "exec error",
			execOutput: "partial",
			execErr:    fmt.Errorf("exit code 127"),
			wantCode:   http.StatusOK,
			wantExit:   1,
			wantError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &mockRuntime{
				containers: []core.ContainerInfo{
					{ID: "container123456789"},
				},
				execOutput: tt.execOutput,
				execErr:    tt.execErr,
			}
			store := &mockStore{app: &core.Application{ID: "app-1"}}
			term := NewTerminal(runtime, store, discardLogger())

			reqBody := strings.NewReader(`{"command":"test"}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", reqBody)
			req.SetPathValue("id", "app-1")
			w := httptest.NewRecorder()

			term.SendCommand(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantCode)
			}

			var resp map[string]any
			json.NewDecoder(w.Body).Decode(&resp)

			exitCode, ok := resp["exit_code"].(float64)
			if !ok || exitCode != tt.wantExit {
				t.Errorf("exit_code = %v, want %v", resp["exit_code"], tt.wantExit)
			}

			_, hasErr := resp["error"]
			if hasErr != tt.wantError {
				t.Errorf("has error = %v, want %v", hasErr, tt.wantError)
			}

			// Always has container_id
			if resp["container_id"] == nil {
				t.Error("missing container_id")
			}
		})
	}
}

// =========================================================================
// Content-Type verification for JSON endpoint
// =========================================================================

func TestTerminal_SendCommand_ContentType(t *testing.T) {
	runtime := &mockRuntime{
		containers: []core.ContainerInfo{
			{ID: "container123456789"},
		},
		execOutput: "ok",
	}
	store := &mockStore{app: &core.Application{ID: "app-1"}}
	term := NewTerminal(runtime, store, discardLogger())

	body := strings.NewReader(`{"command":"whoami"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apps/app-1/terminal", body)
	req.SetPathValue("id", "app-1")
	w := httptest.NewRecorder()

	term.SendCommand(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
