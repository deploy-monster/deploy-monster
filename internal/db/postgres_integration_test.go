//go:build pgintegration
// +build pgintegration

// Package db postgres_integration_test.go
//
// End-to-end CRUD smoke test against a real PostgreSQL instance.
//
// Gated behind the `pgintegration` build tag so it does not run during the
// default `go test ./...` pass (which has no database). The GitHub Actions
// `test-postgres` job starts a `postgres:16` service container and runs:
//
//	go test -tags pgintegration -run TestPostgresIntegration ./internal/db/...
//
// with TEST_POSTGRES_DSN pointed at the service. If the env var is unset
// the test simply skips — that makes it safe to run the tag locally on a
// machine without Postgres.
//
// The test exercises the major store surfaces end-to-end: tenants, users,
// projects, applications, deployments, domains, and secrets — enough to
// catch any `$1` placeholder typos, type-coercion bugs, or schema drift
// between the SQLite and PostgreSQL implementations.

package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestPostgresIntegration(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set; skipping real-Postgres integration test")
	}

	pg, err := NewPostgres(dsn)
	if err != nil {
		t.Fatalf("NewPostgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := pg.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	// Unique suffix so parallel CI runs on the same DB do not collide.
	suffix := core.GenerateID()

	// ---- Tenant ---------------------------------------------------------
	tenant := &core.Tenant{
		ID:        "tenant-" + suffix,
		Name:      "Integration Tenant " + suffix,
		Slug:      "integration-" + suffix,
		PlanID:    "free",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := pg.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}
	t.Cleanup(func() { _ = pg.DeleteTenant(context.Background(), tenant.ID) })

	got, err := pg.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if got.Slug != tenant.Slug {
		t.Errorf("GetTenant slug: got %q, want %q", got.Slug, tenant.Slug)
	}

	bySlug, err := pg.GetTenantBySlug(ctx, tenant.Slug)
	if err != nil {
		t.Fatalf("GetTenantBySlug: %v", err)
	}
	if bySlug.ID != tenant.ID {
		t.Errorf("GetTenantBySlug id: got %q, want %q", bySlug.ID, tenant.ID)
	}

	tenant.Name = "Integration Tenant Updated"
	if err := pg.UpdateTenant(ctx, tenant); err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}
	if got, _ := pg.GetTenant(ctx, tenant.ID); got.Name != tenant.Name {
		t.Errorf("UpdateTenant did not persist name change: got %q", got.Name)
	}

	// ---- User -----------------------------------------------------------
	user := &core.User{
		ID:           "user-" + suffix,
		Email:        fmt.Sprintf("integration-%s@test.local", suffix),
		PasswordHash: "$2a$10$fakehashforintegrationtest",
		Name:         "Integration User",
		Status:       "active",
	}
	if err := pg.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	byEmail, err := pg.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if byEmail.ID != user.ID {
		t.Errorf("GetUserByEmail id: got %q, want %q", byEmail.ID, user.ID)
	}

	count, err := pg.CountUsers(ctx)
	if err != nil {
		t.Fatalf("CountUsers: %v", err)
	}
	if count < 1 {
		t.Errorf("CountUsers: got %d, want >= 1", count)
	}

	// ---- Project --------------------------------------------------------
	project := &core.Project{
		ID:          "proj-" + suffix,
		TenantID:    tenant.ID,
		Name:        "Integration Project",
		Description: "created by postgres integration test",
		Environment: "production",
	}
	if err := pg.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	projects, err := pg.ListProjectsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListProjectsByTenant: %v", err)
	}
	if len(projects) != 1 || projects[0].ID != project.ID {
		t.Errorf("ListProjectsByTenant: got %+v, want single project %q", projects, project.ID)
	}

	// ---- Application ----------------------------------------------------
	app := &core.Application{
		ID:         "app-" + suffix,
		ProjectID:  project.ID,
		TenantID:   tenant.ID,
		Name:       "integration-app",
		Type:       "web",
		SourceType: "image",
		SourceURL:  "nginx:alpine",
		Branch:     "main",
		Replicas:   1,
		Status:     "created",
	}
	if err := pg.CreateApp(ctx, app); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	gotApp, err := pg.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if gotApp.Name != app.Name {
		t.Errorf("GetApp name: got %q, want %q", gotApp.Name, app.Name)
	}

	byName, err := pg.GetAppByName(ctx, tenant.ID, app.Name)
	if err != nil {
		t.Fatalf("GetAppByName: %v", err)
	}
	if byName.ID != app.ID {
		t.Errorf("GetAppByName id: got %q, want %q", byName.ID, app.ID)
	}

	apps, total, err := pg.ListAppsByTenant(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant: %v", err)
	}
	if total != 1 || len(apps) != 1 {
		t.Errorf("ListAppsByTenant: got total=%d len=%d, want 1/1", total, len(apps))
	}

	if err := pg.UpdateAppStatus(ctx, app.ID, "running"); err != nil {
		t.Fatalf("UpdateAppStatus: %v", err)
	}
	if got, _ := pg.GetApp(ctx, app.ID); got.Status != "running" {
		t.Errorf("UpdateAppStatus did not persist: got %q", got.Status)
	}

	// ---- Deployment -----------------------------------------------------
	nextVer, err := pg.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetNextDeployVersion: %v", err)
	}
	if nextVer != 1 {
		t.Errorf("GetNextDeployVersion on fresh app: got %d, want 1", nextVer)
	}

	now := time.Now()
	deployment := &core.Deployment{
		ID:          "deploy-" + suffix,
		AppID:       app.ID,
		Version:     nextVer,
		Image:       "nginx:alpine",
		Status:      "success",
		TriggeredBy: "manual",
		Strategy:    "recreate",
		StartedAt:   &now,
	}
	if err := pg.CreateDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	latest, err := pg.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if latest.ID != deployment.ID {
		t.Errorf("GetLatestDeployment id: got %q, want %q", latest.ID, deployment.ID)
	}

	deployments, err := pg.ListDeploymentsByApp(ctx, app.ID, 10)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp: %v", err)
	}
	if len(deployments) != 1 {
		t.Errorf("ListDeploymentsByApp: got %d, want 1", len(deployments))
	}

	nextVer2, err := pg.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetNextDeployVersion post-insert: %v", err)
	}
	if nextVer2 != 2 {
		t.Errorf("GetNextDeployVersion post-insert: got %d, want 2", nextVer2)
	}

	// ---- Domain ---------------------------------------------------------
	domain := &core.Domain{
		ID:        "dom-" + suffix,
		AppID:     app.ID,
		FQDN:      fmt.Sprintf("%s.integration.test", suffix),
		Type:      "custom",
		DNSSynced: false,
		Verified:  false,
	}
	if err := pg.CreateDomain(ctx, domain); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	byFQDN, err := pg.GetDomainByFQDN(ctx, domain.FQDN)
	if err != nil {
		t.Fatalf("GetDomainByFQDN: %v", err)
	}
	if byFQDN.ID != domain.ID {
		t.Errorf("GetDomainByFQDN id: got %q, want %q", byFQDN.ID, domain.ID)
	}

	domains, err := pg.ListDomainsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListDomainsByApp: %v", err)
	}
	if len(domains) != 1 {
		t.Errorf("ListDomainsByApp: got %d, want 1", len(domains))
	}

	// ---- Secret ---------------------------------------------------------
	secret := &core.Secret{
		ID:             "secret-" + suffix,
		TenantID:       tenant.ID,
		AppID:          app.ID,
		Name:           "DB_PASSWORD",
		Type:           "env",
		Scope:          "app",
		CurrentVersion: 1,
	}
	if err := pg.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	version := &core.SecretVersion{
		ID:        "sv-" + suffix,
		SecretID:  secret.ID,
		Version:   1,
		ValueEnc:  "encrypted-blob",
		CreatedBy: user.ID,
	}
	if err := pg.CreateSecretVersion(ctx, version); err != nil {
		t.Fatalf("CreateSecretVersion: %v", err)
	}

	secrets, err := pg.ListSecretsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("ListSecretsByTenant: %v", err)
	}
	if len(secrets) < 1 {
		t.Errorf("ListSecretsByTenant: got %d, want >= 1", len(secrets))
	}

	// ---- Audit ----------------------------------------------------------
	entry := &core.AuditEntry{
		TenantID:     tenant.ID,
		UserID:       user.ID,
		Action:       "app.created",
		ResourceType: "app",
		ResourceID:   app.ID,
		DetailsJSON:  `{"integration":true}`,
		IPAddress:    "127.0.0.1",
		UserAgent:    "postgres-integration-test",
	}
	if err := pg.CreateAuditLog(ctx, entry); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}

	entries, auditTotal, err := pg.ListAuditLogs(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if auditTotal < 1 || len(entries) < 1 {
		t.Errorf("ListAuditLogs: got total=%d len=%d, want >= 1", auditTotal, len(entries))
	}

	// ---- Transaction helper: CreateTenantWithDefaults ------------------
	//
	// Exercises the tx.BeginTx / tx.Commit path: creates a tenant + default
	// project atomically. If the tx layer were broken we would see either a
	// partial write (tenant but no project) or a panic. On success both rows
	// must be queryable through the normal Get / List methods.
	txTenantName := "tx-tenant-" + suffix
	txTenantSlug := "tx-tenant-" + suffix
	txTenantID, err := pg.CreateTenantWithDefaults(ctx, txTenantName, txTenantSlug)
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pg.db.ExecContext(context.Background(),
			`DELETE FROM projects WHERE tenant_id = $1`, txTenantID)
		_ = pg.DeleteTenant(context.Background(), txTenantID)
	})

	txTenant, err := pg.GetTenant(ctx, txTenantID)
	if err != nil {
		t.Fatalf("GetTenant(tx): %v", err)
	}
	if txTenant.Slug != txTenantSlug {
		t.Errorf("CreateTenantWithDefaults slug: got %q, want %q", txTenant.Slug, txTenantSlug)
	}
	txProjects, err := pg.ListProjectsByTenant(ctx, txTenantID)
	if err != nil {
		t.Fatalf("ListProjectsByTenant(tx): %v", err)
	}
	if len(txProjects) != 1 || txProjects[0].Name != "Default" {
		t.Errorf("CreateTenantWithDefaults did not create default project: got %+v", txProjects)
	}

	// ---- Transaction rollback safety -----------------------------------
	//
	// Force a unique-constraint collision on slug inside a fresh
	// CreateTenant call — the driver must rollback cleanly and leave no
	// ghost row behind. We verify by asserting the tenant count by slug is
	// unchanged.
	dupErr := pg.CreateTenant(ctx, &core.Tenant{
		ID:        "dup-" + suffix,
		Name:      "Duplicate",
		Slug:      txTenantSlug, // collision
		PlanID:    "free",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	if dupErr == nil {
		t.Errorf("CreateTenant with duplicate slug: want error, got nil")
		_ = pg.DeleteTenant(ctx, "dup-"+suffix)
	}
	var dupCount int
	if err := pg.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tenants WHERE id = $1`, "dup-"+suffix).Scan(&dupCount); err != nil {
		t.Fatalf("count dup tenant: %v", err)
	}
	if dupCount != 0 {
		t.Errorf("failed CreateTenant left ghost row: count=%d", dupCount)
	}

	// ---- Connection pool validation ------------------------------------
	//
	// NewPostgres sets MaxOpenConns=25, MaxIdleConns=5. Confirm those stuck
	// through driver initialization — if the pool config is dropped we lose
	// concurrency headroom silently.
	stats := pg.db.Stats()
	if stats.MaxOpenConnections != 25 {
		t.Errorf("pool MaxOpenConnections: got %d, want 25", stats.MaxOpenConnections)
	}

	// ---- Teardown (reverse dependency order) ----------------------------
	//
	// Tables with FK→tenants ON DELETE CASCADE (team_members, roles, secrets,
	// secret_versions, invitations) are cleaned by the t.Cleanup DeleteTenant
	// above. Tables without FK (users, applications, deployments, domains,
	// projects, audit_log) need explicit cleanup so repeated runs against the
	// same database do not accumulate orphans.
	if _, err := pg.DeleteDomainsByApp(ctx, app.ID); err != nil {
		t.Errorf("DeleteDomainsByApp: %v", err)
	}
	if _, err := pg.db.ExecContext(ctx, `DELETE FROM deployments WHERE app_id = $1`, app.ID); err != nil {
		t.Errorf("DELETE deployments: %v", err)
	}
	if err := pg.DeleteApp(ctx, app.ID); err != nil {
		t.Errorf("DeleteApp: %v", err)
	}
	if err := pg.DeleteProject(ctx, project.ID); err != nil {
		t.Errorf("DeleteProject: %v", err)
	}
	if _, err := pg.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, user.ID); err != nil {
		t.Errorf("DELETE users: %v", err)
	}
	// Leave audit_log rows in place — SERIAL PK + unique suffix mean no
	// collisions; cleanup is out of scope for an append-only log.
}
