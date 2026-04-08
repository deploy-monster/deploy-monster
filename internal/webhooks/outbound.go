package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Compile-time check.
var _ core.OutboundWebhookSender = (*OutboundSender)(nil)

// OutboundSender delivers webhook payloads to external URLs.
// It handles JSON serialization, HMAC signing, retries, and logging.
type OutboundSender struct {
	client *http.Client
	events *core.EventBus
}

// NewOutboundSender creates a new outbound webhook sender.
func NewOutboundSender(events *core.EventBus) *OutboundSender {
	return &OutboundSender{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		events: events,
	}
}

// Send delivers a webhook payload to the configured URL with retry on 5xx/network errors.
func (s *OutboundSender) Send(ctx context.Context, webhook core.OutboundWebhook) error {
	// Serialize payload
	body, err := json.Marshal(webhook.Payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	method := webhook.Method
	if method == "" {
		method = http.MethodPost
	}

	timeout := webhook.Timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	// HMAC signature (computed once — body is static)
	var signature string
	if webhook.Secret != "" {
		signature = "sha256=" + signPayload(body, webhook.Secret)
	}

	retryCfg := core.DefaultRetryConfig()

	var clientErr error // non-retryable 4xx error
	err = core.Retry(ctx, retryCfg, func() error {
		req, reqErr := http.NewRequestWithContext(ctx, method, webhook.URL, bytes.NewReader(body))
		if reqErr != nil {
			return fmt.Errorf("create request: %w", reqErr)
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "DeployMonster/1.0")

		for k, v := range webhook.Headers {
			req.Header.Set(k, v)
		}
		if signature != "" {
			req.Header.Set("X-Monster-Signature", signature)
		}

		resp, doErr := s.client.Do(req)
		if doErr != nil {
			return fmt.Errorf("send webhook to %s: %w", webhook.URL, doErr)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)

		if resp.StatusCode >= 500 {
			return fmt.Errorf("webhook %s returned %d", webhook.URL, resp.StatusCode)
		}
		if resp.StatusCode >= 400 {
			clientErr = fmt.Errorf("webhook %s returned %d", webhook.URL, resp.StatusCode)
			return nil // stop retry — 4xx won't succeed on retry
		}
		return nil
	})

	if clientErr != nil {
		err = clientErr
	}

	if err != nil {
		s.emitFailure(ctx, webhook.URL, err.Error())
		return err
	}

	// Emit success event
	if s.events != nil {
		s.events.PublishAsync(ctx, core.NewEvent(
			core.EventOutboundSent, "webhooks",
			core.NotificationEventData{
				Channel:   "webhook",
				Recipient: webhook.URL,
			},
		))
	}

	return nil
}

func (s *OutboundSender) emitFailure(ctx context.Context, url, errMsg string) {
	if s.events != nil {
		s.events.PublishAsync(ctx, core.NewEvent(
			core.EventOutboundFailed, "webhooks",
			core.NotificationEventData{
				Channel:   "webhook",
				Recipient: url,
				Error:     errMsg,
			},
		))
	}
}

// signPayload creates an HMAC-SHA256 signature.
func signPayload(payload []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}
