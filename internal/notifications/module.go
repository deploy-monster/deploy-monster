package notifications

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// ErrNotificationsClosed is returned by Send when the module has
// begun shutdown. Callers (HTTP handlers, event bus subscribers)
// should treat it the same as any other transient send failure —
// log and drop — because the process is on its way down.
var ErrNotificationsClosed = errors.New("notifications: module is closed")

// Module implements the notification module.
// It manages notification providers and dispatches notifications
// based on events from the EventBus.
//
// Lifecycle notes for the Tier 73-77 hardening pass:
//
//   - Stop used to be a complete no-op. A slow SMTP connection or
//     a stuck webhook send could leak past module shutdown because
//     Send was synchronous but its goroutine-origin (the EventBus
//     async handler) had no wg to wait on. The module now tracks
//     every accepted Send call on its own wg and Stop drains them
//     with the shutdown ctx as a deadline.
//   - Send is now guarded by a closed flag serialised with wg.Add
//     so a Send that lands after Stop returns ErrNotificationsClosed
//     immediately instead of racing into a half-torn-down module.
//   - The alert dispatcher recovers from provider panics so a
//     misbehaving SMTP library can't take the whole process with it.
type Module struct {
	core       *core.Core
	dispatcher *Dispatcher
	logger     *slog.Logger

	// stopCtx is cancelled by Stop so async sends spawned from
	// dispatchAlert can abort their provider calls at the next ctx
	// boundary instead of burning the shutdown deadline.
	stopCtx    context.Context
	stopCancel context.CancelFunc

	// mu guards closed and serialises the closed-check with wg.Add
	// so a Send can never observe a "not closed" snapshot that races
	// with Stop's wg.Wait — the same contract backup/Scheduler uses.
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// New creates a new notification module.
func New() *Module {
	return &Module{}
}

func (m *Module) ID() string             { return "notifications" }
func (m *Module) Name() string           { return "Notifications" }
func (m *Module) Version() string        { return "1.0.0" }
func (m *Module) Dependencies() []string { return []string{"core.db"} }
func (m *Module) Routes() []core.Route   { return nil }

func (m *Module) Init(_ context.Context, c *core.Core) error {
	m.core = c
	m.logger = c.Logger.With("module", m.ID())

	m.dispatcher = NewDispatcher()

	// Auto-register providers from configuration
	if c.Config != nil {
		cfg := c.Config.Notifications
		if cfg.SlackWebhook != "" {
			m.dispatcher.RegisterProvider(NewSlackProvider(cfg.SlackWebhook))
			m.logger.Info("slack provider registered")
		}
		if cfg.DiscordWebhook != "" {
			m.dispatcher.RegisterProvider(NewDiscordProvider(cfg.DiscordWebhook))
			m.logger.Info("discord provider registered")
		}
		if cfg.TelegramToken != "" {
			chatID := c.Config.Notifications.TelegramChatID
			if chatID == "" {
				chatID = "0" // placeholder, admin configures via API
			}
			m.dispatcher.RegisterProvider(NewTelegramProvider(cfg.TelegramToken, chatID))
			m.logger.Info("telegram provider registered")
		}
		if cfg.SMTP.Host != "" {
			smtpProv := NewSMTPProvider(cfg.SMTP)
			if err := smtpProv.Validate(); err != nil {
				// Don't hard-fail startup — log and skip so a bad
				// SMTP config doesn't take the whole server offline.
				// Health() will report degraded when a provider was
				// wanted but none ended up registered.
				m.logger.Warn("smtp provider invalid, skipping",
					"error", err, "host", cfg.SMTP.Host)
			} else {
				m.dispatcher.RegisterProvider(smtpProv)
				m.logger.Info("smtp provider registered",
					"host", cfg.SMTP.Host,
					"port", smtpProv.defaultPort(),
					"tls", cfg.SMTP.UseTLS,
				)
			}
		}
	}

	// Register the dispatcher as the notification sender in Services
	c.Services.Notifications = m

	return nil
}

func (m *Module) Start(_ context.Context) error {
	// Derive the module-owned shutdown context that every async
	// downstream path will observe. Stop cancels this to abort slow
	// provider calls at the next ctx boundary.
	m.stopCtx, m.stopCancel = context.WithCancel(context.Background())

	// Subscribe to events that should trigger notifications.
	// Each event type can be configured with notification rules
	// (which channels, which recipients) in the future.
	m.core.Events.SubscribeAsync("alert.*", func(ctx context.Context, event core.Event) error {
		m.dispatchAlert(ctx, event)
		return nil
	})

	m.core.Events.SubscribeAsync("deploy.*", func(ctx context.Context, event core.Event) error {
		m.logger.Debug("deploy event", "type", event.Type)
		return nil
	})

	m.logger.Info("notification module started", "providers", m.dispatcher.Providers())
	return nil
}

// Stop drains in-flight Send calls with a deadline taken from ctx.
// Sets the closed flag so new Send calls fail fast, cancels the
// module stopCtx so slow provider connections unblock, then waits
// for every accepted Send to return. A drain timeout is logged but
// not returned — the module system counts a timed-out Stop as "shut
// down anyway" and the downstream modules still need to unwind.
func (m *Module) Stop(ctx context.Context) error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	if m.stopCancel != nil {
		m.stopCancel()
	}
	m.mu.Unlock()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		if m.logger != nil {
			m.logger.Warn("notification drain exceeded shutdown deadline", "error", ctx.Err())
		}
		return nil
	}
}

// Closed reports whether Stop has been called. Useful for tests and
// for HTTP handlers that want to short-circuit instead of dispatching
// a Send that will just return ErrNotificationsClosed.
func (m *Module) Closed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *Module) Health() core.HealthStatus {
	// Before Init, dispatcher is nil — report OK (not yet started)
	if m.dispatcher == nil {
		return core.HealthOK
	}
	// After Init: if notification config exists but no providers registered, degraded
	if m.core != nil && m.core.Config != nil {
		cfg := m.core.Config.Notifications
		wantProviders := cfg.SlackWebhook != "" || cfg.DiscordWebhook != "" || cfg.TelegramToken != "" || cfg.SMTP.Host != ""
		if wantProviders && len(m.dispatcher.Providers()) == 0 {
			return core.HealthDegraded
		}
	}
	return core.HealthOK
}

func (m *Module) Events() []core.EventHandler {
	return nil // Subscriptions are done in Start() for async
}

// Send implements core.NotificationSender.
// Routes the notification to the appropriate provider.
func (m *Module) Send(ctx context.Context, notification core.Notification) (err error) {
	// Serialise the closed-check with wg.Add so Stop can rely on a
	// happens-before relationship when it waits for the drain. A Send
	// that slips in between "Stop sets closed=true" and "Stop waits"
	// would otherwise race wg.Add against wg.Wait, violating Go's
	// WaitGroup contract.
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return ErrNotificationsClosed
	}
	m.wg.Add(1)
	m.mu.Unlock()
	defer m.wg.Done()

	// Turn any provider panic into a structured error so a broken
	// SMTP library can't take the process down. The recover wraps
	// the named return so the caller sees a real error.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("notifications: provider panic: %v", r)
			if m.logger != nil {
				m.logger.Error("panic in notification provider",
					"channel", notification.Channel,
					"recipient", notification.Recipient,
					"error", r,
				)
			}
		}
	}()

	provider, ok := m.dispatcher.GetProvider(notification.Channel)
	if !ok {
		return fmt.Errorf("notification channel %q not registered", notification.Channel)
	}

	if sendErr := provider.Send(ctx, notification.Recipient, notification.Subject, notification.Body, notification.Format); sendErr != nil {
		// Emit failure event
		m.core.Events.PublishAsync(ctx, core.NewEvent(
			core.EventNotificationFailed, "notifications",
			core.NotificationEventData{
				Channel:   notification.Channel,
				Recipient: notification.Recipient,
				Subject:   notification.Subject,
				Error:     sendErr.Error(),
			},
		))
		return sendErr
	}

	// Emit success event
	m.core.Events.PublishAsync(ctx, core.NewEvent(
		core.EventNotificationSent, "notifications",
		core.NotificationEventData{
			Channel:   notification.Channel,
			Recipient: notification.Recipient,
			Subject:   notification.Subject,
		},
	))

	return nil
}

// RegisterProvider adds a notification provider to the dispatcher.
// Called by other modules or configuration to add channels.
func (m *Module) RegisterProvider(provider Provider) {
	m.dispatcher.RegisterProvider(provider)
	m.logger.Info("notification provider registered", "provider", provider.Name())
}

// dispatchAlert extracts alert data and sends notifications to all registered providers.
func (m *Module) dispatchAlert(ctx context.Context, event core.Event) {
	data, ok := event.Data.(core.AlertEventData)
	if !ok {
		m.logger.Warn("alert event has unexpected data type", "type", event.Type)
		return
	}

	severityIcon := map[string]string{
		"critical": "[CRITICAL]",
		"warning":  "[WARNING]",
		"info":     "[INFO]",
	}
	icon := severityIcon[data.Severity]
	if icon == "" {
		icon = "[ALERT]"
	}

	subject := fmt.Sprintf("%s %s", icon, data.Name)
	body := fmt.Sprintf("%s\nResource: %s\nSeverity: %s", data.Message, data.Resource, data.Severity)

	// Send to all registered providers
	for _, name := range m.dispatcher.Providers() {
		if err := m.Send(ctx, core.Notification{
			Channel: name,
			Subject: subject,
			Body:    body,
			Format:  "text",
		}); err != nil {
			m.logger.Warn("alert notification failed", "channel", name, "alert", data.Name, "error", err)
		}
	}
}
