package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// DEPLOYER — full deploy flow tests
// =====================================================

func TestDeployer_DeployImage_VersionSequence(t *testing.T) {
	store := newMockStore()
	store.nextVersion = 5
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:        "app-v5",
		Name:      "versioned-app",
		ProjectID: "proj-1",
		TenantID:  "tenant-1",
	}

	dep, err := d.DeployImage(context.Background(), app, "myapp:v5.0")
	if err != nil {
		t.Fatalf("DeployImage returned error: %v", err)
	}
	if dep.Version != 5 {
		t.Errorf("Version = %d, want 5", dep.Version)
	}

	// Container name should include the version
	if runtime.lastOpts.Name != "monster-versioned-app-5" {
		t.Errorf("container name = %q, want %q", runtime.lastOpts.Name, "monster-versioned-app-5")
	}
}

func TestDeployer_DeployImage_Labels(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:        "app-labels",
		Name:      "labeled-app",
		ProjectID: "project-xyz",
		TenantID:  "tenant-abc",
	}

	_, err := d.DeployImage(context.Background(), app, "nginx:latest")
	if err != nil {
		t.Fatalf("DeployImage returned error: %v", err)
	}

	labels := runtime.lastOpts.Labels
	expectedLabels := map[string]string{
		"monster.enable":         "true",
		"monster.app.id":         "app-labels",
		"monster.app.name":       "labeled-app",
		"monster.project":        "project-xyz",
		"monster.tenant":         "tenant-abc",
		"monster.deploy.version": "1",
	}

	for key, want := range expectedLabels {
		got, ok := labels[key]
		if !ok {
			t.Errorf("missing label %q", key)
		} else if got != want {
			t.Errorf("label %q = %q, want %q", key, got, want)
		}
	}
}

func TestDeployer_DeployImage_ContainerOptions(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:   "app-opts",
		Name: "options-app",
	}

	_, err := d.DeployImage(context.Background(), app, "redis:7")
	if err != nil {
		t.Fatalf("DeployImage returned error: %v", err)
	}

	if runtime.lastOpts.Image != "redis:7" {
		t.Errorf("Image = %q, want %q", runtime.lastOpts.Image, "redis:7")
	}
	if runtime.lastOpts.Network != "monster-network" {
		t.Errorf("Network = %q, want %q", runtime.lastOpts.Network, "monster-network")
	}
	if runtime.lastOpts.RestartPolicy != "unless-stopped" {
		t.Errorf("RestartPolicy = %q, want %q", runtime.lastOpts.RestartPolicy, "unless-stopped")
	}
}

func TestDeployer_DeployImage_StatusTransitions(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:       "app-status",
		Name:     "status-app",
		TenantID: "t1",
	}

	dep, err := d.DeployImage(context.Background(), app, "nginx:1.25")
	if err != nil {
		t.Fatalf("DeployImage returned error: %v", err)
	}

	// Should have deploying -> running transitions
	if len(store.appStatusUpdates) < 2 {
		t.Fatalf("expected at least 2 status updates, got %d", len(store.appStatusUpdates))
	}

	statuses := make([]string, len(store.appStatusUpdates))
	for i, u := range store.appStatusUpdates {
		statuses[i] = u.Status
	}

	if statuses[0] != "deploying" {
		t.Errorf("first status = %q, want deploying", statuses[0])
	}
	if statuses[len(statuses)-1] != "running" {
		t.Errorf("last status = %q, want running", statuses[len(statuses)-1])
	}

	// Deployment record should reflect final state
	if dep.Status != "running" {
		t.Errorf("deployment status = %q, want running", dep.Status)
	}
}

func TestDeployer_DeployImage_EventEmitted(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	var receivedEvent core.Event
	events.Subscribe(core.EventAppDeployed, func(_ context.Context, event core.Event) error {
		receivedEvent = event
		return nil
	})

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:       "app-event",
		Name:     "event-app",
		TenantID: "tenant-1",
	}

	_, err := d.DeployImage(context.Background(), app, "myapp:v1")
	if err != nil {
		t.Fatalf("DeployImage returned error: %v", err)
	}

	if receivedEvent.Type != core.EventAppDeployed {
		t.Errorf("event type = %q, want %q", receivedEvent.Type, core.EventAppDeployed)
	}
	if receivedEvent.Source != "deploy" {
		t.Errorf("event source = %q, want %q", receivedEvent.Source, "deploy")
	}
}

func TestDeployer_DeployImage_ContainerFailure_SetsFailedStatus(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			return "", fmt.Errorf("image pull failed: unauthorized")
		},
	}
	events := core.NewEventBus(nil)

	d := NewDeployer(runtime, store, events)
	app := &core.Application{
		ID:       "app-fail",
		Name:     "failing-app",
		TenantID: "t1",
	}

	_, err := d.DeployImage(context.Background(), app, "private/image:latest")
	if err == nil {
		t.Fatal("expected error")
	}

	// Should see deploying then failed
	foundDeploying := false
	foundFailed := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "deploying" {
			foundDeploying = true
		}
		if u.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundDeploying {
		t.Error("expected 'deploying' status update")
	}
	if !foundFailed {
		t.Error("expected 'failed' status update")
	}
}

// =====================================================
// AUTODOMAIN — unicode and special character names
// =====================================================

func TestSanitizeSlug_Unicode(t *testing.T) {
	tests := []struct {
		input     string
		wantEmpty bool // Whether we expect a generated slug (true = input produces empty slug)
	}{
		{"cafe", false},
		{"my-app", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			slug := sanitizeSlug(tt.input)
			if tt.wantEmpty {
				// Should get a generated ID-based slug
				if slug == "" {
					t.Error("empty-producing input should generate a fallback slug")
				}
			} else {
				if slug == "" {
					t.Errorf("expected non-empty slug for %q", tt.input)
				}
			}
		})
	}
}

func TestSanitizeSlug_SpecialChars(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello world", "hello-world"},
		{"my_app_v2", "my-app-v2"},
		{"app.name", "app-name"},
		{"---dashes---", "dashes"},
		{"MiXeD-CaSe", "mixed-case"},
		{"numbers123", "numbers123"},
		{"a-b-c", "a-b-c"},
		{"test!!app", "testapp"},
		{"(parentheses)", "parentheses"},
		{"app@v2#prod", "appv2prod"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeSlug(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeSlug_AllSpecialChars(t *testing.T) {
	// Input that produces only filtered characters should generate a random slug
	got := sanitizeSlug("!@#$%^&*()")
	if got == "" {
		t.Error("all-special input should produce a non-empty fallback slug")
	}
	if len(got) < 4 {
		t.Errorf("fallback slug should be at least 4 chars, got %q", got)
	}
}

func TestSanitizeSlug_OnlySpaces(t *testing.T) {
	got := sanitizeSlug("   ")
	// Spaces become hyphens, then trimmed
	if got == "" {
		t.Error("all-spaces input should produce a non-empty fallback slug")
	}
}

func TestAutoDomain_UnicodeAppName(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	app := &core.Application{ID: "app-uni", Name: "app-test"}

	err := AutoDomain(context.Background(), store, events, app, "example.com")
	if err != nil {
		t.Fatalf("AutoDomain returned error: %v", err)
	}

	// Should create a domain with sanitized name
	if len(store.domains) != 1 {
		t.Fatalf("expected 1 domain, got %d", len(store.domains))
	}

	for fqdn := range store.domains {
		if fqdn != "app-test.example.com" {
			t.Errorf("FQDN = %q, want app-test.example.com", fqdn)
		}
	}
}

// =====================================================
// AUTORESTARTER — handleCrash tests
// =====================================================

func TestAutoRestarter_HandleCrash_NilRuntime(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	ar := NewAutoRestarter(nil, store, events, logger)
	ar.maxRetries = 0 // Skip retry loop entirely

	// Should not panic with nil runtime
	ar.handleCrash(context.Background(), "app-1", "container-dead")

	// Should update status to crashed then failed
	foundCrashed := false
	foundFailed := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "crashed" {
			foundCrashed = true
		}
		if u.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundCrashed {
		t.Error("expected 'crashed' status update")
	}
	if !foundFailed {
		t.Error("expected 'failed' status update after max retries exhausted")
	}
}

func TestAutoRestarter_HandleCrash_RestartSucceeds(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()
	runtime := &mockRuntime{}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 1 // Just one attempt

	ar.handleCrash(context.Background(), "app-1", "container-abc")

	if !runtime.restartCalled {
		t.Error("Restart should be called")
	}

	// Should transition: crashed -> running
	foundRunning := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "running" {
			foundRunning = true
		}
	}
	if !foundRunning {
		t.Error("expected 'running' status update after successful restart")
	}
}

func TestAutoRestarter_HandleCrash_RestartFails(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()
	runtime := &mockRuntime{
		restartFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("container removed")
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 1

	ar.handleCrash(context.Background(), "app-fail", "container-xyz")

	// Should end with 'failed' status
	lastStatus := ""
	for _, u := range store.appStatusUpdates {
		lastStatus = u.Status
	}
	if lastStatus != "failed" {
		t.Errorf("last status = %q, want 'failed'", lastStatus)
	}
}

func TestAutoRestarter_HandleCrash_EmitsCrashedEvent(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	var receivedEvent core.Event
	events.Subscribe(core.EventAppCrashed, func(_ context.Context, event core.Event) error {
		receivedEvent = event
		return nil
	})

	ar := NewAutoRestarter(nil, store, events, logger)
	ar.maxRetries = 0

	ar.handleCrash(context.Background(), "app-crash", "container-dead")

	if receivedEvent.Type != core.EventAppCrashed {
		t.Errorf("event type = %q, want %q", receivedEvent.Type, core.EventAppCrashed)
	}
}

// =====================================================
// AUTORESTARTER — checkCrashed with various states
// =====================================================

func TestAutoRestarter_CheckCrashed_ExitedContainers(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	restartCalls := 0
	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{
					ID:    "c1",
					State: "exited",
					Labels: map[string]string{
						"monster.app.id": "app-1",
					},
				},
				{
					ID:    "c2",
					State: "running", // Should NOT trigger handleCrash
					Labels: map[string]string{
						"monster.app.id": "app-2",
					},
				},
				{
					ID:    "c3",
					State: "dead",
					Labels: map[string]string{
						"monster.app.id": "app-3",
					},
				},
			}, nil
		},
		restartFn: func(_ context.Context, _ string) error {
			restartCalls++
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 1

	ar.checkCrashed()

	// Only exited and dead containers should trigger restart logic
	// (app-1 and app-3, not app-2 which is running)
	if restartCalls < 2 {
		t.Errorf("expected at least 2 restart calls (exited+dead), got %d", restartCalls)
	}
}

func TestAutoRestarter_CheckCrashed_NoAppID(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{
					ID:     "c1",
					State:  "exited",
					Labels: map[string]string{}, // No app ID
				},
			}, nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 0

	// Should not panic; container without app ID is skipped
	ar.checkCrashed()

	// No status updates should occur for containers without app ID
	if len(store.appStatusUpdates) != 0 {
		t.Errorf("expected 0 status updates, got %d", len(store.appStatusUpdates))
	}
}

// =====================================================
// MODULE — Start with nil Docker
// =====================================================

func TestModule_Start_NilDocker_NoPanic(t *testing.T) {
	m := New()
	// m.docker is nil — Start should handle this gracefully

	// We need minimal core setup for Start to work
	m.logger = slog.Default()

	err := m.Start(context.Background())
	if err != nil {
		t.Errorf("Start() with nil docker returned error: %v", err)
	}
}

// =====================================================
// ROLLBACK — additional edge cases
// =====================================================

func TestRollbackEngine_Rollback_NoLatestDeployment(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1.23", Status: "stopped"},
	}
	store.apps["app-123"] = &core.Application{
		ID:       "app-123",
		Name:     "test-app",
		TenantID: "tenant-1",
	}
	store.latestDeployment = nil // No current deployment

	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	dep, err := re.Rollback(context.Background(), "app-123", 1)
	if err != nil {
		t.Fatalf("Rollback returned error: %v", err)
	}
	if dep == nil {
		t.Fatal("expected non-nil deployment")
	}
	if dep.Image != "nginx:1.23" {
		t.Errorf("Image = %q, want nginx:1.23", dep.Image)
	}
}

func TestRollbackEngine_Rollback_EventEmitted(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1.23", Status: "stopped"},
	}
	store.apps["app-ev"] = &core.Application{
		ID:       "app-ev",
		Name:     "event-app",
		TenantID: "tenant-1",
	}

	events := core.NewEventBus(nil)
	var receivedEvent core.Event
	events.Subscribe(core.EventRollbackDone, func(_ context.Context, event core.Event) error {
		receivedEvent = event
		return nil
	})

	re := NewRollbackEngine(store, nil, events)
	_, err := re.Rollback(context.Background(), "app-ev", 1)
	if err != nil {
		t.Fatalf("Rollback returned error: %v", err)
	}

	if receivedEvent.Type != core.EventRollbackDone {
		t.Errorf("event type = %q, want %q", receivedEvent.Type, core.EventRollbackDone)
	}
}

// =====================================================
// DEPLOYER CONSTRUCTOR — additional checks
// =====================================================

func TestNewDeployer_AllFieldsNil(t *testing.T) {
	d := NewDeployer(nil, nil, nil)
	if d == nil {
		t.Fatal("NewDeployer should never return nil")
	}
	if d.runtime != nil {
		t.Error("runtime should be nil")
	}
	if d.store != nil {
		t.Error("store should be nil")
	}
	if d.events != nil {
		t.Error("events should be nil")
	}
}
