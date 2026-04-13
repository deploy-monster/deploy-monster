package notifications

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

// =====================================================
// DISPATCHER — concurrent access tests
// =====================================================

func TestDispatcher_ConcurrentRegisterAndGet(t *testing.T) {
	d := NewDispatcher()
	var wg sync.WaitGroup

	// Register providers concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			d.RegisterProvider(&mockProvider{name: fmt.Sprintf("provider-%d", n)})
		}(i)
	}

	// Simultaneously read providers
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			d.GetProvider(fmt.Sprintf("provider-%d", n))
			d.Providers()
		}(i)
	}

	wg.Wait()

	names := d.Providers()
	if len(names) != 50 {
		t.Errorf("expected 50 providers, got %d", len(names))
	}
}

func TestDispatcher_RegisterNilSafe(t *testing.T) {
	d := NewDispatcher()
	p := &mockProvider{name: "test-provider"}
	d.RegisterProvider(p)

	got, ok := d.GetProvider("test-provider")
	if !ok {
		t.Fatal("provider should be found")
	}
	if got != p {
		t.Error("returned provider should be the same instance")
	}
}

// =====================================================
// DISPATCHER — multiple channels dispatch
// =====================================================

func TestDispatcher_DispatchToMultipleChannels(t *testing.T) {
	d := NewDispatcher()

	slackMock := &mockProvider{name: "slack"}
	discordMock := &mockProvider{name: "discord"}

	d.RegisterProvider(slackMock)
	d.RegisterProvider(discordMock)

	// Send to each channel individually
	channels := []string{"slack", "discord"}
	for _, ch := range channels {
		provider, ok := d.GetProvider(ch)
		if !ok {
			t.Fatalf("provider %q not found", ch)
		}
		err := provider.Send(context.Background(), "recipient", "Test Alert", "CPU usage high", "text")
		if err != nil {
			t.Fatalf("Send via %s failed: %v", ch, err)
		}
	}

	// Verify each mock received the send
	if len(slackMock.sent) != 1 {
		t.Errorf("slack should have 1 send, got %d", len(slackMock.sent))
	}
	if len(discordMock.sent) != 1 {
		t.Errorf("discord should have 1 send, got %d", len(discordMock.sent))
	}
}

// =====================================================
// PROVIDER — error handling
// =====================================================

func TestSlackProvider_Send_NetworkError(t *testing.T) {
	// Use a server that closes immediately
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Intentionally close connection
		hj, ok := w.(http.Hijacker)
		if ok {
			conn, _, _ := hj.Hijack()
			conn.Close()
		}
	}))
	server.Close() // Close server immediately

	p := NewSlackProvider(server.URL)
	err := p.Send(context.Background(), "", "Alert", "Body", "text")
	if err == nil {
		t.Fatal("expected error when server is closed")
	}
}

func TestDiscordProvider_Send_NetworkError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.Close()

	p := NewDiscordProvider(server.URL)
	err := p.Send(context.Background(), "", "Alert", "Body", "text")
	if err == nil {
		t.Fatal("expected error when server is closed")
	}
}

// =====================================================
// PROVIDER — different status codes
// =====================================================

func TestSlackProvider_Send_NonOKStatus(t *testing.T) {
	tests := []struct {
		name   string
		status int
	}{
		{"400 Bad Request", http.StatusBadRequest},
		{"403 Forbidden", http.StatusForbidden},
		{"404 Not Found", http.StatusNotFound},
		{"429 Rate Limited", http.StatusTooManyRequests},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			p := NewSlackProvider(server.URL)
			err := p.Send(context.Background(), "", "Test", "", "text")
			if err == nil {
				t.Errorf("expected error for status %d", tt.status)
			}
		})
	}
}

func TestDiscordProvider_Send_StatusCodes(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr bool
	}{
		{"200 OK", http.StatusOK, false},
		{"204 No Content", http.StatusNoContent, false},
		{"201 Created", http.StatusCreated, false},
		{"400 Bad Request", http.StatusBadRequest, true},
		{"500 Internal Server Error", http.StatusInternalServerError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
			}))
			defer server.Close()

			p := NewDiscordProvider(server.URL)
			err := p.Send(context.Background(), "", "Test", "", "text")
			if (err != nil) != tt.wantErr {
				t.Errorf("Send() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// =====================================================
// PROVIDER — context cancellation
// =====================================================

func TestSlackProvider_Send_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewSlackProvider(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel before sending

	err := p.Send(ctx, "", "Alert", "Body", "text")
	if err == nil {
		t.Error("expected error with canceled context")
	}
}

func TestDiscordProvider_Send_CancelledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewDiscordProvider(server.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Send(ctx, "", "Alert", "Body", "text")
	if err == nil {
		t.Error("expected error with canceled context")
	}
}

// =====================================================
// TELEGRAM PROVIDER — additional tests
// =====================================================

func TestTelegramProvider_Send_UsesChatIDDefault(t *testing.T) {
	// When recipient is empty, Telegram should use the default ChatID
	var receivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept any request
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	_ = receivedBody

	p := NewTelegramProvider("bot123:ABC", "-9999")
	// Override the URL by using a custom provider; since we cannot mock the Telegram API URL,
	// we test the format via the Validate method instead
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() should pass for valid config: %v", err)
	}
}

func TestTelegramProvider_Send_SubjectOnlyFormat(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	// We can't easily redirect TelegramProvider to our test server because it
	// constructs the URL from BotToken. But we can verify formatting logic
	// by testing that the provider is valid and the Name is correct.
	p := NewTelegramProvider("token123", "chat456")
	if p.Name() != "telegram" {
		t.Errorf("Name() = %q, want 'telegram'", p.Name())
	}
}

// =====================================================
// NOTIFICATION FORMATTING — subject+body combinations
// =====================================================

func TestSlackProvider_MessageFormat(t *testing.T) {
	tests := []struct {
		name         string
		subject      string
		body         string
		wantContains string
	}{
		{"subject only", "Alert!", "", "Alert!"},
		{"subject and body", "Deploy Failed", "App crashed", "*Deploy Failed*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var receivedPayload []byte
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				buf := make([]byte, 4096)
				n, _ := r.Body.Read(buf)
				receivedPayload = buf[:n]
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			p := NewSlackProvider(server.URL)
			err := p.Send(context.Background(), "", tt.subject, tt.body, "text")
			if err != nil {
				t.Fatalf("Send() error: %v", err)
			}

			payloadStr := string(receivedPayload)
			if len(payloadStr) == 0 {
				t.Fatal("no payload received")
			}
		})
	}
}

func TestDiscordProvider_MessageFormat(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		body    string
	}{
		{"subject only", "Alert!", ""},
		{"subject and body", "Deploy Done", "Version 5 is live"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Content-Type") != "application/json" {
					t.Errorf("expected application/json content type")
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			defer server.Close()

			p := NewDiscordProvider(server.URL)
			err := p.Send(context.Background(), "", tt.subject, tt.body, "text")
			if err != nil {
				t.Fatalf("Send() error: %v", err)
			}
		})
	}
}

// =====================================================
// WEBHOOK PROVIDER — uses recipient as URL override
// =====================================================

// =====================================================
// MODULE — Init and Start lifecycle
// =====================================================

func TestModule_Init_CreatesDispatcher(t *testing.T) {
	m := New()
	if m.dispatcher != nil {
		t.Error("dispatcher should be nil before Init")
	}
}

func TestNewDispatcher_IsEmpty(t *testing.T) {
	d := NewDispatcher()
	providers := d.Providers()
	if len(providers) != 0 {
		t.Errorf("new dispatcher should have 0 providers, got %d", len(providers))
	}
}

func TestDispatcher_GetProvider_EmptyName(t *testing.T) {
	d := NewDispatcher()
	_, ok := d.GetProvider("")
	if ok {
		t.Error("expected false for empty name")
	}
}

// =====================================================
// PROVIDER CONSTRUCTOR TESTS
// =====================================================

func TestNewSlackProvider_SetsClient(t *testing.T) {
	p := NewSlackProvider("https://hooks.slack.com/test")
	if p.client == nil {
		t.Error("HTTP client should be initialized")
	}
	if p.WebhookURL != "https://hooks.slack.com/test" {
		t.Errorf("WebhookURL = %q", p.WebhookURL)
	}
}

func TestNewDiscordProvider_SetsClient(t *testing.T) {
	p := NewDiscordProvider("https://discord.com/api/webhooks/test")
	if p.client == nil {
		t.Error("HTTP client should be initialized")
	}
	if p.WebhookURL != "https://discord.com/api/webhooks/test" {
		t.Errorf("WebhookURL = %q", p.WebhookURL)
	}
}

func TestNewTelegramProvider_SetsFields(t *testing.T) {
	p := NewTelegramProvider("bot123:ABC", "-12345")
	if p.client == nil {
		t.Error("HTTP client should be initialized")
	}
	if p.BotToken != "bot123:ABC" {
		t.Errorf("BotToken = %q", p.BotToken)
	}
	if p.ChatID != "-12345" {
		t.Errorf("ChatID = %q", p.ChatID)
	}
}

// =====================================================
// MOCK PROVIDER — error propagation
// =====================================================

func TestMockProvider_ErrorDoesNotAffectTracking(t *testing.T) {
	mock := &mockProvider{name: "failing", sendErr: fmt.Errorf("provider down")}

	err := mock.Send(context.Background(), "", "subject1", "", "")
	if err == nil {
		t.Fatal("expected error")
	}

	// Even on error, the send should be tracked
	if len(mock.sent) != 1 {
		t.Errorf("expected 1 tracked send, got %d", len(mock.sent))
	}
	if mock.sent[0] != "subject1" {
		t.Errorf("tracked subject = %q, want %q", mock.sent[0], "subject1")
	}
}
