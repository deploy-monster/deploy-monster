package ws

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// StreamLogs — runtime nil returns 503 (line 36-37)
// =============================================================================

func TestFinal_StreamLogs_NilRuntime(t *testing.T) {
	ls := NewLogStreamer(nil, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/logs/stream", nil)
	req.SetPathValue("id", "app1")
	rr := httptest.NewRecorder()
	ls.StreamLogs(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// =============================================================================
// StreamLogs — no container found (line 57-60)
// =============================================================================

func TestFinal_StreamLogs_NoContainer(t *testing.T) {
	rt := &mockRuntime{containers: nil}
	ls := NewLogStreamer(rt, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/logs/stream", nil)
	req.SetPathValue("id", "app1")
	rr := newFlushRecorder()
	ls.StreamLogs(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "no container found") {
		t.Errorf("expected 'no container found' in body, got %q", body)
	}
}

// =============================================================================
// StreamLogs — Logs() returns error (line 71-73)
// =============================================================================

func TestFinal_StreamLogs_LogsError(t *testing.T) {
	rt := &mockRuntime{
		containers: []core.ContainerInfo{{ID: "cnt-123"}},
		logsErr:    errors.New("log read failure"),
	}
	ls := NewLogStreamer(rt, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/logs/stream", nil)
	req.SetPathValue("id", "app1")
	rr := newFlushRecorder()
	ls.StreamLogs(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "log read failure") {
		t.Errorf("expected error message in body, got %q", body)
	}
}

// =============================================================================
// StreamLogs — successful log streaming with data lines (line 78-82)
// =============================================================================

func TestFinal_StreamLogs_Success(t *testing.T) {
	rt := &mockRuntime{
		containers: []core.ContainerInfo{{ID: "cnt-123"}},
		logsData:   "line1\nline2\n",
	}
	ls := NewLogStreamer(rt, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/logs/stream?tail=50", nil)
	req.SetPathValue("id", "app1")
	rr := newFlushRecorder()
	ls.StreamLogs(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "data: line1") {
		t.Errorf("expected SSE data with 'line1', got %q", body)
	}
	if !strings.Contains(body, "data: line2") {
		t.Errorf("expected SSE data with 'line2', got %q", body)
	}
}

// =============================================================================
// StreamLogs — ListByLabels returns error (err path, line 57)
// =============================================================================

func TestFinal_StreamLogs_ListError(t *testing.T) {
	rt := &mockRuntime{listErr: errors.New("docker unavailable")}
	ls := NewLogStreamer(rt, discardLogger())

	req := httptest.NewRequest("GET", "/api/v1/apps/app1/logs/stream", nil)
	req.SetPathValue("id", "app1")
	rr := newFlushRecorder()
	ls.StreamLogs(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "no container found") {
		t.Errorf("expected 'no container found', got %q", body)
	}
}

// =============================================================================
// StreamEvents — context cancel (line 132, exercises ctx.Done select case)
// =============================================================================

func TestFinal_StreamEvents_ContextCancel(t *testing.T) {
	events := core.NewEventBus(discardLogger())
	es := NewEventStreamer(events, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req := httptest.NewRequest("GET", "/api/v1/events/stream?type=app.*", nil)
	req = req.WithContext(ctx)

	rr := newFlushRecorder()
	es.StreamEvents(rr, req)

	// Should return without blocking since context is already cancelled
}
