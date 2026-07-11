package db

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — CreateDeploymentAtomicVersion (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_CreateDeploymentAtomicVersion_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	dep := &core.Deployment{
		ID: "d1", AppID: "a1", Image: "img:v1", ContainerID: "c1",
		Status: "deploying", BuildLog: "log", CommitSHA: "abc123",
		CommitMessage: "msg", TriggeredBy: "test", Strategy: "recreate",
		StartedAt: &now,
	}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`INSERT INTO deployments`).
		WithArgs(dep.ID, dep.AppID, dep.Image, dep.ContainerID, dep.Status,
			dep.BuildLog, dep.CommitSHA, dep.CommitMessage, dep.TriggeredBy,
			dep.Strategy, dep.StartedAt, dep.AppID).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
	mock.ExpectCommit()

	if err := pg.CreateDeploymentAtomicVersion(context.Background(), dep); err != nil {
		t.Fatalf("CreateDeploymentAtomicVersion: %v", err)
	}
	if dep.Version != 1 {
		t.Errorf("Version = %d, want 1", dep.Version)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}

func TestPostgresDB_CreateDeploymentAtomicVersion_BeginTxError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))

	dep := &core.Deployment{AppID: "a1", Image: "img:v1"}
	if err := pg.CreateDeploymentAtomicVersion(context.Background(), dep); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_CreateDeploymentAtomicVersion_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`INSERT INTO deployments`).WillReturnError(errors.New("query failed"))
	mock.ExpectRollback()

	dep := &core.Deployment{AppID: "a1", Image: "img:v1"}
	if err := pg.CreateDeploymentAtomicVersion(context.Background(), dep); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_CreateDeploymentAtomicVersion_PresetID(t *testing.T) {
	pg, mock := newMockPostgres(t)
	dep := &core.Deployment{
		ID: "custom-id", AppID: "a1", Image: "img:v1",
		TriggeredBy: "test", Strategy: "recreate",
	}
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`INSERT INTO deployments`).
		WithArgs(dep.ID, dep.AppID, dep.Image, sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			sqlmock.AnyArg(), sqlmock.AnyArg(), dep.AppID).
		WillReturnRows(sqlmock.NewRows([]string{"version"}).AddRow(1))
	mock.ExpectCommit()

	if err := pg.CreateDeploymentAtomicVersion(context.Background(), dep); err != nil {
		t.Fatalf("CreateDeploymentAtomicVersion: %v", err)
	}
	if dep.ID != "custom-id" {
		t.Errorf("ID = %q", dep.ID)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — GetAppsByIDs (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_GetAppsByIDs_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM applications WHERE id IN").
		WithArgs("a1", "a2").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "project_id", "tenant_id", "name", "type", "source_type", "source_url",
			"branch", "dockerfile", "build_pack", "env_vars_enc", "labels_json",
			"replicas", "status", "server_id", "created_at", "updated_at",
		}).AddRow("a1", "p1", "t1", "app1", "web", "git", "url1", "main",
			"Dockerfile", "", "", "{}", 1, "running", "", now, now).
		AddRow("a2", "p1", "t1", "app2", "web", "git", "url2", "develop",
			"Dockerfile", "", "", "{}", 1, "running", "", now, now))

	out, err := pg.GetAppsByIDs(context.Background(), []string{"a1", "a2"})
	if err != nil || len(out) != 2 {
		t.Fatalf("GetAppsByIDs: err=%v len=%d", err, len(out))
	}
}

func TestPostgresDB_GetAppsByIDs_Empty(t *testing.T) {
	pg, _ := newMockPostgres(t)
	out, err := pg.GetAppsByIDs(context.Background(), []string{})
	if err != nil || out != nil {
		t.Fatalf("GetAppsByIDs empty: err=%v out=%v", err, out)
	}
}

func TestPostgresDB_GetAppsByIDs_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM applications WHERE id IN").
		WillReturnError(errors.New("query failed"))
	if _, err := pg.GetAppsByIDs(context.Background(), []string{"a1"}); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_GetAppsByIDs_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM applications WHERE id IN").
		WithArgs("a1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("a1"))
	if _, err := pg.GetAppsByIDs(context.Background(), []string{"a1"}); err == nil {
		t.Error("expected scan error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — GetUsersByIDs (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_GetUsersByIDs_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM users WHERE id IN").
		WithArgs("u1", "u2", "t1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "email", "password_hash", "name", "avatar_url", "status",
			"totp_enabled", "totp_secret_enc", "totp_backup_codes_json",
			"last_login_at", "created_at", "updated_at",
		}).AddRow("u1", "a@b.com", "hash1", "User1", "", "active", false, nil, "[]", nil, now, now).
		AddRow("u2", "c@d.com", "hash2", "User2", "", "active", false, nil, "[]", nil, now, now))

	out, err := pg.GetUsersByIDs(context.Background(), []string{"u1", "u2"}, "t1")
	if err != nil || len(out) != 2 {
		t.Fatalf("GetUsersByIDs: err=%v len=%d", err, len(out))
	}
}

func TestPostgresDB_GetUsersByIDs_Empty(t *testing.T) {
	pg, _ := newMockPostgres(t)
	out, err := pg.GetUsersByIDs(context.Background(), []string{}, "t1")
	if err != nil || out != nil {
		t.Fatalf("GetUsersByIDs empty: err=%v out=%v", err, out)
	}
}

func TestPostgresDB_GetUsersByIDs_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM users WHERE id IN").
		WillReturnError(errors.New("query failed"))
	if _, err := pg.GetUsersByIDs(context.Background(), []string{"u1"}, "t1"); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_GetUsersByIDs_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM users WHERE id IN").
		WithArgs("u1", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("u1"))
	if _, err := pg.GetUsersByIDs(context.Background(), []string{"u1"}, "t1"); err == nil {
		t.Error("expected scan error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — GetLatestDeploymentsByAppIDs (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_GetLatestDeploymentsByAppIDs_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM deployments d INNER JOIN").
		WithArgs("a1", "a2").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "app_id", "version", "image", "container_id", "status",
			"commit_sha", "commit_message", "triggered_by", "strategy",
			"started_at", "finished_at", "created_at",
		}).AddRow("d1", "a1", 2, "img:v2", "c1", "running", "", "", "test", "rolling", now, nil, now).
		AddRow("d2", "a2", 5, "img:v5", "c2", "done", "", "", "test", "recreate", now, &now, now))

	out, err := pg.GetLatestDeploymentsByAppIDs(context.Background(), []string{"a1", "a2"})
	if err != nil || len(out) != 2 {
		t.Fatalf("GetLatestDeploymentsByAppIDs: err=%v len=%d", err, len(out))
	}
	if out["a1"] == nil || out["a1"].Version != 2 {
		t.Errorf("a1 version = %d", out["a1"].Version)
	}
	if out["a2"] == nil || out["a2"].Version != 5 {
		t.Errorf("a2 version = %d", out["a2"].Version)
	}
}

func TestPostgresDB_GetLatestDeploymentsByAppIDs_Empty(t *testing.T) {
	pg, _ := newMockPostgres(t)
	out, err := pg.GetLatestDeploymentsByAppIDs(context.Background(), []string{})
	if err != nil || out != nil {
		t.Fatalf("GetLatestDeploymentsByAppIDs empty: err=%v out=%v", err, out)
	}
}

func TestPostgresDB_GetLatestDeploymentsByAppIDs_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM deployments d INNER JOIN").
		WillReturnError(errors.New("query failed"))
	if _, err := pg.GetLatestDeploymentsByAppIDs(context.Background(), []string{"a1"}); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_GetLatestDeploymentsByAppIDs_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM deployments d INNER JOIN").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("d1"))
	if _, err := pg.GetLatestDeploymentsByAppIDs(context.Background(), []string{"a1"}); err == nil {
		t.Error("expected scan error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — ListDomainsByAppIDs (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_ListDomainsByAppIDs_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()

	// First query: get allowed app IDs
	mock.ExpectQuery("SELECT id FROM applications WHERE id IN").
		WithArgs("a1", "a2", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("a1").AddRow("a2"))

	// Second query: get domains for allowed app IDs
	mock.ExpectQuery("SELECT .+ FROM domains WHERE app_id IN").
		WithArgs("a1", "a2").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "app_id", "fqdn", "type", "dns_provider", "dns_synced", "verified", "created_at",
		}).AddRow("d1", "a1", "ex.com", "auto", "cf", true, true, now).
		AddRow("d2", "a2", "ex2.com", "auto", "cf", false, false, now))

	out, err := pg.ListDomainsByAppIDs(context.Background(), []string{"a1", "a2"}, "t1")
	if err != nil || len(out) != 2 {
		t.Fatalf("ListDomainsByAppIDs: err=%v len=%d", err, len(out))
	}
}

func TestPostgresDB_ListDomainsByAppIDs_Empty(t *testing.T) {
	pg, _ := newMockPostgres(t)
	out, err := pg.ListDomainsByAppIDs(context.Background(), []string{}, "t1")
	if err != nil || out != nil {
		t.Fatalf("ListDomainsByAppIDs empty: err=%v out=%v", err, out)
	}
}

func TestPostgresDB_ListDomainsByAppIDs_NoAllowedApps(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT id FROM applications WHERE id IN").
		WithArgs("a1", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	out, err := pg.ListDomainsByAppIDs(context.Background(), []string{"a1"}, "t1")
	if err != nil || len(out) != 0 {
		t.Fatalf("expected empty map, err=%v out=%v", err, out)
	}
}

func TestPostgresDB_ListDomainsByAppIDs_FirstQueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT id FROM applications WHERE id IN").
		WillReturnError(errors.New("query failed"))
	if _, err := pg.ListDomainsByAppIDs(context.Background(), []string{"a1"}, "t1"); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_ListDomainsByAppIDs_SecondQueryScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT id FROM applications WHERE id IN").
		WithArgs("a1", "t1").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("a1"))

	mock.ExpectQuery("SELECT .+ FROM domains WHERE app_id IN").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("d1"))
	if _, err := pg.ListDomainsByAppIDs(context.Background(), []string{"a1"}, "t1"); err == nil {
		t.Error("expected scan error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — Server CRUD functions (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_CreateServer_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	srv := &core.Server{
		ID: "srv1", Hostname: "node1", IPAddress: "10.0.0.1",
		Role: "worker", ProviderType: "custom", SSHPort: 22,
		Status: "provisioning", AgentStatus: "unknown",
	}
	mock.ExpectExec("INSERT INTO servers").
		WithArgs(srv.ID, sqlmock.AnyArg(), srv.Hostname, srv.IPAddress, srv.Role,
			srv.ProviderType, "", "", "", srv.SSHPort, sqlmock.AnyArg(),
			"", 0, 0, 0, 0, 0, srv.AgentStatus, srv.Status).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := pg.CreateServer(context.Background(), srv); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
}

func TestPostgresDB_CreateServer_Defaults(t *testing.T) {
	pg, mock := newMockPostgres(t)
	srv := &core.Server{
		ID: "srv2", Hostname: "node2", IPAddress: "10.0.0.2",
	}
	mock.ExpectExec("INSERT INTO servers").
		WithArgs(srv.ID, sqlmock.AnyArg(), srv.Hostname, srv.IPAddress, "worker",
			"custom", "", "", "", 22, sqlmock.AnyArg(),
			"", 0, 0, 0, 0, 0, "unknown", "provisioning").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := pg.CreateServer(context.Background(), srv); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	// Verify defaults were set
	if srv.Role != "worker" || srv.Status != "provisioning" || srv.AgentStatus != "unknown" {
		t.Errorf("defaults not applied: Role=%q Status=%q AgentStatus=%q", srv.Role, srv.Status, srv.AgentStatus)
	}
}

func TestPostgresDB_CreateServer_SwarmJoined(t *testing.T) {
	pg, mock := newMockPostgres(t)
	srv := &core.Server{
		ID: "srv3", Hostname: "node3", IPAddress: "10.0.0.3",
		SwarmJoined: true, Role: "manager",
	}
	mock.ExpectExec("INSERT INTO servers").
		WithArgs(srv.ID, sqlmock.AnyArg(), srv.Hostname, srv.IPAddress, "manager",
			"custom", "", "", "", 22, sqlmock.AnyArg(),
			"", 0, 0, 0, 0, 1, "unknown", "provisioning").
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := pg.CreateServer(context.Background(), srv); err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
}

func TestPostgresDB_CreateServer_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	srv := &core.Server{ID: "srv_err", Hostname: "err", IPAddress: "10.0.0.99"}
	mock.ExpectExec("INSERT INTO servers").WillReturnError(errors.New("insert failed"))
	if err := pg.CreateServer(context.Background(), srv); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_GetServer_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM servers WHERE id = \\$1").
		WithArgs("srv1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "hostname", "ip_address", "role", "provider_type",
			"provider_ref", "region", "size", "ssh_port", "ssh_key_id",
			"docker_version", "cpu_cores", "ram_mb", "disk_mb",
			"monthly_cost_cents", "swarm_joined", "agent_status", "status", "created_at",
		}).AddRow("srv1", nil, "node1", "10.0.0.1", "worker", "custom", "", "", "", 22, nil,
			"", 0, 0, 0, 0, 0, "unknown", "active", now))

	out, err := pg.GetServer(context.Background(), "srv1")
	if err != nil || out.Hostname != "node1" {
		t.Fatalf("GetServer: err=%v out=%+v", err, out)
	}
}

func TestPostgresDB_GetServer_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM servers WHERE id = \\$1").
		WillReturnError(sql.ErrNoRows)
	_, err := pg.GetServer(context.Background(), "nope")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_GetServer_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM servers WHERE id = \\$1").
		WillReturnError(errors.New("query error"))
	_, err := pg.GetServer(context.Background(), "srv1")
	if err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_ListServersByTenant_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM servers WHERE").
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "hostname", "ip_address", "role", "provider_type",
			"provider_ref", "region", "size", "ssh_port", "ssh_key_id",
			"docker_version", "cpu_cores", "ram_mb", "disk_mb",
			"monthly_cost_cents", "swarm_joined", "agent_status", "status", "created_at",
		}).AddRow("s1", nil, "n1", "10.0.0.1", "worker", "custom", "", "", "", 22, nil,
			"", 0, 0, 0, 0, 0, "unknown", "active", now).
		AddRow("s2", "t1", "n2", "10.0.0.2", "worker", "aws", "", "", "", 22, nil,
			"", 0, 0, 0, 0, 0, "unknown", "active", now))

	out, err := pg.ListServersByTenant(context.Background(), "t1")
	if err != nil || len(out) != 2 {
		t.Fatalf("ListServersByTenant: err=%v len=%d", err, len(out))
	}
}

func TestPostgresDB_ListServersByTenant_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM servers WHERE").
		WillReturnError(errors.New("query failed"))
	if _, err := pg.ListServersByTenant(context.Background(), "t1"); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_ListServersByTenant_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM servers WHERE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s1"))
	if _, err := pg.ListServersByTenant(context.Background(), "t1"); err == nil {
		t.Error("expected scan error")
	}
}

func TestPostgresDB_ListAllServers_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM servers ORDER BY").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "hostname", "ip_address", "role", "provider_type",
			"provider_ref", "region", "size", "ssh_port", "ssh_key_id",
			"docker_version", "cpu_cores", "ram_mb", "disk_mb",
			"monthly_cost_cents", "swarm_joined", "agent_status", "status", "created_at",
		}).AddRow("s1", nil, "n1", "10.0.0.1", "worker", "custom", "", "", "", 22, nil,
			"", 0, 0, 0, 0, 0, "unknown", "active", now))

	out, err := pg.ListAllServers(context.Background())
	if err != nil || len(out) != 1 {
		t.Fatalf("ListAllServers: err=%v len=%d", err, len(out))
	}
}

func TestPostgresDB_ListAllServers_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM servers ORDER BY").
		WillReturnError(errors.New("query failed"))
	if _, err := pg.ListAllServers(context.Background()); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_ListAllServers_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM servers ORDER BY").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("s1"))
	if _, err := pg.ListAllServers(context.Background()); err == nil {
		t.Error("expected scan error")
	}
}

func TestPostgresDB_UpdateServerStatus_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE servers SET status").
		WithArgs("active", "srv1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.UpdateServerStatus(context.Background(), "srv1", "active"); err != nil {
		t.Fatalf("UpdateServerStatus: %v", err)
	}
}

func TestPostgresDB_UpdateServerStatus_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE servers SET status").
		WillReturnError(errors.New("update failed"))
	if err := pg.UpdateServerStatus(context.Background(), "srv1", "active"); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_DeleteServer_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM servers WHERE id").
		WithArgs("srv1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.DeleteServer(context.Background(), "srv1"); err != nil {
		t.Fatalf("DeleteServer: %v", err)
	}
}

func TestPostgresDB_DeleteServer_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM servers WHERE id").
		WillReturnError(errors.New("delete failed"))
	if err := pg.DeleteServer(context.Background(), "srv1"); err == nil {
		t.Error("expected error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — ListTeamMembers (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_ListTeamMembers_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	now := time.Now()
	mock.ExpectQuery("SELECT .+ FROM team_members WHERE").
		WithArgs("t1").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "tenant_id", "user_id", "role_id", "status", "created_at",
		}).AddRow("m1", "t1", "u1", "r1", "active", now).
		AddRow("m2", "t1", "u2", "r2", "active", now))

	out, err := pg.ListTeamMembers(context.Background(), "t1")
	if err != nil || len(out) != 2 {
		t.Fatalf("ListTeamMembers: err=%v len=%d", err, len(out))
	}
}

func TestPostgresDB_ListTeamMembers_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM team_members WHERE").
		WillReturnError(errors.New("query failed"))
	if _, err := pg.ListTeamMembers(context.Background(), "t1"); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_ListTeamMembers_ScanError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT .+ FROM team_members WHERE").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("m1"))
	if _, err := pg.ListTeamMembers(context.Background(), "t1"); err == nil {
		t.Error("expected scan error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — RemoveTeamMember (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_RemoveTeamMember_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE team_members SET status").
		WithArgs("m1", "t1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.RemoveTeamMember(context.Background(), "t1", "m1"); err != nil {
		t.Fatalf("RemoveTeamMember: %v", err)
	}
}

func TestPostgresDB_RemoveTeamMember_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE team_members SET status").
		WithArgs("m1", "t1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := pg.RemoveTeamMember(context.Background(), "t1", "m1"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_RemoveTeamMember_ExecError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE team_members SET status").
		WillReturnError(errors.New("exec failed"))
	if err := pg.RemoveTeamMember(context.Background(), "t1", "m1"); err == nil {
		t.Error("expected error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — DeleteSecret (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_DeleteSecret_Success(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM secrets WHERE id").
		WithArgs("s1", "t1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.DeleteSecret(context.Background(), "t1", "s1"); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
}

func TestPostgresDB_DeleteSecret_NotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM secrets WHERE id").
		WithArgs("s1", "t1").
		WillReturnResult(sqlmock.NewResult(0, 0))
	if err := pg.DeleteSecret(context.Background(), "t1", "s1"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPostgresDB_DeleteSecret_ExecError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM secrets WHERE id").
		WillReturnError(errors.New("exec failed"))
	if err := pg.DeleteSecret(context.Background(), "t1", "s1"); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_DeleteSecret_RowsAffectedError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("DELETE FROM secrets WHERE id").
		WillReturnResult(sqlmock.NewErrorResult(errors.New("rows affected error")))
	if err := pg.DeleteSecret(context.Background(), "t1", "s1"); err == nil {
		t.Error("expected error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — UpdateTOTPEnabled (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_UpdateTOTPEnabled_Enable(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE users SET totp_enabled").
		WithArgs(1, "secret-enc", "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.UpdateTOTPEnabled(context.Background(), "u1", true, "secret-enc"); err != nil {
		t.Fatalf("UpdateTOTPEnabled: %v", err)
	}
}


func TestPostgresDB_UpdateTOTPEnabled_Disable(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE users SET totp_enabled").
		WithArgs(0, "", "u1").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := pg.UpdateTOTPEnabled(context.Background(), "u1", false, ""); err != nil {
		t.Fatalf("UpdateTOTPEnabled disable: %v", err)
	}
}

func TestPostgresDB_UpdateTOTPEnabled_Error(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectExec("UPDATE users SET totp_enabled").
		WillReturnError(errors.New("update failed"))
	if err := pg.UpdateTOTPEnabled(context.Background(), "u1", true, "secret"); err == nil {
		t.Error("expected error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// PostgresDB — Rollback (0%) — various paths
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresDB_Rollback_NoMigrations(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT version, name FROM _migrations ORDER BY version DESC").
		WillReturnRows(sqlmock.NewRows([]string{"version", "name"}))

	if err := pg.Rollback(context.Background(), 1); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
}

func TestPostgresDB_Rollback_QueryError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT version, name FROM _migrations ORDER BY version DESC").
		WillReturnError(errors.New("query failed"))
	if err := pg.Rollback(context.Background(), 1); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_Rollback_StepsZeroRollsBackAll(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT version, name FROM _migrations ORDER BY version DESC").
		WillReturnRows(sqlmock.NewRows([]string{"version", "name"}).
			AddRow(2, "0002_add_indexes.pgsql.sql").
			AddRow(1, "0001_init.pgsql.sql"))

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM _migrations WHERE version").WithArgs(2).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	if err := pg.Rollback(context.Background(), 0); err == nil {
		t.Error("expected error about missing down file")
	}
}

func TestPostgresDB_Rollback_BeginTxError(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT version, name FROM _migrations ORDER BY version DESC").
		WillReturnRows(sqlmock.NewRows([]string{"version", "name"}).
			AddRow(1, "0001_init.pgsql.sql"))

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
	if err := pg.Rollback(context.Background(), 1); err == nil {
		t.Error("expected error")
	}
}

func TestPostgresDB_Rollback_DownFileNotFound(t *testing.T) {
	pg, mock := newMockPostgres(t)
	mock.ExpectQuery("SELECT version, name FROM _migrations ORDER BY version DESC").
		WillReturnRows(sqlmock.NewRows([]string{"version", "name"}).
			AddRow(1, "0001_init.pgsql.sql"))

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM _migrations WHERE version").WithArgs(1).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()
	if err := pg.Rollback(context.Background(), 1); err == nil {
		t.Error("expected error about missing down file")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// Pure functions — leaderLockKey, hostname
// ═══════════════════════════════════════════════════════════════════════════════

func TestLeaderLockKey_Stable(t *testing.T) {
	a := leaderLockKey("deploymonster:leader")
	b := leaderLockKey("deploymonster:leader")
	if a != b {
		t.Errorf("not stable: %d vs %d", a, b)
	}
	c := leaderLockKey("other-key")
	if a == c {
		t.Error("different keys should produce different hashes")
	}
}

func TestLeaderLockKey_Deterministic(t *testing.T) {
	v1 := leaderLockKey("test-key-1")
	v2 := leaderLockKey("test-key-1")
	if v1 != v2 {
		t.Errorf("determinism broken: %d vs %d", v1, v2)
	}
}

func TestHostname_ReturnsValue(t *testing.T) {
	h := hostname()
	if h == "" {
		t.Error("hostname should not be empty")
	}
	expected, err := os.Hostname()
	if err == nil && expected != "" && h != expected {
		t.Logf("hostname() = %q, os.Hostname() = %q", h, expected)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LeaderElector — NewPostgresLeaderElector (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestNewPostgresLeaderElector_Construct(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)
	if le == nil {
		t.Fatal("expected non-nil elector")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LeaderElector — Elect (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresLeaderElector_Elect_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM _leader_election WHERE key").WithArgs("test-key").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO _leader_election").WillReturnResult(sqlmock.NewResult(1, 1))
	host, _ := os.Hostname()
	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").WithArgs("test-key").
		WillReturnRows(sqlmock.NewRows([]string{"instance_id"}).AddRow(host))
	mock.ExpectExec("SELECT pg_advisory_lock").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	won, err := le.Elect(context.Background(), "test-key", 30*time.Second)
	if err != nil {
		t.Fatalf("Elect: %v", err)
	}
	if !won {
		t.Error("expected to win election")
	}
}

func TestPostgresLeaderElector_Elect_OtherInstanceWins(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM _leader_election WHERE key").WithArgs("test-key").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO _leader_election").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").WithArgs("test-key").
		WillReturnRows(sqlmock.NewRows([]string{"instance_id"}).AddRow("other-host"))
	mock.ExpectRollback()

	won, err := le.Elect(context.Background(), "test-key", 30*time.Second)
	if err != nil {
		t.Fatalf("Elect: %v", err)
	}
	if won {
		t.Error("expected not to win (other instance)")
	}
}

func TestPostgresLeaderElector_Elect_NoRowsAfterInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectBegin()
	mock.ExpectExec("DELETE FROM _leader_election WHERE key").WithArgs("test-key").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO _leader_election").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").WithArgs("test-key").
		WillReturnError(sql.ErrNoRows)
	mock.ExpectRollback()

	won, err := le.Elect(context.Background(), "test-key", 30*time.Second)
	if err != nil {
		t.Fatalf("Elect: %v", err)
	}
	if won {
		t.Error("expected not to win (no rows)")
	}
}

func TestPostgresLeaderElector_Elect_BeginError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectBegin().WillReturnError(errors.New("begin failed"))
	if _, err := le.Elect(context.Background(), "test-key", 30*time.Second); err == nil {
		t.Error("expected error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LeaderElector — Renew (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresLeaderElector_Renew_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	host, _ := os.Hostname()
	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").
		WithArgs("test-key").
		WillReturnRows(sqlmock.NewRows([]string{"instance_id"}).AddRow(host))
	mock.ExpectExec("UPDATE _leader_election SET expires_at").
		WithArgs(sqlmock.AnyArg(), "test-key", host).
		WillReturnResult(sqlmock.NewResult(0, 1))

	ok, err := le.Renew(context.Background(), "test-key", 30*time.Second)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if !ok {
		t.Error("expected renew to succeed")
	}
}

func TestPostgresLeaderElector_Renew_NotLeader(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").
		WithArgs("test-key").
		WillReturnRows(sqlmock.NewRows([]string{"instance_id"}).AddRow("other-host"))

	ok, err := le.Renew(context.Background(), "test-key", 30*time.Second)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if ok {
		t.Error("expected renew to fail (not leader)")
	}
}

func TestPostgresLeaderElector_Renew_NoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").
		WithArgs("test-key").
		WillReturnError(sql.ErrNoRows)

	ok, err := le.Renew(context.Background(), "test-key", 30*time.Second)
	if err != nil {
		t.Fatalf("Renew: %v", err)
	}
	if ok {
		t.Error("expected renew to fail (no rows)")
	}
}

func TestPostgresLeaderElector_Renew_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").
		WillReturnError(errors.New("query failed"))

	if _, err := le.Renew(context.Background(), "test-key", 30*time.Second); err == nil {
		t.Error("expected error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LeaderElector — Resign (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresLeaderElector_Resign_Success(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectExec("DELETE FROM _leader_election WHERE key").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := le.Resign(context.Background(), "test-key"); err != nil {
		t.Fatalf("Resign: %v", err)
	}
}

func TestPostgresLeaderElector_Resign_Error(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectExec("DELETE FROM _leader_election WHERE key").
		WillReturnError(errors.New("delete failed"))

	if err := le.Resign(context.Background(), "test-key"); err == nil {
		t.Error("expected error")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// LeaderElector — IsLeader (0%)
// ═══════════════════════════════════════════════════════════════════════════════

func TestPostgresLeaderElector_IsLeader_Yes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	host, _ := os.Hostname()
	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").
		WithArgs("test-key").
		WillReturnRows(sqlmock.NewRows([]string{"instance_id"}).AddRow(host))

	yes, err := le.IsLeader(context.Background(), "test-key")
	if err != nil || !yes {
		t.Fatalf("IsLeader: err=%v yes=%v", err, yes)
	}
}

func TestPostgresLeaderElector_IsLeader_No(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").
		WithArgs("test-key").
		WillReturnRows(sqlmock.NewRows([]string{"instance_id"}).AddRow("other-host"))

	yes, err := le.IsLeader(context.Background(), "test-key")
	if err != nil || yes {
		t.Fatalf("IsLeader: err=%v yes=%v (expected false)", err, yes)
	}
}

func TestPostgresLeaderElector_IsLeader_NoRows(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").
		WithArgs("test-key").
		WillReturnError(sql.ErrNoRows)

	yes, err := le.IsLeader(context.Background(), "test-key")
	if err != nil || yes {
		t.Fatalf("IsLeader: err=%v yes=%v (expected false, no error)", err, yes)
	}
}

func TestPostgresLeaderElector_IsLeader_QueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	le := NewPostgresLeaderElector(db)

	mock.ExpectQuery("SELECT instance_id FROM _leader_election WHERE key").
		WillReturnError(errors.New("query failed"))

	if _, err := le.IsLeader(context.Background(), "test-key"); err == nil {
		t.Error("expected error")
	}
}
