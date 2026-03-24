package db

import (
	"context"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

func BenchmarkCreateApp(b *testing.B) {
	db := benchDB(b)
	ctx := context.Background()

	// Setup tenant and project
	tenant := &core.Tenant{Name: "Bench", Slug: "bench", Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)
	db.DB().ExecContext(ctx, "INSERT INTO projects (id, tenant_id, name) VALUES ('benchproj', ?, 'BenchProj')", tenant.ID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app := &core.Application{
			ProjectID: "benchproj", TenantID: tenant.ID, Name: "bench-app",
			Type: "service", SourceType: "image", Status: "running", Replicas: 1,
		}
		db.CreateApp(ctx, app)
	}
}

func BenchmarkGetApp(b *testing.B) {
	db := benchDB(b)
	ctx := context.Background()

	tenant := &core.Tenant{Name: "Bench", Slug: "bench2", Status: "active", PlanID: "free"}
	db.CreateTenant(ctx, tenant)
	db.DB().ExecContext(ctx, "INSERT INTO projects (id, tenant_id, name) VALUES ('benchproj2', ?, 'BenchProj')", tenant.ID)
	app := &core.Application{
		ProjectID: "benchproj2", TenantID: tenant.ID, Name: "bench-get",
		Type: "service", SourceType: "image", Status: "running", Replicas: 1,
	}
	db.CreateApp(ctx, app)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		db.GetApp(ctx, app.ID)
	}
}

func benchDB(b *testing.B) *SQLiteDB {
	b.Helper()
	dir := b.TempDir()
	db, err := NewSQLite(dir + "/bench.db")
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { db.Close() })
	return db
}
