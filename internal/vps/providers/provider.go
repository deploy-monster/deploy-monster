package providers

import "github.com/deploy-monster/deploy-monster/internal/core"

// Factory creates a VPSProvisioner from an API token.
type Factory func(apiToken string) core.VPSProvisioner

// Registry holds all available VPS provider factories.
var Registry = map[string]Factory{
	"hetzner":      NewHetzner,
	"digitalocean": NewDigitalOcean,
	"vultr":        NewVultr,
	"linode":       NewLinode,
	"custom":       NewCustom,
}
