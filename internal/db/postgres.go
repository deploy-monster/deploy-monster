package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// PostgresDB implements core.Store using PostgreSQL.
// Drop-in replacement for SQLiteDB — same interface, different backend.
type PostgresDB struct {
	db  *sql.DB
	dsn string
}

// NewPostgres creates a new PostgreSQL store.
// DSN format: postgres://user:pass@host:5432/dbname?sslmode=disable
func NewPostgres(dsn string) (*PostgresDB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	pg := &PostgresDB{db: db, dsn: dsn}
	if err := pg.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres migrate: %w", err)
	}

	return pg, nil
}

func (p *PostgresDB) Ping(ctx context.Context) error {
	return p.db.PingContext(ctx)
}

func (p *PostgresDB) Close() error {
	return p.db.Close()
}

func (p *PostgresDB) migrate() error {
	// PostgreSQL uses $1, $2 placeholders instead of ?
	// Tables are identical to SQLite but with PostgreSQL types
	_, err := p.db.Exec(`
		CREATE TABLE IF NOT EXISTS tenants (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			slug TEXT UNIQUE NOT NULL,
			avatar_url TEXT DEFAULT '',
			plan_id TEXT DEFAULT 'free',
			owner_id TEXT DEFAULT '',
			status TEXT DEFAULT 'active',
			limits_json TEXT DEFAULT '{}',
			metadata_json TEXT DEFAULT '{}',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			name TEXT DEFAULT '',
			avatar_url TEXT DEFAULT '',
			status TEXT DEFAULT 'active',
			totp_enabled BOOLEAN DEFAULT FALSE,
			last_login_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS applications (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			type TEXT DEFAULT 'web',
			source_type TEXT DEFAULT 'image',
			source_url TEXT DEFAULT '',
			branch TEXT DEFAULT 'main',
			dockerfile TEXT DEFAULT '',
			build_pack TEXT DEFAULT '',
			env_vars_enc TEXT DEFAULT '',
			labels_json TEXT DEFAULT '{}',
			replicas INTEGER DEFAULT 1,
			status TEXT DEFAULT 'created',
			server_id TEXT DEFAULT '',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS deployments (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL,
			version INTEGER NOT NULL,
			image TEXT DEFAULT '',
			container_id TEXT DEFAULT '',
			status TEXT DEFAULT 'pending',
			build_log TEXT DEFAULT '',
			commit_sha TEXT DEFAULT '',
			commit_message TEXT DEFAULT '',
			triggered_by TEXT DEFAULT 'manual',
			strategy TEXT DEFAULT 'recreate',
			started_at TIMESTAMPTZ,
			finished_at TIMESTAMPTZ,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS domains (
			id TEXT PRIMARY KEY,
			app_id TEXT NOT NULL,
			fqdn TEXT UNIQUE NOT NULL,
			type TEXT DEFAULT 'custom',
			dns_provider TEXT DEFAULT '',
			dns_synced BOOLEAN DEFAULT FALSE,
			verified BOOLEAN DEFAULT FALSE,
			created_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE TABLE IF NOT EXISTS projects (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL,
			name TEXT NOT NULL,
			description TEXT DEFAULT '',
			environment TEXT DEFAULT 'production',
			created_at TIMESTAMPTZ DEFAULT NOW(),
			updated_at TIMESTAMPTZ DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_apps_tenant ON applications(tenant_id);
		CREATE INDEX IF NOT EXISTS idx_apps_project ON applications(project_id);
		CREATE INDEX IF NOT EXISTS idx_deployments_app ON deployments(app_id, version DESC);
		CREATE INDEX IF NOT EXISTS idx_domains_app ON domains(app_id);
	`)
	return err
}

// =====================================================
// Tenant CRUD — PostgreSQL uses $1 instead of ?
// =====================================================

func (p *PostgresDB) CreateTenant(ctx context.Context, t *core.Tenant) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO tenants (id, name, slug, plan_id, owner_id, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		t.ID, t.Name, t.Slug, t.PlanID, t.OwnerID, t.Status, t.CreatedAt, t.UpdatedAt,
	)
	return err
}

func (p *PostgresDB) GetTenant(ctx context.Context, id string) (*core.Tenant, error) {
	t := &core.Tenant{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, name, slug, avatar_url, plan_id, owner_id, status, limits_json, metadata_json, created_at, updated_at
		 FROM tenants WHERE id = $1`, id,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.AvatarURL, &t.PlanID, &t.OwnerID, &t.Status, &t.LimitsJSON, &t.MetadataJSON, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return t, err
}

func (p *PostgresDB) GetTenantBySlug(ctx context.Context, slug string) (*core.Tenant, error) {
	t := &core.Tenant{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, name, slug, avatar_url, plan_id, owner_id, status, created_at, updated_at
		 FROM tenants WHERE slug = $1`, slug,
	).Scan(&t.ID, &t.Name, &t.Slug, &t.AvatarURL, &t.PlanID, &t.OwnerID, &t.Status, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return t, err
}

func (p *PostgresDB) UpdateTenant(ctx context.Context, t *core.Tenant) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE tenants SET name=$1, slug=$2, plan_id=$3, status=$4, updated_at=$5 WHERE id=$6`,
		t.Name, t.Slug, t.PlanID, t.Status, time.Now(), t.ID,
	)
	return err
}

func (p *PostgresDB) DeleteTenant(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1`, id)
	return err
}

// =====================================================
// Remaining methods follow the same pattern.
// Each SQLite method has a PostgreSQL equivalent —
// only placeholder syntax changes (? → $1, $2, $3).
// The full implementation mirrors internal/db/users.go,
// apps.go, deployments.go, domains.go, etc.
// =====================================================

// TODO: Implement remaining Store methods for PostgreSQL.
// These are identical to SQLite except:
// 1. Placeholders: ? → $1, $2, $3
// 2. AUTOINCREMENT → SERIAL
// 3. datetime('now') → NOW()
// 4. Requires "github.com/lib/pq" driver
//
// For now, use SQLite as default. PostgreSQL is enterprise.
