package deploy

import (
	"context"
	"fmt"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestAutoDomain_EmptySuffix(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	app := &core.Application{ID: "app-1", Name: "my-app"}

	err := AutoDomain(context.Background(), store, events, app, "")
	if err != nil {
		t.Fatalf("AutoDomain with empty suffix should return nil, got: %v", err)
	}
}

func TestAutoDomain_DomainAlreadyExists(t *testing.T) {
	store := newMockStore()
	// Pre-populate the domain so it "already exists"
	store.domains["my-app.deploy.monster"] = &core.Domain{
		ID:   "dom-existing",
		FQDN: "my-app.deploy.monster",
	}
	events := core.NewEventBus(nil)
	app := &core.Application{ID: "app-1", Name: "my-app"}

	err := AutoDomain(context.Background(), store, events, app, "deploy.monster")
	if err != nil {
		t.Fatalf("AutoDomain should return nil when domain already exists, got: %v", err)
	}
}

func TestAutoDomain_Success(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	app := &core.Application{ID: "app-1", Name: "my-app"}

	err := AutoDomain(context.Background(), store, events, app, "deploy.monster")
	if err != nil {
		t.Fatalf("AutoDomain returned error: %v", err)
	}

	// Verify domain was created
	domain, exists := store.domains["my-app.deploy.monster"]
	if !exists {
		t.Fatal("domain should have been created")
	}
	if domain.AppID != "app-1" {
		t.Errorf("domain.AppID = %q, want %q", domain.AppID, "app-1")
	}
	if domain.FQDN != "my-app.deploy.monster" {
		t.Errorf("domain.FQDN = %q, want %q", domain.FQDN, "my-app.deploy.monster")
	}
	if domain.Type != "auto" {
		t.Errorf("domain.Type = %q, want %q", domain.Type, "auto")
	}
	if domain.DNSProvider != "auto" {
		t.Errorf("domain.DNSProvider = %q, want %q", domain.DNSProvider, "auto")
	}
}

func TestAutoDomain_CreateDomainError(t *testing.T) {
	store := newMockStore()
	store.createDomainErr = fmt.Errorf("db write error")
	events := core.NewEventBus(nil)
	app := &core.Application{ID: "app-1", Name: "my-app"}

	err := AutoDomain(context.Background(), store, events, app, "deploy.monster")
	if err == nil {
		t.Fatal("expected error when CreateDomain fails")
	}
}

func TestAutoDomain_SanitizedName(t *testing.T) {
	tests := []struct {
		appName      string
		suffix       string
		expectedFQDN string
	}{
		{"My Cool App", "test.io", "my-cool-app.test.io"},
		{"UPPER_case", "example.com", "upper-case.example.com"},
		{"app.name.v2", "host.io", "app-name-v2.host.io"},
	}

	for _, tt := range tests {
		t.Run(tt.appName, func(t *testing.T) {
			store := newMockStore()
			events := core.NewEventBus(nil)
			app := &core.Application{ID: "app-x", Name: tt.appName}

			err := AutoDomain(context.Background(), store, events, app, tt.suffix)
			if err != nil {
				t.Fatalf("AutoDomain returned error: %v", err)
			}

			if _, exists := store.domains[tt.expectedFQDN]; !exists {
				t.Errorf("expected domain %q to be created, domains: %v", tt.expectedFQDN, store.domains)
			}
		})
	}
}
