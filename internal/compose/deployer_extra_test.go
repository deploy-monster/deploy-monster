package compose

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =============================================================================
// rollback — triggered by deployService failure
// =============================================================================

type rollbackRecorder struct {
	mockFinalRuntime
	stopped  []string
	removed  []string
}

func (r *rollbackRecorder) Stop(_ context.Context, id string, _ int) error {
	r.stopped = append(r.stopped, id)
	return nil
}

func (r *rollbackRecorder) Remove(_ context.Context, id string, _ bool) error {
	r.removed = append(r.removed, id)
	return nil
}

func TestDeploy_RollbackOnServiceFailure(t *testing.T) {
	rt := &rollbackRecorder{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	// First service succeeds, second has no image → failure → rollback first
	// depends_on ensures "app" always deploys before "broken"
	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
  broken:
    depends_on:
      - app
    # no image, no build → must fail
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		AppID:     "app-1",
		TenantID:  "t-1",
		StackName: "rollback-test",
		Compose:   cf,
	})
	if err == nil {
		t.Fatal("expected error from broken service")
	}
	if !strings.Contains(err.Error(), "no image specified") {
		t.Errorf("expected 'no image specified' error, got: %v", err)
	}

	// First service ("app") should have been rolled back (stopped and removed)
	if len(rt.stopped) != 1 {
		t.Errorf("expected 1 stop during rollback, got %d: %v", len(rt.stopped), rt.stopped)
	}
	if len(rt.removed) != 1 {
		t.Errorf("expected 1 remove during rollback, got %d: %v", len(rt.removed), rt.removed)
	}
	if len(rt.created) != 1 {
		t.Errorf("expected 1 created before rollback, got %d", len(rt.created))
	}
}

// =============================================================================
// deployService — build: directive (no image) error
// =============================================================================

type buildOnlyRuntime struct {
	mockFinalRuntime
	created []core.ContainerOpts
}

func (b *buildOnlyRuntime) CreateAndStart(_ context.Context, opts core.ContainerOpts) (string, error) {
	b.created = append(b.created, opts)
	return "container-" + opts.Name, nil
}

func TestDeployService_BuildOnlyError(t *testing.T) {
	deployer := NewStackDeployer(&mockFinalRuntime{}, nil, core.NewEventBus(slog.New(slog.NewTextHandler(io.Discard, nil))), slog.New(slog.NewTextHandler(io.Discard, nil)))
	cf := &ComposeFile{
		Services: map[string]*ServiceConfig{
			"builder": {
				Build: &BuildConfig{Context: "."},
				// no Image set
			},
		},
	}
	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "build-test",
		Compose:   cf,
	})
	if err == nil {
		t.Fatal("expected error for build-only service")
	}
	if !strings.Contains(err.Error(), "build: directive") {
		t.Errorf("expected 'build: directive' error, got: %v", err)
	}
}

// =============================================================================
// deployService — HTTP routes added to labels
// =============================================================================

type routeRecorderRuntime struct {
	mockFinalRuntime
}

func (r *routeRecorderRuntime) EnsureNetwork(_ context.Context, name string) error {
	return nil
}

func TestDeploy_WithHTTPRoutes(t *testing.T) {
	rt := &routeRecorderRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  web:
    image: myapp:latest
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		AppID:     "app-1",
		TenantID:  "t-1",
		StackName: "route-test",
		Compose:   cf,
		HTTPRoutes: []HTTPRoute{
			{ServiceName: "web", FQDN: "example.com", Port: 8080},
			{ServiceName: "other", FQDN: "other.com", Port: 80},    // wrong service → skipped
			{ServiceName: "web", FQDN: "", Port: 80},               // empty FQDN → skipped
			{ServiceName: "web", FQDN: "extra.com", Port: 0},       // port 0 → skipped
		},
	})
	if err != nil {
		t.Fatalf("Deploy with routes: %v", err)
	}
}

// =============================================================================
// labelName — comprehensive test
// =============================================================================

func TestLabelName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with spaces", "with-spaces"},
		{"UPPERCASE", "uppercase"},
		{"  trimmed  ", "trimmed"},
		{"special!@#chars", "special-chars"},
		{"multi---dash", "multi-dash"},
		{"---prefix---suffix---", "prefix-suffix"},
		{"", "route"},
		{"___", "route"},
		{"a", "a"},
		{"1abc", "1abc"},
		{"test-service", "test-service"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := labelName(tt.input)
			if got != tt.want {
				t.Errorf("labelName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// =============================================================================
// parseCPUs — comprehensive test
// =============================================================================

func TestParseCPUs(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"", 0},
		{"0", 0},
		{"-1", 0},
		{"0.5", 50000},
		{"1", 100000},
		{"  2  ", 200000},
		{"0.25", 25000},
		{"abc", 0},
	}
	for _, tt := range tests {
		t.Run(strings.ReplaceAll(tt.input, " ", "_"), func(t *testing.T) {
			got := parseCPUs(tt.input)
			if got != tt.want {
				t.Errorf("parseCPUs(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

// =============================================================================
// deployService — with CPU resource limits
// =============================================================================

type cpuLimitRuntime struct {
	mockFinalRuntime
}

func TestDeploy_WithCPULimits(t *testing.T) {
	rt := &cpuLimitRuntime{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp:latest
    deploy:
      resources:
        limits:
          cpus: "0.5"
          memory: 512m
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		AppID:     "app-1",
		TenantID:  "t-1",
		StackName: "cpu-test",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy with CPU limits: %v", err)
	}
}
