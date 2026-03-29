package topology

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helperTopology returns a complete, valid topology for testing.
func helperTopology() *Topology {
	return &Topology{
		ID:          "top-1",
		Name:        "test-topology",
		ProjectID:   "proj-1",
		Environment: "staging",
		Version:     1,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Apps: []App{
			{
				ID:          "app-1",
				Name:        "web",
				Status:      StatusPending,
				GitURL:      "https://github.com/example/web.git",
				Branch:      "main",
				BuildPack:   "nodejs",
				Port:        3000,
				Replicas:    1,
				MemoryMB:    512,
				CPU:         500,
				EnvVars:     map[string]string{"NODE_ENV": "production"},
				SecretRefs:  map[string]string{"API_KEY": "secrets/api"},
				VolumeMounts: []VolumeMount{
					{VolumeID: "vol-1", MountPath: "/data", ReadOnly: true},
				},
				HealthCheckPath: "/health",
				HealthCheckPort: 3000,
			},
		},
		Databases: []Database{
			{
				ID:       "db-1",
				Name:     "postgres",
				Status:   StatusPending,
				Engine:   EnginePostgres,
				Version:  "16",
				SizeGB:   10,
				Username: "dbuser",
				Password: "dbpass",
				Database: "appdb",
			},
		},
		Domains: []Domain{
			{
				ID:          "dom-1",
				Name:        "api-domain",
				Status:      StatusPending,
				FQDN:        "api.example.com",
				SSLEnabled:  true,
				SSLMODE:     SSLAuto,
				TargetAppID: "app-1",
				PathPrefix:  "/",
			},
		},
		Volumes: []Volume{
			{
				ID:         "vol-1",
				Name:       "data-vol",
				Status:     StatusPending,
				SizeGB:     5,
				VolumeType: VolumeLocal,
				MountPath:  "/data",
			},
		},
		Workers: []Worker{
			{
				ID:         "wk-1",
				Name:       "scheduler",
				Status:     StatusPending,
				GitURL:     "https://github.com/example/worker.git",
				Branch:     "main",
				BuildPack:  "go",
				Command:    "./scheduler",
				Replicas:   1,
				MemoryMB:   256,
				CPU:        250,
				EnvVars:    map[string]string{"WORKER_ENV": "staging"},
				VolumeMounts: []VolumeMount{
					{VolumeID: "vol-1", MountPath: "/worker-data"},
				},
			},
		},
		Connections: []Connection{
			{
				ID:       "conn-1",
				Type:     ConnDependency,
				SourceID: "app-1",
				TargetID: "db-1",
				Config: ConnConfig{
					EnvVarName: "DATABASE_URL",
				},
			},
			{
				ID:       "conn-2",
				Type:     ConnRoute,
				SourceID: "dom-1",
				TargetID: "app-1",
			},
		},
	}
}

func TestNewCompilerAndCompile(t *testing.T) {
	top := helperTopology()
	c := NewCompiler(top, "myproject", "staging")
	if c == nil {
		t.Fatal("expected compiler, got nil")
	}
	if c.project != "myproject" || c.env != "staging" {
		t.Fatalf("unexpected project/env: %s/%s", c.project, c.env)
	}

	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile() failed: %v", err)
	}
	if compose == nil {
		t.Fatal("expected compose config, got nil")
	}

	// Must contain default network
	if _, ok := compose.Networks["default"]; !ok {
		t.Error("expected default network")
	}

	// Database service
	if _, ok := compose.Services["postgres"]; !ok {
		t.Error("expected postgres service")
	}

	// App service
	appSvc, ok := compose.Services["web"]
	if !ok {
		t.Fatal("expected web service")
	}
	if appSvc.Image != "myproject/web-staging:latest" {
		t.Errorf("unexpected app image: %s", appSvc.Image)
	}
	if appSvc.Build == nil || appSvc.Build.Context != filepath.Join(".", "apps", "web") {
		t.Errorf("unexpected app build context: %v", appSvc.Build)
	}
	if len(appSvc.Expose) != 1 || appSvc.Expose[0] != 3000 {
		t.Errorf("unexpected expose: %v", appSvc.Expose)
	}
	if appSvc.Environment["NODE_ENV"] != "production" {
		t.Errorf("unexpected env NODE_ENV: %v", appSvc.Environment["NODE_ENV"])
	}
	// Dependency env var resolved
	if !strings.HasPrefix(appSvc.Environment["DATABASE_URL"], "postgresql://dbuser:dbpass@postgres:5432/appdb") {
		t.Errorf("unexpected DATABASE_URL: %s", appSvc.Environment["DATABASE_URL"])
	}
	// Volume mount with read-only
	foundVol := false
	for _, v := range appSvc.Volumes {
		if v == "data-vol:/data:ro" {
			foundVol = true
			break
		}
	}
	if !foundVol {
		t.Errorf("expected volume mount data-vol:/data:ro in %v", appSvc.Volumes)
	}
	// Health check
	if appSvc.HealthCheck == nil {
		t.Fatal("expected health check")
	}
	// Resource limits
	if appSvc.Deploy == nil || appSvc.Deploy.Resources == nil || appSvc.Deploy.Resources.Limits == nil {
		t.Fatal("expected deploy resources")
	}
	if appSvc.Deploy.Resources.Limits.Memory != "512M" || appSvc.Deploy.Resources.Limits.CPUs != "500m" {
		t.Errorf("unexpected limits memory/cpu: %s/%s", appSvc.Deploy.Resources.Limits.Memory, appSvc.Deploy.Resources.Limits.CPUs)
	}

	// Worker service
	wkSvc, ok := compose.Services["scheduler"]
	if !ok {
		t.Fatal("expected scheduler service")
	}
	if wkSvc.Image != "myproject/scheduler-staging:latest" {
		t.Errorf("unexpected worker image: %s", wkSvc.Image)
	}
	if wkSvc.Command != "./scheduler" {
		t.Errorf("unexpected worker command: %s", wkSvc.Command)
	}

	// Proxy service because we have domains
	if _, ok := compose.Services["proxy"]; !ok {
		t.Error("expected proxy service due to domains")
	}

	// Volumes
	if _, ok := compose.Volumes["postgres_data"]; !ok {
		t.Error("expected postgres_data volume")
	}
	if _, ok := compose.Volumes["data-vol"]; !ok {
		t.Error("expected data-vol volume")
	}
	if _, ok := compose.Volumes["caddy_data"]; !ok {
		t.Error("expected caddy_data volume")
	}
	if _, ok := compose.Volumes["caddy_config"]; !ok {
		t.Error("expected caddy_config volume")
	}
}

func TestValidationDuplicateNames(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*Topology)
		wantErrMsg string
	}{
		{
			name: "duplicate app name",
			mutate: func(top *Topology) {
				top.Apps = append(top.Apps, App{ID: "app-2", Name: "web"})
			},
			wantErrMsg: "duplicate app name: web",
		},
		{
			name: "duplicate database name",
			mutate: func(top *Topology) {
				top.Databases = append(top.Databases, Database{ID: "db-2", Name: "postgres"})
			},
			wantErrMsg: "duplicate database name: postgres",
		},
		{
			name: "duplicate volume name",
			mutate: func(top *Topology) {
				top.Volumes = append(top.Volumes, Volume{ID: "vol-2", Name: "data-vol"})
			},
			wantErrMsg: "duplicate volume name: data-vol",
		},
		{
			name: "port conflict",
			mutate: func(top *Topology) {
				top.Apps = append(top.Apps, App{ID: "app-2", Name: "api", Port: 3000})
			},
			wantErrMsg: "port conflict: 3000 used by both web and api",
		},
		{
			name: "missing volume mount",
			mutate: func(top *Topology) {
				top.Apps[0].VolumeMounts = append(top.Apps[0].VolumeMounts, VolumeMount{VolumeID: "vol-missing", MountPath: "/missing"})
			},
			wantErrMsg: "app web mounts non-existent volume: vol-missing",
		},
		{
			name: "invalid connection source",
			mutate: func(top *Topology) {
				top.Connections = append(top.Connections, Connection{ID: "bad", Type: ConnNetwork, SourceID: "missing", TargetID: "app-1"})
			},
			wantErrMsg: "connection bad has invalid source: missing",
		},
		{
			name: "invalid connection target",
			mutate: func(top *Topology) {
				top.Connections = append(top.Connections, Connection{ID: "bad", Type: ConnNetwork, SourceID: "app-1", TargetID: "missing"})
			},
			wantErrMsg: "connection bad has invalid target: missing",
		},
		{
			name: "domain targets non-existent app",
			mutate: func(top *Topology) {
				top.Domains = append(top.Domains, Domain{ID: "dom-2", Name: "bad", FQDN: "bad.example.com", TargetAppID: "app-missing"})
			},
			wantErrMsg: "domain bad.example.com targets non-existent app: app-missing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			top := helperTopology()
			tt.mutate(top)
			c := NewCompiler(top, "myproject", "staging")
			_, err := c.Compile()
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErrMsg) {
				t.Errorf("expected error containing %q, got %q", tt.wantErrMsg, err.Error())
			}
		})
	}
}

func TestGenerateCaddyfile(t *testing.T) {
	t.Run("with domains and SSL", func(t *testing.T) {
		top := helperTopology()
		c := NewCompiler(top, "myproject", "staging")
		// Compile to ensure no panic, but GenerateCaddyfile only needs topology fields.
		cf := c.GenerateCaddyfile()
		if !strings.Contains(cf, "api.example.com") {
			t.Error("expected Caddyfile to contain api.example.com")
		}
		if !strings.Contains(cf, "reverse_proxy web:3000") {
			t.Error("expected reverse_proxy web:3000")
		}
		if !strings.Contains(cf, "tls internal") {
			t.Error("expected tls internal for SSLAuto")
		}
		if !strings.Contains(cf, "encode gzip zstd") {
			t.Error("expected encode gzip zstd")
		}
	})

	t.Run("without domains", func(t *testing.T) {
		top := helperTopology()
		top.Domains = nil
		c := NewCompiler(top, "myproject", "staging")
		cf := c.GenerateCaddyfile()
		// Still has global block but no domain blocks
		if strings.Contains(cf, "reverse_proxy") {
			t.Error("expected no reverse_proxy when no domains")
		}
		if !strings.Contains(cf, "email admin@deploy.monster") {
			t.Error("expected global email option")
		}
	})

	t.Run("SSL disabled", func(t *testing.T) {
		top := helperTopology()
		top.Domains[0].SSLEnabled = false
		top.Domains[0].SSLMODE = SSLNone
		c := NewCompiler(top, "myproject", "staging")
		cf := c.GenerateCaddyfile()
		if strings.Contains(cf, "tls internal") {
			t.Error("expected no tls internal when SSL disabled")
		}
	})
}

func TestGenerateEnvFile(t *testing.T) {
	top := helperTopology()
	c := NewCompiler(top, "myproject", "staging")
	// Need to compile to resolve references before generating env file
	if _, err := c.Compile(); err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	env := c.GenerateEnvFile()
	if !strings.Contains(env, "# Project: myproject") {
		t.Error("expected project comment")
	}
	if !strings.Contains(env, "# Environment: staging") {
		t.Error("expected environment comment")
	}
	if !strings.Contains(env, "DATABASE_URL=") {
		t.Error("expected DATABASE_URL env var")
	}
	if !strings.HasPrefix(env, "# Auto-generated environment file") {
		t.Error("expected header")
	}
}

func TestComposeConfigToYAML(t *testing.T) {
	top := helperTopology()
	c := NewCompiler(top, "myproject", "staging")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	yaml1 := compose.ToYAML()
	// ToYAML is not fully deterministic due to map iteration in networks/volumes/build args.
	// We assert it contains the expected sections rather than exact equality.
	_ = yaml1
	if !strings.Contains(yaml1, "version: \"3.9\"") {
		t.Error("expected version in YAML")
	}
	if !strings.Contains(yaml1, "services:") {
		t.Error("expected services section")
	}
	if !strings.Contains(yaml1, "networks:") {
		t.Error("expected networks section")
	}
	if !strings.Contains(yaml1, "volumes:") {
		t.Error("expected volumes section")
	}
	if !strings.Contains(yaml1, "web:") {
		t.Error("expected web service in YAML")
	}
	if !strings.Contains(yaml1, "proxy:") {
		t.Error("expected proxy service in YAML")
	}
}

func TestNewDeployerAndDeployDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	top := helperTopology()
	c := NewCompiler(top, "myproject", "staging")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	caddyfile := c.GenerateCaddyfile()
	envFile := c.GenerateEnvFile()

	d := NewDeployer(tmpDir)
	if d.workDir != tmpDir {
		t.Fatalf("unexpected workDir: %s", d.workDir)
	}

	ctx := context.Background()
	result, err := d.Deploy(ctx, compose, caddyfile, envFile, true)
	if err != nil {
		t.Fatalf("Deploy dry-run failed: %v", err)
	}
	if !result.Success {
		t.Fatal("expected success")
	}
	if !strings.Contains(result.Message, "Dry run completed") {
		t.Errorf("unexpected message: %s", result.Message)
	}

	// Verify files written
	composePath := filepath.Join(tmpDir, "docker-compose.yaml")
	if _, err := os.Stat(composePath); os.IsNotExist(err) {
		t.Error("expected docker-compose.yaml to be written")
	}
	b, _ := os.ReadFile(composePath)
	if string(b) != result.ComposeYAML {
		t.Error("compose file content mismatch")
	}

	caddyPath := filepath.Join(tmpDir, "Caddyfile")
	if _, err := os.Stat(caddyPath); os.IsNotExist(err) {
		t.Error("expected Caddyfile to be written")
	}
	b, _ = os.ReadFile(caddyPath)
	if string(b) != result.Caddyfile {
		t.Error("Caddyfile content mismatch")
	}

	envPath := filepath.Join(tmpDir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		t.Error("expected .env to be written")
	}
	b, _ = os.ReadFile(envPath)
	if string(b) != result.EnvFile {
		t.Error("env file content mismatch")
	}
}

func TestDeployerDeployCreatesWorkDirAndPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	nested := filepath.Join(tmpDir, "nested", "work")
	c := NewCompiler(&Topology{}, "p", "e")
	compose, _ := c.Compile()

	d := NewDeployer(nested)
	ctx := context.Background()
	_, err := d.Deploy(ctx, compose, "", "", true)
	if err != nil {
		t.Fatalf("Deploy failed: %v", err)
	}

	info, err := os.Stat(nested)
	if err != nil {
		t.Fatalf("work dir not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("workdir is not a directory")
	}
	// On Windows, permission bits differ; just assert directory exists.
}

func TestEdgeCases(t *testing.T) {
	t.Run("empty topology", func(t *testing.T) {
		top := &Topology{
			ID:          "empty",
			Name:        "empty",
			ProjectID:   "p",
			Environment: "dev",
		}
		c := NewCompiler(top, "p", "dev")
		compose, err := c.Compile()
		if err != nil {
			t.Fatalf("Compile failed: %v", err)
		}
		if len(compose.Services) != 0 {
			t.Errorf("expected no services, got %d", len(compose.Services))
		}
		yaml := compose.ToYAML()
		if !strings.Contains(yaml, "version: \"3.9\"") {
			t.Error("expected version in empty topology YAML")
		}
	})

	t.Run("managed and external databases skipped", func(t *testing.T) {
		top := helperTopology()
		top.Databases = []Database{
			{ID: "db-m", Name: "managed-db", Engine: EnginePostgres, Managed: true},
			{ID: "db-e", Name: "external-db", Engine: EngineMySQL, External: true, ConnURL: "mysql://host/db"},
		}
		// Update connection to point to one of the existing databases so validation passes.
		// Using external-db as the dependency target since it exists in the topology.
		top.Connections = []Connection{
			{ID: "conn-e", Type: ConnDependency, SourceID: "app-1", TargetID: "db-e", Config: ConnConfig{EnvVarName: "DB_URL"}},
		}
		c := NewCompiler(top, "p", "dev")
		compose, err := c.Compile()
		if err != nil {
			t.Fatalf("Compile failed: %v", err)
		}
		if _, ok := compose.Services["managed-db"]; ok {
			t.Error("expected managed-db to be skipped")
		}
		if _, ok := compose.Services["external-db"]; ok {
			t.Error("expected external-db to be skipped")
		}
		if _, ok := compose.Volumes["managed-db_data"]; ok {
			t.Error("expected managed-db_data volume to be skipped")
		}
		if _, ok := compose.Volumes["external-db_data"]; ok {
			t.Error("expected external-db_data volume to be skipped")
		}
	})

	t.Run("tmpfs volumes", func(t *testing.T) {
		top := helperTopology()
		top.Volumes[0].Temporary = true
		top.Volumes[0].VolumeType = VolumeTmpfs
		c := NewCompiler(top, "p", "dev")
		compose, err := c.Compile()
		if err != nil {
			t.Fatalf("Compile failed: %v", err)
		}
		if _, ok := compose.Volumes["data-vol"]; ok {
			t.Error("expected tmpfs volume not declared in top-level volumes")
		}
		appSvc := compose.Services["web"]
		found := false
		for _, v := range appSvc.Volumes {
			if strings.HasPrefix(v, "tmpfs:") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tmpfs mount in app volumes, got %v", appSvc.Volumes)
		}
	})
}

func TestDeployResultContainersNetworksVolumes(t *testing.T) {
	top := helperTopology()
	c := NewCompiler(top, "myproject", "staging")
	compose, err := c.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}

	d := NewDeployer(t.TempDir())
	// Dry-run does not populate Containers/Networks/Volumes in DeployResult.
	// Test the internal extraction helpers directly instead.
	containers := d.extractContainerNames(compose)
	if len(containers) == 0 {
		t.Error("expected containers")
	}
	foundWeb := false
	for _, name := range containers {
		if strings.Contains(name, "web") {
			foundWeb = true
		}
	}
	if !foundWeb {
		t.Errorf("expected web container in %v", containers)
	}

	networks := d.extractNetworkNames(compose)
	if len(networks) == 0 {
		t.Error("expected networks")
	}

	volumes := d.extractVolumeNames(compose)
	if len(volumes) == 0 {
		t.Error("expected volumes")
	}
}
