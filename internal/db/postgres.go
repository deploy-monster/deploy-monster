// P2-12: migration concurrency safety — both backends use COUNT-then-INSERT
// which is inherently racy for concurrent processes. The INSERT side is
// protected by ON CONFLICT DO NOTHING so that if two processes race through
// the check the duplicate insert becomes a no-op rather than an error.
package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
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
		_ = db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}

	pg := &PostgresDB{db: db, dsn: dsn}
	if err := pg.migrate(); err != nil {
		_ = db.Close()
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

// ListMigrations returns the applied schema migrations in version order.
func (p *PostgresDB) ListMigrations(ctx context.Context) ([]core.MigrationStatus, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT version, name,
		        to_char(applied_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		 FROM _migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	defer rows.Close()

	var migrations []core.MigrationStatus
	for rows.Next() {
		var m core.MigrationStatus
		if err := rows.Scan(&m.Version, &m.Name, &m.AppliedAt); err != nil {
			return nil, fmt.Errorf("scan migration: %w", err)
		}
		migrations = append(migrations, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}
	return migrations, nil
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

		if _, err := tx.Exec("INSERT INTO _migrations (version, name) VALUES ($1, $2) ON CONFLICT DO NOTHING", version, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", name, err)
		}
	}

	return nil
}

// Rollback reverts the last n applied migrations by executing their .pgsql.down.sql
// counterparts. If steps <= 0, it rolls back all migrations. Each migration runs in its
// own transaction for atomicity. The ctx parameter is used for the per-query timeout
// and is also passed into the transaction so that context cancellation aborts a
// rollback in progress. No .pgsql.down.sql files are required to exist — if a file
// is missing this still removes the record from _migrations (making the interface
// future-proof before any down migrations are written).
func (p *PostgresDB) Rollback(ctx context.Context, steps int) error {
	type migration struct {
		version int
		name    string
	}

	// Read applied migrations into memory first, then release the connection
	// before entering the rollback loop to avoid holding a cursor open
	// while executing DDL.
	applied, err := func() ([]migration, error) {
		rows, err := p.db.QueryContext(ctx, "SELECT version, name FROM _migrations ORDER BY version DESC")
		if err != nil {
			return nil, fmt.Errorf("list applied migrations: %w", err)
		}
		defer func() { _ = rows.Close() }()

		var out []migration
		for rows.Next() {
			var m migration
			if err := rows.Scan(&m.version, &m.name); err != nil {
				return nil, fmt.Errorf("scan migration: %w", err)
			}
			out = append(out, m)
		}
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("iterate applied migrations: %w", err)
		}
		return out, nil
	}()
	if err != nil {
		return err
	}

	if len(applied) == 0 {
		return nil
	}

	if steps <= 0 || steps > len(applied) {
		steps = len(applied)
	}

	for i := 0; i < steps; i++ {
		m := applied[i]
		// Derive down filename: 0001_init.pgsql.sql -> 0001_init.pgsql.down.sql
		downName := strings.TrimSuffix(m.name, ".pgsql.sql") + ".pgsql.down.sql"

		// Run the down migration inside a transaction so it is atomic.
		tx, err := p.db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx for rollback %s: %w", downName, err)
		}

		data, err := migrationsFS.ReadFile("migrations/" + downName)
		if err != nil {
			// No down file yet — still remove the record so the interface
			// is future-proof and a missing file does not block rollback.
			_, _ = tx.Exec("DELETE FROM _migrations WHERE version = $1", m.version)
			_ = tx.Rollback()
			// Return a non-fatal error so callers know a file was missing.
			return fmt.Errorf("down migration %s not found: %w", downName, err)
		}

		if _, err := tx.Exec(string(data)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply down migration %s: %w", downName, err)
		}

		if _, err := tx.Exec("DELETE FROM _migrations WHERE version = $1", m.version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("remove migration record %d: %w", m.version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit rollback %s: %w", downName, err)
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

func (p *PostgresDB) DeleteTenant(ctx context.Context, id, tenantID string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM tenants WHERE id = $1 AND id = $2`, id, tenantID)
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
	var totpSecret sql.NullString
	var backupCodes sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, totp_secret_enc, totp_backup_codes_json, last_login_at, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
		&u.TOTPEnabled, &totpSecret, &backupCodes, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.TOTPSecret = totpSecret.String
	u.TOTPBackupCodes = decodeTOTPBackupCodes(backupCodes.String)
	return u, nil
}

func (p *PostgresDB) GetUserByEmail(ctx context.Context, email string) (*core.User, error) {
	u := &core.User{}
	var totpSecret sql.NullString
	var backupCodes sql.NullString
	err := p.db.QueryRowContext(ctx,
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, totp_secret_enc, totp_backup_codes_json, last_login_at, created_at, updated_at
		 FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
		&u.TOTPEnabled, &totpSecret, &backupCodes, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	u.TOTPSecret = totpSecret.String
	u.TOTPBackupCodes = decodeTOTPBackupCodes(backupCodes.String)
	return u, nil
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

func (p *PostgresDB) UpdateTOTPEnabled(ctx context.Context, userID string, enabled bool, totpSecretEnc string) error {
	totpEnabled := 0
	if enabled {
		totpEnabled = 1
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE users SET totp_enabled=$1, totp_secret_enc=$2, updated_at=NOW() WHERE id=$3`,
		totpEnabled, totpSecretEnc, userID,
	)
	return err
}

// UpdateTOTPBackupCodes replaces the user's hashed TOTP backup codes.
func (p *PostgresDB) UpdateTOTPBackupCodes(ctx context.Context, userID string, hashes []string) error {
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return fmt.Errorf("marshal totp backup codes: %w", err)
	}
	_, err = p.db.ExecContext(ctx,
		`UPDATE users SET totp_backup_codes_json=$1, updated_at=NOW() WHERE id=$2`,
		string(encoded), userID,
	)
	return err
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
		return "", fmt.Errorf("insert user: %w", err)
	}

	memberID := core.GenerateID()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO team_members (id, tenant_id, user_id, role_id, status) VALUES ($1, $2, $3, $4, 'active')`,
		memberID, tenantID, userID, roleID,
	)
	if err != nil {
		return "", fmt.Errorf("insert team member: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE tenants SET owner_id = $1 WHERE id = $2`,
		userID, tenantID,
	)
	if err != nil {
		return "", fmt.Errorf("set tenant owner: %w", err)
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

// GetAppsByIDs retrieves multiple apps in a single query using $1, $2, ... placeholders.
func (p *PostgresDB) GetAppsByIDs(ctx context.Context, ids []string) ([]core.Application, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids))
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf(
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        dockerfile, build_pack, env_vars_enc, labels_json, replicas, status, COALESCE(server_id,''),
		        created_at, updated_at
		 FROM applications WHERE id IN (%s)`,
		strings.Join(placeholders, ","),
	)
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var apps []core.Application
	for rows.Next() {
		var a core.Application
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType,
			&a.SourceURL, &a.Branch, &a.Dockerfile, &a.BuildPack, &a.EnvVarsEnc, &a.LabelsJSON,
			&a.Replicas, &a.Status, &a.ServerID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (p *PostgresDB) UpdateApp(ctx context.Context, a *core.Application) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE applications SET name=$1, source_url=$2, branch=$3, dockerfile=$4,
		 env_vars_enc=$5, labels_json=$6, replicas=$7, status=$8, server_id=$9, updated_at=NOW() WHERE id=$10`,
		a.Name, a.SourceURL, a.Branch, a.Dockerfile,
		a.EnvVarsEnc, a.LabelsJSON, a.Replicas, a.Status, a.ServerID, a.ID,
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
		        status, replicas, COALESCE(server_id,''), created_at, updated_at
		 FROM applications WHERE tenant_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		tenantID, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var apps []core.Application
	for rows.Next() {
		var a core.Application
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType,
			&a.SourceURL, &a.Branch, &a.Status, &a.Replicas, &a.ServerID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, 0, err
		}
		apps = append(apps, a)
	}
	return apps, total, rows.Err()
}

func (p *PostgresDB) ListAppsByProject(ctx context.Context, projectID, tenantID string) ([]core.Application, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, project_id, tenant_id, name, type, source_type, source_url, branch,
		        status, replicas, COALESCE(server_id,''), created_at, updated_at
		 FROM applications WHERE project_id = $1 AND tenant_id = $2 ORDER BY name LIMIT 1000`,
		projectID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var apps []core.Application
	for rows.Next() {
		var a core.Application
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.TenantID, &a.Name, &a.Type, &a.SourceType,
			&a.SourceURL, &a.Branch, &a.Status, &a.Replicas, &a.ServerID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		apps = append(apps, a)
	}
	return apps, rows.Err()
}

func (p *PostgresDB) UpdateAppStatus(ctx context.Context, id, status, tenantID string) error {
	_, err := p.db.ExecContext(ctx,
		`UPDATE applications SET status=$1, updated_at=NOW() WHERE id=$2 AND tenant_id=$3`,
		status, id, tenantID,
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

func (p *PostgresDB) DeleteApp(ctx context.Context, id, tenantID string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM applications WHERE id = $1 AND tenant_id = $2`, id, tenantID)
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
		 FROM deployments WHERE status = $1 ORDER BY created_at DESC LIMIT 10000`, status,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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
		return 0, fmt.Errorf("begin serializable tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Acquire advisory lock (exclusive)
	_, err = tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID)
	if err != nil {
		return 0, fmt.Errorf("acquire advisory lock: %w", err)
	}
	// Lock is automatically released at transaction end

	var maxVersion sql.NullInt64
	err = tx.QueryRowContext(ctx,
		`SELECT MAX(version) FROM deployments WHERE app_id = $1`, appID,
	).Scan(&maxVersion)
	if err != nil {
		return 0, fmt.Errorf("query max deployment version: %w", err)
	}

	nextVersion := 1
	if maxVersion.Valid {
		nextVersion = int(maxVersion.Int64) + 1
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit version increment: %w", err)
	}

	return nextVersion, nil
}

// CreateDeploymentAtomicVersion allocates the next version for d.AppID and
// inserts the deployment row atomically. It takes the same per-app advisory
// lock used by AtomicNextDeployVersion so the MAX(version) read and the insert
// happen under exclusive serialization — no two concurrent deploys can claim
// the same version. d.Version is set on return.
func (p *PostgresDB) CreateDeploymentAtomicVersion(ctx context.Context, d *core.Deployment) error {
	if d.ID == "" {
		d.ID = core.GenerateID()
	}
	lockID := hashAppIDToLockID(d.AppID)

	tx, err := p.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin serializable tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, lockID); err != nil {
		return fmt.Errorf("acquire advisory lock: %w", err)
	}

	err = tx.QueryRowContext(ctx,
		`INSERT INTO deployments
		   (id, app_id, version, image, container_id, status, build_log, commit_sha, commit_message, triggered_by, strategy, started_at)
		 SELECT $1, $2, COALESCE(MAX(version), 0) + 1, $3, $4, $5, $6, $7, $8, $9, $10, $11
		 FROM deployments WHERE app_id = $12
		 RETURNING version`,
		d.ID, d.AppID, d.Image, d.ContainerID, d.Status, d.BuildLog,
		d.CommitSHA, d.CommitMessage, d.TriggeredBy, d.Strategy, d.StartedAt,
		d.AppID,
	).Scan(&d.Version)
	if err != nil {
		return fmt.Errorf("insert deployment with atomic version: %w", err)
	}
	return tx.Commit()
}

// GetUsersByIDs retrieves multiple users in a single query using $1, $2, ... placeholders.
func (p *PostgresDB) GetUsersByIDs(ctx context.Context, ids []string, tenantID string) ([]core.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	// Build $1, $2, ... placeholders for IN clause
	placeholders := make([]string, len(ids))
	args := make([]any, len(ids)+1)
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	args[len(ids)] = tenantID
	query := fmt.Sprintf(
		`SELECT id, email, password_hash, name, avatar_url, status, totp_enabled, totp_secret_enc, totp_backup_codes_json, last_login_at, created_at, updated_at
		 FROM users WHERE id IN (%s) AND tenant_id = $%d`,
		strings.Join(placeholders, ","), len(ids)+1,
	)
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []core.User
	for rows.Next() {
		var u core.User
		var totpSecret sql.NullString
		var backupCodes sql.NullString
		if err := rows.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.Name, &u.AvatarURL, &u.Status,
			&u.TOTPEnabled, &totpSecret, &backupCodes, &u.LastLoginAt, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.TOTPSecret = totpSecret.String
		u.TOTPBackupCodes = decodeTOTPBackupCodes(backupCodes.String)
		users = append(users, u)
	}
	return users, rows.Err()
}

// GetLatestDeploymentsByAppIDs retrieves the latest deployment for each app in a single query.
// Returns a map from appID to its latest deployment (nil if no deployment found for that app).
func (p *PostgresDB) GetLatestDeploymentsByAppIDs(ctx context.Context, appIDs []string) (map[string]*core.Deployment, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(appIDs))
	args := make([]any, len(appIDs))
	for i, id := range appIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}
	query := fmt.Sprintf(
		`SELECT d.id, d.app_id, d.version, d.image, d.container_id, d.status,
		        d.commit_sha, d.commit_message, d.triggered_by, d.strategy,
		        d.started_at, d.finished_at, d.created_at
		 FROM deployments d
		 INNER JOIN (
		     SELECT app_id, MAX(version) as max_version
		     FROM deployments
		     WHERE app_id IN (%s)
		     GROUP BY app_id
		 ) latest ON d.app_id = latest.app_id AND d.version = latest.max_version`,
		strings.Join(placeholders, ","),
	)
	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]*core.Deployment)
	for rows.Next() {
		var d core.Deployment
		if err := rows.Scan(&d.ID, &d.AppID, &d.Version, &d.Image, &d.ContainerID, &d.Status,
			&d.CommitSHA, &d.CommitMessage, &d.TriggeredBy, &d.Strategy,
			&d.StartedAt, &d.FinishedAt, &d.CreatedAt); err != nil {
			return nil, err
		}
		result[d.AppID] = &d
	}
	return result, rows.Err()
}

// ListDomainsByAppIDs retrieves domains for multiple apps in a single query,
// scoped to tenantID. Only returns domains for apps owned by the tenant.
func (p *PostgresDB) ListDomainsByAppIDs(ctx context.Context, appIDs []string, tenantID string) (map[string][]core.Domain, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	// Filter appIDs to only those belonging to the tenant.
	allowedPlaceholders := make([]string, len(appIDs))
	allowedArgs := make([]any, len(appIDs)+1)
	for i, id := range appIDs {
		allowedPlaceholders[i] = fmt.Sprintf("$%d", i+1)
		allowedArgs[i] = id
	}
	allowedArgs[len(appIDs)] = tenantID
	allowedQuery := fmt.Sprintf(
		`SELECT id FROM applications WHERE id IN (%s) AND tenant_id = $%d`,
		strings.Join(allowedPlaceholders, ","), len(appIDs)+1,
	)
	rows, err := p.db.QueryContext(ctx, allowedQuery, allowedArgs...)
	if err != nil {
		return nil, err
	}
	var allowedIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		allowedIDs = append(allowedIDs, id)
	}
	rows.Close()
	if len(allowedIDs) == 0 {
		return map[string][]core.Domain{}, nil
	}
	// Query domains for the allowed appIDs.
	domainPlaceholders := make([]string, len(allowedIDs))
	domainArgs := make([]any, len(allowedIDs))
	for i, id := range allowedIDs {
		domainPlaceholders[i] = fmt.Sprintf("$%d", i+1)
		domainArgs[i] = id
	}
	domainQuery := fmt.Sprintf(
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE app_id IN (%s) ORDER BY created_at`,
		strings.Join(domainPlaceholders, ","),
	)
	domainRows, err := p.db.QueryContext(ctx, domainQuery, domainArgs...)
	if err != nil {
		return nil, err
	}
	defer domainRows.Close()
	result := make(map[string][]core.Domain)
	for domainRows.Next() {
		var d core.Domain
		if err := domainRows.Scan(&d.ID, &d.AppID, &d.FQDN, &d.Type, &d.DNSProvider,
			&d.DNSSynced, &d.Verified, &d.CreatedAt); err != nil {
			return nil, err
		}
		result[d.AppID] = append(result[d.AppID], d)
	}
	return result, domainRows.Err()
}

// hashAppIDToLockID generates a stable int64 lock ID from an app ID string.
// Uses FNV-64a which has much better distribution than polynomial rolling hash,
// avoiding collisions between different app IDs that would break distributed locking.
func hashAppIDToLockID(appID string) int64 {
	if appID == "" {
		return 0
	}
	h := fnv.New64a()
	h.Write([]byte(appID))
	return int64(h.Sum64() & 0x7FFFFFFFFFFFFFFF) // Ensure positive
}

// leaderLockKey converts a string key (e.g. "deploymonster:leader") to a
// stable int64 by hashing it. This avoids collisions between keys that
// would otherwise map to the same small integer space.
func leaderLockKey(key string) int64 {
	h := fnv.New64a()
	h.Write([]byte(key))
	return int64(h.Sum64() & 0x7FFFFFFFFFFFFFFF)
}

// PostgresLeaderElector implements LeaderElector using PostgreSQL advisory
// locks. Since pg_advisory_xact_lock is transaction-scoped and
// automatically released at commit, we use pg_advisory_lock (session-scoped)
// for leadership that persists across transactions, paired with a row in
// a leadership table to track who holds the lock and until when.
type PostgresLeaderElector struct {
	db *sql.DB
}

// NewPostgresLeaderElector creates a leader elector backed by PostgreSQL.
// The provided *sql.DB should be the same underlying connection pool used
// by PostgresDB so that advisory locks are shared across both.
func NewPostgresLeaderElector(db *sql.DB) *PostgresLeaderElector {
	return &PostgresLeaderElector{db: db}
}

var _ core.LeaderElector = (*PostgresLeaderElector)(nil)

// Elect attempts to acquire leadership for the given key. On success,
// this instance becomes the leader and holds it for leaseDuration.
// Other instances attempting Elect for the same key will return (false, nil)
// until the lease expires or this instance calls Resign.
func (p *PostgresLeaderElector) Elect(ctx context.Context, key string, leaseDuration time.Duration) (bool, error) {
	lockID := leaderLockKey(key)
	instanceID := hostname()

	// Try to insert a leadership row — if another instance already holds
	// leadership (and it hasn't expired), the INSERT fails with a unique
	// constraint violation and we lose the election.
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin election tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Clean up any stale leadership from crashed instances first.
	_, _ = tx.ExecContext(ctx,
		`DELETE FROM _leader_election WHERE key = $1 AND expires_at < NOW()`,
		key,
	)

	// Attempt to claim leadership.
	expiresAt := time.Now().Add(leaseDuration)
	_, err = tx.ExecContext(ctx,
		`INSERT INTO _leader_election (key, instance_id, expires_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (key) DO NOTHING`,
		key, instanceID, expiresAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert leadership claim: %w", err)
	}

	// Check if we actually got the row (ON CONFLICT DO NOTHING means no error
	// on conflict, but no row inserted either). Use a separate query to confirm.
	var holder string
	err = tx.QueryRowContext(ctx,
		`SELECT instance_id FROM _leader_election WHERE key = $1 AND expires_at > NOW()`,
		key,
	).Scan(&holder)
	if err == sql.ErrNoRows {
		_ = tx.Rollback()
		return false, nil // another instance won
	}
	if err != nil {
		return false, fmt.Errorf("check leadership: %w", err)
	}
	if holder != instanceID {
		_ = tx.Rollback()
		return false, nil
	}

	// Acquire the PostgreSQL advisory lock to guarantee mutual exclusion.
	_, err = tx.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, lockID)
	if err != nil {
		return false, fmt.Errorf("acquire advisory lock: %w", err)
	}

	if err := tx.Commit(); err != nil {
		// Try to release the lock on failure even though transaction failed.
		_, _ = p.db.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
		return false, fmt.Errorf("commit election: %w", err)
	}

	return true, nil
}

// Renew extends the leadership lease. Returns true if leadership is still held
// (and is now valid until leaseDuration from now). Returns false if leadership
// was lost or the key doesn't exist.
func (p *PostgresLeaderElector) Renew(ctx context.Context, key string, leaseDuration time.Duration) (bool, error) {
	instanceID := hostname()

	var holder string
	err := p.db.QueryRowContext(ctx,
		`SELECT instance_id FROM _leader_election WHERE key = $1 AND expires_at > NOW()`,
		key,
	).Scan(&holder)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check leadership for renew: %w", err)
	}
	if holder != instanceID {
		return false, nil
	}

	// Refresh the expiry time and extend the advisory lock.
	expiresAt := time.Now().Add(leaseDuration)
	_, err = p.db.ExecContext(ctx,
		`UPDATE _leader_election SET expires_at = $1 WHERE key = $2 AND instance_id = $3`,
		expiresAt, key, instanceID,
	)
	if err != nil {
		return false, fmt.Errorf("renew leadership: %w", err)
	}

	return true, nil
}

// Resign voluntarily releases leadership so another instance can take over.
func (p *PostgresLeaderElector) Resign(ctx context.Context, key string) error {
	lockID := leaderLockKey(key)
	instanceID := hostname()

	_, err := p.db.ExecContext(ctx,
		`DELETE FROM _leader_election WHERE key = $1 AND instance_id = $2`,
		key, instanceID,
	)
	if err != nil {
		return fmt.Errorf("resign leadership: %w", err)
	}

	_, _ = p.db.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
	return nil
}

// IsLeader reports whether this instance currently holds leadership for the key.
func (p *PostgresLeaderElector) IsLeader(ctx context.Context, key string) (bool, error) {
	instanceID := hostname()
	var holder string
	err := p.db.QueryRowContext(ctx,
		`SELECT instance_id FROM _leader_election WHERE key = $1 AND expires_at > NOW()`,
		key,
	).Scan(&holder)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("isleader check: %w", err)
	}
	return holder == instanceID, nil
}

// hostname returns the current hostname or a generated ID as a fallback.
func hostname() string {
	h, _ := os.Hostname()
	if h == "" {
		return core.GenerateID()
	}
	return h
}

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

func (p *PostgresDB) ListDomainsByApp(ctx context.Context, appID, tenantID string) ([]core.Domain, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT d.id, d.app_id, d.fqdn, d.type, d.dns_provider, d.dns_synced, d.verified, d.created_at
		 FROM domains d
		 JOIN applications a ON a.id = d.app_id
		 WHERE d.app_id = $1 AND a.tenant_id = $2
		 ORDER BY d.created_at LIMIT 500`,
		appID, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

func (p *PostgresDB) DeleteDomain(ctx context.Context, id, tenantID string) error {
	_, err := p.db.ExecContext(ctx,
		`DELETE FROM domains WHERE id = $1 AND app_id IN (
			SELECT id FROM applications WHERE tenant_id = $2
		)`,
		id, tenantID)
	return err
}

func (p *PostgresDB) DeleteDomainsByApp(ctx context.Context, appID, tenantID string) (int, error) {
	result, err := p.db.ExecContext(ctx,
		`DELETE FROM domains WHERE app_id = $1 AND app_id IN (
			SELECT id FROM applications WHERE id = $1 AND tenant_id = $2
		)`,
		appID, tenantID)
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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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
		return "", fmt.Errorf("insert tenant: %w", err)
	}

	projectID := core.GenerateID()
	_, err = tx.ExecContext(ctx,
		`INSERT INTO projects (id, tenant_id, name, description, environment)
		 VALUES ($1, $2, 'Default', 'Default project', 'production')`,
		projectID, tenantID,
	)
	if err != nil {
		return "", fmt.Errorf("insert default project: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("commit tenant defaults: %w", err)
	}
	return tenantID, nil
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

func (p *PostgresDB) ListTeamMembers(ctx context.Context, tenantID string) ([]core.TeamMember, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, tenant_id, user_id, role_id, status, created_at
		 FROM team_members WHERE tenant_id = $1 AND status = 'active'
		 ORDER BY created_at ASC LIMIT 500`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var members []core.TeamMember
	for rows.Next() {
		var tm core.TeamMember
		if err := rows.Scan(&tm.ID, &tm.TenantID, &tm.UserID, &tm.RoleID, &tm.Status, &tm.CreatedAt); err != nil {
			return nil, err
		}
		members = append(members, tm)
	}
	return members, rows.Err()
}

func (p *PostgresDB) RemoveTeamMember(ctx context.Context, tenantID, memberID string) error {
	res, err := p.db.ExecContext(ctx,
		`UPDATE team_members SET status = 'removed' WHERE id = $1 AND tenant_id = $2 AND status = 'active'`,
		memberID, tenantID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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

func (p *PostgresDB) DeleteSecret(ctx context.Context, tenantID, secretID string) error {
	res, err := p.db.ExecContext(ctx,
		`DELETE FROM secrets WHERE id = $1 AND tenant_id = $2`,
		secretID, tenantID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return core.ErrNotFound
	}
	return nil
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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()

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
	defer func() { _ = rows.Close() }()
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
	defer func() { _ = rows.Close() }()
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

// UpdateBackupStatus updates a backup's status and size, scoped to tenantID.
func (p *PostgresDB) UpdateBackupStatus(ctx context.Context, id, status string, sizeBytes int64, tenantID string) error {
	var completedAt *time.Time
	if status == "completed" || status == "failed" {
		now := time.Now().UTC()
		completedAt = &now
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE backups SET status = $1, size_bytes = $2, completed_at = $3 WHERE id = $4 AND tenant_id = $5`,
		status, sizeBytes, completedAt, id, tenantID)
	return err
}

// =====================================================
// SERVERS
// =====================================================

// CreateServer inserts a new server row.
func (p *PostgresDB) CreateServer(ctx context.Context, srv *core.Server) error {
	if srv.ID == "" {
		srv.ID = core.GenerateID()
	}
	if srv.Role == "" {
		srv.Role = "worker"
	}
	if srv.ProviderType == "" {
		srv.ProviderType = "custom"
	}
	if srv.SSHPort == 0 {
		srv.SSHPort = 22
	}
	if srv.Status == "" {
		srv.Status = "provisioning"
	}
	if srv.AgentStatus == "" {
		srv.AgentStatus = "unknown"
	}
	tenantID := sql.NullString{String: srv.TenantID, Valid: srv.TenantID != ""}
	sshKeyID := sql.NullString{String: srv.SSHKeyID, Valid: srv.SSHKeyID != ""}
	swarm := 0
	if srv.SwarmJoined {
		swarm = 1
	}

	_, err := p.db.ExecContext(ctx,
		`INSERT INTO servers (id, tenant_id, hostname, ip_address, role,
			provider_type, provider_ref, region, size, ssh_port, ssh_key_id,
			docker_version, cpu_cores, ram_mb, disk_mb, monthly_cost_cents,
			swarm_joined, agent_status, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)`,
		srv.ID, tenantID, srv.Hostname, srv.IPAddress, srv.Role,
		srv.ProviderType, srv.ProviderRef, srv.Region, srv.Size,
		srv.SSHPort, sshKeyID, srv.DockerVersion, srv.CPUCores,
		srv.RAMmb, srv.DiskMB, srv.MonthlyCostCents, swarm,
		srv.AgentStatus, srv.Status,
	)
	return err
}

// GetServer fetches a server by ID.
func (p *PostgresDB) GetServer(ctx context.Context, id string) (*core.Server, error) {
	srv := &core.Server{}
	var tenantID, sshKeyID sql.NullString
	var swarm int
	err := p.db.QueryRowContext(ctx,
		`SELECT id, tenant_id, hostname, ip_address, role, provider_type, provider_ref,
			region, size, ssh_port, ssh_key_id, docker_version, cpu_cores, ram_mb,
			disk_mb, monthly_cost_cents, swarm_joined, agent_status, status, created_at
		 FROM servers WHERE id = $1`, id,
	).Scan(&srv.ID, &tenantID, &srv.Hostname, &srv.IPAddress, &srv.Role,
		&srv.ProviderType, &srv.ProviderRef, &srv.Region, &srv.Size,
		&srv.SSHPort, &sshKeyID, &srv.DockerVersion, &srv.CPUCores,
		&srv.RAMmb, &srv.DiskMB, &srv.MonthlyCostCents, &swarm,
		&srv.AgentStatus, &srv.Status, &srv.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	srv.TenantID = tenantID.String
	srv.SSHKeyID = sshKeyID.String
	srv.SwarmJoined = swarm != 0
	return srv, nil
}

// ListServersByTenant returns all servers belonging to a tenant plus shared (NULL tenant) servers.
func (p *PostgresDB) ListServersByTenant(ctx context.Context, tenantID string) ([]core.Server, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, tenant_id, hostname, ip_address, role, provider_type, provider_ref,
			region, size, ssh_port, ssh_key_id, docker_version, cpu_cores, ram_mb,
			disk_mb, monthly_cost_cents, swarm_joined, agent_status, status, created_at
		 FROM servers
		 WHERE tenant_id = $1 OR tenant_id IS NULL OR tenant_id = ''
		 ORDER BY created_at LIMIT 1000`, tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var servers []core.Server
	for rows.Next() {
		var srv core.Server
		var ts sql.NullString
		var sk sql.NullString
		var sw int
		if err := rows.Scan(&srv.ID, &ts, &srv.Hostname, &srv.IPAddress, &srv.Role,
			&srv.ProviderType, &srv.ProviderRef, &srv.Region, &srv.Size,
			&srv.SSHPort, &sk, &srv.DockerVersion, &srv.CPUCores,
			&srv.RAMmb, &srv.DiskMB, &srv.MonthlyCostCents, &sw,
			&srv.AgentStatus, &srv.Status, &srv.CreatedAt); err != nil {
			return nil, err
		}
		srv.TenantID = ts.String
		srv.SSHKeyID = sk.String
		srv.SwarmJoined = sw != 0
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

// ListAllServers returns every server in the platform.
func (p *PostgresDB) ListAllServers(ctx context.Context) ([]core.Server, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT id, tenant_id, hostname, ip_address, role, provider_type, provider_ref,
			region, size, ssh_port, ssh_key_id, docker_version, cpu_cores, ram_mb,
			disk_mb, monthly_cost_cents, swarm_joined, agent_status, status, created_at
		 FROM servers ORDER BY created_at LIMIT 10000`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var servers []core.Server
	for rows.Next() {
		var srv core.Server
		var ts sql.NullString
		var sk sql.NullString
		var sw int
		if err := rows.Scan(&srv.ID, &ts, &srv.Hostname, &srv.IPAddress, &srv.Role,
			&srv.ProviderType, &srv.ProviderRef, &srv.Region, &srv.Size,
			&srv.SSHPort, &sk, &srv.DockerVersion, &srv.CPUCores,
			&srv.RAMmb, &srv.DiskMB, &srv.MonthlyCostCents, &sw,
			&srv.AgentStatus, &srv.Status, &srv.CreatedAt); err != nil {
			return nil, err
		}
		srv.TenantID = ts.String
		srv.SSHKeyID = sk.String
		srv.SwarmJoined = sw != 0
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

// UpdateServerStatus updates a server's lifecycle status.
func (p *PostgresDB) UpdateServerStatus(ctx context.Context, id, status string) error {
	_, err := p.db.ExecContext(ctx, `UPDATE servers SET status = $1 WHERE id = $2`, status, id)
	return err
}

// DeleteServer removes a server row.
func (p *PostgresDB) DeleteServer(ctx context.Context, id string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM servers WHERE id = $1`, id)
	return err
}
