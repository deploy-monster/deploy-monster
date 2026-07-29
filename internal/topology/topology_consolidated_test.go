package topology

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// === merged from deployer_boost_test.go ===

// fakeDockerInPath creates a no-op docker executable in a temp directory
// and prepends that directory to PATH so verifyCompose/pullImages/composeUp
// can succeed without a real Docker installation.
func fakeDockerInPath(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()

	var dockerPath string
	if runtime.GOOS == "windows" {
		dockerPath = filepath.Join(tmpDir, "docker.bat")
		if err := os.WriteFile(dockerPath, []byte("@echo off\r\nexit /b 0\r\n"), 0755); err != nil {
			t.Fatalf("write fake docker.bat: %v", err)
		}
	} else {
		dockerPath = filepath.Join(tmpDir, "docker")
		if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write fake docker: %v", err)
		}
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(filepath.ListSeparator)+oldPath)
	return func() { os.Setenv("PATH", oldPath) }
}

func TestDeployer_verifyCompose(t *testing.T) {
	cleanup := fakeDockerInPath(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	// Write a minimal compose file so the path exists
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	if err := d.verifyCompose(context.Background()); err != nil {
		t.Fatalf("verifyCompose: %v", err)
	}
}

func TestDeployer_pullImages(t *testing.T) {
	cleanup := fakeDockerInPath(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	if err := d.pullImages(context.Background()); err != nil {
		t.Fatalf("pullImages: %v", err)
	}
}

func TestDeployer_composeUp(t *testing.T) {
	cleanup := fakeDockerInPath(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	out, err := d.composeUp(context.Background())
	if err != nil {
		t.Fatalf("composeUp: %v (output: %s)", err, out)
	}
}

func TestDeployer_verifyCompose_Error(t *testing.T) {
	// Remove docker from PATH so it cannot be found
	t.Setenv("PATH", t.TempDir())
	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	err := d.verifyCompose(context.Background())
	if err == nil {
		t.Error("expected error when docker is not available")
	}
}

func TestDeployer_pullImages_Error(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	err := d.pullImages(context.Background())
	if err == nil {
		t.Error("expected error when docker is not available")
	}
}

func TestDeployer_composeUp_Error(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	d := NewDeployer(t.TempDir())
	compose := minimalCompose()
	_ = os.WriteFile(d.composePath, []byte(compose.ToYAML()), 0640)

	_, err := d.composeUp(context.Background())
	if err == nil {
		t.Error("expected error when docker is not available")
	}
}

func fakeDockerConfigFail(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()

	var dockerPath string
	if runtime.GOOS == "windows" {
		dockerPath = filepath.Join(tmpDir, "docker.bat")
		if err := os.WriteFile(dockerPath, []byte("@echo off\nif \"%4\"==\"config\" exit /b 1\nexit /b 0\n"), 0755); err != nil {
			t.Fatalf("write fake docker.bat: %v", err)
		}
	} else {
		dockerPath = filepath.Join(tmpDir, "docker")
		if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nif [ \"$4\" = \"config\" ]; then exit 1; fi\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write fake docker: %v", err)
		}
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(filepath.ListSeparator)+oldPath)
	return func() { os.Setenv("PATH", oldPath) }
}

func fakeDockerConfigOkUpFail(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()

	var dockerPath string
	if runtime.GOOS == "windows" {
		dockerPath = filepath.Join(tmpDir, "docker.bat")
		if err := os.WriteFile(dockerPath, []byte("@echo off\nif \"%4\"==\"up\" exit /b 1\nexit /b 0\n"), 0755); err != nil {
			t.Fatalf("write fake docker.bat: %v", err)
		}
	} else {
		dockerPath = filepath.Join(tmpDir, "docker")
		if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nif [ \"$4\" = \"up\" ]; then exit 1; fi\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write fake docker: %v", err)
		}
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(filepath.ListSeparator)+oldPath)
	return func() { os.Setenv("PATH", oldPath) }
}

// fakeDockerPullFail creates a docker that fails on "pull" but passes on
// "config" and "up" — exercises the non-fatal pullImages warning path.
func fakeDockerPullFail(t *testing.T) func() {
	t.Helper()
	tmpDir := t.TempDir()

	var dockerPath string
	if runtime.GOOS == "windows" {
		dockerPath = filepath.Join(tmpDir, "docker.bat")
		if err := os.WriteFile(dockerPath, []byte("@echo off\nif \"%4\"==\"pull\" exit /b 1\nexit /b 0\n"), 0755); err != nil {
			t.Fatalf("write fake docker.bat: %v", err)
		}
	} else {
		dockerPath = filepath.Join(tmpDir, "docker")
		if err := os.WriteFile(dockerPath, []byte("#!/bin/sh\nif [ \"$4\" = \"pull\" ]; then exit 1; fi\nexit 0\n"), 0755); err != nil {
			t.Fatalf("write fake docker: %v", err)
		}
	}

	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", tmpDir+string(filepath.ListSeparator)+oldPath)
	return func() { os.Setenv("PATH", oldPath) }
}

func TestDeployer_Deploy_Success(t *testing.T) {
	cleanup := fakeDockerInPath(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	result, err := d.Deploy(context.Background(), minimalCompose(), "", "", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true")
	}
	if len(result.Containers) != 1 {
		t.Errorf("expected 1 container, got %d", len(result.Containers))
	}
}

func TestDeployer_Deploy_VerifyComposeError(t *testing.T) {
	cleanup := fakeDockerConfigFail(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	result, err := d.Deploy(context.Background(), minimalCompose(), "", "", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.Success {
		t.Error("expected Success=false")
	}
}

func TestDeployer_Deploy_ComposeUpError(t *testing.T) {
	cleanup := fakeDockerConfigOkUpFail(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	result, err := d.Deploy(context.Background(), minimalCompose(), "", "", false)
	if err == nil {
		t.Fatal("expected error")
	}
	if result.Success {
		t.Error("expected Success=false")
	}
}

// === merged from topology_extra_test.go ===

// =============================================================================
// Compile — error from generateCompose (unsupported DB engine)
// =============================================================================

func TestCompile_UnsupportedDatabaseEngine(t *testing.T) {
	top := helperTopology()
	// Override databases with an unsupported engine so generateDatabaseService fails
	top.Databases = []Database{
		{
			ID:      "db-bad",
			Name:    "customdb",
			Engine:  "unsupported",
			Version: "1.0",
		},
	}
	// Clear connections that reference db-1
	top.Connections = nil
	c := NewCompiler(top, "proj", "dev")
	_, err := c.Compile()
	if err == nil {
		t.Fatal("expected error from unsupported database engine")
	}
	if !strings.Contains(err.Error(), "compose generation failed") {
		t.Errorf("expected compose generation failure, got: %v", err)
	}
}

// =============================================================================
// resolveReferences — db == nil continue (connection target not a database)
// =============================================================================

func TestResolveReferences_ConnectionTargetNotADatabase(t *testing.T) {
	top := helperTopology()
	// Add a connection with ConnDependency type targeting an app (not a DB)
	top.Connections = append(top.Connections, Connection{
		ID:       "conn-app-target",
		Type:     ConnDependency,
		SourceID: "wk-1",
		TargetID: "app-1", // app-1 is not a database
		Config: ConnConfig{
			EnvVarName: "APP_URL",
		},
	})
	c := NewCompiler(top, "proj", "dev")
	_, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile should succeed: %v", err)
	}
}

// =============================================================================
// componentExists — domain and worker matches
// =============================================================================

func TestComponentExists_DomainAndWorker(t *testing.T) {
	top := helperTopology()
	// Add connections that reference domain and worker IDs
	top.Connections = append(top.Connections,
		Connection{
			ID:       "conn-dom",
			Type:     ConnNetwork,
			SourceID: "dom-1",
			TargetID: "app-1",
		},
		Connection{
			ID:       "conn-wk",
			Type:     ConnNetwork,
			SourceID: "wk-1",
			TargetID: "app-1",
		},
	)
	c := NewCompiler(top, "proj", "dev")
	_, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile should succeed: %v", err)
	}
}

// =============================================================================
// generateAppService — HealthCheckPort=0, nil volume, env var references
// =============================================================================

func TestGenerateAppService_HealthCheckPortZero(t *testing.T) {
	top := helperTopology()
	top.Apps[0].HealthCheckPort = 0 // should default to app.Port (3000)
	top.Apps[0].HealthCheckPath = "/healthz"
	c := NewCompiler(top, "proj", "dev")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	svc := compose.Services["web"]
	if svc.HealthCheck == nil {
		t.Fatal("expected health check")
	}
	// Health check URL should include port 3000 since HealthCheckPort=0 defaults to app.Port
	if !strings.Contains(svc.HealthCheck.Test[3], "localhost:3000") {
		t.Errorf("expected health check URL with port 3000, got: %v", svc.HealthCheck.Test)
	}
}

func TestGenerateAppService_EnvVarReferenceResolution(t *testing.T) {
	top := helperTopology()
	// Add an env var that uses ${REF} syntax that resolves to a connection
	top.Apps[0].EnvVars["DB_REF"] = "${DATABASE_URL}"
	top.Connections[0].Config.EnvVarName = "DATABASE_URL"
	c := NewCompiler(top, "proj", "dev")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	svc := compose.Services["web"]
	// DB_REF should be resolved to the DATABASE_URL connection value
	if svc.Environment["DB_REF"] == "" {
		t.Error("expected DB_REF to be resolved from connection")
	}
}

func TestGenerateAppService_EnvVarReferenceUnresolved(t *testing.T) {
	top := helperTopology()
	// Add an env var with ${REF} that does NOT resolve to any connection
	top.Apps[0].EnvVars["UNRESOLVED"] = "${MISSING_REF}"
	top.Connections = nil // no connections available
	c := NewCompiler(top, "proj", "dev")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	svc := compose.Services["web"]
	// Unresolved reference should be kept as-is
	if svc.Environment["UNRESOLVED"] != "${MISSING_REF}" {
		t.Errorf("expected UNRESOLVED to keep original ref, got: %q", svc.Environment["UNRESOLVED"])
	}
}

func TestGenerateAppService_ConnectionEnvVarNotInConnections(t *testing.T) {
	top := helperTopology()
	// Create a ConnDependency from app-1 to a non-DB target (domain dom-1).
	// resolveReferences iterates databases looking for dom-1 → not found → continue.
	// So the env var never enters c.connections and generateAppService will
	// check c.connections for it and find nothing.
	top.Connections = []Connection{
		{
			ID:       "conn-dom-ref",
			Type:     ConnDependency,
			SourceID: "app-1",
			TargetID: "dom-1", // dom-1 is a domain, not a database
			Config: ConnConfig{
				EnvVarName: "DOMAIN_URL",
			},
		},
	}
	// dom-1 exists in the topology and componentExists("dom-1") returns true
	c := NewCompiler(top, "proj", "dev")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	svc := compose.Services["web"]
	// DOMAIN_URL was not set in c.connections (db == nil, continued)
	if _, ok := svc.Environment["DOMAIN_URL"]; ok {
		t.Error("expected DOMAIN_URL not to be set (target is domain, not database)")
	}
}

// =============================================================================
// generateWorkerService — with env var references and dependencies
// =============================================================================

func TestGenerateWorkerService_WithDependencies(t *testing.T) {
	top := helperTopology()
	// Add a database dependency for the worker
	top.Workers = append(top.Workers, Worker{
		ID:        "wk-2",
		Name:      "background",
		Status:    StatusPending,
		GitURL:    "https://github.com/example/worker2.git",
		Branch:    "main",
		BuildPack: "go",
		Command:   "./background",
		Replicas:  1,
		EnvVars:   map[string]string{"LOG_LEVEL": "debug", "DB_REF": "${DATABASE_URL}", "MISSING_REF": "${NONEXISTENT_KEY}"},
		VolumeMounts: []VolumeMount{
			{VolumeID: "vol-1", MountPath: "/worker-data", ReadOnly: true},
		},
	})
	// Add a connection from the new worker to the db
	top.Connections = append(top.Connections, Connection{
		ID:       "conn-wk2-db",
		Type:     ConnDependency,
		SourceID: "wk-2",
		TargetID: "db-1",
		Config: ConnConfig{
			EnvVarName: "DATABASE_URL",
		},
	})
	c := NewCompiler(top, "proj", "dev")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	svc, ok := compose.Services["background"]
	if !ok {
		t.Fatal("expected background service")
	}
	if svc.Command != "./background" {
		t.Errorf("expected command './background', got: %q", svc.Command)
	}
	// Should have depends_on for postgres
	if len(svc.DependsOn) == 0 {
		t.Error("expected depends_on entries for worker with dependency")
	}
	// DB_REF env var reference should be resolved to a connection value
	if svc.Environment["DB_REF"] == "" || svc.Environment["DB_REF"] == "${DATABASE_URL}" {
		t.Errorf("expected DB_REF to be resolved, got: %q", svc.Environment["DB_REF"])
	}
	// MISSING_REF is a ${...} reference not in connections → kept as-is
	if svc.Environment["MISSING_REF"] != "${NONEXISTENT_KEY}" {
		t.Errorf("expected MISSING_REF to remain unresolved, got: %q", svc.Environment["MISSING_REF"])
	}
	// Volume mount should be present
	found := false
	for _, v := range svc.Volumes {
		if v == "data-vol:/worker-data:ro" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected volume mount data-vol:/worker-data:ro, got: %v", svc.Volumes)
	}
}

// =============================================================================
// GenerateCaddyfile — app not found and port zero cases
// =============================================================================

func TestGenerateCaddyfile_AppNotFound(t *testing.T) {
	top := helperTopology()
	// Domain targets an app ID that doesn't exist
	top.Domains = append(top.Domains, Domain{
		ID:          "dom-missing",
		Name:        "missing",
		FQDN:        "missing.example.com",
		SSLEnabled:  false,
		TargetAppID: "app-nonexistent",
	})
	c := NewCompiler(top, "proj", "dev")
	cf := c.GenerateCaddyfile()
	// Should not contain the missing domain
	if strings.Contains(cf, "missing.example.com") {
		t.Error("expected missing domain to be skipped")
	}
}

func TestGenerateCaddyfile_PortZeroDefaultsTo3000(t *testing.T) {
	top := helperTopology()
	top.Apps[0].Port = 0 // port is 0, should default to 3000 in Caddyfile
	c := NewCompiler(top, "proj", "dev")
	cf := c.GenerateCaddyfile()
	if !strings.Contains(cf, "reverse_proxy web:3000") {
		t.Errorf("expected reverse_proxy web:3000 when port is 0, got:\n%s", cf)
	}
}

func TestGenerateCaddyfile_TLSNotEnabled(t *testing.T) {
	top := helperTopology()
	top.Domains[0].SSLEnabled = false
	top.Domains[0].SSLMODE = SSLNone
	c := NewCompiler(top, "proj", "dev")
	cf := c.GenerateCaddyfile()
	// TLS must not be present when SSL is disabled
	if strings.Contains(cf, "tls") {
		t.Error("expected no tls directive when SSL is disabled")
	}
}

// =============================================================================
// ToYAML — edge cases: empty version, networks driver, volumes external
// =============================================================================

func TestToYAML_EmptyVersion(t *testing.T) {
	config := &ComposeConfig{
		Services: map[string]Service{
			"app": {Image: "test:latest"},
		},
	}
	yaml := config.ToYAML()
	if strings.Contains(yaml, "version:") {
		t.Error("expected no version line when version is empty")
	}
	if !strings.Contains(yaml, "services:") {
		t.Error("expected services section")
	}
}

func TestToYAML_NetworkWithDriver(t *testing.T) {
	config := &ComposeConfig{
		Version: "3.9",
		Services: map[string]Service{
			"app": {Image: "test:latest"},
		},
		Networks: map[string]Network{
			"custom": {Driver: "overlay"},
		},
	}
	yaml := config.ToYAML()
	if !strings.Contains(yaml, "driver: overlay") {
		t.Errorf("expected driver overlay in YAML:\n%s", yaml)
	}
}

func TestToYAML_VolumeWithExternal(t *testing.T) {
	config := &ComposeConfig{
		Version: "3.9",
		Services: map[string]Service{
			"app": {Image: "test:latest"},
		},
		Volumes: map[string]VolumeSpec{
			"external-vol": {External: true},
		},
	}
	yaml := config.ToYAML()
	if !strings.Contains(yaml, "external: true") {
		t.Errorf("expected external: true in YAML:\n%s", yaml)
	}
}

// =============================================================================
// toYAML (Service.toYAML) — Build Args, Port protocol, Deploy, Labels, HC, Cmd
// =============================================================================

func TestServiceToYAML_BuildArgs(t *testing.T) {
	svc := &Service{
		Image: "myapp:latest",
		Build: &BuildConfig{
			Context:    ".",
			Dockerfile: "Dockerfile.custom",
			Args:       map[string]string{"BUILD_ARG": "value1", "ANOTHER": "value2"},
		},
	}
	yaml := svc.toYAML(4)
	if !strings.Contains(yaml, "args:") {
		t.Errorf("expected build args in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, "BUILD_ARG:") {
		t.Errorf("expected BUILD_ARG in YAML:\n%s", yaml)
	}
}

func TestServiceToYAML_PortWithProtocol(t *testing.T) {
	svc := &Service{
		Image: "myapp:latest",
		Ports: []PortMapping{
			{Host: 53, Container: 53, Protocol: "udp"},
			{Host: 80, Container: 80}, // no protocol
		},
	}
	yaml := svc.toYAML(4)
	if !strings.Contains(yaml, "53:53/udp") {
		t.Errorf("expected port with protocol in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, "80:80") {
		t.Errorf("expected port without protocol in YAML:\n%s", yaml)
	}
}

func TestServiceToYAML_DeployWithReplicasAndResources(t *testing.T) {
	svc := &Service{
		Image: "myapp:latest",
		Deploy: &DeployConfig{
			Replicas: 3,
			Resources: &Resources{
				Limits: &ResourceLimit{
					CPUs:   "500m",
					Memory: "512M",
				},
			},
		},
	}
	yaml := svc.toYAML(4)
	if !strings.Contains(yaml, "replicas: 3") {
		t.Errorf("expected replicas in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, "cpus: 500m") {
		t.Errorf("expected cpus in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, "memory: 512M") {
		t.Errorf("expected memory in YAML:\n%s", yaml)
	}
}

func TestServiceToYAML_WithLabels(t *testing.T) {
	svc := &Service{
		Image: "myapp:latest",
		Labels: map[string]string{
			"monster.type":    "app",
			"monster.project": "test",
		},
	}
	yaml := svc.toYAML(4)
	if !strings.Contains(yaml, "labels:") {
		t.Errorf("expected labels section in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, "monster.type:") {
		t.Errorf("expected monster.type label in YAML:\n%s", yaml)
	}
}

func TestServiceToYAML_FullHealthCheck(t *testing.T) {
	svc := &Service{
		Image: "myapp:latest",
		HealthCheck: &HealthCheck{
			Test:     []string{"CMD", "curl", "-f", "http://localhost/health"},
			Interval: "30s",
			Timeout:  "10s",
			Retries:  3,
			Disable:  false,
		},
	}
	yaml := svc.toYAML(4)
	if !strings.Contains(yaml, "healthcheck:") {
		t.Errorf("expected healthcheck in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, "interval: 30s") {
		t.Errorf("expected interval in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, "retries: 3") {
		t.Errorf("expected retries in YAML:\n%s", yaml)
	}
}

func TestServiceToYAML_Command(t *testing.T) {
	svc := &Service{
		Image:   "myapp:latest",
		Command: "./start.sh --verbose",
	}
	yaml := svc.toYAML(4)
	if !strings.Contains(yaml, "command:") {
		t.Errorf("expected command in YAML:\n%s", yaml)
	}
	if !strings.Contains(yaml, "./start.sh") {
		t.Errorf("expected command content in YAML:\n%s", yaml)
	}
}

// =============================================================================
// resolveReferences — default env var name when EnvVarName is empty
// =============================================================================

func TestResolveReferences_DefaultEnvVarName(t *testing.T) {
	top := helperTopology()
	// Connection with empty EnvVarName — should default to DB_NAME_URL
	top.Connections[0].Config.EnvVarName = ""
	// Add another connection to exercise the default path
	top.Connections = append(top.Connections, Connection{
		ID:       "conn-db2",
		Type:     ConnDependency,
		SourceID: "app-1",
		TargetID: "db-1",
	})
	c := NewCompiler(top, "proj", "dev")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	// The default env var name should be POSTGRES_URL
	if _, ok := compose.Services["web"]; !ok {
		t.Fatal("expected web service")
	}
}

// =============================================================================
// componentExists — volume match (connection targeting a volume ID)
// =============================================================================

func TestComponentExists_VolumeMatch(t *testing.T) {
	top := helperTopology()
	// Add a connection that references a volume as a component
	top.Connections = append(top.Connections, Connection{
		ID:       "conn-volume",
		Type:     ConnNetwork,
		SourceID: "vol-1", // volume ID
		TargetID: "app-1",
	})
	c := NewCompiler(top, "proj", "dev")
	_, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile should succeed: %v", err)
	}
}

// =============================================================================
// GenerateCaddyfile — domain with no TargetAppID (skip case)
// =============================================================================

func TestGenerateCaddyfile_TargetAppIDEmpty(t *testing.T) {
	top := helperTopology()
	// Domain with empty TargetAppID — should be skipped entirely
	top.Domains = append(top.Domains, Domain{
		ID:          "dom-no-target",
		Name:        "no-target",
		FQDN:        "notarget.example.com",
		SSLEnabled:  false,
		TargetAppID: "",
	})
	c := NewCompiler(top, "proj", "dev")
	cf := c.GenerateCaddyfile()
	if strings.Contains(cf, "notarget.example.com") {
		t.Error("expected domain with no TargetAppID to be skipped")
	}
}

// =============================================================================
// toYAML — HealthCheck.Disable branch
// =============================================================================

func TestServiceToYAML_HealthCheckDisabled(t *testing.T) {
	svc := &Service{
		Image: "myapp:latest",
		HealthCheck: &HealthCheck{
			Disable: true,
		},
	}
	yaml := svc.toYAML(4)
	if !strings.Contains(yaml, "disable: true") {
		t.Errorf("expected disable: true in YAML:\n%s", yaml)
	}
}

// =============================================================================
// Deploy — pull images failure path (non-fatal, logs warning and continues)
// =============================================================================

func TestDeployer_Deploy_PullImagesFailureContinues(t *testing.T) {
	// Create a fake docker that fails on "pull" but passes on "config" and "up"
	cleanup := fakeDockerPullFail(t)
	defer cleanup()

	d := NewDeployer(t.TempDir())
	compose := minimalCompose()

	result, err := d.Deploy(context.Background(), compose, "", "", false)
	if err != nil {
		t.Fatalf("Deploy should succeed even when pull fails: %v", err)
	}
	if !result.Success {
		t.Error("expected Success=true even when pull images fails")
	}
}
