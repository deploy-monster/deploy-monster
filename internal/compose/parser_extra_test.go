package compose

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ---------------------------------------------------------------------------
// Variable interpolation ${VAR:-default}
// ---------------------------------------------------------------------------

func TestInterpolate_DefaultValues(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		vars     map[string]string
		contains string
	}{
		{
			name:     "use default when var missing",
			input:    "image: ${TAG:-latest}",
			vars:     map[string]string{},
			contains: "image: latest",
		},
		{
			name:     "use default when var empty",
			input:    "image: ${TAG:-latest}",
			vars:     map[string]string{"TAG": ""},
			contains: "image: latest",
		},
		{
			name:     "override default when var present",
			input:    "image: ${TAG:-latest}",
			vars:     map[string]string{"TAG": "v2.0"},
			contains: "image: v2.0",
		},
		{
			name:     "multiple vars in one line",
			input:    "${HOST:-localhost}:${PORT:-5432}",
			vars:     map[string]string{"HOST": "db.prod"},
			contains: "db.prod:5432",
		},
		{
			name:     "var without default - found",
			input:    "host: ${DB_HOST}",
			vars:     map[string]string{"DB_HOST": "10.0.0.1"},
			contains: "host: 10.0.0.1",
		},
		{
			name:     "var without default - not found (unchanged)",
			input:    "host: ${DB_HOST}",
			vars:     map[string]string{},
			contains: "host: ${DB_HOST}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := string(Interpolate([]byte(tt.input), tt.vars))
			if !strings.Contains(result, tt.contains) {
				t.Errorf("expected result to contain %q, got %q", tt.contains, result)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Dependency ordering (depends_on)
// ---------------------------------------------------------------------------

func TestDependencyOrder_LinearChain(t *testing.T) {
	yaml := `
services:
  frontend:
    image: nginx
    depends_on:
      - backend
  backend:
    image: node
    depends_on:
      - database
  database:
    image: postgres
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	order := cf.DependencyOrder()
	idx := indexMap(order)

	if idx["database"] > idx["backend"] {
		t.Error("database must come before backend")
	}
	if idx["backend"] > idx["frontend"] {
		t.Error("backend must come before frontend")
	}
}

func TestDependencyOrder_MapFormat(t *testing.T) {
	yaml := `
services:
  web:
    image: nginx
    depends_on:
      api:
        condition: service_healthy
  api:
    image: node
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	order := cf.DependencyOrder()
	idx := indexMap(order)

	if idx["api"] > idx["web"] {
		t.Error("api must come before web")
	}
}

func TestDependencyOrder_NoDeps(t *testing.T) {
	yaml := `
services:
  svc1:
    image: alpine
  svc2:
    image: alpine
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	order := cf.DependencyOrder()
	if len(order) != 2 {
		t.Errorf("expected 2 services, got %d", len(order))
	}
}

// ---------------------------------------------------------------------------
// Volume parsing
// ---------------------------------------------------------------------------

func TestParse_Volumes(t *testing.T) {
	yaml := `
services:
  db:
    image: postgres
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./config:/etc/config:ro

volumes:
  pgdata:
    driver: local
  logs:
    external: true
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(cf.Volumes) != 2 {
		t.Errorf("expected 2 named volumes, got %d", len(cf.Volumes))
	}

	if cf.Volumes["pgdata"] == nil {
		t.Fatal("pgdata volume not found")
	}
	if cf.Volumes["pgdata"].Driver != "local" {
		t.Errorf("expected driver 'local', got %q", cf.Volumes["pgdata"].Driver)
	}

	if cf.Volumes["logs"] == nil {
		t.Fatal("logs volume not found")
	}
	if !cf.Volumes["logs"].External {
		t.Error("logs volume should be external")
	}

	db := cf.Services["db"]
	if db == nil {
		t.Fatal("db service not found")
	}
	if len(db.Volumes) != 2 {
		t.Errorf("expected 2 volume mounts, got %d", len(db.Volumes))
	}
}

// ---------------------------------------------------------------------------
// Network parsing
// ---------------------------------------------------------------------------

func TestParse_Networks(t *testing.T) {
	yaml := `
services:
  app:
    image: myapp
    networks:
      - frontend
      - backend

networks:
  frontend:
    driver: bridge
  backend:
    driver: overlay
    labels:
      env: production
  external_net:
    external: true
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(cf.Networks) != 3 {
		t.Errorf("expected 3 networks, got %d", len(cf.Networks))
	}

	fe := cf.Networks["frontend"]
	if fe == nil || fe.Driver != "bridge" {
		t.Error("frontend network should have driver 'bridge'")
	}

	be := cf.Networks["backend"]
	if be == nil || be.Driver != "overlay" {
		t.Error("backend network should have driver 'overlay'")
	}
	if be.Labels["env"] != "production" {
		t.Errorf("backend label 'env' expected 'production', got %q", be.Labels["env"])
	}

	ext := cf.Networks["external_net"]
	if ext == nil || !ext.External {
		t.Error("external_net should be external")
	}
}

// ---------------------------------------------------------------------------
// Port mapping parsing (short format)
// ---------------------------------------------------------------------------

func TestParse_Ports(t *testing.T) {
	yaml := `
services:
  web:
    image: nginx
    ports:
      - "80:80"
      - "443:443"
      - "8080:80"
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	web := cf.Services["web"]
	if web == nil {
		t.Fatal("web service not found")
	}

	expected := []string{"80:80", "443:443", "8080:80"}
	if len(web.Ports) != len(expected) {
		t.Fatalf("expected %d ports, got %d", len(expected), len(web.Ports))
	}
	for i, p := range expected {
		if web.Ports[i] != p {
			t.Errorf("port[%d]: expected %q, got %q", i, p, web.Ports[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Service environment variables — both list and map format
// ---------------------------------------------------------------------------

func TestResolveEnv_MapFormat(t *testing.T) {
	env := resolveEnv(map[string]any{
		"DB_HOST":     "localhost",
		"DB_PORT":     5432,
		"DEBUG":       true,
		"EMPTY_VALUE": "",
	})

	tests := []struct {
		key, expected string
	}{
		{"DB_HOST", "localhost"},
		{"DB_PORT", "5432"},
		{"DEBUG", "true"},
		{"EMPTY_VALUE", ""},
	}
	for _, tt := range tests {
		if env[tt.key] != tt.expected {
			t.Errorf("env[%s]: expected %q, got %q", tt.key, tt.expected, env[tt.key])
		}
	}
}

func TestResolveEnv_ListFormat(t *testing.T) {
	env := resolveEnv([]any{
		"KEY1=value1",
		"KEY2=value2",
		"KEY3=has=equals",
		"KEY_NO_VALUE",
	})

	if env["KEY1"] != "value1" {
		t.Errorf("KEY1: expected 'value1', got %q", env["KEY1"])
	}
	if env["KEY2"] != "value2" {
		t.Errorf("KEY2: expected 'value2', got %q", env["KEY2"])
	}
	if env["KEY3"] != "has=equals" {
		t.Errorf("KEY3: expected 'has=equals', got %q", env["KEY3"])
	}
	if val, ok := env["KEY_NO_VALUE"]; !ok || val != "" {
		t.Errorf("KEY_NO_VALUE: expected empty string, got %q (ok=%v)", val, ok)
	}
}

func TestResolveEnv_Nil(t *testing.T) {
	env := resolveEnv(nil)
	if len(env) != 0 {
		t.Errorf("expected empty map for nil input, got %d entries", len(env))
	}
}

func TestResolveEnv_UnsupportedType(t *testing.T) {
	env := resolveEnv("not-a-map-or-list")
	if len(env) != 0 {
		t.Errorf("expected empty map for unsupported type, got %d entries", len(env))
	}
}

// ---------------------------------------------------------------------------
// Environment variables parsed from full YAML
// ---------------------------------------------------------------------------

func TestParse_EnvMapInYAML(t *testing.T) {
	yaml := `
services:
  app:
    image: myapp
    environment:
      DB_HOST: localhost
      DB_PORT: 5432
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	app := cf.Services["app"]
	if app.ResolvedEnv["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST: expected 'localhost', got %q", app.ResolvedEnv["DB_HOST"])
	}
	if app.ResolvedEnv["DB_PORT"] != "5432" {
		t.Errorf("DB_PORT: expected '5432', got %q", app.ResolvedEnv["DB_PORT"])
	}
}

func TestParse_EnvListInYAML(t *testing.T) {
	yaml := `
services:
  app:
    image: myapp
    environment:
      - DB_HOST=localhost
      - DB_PORT=5432
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	app := cf.Services["app"]
	if app.ResolvedEnv["DB_HOST"] != "localhost" {
		t.Errorf("DB_HOST: expected 'localhost', got %q", app.ResolvedEnv["DB_HOST"])
	}
}

// ---------------------------------------------------------------------------
// Invalid YAML handling
// ---------------------------------------------------------------------------

func TestParse_InvalidYAML(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"garbage", "{{{{not yaml at all"},
		{"empty", ""},
		{"no services key", "version: '3'\nfoo: bar"},
		{"services is null", "services:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.input))
			if err == nil {
				t.Error("expected error for invalid YAML")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Parse — service with build directive
// ---------------------------------------------------------------------------

func TestParse_BuildDirective(t *testing.T) {
	yaml := `
services:
  app:
    build:
      context: .
      dockerfile: Dockerfile.prod
      args:
        NODE_ENV: production
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	app := cf.Services["app"]
	if app.Build == nil {
		t.Fatal("build config should not be nil")
	}
	if app.Build.Context != "." {
		t.Errorf("build context: expected '.', got %q", app.Build.Context)
	}
	if app.Build.Dockerfile != "Dockerfile.prod" {
		t.Errorf("dockerfile: expected 'Dockerfile.prod', got %q", app.Build.Dockerfile)
	}
	if app.Build.Args["NODE_ENV"] != "production" {
		t.Errorf("build arg NODE_ENV: expected 'production', got %q", app.Build.Args["NODE_ENV"])
	}
}

// ---------------------------------------------------------------------------
// Parse — healthcheck, deploy, labels
// ---------------------------------------------------------------------------

func TestParse_HealthCheck(t *testing.T) {
	yaml := `
services:
  app:
    image: myapp
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost/health"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 5s
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	hc := cf.Services["app"].HealthCheck
	if hc == nil {
		t.Fatal("healthcheck should not be nil")
	}
	if hc.Interval != "30s" {
		t.Errorf("interval: expected '30s', got %q", hc.Interval)
	}
	if hc.Retries != 3 {
		t.Errorf("retries: expected 3, got %d", hc.Retries)
	}
}

func TestParse_Labels(t *testing.T) {
	yaml := `
services:
  web:
    image: nginx
    labels:
      monster.enable: "true"
      traefik.enable: "false"
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	labels := cf.Services["web"].Labels
	if labels["monster.enable"] != "true" {
		t.Errorf("monster.enable: expected 'true', got %q", labels["monster.enable"])
	}
	if labels["traefik.enable"] != "false" {
		t.Errorf("traefik.enable: expected 'false', got %q", labels["traefik.enable"])
	}
}

// ---------------------------------------------------------------------------
// Parse — restart policy and other fields
// ---------------------------------------------------------------------------

func TestParse_RestartPolicy(t *testing.T) {
	yaml := `
services:
  app:
    image: myapp
    restart: always
    hostname: myhost
    working_dir: /app
    user: "1000:1000"
`
	cf, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	app := cf.Services["app"]
	if app.Restart != "always" {
		t.Errorf("restart: expected 'always', got %q", app.Restart)
	}
	if app.Hostname != "myhost" {
		t.Errorf("hostname: expected 'myhost', got %q", app.Hostname)
	}
	if app.WorkingDir != "/app" {
		t.Errorf("working_dir: expected '/app', got %q", app.WorkingDir)
	}
	if app.User != "1000:1000" {
		t.Errorf("user: expected '1000:1000', got %q", app.User)
	}
}

// ---------------------------------------------------------------------------
// parseMemory
// ---------------------------------------------------------------------------

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
	}{
		{"512m", 512},
		{"512mb", 512},
		{"1g", 1024},
		{"2gb", 2048},
		{"256M", 256},
		{"1G", 1024},
		{"unknown", 0},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseMemory(tt.input)
			if got != tt.expected {
				t.Errorf("parseMemory(%q): expected %d, got %d", tt.input, tt.expected, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// parseDependsOn
// ---------------------------------------------------------------------------

func TestParseDependsOn_Nil(t *testing.T) {
	deps := parseDependsOn(nil)
	if deps != nil {
		t.Errorf("expected nil, got %v", deps)
	}
}

func TestParseDependsOn_List(t *testing.T) {
	deps := parseDependsOn([]any{"db", "redis"})
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
	found := map[string]bool{}
	for _, d := range deps {
		found[d] = true
	}
	if !found["db"] || !found["redis"] {
		t.Errorf("expected [db, redis], got %v", deps)
	}
}

func TestParseDependsOn_Map(t *testing.T) {
	deps := parseDependsOn(map[string]any{
		"db":    map[string]any{"condition": "service_healthy"},
		"redis": map[string]any{"condition": "service_started"},
	})
	if len(deps) != 2 {
		t.Fatalf("expected 2 deps, got %d", len(deps))
	}
}

func TestParseDependsOn_UnsupportedType(t *testing.T) {
	deps := parseDependsOn("not-a-list-or-map")
	if deps != nil {
		t.Errorf("expected nil for unsupported type, got %v", deps)
	}
}

// ---------------------------------------------------------------------------
// Compose deployer with mock runtime
// ---------------------------------------------------------------------------

// mockRuntime implements core.ContainerRuntime for testing.
type mockRuntime struct {
	created []core.ContainerOpts
}

func (m *mockRuntime) Ping() error { return nil }
func (m *mockRuntime) CreateAndStart(_ context.Context, opts core.ContainerOpts) (string, error) {
	m.created = append(m.created, opts)
	return "container-" + opts.Name, nil
}
func (m *mockRuntime) Stop(_ context.Context, _ string, _ int) error           { return nil }
func (m *mockRuntime) Remove(_ context.Context, _ string, _ bool) error        { return nil }
func (m *mockRuntime) Restart(_ context.Context, _ string) error               { return nil }
func (m *mockRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (m *mockRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}

func TestStackDeployer_Deploy(t *testing.T) {
	rt := &mockRuntime{}
	logger := slog.Default()
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, err := Parse([]byte(`
services:
  web:
    image: nginx:alpine
    depends_on:
      - api
  api:
    image: node:22-alpine
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	err = deployer.Deploy(context.Background(), DeployOpts{
		AppID:     "app-123",
		TenantID:  "tenant-1",
		StackName: "mystack",
		Compose:   cf,
		EnvVars:   map[string]string{"GLOBAL": "val"},
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if len(rt.created) != 2 {
		t.Fatalf("expected 2 containers created, got %d", len(rt.created))
	}

	// Verify container naming
	names := map[string]bool{}
	for _, c := range rt.created {
		names[c.Name] = true
	}
	if !names["monster-mystack-web"] {
		t.Error("expected container 'monster-mystack-web'")
	}
	if !names["monster-mystack-api"] {
		t.Error("expected container 'monster-mystack-api'")
	}

	// Verify labels are set
	for _, c := range rt.created {
		if c.Labels["monster.enable"] != "true" {
			t.Errorf("container %s: missing monster.enable label", c.Name)
		}
		if c.Labels["monster.app.id"] != "app-123" {
			t.Errorf("container %s: wrong app.id label", c.Name)
		}
		if c.Labels["monster.stack"] != "mystack" {
			t.Errorf("container %s: wrong stack label", c.Name)
		}
	}
}

func TestStackDeployer_Deploy_NilRuntime(t *testing.T) {
	logger := slog.Default()
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(nil, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: alpine
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "test",
		Compose:   cf,
	})
	if err == nil {
		t.Fatal("expected error for nil runtime")
	}
	if !strings.Contains(err.Error(), "runtime not available") {
		t.Errorf("expected 'runtime not available' error, got: %v", err)
	}
}

func TestStackDeployer_Deploy_NoImage(t *testing.T) {
	rt := &mockRuntime{}
	logger := slog.Default()
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: placeholder
`))
	// Remove the image to simulate "no image"
	cf.Services["app"].Image = ""

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "test",
		Compose:   cf,
	})
	if err == nil {
		t.Fatal("expected error for no image")
	}
}

func TestStackDeployer_Deploy_BuildDirectiveError(t *testing.T) {
	rt := &mockRuntime{}
	logger := slog.Default()
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: placeholder
`))
	// Set build directive but no image
	cf.Services["app"].Image = ""
	cf.Services["app"].Build = &BuildConfig{Context: "."}

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "test",
		Compose:   cf,
	})
	if err == nil {
		t.Fatal("expected error for build directive without pre-built image")
	}
	if !strings.Contains(err.Error(), "build: directive") {
		t.Errorf("expected 'build: directive' in error, got: %v", err)
	}
}

func TestStackDeployer_Deploy_EnvVarsMerge(t *testing.T) {
	rt := &mockRuntime{}
	logger := slog.Default()
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp
    environment:
      SVC_VAR: from-service
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "test",
		Compose:   cf,
		EnvVars:   map[string]string{"GLOBAL_VAR": "from-opts"},
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if len(rt.created) != 1 {
		t.Fatalf("expected 1 container, got %d", len(rt.created))
	}

	envMap := map[string]bool{}
	for _, e := range rt.created[0].Env {
		envMap[e] = true
	}

	if !envMap["GLOBAL_VAR=from-opts"] {
		t.Error("expected GLOBAL_VAR=from-opts in env")
	}
	if !envMap["SVC_VAR=from-service"] {
		t.Error("expected SVC_VAR=from-service in env")
	}
}

func TestStackDeployer_Deploy_DefaultRestartPolicy(t *testing.T) {
	rt := &mockRuntime{}
	logger := slog.Default()
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "test",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if rt.created[0].RestartPolicy != "unless-stopped" {
		t.Errorf("expected default restart policy 'unless-stopped', got %q", rt.created[0].RestartPolicy)
	}
}

func TestStackDeployer_Deploy_CustomRestartPolicy(t *testing.T) {
	rt := &mockRuntime{}
	logger := slog.Default()
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp
    restart: always
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "test",
		Compose:   cf,
	})
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}

	if rt.created[0].RestartPolicy != "always" {
		t.Errorf("expected restart policy 'always', got %q", rt.created[0].RestartPolicy)
	}
}

// ---------------------------------------------------------------------------
// Mock runtime that fails on CreateAndStart
// ---------------------------------------------------------------------------

type failRuntime struct {
	mockRuntime
}

func (m *failRuntime) CreateAndStart(_ context.Context, _ core.ContainerOpts) (string, error) {
	return "", fmt.Errorf("docker daemon unavailable")
}

func TestStackDeployer_Deploy_RuntimeError(t *testing.T) {
	rt := &failRuntime{}
	logger := slog.Default()
	events := core.NewEventBus(logger)
	deployer := NewStackDeployer(rt, nil, events, logger)

	cf, _ := Parse([]byte(`
services:
  app:
    image: myapp
`))

	err := deployer.Deploy(context.Background(), DeployOpts{
		StackName: "test",
		Compose:   cf,
	})
	if err == nil {
		t.Fatal("expected error from failing runtime")
	}
	if !strings.Contains(err.Error(), "docker daemon") {
		t.Errorf("expected docker daemon error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func indexMap(order []string) map[string]int {
	m := make(map[string]int, len(order))
	for i, name := range order {
		m[name] = i
	}
	return m
}
