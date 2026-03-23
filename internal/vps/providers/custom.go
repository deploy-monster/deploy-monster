package providers

import (
	"context"
	"fmt"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// Custom implements core.VPSProvisioner for existing servers via SSH.
// No cloud API — the server already exists and is connected by IP + SSH key.
type Custom struct{}

func NewCustom(_ string) core.VPSProvisioner {
	return &Custom{}
}

func (c *Custom) Name() string { return "custom" }

func (c *Custom) ListRegions(_ context.Context) ([]core.VPSRegion, error) {
	return []core.VPSRegion{{ID: "custom", Name: "Custom Server"}}, nil
}

func (c *Custom) ListSizes(_ context.Context, _ string) ([]core.VPSSize, error) {
	return []core.VPSSize{{ID: "custom", Name: "Custom", CPUs: 0, MemoryMB: 0, DiskGB: 0}}, nil
}

func (c *Custom) Create(_ context.Context, opts core.VPSCreateOpts) (*core.VPSInstance, error) {
	// Custom servers are registered, not provisioned
	return &core.VPSInstance{
		ID:     core.GenerateID(),
		Name:   opts.Name,
		Status: "active",
		Region: "custom",
	}, nil
}

func (c *Custom) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("custom servers cannot be deleted via API — deregister instead")
}

func (c *Custom) Status(_ context.Context, _ string) (string, error) {
	return "active", nil
}
