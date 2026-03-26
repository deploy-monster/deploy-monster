package notifications

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func init() {
	core.RegisterModule(func() core.Module { return New() })
}

// Module implements the notification module.
// It manages notification providers and dispatches notifications
// based on events from the EventBus.
type Module struct {
	core       *core.Core
	dispatcher *Dispatcher
	logger     *slog.Logger
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

	// Register the dispatcher as the notification sender in Services
	c.Services.Notifications = m

	return nil
}

func (m *Module) Start(_ context.Context) error {
	// Subscribe to events that should trigger notifications.
	// Each event type can be configured with notification rules
	// (which channels, which recipients) in the future.
	m.core.Events.SubscribeAsync("alert.*", func(ctx context.Context, event core.Event) error {
		m.logger.Info("alert event received", "type", event.Type)
		// In future phases, this will dispatch to configured channels
		return nil
	})

	m.core.Events.SubscribeAsync("deploy.*", func(ctx context.Context, event core.Event) error {
		m.logger.Debug("deploy event", "type", event.Type)
		return nil
	})

	m.logger.Info("notification module started", "providers", m.dispatcher.Providers())
	return nil
}

func (m *Module) Stop(_ context.Context) error {
	return nil
}

func (m *Module) Health() core.HealthStatus {
	return core.HealthOK
}

func (m *Module) Events() []core.EventHandler {
	return nil // Subscriptions are done in Start() for async
}

// Send implements core.NotificationSender.
// Routes the notification to the appropriate provider.
func (m *Module) Send(ctx context.Context, notification core.Notification) error {
	provider, ok := m.dispatcher.GetProvider(notification.Channel)
	if !ok {
		return fmt.Errorf("notification channel %q not registered", notification.Channel)
	}

	if err := provider.Send(ctx, notification.Recipient, notification.Subject, notification.Body, notification.Format); err != nil {
		// Emit failure event
		m.core.Events.PublishAsync(ctx, core.NewEvent(
			core.EventNotificationFailed, "notifications",
			core.NotificationEventData{
				Channel:   notification.Channel,
				Recipient: notification.Recipient,
				Subject:   notification.Subject,
				Error:     err.Error(),
			},
		))
		return err
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
