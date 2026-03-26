package engines

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// =====================================================
// REGISTRY TESTS
// =====================================================

func TestRegistryGet_AllEngines(t *testing.T) {
	tests := []struct {
		name     string
		wantPort int
	}{
		{"postgres", 5432},
		{"mysql", 3306},
		{"mariadb", 3306},
		{"redis", 6379},
		{"mongodb", 27017},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine, ok := Get(tt.name)
			if !ok {
				t.Fatalf("engine %q not found in registry", tt.name)
			}
			if engine.DefaultPort() != tt.wantPort {
				t.Errorf("DefaultPort() = %d, want %d", engine.DefaultPort(), tt.wantPort)
			}
		})
	}
}

func TestRegistryGet_NotFound(t *testing.T) {
	_, ok := Get("cockroachdb")
	if ok {
		t.Error("expected false for unregistered engine")
	}

	_, ok = Get("")
	if ok {
		t.Error("expected false for empty name")
	}

	_, ok = Get("POSTGRES") // case-sensitive
	if ok {
		t.Error("expected false for wrong case (registry is case-sensitive)")
	}
}

func TestRegistryContainsAllExpected(t *testing.T) {
	expected := map[string]bool{
		"postgres": true,
		"mysql":    true,
		"mariadb":  true,
		"redis":    true,
		"mongodb":  true,
	}

	if len(Registry) != len(expected) {
		t.Errorf("Registry has %d engines, want %d", len(Registry), len(expected))
	}

	for name := range expected {
		if _, ok := Registry[name]; !ok {
			t.Errorf("engine %q missing from Registry", name)
		}
	}
}

// =====================================================
// CONNECTION STRING TESTS (table-driven per engine)
// =====================================================

func TestPostgres_ConnectionString_Format(t *testing.T) {
	e := &Postgres{}
	tests := []struct {
		name     string
		host     string
		port     int
		creds    Credentials
		contains []string
	}{
		{
			"standard",
			"db.example.com", 5432,
			Credentials{Database: "mydb", User: "admin", Password: "s3cret"},
			[]string{"postgres://", "admin", "s3cret", "db.example.com", "5432", "mydb", "sslmode=disable"},
		},
		{
			"non-standard port",
			"localhost", 15432,
			Credentials{Database: "testdb", User: "test", Password: "pass"},
			[]string{"15432"},
		},
		{
			"special chars in password",
			"localhost", 5432,
			Credentials{Database: "app", User: "user", Password: "p@ss:word"},
			[]string{"p@ss:word"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := e.ConnectionString(tt.host, tt.port, tt.creds)
			for _, s := range tt.contains {
				if !strings.Contains(conn, s) {
					t.Errorf("ConnectionString() = %q, missing %q", conn, s)
				}
			}
		})
	}
}

func TestMySQL_ConnectionString_Format(t *testing.T) {
	e := &MySQL{}
	tests := []struct {
		name     string
		host     string
		port     int
		creds    Credentials
		contains []string
	}{
		{
			"standard",
			"mysql.local", 3306,
			Credentials{Database: "webapp", User: "root", Password: "rootpass"},
			[]string{"root:rootpass", "tcp(mysql.local:3306)", "webapp", "parseTime=true"},
		},
		{
			"non-standard port",
			"127.0.0.1", 13306,
			Credentials{Database: "db", User: "u", Password: "p"},
			[]string{"tcp(127.0.0.1:13306)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := e.ConnectionString(tt.host, tt.port, tt.creds)
			for _, s := range tt.contains {
				if !strings.Contains(conn, s) {
					t.Errorf("ConnectionString() = %q, missing %q", conn, s)
				}
			}
		})
	}
}

func TestMariaDB_ConnectionString_Format(t *testing.T) {
	e := &MariaDB{}
	creds := Credentials{Database: "maria_db", User: "maria", Password: "pass"}
	conn := e.ConnectionString("db-host", 3306, creds)

	expected := []string{"maria:pass", "tcp(db-host:3306)", "maria_db", "parseTime=true"}
	for _, s := range expected {
		if !strings.Contains(conn, s) {
			t.Errorf("ConnectionString() = %q, missing %q", conn, s)
		}
	}
}

func TestRedis_ConnectionString_WithAndWithoutPassword(t *testing.T) {
	e := &Redis{}

	t.Run("with password", func(t *testing.T) {
		conn := e.ConnectionString("redis.local", 6379, Credentials{Password: "redispass"})
		if !strings.HasPrefix(conn, "redis://") {
			t.Errorf("should start with redis://, got %q", conn)
		}
		if !strings.Contains(conn, ":redispass@") {
			t.Errorf("should contain password, got %q", conn)
		}
		if !strings.Contains(conn, "redis.local:6379") {
			t.Errorf("should contain host:port, got %q", conn)
		}
	})

	t.Run("without password", func(t *testing.T) {
		conn := e.ConnectionString("cache", 6380, Credentials{})
		if !strings.HasPrefix(conn, "redis://") {
			t.Errorf("should start with redis://, got %q", conn)
		}
		if strings.Contains(conn, "@") {
			t.Errorf("no-password URL should not contain @, got %q", conn)
		}
		if !strings.Contains(conn, "cache:6380") {
			t.Errorf("should contain host:port, got %q", conn)
		}
	})
}

func TestMongoDB_ConnectionString_Format(t *testing.T) {
	e := &MongoDB{}
	creds := Credentials{Database: "mydb", User: "admin", Password: "secret"}
	conn := e.ConnectionString("mongo-host", 27017, creds)

	expected := []string{"mongodb://", "admin:secret@", "mongo-host:27017", "/mydb", "authSource=admin"}
	for _, s := range expected {
		if !strings.Contains(conn, s) {
			t.Errorf("ConnectionString() = %q, missing %q", conn, s)
		}
	}
}

// =====================================================
// IMAGE TESTS
// =====================================================

func TestAllEngines_ImageFormat(t *testing.T) {
	tests := []struct {
		engine   Engine
		version  string
		contains string
	}{
		{&Postgres{}, "16", "postgres:16-alpine"},
		{&Postgres{}, "17", "postgres:17-alpine"},
		{&MySQL{}, "8.4", "mysql:8.4"},
		{&MySQL{}, "8.0", "mysql:8.0"},
		{&MariaDB{}, "11", "mariadb:11"},
		{&MariaDB{}, "10.11", "mariadb:10.11"},
		{&Redis{}, "7", "redis:7-alpine"},
		{&MongoDB{}, "7", "mongo:7"},
	}

	for _, tt := range tests {
		t.Run(tt.engine.Name()+"/"+tt.version, func(t *testing.T) {
			img := tt.engine.Image(tt.version)
			if img != tt.contains {
				t.Errorf("Image(%q) = %q, want %q", tt.version, img, tt.contains)
			}
		})
	}
}

// =====================================================
// ENV VAR TESTS
// =====================================================

func TestPostgres_EnvVars(t *testing.T) {
	e := &Postgres{}
	creds := Credentials{Database: "testdb", User: "testuser", Password: "testpass"}
	env := e.Env(creds)

	wantEnv := map[string]bool{
		"POSTGRES_DB=testdb":         true,
		"POSTGRES_USER=testuser":     true,
		"POSTGRES_PASSWORD=testpass": true,
	}

	if len(env) != len(wantEnv) {
		t.Fatalf("Env() returned %d vars, want %d", len(env), len(wantEnv))
	}

	for _, v := range env {
		if !wantEnv[v] {
			t.Errorf("unexpected env var: %q", v)
		}
	}
}

func TestMySQL_EnvVars(t *testing.T) {
	e := &MySQL{}
	creds := Credentials{Database: "mydb", User: "myuser", Password: "mypass"}
	env := e.Env(creds)

	if len(env) != 4 {
		t.Fatalf("MySQL Env() should have 4 vars (includes root password), got %d", len(env))
	}

	wantEnv := map[string]bool{
		"MYSQL_DATABASE=mydb":        true,
		"MYSQL_USER=myuser":          true,
		"MYSQL_PASSWORD=mypass":      true,
		"MYSQL_ROOT_PASSWORD=mypass": true,
	}

	for _, v := range env {
		if !wantEnv[v] {
			t.Errorf("unexpected env var: %q", v)
		}
	}
}

func TestMariaDB_EnvVars(t *testing.T) {
	e := &MariaDB{}
	creds := Credentials{Database: "mdb", User: "muser", Password: "mpass"}
	env := e.Env(creds)

	if len(env) != 4 {
		t.Fatalf("MariaDB Env() should have 4 vars, got %d", len(env))
	}

	wantEnv := map[string]bool{
		"MARIADB_DATABASE=mdb":        true,
		"MARIADB_USER=muser":          true,
		"MARIADB_PASSWORD=mpass":      true,
		"MARIADB_ROOT_PASSWORD=mpass": true,
	}

	for _, v := range env {
		if !wantEnv[v] {
			t.Errorf("unexpected env var: %q", v)
		}
	}
}

func TestRedis_EnvVars_WithPassword(t *testing.T) {
	e := &Redis{}
	env := e.Env(Credentials{Password: "redis123"})
	if len(env) != 1 {
		t.Fatalf("Redis Env with password should have 1 var, got %d", len(env))
	}
	if env[0] != "REDIS_PASSWORD=redis123" {
		t.Errorf("unexpected env var: %q", env[0])
	}
}

func TestRedis_EnvVars_NoPassword(t *testing.T) {
	e := &Redis{}
	env := e.Env(Credentials{})
	if env != nil {
		t.Errorf("Redis Env without password should return nil, got %v", env)
	}
}

func TestMongoDB_EnvVars(t *testing.T) {
	e := &MongoDB{}
	creds := Credentials{Database: "admin_db", User: "root", Password: "mongopass"}
	env := e.Env(creds)

	if len(env) != 3 {
		t.Fatalf("MongoDB Env() should have 3 vars, got %d", len(env))
	}

	wantEnv := map[string]bool{
		"MONGO_INITDB_ROOT_USERNAME=root":      true,
		"MONGO_INITDB_ROOT_PASSWORD=mongopass": true,
		"MONGO_INITDB_DATABASE=admin_db":       true,
	}

	for _, v := range env {
		if !wantEnv[v] {
			t.Errorf("unexpected env var: %q", v)
		}
	}
}

// =====================================================
// HEALTH CHECK COMMAND TESTS
// =====================================================

func TestHealthCmd_Content(t *testing.T) {
	tests := []struct {
		engine      Engine
		wantFirst   string
		wantMinArgs int
	}{
		{&Postgres{}, "pg_isready", 3},
		{&MySQL{}, "mysqladmin", 4},
		{&MariaDB{}, "healthcheck.sh", 3},
		{&Redis{}, "redis-cli", 2},
		{&MongoDB{}, "mongosh", 3},
	}

	for _, tt := range tests {
		t.Run(tt.engine.Name(), func(t *testing.T) {
			cmd := tt.engine.HealthCmd()
			if len(cmd) < tt.wantMinArgs {
				t.Errorf("HealthCmd() has %d args, want at least %d", len(cmd), tt.wantMinArgs)
			}
			if cmd[0] != tt.wantFirst {
				t.Errorf("HealthCmd()[0] = %q, want %q", cmd[0], tt.wantFirst)
			}
		})
	}
}

// =====================================================
// VERSIONS TESTS
// =====================================================

func TestVersions_LatestFirst(t *testing.T) {
	// The first version returned should be the latest (convention for Provision default).
	tests := []struct {
		engine    Engine
		wantFirst string
	}{
		{&Postgres{}, "17"},
		{&MySQL{}, "8.4"},
		{&MariaDB{}, "11"},
		{&Redis{}, "7"},
		{&MongoDB{}, "7"},
	}

	for _, tt := range tests {
		t.Run(tt.engine.Name(), func(t *testing.T) {
			versions := tt.engine.Versions()
			if len(versions) == 0 {
				t.Fatal("no versions returned")
			}
			if versions[0] != tt.wantFirst {
				t.Errorf("Versions()[0] = %q, want %q (latest first)", versions[0], tt.wantFirst)
			}
		})
	}
}

// =====================================================
// PROVISION TESTS
// =====================================================

type stubRuntime struct {
	createOpts core.ContainerOpts
	createErr  error
}

func (s *stubRuntime) Ping() error { return nil }
func (s *stubRuntime) CreateAndStart(_ context.Context, opts core.ContainerOpts) (string, error) {
	s.createOpts = opts
	if s.createErr != nil {
		return "", s.createErr
	}
	return "container-abc123", nil
}
func (s *stubRuntime) Stop(_ context.Context, _ string, _ int) error    { return nil }
func (s *stubRuntime) Remove(_ context.Context, _ string, _ bool) error { return nil }
func (s *stubRuntime) Restart(_ context.Context, _ string) error        { return nil }
func (s *stubRuntime) Logs(_ context.Context, _ string, _ string, _ bool) (io.ReadCloser, error) {
	return nil, nil
}
func (s *stubRuntime) ListByLabels(_ context.Context, _ map[string]string) ([]core.ContainerInfo, error) {
	return nil, nil
}

func (s *stubRuntime) Exec(_ context.Context, _ string, _ []string) (string, error) {
	return "", nil
}

func (s *stubRuntime) Stats(_ context.Context, _ string) (*core.ContainerStats, error) {
	return nil, nil
}

func (s *stubRuntime) ImagePull(_ context.Context, _ string) error { return nil }

func (s *stubRuntime) ImageList(_ context.Context) ([]core.ImageInfo, error) {
	return nil, nil
}

func (s *stubRuntime) ImageRemove(_ context.Context, _ string) error { return nil }

func (s *stubRuntime) NetworkList(_ context.Context) ([]core.NetworkInfo, error) {
	return nil, nil
}

func (s *stubRuntime) VolumeList(_ context.Context) ([]core.VolumeInfo, error) {
	return nil, nil
}

func TestProvision_NilRuntime(t *testing.T) {
	engine := &Postgres{}
	opts := ProvisionOpts{
		TenantID: "t1",
		Name:     "mydb",
		Engine:   "postgres",
	}

	_, _, err := Provision(context.Background(), nil, engine, opts)
	if err == nil {
		t.Fatal("expected error with nil runtime")
	}
	if !strings.Contains(err.Error(), "container runtime not available") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProvision_Success(t *testing.T) {
	runtime := &stubRuntime{}
	engine := &Postgres{}
	opts := ProvisionOpts{
		TenantID: "tenant-1",
		Name:     "my-database",
		Engine:   "postgres",
		Version:  "16",
	}

	containerID, creds, err := Provision(context.Background(), runtime, engine, opts)
	if err != nil {
		t.Fatalf("Provision returned error: %v", err)
	}

	if containerID != "container-abc123" {
		t.Errorf("containerID = %q, want %q", containerID, "container-abc123")
	}

	if creds.Database != "my-database" {
		t.Errorf("creds.Database = %q, want %q", creds.Database, "my-database")
	}
	if creds.User != "my-database" {
		t.Errorf("creds.User = %q, want %q", creds.User, "my-database")
	}
	if len(creds.Password) != 24 {
		t.Errorf("creds.Password length = %d, want 24", len(creds.Password))
	}

	// Verify container options
	if !strings.Contains(runtime.createOpts.Image, "postgres:16") {
		t.Errorf("image = %q, should contain postgres:16", runtime.createOpts.Image)
	}
	if runtime.createOpts.Network != "monster-network" {
		t.Errorf("network = %q, want monster-network", runtime.createOpts.Network)
	}
	if runtime.createOpts.RestartPolicy != "unless-stopped" {
		t.Errorf("restart policy = %q, want unless-stopped", runtime.createOpts.RestartPolicy)
	}
	if runtime.createOpts.Labels["monster.enable"] != "true" {
		t.Error("expected monster.enable=true label")
	}
	if runtime.createOpts.Labels["monster.managed"] != "database" {
		t.Error("expected monster.managed=database label")
	}
	if runtime.createOpts.Labels["monster.db.engine"] != "postgres" {
		t.Error("expected monster.db.engine=postgres label")
	}
	if runtime.createOpts.Labels["monster.tenant"] != "tenant-1" {
		t.Error("expected monster.tenant=tenant-1 label")
	}
}

func TestProvision_DefaultVersion(t *testing.T) {
	runtime := &stubRuntime{}
	engine := &MySQL{}
	opts := ProvisionOpts{
		TenantID: "t1",
		Name:     "db",
		Engine:   "mysql",
		Version:  "", // Should default to first version
	}

	_, _, err := Provision(context.Background(), runtime, engine, opts)
	if err != nil {
		t.Fatalf("Provision returned error: %v", err)
	}

	// Default version for MySQL is "8.4" (first in Versions())
	if !strings.Contains(runtime.createOpts.Image, "mysql:8.4") {
		t.Errorf("image = %q, expected mysql:8.4 for default version", runtime.createOpts.Image)
	}
}

func TestProvision_RuntimeError(t *testing.T) {
	runtime := &stubRuntime{createErr: fmt.Errorf("docker: image not found")}
	engine := &Redis{}
	opts := ProvisionOpts{
		TenantID: "t1",
		Name:     "cache",
		Engine:   "redis",
		Version:  "7",
	}

	_, _, err := Provision(context.Background(), runtime, engine, opts)
	if err == nil {
		t.Fatal("expected error when runtime fails")
	}
	if !strings.Contains(err.Error(), "provision redis") {
		t.Errorf("error should mention engine name, got: %v", err)
	}
}

func TestProvision_ContainerNameFormat(t *testing.T) {
	runtime := &stubRuntime{}
	engine := &MongoDB{}
	opts := ProvisionOpts{
		TenantID: "t1",
		Name:     "analytics",
		Engine:   "mongodb",
		Version:  "7",
	}

	_, _, err := Provision(context.Background(), runtime, engine, opts)
	if err != nil {
		t.Fatalf("Provision returned error: %v", err)
	}

	if !strings.HasPrefix(runtime.createOpts.Name, "monster-db-mongodb-") {
		t.Errorf("container name = %q, should start with 'monster-db-mongodb-'", runtime.createOpts.Name)
	}
}

// =====================================================
// ENGINE NAME CONSISTENCY
// =====================================================

func TestEngineNameMatchesRegistryKey(t *testing.T) {
	for key, engine := range Registry {
		if engine.Name() != key {
			t.Errorf("Registry key %q does not match engine.Name() %q", key, engine.Name())
		}
	}
}
