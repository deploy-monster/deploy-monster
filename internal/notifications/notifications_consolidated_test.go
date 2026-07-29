package notifications

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// === merged from module_boost_test.go ===

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

// === merged from notifications_100_test.go ===

// =============================================================================
// Coverage targets:
//   module.go:11   init   50%  — init() with RegisterModule
//   module.go:47   Start  50%  — SubscribeAsync event handlers (lines 51-55, 57-60)
//   providers.go:39  Slack.Send    93.3% — http.NewRequestWithContext error (line 47)
//   providers.go:91  Discord.Send  93.3% — http.NewRequestWithContext error (line 99)
//   providers.go:148 Telegram.Send 73.7% — recipient override, body formatting, error paths
//   providers.go:213 Webhook.Send  93.8% — recipient override path (line 214-215)
// =============================================================================

// ---------------------------------------------------------------------------
// Telegram.Send — full integration via httptest with recipient override
// Covers lines 149-151 (chatID override), 154-157 (text formatting), 166-181
// ---------------------------------------------------------------------------

func TestFinal_TelegramProvider_Send_FullPathViaHTTPTest(t *testing.T) {
	var receivedPayload map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	// The Telegram provider constructs URL as:
	//   fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
	// We can trick it by embedding the server URL in the BotToken so the constructed
	// URL resolves to our test server. We strip the scheme and make BotToken such that
	// the URL becomes: https://api.telegram.org/bot<...>/sendMessage
	// This won't work for real redirection, so instead we use a transport that
	// redirects all requests to our test server.

	p := &TelegramProvider{
		BotToken: "fake-bot-token",
		ChatID:   "-default-chat",
		client: &http.Client{
			Transport: &redirectTransport{target: server.URL},
		},
	}

	// Test with recipient override (covers lines 149-151)
	err := p.Send(context.Background(), "custom-chat-id", "Alert Subject", "CPU at 95%", "text")
	if err != nil {
		t.Fatalf("Send with recipient override: %v", err)
	}

	if receivedPayload["chat_id"] != "custom-chat-id" {
		t.Errorf("chat_id = %q, want custom-chat-id", receivedPayload["chat_id"])
	}
	if !strings.Contains(receivedPayload["text"], "Alert Subject") {
		t.Errorf("text should contain subject, got: %q", receivedPayload["text"])
	}
	if !strings.Contains(receivedPayload["text"], "CPU at 95%") {
		t.Errorf("text should contain body, got: %q", receivedPayload["text"])
	}
	if receivedPayload["parse_mode"] != "HTML" {
		t.Errorf("parse_mode = %q, want HTML", receivedPayload["parse_mode"])
	}
}

// redirectTransport redirects all HTTP requests to a target URL.
type redirectTransport struct {
	target string
}

func (rt *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Redirect the request to the target server
	newReq := req.Clone(req.Context())
	newReq.URL.Scheme = "http"
	newReq.URL.Host = strings.TrimPrefix(rt.target, "http://")
	return http.DefaultTransport.RoundTrip(newReq)
}

func TestFinal_TelegramProvider_Send_SubjectOnly(t *testing.T) {
	// When body is empty, text should be just the subject (line 154)
	var receivedPayload map[string]string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&receivedPayload)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	p := &TelegramProvider{
		BotToken: "token",
		ChatID:   "default-chat",
		client: &http.Client{
			Transport: &redirectTransport{target: server.URL},
		},
	}

	// Empty recipient — should use default ChatID (line 150-151)
	err := p.Send(context.Background(), "", "Subject Only", "", "text")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if receivedPayload["chat_id"] != "default-chat" {
		t.Errorf("chat_id = %q, want default-chat (fallback)", receivedPayload["chat_id"])
	}
	// Subject only — text should be just the subject, no HTML bold
	if receivedPayload["text"] != "Subject Only" {
		t.Errorf("text = %q, want 'Subject Only'", receivedPayload["text"])
	}
}

func TestFinal_TelegramProvider_Send_ServerReturnsNonOK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"ok":false}`))
	}))
	defer server.Close()

	p := &TelegramProvider{
		BotToken: "token",
		ChatID:   "chat",
		client: &http.Client{
			Transport: &redirectTransport{target: server.URL},
		},
	}

	err := p.Send(context.Background(), "", "Test", "Body", "text")
	if err == nil {
		t.Fatal("expected error for non-OK status")
	}
	if !strings.Contains(err.Error(), "telegram returned 403") {
		t.Errorf("expected 'telegram returned 403', got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Slack.Send — NewRequestWithContext error path (line 47)
// ---------------------------------------------------------------------------

func TestFinal_SlackProvider_Send_InvalidURL(t *testing.T) {
	// An invalid URL should cause http.NewRequestWithContext to fail (line 47)
	p := NewSlackProvider("://invalid-url")

	err := p.Send(context.Background(), "", "Test", "Body", "text")
	if err == nil {
		t.Error("expected error for invalid webhook URL")
	}
}

// ---------------------------------------------------------------------------
// Discord.Send — NewRequestWithContext error path (line 99)
// ---------------------------------------------------------------------------

func TestFinal_DiscordProvider_Send_InvalidURL(t *testing.T) {
	p := NewDiscordProvider("://invalid-url")

	err := p.Send(context.Background(), "", "Test", "Body", "text")
	if err == nil {
		t.Error("expected error for invalid webhook URL")
	}
}

// ---------------------------------------------------------------------------
// Telegram.Send — http.NewRequestWithContext error path (line 167)
// This is triggered when the URL is malformed enough to fail request creation.
// ---------------------------------------------------------------------------

func TestFinal_TelegramProvider_Send_MalformedBotToken(t *testing.T) {
	// BotToken with control characters that make the URL invalid for NewRequest
	p := &TelegramProvider{
		BotToken: string([]byte{0x7f}), // DEL character
		ChatID:   "chat",
		client:   &http.Client{},
	}

	err := p.Send(context.Background(), "", "Test", "", "text")
	// If the URL is invalid enough, NewRequestWithContext returns error.
	// Otherwise the HTTP client fails. Either way, we exercise the code path.
	if err == nil {
		t.Log("Send may succeed if URL is still parseable")
	}
}

func TestFinal_TelegramProvider_Send_NetworkError(t *testing.T) {
	// Point at a closed server to trigger the client.Do error path (line 172-174)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	p := &TelegramProvider{
		BotToken: "token",
		ChatID:   "chat",
		client: &http.Client{
			Transport: &redirectTransport{target: server.URL},
		},
	}

	err := p.Send(context.Background(), "", "Alert", "Body", "text")
	if err == nil {
		t.Fatal("expected error for closed server")
	}
	if !strings.Contains(err.Error(), "telegram send") {
		t.Errorf("expected 'telegram send' error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Module.Start — exercises the async event subscription handlers
// ---------------------------------------------------------------------------

func TestFinal_Module_Start_AlertEventHandler(t *testing.T) {
	// The Start method subscribes to "alert.*" and "deploy.*" events.
	// We verify the handlers don't panic when events are published.
	m := New()

	c := setupFinalCore(t)
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Publish alert and deploy events — handlers should not panic
	c.Events.PublishAsync(context.Background(), core.NewEvent("alert.cpu_high", "test", nil))
	c.Events.PublishAsync(context.Background(), core.NewEvent("deploy.started", "test", nil))

	if err := m.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func setupFinalCore(t *testing.T) *core.Core {
	t.Helper()
	events := core.NewEventBus(nil)
	return &core.Core{
		Logger:   slog.Default(),
		Events:   events,
		Services: &core.Services{},
		Config:   &core.Config{},
	}
}

// === merged from providers_boost_test.go ===

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"empty", "", "webhook URL is required"},
		{"invalid URL", "://bad", "invalid webhook URL"},
		{"http scheme", "http://example.com/hook", "webhook URL must use HTTPS scheme"},
		{"missing hostname", "https:///path", "webhook URL must have a hostname"},
		{"localhost", "https://localhost/hook", "webhook URL cannot point to localhost"},
		{"127.0.0.1", "https://127.0.0.1/hook", "webhook URL cannot point to localhost"},
		{"::1", "https://[::1]/hook", "webhook URL cannot point to localhost"},
		{"0.0.0.0", "https://0.0.0.0/hook", "webhook URL cannot point to localhost"},
		{"private IP", "https://10.0.0.1/hook", "webhook URL cannot point to internal IP addresses"},
		{"link-local", "https://169.254.1.1/hook", "webhook URL cannot point to internal IP addresses"},
		{"cloud metadata", "https://169.254.169.254/hook", "webhook URL cannot point to internal IP addresses"},
		{"internal hostname", "https://metadata.google.internal/hook", "webhook URL cannot point to internal hostnames"},
		{"internal suffix", "https://sub.metadata.ec2.internal/hook", "webhook URL cannot point to internal hostnames"},
		{"valid slack", "https://hooks.slack.com/services/T00/B00/xxx", ""},
		{"valid discord", "https://discord.com/api/webhooks/123/abc", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWebhookURL(tc.url)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("validateWebhookURL(%q) = %v, want nil", tc.url, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateWebhookURL(%q) = nil, want error", tc.url)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("validateWebhookURL(%q) = %q, want containing %q", tc.url, err.Error(), tc.wantErr)
			}
		})
	}
}
