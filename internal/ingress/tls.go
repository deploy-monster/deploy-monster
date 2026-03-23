package ingress

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// CertStore manages TLS certificates with thread-safe in-memory cache.
// Certificates are loaded on-demand via GetCertificate callback.
type CertStore struct {
	mu    sync.RWMutex
	certs map[string]*tls.Certificate // domain -> cert
}

// NewCertStore creates a new certificate store.
func NewCertStore() *CertStore {
	return &CertStore{
		certs: make(map[string]*tls.Certificate),
	}
}

// Put stores a certificate for a domain.
func (cs *CertStore) Put(domain string, cert *tls.Certificate) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.certs[domain] = cert
}

// Get retrieves a certificate for a domain.
func (cs *CertStore) Get(domain string) *tls.Certificate {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return cs.certs[domain]
}

// Remove deletes a certificate for a domain.
func (cs *CertStore) Remove(domain string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	delete(cs.certs, domain)
}

// GetCertificate implements the tls.Config.GetCertificate callback.
// It looks up the certificate by SNI hostname.
func (cs *CertStore) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert := cs.Get(hello.ServerName)
	if cert != nil {
		return cert, nil
	}
	// Fallback: generate self-signed cert for unknown domains (dev only)
	return nil, fmt.Errorf("no certificate for %s", hello.ServerName)
}

// GenerateSelfSigned creates a self-signed certificate for development.
func GenerateSelfSigned(domain string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate serial: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"DeployMonster Dev"},
			CommonName:   domain,
		},
		DNSNames:    []string{domain, "*." + domain},
		NotBefore:   time.Now(),
		NotAfter:    time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}

	return &cert, nil
}
