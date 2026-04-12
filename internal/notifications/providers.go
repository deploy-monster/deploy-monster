package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// SLACK PROVIDER
// =====================================================

// SlackProvider sends notifications via Slack webhook.
type SlackProvider struct {
	WebhookURL string
	client     *http.Client
}

// NewSlackProvider creates a Slack notification provider.
func NewSlackProvider(webhookURL string) *SlackProvider {
	return &SlackProvider{
		WebhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *SlackProvider) Name() string { return "slack" }

func (s *SlackProvider) Validate() error {
	if s.WebhookURL == "" {
		return fmt.Errorf("slack webhook URL is required")
	}
	return nil
}

func (s *SlackProvider) Send(ctx context.Context, recipient, subject, body, format string) error {
	text := subject
	if body != "" {
		text = fmt.Sprintf("*%s*\n%s", subject, body)
	}

	payload, _ := json.Marshal(map[string]string{"text": text})

	return core.Retry(ctx, core.DefaultRetryConfig(), func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("slack send: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("slack returned %d", resp.StatusCode)
		}
		return nil
	})
}

// =====================================================
// DISCORD PROVIDER
// =====================================================

// DiscordProvider sends notifications via Discord webhook.
type DiscordProvider struct {
	WebhookURL string
	client     *http.Client
}

// NewDiscordProvider creates a Discord notification provider.
func NewDiscordProvider(webhookURL string) *DiscordProvider {
	return &DiscordProvider{
		WebhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *DiscordProvider) Name() string { return "discord" }

func (d *DiscordProvider) Validate() error {
	if d.WebhookURL == "" {
		return fmt.Errorf("discord webhook URL is required")
	}
	return nil
}

func (d *DiscordProvider) Send(ctx context.Context, recipient, subject, body, format string) error {
	content := subject
	if body != "" {
		content = fmt.Sprintf("**%s**\n%s", subject, body)
	}

	payload, _ := json.Marshal(map[string]string{"content": content})

	return core.Retry(ctx, core.DefaultRetryConfig(), func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.WebhookURL, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.client.Do(req)
		if err != nil {
			return fmt.Errorf("discord send: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			return fmt.Errorf("discord returned %d", resp.StatusCode)
		}
		return nil
	})
}

// =====================================================
// TELEGRAM PROVIDER
// =====================================================

// TelegramProvider sends notifications via Telegram Bot API.
type TelegramProvider struct {
	BotToken string
	ChatID   string
	client   *http.Client
}

// NewTelegramProvider creates a Telegram notification provider.
func NewTelegramProvider(botToken, chatID string) *TelegramProvider {
	return &TelegramProvider{
		BotToken: botToken,
		ChatID:   chatID,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TelegramProvider) Name() string { return "telegram" }

func (t *TelegramProvider) Validate() error {
	if t.BotToken == "" {
		return fmt.Errorf("telegram bot token is required")
	}
	if t.ChatID == "" {
		return fmt.Errorf("telegram chat ID is required")
	}
	return nil
}

func (t *TelegramProvider) Send(ctx context.Context, recipient, subject, body, format string) error {
	chatID := recipient
	if chatID == "" {
		chatID = t.ChatID
	}

	text := subject
	if body != "" {
		text = fmt.Sprintf("<b>%s</b>\n%s", subject, body)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.BotToken)
	payload, _ := json.Marshal(map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	})

	return core.Retry(ctx, core.DefaultRetryConfig(), func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := t.client.Do(req)
		if err != nil {
			return fmt.Errorf("telegram send: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("telegram returned %d", resp.StatusCode)
		}
		return nil
	})
}
