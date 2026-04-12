package notifications

import (
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_Health_NoDispatcher(t *testing.T) {
	m := &Module{}
	// Before Init, Health should report OK (not yet started)
	if m.Health() != core.HealthOK {
		t.Error("expected HealthOK before Init (dispatcher nil)")
	}
}

func TestModule_Health_NoProviders_NoConfig(t *testing.T) {
	m := &Module{
		dispatcher: NewDispatcher(),
		core: &core.Core{
			Config: &core.Config{},
		},
	}
	if m.Health() != core.HealthOK {
		t.Error("expected HealthOK when no providers configured and none expected")
	}
}

func TestModule_Health_ConfiguredButNoProviders(t *testing.T) {
	m := &Module{
		dispatcher: NewDispatcher(),
		core: &core.Core{
			Config: &core.Config{
				Notifications: core.NotificationConfig{
					SlackWebhook: "https://hooks.slack.com/test",
				},
			},
		},
	}
	if m.Health() != core.HealthDegraded {
		t.Error("expected HealthDegraded when slack configured but no providers registered")
	}
}

func TestModule_Health_ProviderRegistered(t *testing.T) {
	d := NewDispatcher()
	d.RegisterProvider(NewSlackProvider("https://hooks.slack.com/test"))

	m := &Module{
		dispatcher: d,
		core: &core.Core{
			Config: &core.Config{
				Notifications: core.NotificationConfig{
					SlackWebhook: "https://hooks.slack.com/test",
				},
			},
		},
	}
	if m.Health() != core.HealthOK {
		t.Error("expected HealthOK when provider is registered")
	}
}
