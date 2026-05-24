package main

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInjectSystemdEnvironmentFile(t *testing.T) {
	unit := `[Unit]
Description=DeployMonster

[Service]
ExecStart=/usr/local/bin/deploymonster serve
Environment=MONSTER_ADMIN_EMAIL=admin@example.com
Environment=MONSTER_ADMIN_PASSWORD=secret

[Install]
WantedBy=multi-user.target`

	got, err := injectSystemdEnvironmentFile(unit)
	if err != nil {
		t.Fatalf("injectSystemdEnvironmentFile: %v", err)
	}
	if !strings.Contains(got, "EnvironmentFile=-"+systemdAdminEnvFile) {
		t.Fatalf("missing EnvironmentFile reference:\n%s", got)
	}
	if strings.Contains(got, "MONSTER_ADMIN_PASSWORD=secret") || strings.Contains(got, "MONSTER_ADMIN_EMAIL=admin@example.com") {
		t.Fatalf("unit still contains inline admin credentials:\n%s", got)
	}
}

func TestInjectSystemdEnvironmentFileRequiresServiceSection(t *testing.T) {
	if _, err := injectSystemdEnvironmentFile("[Unit]\nDescription=DeployMonster"); err == nil {
		t.Fatal("expected missing [Service] section to fail")
	}
}

func TestEnvInt(t *testing.T) {
	t.Setenv("DM_TEST_INT", "")
	if got := envInt("DM_TEST_INT", 7); got != 7 {
		t.Fatalf("empty env = %d, want default", got)
	}
	t.Setenv("DM_TEST_INT", "42")
	if got := envInt("DM_TEST_INT", 7); got != 42 {
		t.Fatalf("valid env = %d, want 42", got)
	}
	t.Setenv("DM_TEST_INT", "not-an-int")
	if got := envInt("DM_TEST_INT", 7); got != 7 {
		t.Fatalf("invalid env = %d, want default", got)
	}
}

func TestPromptHelpers(t *testing.T) {
	if got := prompt(bufio.NewReader(strings.NewReader("\n")), "Name", "default"); got != "default" {
		t.Fatalf("prompt default = %q", got)
	}
	if got := prompt(bufio.NewReader(strings.NewReader("custom\n")), "Name", "default"); got != "custom" {
		t.Fatalf("prompt custom = %q", got)
	}
	if got := promptBool(bufio.NewReader(strings.NewReader("\n")), "Enabled", true); !got {
		t.Fatal("promptBool default true returned false")
	}
	if got := promptBool(bufio.NewReader(strings.NewReader("yes\n")), "Enabled", false); !got {
		t.Fatal("promptBool yes returned false")
	}
	if got := promptBool(bufio.NewReader(strings.NewReader("no\n")), "Enabled", true); got {
		t.Fatal("promptBool no returned true")
	}
}

func TestRunVersionAndUsage(t *testing.T) {
	oldVersion, oldCommit, oldDate := version, commit, date
	version, commit, date = "1.2.3", "abc123", "2026-05-24"
	t.Cleanup(func() { version, commit, date = oldVersion, oldCommit, oldDate })

	out := captureStdout(t, runVersion)
	for _, want := range []string{"DeployMonster 1.2.3", "commit:  abc123", "built:   2026-05-24"} {
		if !strings.Contains(out, want) {
			t.Fatalf("runVersion output missing %q:\n%s", want, out)
		}
	}

	out = captureStdout(t, printUsage)
	if !strings.Contains(out, "deploymonster [command]") || !strings.Contains(out, "rotate-keys") {
		t.Fatalf("usage output missing expected content:\n%s", out)
	}
}

func TestRunInitCreatesMonsterYAML(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	out := captureStdout(t, runInit)
	if !strings.Contains(out, "Created monster.yaml") {
		t.Fatalf("runInit output = %q", out)
	}
	data, err := os.ReadFile(filepath.Join(dir, "monster.yaml"))
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if !bytes.Contains(data, []byte("registration:")) || !bytes.Contains(data, []byte("max_apps_per_tenant: 100")) {
		t.Fatalf("generated config missing expected defaults:\n%s", string(data))
	}
}

func TestRunConfigCheckDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	out := captureStdout(t, runConfigCheck)
	if !strings.Contains(out, "Config OK") || !strings.Contains(out, "\"Server\"") {
		t.Fatalf("runConfigCheck output missing config JSON:\n%s", out)
	}
}

func TestRunHealthCheckSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("path = %q, want /health", r.URL.Path)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}

	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})
	config := "server:\n  host: " + host + "\n  port: " + port + "\n"
	if err := os.WriteFile("monster.yaml", []byte(config), 0600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	out := captureStdout(t, runHealthCheck)
	if strings.TrimSpace(out) != "ok" {
		t.Fatalf("health output = %q, want ok", out)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return buf.String()
}
