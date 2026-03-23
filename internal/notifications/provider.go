package notifications

import (
	"context"
	"sync"
)

// Provider is the interface that all notification channels implement.
// Each channel (email, Slack, Discord, Telegram, webhook) provides its own
// implementation that is registered with the Dispatcher.
type Provider interface {
	// Name returns the channel identifier (e.g., "email", "slack").
	Name() string

	// Send delivers a notification through this channel.
	Send(ctx context.Context, recipient, subject, body, format string) error

	// Validate checks if the provider is properly configured.
	Validate() error
}

// Dispatcher routes notifications to the appropriate provider.
// It implements core.NotificationSender.
type Dispatcher struct {
	mu        sync.RWMutex
	providers map[string]Provider
}

// NewDispatcher creates a new notification dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		providers: make(map[string]Provider),
	}
}

// RegisterProvider adds a notification provider.
func (d *Dispatcher) RegisterProvider(provider Provider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providers[provider.Name()] = provider
}

// GetProvider returns a provider by name.
func (d *Dispatcher) GetProvider(name string) (Provider, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	p, ok := d.providers[name]
	return p, ok
}

// Providers returns all registered provider names.
func (d *Dispatcher) Providers() []string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	names := make([]string, 0, len(d.providers))
	for name := range d.providers {
		names = append(names, name)
	}
	return names
}
