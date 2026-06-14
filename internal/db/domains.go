package db

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// CreateDomain inserts a new domain.
func (s *SQLiteDB) CreateDomain(ctx context.Context, d *core.Domain) error {
	if d.ID == "" {
		d.ID = core.GenerateID()
	}
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO domains (id, app_id, fqdn, type, dns_provider, dns_synced, verified)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			d.ID, d.AppID, d.FQDN, d.Type, d.DNSProvider, d.DNSSynced, d.Verified,
		)
		return err
	})
}

// GetDomainByFQDN retrieves a domain by its fully qualified domain name.
func (s *SQLiteDB) GetDomainByFQDN(ctx context.Context, fqdn string) (*core.Domain, error) {
	d := &core.Domain{}
	err := s.QueryRowContext(ctx,
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE fqdn = ?`, fqdn,
	).Scan(&d.ID, &d.AppID, &d.FQDN, &d.Type, &d.DNSProvider, &d.DNSSynced, &d.Verified, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return d, err
}

// GetDomain retrieves a domain by its ID.
func (s *SQLiteDB) GetDomain(ctx context.Context, id string) (*core.Domain, error) {
	d := &core.Domain{}
	err := s.QueryRowContext(ctx,
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE id = ?`, id,
	).Scan(&d.ID, &d.AppID, &d.FQDN, &d.Type, &d.DNSProvider, &d.DNSSynced, &d.Verified, &d.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, core.ErrNotFound
	}
	return d, err
}

// ListDomainsByApp returns all domains for an application, scoped to tenantID.
func (s *SQLiteDB) ListDomainsByApp(ctx context.Context, appID, tenantID string) ([]core.Domain, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT d.id, d.app_id, d.fqdn, d.type, d.dns_provider, d.dns_synced, d.verified, d.created_at
		 FROM domains d
		 JOIN applications a ON a.id = d.app_id
		 WHERE d.app_id = ? AND a.tenant_id = ?
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

// DeleteDomainsByApp removes all domains for an application, scoped to tenantID.
func (s *SQLiteDB) DeleteDomainsByApp(ctx context.Context, appID, tenantID string) (int, error) {
	result, err := s.ExecContext(ctx,
		`DELETE FROM domains WHERE app_id = ? AND app_id IN (
			SELECT id FROM applications WHERE id = ? AND tenant_id = ?
		)`,
		appID, appID, tenantID)
	if err != nil {
		return 0, err
	}
	n, _ := result.RowsAffected()
	return int(n), nil
}

// ListAllDomains returns all domains across all applications.
func (s *SQLiteDB) ListAllDomains(ctx context.Context) ([]core.Domain, error) {
	rows, err := s.QueryContext(ctx,
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

// DeleteDomain removes a domain by ID, scoped to tenantID.
func (s *SQLiteDB) DeleteDomain(ctx context.Context, id, tenantID string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`DELETE FROM domains WHERE id = ? AND app_id IN (
				SELECT id FROM applications WHERE tenant_id = ?
			)`,
			id, tenantID)
		return err
	})
}

// ListDomainsByAppIDs retrieves domains for multiple apps in a single query,
// scoped to tenantID. Only returns domains for apps owned by the tenant.
func (s *SQLiteDB) ListDomainsByAppIDs(ctx context.Context, appIDs []string, tenantID string) (map[string][]core.Domain, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	// Filter appIDs to only those belonging to the tenant.
	allowedPlaceholders := strings.Repeat("?,", len(appIDs))
	allowedPlaceholders = allowedPlaceholders[:len(allowedPlaceholders)-1]
	allowedQuery := fmt.Sprintf(
		`SELECT id FROM applications WHERE id IN (%s) AND tenant_id = ?`,
		allowedPlaceholders,
	)
	args := make([]any, len(appIDs)+1)
	for i, id := range appIDs {
		args[i] = id
	}
	args[len(appIDs)] = tenantID
	rows, err := s.QueryContext(ctx, allowedQuery, args...)
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
	domainPlaceholders := strings.Repeat("?,", len(allowedIDs))
	domainPlaceholders = domainPlaceholders[:len(domainPlaceholders)-1]
	domainQuery := fmt.Sprintf(
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE app_id IN (%s) ORDER BY created_at`,
		domainPlaceholders,
	)
	domainArgs := make([]any, len(allowedIDs))
	for i, id := range allowedIDs {
		domainArgs[i] = id
	}
	domainRows, err := s.QueryContext(ctx, domainQuery, domainArgs...)
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
