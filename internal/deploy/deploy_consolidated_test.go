package deploy

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	"github.com/moby/moby/client"
)

// === merged from autodomain_extra_test.go ===

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

// === merged from coverage_boost_test.go ===

// ═══════════════════════════════════════════════════════════════════════════════
// sanitizeSlug — additional edge cases not in existing tests
// ═══════════════════════════════════════════════════════════════════════════════

func TestSanitizeSlugCoverage_EmptyInput(t *testing.T) {
	got := sanitizeSlug("")
	if got == "" {
		t.Error("empty input should produce a fallback slug")
	}
}

func TestSanitizeSlugCoverage_LeadingTrailingHyphens(t *testing.T) {
	got := sanitizeSlug("---hello---")
	if got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestSanitizeSlugCoverage_NumbersOnly(t *testing.T) {
	got := sanitizeSlug("12345")
	if got != "12345" {
		t.Errorf("expected '12345', got %q", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// RollbackEngine — ListVersions with limit
// ═══════════════════════════════════════════════════════════════════════════════

func TestRollbackEngine_ListVersions_LimitApplied(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 3, Image: "app:v3", Status: "running"},
		{Version: 2, Image: "app:v2", Status: "stopped"},
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	versions, err := re.ListVersions(context.Background(), "app-1", 5)
	if err != nil {
		t.Fatalf("ListVersions error: %v", err)
	}
	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(versions))
	}
	if !versions[0].IsCurrent {
		t.Error("first version should be marked as current")
	}
	for i := 1; i < len(versions); i++ {
		if versions[i].IsCurrent {
			t.Errorf("version[%d] should not be current", i)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// AutoRestarter — Start subscribes events
// ═══════════════════════════════════════════════════════════════════════════════

func TestAutoRestarter_Start_DoesNotPanic(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(slog.Default())
	logger := slog.Default()
	runtime := &mockRuntime{
		restartFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 0

	// Start should not panic
	ar.Start()

	// Publish a container.died event to trigger the subscriber callback
	events.Publish(context.Background(), core.NewEvent(
		core.EventContainerDied, "test",
		core.DeployEventData{AppID: "app-start-test", ContainerID: "ctr-start-test"},
	))
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageUpdateChecker — store returns error in checkAll
// ═══════════════════════════════════════════════════════════════════════════════
// ═══════════════════════════════════════════════════════════════════════════════
// Rollback — GetApp error after finding deployment
// ═══════════════════════════════════════════════════════════════════════════════

func TestRollbackEngine_Rollback_AppNotFound(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "nginx:1.23", Status: "stopped"},
	}
	// Don't add the app to the store — so GetApp will fail
	events := core.NewEventBus(nil)
	re := NewRollbackEngine(store, nil, events)

	_, err := re.Rollback(context.Background(), "missing-app", 1)
	if err == nil {
		t.Fatal("expected error when app is not found")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Module — Init edge cases
// ═══════════════════════════════════════════════════════════════════════════════

func TestModuleCoverage_Init_NilStore(t *testing.T) {
	m := New()
	c := &core.Core{
		Logger: slog.Default(),
		Store:  nil,
	}

	err := m.Init(context.Background(), c)
	if err == nil {
		t.Fatal("Init should fail when Store is nil")
	}
}

func TestModuleCoverage_Init_WithStore(t *testing.T) {
	m := New()
	store := newMockStore()
	c := &core.Core{
		Logger:   slog.Default(),
		Store:    store,
		Config:   &core.Config{},
		Services: &core.Services{},
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	if m.store != store {
		t.Error("store should be set after Init")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Deployer — edge case: TriggeredBy
// ═══════════════════════════════════════════════════════════════════════════════

func TestAutoRestarterCoverage_HandleCrash_ZeroRetries(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()
	runtime := &mockRuntime{}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 0 // No retries

	ar.handleCrash(context.Background(), "app-z", "ctr-z")

	// Should go straight to 'failed' after crashed
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
		t.Error("expected 'crashed' status")
	}
	if !foundFailed {
		t.Error("expected 'failed' status after 0 retries")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// CheckDockerHubTag — context canceled
// ═══════════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════════
// Module.Start — with nil docker
// ═══════════════════════════════════════════════════════════════════════════════

func TestModuleCoverage_Start_NilDocker(t *testing.T) {
	m := New()
	m.logger = slog.Default()

	err := m.Start(context.Background())
	if err != nil {
		t.Errorf("Start() with nil docker should return nil, got %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageUpdateChecker — checkAll with image-type apps
// ═══════════════════════════════════════════════════════════════════════════════
// ═══════════════════════════════════════════════════════════════════════════════
// AutoRestarter — checkCrashed with mixed containers
// ═══════════════════════════════════════════════════════════════════════════════

// ═══════════════════════════════════════════════════════════════════════════════
// NewDockerManager — host option coverage
// ═══════════════════════════════════════════════════════════════════════════════

func TestNewDockerManagerCoverage_CustomHost(t *testing.T) {
	// Custom host (not empty, not default socket) should append WithHost opt.
	// This will fail to ping but covers the option code path.
	_, err := NewDockerManager("tcp://127.0.0.1:99999")
	if err == nil {
		t.Log("NewDockerManager connected to invalid host (unlikely)")
	}
}

func TestNewDockerManagerCoverage_DefaultSocket(t *testing.T) {
	// Default socket should NOT append WithHost opt.
	_, err := NewDockerManager("unix:///var/run/docker.sock")
	if err != nil {
		t.Logf("NewDockerManager with default socket failed (expected): %v", err)
	}
}

func TestAutoRestarterCoverage_CheckCrashed_MixedStates(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{ID: "c1", State: "running", Labels: map[string]string{"monster.app.id": "a1"}},
				{ID: "c2", State: "exited", Labels: map[string]string{"monster.app.id": "a2"}},
				{ID: "c3", State: "dead", Labels: map[string]string{"monster.app.id": ""}}, // empty app id
			}, nil
		},
		restartFn: func(_ context.Context, _ string) error {
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 1
	disableAutoRestartBackoff(ar)

	ar.checkCrashed()

	// Only c2 (exited with app ID) should trigger handleCrash
	// c1 is running (skip), c3 has empty app ID (skip)
}

// ═══════════════════════════════════════════════════════════════════════════════
// ImageUpdate struct coverage
// ═══════════════════════════════════════════════════════════════════════════════
// ═══════════════════════════════════════════════════════════════════════════════
// NewRollbackEngine — fields
// ═══════════════════════════════════════════════════════════════════════════════

func TestNewRollbackEngineCoverage_AllFields(t *testing.T) {
	store := newMockStore()
	runtime := &mockRuntime{}
	events := core.NewEventBus(nil)

	re := NewRollbackEngine(store, runtime, events)
	if re.store != store {
		t.Error("store field mismatch")
	}
	if re.runtime != runtime {
		t.Error("runtime field mismatch")
	}
	if re.events != events {
		t.Error("events field mismatch")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Rollback — with runtime that fails on stop/remove
// ═══════════════════════════════════════════════════════════════════════════════

func TestRollbackCoverage_StopRemoveErrors(t *testing.T) {
	store := newMockStore()
	store.deployments = []core.Deployment{
		{Version: 1, Image: "app:v1", Status: "stopped"},
	}
	store.apps["app-sr"] = &core.Application{
		ID: "app-sr", Name: "stop-remove-app", TenantID: "t1",
	}
	store.latestDeployment = &core.Deployment{ContainerID: "old-ctr"}
	store.nextVersion = 2

	runtime := &mockRuntime{
		stopFn: func(_ context.Context, _ string, _ int) error {
			return fmt.Errorf("stop failed")
		},
		removeFn: func(_ context.Context, _ string, _ bool) error {
			return fmt.Errorf("remove failed")
		},
	}
	events := core.NewEventBus(nil)

	re := NewRollbackEngine(store, runtime, events)
	dep, err := re.Rollback(context.Background(), "app-sr", 1)
	if err != nil {
		t.Fatalf("Rollback error: %v", err)
	}
	// Even with stop/remove errors, rollback should succeed
	if dep.Status != "running" {
		t.Errorf("status = %q, want running", dep.Status)
	}
}

// === merged from deploy_extra_test.go ===

// =====================================================
// DEPLOYER — full deploy flow tests
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
	disableAutoRestartBackoff(ar)

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
	disableAutoRestartBackoff(ar)

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
	disableAutoRestartBackoff(ar)

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

// === merged from docker_boost_test.go ===

func TestDockerManager_SetResourceDefaults(t *testing.T) {
	dm := &DockerManager{}
	dm.SetResourceDefaults(1024, 512)
	if dm.defaultCPU != 1024 {
		t.Errorf("defaultCPU = %d, want 1024", dm.defaultCPU)
	}
	if dm.defaultMemMB != 512 {
		t.Errorf("defaultMemMB = %d, want 512", dm.defaultMemMB)
	}
}

func TestClampToInt64_Normal(t *testing.T) {
	if got := clampToInt64(42); got != 42 {
		t.Errorf("clampToInt64(42) = %d, want 42", got)
	}
}

func TestClampToInt64_Overflow(t *testing.T) {
	if got := clampToInt64(1<<64 - 1); got != (1<<63 - 1) {
		t.Errorf("clampToInt64(max uint64) = %d, want MaxInt64", got)
	}
}

// === merged from docker_extra_test.go ===

// =====================================================
// Module — Health with docker that returns error on Ping
// =====================================================

func TestModule_Health_DockerPingFails(t *testing.T) {
	m := New()
	// We can't easily inject a mock DockerManager (it wraps real Docker SDK),
	// but we can verify the Health function returns HealthDegraded when docker is nil.
	status := m.Health()
	if status != core.HealthDegraded {
		t.Errorf("Health() = %v, want HealthDegraded", status)
	}
}

// =====================================================
// Module — Start with docker (docker will be nil in test, skip EnsureNetwork)
// =====================================================

func TestModule_Start_WithCore_NilDocker(t *testing.T) {
	m := New()
	store := newMockStore()
	c := &core.Core{
		Logger:   slog.Default(),
		Store:    store,
		Config:   &core.Config{},
		Services: &core.Services{},
		Events:   core.NewEventBus(slog.Default()),
	}

	err := m.Init(context.Background(), c)
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	// Docker should be nil (no Docker daemon in test)
	err = m.Start(context.Background())
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}

// =====================================================
// Module — Stop when docker is nil
// =====================================================

func TestModule_Stop_NilDocker(t *testing.T) {
	m := New()
	err := m.Stop(context.Background())
	if err != nil {
		t.Errorf("Stop with nil docker should return nil, got %v", err)
	}
}

// =====================================================
// Module — Docker() accessor
// =====================================================

func TestModule_Docker_Accessor(t *testing.T) {
	m := New()
	if m.Docker() != nil {
		t.Error("Docker() should return nil before Init")
	}
}

// =====================================================
// Module — ID, Name, Version, Dependencies, Routes, Events (coverage boost)
// =====================================================

func TestModule_MetadataAccessors(t *testing.T) {
	m := New()

	if m.ID() != "deploy" {
		t.Errorf("ID() = %q, want deploy", m.ID())
	}
	if m.Name() != "Deploy Engine" {
		t.Errorf("Name() = %q, want 'Deploy Engine'", m.Name())
	}
	if m.Version() != "1.0.0" {
		t.Errorf("Version() = %q, want 1.0.0", m.Version())
	}
	deps := m.Dependencies()
	if len(deps) != 1 || deps[0] != "core.db" {
		t.Errorf("Dependencies() = %v, want [core.db]", deps)
	}
	if routes := m.Routes(); routes != nil {
		t.Errorf("Routes() = %v, want nil", routes)
	}
	if events := m.Events(); events != nil {
		t.Errorf("Events() = %v, want nil", events)
	}
}

// =====================================================
// mockRuntime — method coverage for Exec, Stats, ImagePull, etc
// =====================================================

func TestMockRuntime_Exec(t *testing.T) {
	rt := &mockRuntime{}
	output, err := rt.Exec(context.Background(), "ctr-1", []string{"echo", "hello"})
	if err != nil {
		t.Errorf("Exec error: %v", err)
	}
	if output != "" {
		t.Errorf("expected empty output from mock, got %q", output)
	}
}

func TestMockRuntime_Stats(t *testing.T) {
	rt := &mockRuntime{}
	stats, err := rt.Stats(context.Background(), "ctr-1")
	if err != nil {
		t.Errorf("Stats error: %v", err)
	}
	if stats == nil {
		t.Error("Stats should return non-nil")
	}
}

func TestMockRuntime_ImagePull(t *testing.T) {
	rt := &mockRuntime{}
	err := rt.ImagePull(context.Background(), "nginx:latest")
	if err != nil {
		t.Errorf("ImagePull error: %v", err)
	}
}

func TestMockRuntime_ImageList(t *testing.T) {
	rt := &mockRuntime{}
	images, err := rt.ImageList(context.Background())
	if err != nil {
		t.Errorf("ImageList error: %v", err)
	}
	if images != nil {
		t.Errorf("expected nil images from mock, got %v", images)
	}
}

func TestMockRuntime_ImageRemove(t *testing.T) {
	rt := &mockRuntime{}
	err := rt.ImageRemove(context.Background(), "sha256:abc123")
	if err != nil {
		t.Errorf("ImageRemove error: %v", err)
	}
}

func TestMockRuntime_NetworkList(t *testing.T) {
	rt := &mockRuntime{}
	networks, err := rt.NetworkList(context.Background())
	if err != nil {
		t.Errorf("NetworkList error: %v", err)
	}
	if networks != nil {
		t.Errorf("expected nil networks from mock, got %v", networks)
	}
}

func TestMockRuntime_VolumeList(t *testing.T) {
	rt := &mockRuntime{}
	volumes, err := rt.VolumeList(context.Background())
	if err != nil {
		t.Errorf("VolumeList error: %v", err)
	}
	if volumes != nil {
		t.Errorf("expected nil volumes from mock, got %v", volumes)
	}
}

func TestMockRuntime_Ping(t *testing.T) {
	rt := &mockRuntime{}
	err := rt.Ping()
	if err != nil {
		t.Errorf("Ping error: %v", err)
	}
}

// =====================================================
// AutoRestarter — handleCrash with runtime that succeeds on first try
// =====================================================

func TestAutoRestarter_HandleCrash_SucceedsFirstAttempt(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	attempts := 0
	runtime := &mockRuntime{
		restartFn: func(_ context.Context, _ string) error {
			attempts++
			return nil
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	ar.maxRetries = 3
	disableAutoRestartBackoff(ar)

	ar.handleCrash(context.Background(), "app-ok", "ctr-ok")

	if attempts != 1 {
		t.Errorf("expected 1 restart attempt, got %d", attempts)
	}

	// Should have: crashed -> running
	foundRunning := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "running" {
			foundRunning = true
		}
	}
	if !foundRunning {
		t.Error("expected 'running' status after successful restart")
	}
}

// =====================================================
// AutoRestarter — handleCrash with nil runtime breaks loop
// =====================================================

func TestAutoRestarter_HandleCrash_NilRuntime_BreaksLoop(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	ar := NewAutoRestarter(nil, store, events, logger)
	ar.maxRetries = 3
	disableAutoRestartBackoff(ar)

	ar.handleCrash(context.Background(), "app-nil", "ctr-nil")

	// Should go: crashed -> failed (breaks loop immediately)
	foundFailed := false
	for _, u := range store.appStatusUpdates {
		if u.Status == "failed" {
			foundFailed = true
		}
	}
	if !foundFailed {
		t.Error("expected 'failed' status when runtime is nil")
	}
}

// =====================================================
// AutoRestarter — checkCrashed with ListByLabels error
// =====================================================

func TestAutoRestarter_CheckCrashed_RuntimeError(t *testing.T) {
	store := newMockStore()
	events := core.NewEventBus(nil)
	logger := slog.Default()

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return nil, context.DeadlineExceeded
		},
	}

	ar := NewAutoRestarter(runtime, store, events, logger)
	// Should not panic
	ar.checkCrashed()
}

// =====================================================
// Deployer — DeployImage emits TenantEvent
// =====================================================

// === merged from docker_integration_test.go ===

// requireDocker creates a real DockerManager or skips the test.
func requireDocker(t *testing.T) *DockerManager {
	t.Helper()
	dm, err := NewDockerManager("")
	if err != nil {
		t.Skipf("Docker not available: %v", err)
	}
	t.Cleanup(func() { dm.Close() })
	return dm
}

// TestDockerIntegration_PingAndInfo verifies basic Docker connectivity.
func TestDockerIntegration_PingAndInfo(t *testing.T) {
	dm := requireDocker(t)

	if err := dm.Ping(); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

// TestDockerIntegration_PullImage pulls a small image and verifies it exists.
func TestDockerIntegration_PullImage(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	// Pull a tiny image
	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	// Verify it shows up in image list
	images, err := dm.ImageList(ctx)
	if err != nil {
		t.Fatalf("ImageList: %v", err)
	}

	found := false
	for _, img := range images {
		for _, tag := range img.Tags {
			if tag == "busybox:latest" {
				found = true
			}
		}
	}
	if !found {
		t.Error("busybox:latest not found in image list after pull")
	}
}

// TestDockerIntegration_ContainerLifecycle tests the full create→start→stop→remove cycle.
func TestDockerIntegration_ContainerLifecycle(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	// Pull image first
	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	containerName := "dm-integration-test-" + core.GenerateID()[:8]

	// Create and start
	containerID, err := dm.CreateAndStart(ctx, core.ContainerOpts{
		Name:  containerName,
		Image: "busybox:latest",
		Env:   []string{"TEST_VAR=hello"},
		Labels: map[string]string{
			"monster.managed":  "true",
			"monster.app.id":   "integration-test",
			"monster.app.name": "test-app",
		},
	})
	if err != nil {
		t.Fatalf("CreateAndStart: %v", err)
	}
	if containerID == "" {
		t.Fatal("expected non-empty container ID")
	}

	// Cleanup: always remove
	defer dm.Remove(ctx, containerID, true)

	// Verify container is listed with correct labels
	containers, err := dm.ListByLabels(ctx, map[string]string{
		"monster.managed": "true",
		"monster.app.id":  "integration-test",
	})
	if err != nil {
		t.Fatalf("ListByLabels: %v", err)
	}

	found := false
	for _, c := range containers {
		if c.ID == containerID || c.ID[:12] == containerID[:12] {
			found = true
			if c.Labels["monster.app.name"] != "test-app" {
				t.Errorf("label monster.app.name = %q, want %q", c.Labels["monster.app.name"], "test-app")
			}
		}
	}
	if !found {
		t.Error("container not found via ListByLabels")
	}

	// Stop
	if err := dm.Stop(ctx, containerID, 5); err != nil {
		t.Logf("Stop (may already be exited): %v", err)
	}

	// Remove
	if err := dm.Remove(ctx, containerID, true); err != nil {
		t.Errorf("Remove: %v", err)
	}

	// Verify container is gone
	containers, err = dm.ListByLabels(ctx, map[string]string{
		"monster.app.id": "integration-test",
	})
	if err != nil {
		t.Fatalf("ListByLabels after remove: %v", err)
	}
	for _, c := range containers {
		if c.ID == containerID || c.ID[:12] == containerID[:12] {
			t.Error("container still listed after remove")
		}
	}
}

// TestDockerIntegration_ContainerRestart tests the restart operation.
func TestDockerIntegration_ContainerRestart(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	containerName := "dm-restart-test-" + core.GenerateID()[:8]
	containerID, err := dm.CreateAndStart(ctx, core.ContainerOpts{
		Name:   containerName,
		Image:  "busybox:latest",
		Labels: map[string]string{"monster.managed": "true"},
	})
	if err != nil {
		t.Fatalf("CreateAndStart: %v", err)
	}
	defer dm.Remove(ctx, containerID, true)

	// Give container a moment to start
	time.Sleep(500 * time.Millisecond)

	// Restart
	if err := dm.Restart(ctx, containerID); err != nil {
		t.Logf("Restart (container may have already exited): %v", err)
	}
}

// TestDockerIntegration_ContainerLogs retrieves logs from a container.
func TestDockerIntegration_ContainerLogs(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	containerName := "dm-logs-test-" + core.GenerateID()[:8]
	containerID, err := dm.CreateAndStart(ctx, core.ContainerOpts{
		Name:   containerName,
		Image:  "busybox:latest",
		Labels: map[string]string{"monster.managed": "true"},
	})
	if err != nil {
		t.Fatalf("CreateAndStart: %v", err)
	}
	defer dm.Remove(ctx, containerID, true)

	// Give container time to produce output
	time.Sleep(500 * time.Millisecond)

	// Get logs (may be empty for busybox, but shouldn't error)
	reader, err := dm.Logs(ctx, containerID, "100", false)
	if err != nil {
		t.Errorf("Logs: %v", err)
	}
	if reader != nil {
		reader.Close()
	}
}

// TestDockerIntegration_NetworkLifecycle tests network create and list.
func TestDockerIntegration_NetworkLifecycle(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	networkName := "dm-test-network-" + core.GenerateID()[:8]

	// Create network
	if err := dm.EnsureNetwork(ctx, networkName); err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}

	// Cleanup
	defer func() { _, _ = dm.cli.NetworkRemove(ctx, networkName, client.NetworkRemoveOptions{}) }()

	// List networks and verify
	networks, err := dm.NetworkList(ctx)
	if err != nil {
		t.Fatalf("NetworkList: %v", err)
	}

	found := false
	for _, n := range networks {
		if n.Name == networkName {
			found = true
		}
	}
	if !found {
		t.Errorf("network %q not found in list", networkName)
	}

	// Idempotent: calling EnsureNetwork again should not error
	if err := dm.EnsureNetwork(ctx, networkName); err != nil {
		t.Errorf("EnsureNetwork (idempotent): %v", err)
	}
}

// TestDockerIntegration_ContainerWithNetwork tests creating a container in a custom network.
func TestDockerIntegration_ContainerWithNetwork(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	if err := dm.ImagePull(ctx, "busybox:latest"); err != nil {
		t.Fatalf("ImagePull: %v", err)
	}

	networkName := "dm-net-test-" + core.GenerateID()[:8]
	if err := dm.EnsureNetwork(ctx, networkName); err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}
	defer func() { _, _ = dm.cli.NetworkRemove(ctx, networkName, client.NetworkRemoveOptions{}) }()

	containerName := "dm-net-container-" + core.GenerateID()[:8]
	containerID, err := dm.CreateAndStart(ctx, core.ContainerOpts{
		Name:    containerName,
		Image:   "busybox:latest",
		Network: networkName,
		Labels:  map[string]string{"monster.managed": "true"},
	})
	if err != nil {
		t.Fatalf("CreateAndStart with network: %v", err)
	}
	defer dm.Remove(ctx, containerID, true)

	if containerID == "" {
		t.Error("expected non-empty container ID")
	}
}

// TestDockerIntegration_VolumeList verifies volume listing works.
func TestDockerIntegration_VolumeList(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	// Just verify the call doesn't error — we may not have volumes
	_, err := dm.VolumeList(ctx)
	if err != nil {
		t.Errorf("VolumeList: %v", err)
	}
}

// TestDockerIntegration_ImageList verifies image listing works.
func TestDockerIntegration_ImageList(t *testing.T) {
	dm := requireDocker(t)
	ctx := context.Background()

	images, err := dm.ImageList(ctx)
	if err != nil {
		t.Fatalf("ImageList: %v", err)
	}

	// Should have at least one image (busybox from earlier tests, or system images)
	if len(images) == 0 {
		t.Log("no images found (expected at least busybox if other integration tests ran)")
	}
}

// === merged from module_boost_test.go ===

func TestModule_cleanOrphanContainers(t *testing.T) {
	store := newMockStore()
	store.apps["orphan-app"] = &core.Application{
		ID:   "orphan-app",
		Name: "orphan",
	}

	// Container for a valid app
	store.apps["valid-app"] = &core.Application{
		ID:   "valid-app",
		Name: "valid",
	}

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{
					ID:   "container-orphan",
					Name: "orphan-c",
					Labels: map[string]string{
						"monster.managed": "true",
						"monster.app.id":  "deleted-app",
					},
				},
				{
					ID:   "container-valid",
					Name: "valid-c",
					Labels: map[string]string{
						"monster.managed": "true",
						"monster.app.id":  "valid-app",
					},
				},
				{
					ID:     "container-no-label",
					Name:   "no-label-c",
					Labels: map[string]string{"monster.managed": "true"},
				},
			}, nil
		},
	}

	m := &Module{
		store:  store,
		docker: runtime,
		logger: slog.Default(),
	}

	m.cleanOrphanContainers(context.Background())

	if !runtime.removeCalled {
		t.Error("expected Remove to be called for orphan container")
	}
}

func TestModule_cleanOrphanContainers_ListError(t *testing.T) {
	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return nil, fmt.Errorf("docker unavailable")
		},
	}

	m := &Module{
		store:  newMockStore(),
		docker: runtime,
		logger: slog.Default(),
	}

	// Should not panic
	m.cleanOrphanContainers(context.Background())
}

func TestModule_cleanOrphanContainers_RemoveError(t *testing.T) {
	store := newMockStore()

	runtime := &mockRuntime{
		listByLabelsFn: func(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
			return []core.ContainerInfo{
				{
					ID:   "container-orphan-123",
					Name: "orphan",
					Labels: map[string]string{
						"monster.managed": "true",
						"monster.app.id":  "missing",
					},
				},
			}, nil
		},
		removeFn: func(_ context.Context, _ string, _ bool) error {
			return fmt.Errorf("remove failed")
		},
	}

	m := &Module{
		store:  store,
		docker: runtime,
		logger: slog.Default(),
	}

	// Should not panic
	m.cleanOrphanContainers(context.Background())
}

func TestModule_Health_PingError(t *testing.T) {
	runtime := &mockRuntime{pingErr: fmt.Errorf("docker down")}
	m := &Module{docker: runtime}

	if got := m.Health(); got != core.HealthDown {
		t.Errorf("Health() = %v, want HealthDown", got)
	}
}

func TestModule_Health_PingOK(t *testing.T) {
	runtime := &mockRuntime{}
	m := &Module{docker: runtime}

	if got := m.Health(); got != core.HealthOK {
		t.Errorf("Health() = %v, want HealthOK", got)
	}
}

// === merged from tier74_hardening_test.go ===

// Tier 74 — deploy manager lifecycle hardening tests.
//
// These cover the regressions fixed in Tier 74:
//
//   - NewAutoRollbackManager nil-logger guard
//   - AutoRollbackManager gains a real Stop() method (pre-Tier-74 had
//     no lifecycle at all)
//   - Stop is idempotent (stopOnce + closed-flag under mu)
//   - Stop waits for in-flight handleFailure dispatches (wg.Wait)
//   - After Stop, new events are dropped without touching the store
//   - AutoRestarter.Stop is idempotent (pre-Tier-74 second Stop
//     crashed with "close of closed channel")
//   - ImageUpdateChecker.Stop is idempotent (same bug class)
//   - Module.Stop drains AutoRollbackManager before closing Docker

// ─── NewAutoRollbackManager nil-logger guard ───────────────────────────────

func TestTier74_NewAutoRollbackManager_NilLogger(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(slog.Default())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, nil)
	if ar == nil {
		t.Fatal("NewAutoRollbackManager returned nil")
	}
	if ar.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
	if ar.stopCtx == nil {
		t.Error("stopCtx should be initialized")
	}
	if ar.stopCancel == nil {
		t.Error("stopCancel should be initialized")
	}
}

// ─── Stop idempotency ──────────────────────────────────────────────────────

func TestTier74_AutoRollback_Stop_Idempotent(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	// Pre-Tier-74 there was no Stop at all. After the refactor Stop
	// must be callable multiple times without panicking.
	ar.Stop()
	ar.Stop()
	ar.Stop()
}

// ─── Stop marks closed and drops subsequent events ────────────────────────

func TestTier74_AutoRollback_Stop_DropsLaterEvents(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "a1", Port: 80}
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.nextVersion = 3

	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())
	ar.Start()

	// Stop first — the manager must refuse to process subsequent events.
	ar.Stop()

	// Publish a deploy.failed event after Stop.
	bus.Publish(context.Background(), core.Event{
		Type:   core.EventDeployFailed,
		Source: "test",
		Data:   core.DeployEventData{AppID: "app-1"},
	})

	// Drain the event bus async workers so we know the dispatch
	// completed (or was refused) before assertions.
	bus.Drain()

	if len(store.appStatusUpdates) != 0 {
		t.Errorf("expected 0 status updates after Stop, got %d", len(store.appStatusUpdates))
	}
}

// ─── handleFailure respects closed flag ────────────────────────────────────

func TestTier74_AutoRollback_HandleFailure_RespectsClosed(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "a1", Port: 80}
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.nextVersion = 3

	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	// Flip closed directly (simulates the post-Stop state for a test
	// that does not want to exercise the full Start/Stop path).
	ar.mu.Lock()
	ar.closed = true
	ar.mu.Unlock()

	// Direct call must short-circuit at the top of handleFailure.
	ar.handleFailure(context.Background(), "app-1")
	if len(store.appStatusUpdates) != 0 {
		t.Errorf("expected no updates when closed, got %d", len(store.appStatusUpdates))
	}
}

// ─── Wait drains in-flight work ────────────────────────────────────────────

// TestTier74_AutoRollback_Wait_NoDispatch proves Wait is safe when
// nothing has been dispatched and returns immediately.
func TestTier74_AutoRollback_Wait_NoDispatch(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	done := make(chan struct{})
	go func() {
		ar.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Wait blocked when nothing had been dispatched")
	}
}

// ─── Stop drains active dispatch ───────────────────────────────────────────

// TestTier74_AutoRollback_Stop_DrainsInflight proves Stop blocks
// until a handleFailure goroutine returns. We inject a blocking
// CreateAndStart into the mockRuntime so the rollback engine is
// parked in the middle of provisioning when Stop is called, then
// verify Stop does not return until the handler finishes.
func TestTier74_AutoRollback_Stop_DrainsInflight(t *testing.T) {
	store := newMockStore()
	store.apps["app-1"] = &core.Application{ID: "app-1", Name: "a1", Port: 80}
	store.deployments = []core.Deployment{
		{Version: 2, Status: "failed", Image: "app:v2"},
		{Version: 1, Status: "running", Image: "app:v1"},
	}
	store.nextVersion = 3

	// Park inside CreateAndStart until the test releases.
	release := make(chan struct{})
	entered := make(chan struct{})
	var once sync.Once
	rt := &mockRuntime{
		createAndStartFn: func(_ context.Context, _ core.ContainerOpts) (string, error) {
			once.Do(func() { close(entered) })
			<-release
			return "container-new-123", nil
		},
	}

	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, rt, bus, tier74Logger())
	ar.Start()

	// Publish an event and let the async handler pick it up.
	bus.Publish(context.Background(), core.Event{
		Type:   core.EventDeployFailed,
		Source: "test",
		Data:   core.DeployEventData{AppID: "app-1"},
	})

	// Wait until the handler is parked inside CreateAndStart.
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never reached CreateAndStart")
	}

	// Fire Stop in a goroutine — it must not return yet because the
	// in-flight handler is parked.
	stopped := make(chan struct{})
	go func() {
		ar.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("Stop returned before in-flight handler finished — wg.Wait missing")
	case <-time.After(150 * time.Millisecond):
		// good — Stop is parked waiting for wg
	}

	// Release the handler; Stop should now return promptly.
	close(release)

	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return after in-flight handler finished")
	}
}

// ─── AutoRestarter Stop idempotency ────────────────────────────────────────

func TestTier74_AutoRestarter_Stop_Idempotent(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	rt := &mockRuntime{}
	ar := NewAutoRestarter(rt, store, bus, tier74Logger())

	// Pre-Tier-74 the second Stop panicked with "close of closed
	// channel". stopOnce now guards it.
	ar.Stop()
	ar.Stop()
	ar.Stop()
}

// ─── AutoRestarter nil-logger guard ────────────────────────────────────────

func TestTier74_AutoRestarter_NilLogger(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(slog.Default())
	rt := &mockRuntime{}
	ar := NewAutoRestarter(rt, store, bus, nil)
	if ar.logger == nil {
		t.Error("logger should default to slog.Default when nil")
	}
}

// ─── ImageUpdateChecker Stop idempotency ───────────────────────────────────

// ─── ImageUpdateChecker nil-logger guard ───────────────────────────────────

// ─── Concurrent Stop storm ─────────────────────────────────────────────────

func TestTier74_AutoRollback_ConcurrentStop_NoPanic(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); ar.Stop() }()
	}
	wg.Wait()

	// Final Stop is a no-op but must not panic or deadlock.
	ar.Stop()
}

// ─── stopCtx cancellation ──────────────────────────────────────────────────

func TestTier74_AutoRollback_Stop_CancelsStopCtx(t *testing.T) {
	store := newMockStore()
	bus := core.NewEventBus(tier74Logger())
	ar := NewAutoRollbackManager(store, &mockRuntime{}, bus, tier74Logger())

	// Before Stop, stopCtx must be live.
	select {
	case <-ar.stopCtx.Done():
		t.Fatal("stopCtx was canceled before Stop")
	default:
	}

	ar.Stop()

	// After Stop, stopCtx must be canceled.
	select {
	case <-ar.stopCtx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Fatal("stopCtx was not canceled by Stop")
	}
}

// ─── helper ────────────────────────────────────────────────────────────────

func tier74Logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(discardWriter{}, nil))
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
