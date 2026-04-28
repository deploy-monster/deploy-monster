package notifications

import (
	"context"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_dispatchAlert(t *testing.T) {
	m := New()
	m.dispatcher = NewDispatcher()
	m.logger = slog.Default()

	mock := &mockProvider{name: "slack"}
	m.dispatcher.RegisterProvider(mock)

	m.dispatchAlert(context.Background(), core.Event{
		Type: "alert.critical",
		Data: core.AlertEventData{
			Name:     "Disk Full",
			Message:  "Disk usage exceeded 90%",
			Resource: "server-1",
			Severity: "critical",
		},
	})

	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 alert sent, got %d", len(mock.sent))
	}
	if mock.sent[0] != "[CRITICAL] Disk Full" {
		t.Errorf("subject = %q, want [CRITICAL] Disk Full", mock.sent[0])
	}
}

func TestModule_dispatchAlert_WarningSeverity(t *testing.T) {
	m := New()
	m.dispatcher = NewDispatcher()
	m.logger = slog.Default()

	mock := &mockProvider{name: "discord"}
	m.dispatcher.RegisterProvider(mock)

	m.dispatchAlert(context.Background(), core.Event{
		Type: "alert.warning",
		Data: core.AlertEventData{
			Name:     "High CPU",
			Message:  "CPU usage high",
			Resource: "app-1",
			Severity: "warning",
		},
	})

	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 alert sent, got %d", len(mock.sent))
	}
	if mock.sent[0] != "[WARNING] High CPU" {
		t.Errorf("subject = %q, want [WARNING] High CPU", mock.sent[0])
	}
}

func TestModule_dispatchAlert_InfoSeverity(t *testing.T) {
	m := New()
	m.dispatcher = NewDispatcher()
	m.logger = slog.Default()

	mock := &mockProvider{name: "slack"}
	m.dispatcher.RegisterProvider(mock)

	m.dispatchAlert(context.Background(), core.Event{
		Type: "alert.info",
		Data: core.AlertEventData{
			Name:     "Deploy Done",
			Message:  "Deployment completed",
			Resource: "app-1",
			Severity: "info",
		},
	})

	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 alert sent, got %d", len(mock.sent))
	}
	if mock.sent[0] != "[INFO] Deploy Done" {
		t.Errorf("subject = %q, want [INFO] Deploy Done", mock.sent[0])
	}
}

func TestModule_dispatchAlert_UnknownSeverity(t *testing.T) {
	m := New()
	m.dispatcher = NewDispatcher()
	m.logger = slog.Default()

	mock := &mockProvider{name: "slack"}
	m.dispatcher.RegisterProvider(mock)

	m.dispatchAlert(context.Background(), core.Event{
		Type: "alert.unknown",
		Data: core.AlertEventData{
			Name:     "Something",
			Message:  "Happened",
			Resource: "app-1",
			Severity: "unknown",
		},
	})

	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 alert sent, got %d", len(mock.sent))
	}
	if mock.sent[0] != "[ALERT] Something" {
		t.Errorf("subject = %q, want [ALERT] Something", mock.sent[0])
	}
}

func TestModule_dispatchAlert_WrongDataType(t *testing.T) {
	m := New()
	m.dispatcher = NewDispatcher()
	m.logger = slog.Default()

	mock := &mockProvider{name: "slack"}
	m.dispatcher.RegisterProvider(mock)

	m.dispatchAlert(context.Background(), core.Event{
		Type: "alert.critical",
		Data: "not alert data",
	})

	if len(mock.sent) != 0 {
		t.Errorf("expected 0 alerts sent for bad data type, got %d", len(mock.sent))
	}
}

func TestModule_dispatchAlert_ProviderError(t *testing.T) {
	m := New()
	m.dispatcher = NewDispatcher()
	m.logger = slog.Default()

	failing := &mockProvider{name: "failing", sendErr: context.Canceled}
	m.dispatcher.RegisterProvider(failing)

	// Should not panic even when provider returns error
	m.dispatchAlert(context.Background(), core.Event{
		Type: "alert.critical",
		Data: core.AlertEventData{
			Name:     "Disk Full",
			Message:  "Disk usage exceeded 90%",
			Resource: "server-1",
			Severity: "critical",
		},
	})

	if len(failing.sent) != 1 {
		t.Errorf("expected 1 tracked send even on error, got %d", len(failing.sent))
	}
}

func TestModule_dispatchAlert_NoProviders(t *testing.T) {
	m := New()
	m.dispatcher = NewDispatcher()
	m.logger = slog.Default()

	// Should not panic with no providers registered
	m.dispatchAlert(context.Background(), core.Event{
		Type: "alert.critical",
		Data: core.AlertEventData{
			Name:     "Disk Full",
			Message:  "Disk usage exceeded 90%",
			Resource: "server-1",
			Severity: "critical",
		},
	})
}
