package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// validateWebhookURL validates that a webhook URL is safe to use.
// Prevents SSRF attacks by blocking private IPs, localhost, and file URLs.
func validateWebhookURL(webhookURL string) error {
	if webhookURL == "" {
		return fmt.Errorf("webhook URL is required")
	}

	u, err := url.Parse(webhookURL)
	if err != nil {
		return fmt.Errorf("invalid webhook URL: %w", err)
	}

	// Only allow HTTPS URLs for webhooks
	if u.Scheme != "https" {
		return fmt.Errorf("webhook URL must use HTTPS scheme")
	}

	// Get the hostname
	hostname := u.Hostname()
	if hostname == "" {
		return fmt.Errorf("webhook URL must have a hostname")
	}

	// Check for localhost variants
	localhostVariants := []string{"localhost", "127.0.0.1", "::1", "0.0.0.0", "[::1]"}
	for _, variant := range localhostVariants {
		if strings.EqualFold(hostname, variant) {
			return fmt.Errorf("webhook URL cannot point to localhost")
		}
	}

	// Check for private/internal IP ranges
	ip := net.ParseIP(hostname)
	if ip != nil {
		// Block private, loopback, link-local, and multicast ranges
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
			ip.IsLinkLocalMulticast() || ip.IsMulticast() {
			return fmt.Errorf("webhook URL cannot point to internal IP addresses")
		}
		// Block cloud metadata endpoints
		if ip.String() == "169.254.169.254" {
			return fmt.Errorf("webhook URL cannot point to cloud metadata endpoints")
		}
	}

	// Block common internal hostnames
	internalHostnames := []string{"metadata.google.internal", "metadata", "metadata.ec2.internal"}
	for _, internal := range internalHostnames {
		if strings.EqualFold(hostname, internal) || strings.HasSuffix(strings.ToLower(hostname), "."+strings.ToLower(internal)) {
			return fmt.Errorf("webhook URL cannot point to internal hostnames")
		}
	}

	return nil
}

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
	// SECURITY FIX: Validate webhook URL to prevent SSRF
	if err := validateWebhookURL(s.WebhookURL); err != nil {
		return fmt.Errorf("slack: %w", err)
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
			return fmt.Errorf("build slack request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("slack send: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

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
	// SECURITY FIX: Validate webhook URL to prevent SSRF
	if err := validateWebhookURL(d.WebhookURL); err != nil {
		return fmt.Errorf("discord: %w", err)
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
			return fmt.Errorf("build discord request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.client.Do(req)
		if err != nil {
			return fmt.Errorf("discord send: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

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
			return fmt.Errorf("build telegram request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := t.client.Do(req)
		if err != nil {
			return fmt.Errorf("telegram send: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("telegram returned %d", resp.StatusCode)
		}
		return nil
	})
}
