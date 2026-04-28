package handlers

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestDeployTriggerHandler_buildDeployLabels(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:        "app-1",
		ProjectID: "project-1",
		TenantID:  "tenant-1",
		Name:      "my-app",
		Port:      8080,
	})
	store.addDomain(&core.Domain{
		ID:    "dom-1",
		AppID: "app-1",
		FQDN:  "myapp.example.com",
	})

	h := NewDeployTriggerHandler(store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 3)

	if labels["monster.app.id"] != "app-1" {
		t.Errorf("app.id = %q, want app-1", labels["monster.app.id"])
	}
	if labels["monster.deploy.version"] != "3" {
		t.Errorf("version = %q, want 3", labels["monster.deploy.version"])
	}
	if labels["monster.http.routers.my-app-0.rule"] != "Host(`myapp.example.com`)" {
		t.Errorf("router rule = %q, want Host(`myapp.example.com`)", labels["monster.http.routers.my-app-0.rule"])
	}
	if labels["monster.http.services.my-app-0.loadbalancer.server.port"] != "8080" {
		t.Errorf("port = %q, want 8080", labels["monster.http.services.my-app-0.loadbalancer.server.port"])
	}
}

func TestDeployTriggerHandler_buildDeployLabels_DefaultPort(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:        "app-1",
		ProjectID: "project-1",
		TenantID:  "tenant-1",
		Name:      "my-app",
		Port:      0,
	})
	store.addDomain(&core.Domain{
		ID:    "dom-1",
		AppID: "app-1",
		FQDN:  "myapp.example.com",
	})

	h := NewDeployTriggerHandler(store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 1)

	if labels["monster.http.services.my-app-0.loadbalancer.server.port"] != "80" {
		t.Errorf("default port = %q, want 80", labels["monster.http.services.my-app-0.loadbalancer.server.port"])
	}
}

func TestDeployTriggerHandler_buildDeployLabels_NoDomains(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:        "app-1",
		ProjectID: "project-1",
		TenantID:  "tenant-1",
		Name:      "my-app",
	})

	h := NewDeployTriggerHandler(store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 1)

	if labels["monster.app.id"] != "app-1" {
		t.Errorf("app.id = %q, want app-1", labels["monster.app.id"])
	}
	// No router labels should exist
	if _, ok := labels["monster.http.routers.my-app-0.rule"]; ok {
		t.Error("expected no router labels when no domains")
	}
}

func TestDeployTriggerHandler_buildDeployLabels_ListDomainsError(t *testing.T) {
	store := newMockStore()
	store.addApp(&core.Application{
		ID:        "app-1",
		ProjectID: "project-1",
		TenantID:  "tenant-1",
		Name:      "my-app",
	})
	store.errListDomainsByApp = context.Canceled

	h := NewDeployTriggerHandler(store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 1)

	// Should still return base labels even when domain list fails
	if labels["monster.app.id"] != "app-1" {
		t.Errorf("app.id = %q, want app-1", labels["monster.app.id"])
	}
}
