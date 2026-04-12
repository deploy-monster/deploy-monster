package db

import (
	"context"
	"database/sql"

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

// DeleteDomain removes a domain by ID.
func (s *SQLiteDB) DeleteDomain(ctx context.Context, id string) error {
	return s.Tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `DELETE FROM domains WHERE id = ?`, id)
		return err
	})
}
