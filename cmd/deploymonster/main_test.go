package main

import (
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
