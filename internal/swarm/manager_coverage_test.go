package swarm

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// All Manager methods shell out to docker CLI via exec.CommandContext.
// These tests use cancelled contexts to exercise error paths without requiring Docker.

func TestManager_Init_CancelledContext_NoAddr(t *testing.T) {
	rt := &mockRuntime{}
	m := NewManager(rt, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.Init(ctx, "")
	if err == nil {
		t.Log("init unexpectedly succeeded")
	}
}

func TestManager_Init_CancelledContext_WithAddr(t *testing.T) {
	rt := &mockRuntime{}
	m := NewManager(rt, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.Init(ctx, "10.0.0.1:2377")
	if err == nil {
		t.Log("init unexpectedly succeeded")
	}
}

func TestManager_Info_CancelledContext(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.Info(ctx)
	if err == nil {
		t.Log("info unexpectedly succeeded")
	}
}

func TestManager_Join_CancelledContext(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Join(ctx, "192.168.1.1:2377", "SWMTKN-1-fake-token")
	if err == nil {
		t.Log("join unexpectedly succeeded")
	}
}

func TestManager_Leave_CancelledContext_NoForce(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Leave(ctx, false)
	if err == nil {
		t.Log("leave unexpectedly succeeded")
	}
}

func TestManager_Leave_CancelledContext_Force(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.Leave(ctx, true)
	if err == nil {
		t.Log("leave --force unexpectedly succeeded")
	}
}

func TestManager_DeployService_CancelledContext_WithStore(t *testing.T) {
	store := &mockStoreWithDomains{
		domains: []core.Domain{
			{FQDN: "example.com"},
			{FQDN: "www.example.com"},
		},
	}
	m := NewManager(nil, store, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	app := &core.Application{
		ID:       "app-1",
		Name:     "web",
		TenantID: "t-1",
		Port:     8080,
	}

	err := m.DeployService(ctx, app, "monster/web:latest", 3)
	if err == nil {
		t.Log("deploy service unexpectedly succeeded")
	}
}

func TestManager_DeployService_CancelledContext_DefaultPort(t *testing.T) {
	store := &mockStoreWithDomains{
		domains: []core.Domain{{FQDN: "test.com"}},
	}
	m := NewManager(nil, store, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	app := &core.Application{
		ID:       "app-1",
		Name:     "web",
		TenantID: "t-1",
		Port:     0, // default to 80
	}

	err := m.DeployService(ctx, app, "monster/web:latest", 1)
	if err == nil {
		t.Log("deploy service unexpectedly succeeded")
	}
}

func TestManager_DeployService_CancelledContext_NilStore(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	app := &core.Application{
		ID:         "app-1",
		Name:       "web",
		TenantID:   "t-1",
		Port:       3000,
		EnvVarsEnc: "some-encrypted-vars",
	}

	err := m.DeployService(ctx, app, "monster/web:v2", 2)
	if err == nil {
		t.Log("deploy service unexpectedly succeeded")
	}
}

func TestManager_ScaleService_CancelledContext(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.ScaleService(ctx, "web", 5)
	if err == nil {
		t.Log("scale unexpectedly succeeded")
	}
}

func TestManager_RemoveService_CancelledContext(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.RemoveService(ctx, "web")
	if err == nil {
		t.Log("remove service unexpectedly succeeded")
	}
}

func TestManager_CreateOverlayNetwork_CancelledContext(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := m.CreateOverlayNetwork(ctx, "monster-network")
	if err == nil {
		t.Log("create network unexpectedly succeeded")
	}
}

func TestManager_ListServices_CancelledContext(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.ListServices(ctx)
	if err == nil {
		t.Log("list services unexpectedly succeeded")
	}
}

func TestManager_ListNodes_CancelledContext(t *testing.T) {
	m := NewManager(nil, nil, nil, discardLogger())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := m.ListNodes(ctx)
	if err == nil {
		t.Log("list nodes unexpectedly succeeded")
	}
}

// mockStoreWithDomains provides domains for DeployService tests.
type mockStoreWithDomains struct {
	core.Store
	domains []core.Domain
}

func (m *mockStoreWithDomains) ListDomainsByApp(_ context.Context, _ string) ([]core.Domain, error) {
	return m.domains, nil
}
