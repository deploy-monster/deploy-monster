//go:build integration
// +build integration

// End-to-end CRUD smoke test against a fresh file-backed SQLite database.
//
// Gated behind the `integration` build tag so it does not run during the
// default `go test ./...` pass. The GitHub Actions `test-integration`
// job runs:
//
//	go test -tags integration -run TestSQLiteIntegration ./internal/db/...
//
// against a fresh database file. SQLite needs no service container, so
// this test runs anywhere `sqlite` is available (every CI runner by
// default).
//
// The intent is to mirror the Postgres integration test end-to-end flow
// so both store implementations stay in sync — catches migration drift,
// schema changes, and transaction helper regressions without needing a
// Postgres service container.

package db

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func TestSQLiteIntegration(t *testing.T) {
	// Fresh file-backed DB per run — file-backed (not :memory:) so we
	// exercise the on-disk WAL + migration codepath, not just the in-memory
	// pragma-less path.
	dir := t.TempDir()
	path := filepath.Join(dir, "integration.db")

	s, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("NewSQLite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	// File must exist on disk after open — confirms migrate() flushed the
	// schema to the DB file rather than leaving it on an anonymous page.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db file not created: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	suffix := core.GenerateID()[:8]

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
	if err := s.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("CreateTenant: %v", err)
	}

	got, err := s.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("GetTenant: %v", err)
	}
	if got.Slug != tenant.Slug {
		t.Errorf("GetTenant slug: got %q, want %q", got.Slug, tenant.Slug)
	}

	bySlug, err := s.GetTenantBySlug(ctx, tenant.Slug)
	if err != nil {
		t.Fatalf("GetTenantBySlug: %v", err)
	}
	if bySlug.ID != tenant.ID {
		t.Errorf("GetTenantBySlug id: got %q, want %q", bySlug.ID, tenant.ID)
	}

	tenant.Name = "Integration Tenant Updated"
	if err := s.UpdateTenant(ctx, tenant); err != nil {
		t.Fatalf("UpdateTenant: %v", err)
	}
	if got, _ := s.GetTenant(ctx, tenant.ID); got.Name != tenant.Name {
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
	if err := s.CreateUser(ctx, user); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	byEmail, err := s.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if byEmail.ID != user.ID {
		t.Errorf("GetUserByEmail id: got %q, want %q", byEmail.ID, user.ID)
	}

	count, err := s.CountUsers(ctx)
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
		Description: "created by sqlite integration test",
		Environment: "production",
	}
	if err := s.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	projects, err := s.ListProjectsByTenant(ctx, tenant.ID)
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
		Type:       "service",
		SourceType: "image",
		SourceURL:  "nginx:alpine",
		Branch:     "main",
		Replicas:   1,
		Status:     "pending",
	}
	if err := s.CreateApp(ctx, app); err != nil {
		t.Fatalf("CreateApp: %v", err)
	}

	gotApp, err := s.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp: %v", err)
	}
	if gotApp.Name != app.Name {
		t.Errorf("GetApp name: got %q, want %q", gotApp.Name, app.Name)
	}

	byName, err := s.GetAppByName(ctx, tenant.ID, app.Name)
	if err != nil {
		t.Fatalf("GetAppByName: %v", err)
	}
	if byName.ID != app.ID {
		t.Errorf("GetAppByName id: got %q, want %q", byName.ID, app.ID)
	}

	apps, total, err := s.ListAppsByTenant(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAppsByTenant: %v", err)
	}
	if total != 1 || len(apps) != 1 {
		t.Errorf("ListAppsByTenant: got total=%d len=%d, want 1/1", total, len(apps))
	}

	if err := s.UpdateAppStatus(ctx, app.ID, "running"); err != nil {
		t.Fatalf("UpdateAppStatus: %v", err)
	}
	if got, _ := s.GetApp(ctx, app.ID); got.Status != "running" {
		t.Errorf("UpdateAppStatus did not persist: got %q", got.Status)
	}

	// ---- Deployment -----------------------------------------------------
	nextVer, err := s.GetNextDeployVersion(ctx, app.ID)
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
	if err := s.CreateDeployment(ctx, deployment); err != nil {
		t.Fatalf("CreateDeployment: %v", err)
	}

	latest, err := s.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetLatestDeployment: %v", err)
	}
	if latest.ID != deployment.ID {
		t.Errorf("GetLatestDeployment id: got %q, want %q", latest.ID, deployment.ID)
	}

	deployments, err := s.ListDeploymentsByApp(ctx, app.ID, 10)
	if err != nil {
		t.Fatalf("ListDeploymentsByApp: %v", err)
	}
	if len(deployments) != 1 {
		t.Errorf("ListDeploymentsByApp: got %d, want 1", len(deployments))
	}

	nextVer2, err := s.GetNextDeployVersion(ctx, app.ID)
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
	if err := s.CreateDomain(ctx, domain); err != nil {
		t.Fatalf("CreateDomain: %v", err)
	}

	byFQDN, err := s.GetDomainByFQDN(ctx, domain.FQDN)
	if err != nil {
		t.Fatalf("GetDomainByFQDN: %v", err)
	}
	if byFQDN.ID != domain.ID {
		t.Errorf("GetDomainByFQDN id: got %q, want %q", byFQDN.ID, domain.ID)
	}

	domains, err := s.ListDomainsByApp(ctx, app.ID)
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
	if err := s.CreateSecret(ctx, secret); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}

	version := &core.SecretVersion{
		ID:        "sv-" + suffix,
		SecretID:  secret.ID,
		Version:   1,
		ValueEnc:  "encrypted-blob",
		CreatedBy: user.ID,
	}
	if err := s.CreateSecretVersion(ctx, version); err != nil {
		t.Fatalf("CreateSecretVersion: %v", err)
	}

	secrets, err := s.ListSecretsByTenant(ctx, tenant.ID)
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
		UserAgent:    "sqlite-integration-test",
	}
	if err := s.CreateAuditLog(ctx, entry); err != nil {
		t.Fatalf("CreateAuditLog: %v", err)
	}

	entries, auditTotal, err := s.ListAuditLogs(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListAuditLogs: %v", err)
	}
	if auditTotal < 1 || len(entries) < 1 {
		t.Errorf("ListAuditLogs: got total=%d len=%d, want >= 1", auditTotal, len(entries))
	}

	// ---- Transaction helper: CreateTenantWithDefaults ------------------
	//
	// Exercises the Tx / commit path: creates a tenant + default project
	// atomically. If the tx layer were broken we would see either a
	// partial write (tenant but no project) or a panic.
	txTenantName := "tx-tenant-" + suffix
	txTenantSlug := "tx-tenant-" + suffix
	txTenantID, err := s.CreateTenantWithDefaults(ctx, txTenantName, txTenantSlug)
	if err != nil {
		t.Fatalf("CreateTenantWithDefaults: %v", err)
	}

	if _, err := s.GetTenant(ctx, txTenantID); err != nil {
		t.Errorf("GetTenant(tx): %v", err)
	}
	txProjects, err := s.ListProjectsByTenant(ctx, txTenantID)
	if err != nil {
		t.Fatalf("ListProjectsByTenant(tx): %v", err)
	}
	if len(txProjects) != 1 || txProjects[0].Name != "Default" {
		t.Errorf("CreateTenantWithDefaults did not create default project: got %+v", txProjects)
	}

	// ---- Transaction rollback safety -----------------------------------
	//
	// Force a unique-constraint collision on slug — the Tx helper must
	// roll back cleanly and leave no ghost row behind.
	dupErr := s.CreateTenant(ctx, &core.Tenant{
		ID:        "dup-" + suffix,
		Name:      "Duplicate",
		Slug:      txTenantSlug,
		PlanID:    "free",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	if dupErr == nil {
		t.Errorf("CreateTenant with duplicate slug: want error, got nil")
	}
	var dupCount int
	if err := s.DB().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM tenants WHERE id = ?`, "dup-"+suffix).Scan(&dupCount); err != nil {
		t.Fatalf("count dup tenant: %v", err)
	}
	if dupCount != 0 {
		t.Errorf("failed CreateTenant left ghost row: count=%d", dupCount)
	}

	// ---- Persistence across reopen -------------------------------------
	//
	// Close and reopen the database file to confirm every row written
	// above survives an OS-level flush. This is the thing :memory: tests
	// cannot catch — a migration that accidentally drops a table or a
	// pragma that forgets to fsync would surface here.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := NewSQLite(path)
	if err != nil {
		t.Fatalf("reopen NewSQLite: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })

	reopened, err := s2.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("GetApp after reopen: %v", err)
	}
	if reopened.Status != "running" {
		t.Errorf("app status not persisted across reopen: got %q", reopened.Status)
	}
	reopenedDomains, err := s2.ListDomainsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("ListDomainsByApp after reopen: %v", err)
	}
	if len(reopenedDomains) != 1 {
		t.Errorf("domains not persisted across reopen: got %d", len(reopenedDomains))
	}
}
