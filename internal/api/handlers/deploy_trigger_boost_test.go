package handlers

import (
	"context"
	"errors"
	"io"
	"strings"
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

	h := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
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

	h := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
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

	h := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
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

	h := NewDeployTriggerHandler(context.Background(), store, nil, testCore().Events)
	labels := h.buildDeployLabels(context.Background(), store.apps["app-1"], 1)

	// Should still return base labels even when domain list fails
	if labels["monster.app.id"] != "app-1" {
		t.Errorf("app.id = %q, want app-1", labels["monster.app.id"])
	}
}

func TestDeployTriggerHandler_RuntimeSelection(t *testing.T) {
	localRuntime := &mockContainerRuntime{}
	h := NewDeployTriggerHandler(context.Background(), newMockStore(), localRuntime, nil)

	rt, err := h.deployRuntimeForApp(&core.Application{ID: "app-1"})
	if err != nil {
		t.Fatalf("local runtime: %v", err)
	}
	if rt != localRuntime {
		t.Fatal("expected local runtime for empty server ID")
	}

	rt, err = h.deployRuntimeForApp(&core.Application{ID: "app-1", ServerID: "local"})
	if err != nil {
		t.Fatalf("explicit local runtime: %v", err)
	}
	if rt != localRuntime {
		t.Fatal("expected local runtime for serverID=local")
	}

	if _, err := NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).deployRuntimeForApp(&core.Application{}); err == nil {
		t.Fatal("expected nil local runtime to fail")
	}
	if _, err := h.deployRuntimeForApp(&core.Application{ID: "app-1", ServerID: "remote-1"}); err == nil {
		t.Fatal("expected remote app without node manager to fail")
	}

	node := &fakeNodeExecutor{id: "remote-1"}
	h.SetNodeManager(&fakeNodeManager{nodes: map[string]core.NodeExecutor{"remote-1": node}})
	rt, err = h.deployRuntimeForApp(&core.Application{ID: "app-1", ServerID: "remote-1"})
	if err != nil {
		t.Fatalf("remote runtime: %v", err)
	}
	if rt != node {
		t.Fatal("expected remote node executor")
	}
}

func TestDeployTriggerHelpers_ImageNamesAndRegistryRefs(t *testing.T) {
	cases := map[string]bool{
		"nginx:latest":                 false,
		"library/nginx:latest":         false,
		"localhost/nginx:latest":       true,
		"registry.example.com/app:tag": true,
		"registry:5000/app:tag":        true,
	}
	for ref, want := range cases {
		if got := imageRefHasRegistry(ref); got != want {
			t.Fatalf("imageRefHasRegistry(%q) = %v, want %v", ref, got, want)
		}
	}
	if got := imageNamePart("My_App. Prod", "fallback"); got != "my-app-prod" {
		t.Fatalf("imageNamePart sanitized = %q", got)
	}
	if got := buildImageTagForRegistry(" registry.example.com/team/ ", &core.Application{Name: "My App", ID: "app-1"}, "abcdef1234567890"); got != "registry.example.com/team/my-app:abcdef123456" {
		t.Fatalf("buildImageTagForRegistry = %q", got)
	}
	if got := buildImageTagForRegistry("", &core.Application{Name: "My App"}, "abcdef"); got != "" {
		t.Fatalf("empty registry prefix produced %q", got)
	}
	if got := buildImageTagForRegistry("repo", nil, "abcdef"); got != "" {
		t.Fatalf("nil app produced %q", got)
	}
}

func TestDeployTriggerCleanupPreviousContainers(t *testing.T) {
	runtime := &recordingDeployRuntime{
		containers: []core.ContainerInfo{{ID: "keep"}, {ID: "old-1"}, {ID: "old-2"}, {ID: ""}},
	}
	NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).cleanupPreviousAppContainers(context.Background(), runtime, "app-1", "keep")

	if len(runtime.stopped) != 2 || len(runtime.removed) != 2 {
		t.Fatalf("stopped=%v removed=%v", runtime.stopped, runtime.removed)
	}
	if runtime.stopped[0] != "old-1" || runtime.removed[1] != "old-2" {
		t.Fatalf("unexpected cleanup order: stopped=%v removed=%v", runtime.stopped, runtime.removed)
	}

	errRuntime := &recordingDeployRuntime{listErr: errors.New("list failed")}
	NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).cleanupPreviousAppContainers(context.Background(), errRuntime, "app-1", "")
	if len(errRuntime.stopped) != 0 || len(errRuntime.removed) != 0 {
		t.Fatalf("cleanup should not continue after list error: %+v", errRuntime)
	}
}

func TestEnsureDeployNetwork(t *testing.T) {
	if err := ensureDeployNetwork(context.Background(), &recordingDeployRuntime{}); err != nil {
		t.Fatalf("non network runtime should be ignored: %v", err)
	}
	rt := &networkDeployRuntime{}
	if err := ensureDeployNetwork(context.Background(), rt); err != nil {
		t.Fatalf("ensure network: %v", err)
	}
	if rt.name != "monster-network" {
		t.Fatalf("network name = %q", rt.name)
	}
	rt.err = errors.New("network failed")
	if err := ensureDeployNetwork(context.Background(), rt); err == nil {
		t.Fatal("expected network error")
	}
}

func TestDeployTriggerHandler_SubscribeWebhookDeploysBranches(t *testing.T) {
	NewDeployTriggerHandler(context.Background(), newMockStore(), nil, nil).SubscribeWebhookDeploys()

	events := core.NewEventBus(nil)
	store := newMockStore()
	store.addApp(&core.Application{ID: "image-app", TenantID: "t1", SourceType: "image"})
	store.addApp(&core.Application{ID: "git-app", TenantID: "t1", SourceType: "git", Branch: "main"})
	h := NewDeployTriggerHandler(context.Background(), store, nil, events)
	h.SubscribeWebhookDeploys()

	ctx := context.Background()
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", "bad payload"))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{}))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "missing"}))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "image-app"}))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "git-app", Branch: "feature"}))
	events.Publish(ctx, core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "git-app", Branch: "main", CommitSHA: "abcdef"}))
	events.Drain()

	if store.updatedStatus["git-app"] != "failed" {
		t.Fatalf("git app status = %q, want failed after nil runtime deploy", store.updatedStatus["git-app"])
	}
}

func TestDeployTriggerHandler_SubscribeWebhookDeploysHonorsFreeze(t *testing.T) {
	events := core.NewEventBus(nil)
	store := newMockStore()
	store.addApp(&core.Application{ID: "git-app", TenantID: "t1", SourceType: "git"})
	bolt := newMockBoltStore()
	if err := seedActiveDeployFreeze(bolt, "t1"); err != nil {
		t.Fatalf("seed freeze: %v", err)
	}
	h := NewDeployTriggerHandler(context.Background(), store, nil, events)
	h.SetDeployFreezeStore(bolt)
	h.SubscribeWebhookDeploys()

	events.Publish(context.Background(), core.NewEvent(core.EventWebhookReceived, "test", core.WebhookEventData{WebhookID: "git-app"}))
	events.Drain()

	if store.updatedStatus["git-app"] != "" {
		t.Fatalf("frozen webhook should not update app status, got %q", store.updatedStatus["git-app"])
	}
}

type fakeNodeManager struct {
	nodes map[string]core.NodeExecutor
}

func (m *fakeNodeManager) Get(serverID string) (core.NodeExecutor, error) {
	node, ok := m.nodes[serverID]
	if !ok {
		return nil, core.ErrNotFound
	}
	return node, nil
}
func (m *fakeNodeManager) Local() core.NodeExecutor            { return nil }
func (m *fakeNodeManager) All() []string                       { return nil }
func (m *fakeNodeManager) OnConnect(func(info core.AgentInfo)) {}
func (m *fakeNodeManager) OnDisconnect(func(serverID string))  {}

type fakeNodeExecutor struct {
	id string
}

func (e *fakeNodeExecutor) ServerID() string { return e.id }
func (e *fakeNodeExecutor) IsLocal() bool    { return false }
func (e *fakeNodeExecutor) CreateAndStart(context.Context, core.ContainerOpts) (string, error) {
	return "remote-container", nil
}
func (e *fakeNodeExecutor) Stop(context.Context, string, int) error     { return nil }
func (e *fakeNodeExecutor) Remove(context.Context, string, bool) error  { return nil }
func (e *fakeNodeExecutor) Restart(context.Context, string) error       { return nil }
func (e *fakeNodeExecutor) EnsureNetwork(context.Context, string) error { return nil }
func (e *fakeNodeExecutor) Logs(context.Context, string, string, bool) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (e *fakeNodeExecutor) ListByLabels(context.Context, map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}
func (e *fakeNodeExecutor) Exec(context.Context, string) (string, error)         { return "", nil }
func (e *fakeNodeExecutor) Metrics(context.Context) (*core.ServerMetrics, error) { return nil, nil }
func (e *fakeNodeExecutor) Ping(context.Context) error                           { return nil }

type recordingDeployRuntime struct {
	containers []core.ContainerInfo
	listErr    error
	stopped    []string
	removed    []string
}

func (r *recordingDeployRuntime) CreateAndStart(context.Context, core.ContainerOpts) (string, error) {
	return "container", nil
}
func (r *recordingDeployRuntime) Stop(_ context.Context, id string, _ int) error {
	r.stopped = append(r.stopped, id)
	return nil
}
func (r *recordingDeployRuntime) Remove(_ context.Context, id string, _ bool) error {
	r.removed = append(r.removed, id)
	return nil
}
func (r *recordingDeployRuntime) ListByLabels(context.Context, map[string]string) ([]core.ContainerInfo, error) {
	return r.containers, r.listErr
}

type networkDeployRuntime struct {
	recordingDeployRuntime
	name string
	err  error
}

func (r *networkDeployRuntime) EnsureNetwork(_ context.Context, name string) error {
	r.name = name
	return r.err
}
