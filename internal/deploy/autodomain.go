package deploy

import (
	"context"
	"fmt"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// AutoDomain generates and registers a subdomain for a newly created app.
// Format: {app-name}.{auto_subdomain_suffix}
// e.g., my-app.deploy.monster
func AutoDomain(ctx context.Context, store core.Store, events *core.EventBus, app *core.Application, suffix string) error {
	if suffix == "" {
		return nil // Auto-domain not configured
	}

	slug := sanitizeSlug(app.Name)
	fqdn := fmt.Sprintf("%s.%s", slug, suffix)

	// Check if domain already exists
	if _, err := store.GetDomainByFQDN(ctx, fqdn); err == nil {
		return nil // Already exists
	}

	domain := &core.Domain{
		AppID:       app.ID,
		FQDN:        fqdn,
		Type:        "auto",
		DNSProvider: "auto",
	}

	if err := store.CreateDomain(ctx, domain); err != nil {
		return fmt.Errorf("create auto-domain: %w", err)
	}

	events.PublishAsync(ctx, core.NewEvent(core.EventDomainAdded, "deploy",
		core.DomainEventData{DomainID: domain.ID, FQDN: fqdn, AppID: app.ID}))

	return nil
}

// sanitizeSlug converts an app name to a valid subdomain part.
func sanitizeSlug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else if r == ' ' || r == '_' || r == '.' {
			b.WriteRune('-')
		}
	}
	slug := b.String()

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = core.GenerateID()[:8]
	}
	return slug
}
