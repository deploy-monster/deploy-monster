package notifications

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// =====================================================
// DISPATCHER TESTS
// =====================================================

func TestDispatcher_RegisterAndGet(t *testing.T) {
	d := NewDispatcher()

	mock := &mockProvider{name: "test"}
	d.RegisterProvider(mock)

	got, ok := d.GetProvider("test")
	if !ok {
		t.Fatal("expected provider to be found")
	}
	if got.Name() != "test" {
		t.Errorf("expected name 'test', got %q", got.Name())
	}
}

func TestDispatcher_NotFound(t *testing.T) {
	d := NewDispatcher()
	_, ok := d.GetProvider("nonexistent")
	if ok {
		t.Error("expected false for missing provider")
	}
}

func TestDispatcher_Providers(t *testing.T) {
	d := NewDispatcher()
	d.RegisterProvider(&mockProvider{name: "slack"})
	d.RegisterProvider(&mockProvider{name: "discord"})

	names := d.Providers()
	if len(names) != 2 {
		t.Errorf("expected 2 providers, got %d", len(names))
	}
}

func TestDispatcher_OverwriteProvider(t *testing.T) {
	d := NewDispatcher()

	first := &mockProvider{name: "slack", sendErr: fmt.Errorf("old")}
	second := &mockProvider{name: "slack", sendErr: nil}

	d.RegisterProvider(first)
	d.RegisterProvider(second)

	got, ok := d.GetProvider("slack")
	if !ok {
		t.Fatal("expected provider to be found after overwrite")
	}
	if err := got.Send(context.Background(), "", "", "", ""); err != nil {
		t.Errorf("expected overwritten provider (no error), got %v", err)
	}
}

func TestDispatcher_EmptyProviders(t *testing.T) {
	d := NewDispatcher()
	names := d.Providers()
	if len(names) != 0 {
		t.Errorf("expected 0 providers, got %d", len(names))
	}
}

func TestDispatcher_MultipleProviders(t *testing.T) {
	d := NewDispatcher()
	providerNames := []string{"slack", "discord", "telegram", "email"}

	for _, name := range providerNames {
		d.RegisterProvider(&mockProvider{name: name})
	}

	names := d.Providers()
	if len(names) != len(providerNames) {
		t.Errorf("expected %d providers, got %d", len(providerNames), len(names))
	}

	for _, name := range providerNames {
		_, ok := d.GetProvider(name)
		if !ok {
			t.Errorf("provider %q should be registered", name)
		}
	}
}

// =====================================================
// PROVIDER VALIDATION TESTS
// =====================================================

func TestSlackProvider_Validate(t *testing.T) {
	tests := []struct {
		name       string
		webhookURL string
		wantErr    bool
	}{
		{"valid URL", "https://hooks.slack.com/services/T00/B00/xxx", false},
		{"empty URL", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewSlackProvider(tt.webhookURL)
			err := p.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSlackProvider_Name(t *testing.T) {
	p := NewSlackProvider("https://example.com")
	if p.Name() != "slack" {
		t.Errorf("Name() = %q, want %q", p.Name(), "slack")
	}
}

func TestDiscordProvider_Validate(t *testing.T) {
	tests := []struct {
		name       string
		webhookURL string
		wantErr    bool
	}{
		{"valid URL", "https://discord.com/api/webhooks/123/abc", false},
		{"empty URL", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewDiscordProvider(tt.webhookURL)
			err := p.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDiscordProvider_Name(t *testing.T) {
	p := NewDiscordProvider("https://example.com")
	if p.Name() != "discord" {
		t.Errorf("Name() = %q, want %q", p.Name(), "discord")
	}
}

func TestTelegramProvider_Validate(t *testing.T) {
	tests := []struct {
		name     string
		botToken string
		chatID   string
		wantErr  bool
	}{
		{"valid", "123:ABCdef", "-12345", false},
		{"empty bot token", "", "-12345", true},
		{"empty chat ID", "123:ABCdef", "", true},
		{"both empty", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewTelegramProvider(tt.botToken, tt.chatID)
			err := p.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTelegramProvider_Name(t *testing.T) {
	p := NewTelegramProvider("token", "chat")
	if p.Name() != "telegram" {
		t.Errorf("Name() = %q, want %q", p.Name(), "telegram")
	}
}

// =====================================================
// PROVIDER SEND TESTS (with httptest)
// =====================================================

func TestSlackProvider_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content type")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewSlackProvider(server.URL)
	err := p.Send(context.Background(), "", "Test Alert", "Details here", "text")
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}
}

func TestSlackProvider_Send_SubjectOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	p := NewSlackProvider(server.URL)
	err := p.Send(context.Background(), "", "Subject Only", "", "text")
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}
}

func TestSlackProvider_Send_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := NewSlackProvider(server.URL)
	err := p.Send(context.Background(), "", "Test", "Body", "text")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestDiscordProvider_Send_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent) // Discord returns 204
	}))
	defer server.Close()

	p := NewDiscordProvider(server.URL)
	err := p.Send(context.Background(), "", "Deploy Complete", "App deployed successfully", "text")
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}
}

func TestDiscordProvider_Send_SubjectOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	p := NewDiscordProvider(server.URL)
	err := p.Send(context.Background(), "", "Subject Only", "", "text")
	if err != nil {
		t.Fatalf("Send() returned error: %v", err)
	}
}

func TestDiscordProvider_Send_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	p := NewDiscordProvider(server.URL)
	err := p.Send(context.Background(), "", "Test", "Body", "text")
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
}

// =====================================================
// PROVIDER INTERFACE COMPLIANCE
// =====================================================

func TestProviderInterfaceCompliance(t *testing.T) {
	tests := []struct {
		name     string
		provider Provider
	}{
		{"SlackProvider", NewSlackProvider("https://hooks.slack.com/test")},
		{"DiscordProvider", NewDiscordProvider("https://discord.com/api/webhooks/test")},
		{"TelegramProvider", NewTelegramProvider("token", "chatid")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.provider.Name() == "" {
				t.Error("Name() should not be empty")
			}
			// Validate should succeed for properly configured providers.
			if err := tt.provider.Validate(); err != nil {
				t.Errorf("Validate() returned error for valid config: %v", err)
			}
		})
	}
}

// =====================================================
// MOCK PROVIDER
// =====================================================

type mockProvider struct {
	name    string
	sendErr error
	sent    []string // tracks subjects sent
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Send(_ context.Context, _, subject, _, _ string) error {
	m.sent = append(m.sent, subject)
	return m.sendErr
}
func (m *mockProvider) Validate() error { return nil }

// =====================================================
// MOCK PROVIDER SEND TRACKING
// =====================================================

func TestMockProvider_TracksSends(t *testing.T) {
	mock := &mockProvider{name: "test"}

	_ = mock.Send(context.Background(), "", "first", "", "")
	_ = mock.Send(context.Background(), "", "second", "", "")

	if len(mock.sent) != 2 {
		t.Errorf("expected 2 sends tracked, got %d", len(mock.sent))
	}
	if mock.sent[0] != "first" {
		t.Errorf("first sent = %q, want %q", mock.sent[0], "first")
	}
	if mock.sent[1] != "second" {
		t.Errorf("second sent = %q, want %q", mock.sent[1], "second")
	}
}

func TestMockProvider_ReturnsError(t *testing.T) {
	mock := &mockProvider{name: "test", sendErr: fmt.Errorf("send failed")}
	err := mock.Send(context.Background(), "", "subject", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "send failed" {
		t.Errorf("error = %q, want %q", err.Error(), "send failed")
	}
}
