package db

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresDB implements core.Store using PostgreSQL.
// Drop-in replacement for SQLiteDB — same interface, different backend.
type PostgresDB struct {
	db  *sql.DB
	dsn string
}

var _ core.Store = (*PostgresDB)(nil)

// NewPostgres creates a new PostgreSQL store.
// DSN format: postgres://user:pass@host:5432/dbname?sslmode=disable
// Uses github.com/jackc/pgx/v5/stdlib as the database/sql driver.
func NewPostgres(dsn string) (*PostgresDB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

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

// DB returns the underlying *sql.DB. Mirrors SQLiteDB.DB so cross-backend
// tests can touch raw SQL without reaching into unexported fields.
func (p *PostgresDB) DB() *sql.DB {
	return p.db
}

// migrate applies all pending Postgres migrations from the embedded
// migrations/*.pgsql.sql filesystem. It mirrors SQLiteDB.migrate so the
// two backends share the same versioning contract: a _migrations tracking
// table, one transaction per migration, and filename-derived version
// numbers. SQLite migrations (`*.sql`) and Postgres migrations
// (`*.pgsql.sql`) coexist in the same embed.FS; each loader skips the
// files meant for the other backend.
func (p *PostgresDB) migrate() error {
	_, err := p.db.Exec(`
		CREATE TABLE IF NOT EXISTS _migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create _migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	for _, entry := range entries {
		name := entry.Name()
		// Only consume .pgsql.sql up migrations; skip SQLite files and
		// down files. Down files for Postgres would be `.pgsql.down.sql`.
		if !strings.HasSuffix(name, ".pgsql.sql") ||
			strings.HasSuffix(name, ".pgsql.down.sql") {
			continue
		}

		var version int
		if _, err := fmt.Sscanf(name, "%04d", &version); err != nil {
			continue
		}

		var count int
		if err := p.db.QueryRow("SELECT COUNT(*) FROM _migrations WHERE version = $1", version).Scan(&count); err != nil {
			return fmt.Errorf("check migration %d: %w", version, err)
		}
		if count > 0 {
			continue
		}

		data, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := p.db.Begin()
		if err != nil {
			return fmt.Errorf("begin tx for migration %s: %w", name, err)
		}

		if _, err := tx.Exec(string(data)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}

		if _, err := tx.Exec("INSERT INTO _migrations (version, name) VALUES ($1, $2)", version, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
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
	// COALESCE the nullable TEXT/JSON columns so a freshly-created
	// tenant (e.g. CreateTenantWithDefaults, which leaves owner_id
	// unset) scans into the empty string instead of a Go scan error.
	// Tier 101: matched the ListTenants query which already did this.
	err := p.db.QueryRowContext(ctx,
		`SELECT id, name, slug, COALESCE(avatar_url,''), plan_id,
		        COALESCE(owner_id,''), status, COALESCE(limits_json,''),
		        COALESCE(metadata_json,''), created_at, updated_at
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
		`SELECT id, name, slug, COALESCE(avatar_url,''), plan_id,
		        COALESCE(owner_id,''), status, created_at, updated_at
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
// User CRUD
// =====================================================

func (p *PostgresDB) CreateUser(ctx context.Context, u *core.User) error {
	if u.ID == "" {
		u.ID = core.GenerateID()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, name, avatar_url, status)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		u.ID, u.Email, u.PasswordHash, u.Name, u.AvatarURL, u.Status,
	)
	return err
}

func (p *PostgresDB) GetUser(ctx context.Context, id string) (*core.User, error) {
	u := &core.User{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, last_login_at, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
		&u.TOTPEnabled, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return u, err
}

func (p *PostgresDB) GetUserByEmail(ctx context.Context, email string) (*core.User, error) {
	u := &core.User{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, last_login_at, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
		&u.TOTPEnabled, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return u, err
}

func (p *PostgresDB) UpdateUser(ctx context.Context, u *core.User) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE users SET email=$1, name=$2, avatar_url=$3, status=$4, updated_at=NOW() WHERE id=$5`,
		u.Email, u.Name, u.AvatarURL, u.Status, u.ID,
	)
	return err
}

func (p *PostgresDB) UpdatePassword(ctx context.Context, userID, passwordHash string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE users SET password_hash=$1, updated_at=NOW() WHERE id=$2`,
		passwordHash, userID,
	)
	return err
}

func (p *PostgresDB) UpdateLastLogin(ctx context.Context, userID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE users SET last_login_at=NOW() WHERE id=$1`, userID,
	)
	return err
}

func (p *PostgresDB) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (p *PostgresDB) CreateUserWithMembership(ctx context.Context, email, passwordHash, name, status, tenantID, roleID string) (string, error) {
	userID := core.GenerateID()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, email, password_hash, name, status) VALUES ($1, $2, $3, $4, $5)`,
		userID, email, passwordHash, name, status,
	)
	if err != nil {
		return "", err
	}

	memberID := core.GenerateID()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO team_members (id, tenant_id, user_id, role_id, status) VALUES ($1, $2, $3, $4, 'active')`,
		memberID, tenantID, userID, roleID,
	)
	if err != nil {
		return "", err
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE tenants SET owner_id = $1 WHERE id = $2`,
		userID, tenantID,
	)
	if err != nil {
		return "", err
	}

	return userID, tx.Commit()
}

// =====================================================
// App CRUD
// =====================================================

func (p *PostgresDB) CreateApp(ctx context.Context, a *core.Application) error {
	if a.ID == "" {
		a.ID = core.GenerateID()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO applications (id, project_id, tenant_id, name, type, source_type, source_url, branch, dockerfile, build_pack, env_vars_enc, labels_json, replicas, status, server_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)`,
		a.ID, a.ProjectID, a.TenantID, a.Name, a.Type, a.SourceType, a.SourceURL, a.Branch,
		a.Dockerfile, a.BuildPack, a.EnvVarsEnc, a.LabelsJSON, a.Replicas, a.Status, a.ServerID,
	)
	return err
}

func (p *PostgresDB) GetApp(ctx context.Context, id string) (*core.Application, error) {
	a := &core.Application{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        dockerfile, build_pack, env_vars_enc, labels_json, replicas, status, COALESCE(server_id,''),
		        created_at, updated_at
		 FROM applications WHERE id = $1`, id,
	).Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType, &a.SourceURL, &a.Branch,
		&a.Dockerfile, &a.BuildPack, &a.EnvVarsEnc, &a.LabelsJSON, &a.Replicas, &a.Status, &a.ServerID,
		&a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return a, err
}

func (p *PostgresDB) UpdateApp(ctx context.Context, a *core.Application) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE applications SET name=$1, source_url=$2, branch=$3, dockerfile=$4,
		 env_vars_enc=$5, labels_json=$6, replicas=$7, status=$8, updated_at=NOW() WHERE id=$9`,
		a.Name, a.SourceURL, a.Branch, a.Dockerfile,
		a.EnvVarsEnc, a.LabelsJSON, a.Replicas, a.Status, a.ID,
	)
	return err
}

func (p *PostgresDB) ListAppsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]core.Application, int, error) {
	var total int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM applications WHERE tenant_id = $1`, tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        status, replicas, created_at, updated_at
		 FROM applications WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var apps []core.Application
	for rows.Next() {
		var a core.Application
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType,
			&a.SourceURL, &a.Branch, &a.Status, &a.Replicas, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		apps = append(apps, a)
	}
	return apps, total, rows.Err()
}

func (p *PostgresDB) ListAppsByProject(ctx context.Context, projectID string) ([]core.Application, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        status, replicas, created_at, updated_at
		 FROM applications WHERE project_id = $1 ORDER BY name LIMIT 1000`,
		projectID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var apps []core.Application
	for rows.Next() {
		var a core.Application
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType,
			&a.SourceURL, &a.Branch, &a.Status, &a.Replicas, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (p *PostgresDB) UpdateAppStatus(ctx context.Context, id, status string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE applications SET status=$1, updated_at=NOW() WHERE id=$2`,
		status, id,
	)
	return err
}

func (p *PostgresDB) GetAppByName(ctx context.Context, tenantID, name string) (*core.Application, error) {
	a := &core.Application{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        dockerfile, build_pack, env_vars_enc, labels_json, replicas, status, COALESCE(server_id,''),
		        created_at, updated_at
		 FROM applications WHERE tenant_id = $1 AND name = $2`, tenantID, name,
	).Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType, &a.SourceURL, &a.Branch,
		&a.Dockerfile, &a.BuildPack, &a.EnvVarsEnc, &a.LabelsJSON, &a.Replicas, &a.Status, &a.ServerID,
		&a.CreatedAt, &a.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return a, err
}

func (p *PostgresDB) DeleteApp(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM applications WHERE id = $1`, id)
	return err
}

// =====================================================
// Deployment CRUD
// =====================================================

func (p *PostgresDB) CreateDeployment(ctx context.Context, d *core.Deployment) error {
	if d.ID == "" {
		d.ID = core.GenerateID()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO deployments (id, app_id, version, image, container_id, status, build_log, commit_sha, commit_message, triggered_by, strategy, started_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
		d.ID, d.AppID, d.Version, d.Image, d.ContainerID, d.Status, d.BuildLog,
		d.CommitSHA, d.CommitMessage, d.TriggeredBy, d.Strategy, d.StartedAt,
	)
	return err
}

// UpdateDeployment persists a mutation to an existing deployment row. See
// the SQLiteDB implementation for the Tier 100 rationale.
func (p *PostgresDB) UpdateDeployment(ctx context.Context, d *core.Deployment) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE deployments
		 SET status = $1, container_id = $2, build_log = $3, finished_at = $4
		 WHERE id = $5`,
		d.Status, d.ContainerID, d.BuildLog, d.FinishedAt, d.ID,
	)
	return err
}

// ListDeploymentsByStatus returns every deployment in the given status,
// newest first. Used by deploy.Module.Start to reclaim in-flight deployments
// left behind by a crashed master (Phase 3.1.2 restart storm).
func (p *PostgresDB) ListDeploymentsByStatus(ctx context.Context, status string) ([]core.Deployment, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, app_id, version, image, container_id, status, commit_sha, commit_message,
		        triggered_by, strategy, started_at, finished_at, created_at
		 FROM deployments WHERE status = $1 ORDER BY created_at DESC`, status,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []core.Deployment
	for rows.Next() {
		var d core.Deployment
		if err := rows.Scan(&d.ID, &d.AppID, &d.Version, &d.Image, &d.ContainerID, &d.Status,
			&d.CommitSHA, &d.CommitMessage, &d.TriggeredBy, &d.Strategy,
			&d.StartedAt, &d.FinishedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

func (p *PostgresDB) GetLatestDeployment(ctx context.Context, appID string) (*core.Deployment, error) {
	d := &core.Deployment{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, app_id, version, image, container_id, status, commit_sha, commit_message,
		        triggered_by, strategy, started_at, finished_at, created_at
		 FROM deployments WHERE app_id = $1 ORDER BY version DESC LIMIT 1`, appID,
	).Scan(&d.ID, &d.AppID, &d.Version, &d.Image, &d.ContainerID, &d.Status,
		&d.CommitSHA, &d.CommitMessage, &d.TriggeredBy, &d.Strategy,
		&d.StartedAt, &d.FinishedAt, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return d, err
}

func (p *PostgresDB) ListDeploymentsByApp(ctx context.Context, appID string, limit int) ([]core.Deployment, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, app_id, version, image, container_id, status, commit_sha, commit_message,
		        triggered_by, strategy, started_at, finished_at, created_at
		 FROM deployments WHERE app_id = $1 ORDER BY version DESC LIMIT $2`,
		appID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []core.Deployment
	for rows.Next() {
		var d core.Deployment
		if err := rows.Scan(&d.ID, &d.AppID, &d.Version, &d.Image, &d.ContainerID, &d.Status,
			&d.CommitSHA, &d.CommitMessage, &d.TriggeredBy, &d.Strategy,
			&d.StartedAt, &d.FinishedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}
	return deployments, rows.Err()
}

func (p *PostgresDB) GetNextDeployVersion(ctx context.Context, appID string) (int, error) {
	var maxVersion sql.NullInt64
	err := p.db.QueryRowContext(ctx,
		`SELECT MAX(version) FROM deployments WHERE app_id = $1`, appID,
	).Scan(&maxVersion)
	if err != nil {
		return 1, err
	}
	if !maxVersion.Valid {
		return 1, nil
	}
	return int(maxVersion.Int64) + 1, nil
}

// AtomicNextDeployVersion atomically allocates the next deployment version using
// PostgreSQL advisory locks for distributed locking.
// SECURITY FIX (RACE-002): Uses advisory lock to prevent race conditions.
func (p *PostgresDB) AtomicNextDeployVersion(ctx context.Context, appID string) (int, error) {
	// Generate a lock ID from appID (hashed to int64 range)
	lockID := hashAppIDToLockID(appID)

	// Use a transaction with advisory lock
	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Acquire advisory lock (exclusive)
	_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID)
	if err != nil {
		return 0, err
	}
	// Lock is automatically released at transaction end

	var maxVersion sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`SELECT MAX(version) FROM deployments WHERE app_id = $1`, appID,
	).Scan(&maxVersion)
	if err != nil {
		return 0, err
	}

	nextVersion := 1
	if maxVersion.Valid {
		nextVersion = int(maxVersion.Int64) + 1
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}

	return nextVersion, nil
}

// hashAppIDToLockID generates a stable int64 lock ID from an app ID string.
func hashAppIDToLockID(appID string) int64 {
	h := 0
	for i := 0; i < len(appID); i++ {
		h = 31*h + int(appID[i])
	}
	return int64(h & 0x7FFFFFFFFFFFFFFF) // Ensure positive
}

// =====================================================
// Domain CRUD
// =====================================================

func (p *PostgresDB) CreateDomain(ctx context.Context, d *core.Domain) error {
	if d.ID == "" {
		d.ID = core.GenerateID()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO domains (id, app_id, fqdn, type, dns_provider, dns_synced, verified)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		d.ID, d.AppID, d.FQDN, d.Type, d.DNSProvider, d.DNSSynced, d.Verified,
	)
	return err
}

func (p *PostgresDB) GetDomainByFQDN(ctx context.Context, fqdn string) (*core.Domain, error) {
	d := &core.Domain{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE fqdn = $1`, fqdn,
	).Scan(&d.ID, &d.AppID, &d.FQDN, &d.Type, &d.DNSProvider, &d.DNSSynced, &d.Verified, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return d, err
}

func (p *PostgresDB) GetDomain(ctx context.Context, id string) (*core.Domain, error) {
	d := &core.Domain{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE id = $1`, id,
	).Scan(&d.ID, &d.AppID, &d.FQDN, &d.Type, &d.DNSProvider, &d.DNSSynced, &d.Verified, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return d, err
}

func (p *PostgresDB) ListDomainsByApp(ctx context.Context, appID string) ([]core.Domain, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE app_id = $1 ORDER BY created_at LIMIT 500`,
		appID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []core.Domain
	for rows.Next() {
		var d core.Domain
		if err := rows.Scan(&d.ID, &d.AppID, &d.FQDN, &d.Type, &d.DNSProvider,
			&d.DNSSynced, &d.Verified, &d.CreatedAt); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

func (p *PostgresDB) DeleteDomain(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM domains WHERE id = $1`, id)
	return err
}

func (p *PostgresDB) DeleteDomainsByApp(ctx context.Context, appID string) (int, error) {
	result, err := p.db.ExecContext(ctx, `DELETE FROM domains WHERE app_id = $1`, appID)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

func (p *PostgresDB) ListAllDomains(ctx context.Context) ([]core.Domain, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains ORDER BY created_at LIMIT 10000`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var domains []core.Domain
	for rows.Next() {
		var d core.Domain
		if err := rows.Scan(&d.ID, &d.AppID, &d.FQDN, &d.Type, &d.DNSProvider,
			&d.DNSSynced, &d.Verified, &d.CreatedAt); err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// =====================================================
// Project CRUD
// =====================================================

func (p *PostgresDB) CreateProject(ctx context.Context, proj *core.Project) error {
	if proj.ID == "" {
		proj.ID = core.GenerateID()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO projects (id, tenant_id, name, description, environment)
		 VALUES ($1, $2, $3, $4, $5)`,
		proj.ID, proj.TenantID, proj.Name, proj.Description, proj.Environment,
	)
	return err
}

func (p *PostgresDB) GetProject(ctx context.Context, id string) (*core.Project, error) {
	proj := &core.Project{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, description, environment, created_at, updated_at
		 FROM projects WHERE id = $1`, id,
	).Scan(&proj.ID, &proj.TenantID, &proj.Name, &proj.Description, &proj.Environment, &proj.CreatedAt, &proj.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return proj, err
}

func (p *PostgresDB) ListProjectsByTenant(ctx context.Context, tenantID string) ([]core.Project, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, tenant_id, name, description, environment, created_at, updated_at
		 FROM projects WHERE tenant_id = $1 ORDER BY name LIMIT 1000`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []core.Project
	for rows.Next() {
		var proj core.Project
		if err := rows.Scan(&proj.ID, &proj.TenantID, &proj.Name, &proj.Description,
			&proj.Environment, &proj.CreatedAt, &proj.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, proj)
	}
	return projects, rows.Err()
}

func (p *PostgresDB) DeleteProject(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, id)
	return err
}

func (p *PostgresDB) CreateTenantWithDefaults(ctx context.Context, name, slug string) (string, error) {
	tenantID := core.GenerateID()
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO tenants (id, name, slug, status) VALUES ($1, $2, $3, 'active')`,
		tenantID, name, slug,
	)
	if err != nil {
		return "", err
	}

	projectID := core.GenerateID()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO projects (id, tenant_id, name, description, environment)
		 VALUES ($1, $2, 'Default', 'Default project', 'production')`,
		projectID, tenantID,
	)
	if err != nil {
		return "", err
	}

	return tenantID, tx.Commit()
}

// =====================================================
// Role + TeamMember queries
// =====================================================

func (p *PostgresDB) GetRole(ctx context.Context, roleID string) (*core.Role, error) {
	r := &core.Role{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), name, description, permissions_json, is_builtin, created_at
		 FROM roles WHERE id = $1`, roleID,
	).Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.PermissionsJSON, &r.IsBuiltin, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return r, err
}

func (p *PostgresDB) GetUserMembership(ctx context.Context, userID string) (*core.TeamMember, error) {
	tm := &core.TeamMember{}
	err := p.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, user_id, role_id, status, created_at
		 FROM team_members WHERE user_id = $1 AND status = 'active' LIMIT 1`, userID,
	).Scan(&tm.ID, &tm.TenantID, &tm.UserID, &tm.RoleID, &tm.Status, &tm.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return tm, err
}

func (p *PostgresDB) ListRoles(ctx context.Context, tenantID string) ([]core.Role, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), name, description, permissions_json, is_builtin, created_at
		 FROM roles WHERE tenant_id = $1 OR is_builtin = TRUE ORDER BY is_builtin DESC, name LIMIT 500`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []core.Role
	for rows.Next() {
		var r core.Role
		if err := rows.Scan(&r.ID, &r.TenantID, &r.Name, &r.Description,
			&r.PermissionsJSON, &r.IsBuiltin, &r.CreatedAt); err != nil {
			return nil, err
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// =====================================================
// Audit Log
// =====================================================

func (p *PostgresDB) CreateAuditLog(ctx context.Context, entry *core.AuditEntry) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO audit_log (tenant_id, user_id, action, resource_type, resource_id, details_json, ip_address, user_agent)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		entry.TenantID, entry.UserID, entry.Action, entry.ResourceType,
		entry.ResourceID, entry.DetailsJSON, entry.IPAddress, entry.UserAgent,
	)
	return err
}

func (p *PostgresDB) ListAuditLogs(ctx context.Context, tenantID string, limit, offset int) ([]core.AuditEntry, int, error) {
	var total int
	err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE tenant_id = $1`, tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, tenant_id, user_id, action, resource_type, resource_id, details_json,
		        ip_address, user_agent, created_at
		 FROM audit_log WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []core.AuditEntry
	for rows.Next() {
		var e core.AuditEntry
		if err := rows.Scan(&e.ID, &e.TenantID, &e.UserID, &e.Action, &e.ResourceType,
			&e.ResourceID, &e.DetailsJSON, &e.IPAddress, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	return entries, total, rows.Err()
}

// =====================================================
// Secret CRUD
// =====================================================

func (p *PostgresDB) CreateSecret(ctx context.Context, secret *core.Secret) error {
	if secret.ID == "" {
		secret.ID = core.GenerateID()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO secrets (id, tenant_id, project_id, app_id, name, type, description, scope, current_version)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		secret.ID, nullIfEmpty(secret.TenantID), nullIfEmpty(secret.ProjectID), nullIfEmpty(secret.AppID),
		secret.Name, secret.Type, secret.Description, secret.Scope, secret.CurrentVersion,
	)
	return err
}

func (p *PostgresDB) CreateSecretVersion(ctx context.Context, version *core.SecretVersion) error {
	if version.ID == "" {
		version.ID = core.GenerateID()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO secret_versions (id, secret_id, version, value_enc, created_by)
		 VALUES ($1, $2, $3, $4, $5)`,
		version.ID, version.SecretID, version.Version, version.ValueEnc, nullIfEmpty(version.CreatedBy),
	)
	return err
}

func (p *PostgresDB) ListSecretsByTenant(ctx context.Context, tenantID string) ([]core.Secret, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), COALESCE(project_id,''), COALESCE(app_id,''),
		        name, type, description, scope, current_version, created_at, updated_at
		 FROM secrets WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 1000`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []core.Secret
	for rows.Next() {
		var s core.Secret
		if err := rows.Scan(&s.ID, &s.TenantID, &s.ProjectID, &s.AppID,
			&s.Name, &s.Type, &s.Description, &s.Scope, &s.CurrentVersion,
			&s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

// =====================================================
// Invite CRUD
// =====================================================

func (p *PostgresDB) CreateInvite(ctx context.Context, invite *core.Invitation) error {
	if invite.ID == "" {
		invite.ID = core.GenerateID()
	}
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO invitations (id, tenant_id, email, role_id, invited_by, token_hash, expires_at, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		invite.ID, invite.TenantID, invite.Email, invite.RoleID,
		invite.InvitedBy, invite.TokenHash, invite.ExpiresAt, invite.Status,
	)
	return err
}

func (p *PostgresDB) ListInvitesByTenant(ctx context.Context, tenantID string) ([]core.Invitation, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, tenant_id, email, role_id, COALESCE(invited_by,''), token_hash,
		        expires_at, accepted_at, status, created_at
		 FROM invitations WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT 1000`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var invites []core.Invitation
	for rows.Next() {
		var inv core.Invitation
		if err := rows.Scan(&inv.ID, &inv.TenantID, &inv.Email, &inv.RoleID,
			&inv.InvitedBy, &inv.TokenHash, &inv.ExpiresAt, &inv.AcceptedAt,
			&inv.Status, &inv.CreatedAt); err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}
	return invites, rows.Err()
}

func (p *PostgresDB) ListAllTenants(ctx context.Context, limit, offset int) ([]core.Tenant, int, error) {
	var total int
	if err := p.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM tenants`).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := p.db.QueryContext(ctx,
		`SELECT id, name, slug, avatar_url, plan_id, COALESCE(owner_id,''),
		        status, limits_json, metadata_json, created_at, updated_at
		 FROM tenants ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var tenants []core.Tenant
	for rows.Next() {
		var t core.Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.Slug, &t.AvatarURL, &t.PlanID, &t.OwnerID,
			&t.Status, &t.LimitsJSON, &t.MetadataJSON, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		tenants = append(tenants, t)
	}
	return tenants, total, rows.Err()
}

// =====================================================
// Secret Lookup Methods
// =====================================================

// GetSecretByScopeAndName returns a secret by its scope and name.
func (p *PostgresDB) GetSecretByScopeAndName(ctx context.Context, scope, name string) (*core.Secret, error) {
	var secret core.Secret
	err := p.db.QueryRowContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), COALESCE(project_id,''), COALESCE(app_id,''),
		        name, type, description, scope, current_version, created_at, updated_at
		 FROM secrets WHERE scope = $1 AND name = $2`, scope, name,
	).Scan(&secret.ID, &secret.TenantID, &secret.ProjectID, &secret.AppID,
		&secret.Name, &secret.Type, &secret.Description, &secret.Scope, &secret.CurrentVersion,
		&secret.CreatedAt, &secret.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("secret %s/%s not found", scope, name)
	}
	if err != nil {
		return nil, err
	}
	return &secret, nil
}

// GetLatestSecretVersion returns the most recent version of a secret.
func (p *PostgresDB) GetLatestSecretVersion(ctx context.Context, secretID string) (*core.SecretVersion, error) {
	var version core.SecretVersion
	err := p.db.QueryRowContext(ctx,
		`SELECT id, secret_id, version, value_enc, created_by, created_at
		 FROM secret_versions WHERE secret_id = $1 ORDER BY version DESC LIMIT 1`, secretID,
	).Scan(&version.ID, &version.SecretID, &version.Version, &version.ValueEnc,
		&version.CreatedBy, &version.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no versions found for secret %s", secretID)
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}

// ListAllSecretVersions returns every secret version row (for key rotation).
func (p *PostgresDB) ListAllSecretVersions(ctx context.Context) ([]core.SecretVersion, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, secret_id, version, value_enc, created_by, created_at
		 FROM secret_versions ORDER BY secret_id, version LIMIT 10000`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []core.SecretVersion
	for rows.Next() {
		var v core.SecretVersion
		if err := rows.Scan(&v.ID, &v.SecretID, &v.Version, &v.ValueEnc, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// UpdateSecretVersionValue updates the encrypted value of a secret version (for key rotation).
func (p *PostgresDB) UpdateSecretVersionValue(ctx context.Context, id, valueEnc string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE secret_versions SET value_enc = $1 WHERE id = $2`, valueEnc, id,
	)
	return err
}

// CreateUsageRecord inserts a new usage record.
func (p *PostgresDB) CreateUsageRecord(ctx context.Context, record *core.UsageRecord) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO usage_records (tenant_id, app_id, metric_type, value, hour_bucket, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		record.TenantID, record.AppID, record.MetricType, record.Value, record.HourBucket.UTC(), time.Now().UTC())
	return err
}

// ListUsageRecordsByTenant lists usage records for a tenant with pagination.
func (p *PostgresDB) ListUsageRecordsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]core.UsageRecord, int, error) {
	var total int
	if err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM usage_records WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, tenant_id, app_id, metric_type, value, hour_bucket, created_at
		 FROM usage_records
		 WHERE tenant_id = $1
		 ORDER BY hour_bucket DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var records []core.UsageRecord
	for rows.Next() {
		var r core.UsageRecord
		if err := rows.Scan(&r.ID, &r.TenantID, &r.AppID, &r.MetricType, &r.Value, &r.HourBucket, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		records = append(records, r)
	}
	return records, total, rows.Err()
}

// CreateBackup inserts a new backup record.
func (p *PostgresDB) CreateBackup(ctx context.Context, backup *core.Backup) error {
	_, err := p.db.ExecContext(ctx,
		`INSERT INTO backups (id, tenant_id, source_type, source_id, storage_target, file_path, size_bytes, encryption, status, scheduled, retention_days, started_at, completed_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		backup.ID, backup.TenantID, backup.SourceType, backup.SourceID, backup.StorageTarget,
		backup.FilePath, backup.SizeBytes, backup.Encryption, backup.Status, backup.Scheduled,
		backup.RetentionDays, nil, nil, time.Now().UTC())
	return err
}

// ListBackupsByTenant lists backups for a tenant with pagination.
func (p *PostgresDB) ListBackupsByTenant(ctx context.Context, tenantID string, limit, offset int) ([]core.Backup, int, error) {
	var total int
	if err := p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM backups WHERE tenant_id = $1`, tenantID).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, tenant_id, source_type, source_id, storage_target, file_path, size_bytes, encryption, status, scheduled, retention_days, started_at, completed_at, created_at
		 FROM backups
		 WHERE tenant_id = $1
		 ORDER BY created_at DESC
		 LIMIT $2 OFFSET $3`,
		tenantID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var backups []core.Backup
	for rows.Next() {
		var b core.Backup
		var startedAt, completedAt *time.Time
		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.SourceType, &b.SourceID, &b.StorageTarget,
			&b.FilePath, &b.SizeBytes, &b.Encryption, &b.Status, &b.Scheduled,
			&b.RetentionDays, &startedAt, &completedAt, &b.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		b.StartedAt = startedAt
		b.CompletedAt = completedAt
		backups = append(backups, b)
	}
	return backups, total, rows.Err()
}

// UpdateBackupStatus updates a backup's status and size.
func (p *PostgresDB) UpdateBackupStatus(ctx context.Context, id, status string, sizeBytes int64) error {
	var completedAt *time.Time
	if status == "completed" || status == "failed" {
		now := time.Now().UTC()
		completedAt = &now
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE backups SET status = $1, size_bytes = $2, completed_at = $3 WHERE id = $4`,
		status, sizeBytes, completedAt, id)
	return err
}
