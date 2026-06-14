package db

import (
	"context"
	"errors"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestSQLite_ServerCRUDAndDefaults(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	srv := &core.Server{
		TenantID:         "tenant-1",
		Hostname:         "worker-1",
		IPAddress:        "10.0.0.2",
		ProviderRef:      "i-123",
		Region:           "eu-north-1",
		Size:             "small",
		SSHKeyID:         "key-1",
		DockerVersion:    "26.0",
		CPUCores:         4,
		RAMmb:            8192,
		DiskMB:           102400,
		MonthlyCostCents: 1234,
		SwarmJoined:      true,
	}
	if err := db.CreateServer(ctx, srv); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	if srv.ID == "" {
		t.Fatal("CreateServer should assign an ID")
	}

	got, err := db.GetServer(ctx, srv.ID)
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if got.Role != "worker" || got.ProviderType != "custom" || got.SSHPort != 22 ||
		got.Status != "provisioning" || got.AgentStatus != "unknown" {
		t.Fatalf("defaults not applied: %+v", got)
	}
	if got.TenantID != "tenant-1" || got.SSHKeyID != "key-1" || !got.SwarmJoined {
		t.Fatalf("nullable/scanned fields mismatch: %+v", got)
	}

	if err := db.UpdateServerStatus(ctx, srv.ID, "active"); err != nil {
		t.Fatalf("UpdateServerStatus: %v", err)
	}
	got, err = db.GetServer(ctx, srv.ID)
	if err != nil {
		t.Fatalf("GetServer after update: %v", err)
	}
	if got.Status != "active" {
		t.Fatalf("status = %q, want active", got.Status)
	}

	tenantServers, err := db.ListServersByTenant(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("ListServersByTenant: %v", err)
	}
	if len(tenantServers) != 1 || tenantServers[0].ID != srv.ID {
		t.Fatalf("tenant servers = %+v", tenantServers)
	}

	allServers, err := db.ListAllServers(ctx)
	if err != nil {
		t.Fatalf("ListAllServers: %v", err)
	}
	if len(allServers) != 1 || allServers[0].ID != srv.ID {
		t.Fatalf("all servers = %+v", allServers)
	}

	if err := db.DeleteServer(ctx, srv.ID); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
	if _, err := db.GetServer(ctx, srv.ID); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("GetServer deleted err = %v, want ErrNotFound", err)
	}
}

func TestSQLite_ListServersByTenantIncludesSharedServers(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tenantServer := &core.Server{TenantID: "tenant-1", Hostname: "tenant-worker", IPAddress: "10.0.0.2"}
	sharedServer := &core.Server{Hostname: "shared-worker", IPAddress: "10.0.0.3"}
	otherServer := &core.Server{TenantID: "tenant-2", Hostname: "other-worker", IPAddress: "10.0.0.4"}

	for _, srv := range []*core.Server{tenantServer, sharedServer, otherServer} {
		if err := db.CreateServer(ctx, srv); err != nil {
			t.Fatalf("CreateServer %s: %v", srv.Hostname, err)
		}
	}

	servers, err := db.ListServersByTenant(ctx, "tenant-1")
	if err != nil {
		t.Fatalf("ListServersByTenant: %v", err)
	}
	seen := map[string]bool{}
	for _, srv := range servers {
		seen[srv.Hostname] = true
	}
	if !seen["tenant-worker"] || !seen["shared-worker"] {
		t.Fatalf("expected tenant and shared servers, got %+v", servers)
	}
	if seen["other-worker"] {
		t.Fatalf("tenant-2 server leaked into tenant-1 list: %+v", servers)
	}
}
