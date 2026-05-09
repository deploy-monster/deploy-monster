package db

import (
	"context"
	"database/sql"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

const serverColumns = `id, tenant_id, hostname, ip_address, role, provider_type, provider_ref,
		region, size, ssh_port, ssh_key_id, docker_version, cpu_cores, ram_mb, disk_mb,
		monthly_cost_cents, swarm_joined, agent_status, status, created_at`

func scanServer(rs interface {
	Scan(dest ...any) error
}, srv *core.Server) error {
	var swarm int
	var tenantID, sshKeyID sql.NullString
	err := rs.Scan(
		&srv.ID, &tenantID, &srv.Hostname, &srv.IPAddress, &srv.Role,
		&srv.ProviderType, &srv.ProviderRef, &srv.Region, &srv.Size,
		&srv.SSHPort, &sshKeyID, &srv.DockerVersion, &srv.CPUCores, &srv.RAMmb,
		&srv.DiskMB, &srv.MonthlyCostCents, &swarm, &srv.AgentStatus,
		&srv.Status, &srv.CreatedAt,
	)
	srv.TenantID = tenantID.String
	srv.SSHKeyID = sshKeyID.String
	srv.SwarmJoined = swarm != 0
	return err
}

// CreateServer inserts a new server row.
func (s *SQLiteDB) CreateServer(ctx context.Context, srv *core.Server) error {
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
	swarm := 0
	if srv.SwarmJoined {
		swarm = 1
	}

	tenantID := sql.NullString{String: srv.TenantID, Valid: srv.TenantID != ""}
	sshKeyID := sql.NullString{String: srv.SSHKeyID, Valid: srv.SSHKeyID != ""}

	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO servers (id, tenant_id, hostname, ip_address, role,
				provider_type, provider_ref, region, size, ssh_port, ssh_key_id,
				docker_version, cpu_cores, ram_mb, disk_mb, monthly_cost_cents,
				swarm_joined, agent_status, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			srv.ID, tenantID, srv.Hostname, srv.IPAddress, srv.Role,
			srv.ProviderType, srv.ProviderRef, srv.Region, srv.Size,
			srv.SSHPort, sshKeyID, srv.DockerVersion, srv.CPUCores,
			srv.RAMmb, srv.DiskMB, srv.MonthlyCostCents, swarm,
			srv.AgentStatus, srv.Status,
		)
		return err
	})
}

// GetServer retrieves a server by ID.
func (s *SQLiteDB) GetServer(ctx context.Context, id string) (*core.Server, error) {
	srv := &core.Server{}
	row := s.QueryRowContext(ctx,
		`SELECT `+serverColumns+` FROM servers WHERE id = ?`, id,
	)
	if err := scanServer(row, srv); err != nil {
		if err == sql.ErrNoRows {
			return nil, core.ErrNotFound
		}
		return nil, err
	}
	return srv, nil
}

// ListServersByTenant returns all servers attached to a tenant. Servers with a
// NULL tenant (platform-shared infrastructure) are also included so admins
// see hardware they can place workloads on regardless of tenancy.
func (s *SQLiteDB) ListServersByTenant(ctx context.Context, tenantID string) ([]core.Server, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT `+serverColumns+`
		 FROM servers
		 WHERE tenant_id = ? OR tenant_id IS NULL OR tenant_id = ''
		 ORDER BY created_at LIMIT 1000`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var servers []core.Server
	for rows.Next() {
		var srv core.Server
		if err := scanServer(rows, &srv); err != nil {
			return nil, err
		}
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

// ListAllServers returns every server in the platform (admin scope).
func (s *SQLiteDB) ListAllServers(ctx context.Context) ([]core.Server, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT `+serverColumns+` FROM servers ORDER BY created_at LIMIT 10000`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var servers []core.Server
	for rows.Next() {
		var srv core.Server
		if err := scanServer(rows, &srv); err != nil {
			return nil, err
		}
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

// UpdateServerStatus changes the lifecycle status of a server.
func (s *SQLiteDB) UpdateServerStatus(ctx context.Context, id, status string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE servers SET status = ? WHERE id = ?`, status, id)
		return err
	})
}

// DeleteServer removes a server row.
func (s *SQLiteDB) DeleteServer(ctx context.Context, id string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM servers WHERE id = ?`, id)
		return err
	})
}
