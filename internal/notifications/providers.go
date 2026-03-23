package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
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
}

// =====================================================
// GENERIC WEBHOOK PROVIDER
// =====================================================

// WebhookProvider sends notifications to a custom HTTP endpoint.
type WebhookProvider struct {
	URL    string
	Secret string
	client *http.Client
}

// NewWebhookProvider creates a generic webhook notification provider.
func NewWebhookProvider(url, secret string) *WebhookProvider {
	return &WebhookProvider{
		URL:    url,
		Secret: secret,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (w *WebhookProvider) Name() string { return "webhook" }

func (w *WebhookProvider) Validate() error {
	if w.URL == "" {
		return fmt.Errorf("webhook URL is required")
	}
	return nil
}

func (w *WebhookProvider) Send(ctx context.Context, recipient, subject, body, format string) error {
	url := recipient
	if url == "" {
		url = w.URL
	}

	payload, _ := json.Marshal(map[string]string{
		"subject": subject,
		"body":    body,
		"format":  format,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DeployMonster/1.0")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook send: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
