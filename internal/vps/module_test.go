package vps

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestModule_Init_RegistersConfiguredProviders(t *testing.T) {
	c := &core.Core{
		Config: &core.Config{
			VPSProviders: core.VPSProvidersConfig{
				HetznerToken: "hetzner-token",
			},
		},
		Services: core.NewServices(),
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
	}

	m := New()
	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if c.Services.VPSProvisioner("custom") == nil {
		t.Fatal("custom provider should always be registered")
	}
	if c.Services.VPSProvisioner("hetzner") == nil {
		t.Fatal("configured hetzner provider was not registered")
	}
	if c.Services.VPSProvisioner("digitalocean") != nil {
		t.Fatal("digitalocean provider should not register without a token")
	}
}
