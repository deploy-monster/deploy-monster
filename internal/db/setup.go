package db

import (
	"context"
	"database/sql"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateTenantWithDefaults creates a tenant with a default project.
func (s *SQLiteDB) CreateTenantWithDefaults(ctx context.Context, name, slug string) (string, error) {
	tenantID := core.GenerateID()
	return tenantID, s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO tenants (id, name, slug, status) VALUES (?, ?, ?, 'active')`,
			tenantID, name, slug,
		)
		if err != nil {
			return err
		}

		// Create default project
		projectID := core.GenerateID()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO projects (id, tenant_id, name, description, environment)
			 VALUES (?, ?, 'Default', 'Default project', 'production')`,
			projectID, tenantID,
		)
		return err
	})
}

// CreateUserWithMembership creates a user and links them to a tenant with a role.
func (s *SQLiteDB) CreateUserWithMembership(ctx context.Context, email, passwordHash, name, status, tenantID, roleID string) (string, error) {
	userID := core.GenerateID()
	return userID, s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO users (id, email, password_hash, name, status) VALUES (?, ?, ?, ?, ?)`,
			userID, email, passwordHash, name, status,
		)
		if err != nil {
			return err
		}

		memberID := core.GenerateID()
		_, err = tx.ExecContext(ctx,
			`INSERT INTO team_members (id, tenant_id, user_id, role_id, status) VALUES (?, ?, ?, ?, 'active')`,
			memberID, tenantID, userID, roleID,
		)
		if err != nil {
			return err
		}

		// Update tenant owner
		_, err = tx.ExecContext(ctx,
			`UPDATE tenants SET owner_id = ? WHERE id = ?`,
			userID, tenantID,
		)
		return err
	})
}

// GetUserMembership returns the team membership (tenant + role) for a user.
func (s *SQLiteDB) GetUserMembership(ctx context.Context, userID string) (*core.TeamMember, error) {
	tm := &core.TeamMember{}
	err := s.QueryRowContext(ctx,
		`SELECT id, tenant_id, user_id, role_id, status, created_at
		 FROM team_members WHERE user_id = ? AND status = 'active' LIMIT 1`, userID,
	).Scan(&tm.ID, &tm.TenantID, &tm.UserID, &tm.RoleID, &tm.Status, &tm.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return tm, err
}

// GetRole retrieves a role by ID.
func (s *SQLiteDB) GetRole(ctx context.Context, roleID string) (*core.Role, error) {
	r := &core.Role{}
	err := s.QueryRowContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), name, description, permissions_json, is_builtin, created_at
		 FROM roles WHERE id = ?`, roleID,
	).Scan(&r.ID, &r.TenantID, &r.Name, &r.Description, &r.PermissionsJSON, &r.IsBuiltin, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return r, err
}

// ListRoles returns all roles for a tenant, including built-in roles.
func (s *SQLiteDB) ListRoles(ctx context.Context, tenantID string) ([]core.Role, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT id, COALESCE(tenant_id,''), name, description, permissions_json, is_builtin, created_at
		 FROM roles WHERE tenant_id = ? OR is_builtin = 1 ORDER BY is_builtin DESC, name LIMIT 500`,
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

// CreateProject inserts a new project.
func (s *SQLiteDB) CreateProject(ctx context.Context, p *core.Project) error {
	if p.ID == "" {
		p.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO projects (id, tenant_id, name, description, environment)
			 VALUES (?, ?, ?, ?, ?)`,
			p.ID, p.TenantID, p.Name, p.Description, p.Environment,
		)
		return err
	})
}

// GetProject retrieves a project by ID.
func (s *SQLiteDB) GetProject(ctx context.Context, id string) (*core.Project, error) {
	p := &core.Project{}
	err := s.QueryRowContext(ctx,
		`SELECT id, tenant_id, name, description, environment, created_at, updated_at
		 FROM projects WHERE id = ?`, id,
	).Scan(&p.ID, &p.TenantID, &p.Name, &p.Description, &p.Environment, &p.CreatedAt, &p.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return p, err
}

// ListProjectsByTenant returns all projects for a tenant.
func (s *SQLiteDB) ListProjectsByTenant(ctx context.Context, tenantID string) ([]core.Project, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT id, tenant_id, name, description, environment, created_at, updated_at
		 FROM projects WHERE tenant_id = ? ORDER BY name LIMIT 1000`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []core.Project
	for rows.Next() {
		var p core.Project
		if err := rows.Scan(&p.ID, &p.TenantID, &p.Name, &p.Description,
			&p.Environment, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// DeleteProject removes a project by ID.
func (s *SQLiteDB) DeleteProject(ctx context.Context, id string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM projects WHERE id = ?`, id)
		return err
	})
}

// CreateAuditLog inserts an audit log entry.
func (s *SQLiteDB) CreateAuditLog(ctx context.Context, entry *core.AuditEntry) error {
	_, err := s.ExecContext(ctx,
		`INSERT INTO audit_log (tenant_id, user_id, action, resource_type, resource_id, details_json, ip_address, user_agent)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.TenantID, entry.UserID, entry.Action, entry.ResourceType,
		entry.ResourceID, entry.DetailsJSON, entry.IPAddress, entry.UserAgent,
	)
	return err
}

// ListAuditLogs returns audit log entries for a tenant with pagination.
func (s *SQLiteDB) ListAuditLogs(ctx context.Context, tenantID string, limit, offset int) ([]core.AuditEntry, int, error) {
	var total int
	err := s.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_log WHERE tenant_id = ?`, tenantID,
	).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.QueryContext(ctx,
		`SELECT id, tenant_id, user_id, action, resource_type, resource_id, details_json,
		        ip_address, user_agent, created_at
		 FROM audit_log WHERE tenant_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
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
