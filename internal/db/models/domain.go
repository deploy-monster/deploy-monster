package models

import "time"

// Domain represents a domain mapped to an application.
type Domain struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	FQDN        string    `json:"fqdn"`
	Type        string    `json:"type"`
	DNSProvider string    `json:"dns_provider"`
	DNSSynced   bool      `json:"dns_synced"`
	Verified    bool      `json:"verified"`
	CreatedAt   time.Time `json:"created_at"`
}

// SSLCert represents an SSL certificate for a domain.
type SSLCert struct {
	ID        string    `json:"id"`
	DomainID  string    `json:"domain_id"`
	CertPEM   string    `json:"-"`
	KeyPEMEnc string    `json:"-"`
	Issuer    string    `json:"issuer"`
	ExpiresAt time.Time `json:"expires_at"`
	AutoRenew bool      `json:"auto_renew"`
	CreatedAt time.Time `json:"created_at"`
}
