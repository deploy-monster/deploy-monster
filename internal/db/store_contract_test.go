//go:build integration || pgintegration
// +build integration pgintegration

// Cross-backend store contract test.
//
// This file defines runStoreContract — the single source of truth for
// "what every core.Store implementation must do end-to-end." Both the
// SQLite integration test (internal/db/sqlite_integration_test.go, tag
// `integration`) and the Postgres integration test
// (internal/db/postgres_integration_test.go, tag `pgintegration`) call
// it against a freshly-migrated store.
//
// Rule of thumb: anything that should behave identically on both
// backends lives here. Anything that is backend-specific (on-disk
// persistence for SQLite, pool stats for Postgres) stays in the
// backend-specific wrapper.
//
// The shared file is compiled under either tag so both suites link
// against the same implementation. Default `go test ./...` (no tag)
// never sees it.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// storeContractOpts carries the per-backend knobs the contract needs.
// A new backend is onboarded by filling one of these out and calling
// runStoreContract.
type storeContractOpts struct {
	// backend names the driver in test output: "sqlite" or "postgres".
	backend string

	// rawDB is the underlying *sql.DB used for escape-hatch assertions
	// (e.g. the duplicate-slug ghost-row check). Both backends expose
	// this via a DB() accessor.
	rawDB *sql.DB

	// placeholder is the parameter marker for ad-hoc raw SQL: "?" on
	// SQLite, "$1" on Postgres. Used exactly once — for the ghost-row
	// assertion below.
	placeholder string
}

// runStoreContract exercises the major core.Store surfaces end-to-end
// against a freshly-migrated store. It must stay backend-agnostic: only
// methods on core.Store plus opts.rawDB / opts.placeholder.
//
// If either backend grows a capability the other doesn't, add it to a
// backend-specific wrapper, not here.
func runStoreContract(t *testing.T, s core.Store, opts storeContractOpts) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.Ping(ctx); err != nil {
		t.Fatalf("[%s] Ping: %v", opts.backend, err)
	}

	// Unique suffix so parallel CI runs on a shared database (Postgres)
	// do not collide, and so repeated local runs stay independent.
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
	if err := s.CreateTenant(ctx, tenant); err != nil {
		t.Fatalf("[%s] CreateTenant: %v", opts.backend, err)
	}
	t.Cleanup(func() { _ = s.DeleteTenant(context.Background(), tenant.ID) })

	got, err := s.GetTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("[%s] GetTenant: %v", opts.backend, err)
	}
	if got.Slug != tenant.Slug {
		t.Errorf("[%s] GetTenant slug: got %q, want %q", opts.backend, got.Slug, tenant.Slug)
	}

	bySlug, err := s.GetTenantBySlug(ctx, tenant.Slug)
	if err != nil {
		t.Fatalf("[%s] GetTenantBySlug: %v", opts.backend, err)
	}
	if bySlug.ID != tenant.ID {
		t.Errorf("[%s] GetTenantBySlug id: got %q, want %q", opts.backend, bySlug.ID, tenant.ID)
	}

	tenant.Name = "Integration Tenant Updated"
	if err := s.UpdateTenant(ctx, tenant); err != nil {
		t.Fatalf("[%s] UpdateTenant: %v", opts.backend, err)
	}
	if got, _ := s.GetTenant(ctx, tenant.ID); got.Name != tenant.Name {
		t.Errorf("[%s] UpdateTenant did not persist name change: got %q", opts.backend, got.Name)
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
		t.Fatalf("[%s] CreateUser: %v", opts.backend, err)
	}
	t.Cleanup(func() {
		_, _ = opts.rawDB.ExecContext(context.Background(),
			"DELETE FROM users WHERE id = "+opts.placeholder, user.ID)
	})

	byEmail, err := s.GetUserByEmail(ctx, user.Email)
	if err != nil {
		t.Fatalf("[%s] GetUserByEmail: %v", opts.backend, err)
	}
	if byEmail.ID != user.ID {
		t.Errorf("[%s] GetUserByEmail id: got %q, want %q", opts.backend, byEmail.ID, user.ID)
	}

	count, err := s.CountUsers(ctx)
	if err != nil {
		t.Fatalf("[%s] CountUsers: %v", opts.backend, err)
	}
	if count < 1 {
		t.Errorf("[%s] CountUsers: got %d, want >= 1", opts.backend, count)
	}

	// ---- Project --------------------------------------------------------
	project := &core.Project{
		ID:          "proj-" + suffix,
		TenantID:    tenant.ID,
		Name:        "Integration Project",
		Description: "created by store contract test",
		Environment: "production",
	}
	if err := s.CreateProject(ctx, project); err != nil {
		t.Fatalf("[%s] CreateProject: %v", opts.backend, err)
	}

	projects, err := s.ListProjectsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("[%s] ListProjectsByTenant: %v", opts.backend, err)
	}
	if len(projects) != 1 || projects[0].ID != project.ID {
		t.Errorf("[%s] ListProjectsByTenant: got %+v, want single project %q", opts.backend, projects, project.ID)
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
		t.Fatalf("[%s] CreateApp: %v", opts.backend, err)
	}

	gotApp, err := s.GetApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("[%s] GetApp: %v", opts.backend, err)
	}
	if gotApp.Name != app.Name {
		t.Errorf("[%s] GetApp name: got %q, want %q", opts.backend, gotApp.Name, app.Name)
	}

	byName, err := s.GetAppByName(ctx, tenant.ID, app.Name)
	if err != nil {
		t.Fatalf("[%s] GetAppByName: %v", opts.backend, err)
	}
	if byName.ID != app.ID {
		t.Errorf("[%s] GetAppByName id: got %q, want %q", opts.backend, byName.ID, app.ID)
	}

	apps, total, err := s.ListAppsByTenant(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("[%s] ListAppsByTenant: %v", opts.backend, err)
	}
	if total != 1 || len(apps) != 1 {
		t.Errorf("[%s] ListAppsByTenant: got total=%d len=%d, want 1/1", opts.backend, total, len(apps))
	}

	if err := s.UpdateAppStatus(ctx, app.ID, "running"); err != nil {
		t.Fatalf("[%s] UpdateAppStatus: %v", opts.backend, err)
	}
	if got, _ := s.GetApp(ctx, app.ID); got.Status != "running" {
		t.Errorf("[%s] UpdateAppStatus did not persist: got %q", opts.backend, got.Status)
	}

	// ---- Deployment -----------------------------------------------------
	nextVer, err := s.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("[%s] GetNextDeployVersion: %v", opts.backend, err)
	}
	if nextVer != 1 {
		t.Errorf("[%s] GetNextDeployVersion on fresh app: got %d, want 1", opts.backend, nextVer)
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
		t.Fatalf("[%s] CreateDeployment: %v", opts.backend, err)
	}

	latest, err := s.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("[%s] GetLatestDeployment: %v", opts.backend, err)
	}
	if latest.ID != deployment.ID {
		t.Errorf("[%s] GetLatestDeployment id: got %q, want %q", opts.backend, latest.ID, deployment.ID)
	}

	deployments, err := s.ListDeploymentsByApp(ctx, app.ID, 10)
	if err != nil {
		t.Fatalf("[%s] ListDeploymentsByApp: %v", opts.backend, err)
	}
	if len(deployments) != 1 {
		t.Errorf("[%s] ListDeploymentsByApp: got %d, want 1", opts.backend, len(deployments))
	}

	nextVer2, err := s.GetNextDeployVersion(ctx, app.ID)
	if err != nil {
		t.Fatalf("[%s] GetNextDeployVersion post-insert: %v", opts.backend, err)
	}
	if nextVer2 != 2 {
		t.Errorf("[%s] GetNextDeployVersion post-insert: got %d, want 2", opts.backend, nextVer2)
	}

	// ---- UpdateDeployment + ListDeploymentsByStatus --------------------
	// Tier 100: exercise the new DeploymentStore methods against the
	// real backend so SQLite and Postgres are contract-checked in lockstep.
	byStatus, err := s.ListDeploymentsByStatus(ctx, "success")
	if err != nil {
		t.Fatalf("[%s] ListDeploymentsByStatus(success): %v", opts.backend, err)
	}
	foundSeeded := false
	for _, d := range byStatus {
		if d.ID == deployment.ID {
			foundSeeded = true
			break
		}
	}
	if !foundSeeded {
		t.Errorf("[%s] ListDeploymentsByStatus(success): seeded deployment %q not found", opts.backend, deployment.ID)
	}

	// Mutate the row via UpdateDeployment and verify the mutation is
	// visible on a subsequent ListDeploymentsByStatus call.
	finished := now.Add(2 * time.Minute)
	deployment.Status = "failed"
	deployment.BuildLog = "contract test: transitioned to failed"
	deployment.FinishedAt = &finished
	if err := s.UpdateDeployment(ctx, deployment); err != nil {
		t.Fatalf("[%s] UpdateDeployment: %v", opts.backend, err)
	}
	after, err := s.GetLatestDeployment(ctx, app.ID)
	if err != nil {
		t.Fatalf("[%s] GetLatestDeployment post-update: %v", opts.backend, err)
	}
	if after.Status != "failed" {
		t.Errorf("[%s] post-update status: got %q, want failed", opts.backend, after.Status)
	}
	if after.FinishedAt == nil {
		t.Errorf("[%s] post-update FinishedAt: nil", opts.backend)
	}
	// The seeded row must no longer appear in the "success" bucket.
	successAfter, _ := s.ListDeploymentsByStatus(ctx, "success")
	for _, d := range successAfter {
		if d.ID == deployment.ID {
			t.Errorf("[%s] post-update ListDeploymentsByStatus(success) still contains updated row", opts.backend)
		}
	}
	// And it must now appear in the "failed" bucket.
	failedAfter, err := s.ListDeploymentsByStatus(ctx, "failed")
	if err != nil {
		t.Fatalf("[%s] ListDeploymentsByStatus(failed): %v", opts.backend, err)
	}
	foundInFailed := false
	for _, d := range failedAfter {
		if d.ID == deployment.ID {
			foundInFailed = true
			break
		}
	}
	if !foundInFailed {
		t.Errorf("[%s] ListDeploymentsByStatus(failed): updated deployment %q not found", opts.backend, deployment.ID)
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
		t.Fatalf("[%s] CreateDomain: %v", opts.backend, err)
	}

	byFQDN, err := s.GetDomainByFQDN(ctx, domain.FQDN)
	if err != nil {
		t.Fatalf("[%s] GetDomainByFQDN: %v", opts.backend, err)
	}
	if byFQDN.ID != domain.ID {
		t.Errorf("[%s] GetDomainByFQDN id: got %q, want %q", opts.backend, byFQDN.ID, domain.ID)
	}

	domains, err := s.ListDomainsByApp(ctx, app.ID)
	if err != nil {
		t.Fatalf("[%s] ListDomainsByApp: %v", opts.backend, err)
	}
	if len(domains) != 1 {
		t.Errorf("[%s] ListDomainsByApp: got %d, want 1", opts.backend, len(domains))
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
		t.Fatalf("[%s] CreateSecret: %v", opts.backend, err)
	}

	version := &core.SecretVersion{
		ID:        "sv-" + suffix,
		SecretID:  secret.ID,
		Version:   1,
		ValueEnc:  "encrypted-blob",
		CreatedBy: user.ID,
	}
	if err := s.CreateSecretVersion(ctx, version); err != nil {
		t.Fatalf("[%s] CreateSecretVersion: %v", opts.backend, err)
	}

	secrets, err := s.ListSecretsByTenant(ctx, tenant.ID)
	if err != nil {
		t.Fatalf("[%s] ListSecretsByTenant: %v", opts.backend, err)
	}
	if len(secrets) < 1 {
		t.Errorf("[%s] ListSecretsByTenant: got %d, want >= 1", opts.backend, len(secrets))
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
		UserAgent:    opts.backend + "-store-contract",
	}
	if err := s.CreateAuditLog(ctx, entry); err != nil {
		t.Fatalf("[%s] CreateAuditLog: %v", opts.backend, err)
	}

	entries, auditTotal, err := s.ListAuditLogs(ctx, tenant.ID, 10, 0)
	if err != nil {
		t.Fatalf("[%s] ListAuditLogs: %v", opts.backend, err)
	}
	if auditTotal < 1 || len(entries) < 1 {
		t.Errorf("[%s] ListAuditLogs: got total=%d len=%d, want >= 1", opts.backend, auditTotal, len(entries))
	}

	// ---- Transaction helper: CreateTenantWithDefaults ------------------
	//
	// Exercises the Tx / Commit path: creates a tenant + default project
	// atomically. If the tx layer were broken we would see either a
	// partial write (tenant but no project) or a panic.
	txTenantName := "tx-tenant-" + suffix
	txTenantSlug := "tx-tenant-" + suffix
	txTenantID, err := s.CreateTenantWithDefaults(ctx, txTenantName, txTenantSlug)
	if err != nil {
		t.Fatalf("[%s] CreateTenantWithDefaults: %v", opts.backend, err)
	}
	t.Cleanup(func() { _ = s.DeleteTenant(context.Background(), txTenantID) })

	if _, err := s.GetTenant(ctx, txTenantID); err != nil {
		t.Errorf("[%s] GetTenant(tx): %v", opts.backend, err)
	}
	txProjects, err := s.ListProjectsByTenant(ctx, txTenantID)
	if err != nil {
		t.Fatalf("[%s] ListProjectsByTenant(tx): %v", opts.backend, err)
	}
	if len(txProjects) != 1 || txProjects[0].Name != "Default" {
		t.Errorf("[%s] CreateTenantWithDefaults did not create default project: got %+v", opts.backend, txProjects)
	}

	// ---- Transaction rollback safety -----------------------------------
	//
	// Force a unique-constraint collision on slug. The driver must
	// roll back cleanly and leave no ghost row behind.
	dupErr := s.CreateTenant(ctx, &core.Tenant{
		ID:        "dup-" + suffix,
		Name:      "Duplicate",
		Slug:      txTenantSlug, // collision
		PlanID:    "free",
		Status:    "active",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	})
	if dupErr == nil {
		t.Errorf("[%s] CreateTenant with duplicate slug: want error, got nil", opts.backend)
		_ = s.DeleteTenant(ctx, "dup-"+suffix)
	}
	var dupCount int
	if err := opts.rawDB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tenants WHERE id = "+opts.placeholder, "dup-"+suffix).Scan(&dupCount); err != nil {
		t.Fatalf("[%s] count dup tenant: %v", opts.backend, err)
	}
	if dupCount != 0 {
		t.Errorf("[%s] failed CreateTenant left ghost row: count=%d", opts.backend, dupCount)
	}

	// ---- Teardown of the non-tenant-cascaded rows ----------------------
	//
	// Tables with FK→tenants ON DELETE CASCADE (secrets, secret_versions,
	// invitations, roles, team_members) are cleaned by the t.Cleanup
	// DeleteTenant above. Tables without that cascade (applications,
	// deployments, domains, projects) need explicit cleanup so repeated
	// runs against the same Postgres database do not accumulate orphans.
	// For SQLite this is a no-op — the test DB is file-scoped and the
	// t.TempDir wrapper deletes the whole file.
	if _, err := s.DeleteDomainsByApp(ctx, app.ID); err != nil {
		t.Errorf("[%s] DeleteDomainsByApp: %v", opts.backend, err)
	}
	if _, err := opts.rawDB.ExecContext(ctx,
		"DELETE FROM deployments WHERE app_id = "+opts.placeholder, app.ID); err != nil {
		t.Errorf("[%s] DELETE deployments: %v", opts.backend, err)
	}
	if err := s.DeleteApp(ctx, app.ID); err != nil {
		t.Errorf("[%s] DeleteApp: %v", opts.backend, err)
	}
	if err := s.DeleteProject(ctx, project.ID); err != nil {
		t.Errorf("[%s] DeleteProject: %v", opts.backend, err)
	}
}
