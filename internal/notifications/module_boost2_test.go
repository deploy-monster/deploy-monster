package notifications

import (
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_Init_NilConfig(t *testing.T) {
	m := New()
	c := &core.Core{
		Config:   nil,
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if c.Services.Notifications != m {
		t.Error("expected module to be registered as notification sender")
	}
}

func TestModule_Init_SlackProvider(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				SlackWebhook: "https://hooks.slack.com/test",
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(m.dispatcher.Providers()) != 1 {
		t.Errorf("expected 1 provider, got %d", len(m.dispatcher.Providers()))
	}
}

func TestModule_Init_DiscordProvider(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				DiscordWebhook: "https://discord.com/api/webhooks/test",
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(m.dispatcher.Providers()) != 1 {
		t.Errorf("expected 1 provider, got %d", len(m.dispatcher.Providers()))
	}
}

func TestModule_Init_TelegramProvider(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				TelegramToken: "123:ABC",
				TelegramChatID: "-12345",
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(m.dispatcher.Providers()) != 1 {
		t.Errorf("expected 1 provider, got %d", len(m.dispatcher.Providers()))
	}
}

func TestModule_Init_TelegramProvider_NoChatID(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				TelegramToken: "123:ABC",
				// ChatID empty — should default to "0"
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(m.dispatcher.Providers()) != 1 {
		t.Errorf("expected 1 provider, got %d", len(m.dispatcher.Providers()))
	}
}

func TestModule_Init_SMTPProvider_Valid(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				SMTP: core.SMTPConfig{
					Host: "smtp.example.com",
					Port: 587,
					From: "test@example.com",
				},
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(m.dispatcher.Providers()) != 1 {
		t.Errorf("expected 1 provider, got %d", len(m.dispatcher.Providers()))
	}
}

func TestModule_Init_SMTPProvider_Invalid(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				SMTP: core.SMTPConfig{
					Host: "smtp.example.com",
					// Missing From — validation should fail
				},
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Invalid SMTP should be skipped, so no providers registered
	if len(m.dispatcher.Providers()) != 0 {
		t.Errorf("expected 0 providers (invalid SMTP skipped), got %d", len(m.dispatcher.Providers()))
	}
}

func TestModule_Init_MultipleProviders(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				SlackWebhook:   "https://hooks.slack.com/test",
				DiscordWebhook: "https://discord.com/api/webhooks/test",
				TelegramToken:  "123:ABC",
				TelegramChatID: "-12345",
				SMTP: core.SMTPConfig{
					Host: "smtp.example.com",
					Port: 587,
					From: "test@example.com",
				},
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(m.dispatcher.Providers()) != 4 {
		t.Errorf("expected 4 providers, got %d", len(m.dispatcher.Providers()))
	}
}

func TestModule_Health_NilDispatcher(t *testing.T) {
	m := New()
	if m.Health() != core.HealthOK {
		t.Error("expected HealthOK before Init")
	}
}

func TestModule_Health_Degraded(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				SlackWebhook: "https://hooks.slack.com/test",
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Manually clear providers to simulate validation failure
	m.dispatcher = NewDispatcher()
	if m.Health() != core.HealthDegraded {
		t.Error("expected HealthDegraded when wanted providers but none registered")
	}
}

func TestModule_Health_OK(t *testing.T) {
	m := New()
	c := &core.Core{
		Config: &core.Config{
			Notifications: core.NotificationConfig{
				SlackWebhook: "https://hooks.slack.com/test",
			},
		},
		Logger:   slog.Default(),
		Services: core.NewServices(),
	}
	if err := m.Init(nil, c); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if m.Health() != core.HealthOK {
		t.Error("expected HealthOK when providers registered")
	}
}
