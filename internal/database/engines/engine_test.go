package engines

import (
	"strings"
	"testing"
)

func TestAllEngines_Registered(t *testing.T) {
	expected := []string{"postgres", "mysql", "mariadb", "redis", "mongodb"}
	for _, name := range expected {
		e, ok := Get(name)
		if !ok {
			t.Errorf("engine %q not registered", name)
			continue
		}
		if e.Name() != name {
			t.Errorf("expected name %q, got %q", name, e.Name())
		}
	}
}

func TestPostgres(t *testing.T) {
	e := &Postgres{}

	if e.DefaultPort() != 5432 {
		t.Errorf("expected port 5432, got %d", e.DefaultPort())
	}

	if !strings.Contains(e.Image("17"), "postgres:17") {
		t.Errorf("unexpected image: %s", e.Image("17"))
	}

	creds := Credentials{Database: "mydb", User: "myuser", Password: "pass123"}
	env := e.Env(creds)
	if len(env) != 3 {
		t.Errorf("expected 3 env vars, got %d", len(env))
	}

	conn := e.ConnectionString("localhost", 5432, creds)
	if !strings.Contains(conn, "postgres://") {
		t.Errorf("connection string should start with postgres://")
	}
	if !strings.Contains(conn, "myuser") || !strings.Contains(conn, "pass123") {
		t.Error("connection string should contain credentials")
	}
}

func TestMySQL(t *testing.T) {
	e := &MySQL{}

	if e.DefaultPort() != 3306 {
		t.Errorf("expected port 3306, got %d", e.DefaultPort())
	}

	creds := Credentials{Database: "app", User: "app", Password: "secret"}
	env := e.Env(creds)
	if len(env) != 4 { // includes root password
		t.Errorf("expected 4 env vars, got %d", len(env))
	}

	conn := e.ConnectionString("db", 3306, creds)
	if !strings.Contains(conn, "tcp(db:3306)") {
		t.Errorf("connection string should contain host: %s", conn)
	}
}

func TestRedis(t *testing.T) {
	e := &Redis{}

	if e.DefaultPort() != 6379 {
		t.Errorf("expected port 6379, got %d", e.DefaultPort())
	}

	conn := e.ConnectionString("cache", 6379, Credentials{Password: "secret"})
	if !strings.HasPrefix(conn, "redis://") {
		t.Errorf("connection string should start with redis://")
	}

	// No password
	conn2 := e.ConnectionString("cache", 6379, Credentials{})
	if strings.Contains(conn2, ":@") {
		t.Error("no-password connection string should not have :@")
	}
}

func TestMongoDB(t *testing.T) {
	e := &MongoDB{}

	if e.DefaultPort() != 27017 {
		t.Errorf("expected port 27017, got %d", e.DefaultPort())
	}

	conn := e.ConnectionString("mongo", 27017, Credentials{User: "root", Password: "pass", Database: "app"})
	if !strings.HasPrefix(conn, "mongodb://") {
		t.Errorf("connection string should start with mongodb://")
	}
}

func TestVersions_NotEmpty(t *testing.T) {
	for name, engine := range Registry {
		versions := engine.Versions()
		if len(versions) == 0 {
			t.Errorf("engine %q has no versions", name)
		}
	}
}

func TestHealthCmd_NotEmpty(t *testing.T) {
	for name, engine := range Registry {
		cmd := engine.HealthCmd()
		if len(cmd) == 0 {
			t.Errorf("engine %q has no health check command", name)
		}
	}
}
