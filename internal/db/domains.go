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

// ListDomainsByApp returns all domains for an application.
func (s *SQLiteDB) ListDomainsByApp(ctx context.Context, appID string) ([]core.Domain, error) {
	rows, err := s.QueryContext(ctx,
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE app_id = ? ORDER BY created_at LIMIT 500`,
		appID,
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

// DeleteDomainsByApp removes all domains for an application. Returns the count deleted.
func (s *SQLiteDB) DeleteDomainsByApp(ctx context.Context, appID string) (int, error) {
	result, err := s.ExecContext(ctx, `DELETE FROM domains WHERE app_id = ?`, appID)
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

// DeleteDomain removes a domain by ID.
func (s *SQLiteDB) DeleteDomain(ctx context.Context, id string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM domains WHERE id = ?`, id)
		return err
	})
}

// ListDomainsByAppIDs retrieves domains for multiple apps in a single query.
// Returns a map from appID to its domains.
func (s *SQLiteDB) ListDomainsByAppIDs(ctx context.Context, appIDs []string) (map[string][]core.Domain, error) {
	if len(appIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(appIDs))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf(
		`SELECT id, app_id, fqdn, type, dns_provider, dns_synced, verified, created_at
		 FROM domains WHERE app_id IN (%s) ORDER BY created_at`,
		placeholders,
	)
	args := make([]any, len(appIDs))
	for i, id := range appIDs {
		args[i] = id
	}
	rows, err := s.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string][]core.Domain)
	for rows.Next() {
		var d core.Domain
		if err := rows.Scan(&d.ID, &d.AppID, &d.FQDN, &d.Type, &d.DNSProvider,
			&d.DNSSynced, &d.Verified, &d.CreatedAt); err != nil {
			return nil, err
		}
		result[d.AppID] = append(result[d.AppID], d)
	}
	return result, rows.Err()
}
